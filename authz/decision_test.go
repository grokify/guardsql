package authz

import (
	"testing"

	"github.com/grokify/guardsql"
)

func salariesConfig(p guardsql.Policy) guardsql.Config {
	return guardsql.Config{
		Schema: guardsql.Schema{Entities: map[string]guardsql.Entity{
			"salaries": {Fields: map[string]guardsql.Field{
				"salary": {Type: guardsql.FieldNumber, Selectable: true},
			}},
		}},
		Policy: p,
	}
}

// A decision that allows nothing must deny, not leave the query unrestricted.
func TestPolicyFromEmptyDecisionDenies(t *testing.T) {
	p := PolicyFromDecision(Decision{})
	if !p.StrictAllowlists {
		t.Fatal("PolicyFromDecision did not set StrictAllowlists")
	}
	if _, err := salariesConfig(p).Check(`SELECT salary FROM salaries`); err == nil {
		t.Fatal("empty decision allowed SELECT salary FROM salaries")
	}
}

func TestPolicyFromDecisionAllowsListed(t *testing.T) {
	p := PolicyFromDecision(Decision{
		AllowedEntities: []string{"salaries"},
		Fields:          map[string]map[string]guardsql.FieldPolicy{"salaries": {"salary": {Selectable: true}}},
	})
	if _, err := salariesConfig(p).Check(`SELECT salary FROM salaries`); err != nil {
		t.Fatalf("allowed query refused: %v", err)
	}
	// An allowed entity whose fields were not granted exposes none.
	p = PolicyFromDecision(Decision{AllowedEntities: []string{"salaries"}})
	if _, err := salariesConfig(p).Check(`SELECT salary FROM salaries`); err == nil {
		t.Fatal("entity without field grants exposed salary")
	}
}
