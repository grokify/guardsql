package guardsql

import "fmt"

// Config is a declarative description of a queryable database together with the
// rules for safely querying it: the schema (entities, fields, types, and
// per-field data classification), the target SQL dialect, and safety defaults.
//
// It is the recommended high-level entry point; the lower-level Parse, Validate,
// CheckPolicy, and Compile primitives remain available. Config applies secure
// defaults: secret columns are never queryable, internal columns are queryable
// only when AllowInternal is set, PII columns only when AllowPII is set, and every identifier must be a safe unquoted identifier
// unless AllowUnsafeIdentifiers is set.
//
// Config is JSON-serializable so an application can load "what the database is
// and how it may be queried" from a file.
type Config struct {
	// Schema describes the entities and fields, including each field's Class.
	Schema Schema `json:"schema"`
	// Dialect is the target SQL dialect for Compile.
	Dialect Dialect `json:"dialect,omitempty"`
	// AllowPII permits querying fields classed ClassPII. Default false.
	AllowPII bool `json:"allowPII,omitempty"`
	// AllowInternal permits querying fields classed ClassInternal (platform
	// data not meant for customers). Default false.
	AllowInternal bool `json:"allowInternal,omitempty"`
	// AllowSecret permits querying fields classed ClassSecret. Reserve this for
	// trusted, application-authored SQL; never enable it for customer-supplied
	// SQL. Default false.
	AllowSecret bool `json:"allowSecret,omitempty"`
	// AllowUnsafeIdentifiers disables the default requirement that every entity
	// and field identifier match the unquoted SQL identifier charset. Leave
	// false (the default) to lock identifiers down; set true only for a database
	// that genuinely relies on quoted identifiers.
	AllowUnsafeIdentifiers bool `json:"allowUnsafeIdentifiers,omitempty"`
	// AllowUnfilteredMutations permits UPDATE and DELETE without a WHERE clause
	// (a full-table write). Default false: unfiltered mutations are rejected
	// even for trusted callers, since they are almost always a mistake.
	AllowUnfilteredMutations bool `json:"allowUnfilteredMutations,omitempty"`
	// RowScope enforces tenant/ownership isolation (IDOR defense): it returns a
	// mandatory predicate for each queried entity, derived from trusted session
	// context, that is ANDed into every compiled SELECT/UPDATE/DELETE. It is
	// per-request and not serialized. See ScopeFunc and ScopeEquals.
	RowScope ScopeFunc `json:"-"`
	// RequireRowScope refuses to compile unless RowScope is set (fail-closed).
	RequireRowScope bool `json:"-"`
	// Policy holds optional additional AST-level limits (depth, node count,
	// required LIMIT, allowed operations, and so on) applied during Check and
	// used to authorize mutations.
	Policy Policy `json:"-"`
}

// NewUntrustedConfig returns a Config hardened for third-party or
// customer-supplied SQL (the Metabase-style case): read-only, secret and PII
// columns stripped from the schema, identifier lockdown on, and a required
// LIMIT. Tighten further via the returned Config's Policy (MaxLimit, MaxDepth,
// and so on).
func NewUntrustedConfig(schema Schema, dialect Dialect) Config {
	return Config{
		Schema:        schema,
		Dialect:       dialect,
		AllowPII:      false,
		AllowInternal: false,
		AllowSecret:   false,
		Policy: Policy{
			AllowedOps:   []Operation{OperationRead},
			RequireLimit: true,
		},
	}
}

// NewTrustedConfig returns a Config for internal, application-authored SQL: it
// may read and mutate, and may access secret, internal and PII columns. Identifier
// lockdown and value parameterization still apply, and mutation targets must
// still be columns marked Writable — trust removes access restrictions, not the
// safety of regenerating parameterized SQL from the validated AST or the
// write-allowlist. Never use this profile for SQL that originates outside the
// application.
func NewTrustedConfig(schema Schema, dialect Dialect) Config {
	return Config{
		Schema:        schema,
		Dialect:       dialect,
		AllowPII:      true,
		AllowInternal: true,
		AllowSecret:   true,
		Policy: Policy{
			AllowedOps: []Operation{OperationRead, OperationCreate, OperationUpdate, OperationDelete},
		},
	}
}

// ScopeEquals returns a ScopeFunc that requires column = value on every queried
// entity, using value from trusted session context (e.g. the tenant ID). If a
// queried entity has no such column, compilation fails (fail-closed), so a
// missing scope column can never silently return unscoped data. For per-entity
// exemptions or multiple scope columns, write a ScopeFunc directly.
func ScopeEquals(column string, value any) ScopeFunc {
	return func(string) (Expr, error) {
		return Eq(column, value), nil
	}
}

// effectiveSchema returns the schema with fields the caller may not query
// removed: secret fields unless AllowSecret is set, internal fields unless
// AllowInternal is set, and PII fields unless AllowPII is set. Denied fields are treated as non-existent, so a query
// referencing one fails as an unknown field rather than disclosing that the
// column exists. This is the AST-step stripping that separates the trusted and
// untrusted profiles.
func (c Config) effectiveSchema() Schema {
	out := Schema{Entities: make(map[string]Entity, len(c.Schema.Entities)), MaxLimit: c.Schema.MaxLimit}
	for entName, ent := range c.Schema.Entities {
		fields := make(map[string]Field, len(ent.Fields))
		for fieldName, field := range ent.Fields {
			switch field.Class {
			case ClassSecret:
				if !c.AllowSecret {
					continue
				}
			case ClassInternal:
				if !c.AllowInternal {
					continue
				}
			case ClassPII:
				if !c.AllowPII {
					continue
				}
			}
			fields[fieldName] = field
		}
		out.Entities[entName] = Entity{Name: ent.Name, Fields: fields}
	}
	return out
}

// Check parses input, enforces identifier safety (unless AllowUnsafeIdentifiers),
// validates it against the classification-filtered schema, and applies Policy.
// It returns the validated AST, ready for Compile.
func (c Config) Check(input string) (*Query, error) {
	schema := c.effectiveSchema()
	if !c.AllowUnsafeIdentifiers {
		if err := schema.ValidateIdentifiers(); err != nil {
			return nil, err
		}
	}
	q, err := Parse(input)
	if err != nil {
		return nil, err
	}
	if err := Validate(q, schema); err != nil {
		return nil, err
	}
	if issues := CheckPolicy(q, c.Policy); len(issues) > 0 {
		return nil, fmt.Errorf("guardsql: policy: %s", issues[0].Message)
	}
	return q, nil
}

// Compile parses, checks, and compiles input to parameterized SQL for the
// configured dialect in one step. It applies the same classification and
// identifier-safety rules as Check.
func (c Config) Compile(input string) (CompiledSQL, error) {
	q, err := c.Check(input)
	if err != nil {
		return CompiledSQL{}, err
	}
	return Compile(q, CompileOptions{
		Dialect:           c.Dialect,
		Schema:            c.effectiveSchema(),
		StrictIdentifiers: !c.AllowUnsafeIdentifiers,
		RowScope:          c.RowScope,
		RequireRowScope:   c.RequireRowScope,
	})
}
