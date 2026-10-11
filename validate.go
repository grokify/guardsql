package guardsql

import (
	"fmt"
	"strings"
)

// Validate checks a query against a schema allowlist.
func Validate(q *Query, schema Schema) error {
	if q == nil {
		return fmt.Errorf("query is nil")
	}
	schema = schema.Normalize()
	for _, cte := range q.With {
		if cte.Query == nil {
			return fmt.Errorf("cte %q query is nil", cte.Name)
		}
		if err := Validate(cte.Query, schema); err != nil {
			return fmt.Errorf("cte %q: %w", cte.Name, err)
		}
	}
	var ent Entity
	if q.FromQuery != nil {
		if strings.TrimSpace(q.FromAlias) == "" {
			return fmt.Errorf("nested FROM query requires an alias")
		}
		if err := Validate(q.FromQuery, schema); err != nil {
			return fmt.Errorf("nested FROM query: %w", err)
		}
		ent = outputEntity(q.FromQuery)
	} else {
		var ok bool
		ent, ok = schema.Entities[strings.ToLower(q.From)]
		if !ok {
			ent, ok = cteEntity(q.With, q.From)
			if !ok {
				return fmt.Errorf("unknown entity %q", q.From)
			}
		}
	}
	aliases := map[string]string{}
	if q.FromAlias != "" {
		aliases[strings.ToLower(q.FromAlias)] = q.From
	}
	baseEntity := strings.ToLower(q.From)
	for _, join := range q.Joins {
		if join.Query != nil {
			if strings.TrimSpace(join.Alias) == "" {
				return fmt.Errorf("joined nested query requires an alias")
			}
			if err := Validate(join.Query, schema); err != nil {
				return fmt.Errorf("join query: %w", err)
			}
		} else if _, ok := schema.Entities[strings.ToLower(join.Entity)]; !ok {
			if _, ok := cteEntity(q.With, join.Entity); !ok {
				return fmt.Errorf("unknown join entity %q", join.Entity)
			}
		}
		if join.Type != JoinCross && join.Condition == nil {
			return fmt.Errorf("%s join requires ON condition", join.Type)
		}
		if join.Type == JoinCross && join.Condition != nil {
			return fmt.Errorf("cross join does not support ON condition")
		}
		if join.Alias != "" {
			alias := strings.ToLower(join.Alias)
			if _, exists := aliases[alias]; exists {
				return fmt.Errorf("duplicate source alias %q", join.Alias)
			}
			aliases[alias] = join.Entity
		}
		joinEntity := strings.ToLower(join.Entity)
		if baseEntity != "" && joinEntity == baseEntity && (q.FromAlias == "" || join.Alias == "") {
			return fmt.Errorf("self join on %q requires aliases on both sources", joinEntity)
		}
	}
	if len(q.Select) == 0 {
		q.Select = []SelectItem{{Star: true}}
	}
	grouped := map[string]struct{}{}
	for _, fieldName := range q.GroupBy {
		ref, ok := resolveField(ent, fieldName, aliases)
		if !ok {
			return fmt.Errorf("unknown group field %q", fieldName)
		}
		if !ref.Field.Sortable {
			return fmt.Errorf("field %q is not groupable", fieldName)
		}
		grouped[ref.Key()] = struct{}{}
	}
	hasAggregate := queryHasAggregate(q)
	for _, item := range q.Select {
		if item.Star {
			if hasAggregate || len(q.GroupBy) > 0 {
				return fmt.Errorf("SELECT * is not allowed with GROUP BY or aggregates")
			}
			continue
		}
		if item.Aggregate != nil {
			if err := validateAggregate(item.Aggregate, ent, aliases); err != nil {
				return err
			}
			continue
		}
		ref, ok := resolveField(ent, item.Field, aliases)
		if !ok {
			return fmt.Errorf("unknown select field %q", item.Field)
		}
		if !ref.Field.Selectable {
			return fmt.Errorf("field %q is not selectable", item.Field)
		}
		if hasAggregate || len(q.GroupBy) > 0 {
			if _, ok := grouped[ref.Key()]; !ok {
				return fmt.Errorf("selected field %q must appear in GROUP BY", item.Field)
			}
		}
	}
	if err := validateExpr(q.Where, ent, aliases); err != nil {
		return err
	}
	if q.Having != nil {
		out := outputEntity(q)
		if err := validateExpr(q.Having, out, nil); err != nil {
			return fmt.Errorf("having: %w", err)
		}
	}
	for _, order := range q.OrderBy {
		orderEnt := ent
		if hasAggregate || len(q.GroupBy) > 0 {
			orderEnt = outputEntity(q)
		}
		ref, ok := resolveField(orderEnt, order.Field, aliases)
		if !ok {
			return fmt.Errorf("unknown order field %q", order.Field)
		}
		if !ref.Field.Sortable {
			return fmt.Errorf("field %q is not sortable", order.Field)
		}
	}
	if q.Limit < -1 {
		return fmt.Errorf("limit must be non-negative")
	}
	if schema.MaxLimit > 0 && (q.Limit == -1 || q.Limit > schema.MaxLimit) {
		q.Limit = schema.MaxLimit
	}
	return nil
}

