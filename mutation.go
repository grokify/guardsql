package guardsql

import (
	"fmt"
	"strings"
)

// This file provides a programmatic builder for trusted, application-authored
// mutations (INSERT/UPDATE/DELETE). Trusted code constructs a statement from
// typed parts rather than parsing SQL text, then compiles it through the same
// hardened path as read queries: every value is bound as a parameter, every
// identifier is validated against the schema and re-quoted, and the operation
// must be permitted by the Config's policy. UPDATE and DELETE require a WHERE
// clause unless Config.AllowUnfilteredMutations is set.

// Assignment sets a column to a value in an INSERT or UPDATE.
type Assignment struct {
	Column string
	Value  any
}

// InsertBuilder builds a parameterized INSERT. Construct with Insert.
type InsertBuilder struct {
	table string
	sets  []Assignment
}

// Insert starts an INSERT into table.
func Insert(table string) *InsertBuilder { return &InsertBuilder{table: table} }

// Set adds a column assignment.
func (b *InsertBuilder) Set(column string, value any) *InsertBuilder {
	b.sets = append(b.sets, Assignment{Column: column, Value: value})
	return b
}

// Compile validates and compiles the INSERT to parameterized SQL.
func (b *InsertBuilder) Compile(cfg Config) (CompiledSQL, error) {
	if len(b.sets) == 0 {
		return CompiledSQL{}, fmt.Errorf("guardsql: INSERT requires at least one column")
	}
	m, err := newMutationCompiler(cfg, OperationCreate, b.table)
	if err != nil {
		return CompiledSQL{}, err
	}
	cols := make([]string, 0, len(b.sets))
	phs := make([]string, 0, len(b.sets))
	for _, a := range b.sets {
		col, err := m.column(a.Column)
		if err != nil {
			return CompiledSQL{}, err
		}
		cols = append(cols, col)
		phs = append(phs, m.c.bind(a.Value))
	}
	sql := "INSERT INTO " + m.table + " (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(phs, ", ") + ")"
	return CompiledSQL{SQL: sql, Args: m.c.args}, nil
}

// UpdateBuilder builds a parameterized UPDATE. Construct with Update.
type UpdateBuilder struct {
	table string
	sets  []Assignment
	where Expr
}

// Update starts an UPDATE of table.
func Update(table string) *UpdateBuilder { return &UpdateBuilder{table: table} }

// Set adds a column assignment.
func (b *UpdateBuilder) Set(column string, value any) *UpdateBuilder {
	b.sets = append(b.sets, Assignment{Column: column, Value: value})
	return b
}

// Where sets the filter. Required unless Config.AllowUnfilteredMutations is set.
// Build conditions with Eq, In, And, Or, Not, and the comparison helpers.
func (b *UpdateBuilder) Where(expr Expr) *UpdateBuilder {
	b.where = expr
	return b
}

// Compile validates and compiles the UPDATE to parameterized SQL.
func (b *UpdateBuilder) Compile(cfg Config) (CompiledSQL, error) {
	if len(b.sets) == 0 {
		return CompiledSQL{}, fmt.Errorf("guardsql: UPDATE requires at least one assignment")
	}
	if b.where == nil && !cfg.AllowUnfilteredMutations {
		return CompiledSQL{}, fmt.Errorf("guardsql: UPDATE requires a WHERE clause (set AllowUnfilteredMutations to override)")
	}
	m, err := newMutationCompiler(cfg, OperationUpdate, b.table)
	if err != nil {
		return CompiledSQL{}, err
	}
	assigns := make([]string, 0, len(b.sets))
	for _, a := range b.sets {
		col, err := m.column(a.Column)
		if err != nil {
			return CompiledSQL{}, err
		}
		assigns = append(assigns, col+" = "+m.c.bind(a.Value))
	}
	sql := "UPDATE " + m.table + " SET " + strings.Join(assigns, ", ")
	scope, err := resolveScope(cfg.RowScope, cfg.RequireRowScope, m.ent.Name)
	if err != nil {
		return CompiledSQL{}, err
	}
	if where := andExpr(b.where, scope); where != nil {
		clause, err := m.c.expr(where, m.ent)
		if err != nil {
			return CompiledSQL{}, err
		}
		sql += " WHERE " + clause
	}
	return CompiledSQL{SQL: sql, Args: m.c.args}, nil
}

