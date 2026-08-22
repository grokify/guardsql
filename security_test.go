package grokifyql

import (
	"strings"
	"testing"
)

func TestParseRejectsDangerousSQLShapes(t *testing.T) {
	cases := []string{
		`SELECT id FROM roadmap_items; DROP TABLE roadmap_items`,
		`DELETE FROM roadmap_items`,
		`UPDATE roadmap_items SET status = "done"`,
		`CREATE TABLE owned (id text)`,
		`SELECT pg_read_file("/etc/passwd") FROM roadmap_items`,
		`SELECT id FROM roadmap_items UNION SELECT id FROM users`,
		`SELECT id FROM roadmap_items FOR UPDATE`,
	}
	for _, input := range cases {
		if _, err := Parse(input); err == nil {
			t.Fatalf("%q parsed successfully, want error", input)
		}
	}
}

func TestSafeAnalyticsPolicyRejectsUnboundedAndBroadQuery(t *testing.T) {
	q, err := Parse(`SELECT * FROM roadmap_items`)
	if err != nil {
		t.Fatal(err)
	}
	issues := CheckPolicy(q, SafeAnalyticsPolicy(testSchema()))
	assertPolicyIssueContains(t, issues, "SELECT * is not allowed")
	assertPolicyIssueContains(t, issues, "LIMIT is required")
}

func TestCheckPolicyRejectsDisallowedFunctionsJoinsCTEsAndNestedSources(t *testing.T) {
	policy := SafeAnalyticsPolicy(testSchema())
	policy.RequireLimit = false

	cases := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "aggregate function",
			sql:  `SELECT AVG(score) FROM roadmap_items LIMIT 10`,
			want: `aggregate function "AVG" is not allowed`,
		},
		{
			name: "full join",
			sql:  `SELECT a.id FROM roadmap_items a FULL JOIN roadmap_items b ON a.status = "planned" LIMIT 10`,
			want: `full join is not allowed`,
		},
		{
			name: "cross join",
			sql:  `SELECT a.id FROM roadmap_items a CROSS JOIN roadmap_items b LIMIT 10`,
			want: `cross join is not allowed`,
		},
		{
			name: "cte",
			sql:  `WITH active AS (SELECT id FROM roadmap_items LIMIT 10) SELECT id FROM active LIMIT 10`,
			want: `CTEs are not allowed`,
		},
		{
			name: "nested",
			sql:  `SELECT id FROM (SELECT id FROM roadmap_items LIMIT 10) AS active LIMIT 10`,
			want: `nested query sources are not allowed`,
		},
	}

	policy.AllowedFunctions = []AggregateFunc{AggCount, AggSum}
	policy.AllowedJoinTypes = []JoinType{JoinInner, JoinLeft}
	for _, tc := range cases {
		q, err := Parse(tc.sql)
		if err != nil {
			t.Fatalf("%s: parse: %v", tc.name, err)
		}
		issues := CheckPolicy(q, policy)
		assertPolicyIssueContains(t, issues, tc.want)
	}
}

func TestCheckPolicyRejectsQueryDoSShapes(t *testing.T) {
	q, err := Parse(`SELECT id, name, status FROM roadmap_items WHERE status IN ("a", "b", "c") GROUP BY id, name, status ORDER BY id, name LIMIT 500`)
	if err != nil {
		t.Fatal(err)
	}
	issues := CheckPolicy(q, Policy{
		AllowedOps:     []Operation{OperationRead},
		MaxLimit:       100,
		MaxSelectItems: 2,
		MaxGroupFields: 2,
		MaxOrderFields: 1,
		MaxInValues:    2,
	})
	assertPolicyIssueContains(t, issues, "LIMIT exceeds maximum 100")
	assertPolicyIssueContains(t, issues, "SELECT item count 3 exceeds maximum 2")
	assertPolicyIssueContains(t, issues, "GROUP BY field count 3 exceeds maximum 2")
	assertPolicyIssueContains(t, issues, "ORDER BY field count 2 exceeds maximum 1")
	assertPolicyIssueContains(t, issues, "IN value count 3 exceeds maximum 2")
}

func TestCheckPolicyRejectsSubqueryDepth(t *testing.T) {
	q, err := Parse(`SELECT id FROM (SELECT id FROM (SELECT id FROM roadmap_items LIMIT 10) AS a LIMIT 10) AS b LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	issues := CheckPolicy(q, Policy{
		AllowNestedSources: true,
		MaxSubqueryDepth:   1,
	})
	assertPolicyIssueContains(t, issues, "subquery depth 2 exceeds maximum 1")
}

func TestCheckAnalysisPolicyRejectsHiddenFieldInference(t *testing.T) {
	q, err := Parse(`SELECT id FROM roadmap_items WHERE internal = "secret" LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := Analyze(q, testSchema())
	if err != nil {
		t.Fatal(err)
	}
	issues := CheckAnalysisPolicy(analysis, SafeAnalyticsPolicy(testSchema()))
	assertPolicyIssueContains(t, issues, `field "roadmap_items"."internal" is not allowed for filter by policy`)
}

func TestCheckAnalysisPolicyEnforcesJoinableFieldUsage(t *testing.T) {
	q, err := Parse(`SELECT parent.id FROM roadmap_items AS parent JOIN roadmap_items AS child ON parent.status = "planned" LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := Analyze(q, testSchema())
	if err != nil {
		t.Fatal(err)
	}
	policy := SafeAnalyticsPolicy(testSchema())
	policy.AllowedJoinTypes = []JoinType{JoinInner}
	issues := CheckAnalysisPolicy(analysis, policy)
	assertPolicyIssueContains(t, issues, `field "roadmap_items"."status" is not allowed for join by policy`)
}

func TestSafeAnalyticsPolicyCanAllowSpecificJoinField(t *testing.T) {
	q, err := Parse(`SELECT parent.id FROM roadmap_items AS parent JOIN roadmap_items AS child ON parent.status = "planned" LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := Analyze(q, testSchema())
	if err != nil {
		t.Fatal(err)
	}
	policy := SafeAnalyticsPolicy(testSchema())
	policy.AllowedJoinTypes = []JoinType{JoinInner}
	policy.Fields["roadmap_items"]["status"] = FieldPolicy{
		Selectable: true,
		Filterable: true,
		Sortable:   true,
		Groupable:  true,
		Joinable:   true,
	}
	if issues := CheckAnalysisPolicy(analysis, policy); len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}

func assertPolicyIssueContains(t *testing.T, issues []PolicyIssue, want string) {
	t.Helper()
	for _, issue := range issues {
		if strings.Contains(issue.Message, want) {
			return
		}
	}
	t.Fatalf("missing policy issue containing %q in %#v", want, issues)
}
