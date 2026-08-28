package service

import (
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupValidateEmbeddingTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.TenantModel{}); err != nil {
		t.Fatalf("failed to migrate test schema: %v", err)
	}
	return db
}

func TestValidateDatasetEmbeddingModels_AllHaveEmbeddingModel(t *testing.T) {
	kbs := []*entity.Knowledgebase{
		{EmbdID: "BAAI/bge-large-zh-v1.5@Builtin"},
		{EmbdID: "BAAI/bge-large-zh-v1.5@Builtin"},
	}
	if err := ValidateDatasetEmbeddingModels(t.Context(), nil, kbs); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDatasetEmbeddingModels_NoneHasEmbeddingModel(t *testing.T) {
	kbs := []*entity.Knowledgebase{
		{EmbdID: ""},
		{EmbdID: ""},
	}
	if err := ValidateDatasetEmbeddingModels(t.Context(), nil, kbs); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDatasetEmbeddingModels_MixedErrors(t *testing.T) {
	kbs := []*entity.Knowledgebase{
		{EmbdID: "BAAI/bge-large-zh-v1.5@Builtin"},
		{EmbdID: ""},
	}
	err := ValidateDatasetEmbeddingModels(t.Context(), nil, kbs)
	if err == nil {
		t.Fatal("expected error for mixed embedding")
	}
	if err.Error() != "cannot search across datasets where some have embedding models and others do not" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDatasetEmbeddingModels_DifferentEmbeddingsErrors(t *testing.T) {
	kbs := []*entity.Knowledgebase{
		{EmbdID: "model-a@provider-1"},
		{EmbdID: "model-b@provider-2"},
	}
	err := ValidateDatasetEmbeddingModels(t.Context(), nil, kbs)
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
	if err := ValidateDatasetEmbeddingModels(t.Context(), nil, kbs); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDatasetEmbeddingModels_DifferentBasesErrors(t *testing.T) {
	kbs := []*entity.Knowledgebase{
		{EmbdID: "model-a@instance1@provider1"},
		{EmbdID: "model-b@instance2@provider2"},
	}
	err := ValidateDatasetEmbeddingModels(t.Context(), nil, kbs)
	if err == nil {
		t.Fatal("expected error for different base models")
	}
}

func TestValidateDatasetEmbeddingModels_EmptyList(t *testing.T) {
	if err := ValidateDatasetEmbeddingModels(t.Context(), nil, nil); err != nil {
		t.Fatalf("expected nil for empty list, got %v", err)
	}
}

func TestValidateDatasetEmbeddingModels_TenantModelIDsResolveToSameBase(t *testing.T) {
	db := setupValidateEmbeddingTestDB(t)
	for _, m := range []entity.TenantModel{
		{ID: "11111111111111111111111111111111", ModelName: "BAAI/bge-m3"},
		{ID: "22222222222222222222222222222222", ModelName: "BAAI/bge-m3"},
	} {
		if err := db.Create(&m).Error; err != nil {
			t.Fatalf("failed to seed tenant_model: %v", err)
		}
	}

	// Two KBs referencing different tenant_model rows (e.g. different provider
	// instances) that serve the same base model must validate together.
	kbs := []*entity.Knowledgebase{
		{EmbdID: "11111111111111111111111111111111"},
		{EmbdID: "22222222222222222222222222222222"},
	}
	if err := ValidateDatasetEmbeddingModels(t.Context(), db, kbs); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDatasetEmbeddingModels_TenantModelIDAndLegacyCompositeMatch(t *testing.T) {
	db := setupValidateEmbeddingTestDB(t)
	if err := db.Create(&entity.TenantModel{ID: "11111111111111111111111111111111", ModelName: "BAAI/bge-m3"}).Error; err != nil {
		t.Fatalf("failed to seed tenant_model: %v", err)
	}

	// One KB stores the tenant_model id, the other a legacy composite name
	// pointing at the same base model through another instance.
	kbs := []*entity.Knowledgebase{
		{EmbdID: "11111111111111111111111111111111"},
		{EmbdID: "BAAI/bge-m3@2@SILICONFLOW"},
	}
	if err := ValidateDatasetEmbeddingModels(t.Context(), db, kbs); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDatasetEmbeddingModels_ResolvedDifferentBasesError(t *testing.T) {
	db := setupValidateEmbeddingTestDB(t)
	for _, m := range []entity.TenantModel{
		{ID: "11111111111111111111111111111111", ModelName: "BAAI/bge-m3"},
		{ID: "22222222222222222222222222222222", ModelName: "Qwen/Qwen3-Embedding-0.6B"},
	} {
		if err := db.Create(&m).Error; err != nil {
			t.Fatalf("failed to seed tenant_model: %v", err)
		}
	}

	kbs := []*entity.Knowledgebase{
		{EmbdID: "11111111111111111111111111111111"},
		{EmbdID: "22222222222222222222222222222222"},
	}
	if err := ValidateDatasetEmbeddingModels(t.Context(), db, kbs); err == nil {
		t.Fatal("expected error for different resolved base models")
	}
}

func TestValidateDatasetEmbeddingModels_UnresolvableIDsStayIsolated(t *testing.T) {
	// Stale ids that no longer resolve must not suddenly compare equal.
	kbs := []*entity.Knowledgebase{
		{EmbdID: "11111111111111111111111111111111"},
		{EmbdID: "22222222222222222222222222222222"},
	}
	if err := ValidateDatasetEmbeddingModels(t.Context(), nil, kbs); err == nil {
		t.Fatal("expected error for distinct unresolvable embedding ids")
	}
}
