package service

import (
	"testing"

	"ragflow/internal/entity"
)

func TestValidateDatasetEmbeddingModels_AllHaveEmbeddingModel(t *testing.T) {
	kbs := []*entity.Knowledgebase{
		{EmbdID: "BAAI/bge-large-zh-v1.5@Builtin"},
		{EmbdID: "BAAI/bge-large-zh-v1.5@Builtin"},
	}
	if err := ValidateDatasetEmbeddingModels(kbs); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDatasetEmbeddingModels_NoneHasEmbeddingModel(t *testing.T) {
	kbs := []*entity.Knowledgebase{
		{EmbdID: ""},
		{EmbdID: ""},
	}
	if err := ValidateDatasetEmbeddingModels(kbs); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDatasetEmbeddingModels_MixedErrors(t *testing.T) {
	kbs := []*entity.Knowledgebase{
		{EmbdID: "BAAI/bge-large-zh-v1.5@Builtin"},
		{EmbdID: ""},
	}
	err := ValidateDatasetEmbeddingModels(kbs)
	if err == nil {
		t.Fatal("expected error for mixed embedding")
	}
	if err.Error() != "Cannot search across datasets where some have embedding models and others do not." {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDatasetEmbeddingModels_DifferentEmbeddingsErrors(t *testing.T) {
	kbs := []*entity.Knowledgebase{
		{EmbdID: "model-a@provider-1"},
		{EmbdID: "model-b@provider-2"},
	}
	err := ValidateDatasetEmbeddingModels(kbs)
	if err == nil {
		t.Fatal("expected error for different embeddings")
	}
}

func TestValidateDatasetEmbeddingModels_SameBaseDifferentInstanceOK(t *testing.T) {
	// Two KBs using the same base model through different provider instances
	// (the rsplit("@", 2) logic should treat them as the same base).
	kbs := []*entity.Knowledgebase{
		{EmbdID: "BAAI/bge-large-zh-v1.5@instance1@provider1"},
		{EmbdID: "BAAI/bge-large-zh-v1.5@instance2@provider2"},
	}
	if err := ValidateDatasetEmbeddingModels(kbs); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDatasetEmbeddingModels_DifferentBasesErrors(t *testing.T) {
	kbs := []*entity.Knowledgebase{
		{EmbdID: "model-a@instance1@provider1"},
		{EmbdID: "model-b@instance2@provider2"},
	}
	err := ValidateDatasetEmbeddingModels(kbs)
	if err == nil {
		t.Fatal("expected error for different base models")
	}
}

func TestValidateDatasetEmbeddingModels_EmptyList(t *testing.T) {
	if err := ValidateDatasetEmbeddingModels(nil); err != nil {
		t.Fatalf("expected nil for empty list, got %v", err)
	}
}
