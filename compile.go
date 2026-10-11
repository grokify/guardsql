package guardsql

import (
	"fmt"
	"math"
	"strings"
)

// Dialect identifies a target SQL dialect for compilation.
type Dialect string

// Supported compilation dialects.
const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
	DialectMySQL    Dialect = "mysql"
)

// CompiledSQL is parameterized SQL emitted from a validated AST. The SQL string
// never contains inlined user literals: every value is a bind parameter in
// Args, in positional order.
type CompiledSQL struct {
	SQL  string
	Args []any
}

// CompileOptions configures compilation.
type CompileOptions struct {
	// Dialect selects the target SQL dialect. Required.
	Dialect Dialect
	// Schema resolves and re-quotes every identifier, and identifies JSON
	// columns. Required: the compiler emits only identifiers present in the
	// schema, so an unknown field is an error rather than passthrough.
	Schema Schema
	// StrictIdentifiers, when true, requires every schema identifier to be a
	// safe unquoted identifier (see IsSafeIdentifier) before any SQL is emitted.
	// Because identifiers cannot be parameterized, this makes identifier-based
	// injection structurally impossible. Config sets this by default.
	StrictIdentifiers bool
	// RowScope, when set, returns a mandatory scope predicate for the queried
	// entity (derived from trusted session context, e.g. tenant_id = <session>).
	// The predicate is ANDed at the top level of the compiled WHERE, so no
	// user-supplied filter can widen it. Returning (nil, nil) explicitly exempts
	// an entity; returning an error refuses compilation. See ScopeFunc.
	RowScope ScopeFunc
	// RequireRowScope, when true, refuses to compile unless RowScope is set,
	// making unscoped access impossible (fail-closed). RowScope may still exempt
	// individual entities by returning (nil, nil).
	RequireRowScope bool
}

// ScopeFunc returns a mandatory row-scope predicate for a queried entity. It is
// how an application enforces tenant/ownership isolation (IDOR defense) at the
// GuardSQL layer: the predicate is built from trusted session context and ANDed
// into every compiled statement so a user's own filter cannot widen scope.
//
// Return an Expr (typically built with Eq/In/And) to require it, (nil, nil) to
// exempt the entity, or an error to refuse compilation.
type ScopeFunc func(entity string) (Expr, error)

// resolveScope returns the mandatory scope predicate for entity, honoring
// RequireRowScope (fail-closed when no scope function is configured).
func resolveScope(rowScope ScopeFunc, required bool, entity string) (Expr, error) {
	if rowScope == nil {
		if required {
			return nil, fmt.Errorf("guardsql: row scope required but none configured for entity %q", entity)
		}
		return nil, nil
	}
	return rowScope(entity)
}

// andExpr combines a user filter with a mandatory scope predicate at the top
// level, so the scope cannot be widened by any user-supplied boolean structure.
func andExpr(userWhere, scope Expr) Expr {
	switch {
	case scope == nil:
		return userWhere
	case userWhere == nil:
		return scope
	default:
		return &LogicalExpr{Op: LogicalAnd, Left: userWhere, Right: scope}
	}
}

