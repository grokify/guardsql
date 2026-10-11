package guardsql_test

import (
	"reflect"
	"testing"

	"github.com/grokify/guardsql"
)

func mutationSchema() guardsql.Schema {
	// rw is readable + writable; ro is read-only (not a valid mutation target).
	rw := func(t guardsql.FieldType, class guardsql.DataClass) guardsql.Field {
		return guardsql.Field{Type: t, Selectable: true, Filterable: true, Sortable: true, Writable: true, Class: class}
	}
	ro := func(t guardsql.FieldType, class guardsql.DataClass) guardsql.Field {
		return guardsql.Field{Type: t, Selectable: true, Filterable: true, Sortable: true, Class: class}
	}
	return guardsql.Schema{Entities: map[string]guardsql.Entity{
		"users": {Name: "users", Fields: map[string]guardsql.Field{
			"id":            ro(guardsql.FieldNumber, guardsql.ClassPublic),
			"email":         rw(guardsql.FieldString, guardsql.ClassPII),
			"status":        rw(guardsql.FieldString, guardsql.ClassPublic),
			"password_hash": rw(guardsql.FieldString, guardsql.ClassSecret),
		}},
	}}
}

func TestInsertTrusted(t *testing.T) {
	cfg := guardsql.NewTrustedConfig(mutationSchema(), guardsql.DialectPostgres)
	out, err := guardsql.Insert("users").
		Set("email", "a@example.com").
		Set("password_hash", "hunter2").
		Compile(cfg)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	want := `INSERT INTO "users" ("email", "password_hash") VALUES ($1, $2)`
	if out.SQL != want {
		t.Errorf("SQL:\n got %s\nwant %s", out.SQL, want)
	}
	if !reflect.DeepEqual(out.Args, []any{"a@example.com", "hunter2"}) {
		t.Errorf("args = %#v", out.Args)
	}
}

func TestUpdateRequiresWhere(t *testing.T) {
	cfg := guardsql.NewTrustedConfig(mutationSchema(), guardsql.DialectSQLite)
	if _, err := guardsql.Update("users").Set("status", "active").Compile(cfg); err == nil {
		t.Error("UPDATE without WHERE must be rejected by default")
	}

	out, err := guardsql.Update("users").
		Set("status", "active").
		Where(guardsql.Eq("id", 42)).
		Compile(cfg)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	want := `UPDATE "users" SET "status" = ? WHERE "id" = ?`
	if out.SQL != want {
		t.Errorf("SQL:\n got %s\nwant %s", out.SQL, want)
	}
	if !reflect.DeepEqual(out.Args, []any{"active", int64(42)}) {
		t.Errorf("args = %#v", out.Args)
	}
}

func TestDeleteWithCompoundWhere(t *testing.T) {
	cfg := guardsql.NewTrustedConfig(mutationSchema(), guardsql.DialectPostgres)
	out, err := guardsql.Delete("users").
		Where(guardsql.And(guardsql.Eq("status", "disabled"), guardsql.Lt("id", 100))).
		Compile(cfg)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	want := `DELETE FROM "users" WHERE ("status" = $1 AND "id" < $2)`
	if out.SQL != want {
		t.Errorf("SQL:\n got %s\nwant %s", out.SQL, want)
	}
	if !reflect.DeepEqual(out.Args, []any{"disabled", int64(100)}) {
		t.Errorf("args = %#v", out.Args)
	}
}

// The untrusted profile must refuse mutations entirely (policy allows only read).
func TestUntrustedCannotMutate(t *testing.T) {
	cfg := guardsql.NewUntrustedConfig(mutationSchema(), guardsql.DialectSQLite)
	if _, err := guardsql.Insert("users").Set("status", "x").Compile(cfg); err == nil {
		t.Error("untrusted profile must not permit INSERT")
	}
	if _, err := guardsql.Delete("users").Where(guardsql.Eq("id", 1)).Compile(cfg); err == nil {
		t.Error("untrusted profile must not permit DELETE")
	}
}

// A zero Config permits no mutations (deny-by-default): AllowedOps is empty.
func TestZeroConfigDeniesMutation(t *testing.T) {
	cfg := guardsql.Config{Dialect: guardsql.DialectSQLite, Schema: mutationSchema()}
	if _, err := guardsql.Insert("users").Set("status", "x").Compile(cfg); err == nil {
		t.Error("zero Config must deny mutations")
	}
}

func TestMutationUnknownColumnRejected(t *testing.T) {
	cfg := guardsql.NewTrustedConfig(mutationSchema(), guardsql.DialectSQLite)
	if _, err := guardsql.Insert("users").Set("nope", 1).Compile(cfg); err == nil {
		t.Error("unknown column must be rejected")
	}
}

// Mass-assignment defense: a column that is readable but not marked Writable
// cannot be an assignment target, even for a trusted caller. Here "id" is
// read-only in the schema.
func TestMutationRejectsNonWritableColumn(t *testing.T) {
	cfg := guardsql.NewTrustedConfig(mutationSchema(), guardsql.DialectSQLite)

	if _, err := guardsql.Insert("users").Set("id", 999).Set("email", "a@b.c").Compile(cfg); err == nil {
		t.Error("INSERT into a non-writable column must be rejected")
	}
	if _, err := guardsql.Update("users").Set("id", 999).Where(guardsql.Eq("email", "a@b.c")).Compile(cfg); err == nil {
		t.Error("UPDATE of a non-writable column must be rejected")
	}
	// A writable column still works.
	if _, err := guardsql.Update("users").Set("status", "active").Where(guardsql.Eq("id", 1)).Compile(cfg); err != nil {
		t.Errorf("writable column should compile: %v", err)
	}
}

// A malicious value in a mutation is a bound parameter, never inlined.
func TestMutationValueParameterized(t *testing.T) {
	cfg := guardsql.NewTrustedConfig(mutationSchema(), guardsql.DialectSQLite)
	out, err := guardsql.Update("users").
		Set("status", "x'; DROP TABLE users; --").
		Where(guardsql.Eq("id", 1)).
		Compile(cfg)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if out.SQL != `UPDATE "users" SET "status" = ? WHERE "id" = ?` {
		t.Errorf("SQL = %s", out.SQL)
	}
	if out.Args[0] != "x'; DROP TABLE users; --" {
		t.Errorf("value not bound as parameter: %#v", out.Args)
	}
}
