package agentic_rag

import (
	"context"
	"strings"
	"testing"
)

// The grade is what a model reads to decide how far it may lean on the rows
// below it. Every level has to be stated, including the one where nothing came
// back: silence would let the rows read as the model's own statement.
func TestOntologyDigestBlockStatesTheEvidenceGrade(t *testing.T) {
	cases := []struct {
		name    string
		digest  *OntologyDigest
		want    []string
		notWant []string
	}{
		{
			name: "structured",
			digest: &OntologyDigest{
				EvidenceLevel:    OntologyEvidenceStructured,
				StructureApplied: true,
				AppliedFilter:    map[string]any{"entity_type_kwd": []string{"agent", "person"}},
			},
			want: []string{"Evidence: structured", "entity_type_kwd in [agent, person]"},
		},
		{
			name:   "schema free with nothing declared matched",
			digest: &OntologyDigest{EvidenceLevel: OntologyEvidenceSchemaFree},
			want: []string{
				"Evidence: schema-free",
				"named nothing this model declares",
				"do not attribute them to the model",
			},
			notWant: []string{"Degraded"},
		},
		{
			name: "schema free after a dropped narrowing",
			digest: &OntologyDigest{
				EvidenceLevel: OntologyEvidenceSchemaFree,
				Degrade:       &OntologyDegrade{Code: OntologyDegradeNoMatch, Reason: "matched no assertion"},
			},
			want: []string{"Evidence: schema-free", OntologyDegradeNoMatch, "matched no assertion"},
			// The grade line already carries this reason; repeating it as a
			// degrade line would read as two separate faults.
			notWant: []string{"Degraded ("},
		},
		{
			name:   "none",
			digest: &OntologyDigest{EvidenceLevel: OntologyEvidenceNone},
			want:   []string{"Evidence: none"},
		},
		{
			name: "structured with a degrade the grade does not explain",
			digest: &OntologyDigest{
				EvidenceLevel:    OntologyEvidenceStructured,
				StructureApplied: true,
				Degrade:          &OntologyDegrade{Code: OntologyDegradeCoverageUnknown, Reason: "the scope could not be sampled"},
			},
			want: []string{
				"Evidence: structured",
				"Degraded (" + OntologyDegradeCoverageUnknown + ")",
				"the scope could not be sampled",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			block := ontologyDigestBlock(ontologyModeGuide, tc.digest, nil)
			for _, want := range tc.want {
				if !strings.Contains(block, want) {
					t.Errorf("block missing %q:\n%s", want, block)
				}
			}
			for _, notWant := range tc.notWant {
				if strings.Contains(block, notWant) {
					t.Errorf("block unexpectedly contains %q:\n%s", notWant, block)
				}
			}
		})
	}
}

// The tool's answer to the model publishes the grade in the same element as the
// count. The narrowing flag it replaces was one bit of it, and a model that read
// the bit would still have to guess whether the rows may be attributed.
func TestOntologyToolResultPublishesTheEvidenceGrade(t *testing.T) {
	out := formatOntologyResults(context.Background(), searchOntologyToolName, "who leads the guild",
		&OntologyDigest{EvidenceLevel: OntologyEvidenceSchemaFree, Total: 2})
	if !strings.Contains(out, `evidence="`+OntologyEvidenceSchemaFree+`"`) {
		t.Fatalf("result missing the grade: %s", out)
	}
	if strings.Contains(out, "narrowing=") {
		t.Fatalf("result still carries the narrowing flag beside the grade: %s", out)
	}
	if !strings.Contains(out, "coverage gap") {
		t.Fatalf("an empty page must still say it is a coverage gap: %s", out)
	}
}
