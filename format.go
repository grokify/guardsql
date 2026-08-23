package guardsql

import (
	"fmt"
	"strconv"
	"strings"
)

// FormatStyle controls whether a query is emitted on one line or as a
// human-readable multiline statement.
type FormatStyle string

const (
	FormatMultiline  FormatStyle = "multiline"
	FormatSingleLine FormatStyle = "singleline"
)

// HighlightStyle controls optional syntax highlighting.
type HighlightStyle string

const (
	HighlightNone HighlightStyle = "none"
	HighlightANSI HighlightStyle = "ansi"
)

// FormatOptions configures GuardSQL formatting.
type FormatOptions struct {
	Style     FormatStyle
	Indent    string
	Highlight HighlightStyle
}

// DefaultFormatOptions returns conservative multiline formatting options.
func DefaultFormatOptions() FormatOptions {
	return FormatOptions{
		Style:     FormatMultiline,
		Indent:    "\t",
		Highlight: HighlightNone,
	}
}

// Format parses input and returns canonical GuardSQL text.
func Format(input string, opts FormatOptions) (string, error) {
	q, err := Parse(input)
	if err != nil {
		return "", err
	}
	return FormatQuery(q, opts), nil
}

// FormatQuery formats a parsed GuardSQL query.
func FormatQuery(q *Query, opts FormatOptions) string {
	if q == nil {
		return ""
	}
	opts = normalizeFormatOptions(opts)
	text := formatQuery(q, opts, 0)
	if opts.Style == FormatSingleLine {
		text = strings.Join(strings.Fields(text), " ")
	}
	if opts.Highlight == HighlightANSI {
		return HighlightANSIText(text)
	}
	return text
}

// HighlightANSIText applies terminal ANSI colors to GuardSQL keywords and
// literals. It is best-effort and intended for display, not parsing.
func HighlightANSIText(input string) string {
	toks, err := lexAll(input)
	if err != nil {
		return input
	}
	var b strings.Builder
	last := 0
	for _, tok := range toks {
		if tok.kind == tokEOF {
			break
		}
		end := tokenEnd(input, tok)
		if end < tok.offset || end > len(input) {
			return input
		}
		if tok.offset > last {
			b.WriteString(input[last:tok.offset])
		}
		text := input[tok.offset:end]
		switch {
		case tok.kind == tokIdent && isFormatKeyword(tok.text):
			b.WriteString("\x1b[1;36m")
			b.WriteString(text)
			b.WriteString("\x1b[0m")
		case tok.kind == tokString:
			b.WriteString("\x1b[32m")
			b.WriteString(text)
			b.WriteString("\x1b[0m")
		case tok.kind == tokNumber || tok.kind == tokBool || tok.kind == tokNull:
			b.WriteString("\x1b[35m")
			b.WriteString(text)
			b.WriteString("\x1b[0m")
		default:
			b.WriteString(text)
		}
		last = end
	}
	if last < len(input) {
		b.WriteString(input[last:])
	}
	return b.String()
}

func tokenEnd(input string, tok token) int {
	if tok.kind != tokString {
		return tok.offset + len(tok.text)
	}
	if tok.offset >= len(input) {
		return tok.offset
	}
	quote := input[tok.offset]
	for i := tok.offset + 1; i < len(input); i++ {
		if input[i] == '\\' {
			i++
			continue
		}
		if input[i] == quote {
			return i + 1
		}
	}
	return len(input)
}

func normalizeFormatOptions(opts FormatOptions) FormatOptions {
	if opts.Style == "" {
		opts.Style = FormatMultiline
	}
	if opts.Indent == "" {
		opts.Indent = "\t"
	}
	if opts.Highlight == "" {
		opts.Highlight = HighlightNone
	}
	return opts
}

func formatQuery(q *Query, opts FormatOptions, level int) string {
	clauses := make([]string, 0, 8)
	if len(q.With) > 0 {
		clauses = append(clauses, formatCTEs(q.With, opts, level))
	}
	if len(q.Select) > 0 {
		clauses = append(clauses, "SELECT "+joinSelectItems(q.Select))
	}
	clauses = append(clauses, "FROM "+formatSource(q, opts, level))
	for _, join := range q.Joins {
		clauses = append(clauses, formatJoin(join, opts, level))
	}
	if q.Where != nil {
		clauses = append(clauses, "WHERE "+formatExpr(q.Where))
	}
	if len(q.GroupBy) > 0 {
		clauses = append(clauses, "GROUP BY "+strings.Join(q.GroupBy, ", "))
	}
	if q.Having != nil {
		clauses = append(clauses, "HAVING "+formatExpr(q.Having))
	}
	if len(q.Prioritize) > 0 {
		clauses = append(clauses, "PRIORITIZE BY "+strings.Join(q.Prioritize, ", "))
	}
	if len(q.OrderBy) > 0 {
		clauses = append(clauses, "ORDER BY "+joinOrders(q.OrderBy))
	}
	if q.Limit >= 0 {
		clauses = append(clauses, "LIMIT "+strconv.Itoa(q.Limit))
	}
	if opts.Style == FormatSingleLine {
		return strings.Join(clauses, " ")
	}
	prefix := strings.Repeat(opts.Indent, level)
	return prefix + strings.Join(clauses, "\n"+prefix)
}

