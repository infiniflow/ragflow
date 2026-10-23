package agentic_rag

import "testing"

// A plain prefix let one entity's attribute list fill the whole instruction
// budget. The selection takes one assertion per subject in turn, so the rows the
// model sees are spread over the subjects the search found.
func TestSelectOntologyAssertionsSpreadsOverSubjects(t *testing.T) {
	assertions := make([]OntologyAssertion, 0, 18)
	for i := 0; i < 15; i++ {
		assertions = append(assertions, OntologyAssertion{Kind: "relation", From: "ada", To: "guild", Name: "leads"})
	}
	assertions = append(assertions,
		OntologyAssertion{Kind: "relation", From: "bob", To: "guild", Name: "leads"},
		OntologyAssertion{Kind: "entity", Name: "guild"},
	)

	shown := selectOntologyAssertions(assertions, 12)
	if len(shown) != 12 {
		t.Fatalf("shown = %d, want the budget", len(shown))
	}
	var ada, bob, guild int
	for _, assertion := range shown {
		switch {
		case assertion.From == "ada":
			ada++
		case assertion.From == "bob":
			bob++
		case assertion.Name == "guild":
			guild++
		}
	}
	if bob != 1 || guild != 1 {
		t.Fatalf("shown = ada:%d bob:%d guild:%d, want the rarer subjects included", ada, bob, guild)
	}
	if ada > 10 {
		t.Fatalf("ada rows = %d, want the budget shared rather than filled by one subject", ada)
	}
	// Reproducible: the same page selects the same rows.
	again := selectOntologyAssertions(assertions, 12)
	for i := range shown {
		if shown[i].From != again[i].From || shown[i].Name != again[i].Name {
			t.Fatalf("selection is not deterministic at %d", i)
		}
	}
}

// A page that already fits the budget is shown as is: no reordering, no copying.
func TestSelectOntologyAssertionsKeepsPagesWithinBudget(t *testing.T) {
	assertions := []OntologyAssertion{
		{Kind: "relation", From: "ada"},
		{Kind: "entity", Name: "guild"},
	}
	shown := selectOntologyAssertions(assertions, 12)
	if len(shown) != 2 || shown[0].From != "ada" {
		t.Fatalf("shown = %#v, want the page unchanged", shown)
	}
}
