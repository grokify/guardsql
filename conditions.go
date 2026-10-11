package guardsql

// Condition helpers build filter expressions (Expr) for the mutation builders'
// Where clauses. Values are wrapped as literals and become bind parameters at
// compile time; field names are resolved and quoted against the schema.

// Eq builds field = value.
func Eq(field string, value any) Expr { return cmp(field, OpEq, value) }

// Ne builds field != value.
func Ne(field string, value any) Expr { return cmp(field, OpNotEq, value) }

// Lt builds field < value.
func Lt(field string, value any) Expr { return cmp(field, OpLT, value) }

// Lte builds field <= value.
func Lte(field string, value any) Expr { return cmp(field, OpLTE, value) }

// Gt builds field > value.
func Gt(field string, value any) Expr { return cmp(field, OpGT, value) }

// Gte builds field >= value.
func Gte(field string, value any) Expr { return cmp(field, OpGTE, value) }

// In builds field IN (values...).
func In(field string, values ...any) Expr {
	vals := make([]Value, 0, len(values))
	for _, v := range values {
		vals = append(vals, literal(v))
	}
	return &CompareExpr{Field: field, Op: OpIn, Values: vals}
}

// IsNull builds field IS NULL.
func IsNull(field string) Expr { return &CompareExpr{Field: field, Op: OpIsNull} }

// IsNotNull builds field IS NOT NULL.
func IsNotNull(field string) Expr { return &CompareExpr{Field: field, Op: OpNotNull} }

// And combines expressions with AND. Nil expressions are ignored; And of none
// returns nil.
func And(exprs ...Expr) Expr { return fold(LogicalAnd, exprs) }

// Or combines expressions with OR. Nil expressions are ignored; Or of none
// returns nil.
func Or(exprs ...Expr) Expr { return fold(LogicalOr, exprs) }

// Not negates an expression.
func Not(expr Expr) Expr {
	if expr == nil {
		return nil
	}
	return &NotExpr{Expr: expr}
}

func cmp(field string, op CompareOp, value any) Expr {
	return &CompareExpr{Field: field, Op: op, Values: []Value{literal(value)}}
}

func fold(op LogicalOp, exprs []Expr) Expr {
	var acc Expr
	for _, e := range exprs {
		if e == nil {
			continue
		}
		if acc == nil {
			acc = e
			continue
		}
		acc = &LogicalExpr{Op: op, Left: acc, Right: e}
	}
	return acc
}

// literal wraps a Go value as a typed query literal. Integers normalize to
// int64 and floats to float64 so drivers receive a supported type; other types
// (e.g. time.Time) are passed through unchanged as bind parameters.
func literal(v any) Value {
	switch x := v.(type) {
	case nil:
		return Value{Type: ValueNull}
	case bool:
		return Value{Type: ValueBool, Any: x}
	case string:
		return Value{Type: ValueString, Any: x}
	case int:
		return Value{Type: ValueNumber, Any: int64(x)}
	case int8:
		return Value{Type: ValueNumber, Any: int64(x)}
	case int16:
		return Value{Type: ValueNumber, Any: int64(x)}
	case int32:
		return Value{Type: ValueNumber, Any: int64(x)}
	case int64:
		return Value{Type: ValueNumber, Any: x}
	case uint, uint8, uint16, uint32, uint64:
		return Value{Type: ValueNumber, Any: x}
	case float32:
		return Value{Type: ValueNumber, Any: float64(x)}
	case float64:
		return Value{Type: ValueNumber, Any: x}
	default:
		// Unknown types (time.Time, []byte, ...) pass through as a bind arg.
		return Value{Type: ValueString, Any: v}
	}
}