// Compile transpiles a validated read query into parameterized SQL for the
// target dialect. It regenerates SQL from the AST rather than passing user text
// through: only constructs the AST can represent and the emitter supports are
// produced, and every literal becomes a bind parameter. The query should have
// passed Validate and CheckPolicy first; Compile independently re-resolves all
// identifiers against the schema as defense in depth.
//
// Joins, CTEs, and nested query sources are not yet supported.
func Compile(q *Query, opts CompileOptions) (CompiledSQL, error) {
	if q == nil {
		return CompiledSQL{}, fmt.Errorf("guardsql: nil query")
	}
	if q.Operation != "" && q.Operation != OperationRead {
		return CompiledSQL{}, fmt.Errorf("guardsql: only read queries can be compiled, got %q", q.Operation)
	}
	if q.FromQuery != nil || len(q.With) > 0 || len(q.Joins) > 0 {
		return CompiledSQL{}, fmt.Errorf("guardsql: joins, CTEs, and nested sources are not yet supported by the compiler")
	}
	d, err := dialectFor(opts.Dialect)
	if err != nil {
		return CompiledSQL{}, err
	}
	schema := opts.Schema.Normalize()
	if opts.StrictIdentifiers {
		if err := schema.ValidateIdentifiers(); err != nil {
			return CompiledSQL{}, err
		}
	}
	ent, ok := schema.Entities[strings.ToLower(strings.TrimSpace(q.From))]
	if !ok {
		return CompiledSQL{}, fmt.Errorf("guardsql: unknown entity %q", q.From)
	}

	c := &compiler{d: d}

	selectSQL, outputNames, err := c.selectClause(q, ent)
	if err != nil {
		return CompiledSQL{}, err
	}

	var b strings.Builder
	b.WriteString("SELECT ")
	b.WriteString(selectSQL)
	b.WriteString(" FROM ")
	b.WriteString(d.quoteIdent(ent.Name))

	scope, err := resolveScope(opts.RowScope, opts.RequireRowScope, ent.Name)
	if err != nil {
		return CompiledSQL{}, err
	}
	if where := andExpr(q.Where, scope); where != nil {
		clause, err := c.expr(where, ent)
		if err != nil {
			return CompiledSQL{}, err
		}
		b.WriteString(" WHERE ")
		b.WriteString(clause)
	}

	if len(q.GroupBy) > 0 {
		parts := make([]string, 0, len(q.GroupBy))
		for _, name := range q.GroupBy {
			ref, ok := resolveField(ent, name, nil)
			if !ok {
				return CompiledSQL{}, fmt.Errorf("guardsql: unknown group field %q", name)
			}
			parts = append(parts, c.fieldRefExpr(ref))
		}
		b.WriteString(" GROUP BY ")
		b.WriteString(strings.Join(parts, ", "))
	}

	if q.Having != nil {
		having, err := c.expr(q.Having, ent)
		if err != nil {
			return CompiledSQL{}, err
		}
		b.WriteString(" HAVING ")
		b.WriteString(having)
	}

	if len(q.OrderBy) > 0 {
		aggregated := queryHasAggregate(q) || len(q.GroupBy) > 0
		parts := make([]string, 0, len(q.OrderBy))
		for _, order := range q.OrderBy {
			expr, err := c.orderExpr(ent, aggregated, outputNames, order.Field)
			if err != nil {
				return CompiledSQL{}, err
			}
			if order.Desc {
				expr += " DESC"
			} else {
				expr += " ASC"
			}
			parts = append(parts, expr)
		}
		b.WriteString(" ORDER BY ")
		b.WriteString(strings.Join(parts, ", "))
	}

	if q.Limit >= 0 {
		fmt.Fprintf(&b, " LIMIT %d", q.Limit)
	}

	return CompiledSQL{SQL: b.String(), Args: c.args}, nil
}

// compiler accumulates bind parameters while emitting SQL for one query.
type compiler struct {
	d    dialect
	args []any
}

// bind appends a parameter and returns its dialect placeholder.
func (c *compiler) bind(v any) string {
	c.args = append(c.args, v)
	return c.d.placeholder(len(c.args))
}

// selectClause emits the SELECT list and returns the set of output names
// (lower-cased aliases / plain column names) for ORDER BY alias resolution.
func (c *compiler) selectClause(q *Query, ent Entity) (string, map[string]bool, error) {
	outputNames := map[string]bool{}
	if len(q.Select) == 1 && q.Select[0].Star {
		return "*", outputNames, nil
	}
	parts := make([]string, 0, len(q.Select))
	for _, item := range q.Select {
		if item.Star {
			parts = append(parts, "*")
			continue
		}
		var expr string
		if item.Aggregate != nil {
			var err error
			expr, err = c.aggregateExpr(item.Aggregate, ent)
			if err != nil {
				return "", nil, err
			}
		} else {
			ref, ok := resolveField(ent, item.Field, nil)
			if !ok {
				return "", nil, fmt.Errorf("guardsql: unknown select field %q", item.Field)
			}
			expr = c.fieldRefExpr(ref)
		}
		if item.Alias != "" {
			outputNames[strings.ToLower(item.Alias)] = true
			expr += " AS " + c.d.quoteIdent(strings.ToLower(item.Alias))
		} else if item.Aggregate == nil {
			if ref, ok := resolveField(ent, item.Field, nil); ok && !ref.IsJSONPath() {
				outputNames[ref.Name] = true
			}
		}
		parts = append(parts, expr)
	}
	return strings.Join(parts, ", "), outputNames, nil
}

