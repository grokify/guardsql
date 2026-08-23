package guardsql

// LintConfig defines static policy checks for user-authored GuardSQL.
type LintConfig struct {
	Schema       Schema
	AllowedOps   []Operation
	MaxDepth     int
	MaxNodes     int
	MaxInValues  int
	RequireLimit bool
}

// LintIssue is a syntax, schema, or policy problem found in a query.
type LintIssue struct {
	Message string
}

// Lint parses and validates a query, then applies static SaaS safety checks.
func Lint(input string, cfg LintConfig) (*Query, []LintIssue) {
	q, err := Parse(input)
	if err != nil {
		return nil, []LintIssue{{Message: err.Error()}}
	}
	hadLimit := q.Limit >= 0
	if err := Validate(q, cfg.Schema); err != nil {
		return q, []LintIssue{{Message: err.Error()}}
	}

	limit := q.Limit
	if !hadLimit {
		limit = -1
	}
	policyIssues := CheckPolicy(&Query{
		Operation: q.Operation,
		Select:    q.Select,
		From:      q.From,
		Where:     q.Where,
		OrderBy:   q.OrderBy,
		Limit:     limit,
	}, Policy{
		AllowedOps:   cfg.AllowedOps,
		RequireLimit: cfg.RequireLimit,
		MaxDepth:     cfg.MaxDepth,
		MaxNodes:     cfg.MaxNodes,
		MaxInValues:  cfg.MaxInValues,
	})
	issues := make([]LintIssue, 0, len(policyIssues))
	for _, issue := range policyIssues {
		issues = append(issues, LintIssue{Message: issue.Message})
	}
	return q, issues
}
