package service

import (
	"testing"

	"ragflow/internal/entity"
)

// The assistant's binding sits between the request and the template: it is the
// layer the person using the assistant controls, so it has to be read before the
// deployment default. A template that turns the knowledge layer on for every user
// of that agent must not override the assistant whose owner turned it off.
func TestOntologyBindingPrefersAssistantOverTemplate(t *testing.T) {
	promptConfig := entity.JSONMap{"ontology": map[string]interface{}{
		"model_id":              "guild",
		"knowledge_dataset_ids": []string{"kb-1", "kb-1"},
	}}
	got := OntologyBindingFor(nil, promptConfig, "unknown-template")
	if got == nil {
		t.Fatal("the assistant's binding was not resolved")
	}
	if got.ModelID != "guild" {
		t.Fatalf("model = %q, want the assistant's", got.ModelID)
	}
	if len(got.KnowledgeDatasetIDs) != 1 || got.KnowledgeDatasetIDs[0] != "kb-1" {
		t.Fatalf("datasets = %v, want the assistant's scope deduped", got.KnowledgeDatasetIDs)
	}
}

// kwargs stay the most specific layer: a request that names its own binding
// answers one question from one scope without changing what the assistant reads
// on the next turn.
func TestOntologyBindingPrefersRequestOverAssistant(t *testing.T) {
	kwargs := map[string]interface{}{"ontology": map[string]interface{}{
		"model_id":              "request-model",
		"knowledge_dataset_ids": []string{"kb-request"},
	}}
	promptConfig := entity.JSONMap{"ontology": map[string]interface{}{
		"model_id":              "assistant-model",
		"knowledge_dataset_ids": []string{"kb-assistant"},
	}}
	got := OntologyBindingFor(kwargs, promptConfig, "unknown-template")
	if got == nil || got.ModelID != "request-model" {
		t.Fatalf("binding = %#v, want the request's model", got)
	}
	if len(got.KnowledgeDatasetIDs) != 1 || got.KnowledgeDatasetIDs[0] != "kb-request" {
		t.Fatalf("datasets = %v, want the request's scope", got.KnowledgeDatasetIDs)
	}
}

// No binding anywhere stays no binding: an unconfigured turn must not acquire a
// knowledge scope from the lookup itself.
func TestOntologyBindingAbsentWhenNothingDeclaresIt(t *testing.T) {
	if got := OntologyBindingFor(nil, nil, "unknown-template"); got != nil {
		t.Fatalf("binding = %#v, want none", got)
	}
	// A partial payload is reported, not completed from a lower layer: the
	// assistant names a model but no scope, so nothing can be read.
	partial := entity.JSONMap{"ontology": map[string]interface{}{"model_id": "guild"}}
	if got := OntologyBindingFor(nil, partial, "unknown-template"); got != nil {
		t.Fatalf("binding = %#v, want the half-configured payload refused", got)
	}
}