// DeleteBuilder builds a parameterized DELETE. Construct with Delete.
type DeleteBuilder struct {
	table string
	where Expr
}

// Delete starts a DELETE from table.
func Delete(table string) *DeleteBuilder { return &DeleteBuilder{table: table} }

// Where sets the filter. Required unless Config.AllowUnfilteredMutations is set.
func (b *DeleteBuilder) Where(expr Expr) *DeleteBuilder {
	b.where = expr
	return b
}

// Compile validates and compiles the DELETE to parameterized SQL.
func (b *DeleteBuilder) Compile(cfg Config) (CompiledSQL, error) {
	if b.where == nil && !cfg.AllowUnfilteredMutations {
		return CompiledSQL{}, fmt.Errorf("guardsql: DELETE requires a WHERE clause (set AllowUnfilteredMutations to override)")
	}
	m, err := newMutationCompiler(cfg, OperationDelete, b.table)
	if err != nil {
		return CompiledSQL{}, err
	}
	sql := "DELETE FROM " + m.table
	scope, err := resolveScope(cfg.RowScope, cfg.RequireRowScope, m.ent.Name)
	if err != nil {
		return CompiledSQL{}, err
	}
	if where := andExpr(b.where, scope); where != nil {
		clause, err := m.c.expr(where, m.ent)
		if err != nil {
			return CompiledSQL{}, err
		}
		sql += " WHERE " + clause
	}
	return CompiledSQL{SQL: sql, Args: m.c.args}, nil
}

// mutationCompiler holds the shared setup for compiling one mutation.
type mutationCompiler struct {
	c     *compiler
	ent   Entity
	table string // quoted table identifier
}

func newMutationCompiler(cfg Config, op Operation, table string) (*mutationCompiler, error) {
	// Mutations are deny-by-default: the operation must be explicitly listed in
	// the policy. An empty AllowedOps (e.g. a zero Config) permits no mutations,
	// so only a trusted profile that lists the op can mutate.
	if !operationAllowed(op, cfg.Policy.AllowedOps) {
		return nil, fmt.Errorf("guardsql: operation %q is not allowed by policy", op)
	}
	d, err := dialectFor(cfg.Dialect)
	if err != nil {
		return nil, err
	}
	schema := cfg.effectiveSchema().Normalize()
	if !cfg.AllowUnsafeIdentifiers {
		if err := schema.ValidateIdentifiers(); err != nil {
			return nil, err
		}
	}
	ent, ok := schema.Entities[strings.ToLower(strings.TrimSpace(table))]
	if !ok {
		return nil, fmt.Errorf("guardsql: unknown entity %q", table)
	}
	return &mutationCompiler{c: &compiler{d: d}, ent: ent, table: d.quoteIdent(ent.Name)}, nil
}

// column resolves and quotes a plain assignable column. It rejects unknown
// columns, JSON-path targets (the builder assigns whole columns, not sub-paths),
// and columns not marked Writable in the schema. The Writable check is the
// mass-assignment defense: only allowlisted columns can be written.
func (m *mutationCompiler) column(name string) (string, error) {
	ref, ok := resolveField(m.ent, name, nil)
	if !ok {
		return "", fmt.Errorf("guardsql: unknown column %q", name)
	}
	if ref.IsJSONPath() {
		return "", fmt.Errorf("guardsql: cannot assign to JSON path %q", name)
	}
	if !ref.Field.Writable {
		return "", fmt.Errorf("guardsql: column %q is not writable", name)
	}
	return m.c.d.quoteIdent(ref.Name), nil
}
