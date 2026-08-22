package grokifyql

import (
	"fmt"
	"strconv"
	"strings"
)

// Parse parses a read-only GrokifyQL statement into an AST.
func Parse(input string) (*Query, error) {
	toks, err := lexAll(input)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: toks}
	return p.parseQuery()
}

type parser struct {
	tokens []token
	pos    int
}

func (p *parser) parseQuery() (*Query, error) {
	q := &Query{Operation: OperationRead, Limit: -1}
	if p.matchKeyword("WITH") {
		ctes, err := p.parseCTEs()
		if err != nil {
			return nil, err
		}
		q.With = ctes
	}
	if p.matchKeyword("SELECT") {
		items, err := p.parseSelectList()
		if err != nil {
			return nil, err
		}
		q.Select = items
		if err := p.expectKeyword("FROM"); err != nil {
			return nil, err
		}
	} else if err := p.expectKeyword("FROM"); err != nil {
		return nil, err
	}

	if p.match(tokLParen) {
		subquery, err := p.parseQuery()
		if err != nil {
			return nil, err
		}
		if !p.match(tokRParen) {
			return nil, p.err("expected ')' after nested query")
		}
		q.FromQuery = subquery
		alias, err := p.parseOptionalAlias()
		if err != nil {
			return nil, err
		}
		q.FromAlias = alias
	} else {
		from, err := p.expectIdent("entity")
		if err != nil {
			return nil, err
		}
		q.From = strings.ToLower(from)
		alias, err := p.parseOptionalAlias()
		if err != nil {
			return nil, err
		}
		q.FromAlias = alias
	}

	joins, err := p.parseJoins()
	if err != nil {
		return nil, err
	}
	q.Joins = joins

	if p.matchKeyword("WHERE") {
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		q.Where = expr
	}
	if p.matchKeyword("GROUP") {
		if err := p.expectKeyword("BY"); err != nil {
			return nil, err
		}
		groupBy, err := p.parseIdentList("group field")
		if err != nil {
			return nil, err
		}
		q.GroupBy = groupBy
	}
	if p.matchKeyword("HAVING") {
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		q.Having = expr
	}
	if p.matchKeyword("PRIORITIZE") {
		if err := p.expectKeyword("BY"); err != nil {
			return nil, err
		}
		strategies, err := p.parseIdentList("strategy")
		if err != nil {
			return nil, err
		}
		q.Prioritize = strategies
	}
	if p.matchKeyword("ORDER") {
		if err := p.expectKeyword("BY"); err != nil {
			return nil, err
		}
		orders, err := p.parseOrderList()
		if err != nil {
			return nil, err
		}
		q.OrderBy = orders
	}
	if p.matchKeyword("LIMIT") {
		tok := p.peek()
		if tok.kind != tokNumber {
			return nil, p.err("expected numeric LIMIT")
		}
		p.pos++
		n, err := strconv.Atoi(tok.text)
		if err != nil {
			return nil, &ParseError{Message: "LIMIT must be an integer", Offset: tok.offset}
		}
		q.Limit = n
	}
	if p.peek().kind != tokEOF && p.peek().kind != tokRParen {
		return nil, p.err("unexpected token " + strconv.Quote(p.peek().text))
	}
	return q, nil
}

func (p *parser) parseCTEs() ([]CTE, error) {
	var ctes []CTE
	for {
		name, err := p.expectIdent("cte name")
		if err != nil {
			return nil, err
		}
		if err := p.expectKeyword("AS"); err != nil {
			return nil, err
		}
		if !p.match(tokLParen) {
			return nil, p.err("expected '(' before CTE query")
		}
		query, err := p.parseQuery()
		if err != nil {
			return nil, err
		}
		if !p.match(tokRParen) {
			return nil, p.err("expected ')' after CTE query")
		}
		ctes = append(ctes, CTE{Name: strings.ToLower(name), Query: query})
		if !p.match(tokComma) {
			return ctes, nil
		}
	}
}

