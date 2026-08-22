package grokifyql

import "fmt"

// Query is the parsed representation of a GrokifyQL query.
type Query struct {
	Operation  Operation
	With       []CTE
	Select     []SelectItem
	From       string
	FromQuery  *Query
	FromAlias  string
	Joins      []Join
	Where      Expr
	GroupBy    []string
	Having     Expr
	Prioritize []string
	OrderBy    []Order
	Limit      int
}

// CTE is a named common table expression.
type CTE struct {
	Name  string
	Query *Query
}

// Join represents a relational join against an entity or nested query.
type Join struct {
	Type      JoinType
	Entity    string
	Query     *Query
	Alias     string
	Condition Expr
}

// JoinType identifies the join variant.
type JoinType string

const (
	JoinInner JoinType = "inner"
	JoinLeft  JoinType = "left"
	JoinRight JoinType = "right"
	JoinFull  JoinType = "full"
	JoinCross JoinType = "cross"
)

// Operation identifies the CRUD operation represented by a query.
type Operation string

const (
	OperationRead   Operation = "read"
	OperationCreate Operation = "create"
	OperationUpdate Operation = "update"
	OperationDelete Operation = "delete"
)

// SelectItem is a projected field. Star is true for SELECT *.
type SelectItem struct {
	Field     string
	Star      bool
	Aggregate *AggregateExpr
	Alias     string
}

// AggregateExpr is an aggregate function call in SELECT.
type AggregateExpr struct {
	Func  AggregateFunc
	Field string
	Star  bool
}

// AggregateFunc identifies a supported aggregate function.
type AggregateFunc string

const (
	AggCount AggregateFunc = "COUNT"
	AggSum   AggregateFunc = "SUM"
	AggAvg   AggregateFunc = "AVG"
	AggMin   AggregateFunc = "MIN"
	AggMax   AggregateFunc = "MAX"
)

// Order defines a sort field and direction.
type Order struct {
	Field string
	Desc  bool
}

// Expr is implemented by all filter expression nodes.
type Expr interface {
	exprNode()
}

// LogicalExpr combines two expressions with AND or OR.
type LogicalExpr struct {
	Op          LogicalOp
	Left, Right Expr
}

func (*LogicalExpr) exprNode() {}

// NotExpr negates an expression.
type NotExpr struct {
	Expr Expr
}

func (*NotExpr) exprNode() {}

// CompareExpr compares one field to one or more literal values.
type CompareExpr struct {
	Field  string
	Op     CompareOp
	Values []Value
}

func (*CompareExpr) exprNode() {}

// LogicalOp is a boolean operator.
type LogicalOp string

const (
	LogicalAnd LogicalOp = "AND"
	LogicalOr  LogicalOp = "OR"
)

// CompareOp is a field comparison operator.
type CompareOp string

const (
	OpEq       CompareOp = "="
	OpNotEq    CompareOp = "!="
	OpLT       CompareOp = "<"
	OpLTE      CompareOp = "<="
	OpGT       CompareOp = ">"
	OpGTE      CompareOp = ">="
	OpIn       CompareOp = "IN"
	OpContains CompareOp = "CONTAINS"
	OpIsNull   CompareOp = "IS NULL"
	OpNotNull  CompareOp = "IS NOT NULL"
)

// Value is a typed query literal.
type Value struct {
	Type ValueType
	Raw  string
	Any  any
}

// ValueType is the literal kind.
type ValueType string

const (
	ValueString ValueType = "string"
	ValueNumber ValueType = "number"
	ValueBool   ValueType = "bool"
	ValueNull   ValueType = "null"
)

// ParseError is returned for invalid syntax.
type ParseError struct {
	Message string
	Offset  int
}

func (e *ParseError) Error() string {
	if e.Offset > 0 {
		return fmt.Sprintf("parse error at byte %d: %s", e.Offset, e.Message)
	}
	return "parse error: " + e.Message
}
