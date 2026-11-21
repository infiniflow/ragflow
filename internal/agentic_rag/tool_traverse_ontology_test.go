package agentic_rag

import (
	"strings"
	"testing"
)

// A walk has to carry its own accounting: an empty path list has at least three
// causes, and a model reading only the paths cannot tell "the entities are not in
// the scope" from "the declaration refused every row".
func TestFormatOntologyTraversalCarriesHopAccounting(t *testing.T) {
	out := formatOntologyTraversal(&OntologyTraversal{
		Model: OntologyModelRef{ID: "tpl-1", Name: "Papers", Version: 3},
		Hops: []OntologyTraverseHop{
			{Hop: 1, Frontier: 1, Rows: 2, Matched: 1, RejectedByDeclaration: 1},
			{Hop: 2, Frontier: 1, Rows: 1, Matched: 1},
		},
		Paths: []OntologyTraversePath{{
			Start: "Paper A",
			Nodes: []string{"Paper A", "Model M", "Variable V"},
			Steps: []OntologyTraverseStep{
				{From: "Paper A", Property: "uses", To: "Model M", FromType: "paper", ToType: "model", Direction: "forward"},
				{From: "Model M", Property: "has_variable", To: "Variable V", ToType: "variable", Direction: "forward"},
			},
		}},
	})

	for _, want := range []string{
		`model_id="tpl-1"`,
		`model_version="3"`,
		`hops="2"`,
		`paths="1"`,
		`frontier="1"`,
		`rejected_by_declaration="1"`,
		`nodes="Paper A &gt; Model M &gt; Variable V"`,
		`property="has_variable"`,
		`direction="forward"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestFormatOntologyTraversalExplainsAnEmptyWalk(t *testing.T) {
	out := formatOntologyTraversal(&OntologyTraversal{
		Model: OntologyModelRef{ID: "tpl-1"},
		Hops:  []OntologyTraverseHop{{Hop: 1, Frontier: 1, Rows: 0}},
	})
	if !strings.Contains(out, "no path was found") {
		t.Fatalf("output = %s, want the empty walk explained", out)
	}
	if !strings.Contains(out, "no rows read") {
		t.Fatalf("output = %s, want the two failure causes named", out)
	}
	if strings.Contains(out, "hyperedge") {
		t.Fatalf("output = %s, want a walk result, not a block result", out)
	}
}

func TestFormatOntologyTraversalHandlesNil(t *testing.T) {
	out := formatOntologyTraversal(nil)
	if !strings.Contains(out, `degraded="true"`) {
		t.Fatalf("output = %s, want a degraded result", out)
	}
}

// The degrade travels with the walk: a walk without a model still returns paths,
// and the caller has to know the pruning was absent.
func TestFormatOntologyTraversalReportsDegrade(t *testing.T) {
	out := formatOntologyTraversal(&OntologyTraversal{
		DegradeCode:   "no_model",
		DegradeReason: "model not found",
		Paths: []OntologyTraversePath{{
			Start: "Paper A",
			Nodes: []string{"Paper A", "Model M"},
			Steps: []OntologyTraverseStep{{From: "Paper A", Property: "mentions", To: "Model M"}},
		}},
	})
	if !strings.Contains(out, `degrade_code="no_model"`) || !strings.Contains(out, "model not found") {
		t.Fatalf("output = %s, want the degrade reported", out)
	}
	if !strings.Contains(out, `chunks=`) && !strings.Contains(out, `<step`) {
		t.Fatalf("output = %s, want the path still rendered", out)
	}
}
