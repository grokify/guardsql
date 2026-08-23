package guardsql

import (
	"fmt"
	"strings"
)

// Policy defines AST-level safety rules for a GuardSQL query. It is intended
// for structural query controls; application authorization systems can compile
// their decisions into this policy shape before execution.
type Policy struct {
	AllowedOps         []Operation
	AllowedEntities    []string
	AllowedJoinTypes   []JoinType
	AllowedFunctions   []AggregateFunc
	Fields             map[string]map[string]FieldPolicy
	AllowStar          bool
	AllowCTEs          bool
	AllowNestedSources bool
	RequireLimit       bool
	MaxLimit           int
	MaxDepth           int
	MaxNodes           int
	MaxInValues        int
	MaxSelectItems     int
	MaxOrderFields     int
	MaxGroupFields     int
	MaxJoins           int
	MaxCTEs            int
	MaxSubqueryDepth   int
}

// FieldPolicy controls how one field may be used in a query.
type FieldPolicy struct {
	Selectable     bool
	Filterable     bool
	Sortable       bool
	Groupable      bool
	Aggregatable   bool
	AggregateFuncs []AggregateFunc
	Joinable       bool
}

// PolicyIssue is a policy violation found in a parsed query.
type PolicyIssue struct {
	Message string
}

// CheckPolicy applies AST-level policy checks to a parsed GuardSQL query. It
// does not mutate the query.
func CheckPolicy(q *Query, policy Policy) []PolicyIssue {
	if q == nil {
		return []PolicyIssue{{Message: "query is nil"}}
	}
	var issues []PolicyIssue
	if len(policy.AllowedOps) > 0 && !operationAllowed(q.Operation, policy.AllowedOps) {
		issues = append(issues, PolicyIssue{Message: fmt.Sprintf("operation %q is not allowed", q.Operation)})
	}
	if len(policy.AllowedEntities) > 0 && !stringAllowed(q.From, policy.AllowedEntities) {
		issues = append(issues, PolicyIssue{Message: fmt.Sprintf("entity %q is not allowed", q.From)})
	}
	issues = append(issues, checkStructuralPolicy(q, policy)...)
	if len(policy.Fields) > 0 {
		issues = append(issues, checkFieldPolicy(q, policy)...)
	}
	if policy.RequireLimit && q.Limit < 0 {
		issues = append(issues, PolicyIssue{Message: "LIMIT is required"})
	}
	if policy.MaxLimit > 0 && (q.Limit < 0 || q.Limit > policy.MaxLimit) {
		issues = append(issues, PolicyIssue{Message: fmt.Sprintf("LIMIT exceeds maximum %d", policy.MaxLimit)})
	}
	if policy.MaxDepth > 0 {
		if depth := exprDepth(q.Where); depth > policy.MaxDepth {
			issues = append(issues, PolicyIssue{Message: fmt.Sprintf("WHERE expression depth %d exceeds maximum %d", depth, policy.MaxDepth)})
		}
	}
	if policy.MaxNodes > 0 {
		if nodes := exprNodes(q.Where); nodes > policy.MaxNodes {
			issues = append(issues, PolicyIssue{Message: fmt.Sprintf("WHERE expression node count %d exceeds maximum %d", nodes, policy.MaxNodes)})
		}
	}
	if policy.MaxInValues > 0 {
		walkExpr(q.Where, func(expr Expr) {
			cmp, ok := expr.(*CompareExpr)
			if !ok || cmp.Op != OpIn {
				return
			}
			if len(cmp.Values) > policy.MaxInValues {
				issues = append(issues, PolicyIssue{Message: fmt.Sprintf("IN value count %d exceeds maximum %d", len(cmp.Values), policy.MaxInValues)})
			}
		})
	}
	return issues
}

