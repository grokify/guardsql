package guardsql_test

import (
	"reflect"
	"testing"

	"github.com/grokify/guardsql"
)

func testSchema() guardsql.Schema {
	f := func(t guardsql.FieldType) guardsql.Field {
		return guardsql.Field{Type: t, Selectable: true, Filterable: true, Sortable: true}
	}
	return guardsql.Schema{Entities: map[string]guardsql.Entity{
		"events": {Name: "events", Fields: map[string]guardsql.Field{
			"status_id":   f(guardsql.FieldNumber),
			"severity_id": f(guardsql.FieldNumber),
			"source":      f(guardsql.FieldString),
			"payload":     f(guardsql.FieldJSON),
		}},
	}}
}

// compile parses, validates, and compiles input for a dialect.
func compile(t *testing.T, input string, d guardsql.Dialect) guardsql.CompiledSQL {
	t.Helper()
	schema := testSchema()
	q, err := guardsql.Parse(input)
	if err != nil {
		t.Fatalf("Parse(%q): %v", input, err)
	}
	if err := guardsql.Validate(q, schema); err != nil {
		t.Fatalf("Validate(%q): %v", input, err)
	}
	out, err := guardsql.Compile(q, guardsql.CompileOptions{Dialect: d, Schema: schema})
	if err != nil {
		t.Fatalf("Compile(%q): %v", input, err)
	}
	return out
}

func TestCompileAggregateGroupOrder(t *testing.T) {
	out := compile(t, `SELECT status_id, count(*) AS findings FROM events GROUP BY status_id ORDER BY findings DESC LIMIT 50`, guardsql.DialectSQLite)
	want := `SELECT "status_id", COUNT(*) AS "findings" FROM "events" GROUP BY "status_id" ORDER BY "findings" DESC LIMIT 50`
	if out.SQL != want {
		t.Errorf("SQL:\n got %s\nwant %s", out.SQL, want)
	}
	if len(out.Args) != 0 {
		t.Errorf("args = %v, want none", out.Args)
	}
}

func TestCompileJSONPathSQLite(t *testing.T) {
	out := compile(t, `SELECT payload.finding_info.title AS t, count(*) AS n FROM events GROUP BY payload.finding_info.title`, guardsql.DialectSQLite)
	want := `SELECT json_extract("payload", ?) AS "t", COUNT(*) AS "n" FROM "events" GROUP BY json_extract("payload", ?)`
	if out.SQL != want {
		t.Errorf("SQL:\n got %s\nwant %s", out.SQL, want)
	}
	if !reflect.DeepEqual(out.Args, []any{"$.finding_info.title", "$.finding_info.title"}) {
		t.Errorf("args = %#v", out.Args)
	}
}

func TestCompileJSONPathPostgres(t *testing.T) {
	out := compile(t, `SELECT payload.finding_info.title AS t FROM events LIMIT 10`, guardsql.DialectPostgres)
	want := `SELECT jsonb_extract_path_text("payload"::jsonb, $1, $2) AS "t" FROM "events" LIMIT 10`
	if out.SQL != want {
		t.Errorf("SQL:\n got %s\nwant %s", out.SQL, want)
	}
	if !reflect.DeepEqual(out.Args, []any{"finding_info", "title"}) {
		t.Errorf("args = %#v", out.Args)
	}
}

func TestCompileJSONPathMySQL(t *testing.T) {
	out := compile(t, `SELECT payload.finding_info.title AS t FROM events LIMIT 10`, guardsql.DialectMySQL)
	want := "SELECT JSON_UNQUOTE(JSON_EXTRACT(`payload`, ?)) AS `t` FROM `events` LIMIT 10"
	if out.SQL != want {
		t.Errorf("SQL:\n got %s\nwant %s", out.SQL, want)
	}
	if !reflect.DeepEqual(out.Args, []any{"$.finding_info.title"}) {
		t.Errorf("args = %#v", out.Args)
	}
}

func TestCompileWhereParameterized(t *testing.T) {
	out := compile(t, `SELECT source FROM events WHERE status_id = 1 AND severity_id IN (2, 3) LIMIT 5`, guardsql.DialectPostgres)
	want := `SELECT "source" FROM "events" WHERE ("status_id" = $1 AND "severity_id" IN ($2, $3)) LIMIT 5`
	if out.SQL != want {
		t.Errorf("SQL:\n got %s\nwant %s", out.SQL, want)
	}
	if !reflect.DeepEqual(out.Args, []any{int64(1), int64(2), int64(3)}) {
		t.Errorf("args = %#v", out.Args)
	}
}

// A malicious string value must become a bind parameter, never inlined SQL.
func TestCompileInjectionNeutralized(t *testing.T) {
	out := compile(t, `SELECT source FROM events WHERE source = "x' OR '1'='1" LIMIT 10`, guardsql.DialectSQLite)
	want := `SELECT "source" FROM "events" WHERE "source" = ? LIMIT 10`
	if out.SQL != want {
		t.Errorf("SQL:\n got %s\nwant %s", out.SQL, want)
	}
	if len(out.Args) != 1 || out.Args[0] != "x' OR '1'='1" {
		t.Errorf("args = %#v, want the raw string as one bound param", out.Args)
	}
}

func TestCompileContainsEscapesLike(t *testing.T) {
	out := compile(t, `SELECT source FROM events WHERE source CONTAINS "50%_off" LIMIT 10`, guardsql.DialectSQLite)
	want := `SELECT "source" FROM "events" WHERE "source" LIKE ? ESCAPE '\' LIMIT 10`
	if out.SQL != want {
		t.Errorf("SQL:\n got %s\nwant %s", out.SQL, want)
	}
	if len(out.Args) != 1 || out.Args[0] != `%50\%\_off%` {
		t.Errorf("args = %#v, want LIKE metacharacters escaped", out.Args)
	}
}

func TestCompileRejectsUnknownField(t *testing.T) {
	schema := testSchema()
	q, err := guardsql.Parse(`SELECT nope FROM events LIMIT 1`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Validation should already reject it; the compiler must too if reached.
	if err := guardsql.Validate(q, schema); err == nil {
		t.Error("Validate accepted unknown field")
	}
	if _, err := guardsql.Compile(q, guardsql.CompileOptions{Dialect: guardsql.DialectSQLite, Schema: schema}); err == nil {
		t.Error("Compile accepted unknown field")
	}
}

func TestCompileRequiresDialect(t *testing.T) {
	schema := testSchema()
	q, _ := guardsql.Parse(`SELECT source FROM events LIMIT 1`)
	if _, err := guardsql.Compile(q, guardsql.CompileOptions{Schema: schema}); err == nil {
		t.Error("expected error for missing dialect")
	}
}
