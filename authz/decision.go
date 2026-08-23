package authz

import "github.com/grokify/guardsql"

// Decision is an authorization result that can be compiled into a GuardSQL
// AST policy. External engines such as SpiceDB should populate this shape from
// trusted application context, not from user-authored query text.
type Decision struct {
	AllowedEntities []string
	Fields          map[string]map[string]guardsql.FieldPolicy
	RequireLimit    bool
	MaxLimit        int
	MaxDepth        int
	MaxNodes        int
	MaxInValues     int
	AllowStar       bool
}

// PolicyFromDecision converts an authorization decision into a GuardSQL
// structural policy. Query endpoints should normally remain read-only.
func PolicyFromDecision(decision Decision) guardsql.Policy {
	return guardsql.Policy{
		AllowedOps:      []guardsql.Operation{guardsql.OperationRead},
		AllowedEntities: decision.AllowedEntities,
		Fields:          decision.Fields,
		AllowStar:       decision.AllowStar,
		RequireLimit:    decision.RequireLimit,
		MaxLimit:        decision.MaxLimit,
		MaxDepth:        decision.MaxDepth,
		MaxNodes:        decision.MaxNodes,
		MaxInValues:     decision.MaxInValues,
	}
}
