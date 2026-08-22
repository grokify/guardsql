package grokifyql

import "testing"

func testSchema() Schema {
	return Schema{
		MaxLimit: 100,
		Entities: map[string]Entity{
			"roadmap_items": {
				Fields: map[string]Field{
					"id":       {Type: FieldString, Selectable: true, Filterable: true, Sortable: true},
					"name":     {Type: FieldString, Selectable: true, Filterable: true, Sortable: true},
					"status":   {Type: FieldString, Selectable: true, Filterable: true, Sortable: true},
					"score":    {Type: FieldNumber, Selectable: true, Filterable: true, Sortable: true},
					"internal": {Type: FieldString},
				},
			},
		},
	}
}

func TestParseValidateEval(t *testing.T) {
	q, err := Parse(`SELECT id, name FROM roadmap_items WHERE status IN ("planned", "active") AND score >= 50 ORDER BY score DESC LIMIT 2`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(q, testSchema()); err != nil {
		t.Fatal(err)
	}
	if q.Operation != OperationRead {
		t.Fatalf("got operation %q, want %q", q.Operation, OperationRead)
	}
	rows, err := Eval(q, []Row{
		{"id": "a", "name": "Alpha", "status": "planned", "score": 80},
		{"id": "b", "name": "Beta", "status": "done", "score": 90},
		{"id": "c", "name": "Gamma", "status": "active", "score": 60},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0]["id"] != "a" || rows[1]["id"] != "c" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	if _, ok := rows[0]["score"]; ok {
		t.Fatalf("projection leaked score: %#v", rows[0])
	}
}

func TestValidateRejectsUnknownAndPrivateFields(t *testing.T) {
	q, err := Parse(`SELECT internal FROM roadmap_items WHERE missing = "x"`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(q, testSchema()); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidateAppliesMaxLimit(t *testing.T) {
	q, err := Parse(`FROM roadmap_items WHERE name CONTAINS "roadmap"`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(q, testSchema()); err != nil {
		t.Fatal(err)
	}
	if q.Limit != 100 {
		t.Fatalf("got limit %d, want 100", q.Limit)
	}
}

func TestLintRejectsPolicyViolations(t *testing.T) {
	_, issues := Lint(`FROM roadmap_items WHERE status IN ("a", "b", "c")`, LintConfig{
		Schema:       testSchema(),
		AllowedOps:   []Operation{OperationRead},
		MaxInValues:  2,
		RequireLimit: true,
	})
	if len(issues) != 2 {
		t.Fatalf("got %d issues, want 2: %#v", len(issues), issues)
	}
}

func TestLintRejectsSQLInjectionShape(t *testing.T) {
	_, issues := Lint(`FROM roadmap_items WHERE name = "x"; DROP TABLE users`, LintConfig{
		Schema: testSchema(),
	})
	if len(issues) == 0 {
		t.Fatal("expected lint issue")
	}
}

func TestLintRejectsDisallowedOperation(t *testing.T) {
	q := &Query{Operation: OperationDelete, From: "roadmap_items", Limit: 1}
	if err := Validate(q, testSchema()); err != nil {
		t.Fatal(err)
	}
	_, issues := Lint(`FROM roadmap_items LIMIT 1`, LintConfig{
		Schema:     testSchema(),
		AllowedOps: []Operation{OperationCreate},
	})
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1: %#v", len(issues), issues)
	}
}

func TestCheckPolicyAllowsScopedQuery(t *testing.T) {
	q, err := Parse(`SELECT id, name FROM roadmap_items WHERE status = "planned" ORDER BY name LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	issues := CheckPolicy(q, Policy{
		AllowedOps:      []Operation{OperationRead},
		AllowedEntities: []string{"roadmap_items"},
		Fields: map[string]map[string]FieldPolicy{
			"roadmap_items": {
				"id":     {Selectable: true},
				"name":   {Selectable: true, Sortable: true},
				"status": {Filterable: true},
			},
		},
		RequireLimit: true,
		MaxLimit:     100,
		MaxDepth:     4,
		MaxNodes:     8,
		MaxInValues:  10,
	})
	if len(issues) != 0 {
		t.Fatalf("got policy issues, want none: %#v", issues)
	}
}

func TestCheckPolicyRejectsEntityAndFieldViolations(t *testing.T) {
	q, err := Parse(`SELECT id, internal FROM roadmap_items WHERE status = "planned" ORDER BY score LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	issues := CheckPolicy(q, Policy{
		AllowedOps:      []Operation{OperationRead},
		AllowedEntities: []string{"initiatives"},
		Fields: map[string]map[string]FieldPolicy{
			"roadmap_items": {
				"id":     {Selectable: true},
				"status": {Filterable: true},
				"score":  {Selectable: true},
			},
		},
	})
	if len(issues) != 3 {
		t.Fatalf("got %d issues, want 3: %#v", len(issues), issues)
	}
}

func TestCheckPolicyRejectsStarWhenFieldPolicyConfigured(t *testing.T) {
	q, err := Parse(`SELECT * FROM roadmap_items LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	issues := CheckPolicy(q, Policy{
		Fields: map[string]map[string]FieldPolicy{
			"roadmap_items": {
				"id": {Selectable: true},
			},
		},
	})
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1: %#v", len(issues), issues)
	}
}

func TestCheckPolicyLimitRules(t *testing.T) {
	q, err := Parse(`FROM roadmap_items LIMIT 500`)
	if err != nil {
		t.Fatal(err)
	}
	issues := CheckPolicy(q, Policy{RequireLimit: true, MaxLimit: 100})
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1: %#v", len(issues), issues)
	}
}

func TestParsePrioritizeBy(t *testing.T) {
	q, err := Parse(`FROM roadmap_items WHERE status = "planned" PRIORITIZE BY moscow_rice LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Prioritize) != 1 || q.Prioritize[0] != "moscow_rice" {
		t.Fatalf("prioritize = %#v, want moscow_rice", q.Prioritize)
	}
}

func TestGroupByAggregatesHavingEval(t *testing.T) {
	q, err := Parse(`SELECT status, COUNT(*) AS count, SUM(score) AS total, AVG(score) AS avg_score FROM roadmap_items GROUP BY status HAVING count > 1 ORDER BY total DESC LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(q, testSchema()); err != nil {
		t.Fatal(err)
	}
	rows, err := Eval(q, []Row{
		{"id": "a", "status": "planned", "score": 80},
		{"id": "b", "status": "planned", "score": 20},
		{"id": "c", "status": "active", "score": 70},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1: %#v", len(rows), rows)
	}
	if rows[0]["status"] != "planned" || rows[0]["count"] != 2 || rows[0]["total"] != 100.0 {
		t.Fatalf("unexpected aggregate row: %#v", rows[0])
	}
}

func TestAggregateDefaultOutputName(t *testing.T) {
	q, err := Parse(`SELECT status, COUNT(*), MAX(score) FROM roadmap_items GROUP BY status`)
	if err != nil {
		t.Fatal(err)
	}
	if got := OutputName(q.Select[1]); got != "count_all" {
		t.Fatalf("got %q, want count_all", got)
	}
	if got := OutputName(q.Select[2]); got != "max_score" {
		t.Fatalf("got %q, want max_score", got)
	}
}

func TestValidateRejectsUngroupedSelectField(t *testing.T) {
	q, err := Parse(`SELECT status, name, COUNT(*) AS count FROM roadmap_items GROUP BY status`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(q, testSchema()); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseCTEJoinAndNestedQueryAST(t *testing.T) {
	q, err := Parse(`WITH active AS (SELECT id, status FROM roadmap_items WHERE status = "active" LIMIT 10) SELECT active.id FROM (SELECT id, status FROM active LIMIT 5) AS active JOIN roadmap_items AS base ON active.id = base.id LIMIT 5`)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.With) != 1 || q.With[0].Name != "active" {
		t.Fatalf("unexpected CTEs: %#v", q.With)
	}
	if q.FromQuery == nil || q.FromAlias != "active" {
		t.Fatalf("unexpected nested source: %#v alias=%q", q.FromQuery, q.FromAlias)
	}
	if len(q.Joins) != 1 || q.Joins[0].Entity != "roadmap_items" || q.Joins[0].Alias != "base" {
		t.Fatalf("unexpected joins: %#v", q.Joins)
	}
}

func TestParseJoinTypesAndAliases(t *testing.T) {
	cases := []struct {
		query string
		want  JoinType
	}{
		{`SELECT a.id FROM roadmap_items AS a JOIN roadmap_items AS b ON a.id = b.id LIMIT 5`, JoinInner},
		{`SELECT a.id FROM roadmap_items a INNER JOIN roadmap_items b ON a.id = b.id LIMIT 5`, JoinInner},
		{`SELECT a.id FROM roadmap_items a LEFT JOIN roadmap_items b ON a.id = b.id LIMIT 5`, JoinLeft},
		{`SELECT a.id FROM roadmap_items a LEFT OUTER JOIN roadmap_items b ON a.id = b.id LIMIT 5`, JoinLeft},
		{`SELECT a.id FROM roadmap_items a RIGHT JOIN roadmap_items b ON a.id = b.id LIMIT 5`, JoinRight},
		{`SELECT a.id FROM roadmap_items a RIGHT OUTER JOIN roadmap_items b ON a.id = b.id LIMIT 5`, JoinRight},
		{`SELECT a.id FROM roadmap_items a FULL JOIN roadmap_items b ON a.id = b.id LIMIT 5`, JoinFull},
		{`SELECT a.id FROM roadmap_items a FULL OUTER JOIN roadmap_items b ON a.id = b.id LIMIT 5`, JoinFull},
		{`SELECT a.id FROM roadmap_items a CROSS JOIN roadmap_items b LIMIT 5`, JoinCross},
	}
	for _, tc := range cases {
		q, err := Parse(tc.query)
		if err != nil {
			t.Fatalf("%s: %v", tc.query, err)
		}
		if q.FromAlias != "a" {
			t.Fatalf("%s: from alias = %q, want a", tc.query, q.FromAlias)
		}
		if len(q.Joins) != 1 || q.Joins[0].Type != tc.want || q.Joins[0].Alias != "b" {
			t.Fatalf("%s: join = %#v, want type %q alias b", tc.query, q.Joins, tc.want)
		}
		if tc.want == JoinCross && q.Joins[0].Condition != nil {
			t.Fatalf("%s: cross join condition = %#v, want nil", tc.query, q.Joins[0].Condition)
		}
	}
}

func TestParseRejectsASWithoutAlias(t *testing.T) {
	cases := []string{
		`SELECT id FROM roadmap_items AS WHERE status = "planned"`,
		`SELECT id FROM (SELECT id FROM roadmap_items LIMIT 5) AS LIMIT 5`,
		`SELECT a.id FROM roadmap_items AS a JOIN roadmap_items AS ON a.id = b.id LIMIT 5`,
	}
	for _, input := range cases {
		if _, err := Parse(input); err == nil {
			t.Fatalf("%q parsed successfully, want error", input)
		}
	}
}

func TestValidateSelfJoinRequiresAliases(t *testing.T) {
	q, err := Parse(`SELECT id FROM roadmap_items JOIN roadmap_items AS child ON id = child.id LIMIT 5`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(q, testSchema()); err == nil {
		t.Fatal("expected self join alias validation error")
	}
}

func TestValidateSelfJoinWithAliases(t *testing.T) {
	q, err := Parse(`SELECT parent.id FROM roadmap_items AS parent JOIN roadmap_items AS child ON parent.id = child.id LIMIT 5`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(q, testSchema()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsDuplicateSourceAlias(t *testing.T) {
	q, err := Parse(`SELECT a.id FROM roadmap_items AS a JOIN roadmap_items AS a ON a.id = a.id LIMIT 5`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(q, testSchema()); err == nil {
		t.Fatal("expected duplicate alias validation error")
	}
}

func TestValidateNestedSourcesRequireAliases(t *testing.T) {
	q, err := Parse(`SELECT id FROM (SELECT id FROM roadmap_items LIMIT 5) LIMIT 5`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(q, testSchema()); err == nil {
		t.Fatal("expected nested source alias validation error")
	}
}