// CheckAnalysisPolicy applies policy to resolved analysis requirements. It is
// preferred for operational authorization because aliases, CTEs, and nested
// sources have already been resolved into concrete resource and field usage.
func CheckAnalysisPolicy(analysis Analysis, policy Policy) []PolicyIssue {
	var issues []PolicyIssue
	reqs := analysis.Requirements()
	for _, op := range reqs.Operations {
		if len(policy.AllowedOps) > 0 && !operationAllowed(op, policy.AllowedOps) {
			issues = append(issues, PolicyIssue{Message: fmt.Sprintf("operation %q is not allowed", op)})
		}
	}
	for _, entity := range reqs.Entities {
		if len(policy.AllowedEntities) > 0 && !stringAllowed(entity, policy.AllowedEntities) {
			issues = append(issues, PolicyIssue{Message: fmt.Sprintf("entity %q is not allowed", entity)})
		}
	}
	for _, fn := range reqs.Functions {
		if len(policy.AllowedFunctions) > 0 && !aggregateAllowed(fn, policy.AllowedFunctions) {
			issues = append(issues, PolicyIssue{Message: fmt.Sprintf("aggregate function %q is not allowed", fn)})
		}
	}
	for _, join := range reqs.Joins {
		if len(policy.AllowedJoinTypes) > 0 && !joinAllowed(join.Type, policy.AllowedJoinTypes) {
			issues = append(issues, PolicyIssue{Message: fmt.Sprintf("%s join is not allowed", join.Type)})
		}
	}
	if len(policy.Fields) > 0 {
		for _, req := range reqs.Fields {
			fieldPolicy, ok := fieldPolicyFor(policy, req.Entity, req.Field)
			if !ok || !fieldUsageAllowed(req, fieldPolicy) {
				issues = append(issues, PolicyIssue{Message: fmt.Sprintf("field %q.%q is not allowed for %s by policy", req.Entity, req.Field, req.Usage)})
			}
		}
	}
	return issues
}

// SafeAnalyticsPolicy returns a deny-by-default read-only policy derived from a
// schema. Host applications should tighten limits and authorization further for
// each deployment.
func SafeAnalyticsPolicy(schema Schema) Policy {
	schema = schema.Normalize()
	policy := Policy{
		AllowedOps:         []Operation{OperationRead},
		AllowedEntities:    make([]string, 0, len(schema.Entities)),
		AllowedJoinTypes:   []JoinType{JoinInner, JoinLeft},
		AllowedFunctions:   []AggregateFunc{AggCount, AggSum, AggAvg, AggMin, AggMax},
		Fields:             map[string]map[string]FieldPolicy{},
		RequireLimit:       true,
		MaxLimit:           firstPositive(schema.MaxLimit, 1000),
		MaxDepth:           8,
		MaxNodes:           80,
		MaxInValues:        100,
		MaxSelectItems:     50,
		MaxOrderFields:     5,
		MaxGroupFields:     5,
		MaxJoins:           3,
		MaxCTEs:            0,
		MaxSubqueryDepth:   0,
		AllowCTEs:          false,
		AllowNestedSources: false,
	}
	for entityName, entity := range schema.Entities {
		policy.AllowedEntities = append(policy.AllowedEntities, entityName)
		fieldPolicies := map[string]FieldPolicy{}
		for fieldName, field := range entity.Fields {
			fieldPolicy := FieldPolicy{
				Selectable: field.Selectable,
				Filterable: field.Filterable,
				Sortable:   field.Sortable,
				Groupable:  field.Sortable,
				Joinable:   false,
			}
			if field.Selectable {
				fieldPolicy.AggregateFuncs = []AggregateFunc{AggCount}
				if field.Type == FieldNumber {
					fieldPolicy.Aggregatable = true
					fieldPolicy.AggregateFuncs = []AggregateFunc{AggCount, AggSum, AggAvg, AggMin, AggMax}
				}
			}
			fieldPolicies[fieldName] = fieldPolicy
		}
		policy.Fields[entityName] = fieldPolicies
	}
	return policy
}

