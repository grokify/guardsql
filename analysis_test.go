package grokifyql

import (
	"strings"
	"testing"
)

func TestAnalyzeRequirementsForSingleSourceQuery(t *testing.T) {
	q, err := Parse(`SELECT id, name FROM roadmap_items WHERE status = "planned" ORDER BY score LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := Analyze(q, testSchema())
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Sources) != 1 || analysis.Sources[0].Entity != "roadmap_items" {
		t.Fatalf("sources = %#v, want roadmap_items", analysis.Sources)
	}
	reqs := analysis.Requirements()
	if len(reqs.Operations) != 1 || reqs.Operations[0] != OperationRead {
		t.Fatalf("operations = %#v, want read", reqs.Operations)
	}
	if !hasEntity(reqs.Entities, "roadmap_items") {
		t.Fatalf("entities = %#v, want roadmap_items", reqs.Entities)
	}
	assertFieldUsage(t, reqs.Fields, "roadmap_items", "id", FieldUsageSelect)
	assertFieldUsage(t, reqs.Fields, "roadmap_items", "name", FieldUsageSelect)
	assertFieldUsage(t, reqs.Fields, "roadmap_items", "status", FieldUsageFilter)
	assertFieldUsage(t, reqs.Fields, "roadmap_items", "score", FieldUsageSort)
}

func TestAnalyzeAggregatesHavingAndGroupBy(t *testing.T) {
	q, err := Parse(`SELECT status, COUNT(*) AS count, SUM(score) AS total FROM roadmap_items GROUP BY status HAVING total > 10 ORDER BY total DESC LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := Analyze(q, testSchema())
	if err != nil {
		t.Fatal(err)
	}
	if !analysis.HasAggregate {
		t.Fatal("HasAggregate = false, want true")
	}
	reqs := analysis.Requirements()
	if !hasFunc(reqs.Functions, AggCount) || !hasFunc(reqs.Functions, AggSum) {
		t.Fatalf("functions = %#v, want COUNT and SUM", reqs.Functions)
	}
	assertFieldUsage(t, reqs.Fields, "roadmap_items", "status", FieldUsageGroup)
	assertAggregateUsage(t, reqs.Fields, "roadmap_items", "score", AggSum, "total")
	assertFieldUsage(t, reqs.Fields, "output", "total", FieldUsageHaving)
	assertFieldUsage(t, reqs.Fields, "output", "total", FieldUsageSort)
}

func TestAnalyzeJoinAliasesAndRequirements(t *testing.T) {
	q, err := Parse(`SELECT parent.id FROM roadmap_items AS parent JOIN roadmap_items AS child ON parent.status = "planned" WHERE child.score > 10 LIMIT 5`)
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := Analyze(q, testSchema())
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Sources) != 2 {
		t.Fatalf("sources = %#v, want 2", analysis.Sources)
	}
	if len(analysis.Joins) != 1 || analysis.Joins[0].Type != JoinInner {
		t.Fatalf("joins = %#v, want inner join", analysis.Joins)
	}
	reqs := analysis.Requirements()
	assertFieldUsage(t, reqs.Fields, "roadmap_items", "id", FieldUsageSelect)
	assertFieldUsage(t, reqs.Fields, "roadmap_items", "status", FieldUsageJoin)
	assertFieldUsage(t, reqs.Fields, "roadmap_items", "score", FieldUsageFilter)
}

func TestAnalyzeRejectsAmbiguousUnqualifiedJoinField(t *testing.T) {
	q, err := Parse(`SELECT id FROM roadmap_items AS parent JOIN roadmap_items AS child ON parent.status = "planned" LIMIT 5`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Analyze(q, testSchema())
	if err == nil || !strings.Contains(err.Error(), "ambiguous field") {
		t.Fatalf("err = %v, want ambiguous field", err)
	}
}

func TestAnalyzeTracksCTEAndNestedSources(t *testing.T) {
	q, err := Parse(`WITH active AS (SELECT id, status FROM roadmap_items WHERE status = "active" LIMIT 10) SELECT active.id FROM (SELECT id, status FROM active LIMIT 5) AS active LIMIT 5`)
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := Analyze(q, testSchema())
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.CTEs) != 1 {
		t.Fatalf("CTEs = %#v, want 1", analysis.CTEs)
	}
	if len(analysis.NestedSources) != 1 {
		t.Fatalf("NestedSources = %#v, want 1", analysis.NestedSources)
	}
	if len(analysis.Sources) != 1 || !analysis.Sources[0].Nested || analysis.Sources[0].Alias != "active" {
		t.Fatalf("sources = %#v, want nested active source", analysis.Sources)
	}
	reqs := analysis.Requirements()
	assertFieldUsage(t, reqs.Fields, "roadmap_items", "status", FieldUsageFilter)
	assertFieldUsage(t, reqs.Fields, "active", "id", FieldUsageSelect)
}

func assertFieldUsage(t *testing.T, fields []FieldRequirement, entity string, field string, usage FieldUsage) {
	t.Helper()
	for _, req := range fields {
		if req.Entity == entity && req.Field == field && req.Usage == usage {
			return
		}
	}
	t.Fatalf("missing field usage entity=%q field=%q usage=%q in %#v", entity, field, usage, fields)
}

func assertAggregateUsage(t *testing.T, fields []FieldRequirement, entity string, field string, agg AggregateFunc, output string) {
	t.Helper()
	for _, req := range fields {
		if req.Entity == entity && req.Field == field && req.Usage == FieldUsageAggregate && req.Aggregate == agg && req.OutputName == output {
			return
		}
	}
	t.Fatalf("missing aggregate entity=%q field=%q agg=%q output=%q in %#v", entity, field, agg, output, fields)
}

func hasEntity(entities []string, want string) bool {
	for _, entity := range entities {
		if entity == want {
			return true
		}
	}
	return false
}

func hasFunc(funcs []AggregateFunc, want AggregateFunc) bool {
	for _, fn := range funcs {
		if fn == want {
			return true
		}
	}
	return false
}
