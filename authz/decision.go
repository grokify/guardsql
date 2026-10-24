package authz

import "github.com/grokify/guardsql"

// Decision is an authorization result that can be compiled into a GuardSQL
// AST policy. External engines such as SpiceDB should populate this shape from
// trusted application context, not from user-authored query text.
//
// AllowedEntities and Fields are strict allowlists: an empty AllowedEntities
// allows no entity, and an entity without a Fields entry has no usable
// fields.
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
// structural policy with strict allowlists, so a decision that allows nothing
// denies every query. Query endpoints should normally remain read-only.
func PolicyFromDecision(decision Decision) guardsql.Policy {
	return guardsql.Policy{
		AllowedOps:       []guardsql.Operation{guardsql.OperationRead},
		AllowedEntities:  decision.AllowedEntities,
		StrictAllowlists: true,
		Fields:           decision.Fields,
		AllowStar:        decision.AllowStar,
		RequireLimit:     decision.RequireLimit,
		MaxLimit:         decision.MaxLimit,
		MaxDepth:         decision.MaxDepth,
		MaxNodes:         decision.MaxNodes,
		MaxInValues:      decision.MaxInValues,
	}
}