func (p *parser) parseJoins() ([]Join, error) {
	var joins []Join
	for {
		joinType, ok, err := p.parseJoinType()
		if err != nil {
			return nil, err
		}
		if !ok {
			return joins, nil
		}
		join := Join{Type: joinType}
		if p.match(tokLParen) {
			subquery, err := p.parseQuery()
			if err != nil {
				return nil, err
			}
			if !p.match(tokRParen) {
				return nil, p.err("expected ')' after joined query")
			}
			join.Query = subquery
			alias, err := p.parseOptionalAlias()
			if err != nil {
				return nil, err
			}
			join.Alias = alias
		} else {
			entity, err := p.expectIdent("join entity")
			if err != nil {
				return nil, err
			}
			join.Entity = strings.ToLower(entity)
			alias, err := p.parseOptionalAlias()
			if err != nil {
				return nil, err
			}
			join.Alias = alias
		}
		if joinType != JoinCross {
			if err := p.expectKeyword("ON"); err != nil {
				return nil, err
			}
			condition, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			join.Condition = condition
		}
		joins = append(joins, join)
	}
}

func (p *parser) parseJoinType() (JoinType, bool, error) {
	if p.matchKeyword("JOIN") {
		return JoinInner, true, nil
	}
	if p.matchKeyword("INNER") {
		if err := p.expectKeyword("JOIN"); err != nil {
			return "", false, err
		}
		return JoinInner, true, nil
	}
	if p.matchKeyword("LEFT") {
		p.matchKeyword("OUTER")
		if err := p.expectKeyword("JOIN"); err != nil {
			return "", false, err
		}
		return JoinLeft, true, nil
	}
	if p.matchKeyword("RIGHT") {
		p.matchKeyword("OUTER")
		if err := p.expectKeyword("JOIN"); err != nil {
			return "", false, err
		}
		return JoinRight, true, nil
	}
	if p.matchKeyword("FULL") {
		p.matchKeyword("OUTER")
		if err := p.expectKeyword("JOIN"); err != nil {
			return "", false, err
		}
		return JoinFull, true, nil
	}
	if p.matchKeyword("CROSS") {
		if err := p.expectKeyword("JOIN"); err != nil {
			return "", false, err
		}
		return JoinCross, true, nil
	}
	return "", false, nil
}

func (p *parser) parseOptionalAlias() (string, error) {
	p.matchKeyword("AS")
	tok := p.peek()
	if tok.kind != tokIdent {
		return "", nil
	}
	if isClauseKeyword(tok.text) {
		return "", nil
	}
	p.pos++
	return strings.ToLower(tok.text), nil
}

func (p *parser) parseIdentList(name string) ([]string, error) {
	var values []string
	for {
		value, err := p.expectIdent(name)
		if err != nil {
			return nil, err
		}
		values = append(values, strings.ToLower(value))
		if !p.match(tokComma) {
			return values, nil
		}
	}
}

func (p *parser) parseSelectList() ([]SelectItem, error) {
	var items []SelectItem
	for {
		if p.match(tokStar) {
			items = append(items, SelectItem{Star: true})
		} else {
			name, err := p.expectIdent("field")
			if err != nil {
				return nil, err
			}
			item := SelectItem{Field: strings.ToLower(name)}
			if p.match(tokLParen) {
				agg, err := p.parseAggregate(name)
				if err != nil {
					return nil, err
				}
				item = SelectItem{Aggregate: agg}
			}
			if p.matchKeyword("AS") {
				alias, err := p.expectIdent("alias")
				if err != nil {
					return nil, err
				}
				item.Alias = alias
			}
			items = append(items, item)
		}
		if !p.match(tokComma) {
			return items, nil
		}
	}
}

func (p *parser) parseAggregate(name string) (*AggregateExpr, error) {
	fn := AggregateFunc(strings.ToUpper(name))
	switch fn {
	case AggCount, AggSum, AggAvg, AggMin, AggMax:
	default:
		return nil, p.err("unsupported aggregate function " + strconv.Quote(name))
	}
	agg := &AggregateExpr{Func: fn}
	if p.match(tokStar) {
		if fn != AggCount {
			return nil, p.err("only COUNT supports '*'")
		}
		agg.Star = true
	} else {
		field, err := p.expectIdent("aggregate field")
		if err != nil {
			return nil, err
		}
		agg.Field = strings.ToLower(field)
	}
	if !p.match(tokRParen) {
		return nil, p.err("expected ')' after aggregate")
	}
	return agg, nil
}

func (p *parser) parseOrderList() ([]Order, error) {
	var orders []Order
	for {
		field, err := p.expectIdent("field")
		if err != nil {
			return nil, err
		}
		order := Order{Field: strings.ToLower(field)}
		if p.matchKeyword("DESC") {
			order.Desc = true
		} else {
			p.matchKeyword("ASC")
		}
		orders = append(orders, order)
		if !p.match(tokComma) {
			return orders, nil
		}
	}
}

