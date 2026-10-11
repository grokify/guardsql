package guardsql_test

import (
	"strings"
	"testing"

	"github.com/grokify/guardsql"
)

func classifiedConfig(allowPII bool) guardsql.Config {
	f := func(t guardsql.FieldType, class guardsql.DataClass) guardsql.Field {
		return guardsql.Field{Type: t, Selectable: true, Filterable: true, Sortable: true, Class: class}
	}
	return guardsql.Config{
		Dialect:  guardsql.DialectSQLite,
		AllowPII: allowPII,
		Schema: guardsql.Schema{Entities: map[string]guardsql.Entity{
			"users": {Name: "users", Fields: map[string]guardsql.Field{
				"id":            f(guardsql.FieldNumber, guardsql.ClassPublic),
				"email":         f(guardsql.FieldString, guardsql.ClassPII),
				"password_hash": f(guardsql.FieldString, guardsql.ClassSecret),
			}},
		}},
	}
}

func TestConfigCompilePublic(t *testing.T) {
	out, err := classifiedConfig(false).Compile(`SELECT id FROM users LIMIT 10`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if out.SQL != `SELECT "id" FROM "users" LIMIT 10` {
		t.Errorf("SQL = %s", out.SQL)
	}
}

func TestConfigSecretColumnDenied(t *testing.T) {
	_, err := classifiedConfig(true).Compile(`SELECT password_hash FROM users LIMIT 10`)
	if err == nil {
		t.Fatal("expected secret column to be denied")
	}
	// Denied as if non-existent (does not disclose the column exists).
	if !strings.Contains(err.Error(), "unknown select field") {
		t.Errorf("err = %v, want unknown-field style denial", err)
	}
}

func TestConfigPIIDeniedByDefault(t *testing.T) {
	_, err := classifiedConfig(false).Compile(`SELECT email FROM users LIMIT 10`)
	if err == nil {
		t.Fatal("expected PII column to be denied without AllowPII")
	}
}

func TestConfigPIIAllowedWhenEnabled(t *testing.T) {
	out, err := classifiedConfig(true).Compile(`SELECT email FROM users LIMIT 10`)
	if err != nil {
		t.Fatalf("Compile with AllowPII: %v", err)
	}
	if out.SQL != `SELECT "email" FROM "users" LIMIT 10` {
		t.Errorf("SQL = %s", out.SQL)
	}
}

func TestConfigRejectsUnsafeSchemaIdentifier(t *testing.T) {
	cfg := guardsql.Config{
		Dialect: guardsql.DialectSQLite,
		Schema: guardsql.Schema{Entities: map[string]guardsql.Entity{
			"events": {Name: "events", Fields: map[string]guardsql.Field{
				// A hostile column name that quoting alone would have to contain.
				`x"; DROP TABLE users; --`: {Type: guardsql.FieldString, Selectable: true},
			}},
		}},
	}
	if _, err := cfg.Compile(`SELECT id FROM events LIMIT 1`); err == nil {
		t.Fatal("expected unsafe schema identifier to be rejected by default")
	}

	cfg.AllowUnsafeIdentifiers = true
	// Now the identifier check is bypassed; the query still fails only because
	// it references a different (unknown) field, proving the gate was the
	// identifier policy, not the query.
	if _, err := cfg.Compile(`SELECT id FROM events LIMIT 1`); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Errorf("with AllowUnsafeIdentifiers, expected unknown-field error, got %v", err)
	}
}

func trustSchema() guardsql.Schema {
	f := func(t guardsql.FieldType, class guardsql.DataClass) guardsql.Field {
		return guardsql.Field{Type: t, Selectable: true, Filterable: true, Sortable: true, Class: class}
	}
	return guardsql.Schema{Entities: map[string]guardsql.Entity{
		"users": {Name: "users", Fields: map[string]guardsql.Field{
			"id":            f(guardsql.FieldNumber, guardsql.ClassPublic),
			"email":         f(guardsql.FieldString, guardsql.ClassPII),
			"password_hash": f(guardsql.FieldString, guardsql.ClassSecret),
		}},
	}}
}

// The same secret/PII query is denied under the untrusted profile and allowed
// under the trusted profile — the AST-step stripping is the only difference.
func TestTrustProfiles(t *testing.T) {
	const q = `SELECT id, email, password_hash FROM users LIMIT 10`

	if _, err := guardsql.NewUntrustedConfig(trustSchema(), guardsql.DialectSQLite).Compile(q); err == nil {
		t.Error("untrusted profile must strip secret/PII columns")
	}

	out, err := guardsql.NewTrustedConfig(trustSchema(), guardsql.DialectSQLite).Compile(q)
	if err != nil {
		t.Fatalf("trusted profile: %v", err)
	}
	for _, col := range []string{`"email"`, `"password_hash"`} {
		if !strings.Contains(out.SQL, col) {
			t.Errorf("trusted SQL missing %s: %s", col, out.SQL)
		}
	}
}

// Untrusted profile requires a LIMIT and forbids non-read operations by policy.
func TestUntrustedRequiresLimit(t *testing.T) {
	if _, err := guardsql.NewUntrustedConfig(trustSchema(), guardsql.DialectSQLite).Compile(`SELECT id FROM users`); err == nil {
		t.Error("untrusted profile must require a LIMIT")
	}
}

func TestIsSafeIdentifier(t *testing.T) {
	safe := []string{"id", "status_id", "_x", "Payload", "a1"}
	unsafe := []string{"", "1col", "a b", `a"b`, "a;b", "a-b", "café", strings.Repeat("a", 129)}
	for _, s := range safe {
		if !guardsql.IsSafeIdentifier(s) {
			t.Errorf("IsSafeIdentifier(%q) = false, want true", s)
		}
	}
	for _, s := range unsafe {
		if guardsql.IsSafeIdentifier(s) {
			t.Errorf("IsSafeIdentifier(%q) = true, want false", s)
		}
	}
}

func TestConfigInternalColumnDeniedByDefault(t *testing.T) {
	cfg := guardsql.NewUntrustedConfig(guardsql.Schema{Entities: map[string]guardsql.Entity{
		"users": {Name: "users", Fields: map[string]guardsql.Field{
			"id":     {Type: guardsql.FieldNumber, Selectable: true, Filterable: true, Sortable: true},
			"region": {Type: guardsql.FieldString, Selectable: true, Filterable: true, Sortable: true, Class: guardsql.ClassInternal},
		}},
	}}, guardsql.DialectSQLite)

	if _, err := cfg.Compile(`SELECT region FROM users LIMIT 10`); err == nil ||
		!strings.Contains(err.Error(), "unknown select field") {
		t.Fatalf("untrusted internal select err = %v, want unknown-field denial", err)
	}
	if _, err := cfg.Compile(`SELECT id FROM users WHERE region = 'x' LIMIT 10`); err == nil {
		t.Fatal("untrusted internal filter should be denied")
	}

	cfg.AllowInternal = true
	if _, err := cfg.Compile(`SELECT region FROM users LIMIT 10`); err != nil {
		t.Fatalf("AllowInternal compile: %v", err)
	}
}

func TestConfigProfilesInternalFlag(t *testing.T) {
	s := guardsql.Schema{}
	if guardsql.NewUntrustedConfig(s, guardsql.DialectSQLite).AllowInternal {
		t.Error("untrusted profile must not allow internal")
	}
	if !guardsql.NewTrustedConfig(s, guardsql.DialectSQLite).AllowInternal {
		t.Error("trusted profile must allow internal")
	}
}