func checkFieldPolicy(q *Query, policy Policy) []PolicyIssue {
	fieldPolicies, ok := policy.Fields[strings.ToLower(q.From)]
	if !ok {
		return nil
	}
	var issues []PolicyIssue
	for _, item := range q.Select {
		if item.Star {
			if !policy.AllowStar {
				issues = append(issues, PolicyIssue{Message: "SELECT * is not allowed when field policy is configured"})
			}
			continue
		}
		if item.Aggregate != nil {
			if item.Aggregate.Star {
				continue
			}
			fieldPolicy, ok := fieldPolicies[strings.ToLower(item.Aggregate.Field)]
			if !ok || !aggregateFieldAllowed(item.Aggregate.Func, fieldPolicy) {
				issues = append(issues, PolicyIssue{Message: fmt.Sprintf("field %q is not aggregatable by policy", item.Aggregate.Field)})
			}
			continue
		}
		fieldPolicy, ok := fieldPolicies[strings.ToLower(item.Field)]
		if !ok || !fieldPolicy.Selectable {
			issues = append(issues, PolicyIssue{Message: fmt.Sprintf("field %q is not selectable by policy", item.Field)})
		}
	}
	for _, field := range q.GroupBy {
		fieldPolicy, ok := fieldPolicies[strings.ToLower(field)]
		if !ok || !groupAllowed(fieldPolicy) {
			issues = append(issues, PolicyIssue{Message: fmt.Sprintf("field %q is not groupable by policy", field)})
		}
	}
	walkExpr(q.Where, func(expr Expr) {
		cmp, ok := expr.(*CompareExpr)
		if !ok {
			return
		}
		fieldPolicy, ok := fieldPolicies[strings.ToLower(cmp.Field)]
		if !ok || !fieldPolicy.Filterable {
			issues = append(issues, PolicyIssue{Message: fmt.Sprintf("field %q is not filterable by policy", cmp.Field)})
		}
	})
	for _, order := range q.OrderBy {
		fieldPolicy, ok := fieldPolicies[strings.ToLower(order.Field)]
		if !ok || !fieldPolicy.Sortable {
			issues = append(issues, PolicyIssue{Message: fmt.Sprintf("field %q is not sortable by policy", order.Field)})
		}
	}
	return issues
}

func checkStructuralPolicy(q *Query, policy Policy) []PolicyIssue {
	var issues []PolicyIssue
	if len(q.Select) > 0 && policy.MaxSelectItems > 0 && len(q.Select) > policy.MaxSelectItems {
		issues = append(issues, PolicyIssue{Message: fmt.Sprintf("SELECT item count %d exceeds maximum %d", len(q.Select), policy.MaxSelectItems)})
	}
	if policy.MaxOrderFields > 0 && len(q.OrderBy) > policy.MaxOrderFields {
		issues = append(issues, PolicyIssue{Message: fmt.Sprintf("ORDER BY field count %d exceeds maximum %d", len(q.OrderBy), policy.MaxOrderFields)})
	}
	if policy.MaxGroupFields > 0 && len(q.GroupBy) > policy.MaxGroupFields {
		issues = append(issues, PolicyIssue{Message: fmt.Sprintf("GROUP BY field count %d exceeds maximum %d", len(q.GroupBy), policy.MaxGroupFields)})
	}
	if policy.MaxJoins > 0 && len(q.Joins) > policy.MaxJoins {
		issues = append(issues, PolicyIssue{Message: fmt.Sprintf("join count %d exceeds maximum %d", len(q.Joins), policy.MaxJoins)})
	}
	if len(q.Joins) > 0 {
		for _, join := range q.Joins {
			if len(policy.AllowedJoinTypes) > 0 && !joinAllowed(join.Type, policy.AllowedJoinTypes) {
				issues = append(issues, PolicyIssue{Message: fmt.Sprintf("%s join is not allowed", join.Type)})
			}
		}
	}
	if len(q.With) > 0 && !policy.AllowCTEs {
		issues = append(issues, PolicyIssue{Message: "CTEs are not allowed"})
	}
	if policy.MaxCTEs > 0 && len(q.With) > policy.MaxCTEs {
		issues = append(issues, PolicyIssue{Message: fmt.Sprintf("CTE count %d exceeds maximum %d", len(q.With), policy.MaxCTEs)})
	}
	if (q.FromQuery != nil || hasJoinedNestedSource(q)) && !policy.AllowNestedSources {
		issues = append(issues, PolicyIssue{Message: "nested query sources are not allowed"})
	}
	if policy.MaxSubqueryDepth > 0 {
		if depth := subqueryDepth(q); depth > policy.MaxSubqueryDepth {
			issues = append(issues, PolicyIssue{Message: fmt.Sprintf("subquery depth %d exceeds maximum %d", depth, policy.MaxSubqueryDepth)})
		}
	}
	for _, item := range q.Select {
		if item.Aggregate == nil {
			continue
		}
		if len(policy.AllowedFunctions) > 0 && !aggregateAllowed(item.Aggregate.Func, policy.AllowedFunctions) {
			issues = append(issues, PolicyIssue{Message: fmt.Sprintf("aggregate function %q is not allowed", item.Aggregate.Func)})
		}
	}
	for _, cte := range q.With {
		if cte.Query != nil {
			issues = append(issues, CheckPolicy(cte.Query, childPolicy(policy))...)
		}
	}
	if q.FromQuery != nil {
		issues = append(issues, CheckPolicy(q.FromQuery, childPolicy(policy))...)
	}
	for _, join := range q.Joins {
		if join.Query != nil {
			issues = append(issues, CheckPolicy(join.Query, childPolicy(policy))...)
		}
	}
	return issues
}

