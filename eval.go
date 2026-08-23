package guardsql

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Row is a generic record used by the in-memory evaluator and tests.
type Row map[string]any

// Eval filters, sorts, limits, and projects rows using a validated query.
func Eval(q *Query, rows []Row) ([]Row, error) {
	if q.FromQuery != nil || len(q.With) > 0 || len(q.Joins) > 0 {
		return nil, fmt.Errorf("nested queries, CTEs, and joins require a backend compiler")
	}
	var filtered []Row
	for _, row := range rows {
		ok, err := evalExpr(q.Where, row)
		if err != nil {
			return nil, err
		}
		if ok {
			filtered = append(filtered, row)
		}
	}
	if len(q.GroupBy) > 0 || queryHasAggregate(q) {
		var err error
		filtered, err = aggregateRows(q, filtered)
		if err != nil {
			return nil, err
		}
		if q.Having != nil {
			var havingRows []Row
			for _, row := range filtered {
				ok, err := evalExpr(q.Having, row)
				if err != nil {
					return nil, err
				}
				if ok {
					havingRows = append(havingRows, row)
				}
			}
			filtered = havingRows
		}
	}
	if len(q.OrderBy) > 0 {
		sort.SliceStable(filtered, func(i, j int) bool {
			for _, order := range q.OrderBy {
				cmp := compareAny(filtered[i][order.Field], filtered[j][order.Field])
				if cmp == 0 {
					continue
				}
				if order.Desc {
					return cmp > 0
				}
				return cmp < 0
			}
			return false
		})
	}
	if q.Limit >= 0 && len(filtered) > q.Limit {
		filtered = filtered[:q.Limit]
	}
	return projectRows(q.Select, filtered), nil
}

func aggregateRows(q *Query, rows []Row) ([]Row, error) {
	type group struct {
		key    string
		values []any
		rows   []Row
	}
	groupsByKey := map[string]*group{}
	var order []string
	for _, row := range rows {
		values := make([]any, 0, len(q.GroupBy))
		for _, field := range q.GroupBy {
			values = append(values, row[field])
		}
		key := groupKey(values)
		g, ok := groupsByKey[key]
		if !ok {
			g = &group{key: key, values: values}
			groupsByKey[key] = g
			order = append(order, key)
		}
		g.rows = append(g.rows, row)
	}
	if len(q.GroupBy) == 0 {
		key := groupKey(nil)
		groupsByKey[key] = &group{key: key, rows: rows}
		order = []string{key}
	}
	out := make([]Row, 0, len(order))
	for _, key := range order {
		g := groupsByKey[key]
		row := Row{}
		for i, field := range q.GroupBy {
			row[field] = g.values[i]
		}
		for _, item := range q.Select {
			if item.Aggregate == nil {
				if item.Star {
					continue
				}
				name := outputName(item)
				row[name] = row[item.Field]
				continue
			}
			value, err := evalAggregate(item.Aggregate, g.rows)
			if err != nil {
				return nil, err
			}
			row[outputName(item)] = value
		}
		out = append(out, row)
	}
	return out, nil
}

func groupKey(values []any) string {
	var b strings.Builder
	for i, value := range values {
		if i > 0 {
			b.WriteByte('\x1f')
		}
		b.WriteString(fmt.Sprintf("%T:%v", value, value))
	}
	return b.String()
}

func evalAggregate(agg *AggregateExpr, rows []Row) (any, error) {
	switch agg.Func {
	case AggCount:
		if agg.Star {
			return len(rows), nil
		}
		count := 0
		for _, row := range rows {
			if row[agg.Field] != nil {
				count++
			}
		}
		return count, nil
	case AggSum, AggAvg:
		sum := 0.0
		count := 0
		for _, row := range rows {
			if v, ok := asFloat(row[agg.Field]); ok {
				sum += v
				count++
			}
		}
		if agg.Func == AggAvg {
			if count == 0 {
				return nil, nil
			}
			return sum / float64(count), nil
		}
		return sum, nil
	case AggMin, AggMax:
		var best any
		set := false
		for _, row := range rows {
			value := row[agg.Field]
			if value == nil {
				continue
			}
			if !set {
				best = value
				set = true
				continue
			}
			cmp := compareAny(value, best)
			if agg.Func == AggMin && cmp < 0 {
				best = value
			}
			if agg.Func == AggMax && cmp > 0 {
				best = value
			}
		}
		return best, nil
	default:
		return nil, fmt.Errorf("unsupported aggregate function %q", agg.Func)
	}
}

func evalExpr(expr Expr, row Row) (bool, error) {
	switch e := expr.(type) {
	case nil:
		return true, nil
	case *LogicalExpr:
		left, err := evalExpr(e.Left, row)
		if err != nil {
			return false, err
		}
		if e.Op == LogicalAnd {
			if !left {
				return false, nil
			}
			return evalExpr(e.Right, row)
		}
		if left {
			return true, nil
		}
		return evalExpr(e.Right, row)
	case *NotExpr:
		ok, err := evalExpr(e.Expr, row)
		return !ok, err
	case *CompareExpr:
		return evalCompare(e, row)
	default:
		return false, fmt.Errorf("unsupported expression %T", expr)
	}
}

func evalCompare(expr *CompareExpr, row Row) (bool, error) {
	actual := row[expr.Field]
	switch expr.Op {
	case OpIsNull:
		return actual == nil, nil
	case OpNotNull:
		return actual != nil, nil
	case OpIn:
		for _, value := range expr.Values {
			if compareAny(actual, value.Any) == 0 {
				return true, nil
			}
		}
		return false, nil
	case OpContains:
		if len(expr.Values) != 1 {
			return false, fmt.Errorf("CONTAINS requires one value")
		}
		return strings.Contains(strings.ToLower(fmt.Sprint(actual)), strings.ToLower(fmt.Sprint(expr.Values[0].Any))), nil
	}
	if len(expr.Values) != 1 {
		return false, fmt.Errorf("%s requires one value", expr.Op)
	}
	cmp := compareAny(actual, expr.Values[0].Any)
	switch expr.Op {
	case OpEq:
		return cmp == 0, nil
	case OpNotEq:
		return cmp != 0, nil
	case OpLT:
		return cmp < 0, nil
	case OpLTE:
		return cmp <= 0, nil
	case OpGT:
		return cmp > 0, nil
	case OpGTE:
		return cmp >= 0, nil
	default:
		return false, fmt.Errorf("unsupported comparison operator %s", expr.Op)
	}
}

func compareAny(a, b any) int {
	af, aok := asFloat(a)
	bf, bok := asFloat(b)
	if aok && bok {
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		default:
			return 0
		}
	}
	as := strings.ToLower(fmt.Sprint(a))
	bs := strings.ToLower(fmt.Sprint(b))
	switch {
	case as < bs:
		return -1
	case as > bs:
		return 1
	default:
		return 0
	}
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func projectRows(selects []SelectItem, rows []Row) []Row {
	if len(selects) == 0 || (len(selects) == 1 && selects[0].Star) {
		return rows
	}
	out := make([]Row, 0, len(rows))
	for _, row := range rows {
		next := Row{}
		for _, item := range selects {
			if item.Star {
				for k, v := range row {
					next[k] = v
				}
				continue
			}
			key := item.Field
			if item.Aggregate != nil || item.Alias != "" {
				key = outputName(item)
			}
			next[key] = row[key]
		}
		out = append(out, next)
	}
	return out
}
