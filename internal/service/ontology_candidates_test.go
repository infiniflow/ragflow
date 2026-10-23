package service

import "testing"

// ontologyConfigForTerms builds a declared model from plain names, which is all
// the candidate matcher reads.
func ontologyConfigForTerms(classes, properties []string) declaredOntology {
	entityFields := make([]map[string]interface{}, 0, len(classes))
	for _, name := range classes {
		entityFields = append(entityFields, map[string]interface{}{"type": name})
	}
	relationFields := make([]map[string]interface{}, 0, len(properties))
	for _, name := range properties {
		relationFields = append(relationFields, map[string]interface{}{"type": name, "kind": "object"})
	}
	return parseDeclaredOntology(ontologyTestConfig(entityFields, relationFields))
}

// Questions are plural far more often than declarations are: "which works…" has
// to name the declared class `work`, or the most common shape of a knowledge
// question arrives with no constraint at all.
func TestOntologyQueryCandidatesToleratesGrammaticalNumber(t *testing.T) {
	declared := ontologyConfigForTerms([]string{"work", "company"}, nil)

	for question, want := range map[string]string{
		"which works were written by james joyce": "work",
		"which companies were founded in dublin":  "company",
		"which work is the longest":               "work",
	} {
		got := OntologyQueryCandidates(question, declared, 6, 6)
		if len(got.EntityTypes) != 1 || got.EntityTypes[0] != want {
			t.Fatalf("question %q -> %#v, want %q", question, got.EntityTypes, want)
		}
	}
}

// A declared name written with separators is matched the way a question spells
// it, in either direction: `written_by` against "written by", and camelCase
// against the same two words.
func TestOntologyQueryCandidatesMatchesNameSpellings(t *testing.T) {
	declared := ontologyConfigForTerms(nil, []string{"written_by", "placeOfBirth"})

	for question, want := range map[string]string{
		"which works were written by joyce": "written_by",
		"where was the place of birth":      "placeOfBirth",
	} {
		got := OntologyQueryCandidates(question, declared, 6, 6)
		if len(got.Properties) != 1 || got.Properties[0] != want {
			t.Fatalf("question %q -> %#v, want %q", question, got.Properties, want)
		}
	}
}

// The candidate cap is applied on match strength, not on declaration order: a
// model that lists thirty classes must not spend its six slots on short generic
// names that happened to appear in the question.
func TestOntologyQueryCandidatesKeepsTheStrongestMatchUnderTheCap(t *testing.T) {
	declared := ontologyConfigForTerms([]string{"ship", "person"}, nil)

	got := OntologyQueryCandidates("the person on the ship", declared, 1, 0)
	if len(got.EntityTypes) != 1 || got.EntityTypes[0] != "person" {
		t.Fatalf("candidates = %#v, want the longer matched term under a cap of one", got.EntityTypes)
	}
	if got := OntologyQueryCandidates("the person on the ship", declared, 0, 0); len(got.EntityTypes) != 2 {
		t.Fatalf("candidates = %#v, want both terms when nothing is capped", got.EntityTypes)
	}
}

// Short Latin words are the failure mode that makes a lexical match worse than no
// match: "id" is in most questions and means nothing structural.
func TestOntologyQueryCandidatesIgnoresShortAndPartialWords(t *testing.T) {
	declared := ontologyConfigForTerms([]string{"id", "art"}, nil)

	// "art" appears inside "article" but not as a word of its own.
	if got := OntologyQueryCandidates("read the article about identity", declared, 6, 6); len(got.EntityTypes) != 0 {
		t.Fatalf("candidates = %#v, want none", got.EntityTypes)
	}
	if got := OntologyQueryCandidates("what art is this", declared, 6, 6); len(got.EntityTypes) != 1 || got.EntityTypes[0] != "art" {
		t.Fatalf("candidates = %#v, want [art]", got.EntityTypes)
	}
}