func (c *compiler) aggregateExpr(agg *AggregateExpr, ent Entity) (string, error) {
	if agg.Star {
		if agg.Func != AggCount {
			return "", fmt.Errorf("guardsql: only COUNT supports '*'")
		}
		return "COUNT(*)", nil
	}
	ref, ok := resolveField(ent, agg.Field, nil)
	if !ok {
		return "", fmt.Errorf("guardsql: unknown aggregate field %q", agg.Field)
	}
	return string(agg.Func) + "(" + c.fieldRefExpr(ref) + ")", nil
}

// fieldRefExpr emits the SQL expression for a resolved field reference: a quoted
// column, or dialect-specific JSON extraction for a path.
func (c *compiler) fieldRefExpr(ref FieldRef) string {
	col := c.d.quoteIdent(ref.Name)
	if !ref.IsJSONPath() {
		return col
	}
	return c.d.jsonExtractText(col, ref.JSONPath, c.bind)
}

func (c *compiler) orderExpr(ent Entity, aggregated bool, outputNames map[string]bool, name string) (string, error) {
	// A base column or JSON path resolvable against the entity emits its
	// expression (matches the SELECT/GROUP BY expression).
	if ref, ok := resolveField(ent, name, nil); ok {
		return c.fieldRefExpr(ref), nil
	}
	// Otherwise, in an aggregated query, order by a projected output alias.
	if aggregated && outputNames[strings.ToLower(name)] {
		return c.d.quoteIdent(strings.ToLower(name)), nil
	}
	return "", fmt.Errorf("guardsql: cannot order by %q", name)
}

func (c *compiler) expr(e Expr, ent Entity) (string, error) {
	switch x := e.(type) {
	case nil:
		return "", nil
	case *LogicalExpr:
		left, err := c.expr(x.Left, ent)
		if err != nil {
			return "", err
		}
		right, err := c.expr(x.Right, ent)
		if err != nil {
			return "", err
		}
		return "(" + left + " " + string(x.Op) + " " + right + ")", nil
	case *NotExpr:
		inner, err := c.expr(x.Expr, ent)
		if err != nil {
			return "", err
		}
		return "NOT (" + inner + ")", nil
	case *CompareExpr:
		return c.compareExpr(x, ent)
	default:
		return "", fmt.Errorf("guardsql: unsupported expression %T", e)
	}
}

func (c *compiler) compareExpr(cmp *CompareExpr, ent Entity) (string, error) {
	ref, ok := resolveField(ent, cmp.Field, nil)
	if !ok {
		return "", fmt.Errorf("guardsql: unknown filter field %q", cmp.Field)
	}
	col := c.fieldRefExpr(ref)

	switch cmp.Op {
	case OpIsNull:
		return col + " IS NULL", nil
	case OpNotNull:
		return col + " IS NOT NULL", nil
	case OpIn:
		if len(cmp.Values) == 0 {
			return "", fmt.Errorf("guardsql: IN requires at least one value")
		}
		phs := make([]string, 0, len(cmp.Values))
		for _, v := range cmp.Values {
			phs = append(phs, c.bind(valueArg(v)))
		}
		return col + " IN (" + strings.Join(phs, ", ") + ")", nil
	case OpContains:
		if len(cmp.Values) != 1 {
			return "", fmt.Errorf("guardsql: CONTAINS requires one value")
		}
		pattern := "%" + escapeLike(fmt.Sprint(valueArg(cmp.Values[0]))) + "%"
		return col + " LIKE " + c.bind(pattern) + " ESCAPE '\\'", nil
	case OpEq, OpNotEq, OpLT, OpLTE, OpGT, OpGTE:
		if len(cmp.Values) != 1 {
			return "", fmt.Errorf("guardsql: operator %s requires one value", cmp.Op)
		}
		return col + " " + string(cmp.Op) + " " + c.bind(valueArg(cmp.Values[0])), nil
	default:
		return "", fmt.Errorf("guardsql: unsupported operator %q", cmp.Op)
	}
}

// valueArg converts a parsed literal into a bind argument. Whole numbers are
// bound as int64 so integer columns compare cleanly.
func valueArg(v Value) any {
	if v.Type == ValueNumber {
		if f, ok := v.Any.(float64); ok {
			if f == math.Trunc(f) && !math.IsInf(f, 0) {
				return int64(f)
			}
			return f
		}
	}
	return v.Any
}

// escapeLike escapes LIKE metacharacters so a CONTAINS value is matched
// literally under `ESCAPE '\'`.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
