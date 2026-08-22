package grokifyql

import (
	"strings"
	"testing"
)

func TestFormatMultiline(t *testing.T) {
	got, err := Format(`select name,count(*) as count from initiatives where provider="aha-studio" and workspace_ref="SAVIN" group by name having count > 1 prioritize by moscow_rice order by count desc limit 10`, DefaultFormatOptions())
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}
	want := strings.Join([]string{
		"SELECT name, COUNT(*) AS count",
		"FROM initiatives",
		`WHERE provider = "aha-studio" AND workspace_ref = "SAVIN"`,
		"GROUP BY name",
		"HAVING count > 1",
		"PRIORITIZE BY moscow_rice",
		"ORDER BY count DESC",
		"LIMIT 10",
	}, "\n")
	if got != want {
		t.Fatalf("Format() mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestFormatSingleLine(t *testing.T) {
	got, err := Format(`SELECT id,name FROM roadmap_items WHERE status IN ("planned","active") LIMIT 25`, FormatOptions{Style: FormatSingleLine})
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}
	want := `SELECT id, name FROM roadmap_items WHERE status IN ("planned", "active") LIMIT 25`
	if got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}
}

func TestFormatJoinsAndNestedSources(t *testing.T) {
	input := `select parent.id,child.id from (select id,parent_id from roadmap_items limit 50) as child left join roadmap_items as parent on parent.id = child.parent_id limit 10`
	got, err := Format(input, DefaultFormatOptions())
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}
	want := strings.Join([]string{
		"SELECT parent.id, child.id",
		"FROM (",
		"\tSELECT id, parent_id",
		"\tFROM roadmap_items",
		"\tLIMIT 50",
		") AS child",
		"LEFT JOIN roadmap_items AS parent ON parent.id = \"child.parent_id\"",
		"LIMIT 10",
	}, "\n")
	if got != want {
		t.Fatalf("Format() mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestFormatCTE(t *testing.T) {
	got, err := Format(`with active as (select id,status from roadmap_items where status = "active" limit 100) select active.id from active limit 10`, DefaultFormatOptions())
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}
	want := strings.Join([]string{
		"WITH active AS (",
		"\tSELECT id, status",
		"\tFROM roadmap_items",
		"\tWHERE status = \"active\"",
		"\tLIMIT 100",
		")",
		"SELECT active.id",
		"FROM active",
		"LIMIT 10",
	}, "\n")
	if got != want {
		t.Fatalf("Format() mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestHighlightANSIText(t *testing.T) {
	got := HighlightANSIText(`SELECT name FROM items WHERE active = true`)
	if !strings.Contains(got, "\x1b[1;36mSELECT\x1b[0m") {
		t.Fatalf("expected highlighted SELECT, got %q", got)
	}
	if !strings.Contains(got, "\x1b[35mtrue\x1b[0m") {
		t.Fatalf("expected highlighted bool, got %q", got)
	}
}
