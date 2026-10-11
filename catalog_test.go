package guardsql

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/grokify/datacatalog"
)

// comboFields returns a field for every valid
// visibility/pii/secret combination, plus the invalid secret+public one when
// includeInvalid is set.
func comboFields(includeInvalid bool) []datacatalog.Field {
	var out []datacatalog.Field
	for _, vis := range []datacatalog.Visibility{datacatalog.VisibilityPublic, datacatalog.VisibilityInternal} {
		for _, pii := range []bool{false, true} {
			for _, secret := range []bool{false, true} {
				if secret && vis == datacatalog.VisibilityPublic && !includeInvalid {
					continue
				}
				out = append(out, datacatalog.Field{
					Name:       fmt.Sprintf("f_%s_pii%t_secret%t", vis, pii, secret),
					Type:       datacatalog.TypeString,
					Visibility: vis,
					PII:        pii,
					Secret:     secret,
				})
			}
		}
	}
	return out
}

func surviving(s Schema, entity string) []string {
	var names []string
	for n := range s.Entities[entity].Fields {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func allowed(c *datacatalog.Catalog, allowPII bool) []string {
	var names []string
	for _, f := range c.Entities[0].QueryableFields(allowPII) {
		names = append(names, strings.ToLower(f.Name))
	}
	sort.Strings(names)
	return names
}

func TestFromCatalogMatchesAllowedInQuery(t *testing.T) {
	// Every field, including the secret+public combination catalog validation
	// rejects, is checked through the shared mapping so a misclassified field
	// still fails closed.
	all := comboFields(true)
	s := Schema{Entities: map[string]Entity{"t": {Name: "t", Fields: map[string]Field{}}}}
	for _, f := range all {
		s.Entities["t"].Fields[f.Name] = Field{Name: f.Name, Selectable: true, Filterable: true, Sortable: true, Class: classFromCatalog(f)}
	}
	for _, allowPII := range []bool{false, true} {
		cfg := NewUntrustedConfig(s, DialectSQLite)
		cfg.AllowPII = allowPII
		eff := cfg.effectiveSchema()
		for _, f := range all {
			_, got := eff.Entities["t"].Fields[f.Name]
			if want := f.AllowedInQuery(allowPII); got != want {
				t.Errorf("%s allowPII=%t: survives=%t, AllowedInQuery=%t", f.Name, allowPII, got, want)
			}
		}
	}
}

func TestFromCatalogEffectiveSchemaEqualsQueryable(t *testing.T) {
	c := &datacatalog.Catalog{Name: "app", Entities: []datacatalog.Entity{
		{Name: "accounts", Fields: comboFields(false)},
	}}
	s, err := FromCatalog(c)
	if err != nil {
		t.Fatalf("FromCatalog: %v", err)
	}
	for _, allowPII := range []bool{false, true} {
		cfg := NewUntrustedConfig(s, DialectSQLite)
		cfg.AllowPII = allowPII
		got := surviving(cfg.effectiveSchema(), "accounts")
		want := allowed(c, allowPII)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("allowPII=%t: effective %v != AllowedInQuery %v", allowPII, got, want)
		}
	}

	// The trusted profile admits every field.
	trusted := NewTrustedConfig(s, DialectSQLite).effectiveSchema()
	if n := len(trusted.Entities["accounts"].Fields); n != len(c.Entities[0].Fields) {
		t.Errorf("trusted fields = %d, want %d", n, len(c.Entities[0].Fields))
	}
}

func TestFromCatalogClassPrecedence(t *testing.T) {
	cases := []struct {
		name string
		f    datacatalog.Field
		want DataClass
	}{
		{"public", datacatalog.Field{Visibility: datacatalog.VisibilityPublic}, ClassPublic},
		{"pii", datacatalog.Field{Visibility: datacatalog.VisibilityPublic, PII: true}, ClassPII},
		{"internal", datacatalog.Field{Visibility: datacatalog.VisibilityInternal}, ClassInternal},
		{"internal pii", datacatalog.Field{Visibility: datacatalog.VisibilityInternal, PII: true}, ClassInternal},
		{"secret", datacatalog.Field{Visibility: datacatalog.VisibilityInternal, Secret: true}, ClassSecret},
		{"secret pii", datacatalog.Field{Visibility: datacatalog.VisibilityInternal, Secret: true, PII: true}, ClassSecret},
		{"secret public (invalid)", datacatalog.Field{Visibility: datacatalog.VisibilityPublic, Secret: true}, ClassSecret},
		{"unclassified fails closed", datacatalog.Field{}, ClassInternal},
	}
	for _, tc := range cases {
		if got := classFromCatalog(tc.f); got != tc.want {
			t.Errorf("%s: class = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestFromCatalogRejectsInvalidCatalog(t *testing.T) {
	c := &datacatalog.Catalog{Name: "app", Entities: []datacatalog.Entity{
		{Name: "users", Fields: []datacatalog.Field{{Name: "id"}}}, // no visibility
	}}
	_, err := FromCatalog(c)
	var verr *datacatalog.ValidationError
	if err == nil || !asValidationError(err, &verr) || !verr.Has(datacatalog.RuleVisibilityRequired) {
		t.Fatalf("err = %v, want visibility_required validation error", err)
	}

	secretPublic := &datacatalog.Catalog{Name: "app", Entities: []datacatalog.Entity{
		{Name: "users", Fields: []datacatalog.Field{{Name: "k", Visibility: datacatalog.VisibilityPublic, Secret: true}}},
	}}
	if _, err := FromCatalog(secretPublic); err == nil {
		t.Fatal("secret+public must be rejected")
	}
	if _, err := FromCatalog(nil); err == nil {
		t.Fatal("nil catalog must error")
	}
}

func asValidationError(err error, target **datacatalog.ValidationError) bool {
	return errors.As(err, target)
}

func TestFromCatalogTypesAndCapabilities(t *testing.T) {
	pub := datacatalog.VisibilityPublic
	c := &datacatalog.Catalog{Name: "app", Entities: []datacatalog.Entity{{
		Name: "Items",
		Fields: []datacatalog.Field{
			{Name: "Id", Type: datacatalog.TypeNumber, Visibility: pub},
			{Name: "Title", Type: datacatalog.TypeString, Visibility: pub},
			{Name: "Done", Type: datacatalog.TypeBool, Visibility: pub},
			{Name: "Created", Type: datacatalog.TypeTime, Visibility: pub},
			{Name: "Meta", Type: datacatalog.TypeJSON, Visibility: pub},
			{Name: "Blob", Type: datacatalog.TypeBytes, Visibility: pub},
			{Name: "Untyped", Visibility: pub},
		},
	}}}
	s, err := FromCatalog(c, WithWritable("Items", "Title"), WithNonFilterable("Items", "Meta"))
	if err != nil {
		t.Fatalf("FromCatalog: %v", err)
	}
	fields := s.Entities["items"].Fields
	wantTypes := map[string]FieldType{
		"id": FieldNumber, "title": FieldString, "done": FieldBool, "created": FieldTime,
		"meta": FieldJSON, "blob": FieldString, "untyped": FieldString,
	}
	for name, want := range wantTypes {
		f, ok := fields[name]
		if !ok {
			t.Fatalf("missing field %s", name)
		}
		if f.Type != want {
			t.Errorf("%s type = %q, want %q", name, f.Type, want)
		}
		if !f.Selectable || !f.Sortable {
			t.Errorf("%s should be selectable and sortable", name)
		}
		if f.Writable != (name == "title") {
			t.Errorf("%s writable = %t", name, f.Writable)
		}
		if f.Filterable != (name != "meta") {
			t.Errorf("%s filterable = %t", name, f.Filterable)
		}
	}

	if _, err := FromCatalog(c, WithWritable("Items", "Nope")); err == nil {
		t.Error("unknown option field must error")
	}
	if _, err := FromCatalog(c, WithNonFilterable("Nope", "Id")); err == nil {
		t.Error("unknown option entity must error")
	}
}

func TestFromCatalogCaseCollision(t *testing.T) {
	pub := datacatalog.VisibilityPublic
	c := &datacatalog.Catalog{Name: "app", Entities: []datacatalog.Entity{
		{Name: "t", Fields: []datacatalog.Field{{Name: "A", Visibility: pub}, {Name: "a", Visibility: pub}}},
	}}
	if _, err := FromCatalog(c); err == nil {
		t.Fatal("expected case collision error")
	}
}

func TestFromCatalogEndToEndUntrustedQuery(t *testing.T) {
	pub, internal := datacatalog.VisibilityPublic, datacatalog.VisibilityInternal
	c := &datacatalog.Catalog{Name: "app", Entities: []datacatalog.Entity{{
		Name: "users",
		Fields: []datacatalog.Field{
			{Name: "id", Type: datacatalog.TypeNumber, Visibility: pub},
			{Name: "email", Type: datacatalog.TypeString, Visibility: pub, PII: true},
			{Name: "shard", Type: datacatalog.TypeString, Visibility: internal},
			{Name: "api_token", Type: datacatalog.TypeString, Visibility: internal, Secret: true},
		},
	}}}
	s, err := FromCatalog(c)
	if err != nil {
		t.Fatalf("FromCatalog: %v", err)
	}
	cfg := NewUntrustedConfig(s, DialectSQLite)

	if _, err := cfg.Compile(`SELECT id FROM users LIMIT 10`); err != nil {
		t.Fatalf("public column: %v", err)
	}
	for _, col := range []string{"shard", "api_token", "email"} {
		_, err := cfg.Compile(`SELECT ` + col + ` FROM users LIMIT 10`)
		if err == nil || !strings.Contains(err.Error(), "unknown select field") {
			t.Errorf("%s: err = %v, want unknown select field", col, err)
		}
	}

	cfg.AllowPII = true
	if _, err := cfg.Compile(`SELECT email FROM users LIMIT 10`); err != nil {
		t.Errorf("email with AllowPII: %v", err)
	}
	if _, err := cfg.Compile(`SELECT shard FROM users LIMIT 10`); err == nil {
		t.Error("AllowPII must not admit internal columns")
	}
}
