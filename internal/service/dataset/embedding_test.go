package dataset

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func TestCheckEmbeddingSkipsOnlyTheCurrentlyBoundModel(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Indexed")
	insertDatasetUpdateModelProvider(t, "provider-1", "tenant-1", "ZHIPU-AI")
	insertDatasetUpdateModelInstance(t, "instance-1", "provider-1", "first")
	insertDatasetUpdateModelInstance(t, "instance-2", "provider-1", "second")
	const modelID = "aabbccdd11223344aabbccdd11223344"
	const otherID = "aabbccdd11223344aabbccdd11223345"
	insertDatasetUpdateTenantModel(t, modelID, "provider-1", "instance-1", "embedding-2", int(entity.ModelTypeEmbedding))
	insertDatasetUpdateTenantModel(t, otherID, "provider-1", "instance-2", "embedding-2", int(entity.ModelTypeEmbedding))
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Updates(map[string]any{
		"embd_id": modelID, "tenant_embd_id": modelID, "chunk_num": 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
	// A same-model check succeeds without even opening the document engine.
	svc := testDatasetUpdateService(t)
	for _, ref := range []string{modelID, "embedding-2@first@ZHIPU-AI"} {
		result, code, err := svc.CheckEmbedding(ctx, "kb-1", &service.CheckEmbeddingRequest{EmbeddingID: ref})
		if err != nil || code != common.CodeSuccess || result.Summary.MatchMode != "same_model" {
			t.Fatalf("CheckEmbedding(%q) = (%+v, %d, %v), want same model", ref, result, code, err)
		}
	}
	result, code, err := svc.CheckEmbedding(ctx, "kb-1", &service.CheckEmbeddingRequest{EmbeddingID: otherID})
	if result != nil || code != common.CodeServerError || err == nil || err.Error() != "doc engine not initialized" {
		t.Fatalf("different instance skipped the vector check: (%+v, %d, %v)", result, code, err)
	}
}
