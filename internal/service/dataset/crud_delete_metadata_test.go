package dataset

import (
	"context"
	"testing"

	"ragflow/internal/entity"
)

// recordingMetadataEngine embeds fakeChatDocEngine and records the metadata
// delete condition passed to the engine, so the regression test can assert on
// it without a database.
type recordingMetadataEngine struct {
	fakeChatDocEngine
	cond   map[string]interface{}
	tenant string
}

func (e *recordingMetadataEngine) DeleteMetadata(_ context.Context, cond map[string]interface{}, tenantID string) (int64, error) {
	e.cond, e.tenant = cond, tenantID
	return 0, nil
}

// TestDeleteDatasetEngineDataDeletesMetadataByDataset pins the condition sent
// to the engine when a dataset is deleted: the whole dataset's metadata is
// selected via kb_id. Passing the document id list under doc_id (the old
// behavior) made the ES query builder emit an invalid single-value term query
// with an array payload, failing with a 400 that was only logged as a
// warning; passing the unbounded id list under id risked the same failure via
// index.max_terms_count and missed rows whose document row was already gone.
func TestDeleteDatasetEngineDataDeletesMetadataByDataset(t *testing.T) {
	eng := &recordingMetadataEngine{}
	svc := &DatasetService{docEngine: eng}
	svc.deleteDatasetEngineData(context.Background(), &entity.Knowledgebase{ID: "kb-1", TenantID: "tenant-1"}, []string{"doc-1", "doc-2"})

	if got := eng.cond["kb_id"]; got != "kb-1" {
		t.Fatalf("metadata condition kb_id = %v, want kb-1", got)
	}
	if _, ok := eng.cond["id"]; ok {
		t.Fatalf("metadata condition must not carry an unbounded id list: %v", eng.cond)
	}
	if _, ok := eng.cond["doc_id"]; ok {
		t.Fatalf("metadata condition still uses doc_id: %v", eng.cond)
	}
	if eng.tenant != "tenant-1" {
		t.Fatalf("metadata delete tenant = %q, want tenant-1", eng.tenant)
	}
}