func formatCTEs(ctes []CTE, opts FormatOptions, level int) string {
	parts := make([]string, 0, len(ctes))
	for _, cte := range ctes {
		nested := formatQuery(cte.Query, opts, level+1)
		if opts.Style == FormatSingleLine {
			parts = append(parts, cte.Name+" AS ("+nested+")")
			continue
		}
		parts = append(parts, fmt.Sprintf("%s AS (\n%s\n%s)", cte.Name, nested, strings.Repeat(opts.Indent, level)))
	}
	return "WITH " + strings.Join(parts, ", ")
}

func formatSource(q *Query, opts FormatOptions, level int) string {
	var source string
	if q.FromQuery != nil {
		nested := formatQuery(q.FromQuery, opts, level+1)
		if opts.Style == FormatSingleLine {
			source = "(" + nested + ")"
		} else {
			source = "(\n" + nested + "\n" + strings.Repeat(opts.Indent, level) + ")"
		}
	} else {
		source = q.From
	}
	if q.FromAlias != "" {
		source += " AS " + q.FromAlias
	}
	return source
}

func formatJoin(join Join, opts FormatOptions, level int) string {
	var b strings.Builder
	b.WriteString(joinKeyword(join.Type))
	b.WriteByte(' ')
	if join.Query != nil {
		nested := formatQuery(join.Query, opts, level+1)
		if opts.Style == FormatSingleLine {
			b.WriteString("(")
			b.WriteString(nested)
			b.WriteString(")")
		} else {
			b.WriteString("(\n")
			b.WriteString(nested)
			b.WriteByte('\n')
			b.WriteString(strings.Repeat(opts.Indent, level))
			b.WriteString(")")
		}
	} else {
		b.WriteString(join.Entity)
	}
	if join.Alias != "" {
		b.WriteString(" AS ")
		b.WriteString(join.Alias)
	}
	if join.Type != JoinCross && join.Condition != nil {
		b.WriteString(" ON ")
		b.WriteString(formatExpr(join.Condition))
	}
	return b.String()
}

func joinKeyword(t JoinType) string {
	switch t {
	case JoinLeft:
		return "LEFT JOIN"
	case JoinRight:
		return "RIGHT JOIN"
	case JoinFull:
		return "FULL JOIN"
	case JoinCross:
		return "CROSS JOIN"
	default:
		return "JOIN"
	}
}

func joinSelectItems(items []SelectItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		var text string
		switch {
		case item.Star:
			text = "*"
		case item.Aggregate != nil:
			text = formatAggregate(*item.Aggregate)
		default:
			text = item.Field
		}
		if item.Alias != "" {
			text += " AS " + item.Alias
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, ", ")
}

func formatAggregate(agg AggregateExpr) string {
	if agg.Star {
		return string(agg.Func) + "(*)"
	}
	return string(agg.Func) + "(" + agg.Field + ")"
}

func joinOrders(orders []Order) string {
	parts := make([]string, 0, len(orders))
	for _, order := range orders {
		text := order.Field
		if order.Desc {
			text += " DESC"
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, ", ")
}

func formatExpr(expr Expr) string {
	return formatExprPrec(expr, 0)
}

func formatExprPrec(expr Expr, parentPrec int) string {
	switch e := expr.(type) {
	case *LogicalExpr:
		prec := logicalPrecedence(e.Op)
		left := formatExprPrec(e.Left, prec)
		right := formatExprPrec(e.Right, prec+1)
		text := left + " " + string(e.Op) + " " + right
		if prec < parentPrec {
			return "(" + text + ")"
		}
		return text
	case *NotExpr:
		text := "NOT " + formatExprPrec(e.Expr, 3)
		if 3 < parentPrec {
			return "(" + text + ")"
		}
		return text
	case *CompareExpr:
		return formatCompare(e)
	default:
		return ""
	}
}

func logicalPrecedence(op LogicalOp) int {
	if op == LogicalAnd {
		return 2
	}
	return 1
}

func formatCompare(expr *CompareExpr) string {
	switch expr.Op {
	case OpIsNull, OpNotNull:
		return expr.Field + " " + string(expr.Op)
	case OpIn:
		values := make([]string, 0, len(expr.Values))
		for _, value := range expr.Values {
			values = append(values, formatValue(value))
		}
		return expr.Field + " IN (" + strings.Join(values, ", ") + ")"
	case OpContains:
		if len(expr.Values) == 0 {
			return expr.Field + " CONTAINS"
		}
		return expr.Field + " CONTAINS " + formatValue(expr.Values[0])
	default:
		if len(expr.Values) == 0 {
			return expr.Field + " " + string(expr.Op)
		}
		return expr.Field + " " + string(expr.Op) + " " + formatValue(expr.Values[0])
	}
}

func formatValue(value Value) string {
	switch value.Type {
	case ValueString:
		if text, ok := value.Any.(string); ok {
			return strconv.Quote(text)
		}
		return strconv.Quote(strings.Trim(value.Raw, `"`))
	case ValueBool, ValueNull, ValueNumber:
		return value.Raw
	default:
		return value.Raw
	}
}

func isFormatKeyword(word string) bool {
	switch strings.ToUpper(word) {
	case "WITH", "AS", "SELECT", "FROM", "JOIN", "INNER", "LEFT", "RIGHT", "FULL", "OUTER", "CROSS", "ON", "WHERE", "AND", "OR", "NOT", "IN", "CONTAINS", "IS", "NULL", "GROUP", "BY", "HAVING", "PRIORITIZE", "ORDER", "ASC", "DESC", "LIMIT", "COUNT", "SUM", "AVG", "MIN", "MAX":
		return true
	default:
		return false
	}
}