func operationAllowed(op Operation, allowed []Operation) bool {
	for _, candidate := range allowed {
		if op == candidate {
			return true
		}
	}
	return false
}

func stringAllowed(value string, allowed []string) bool {
	for _, candidate := range allowed {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}

func joinAllowed(join JoinType, allowed []JoinType) bool {
	for _, candidate := range allowed {
		if join == candidate {
			return true
		}
	}
	return false
}

func aggregateAllowed(fn AggregateFunc, allowed []AggregateFunc) bool {
	for _, candidate := range allowed {
		if fn == candidate {
			return true
		}
	}
	return false
}

func fieldPolicyFor(policy Policy, entity string, field string) (FieldPolicy, bool) {
	fields, ok := policy.Fields[strings.ToLower(entity)]
	if !ok {
		return FieldPolicy{}, false
	}
	fieldPolicy, ok := fields[strings.ToLower(field)]
	return fieldPolicy, ok
}

func fieldUsageAllowed(req FieldRequirement, policy FieldPolicy) bool {
	switch req.Usage {
	case FieldUsageSelect:
		return policy.Selectable
	case FieldUsageFilter, FieldUsageHaving:
		return policy.Filterable
	case FieldUsageSort:
		return policy.Sortable
	case FieldUsageGroup:
		return groupAllowed(policy)
	case FieldUsageAggregate:
		return aggregateFieldAllowed(req.Aggregate, policy)
	case FieldUsageJoin:
		return policy.Joinable
	default:
		return false
	}
}

func groupAllowed(policy FieldPolicy) bool {
	return policy.Groupable || policy.Sortable
}

func aggregateFieldAllowed(fn AggregateFunc, policy FieldPolicy) bool {
	if len(policy.AggregateFuncs) > 0 {
		return aggregateAllowed(fn, policy.AggregateFuncs)
	}
	return policy.Aggregatable || policy.Selectable
}

func hasJoinedNestedSource(q *Query) bool {
	for _, join := range q.Joins {
		if join.Query != nil {
			return true
		}
	}
	return false
}

func subqueryDepth(q *Query) int {
	if q == nil {
		return 0
	}
	maxDepth := 0
	for _, cte := range q.With {
		maxDepth = max(maxDepth, subqueryDepth(cte.Query))
	}
	if q.FromQuery != nil {
		maxDepth = max(maxDepth, 1+subqueryDepth(q.FromQuery))
	}
	for _, join := range q.Joins {
		if join.Query != nil {
			maxDepth = max(maxDepth, 1+subqueryDepth(join.Query))
		}
	}
	return maxDepth
}

func childPolicy(policy Policy) Policy {
	policy.RequireLimit = false
	return policy
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func exprDepth(expr Expr) int {
	switch e := expr.(type) {
	case nil:
		return 0
	case *LogicalExpr:
		return 1 + max(exprDepth(e.Left), exprDepth(e.Right))
	case *NotExpr:
		return 1 + exprDepth(e.Expr)
	default:
		return 1
	}
}

func exprNodes(expr Expr) int {
	switch e := expr.(type) {
	case nil:
		return 0
	case *LogicalExpr:
		return 1 + exprNodes(e.Left) + exprNodes(e.Right)
	case *NotExpr:
		return 1 + exprNodes(e.Expr)
	default:
		return 1
	}
}

func walkExpr(expr Expr, fn func(Expr)) {
	if expr == nil {
		return
	}
	fn(expr)
	switch e := expr.(type) {
	case *LogicalExpr:
		walkExpr(e.Left, fn)
		walkExpr(e.Right, fn)
	case *NotExpr:
		walkExpr(e.Expr, fn)
	}
}
