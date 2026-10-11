package guardsql_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/grokify/guardsql"
)

func scopeSchema() guardsql.Schema {
	f := func(t guardsql.FieldType) guardsql.Field {
		return guardsql.Field{Type: t, Selectable: true, Filterable: true, Sortable: true}
	}
	w := func(t guardsql.FieldType) guardsql.Field {
		return guardsql.Field{Type: t, Selectable: true, Filterable: true, Sortable: true, Writable: true}
	}
	return guardsql.Schema{Entities: map[string]guardsql.Entity{
		"events": {Name: "events", Fields: map[string]guardsql.Field{
			"id":        f(guardsql.FieldNumber),
			"status":    w(guardsql.FieldString),
			"tenant_id": f(guardsql.FieldNumber),
		}},
		// A public table with no tenant column.
		"regions": {Name: "regions", Fields: map[string]guardsql.Field{
			"id":   f(guardsql.FieldNumber),
			"name": f(guardsql.FieldString),
		}},
	}}
}

func untrustedScoped(scope guardsql.ScopeFunc) guardsql.Config {
	cfg := guardsql.NewUntrustedConfig(scopeSchema(), guardsql.DialectPostgres)
	cfg.RowScope = scope
	return cfg
}

func TestScopeAppendedToRead(t *testing.T) {
	cfg := untrustedScoped(guardsql.ScopeEquals("tenant_id", 5))
	out, err := cfg.Compile(`SELECT id FROM events WHERE status = "open" LIMIT 10`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	want := `SELECT "id" FROM "events" WHERE ("status" = $1 AND "tenant_id" = $2) LIMIT 10`
	if out.SQL != want {
		t.Errorf("SQL:\n got %s\nwant %s", out.SQL, want)
	}
	if !reflect.DeepEqual(out.Args, []any{"open", int64(5)}) {
		t.Errorf("args = %#v", out.Args)
	}
}

func TestScopeAppendedWithNoUserWhere(t *testing.T) {
	cfg := untrustedScoped(guardsql.ScopeEquals("tenant_id", 5))
	out, err := cfg.Compile(`SELECT id FROM events LIMIT 10`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	want := `SELECT "id" FROM "events" WHERE "tenant_id" = $1 LIMIT 10`
	if out.SQL != want {
		t.Errorf("SQL:\n got %s\nwant %s", out.SQL, want)
	}
}

// The IDOR case: even when the user references the scope column and tries to
// widen with OR, the top-level AND keeps the mandatory scope in force.
func TestScopeCannotBeWidened(t *testing.T) {
	cfg := untrustedScoped(guardsql.ScopeEquals("tenant_id", 5))
	out, err := cfg.Compile(`SELECT id FROM events WHERE tenant_id = 6 OR id > 0 LIMIT 10`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	want := `SELECT "id" FROM "events" WHERE (("tenant_id" = $1 OR "id" > $2) AND "tenant_id" = $3) LIMIT 10`
	if out.SQL != want {
		t.Errorf("SQL:\n got %s\nwant %s", out.SQL, want)
	}
	// The mandatory tenant_id = 5 is always the last, top-level AND term.
	if out.Args[len(out.Args)-1] != int64(5) {
		t.Errorf("mandatory scope value missing/last-arg wrong: %#v", out.Args)
	}
}

// Fail-closed: a scope column the entity lacks refuses to compile rather than
// silently returning unscoped data.
func TestScopeFailsClosedOnMissingColumn(t *testing.T) {
	cfg := untrustedScoped(guardsql.ScopeEquals("tenant_id", 5))
	if _, err := cfg.Compile(`SELECT id FROM regions LIMIT 10`); err == nil {
		t.Error("expected fail-closed error when scope column is absent")
	}
}

// Per-entity exemption: the scope function may return (nil, nil) for a public
// entity, which compiles with no added predicate.
func TestScopePerEntityExemption(t *testing.T) {
	scope := func(entity string) (guardsql.Expr, error) {
		if entity == "regions" {
			return nil, nil
		}
		return guardsql.Eq("tenant_id", 5), nil
	}
	cfg := untrustedScoped(scope)
	out, err := cfg.Compile(`SELECT id FROM regions LIMIT 10`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if strings.Contains(out.SQL, "WHERE") {
		t.Errorf("exempt entity should have no scope WHERE: %s", out.SQL)
	}
}

// RequireRowScope refuses to compile when no scope function is configured.
func TestRequireRowScopeFailsClosed(t *testing.T) {
	cfg := guardsql.NewUntrustedConfig(scopeSchema(), guardsql.DialectPostgres)
	cfg.RequireRowScope = true // but no RowScope set
	if _, err := cfg.Compile(`SELECT id FROM events LIMIT 10`); err == nil {
		t.Error("RequireRowScope must refuse compilation without a scope function")
	}
}

func TestScopeAppliedToMutations(t *testing.T) {
	tcfg := guardsql.NewTrustedConfig(scopeSchema(), guardsql.DialectPostgres)
	tcfg.RowScope = guardsql.ScopeEquals("tenant_id", 5)

	upd, err := guardsql.Update("events").Set("status", "closed").Where(guardsql.Eq("id", 1)).Compile(tcfg)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	wantUpd := `UPDATE "events" SET "status" = $1 WHERE ("id" = $2 AND "tenant_id" = $3)`
	if upd.SQL != wantUpd {
		t.Errorf("UPDATE:\n got %s\nwant %s", upd.SQL, wantUpd)
	}

	del, err := guardsql.Delete("events").Where(guardsql.Eq("id", 1)).Compile(tcfg)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	wantDel := `DELETE FROM "events" WHERE ("id" = $1 AND "tenant_id" = $2)`
	if del.SQL != wantDel {
		t.Errorf("DELETE:\n got %s\nwant %s", del.SQL, wantDel)
	}
}
