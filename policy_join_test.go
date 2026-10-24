package guardsql

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, input string) *Query {
	t.Helper()
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse(%q): %v", input, err)
	}
	return q
}

func hasIssue(issues []PolicyIssue, substr string) bool {
	for _, i := range issues {
		if strings.Contains(i.Message, substr) {
			return true
		}
	}
	return false
}

// A joined entity is checked against AllowedEntities like the FROM entity.
func TestCheckPolicyChecksJoinedEntities(t *testing.T) {
	q := mustParse(t, `SELECT a.id FROM roadmap_items AS a JOIN salaries AS s ON a.id = s.id LIMIT 5`)
	p := Policy{AllowedEntities: []string{"roadmap_items"}}
	if issues := CheckPolicy(q, p); !hasIssue(issues, `entity "salaries" is not allowed`) {
		t.Errorf("join to an unlisted entity allowed: %v", issues)
	}
	p.AllowedEntities = append(p.AllowedEntities, "salaries")
	if issues := CheckPolicy(q, p); hasIssue(issues, "is not allowed") {
		t.Errorf("join to a listed entity refused: %v", issues)
	}
}