func (p *parser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.matchKeyword("OR") {
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &LogicalExpr{Op: LogicalOr, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.matchKeyword("AND") {
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &LogicalExpr{Op: LogicalAnd, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseUnary() (Expr, error) {
	if p.matchKeyword("NOT") {
		expr, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &NotExpr{Expr: expr}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (Expr, error) {
	if p.match(tokLParen) {
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if !p.match(tokRParen) {
			return nil, p.err("expected ')'")
		}
		return expr, nil
	}
	field, err := p.expectIdent("field")
	if err != nil {
		return nil, err
	}
	field = strings.ToLower(field)
	if p.matchKeyword("IS") {
		if p.matchKeyword("NOT") {
			if err := p.expectKeyword("NULL"); err != nil {
				return nil, err
			}
			return &CompareExpr{Field: field, Op: OpNotNull}, nil
		}
		if err := p.expectKeyword("NULL"); err != nil {
			return nil, err
		}
		return &CompareExpr{Field: field, Op: OpIsNull}, nil
	}
	if p.matchKeyword("IN") {
		values, err := p.parseValueList()
		if err != nil {
			return nil, err
		}
		return &CompareExpr{Field: field, Op: OpIn, Values: values}, nil
	}
	if p.matchKeyword("CONTAINS") {
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		return &CompareExpr{Field: field, Op: OpContains, Values: []Value{value}}, nil
	}
	tok := p.peek()
	if tok.kind != tokOp {
		return nil, p.err("expected comparison operator")
	}
	p.pos++
	value, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	return &CompareExpr{Field: field, Op: CompareOp(tok.text), Values: []Value{value}}, nil
}

func (p *parser) parseValueList() ([]Value, error) {
	if !p.match(tokLParen) {
		return nil, p.err("expected '('")
	}
	var values []Value
	for {
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		if !p.match(tokComma) {
			break
		}
	}
	if !p.match(tokRParen) {
		return nil, p.err("expected ')'")
	}
	return values, nil
}

func (p *parser) parseValue() (Value, error) {
	tok := p.peek()
	p.pos++
	switch tok.kind {
	case tokString:
		return Value{Type: ValueString, Raw: strconv.Quote(tok.text), Any: tok.text}, nil
	case tokNumber:
		n, err := strconv.ParseFloat(tok.text, 64)
		if err != nil {
			return Value{}, &ParseError{Message: "invalid number", Offset: tok.offset}
		}
		return Value{Type: ValueNumber, Raw: tok.text, Any: n}, nil
	case tokBool:
		return Value{Type: ValueBool, Raw: tok.text, Any: tok.text == "true"}, nil
	case tokNull:
		return Value{Type: ValueNull, Raw: "null", Any: nil}, nil
	case tokIdent:
		return Value{Type: ValueString, Raw: strconv.Quote(tok.text), Any: tok.text}, nil
	default:
		return Value{}, &ParseError{Message: "expected literal value", Offset: tok.offset}
	}
}

func (p *parser) expectIdent(name string) (string, error) {
	tok := p.peek()
	if tok.kind != tokIdent {
		return "", p.err("expected " + name)
	}
	p.pos++
	return tok.text, nil
}

func (p *parser) expectKeyword(word string) error {
	if p.matchKeyword(word) {
		return nil
	}
	return p.err("expected " + word)
}

func (p *parser) matchKeyword(word string) bool {
	tok := p.peek()
	if tok.kind == tokIdent && strings.EqualFold(tok.text, word) {
		p.pos++
		return true
	}
	if tok.kind == tokNull && strings.EqualFold(word, "NULL") {
		p.pos++
		return true
	}
	return false
}

func (p *parser) match(kind tokenKind) bool {
	if p.peek().kind == kind {
		p.pos++
		return true
	}
	return false
}

func (p *parser) peek() token {
	if p.pos >= len(p.tokens) {
		return token{kind: tokEOF}
	}
	return p.tokens[p.pos]
}

func (p *parser) err(msg string) error {
	tok := p.peek()
	if tok.kind == tokEOF {
		return &ParseError{Message: fmt.Sprintf("%s before end of input", msg), Offset: tok.offset}
	}
	return &ParseError{Message: msg, Offset: tok.offset}
}

func isClauseKeyword(word string) bool {
	switch strings.ToUpper(word) {
	case "JOIN", "INNER", "LEFT", "RIGHT", "FULL", "OUTER", "CROSS", "WHERE", "GROUP", "HAVING", "PRIORITIZE", "ORDER", "LIMIT", "ON":
		return true
	default:
		return false
	}
}
