package guardsql

import "testing"

// A zero Policy adds no restriction; the same empty allowlists under
// StrictAllowlists allow nothing.
func TestStrictAllowlistsEmptyDeniesEverything(t *testing.T) {
	q := mustParse(t, `SELECT id FROM roadmap_items LIMIT 5`)
	if issues := CheckPolicy(q, Policy{}); len(issues) != 0 {
		t.Fatalf("zero policy: %v", issues)
	}
	issues := CheckPolicy(q, Policy{StrictAllowlists: true})
	if !hasIssue(issues, `entity "roadmap_items" is not allowed`) {
		t.Errorf("strict empty policy allowed the entity: %v", issues)
	}
	if !hasIssue(issues, `field "id" is not selectable`) {
		t.Errorf("strict empty policy allowed the field: %v", issues)
	}
}

func TestStrictAllowlistsEntityWithoutFields(t *testing.T) {
	q := mustParse(t, `SELECT id FROM roadmap_items WHERE status = "x" LIMIT 5`)
	p := Policy{StrictAllowlists: true, AllowedEntities: []string{"roadmap_items"}}
	issues := CheckPolicy(q, p)
	if !hasIssue(issues, `field "id" is not selectable`) || !hasIssue(issues, `field "status" is not filterable`) {
		t.Errorf("entity without a Fields entry exposed fields: %v", issues)
	}
	p.Fields = map[string]map[string]FieldPolicy{"roadmap_items": {
		"id": {Selectable: true}, "status": {Filterable: true},
	}}
	if issues := CheckPolicy(q, p); len(issues) != 0 {
		t.Errorf("allowed query refused: %v", issues)
	}
}

func TestStrictAllowlistsAnalysis(t *testing.T) {
	q := mustParse(t, `SELECT id FROM roadmap_items LIMIT 5`)
	analysis, err := Analyze(q, testSchema())
	if err != nil {
		t.Fatal(err)
	}
	if issues := CheckAnalysisPolicy(analysis, Policy{}); len(issues) != 0 {
		t.Fatalf("zero policy: %v", issues)
	}
	issues := CheckAnalysisPolicy(analysis, Policy{StrictAllowlists: true})
	if !hasIssue(issues, `entity "roadmap_items" is not allowed`) || !hasIssue(issues, `"roadmap_items"."id" is not allowed`) {
		t.Errorf("strict empty policy allowed the query: %v", issues)
	}
}