func cteEntity(ctes []CTE, name string) (Entity, bool) {
	for _, cte := range ctes {
		if strings.EqualFold(cte.Name, name) && cte.Query != nil {
			return outputEntity(cte.Query), true
		}
	}
	return Entity{}, false
}

func queryHasAggregate(q *Query) bool {
	for _, item := range q.Select {
		if item.Aggregate != nil {
			return true
		}
	}
	return false
}

func validateAggregate(agg *AggregateExpr, ent Entity, aliases map[string]string) error {
	if agg == nil {
		return nil
	}
	if agg.Star {
		if agg.Func != AggCount {
			return fmt.Errorf("only COUNT supports '*'")
		}
		return nil
	}
	ref, ok := resolveField(ent, agg.Field, aliases)
	if !ok {
		return fmt.Errorf("unknown aggregate field %q", agg.Field)
	}
	if !ref.Field.Selectable {
		return fmt.Errorf("field %q is not selectable", agg.Field)
	}
	switch agg.Func {
	case AggSum, AggAvg:
		// JSON path values are dynamically typed; the backend coerces them.
		if !ref.IsJSONPath() && ref.Field.Type != FieldNumber {
			return fmt.Errorf("%s requires a number field", agg.Func)
		}
	case AggCount, AggMin, AggMax:
	default:
		return fmt.Errorf("unsupported aggregate function %q", agg.Func)
	}
	return nil
}

func outputEntity(q *Query) Entity {
	fields := map[string]Field{}
	if q == nil {
		return Entity{Fields: fields}
	}
	for _, item := range q.Select {
		if item.Star {
			continue
		}
		name := outputName(item)
		if name == "" {
			continue
		}
		typ := FieldString
		if item.Aggregate != nil {
			switch item.Aggregate.Func {
			case AggCount, AggSum, AggAvg:
				typ = FieldNumber
			default:
				typ = FieldString
			}
		}
		fields[strings.ToLower(name)] = Field{Name: name, Type: typ, Selectable: true, Filterable: true, Sortable: true}
	}
	return Entity{Name: q.From, Fields: fields}
}

func validateExpr(expr Expr, ent Entity, aliases map[string]string) error {
	switch e := expr.(type) {
	case nil:
		return nil
	case *LogicalExpr:
		if err := validateExpr(e.Left, ent, aliases); err != nil {
			return err
		}
		return validateExpr(e.Right, ent, aliases)
	case *NotExpr:
		return validateExpr(e.Expr, ent, aliases)
	case *CompareExpr:
		ref, ok := resolveField(ent, e.Field, aliases)
		if !ok {
			return fmt.Errorf("unknown filter field %q", e.Field)
		}
		if !ref.Field.Filterable {
			return fmt.Errorf("field %q is not filterable", e.Field)
		}
		if e.Op != OpIsNull && e.Op != OpNotNull && len(e.Values) == 0 {
			return fmt.Errorf("operator %s requires a value", e.Op)
		}
		if e.Op == OpIn && len(e.Values) == 0 {
			return fmt.Errorf("IN requires at least one value")
		}
		for _, value := range e.Values {
			if err := validateValue(ref, e.Op, value); err != nil {
				return fmt.Errorf("field %q: %w", e.Field, err)
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported expression %T", expr)
	}
}

func validateValue(ref FieldRef, op CompareOp, value Value) error {
	if value.Type == ValueNull {
		return nil
	}
	// JSON path values are dynamically typed; the backend coerces them, so skip
	// static value-type checks for extractions.
	if ref.IsJSONPath() {
		return nil
	}
	field := ref.Field
	switch op {
	case OpContains:
		if field.Type != FieldString {
			return fmt.Errorf("CONTAINS requires a string field")
		}
		if value.Type != ValueString {
			return fmt.Errorf("CONTAINS requires a string value")
		}
		return nil
	case OpLT, OpLTE, OpGT, OpGTE:
		if field.Type != FieldNumber && field.Type != FieldTime {
			return fmt.Errorf("%s requires a number or time field", op)
		}
	}
	switch field.Type {
	case FieldString, FieldTime:
		if value.Type != ValueString {
			return fmt.Errorf("expected string value")
		}
	case FieldNumber:
		if value.Type != ValueNumber {
			return fmt.Errorf("expected number value")
		}
	case FieldBool:
		if value.Type != ValueBool {
			return fmt.Errorf("expected bool value")
		}
	}
	return nil
}
