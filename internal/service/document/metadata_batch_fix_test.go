package document

import (
	"context"
	"fmt"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/service"

	"gorm.io/gorm"
)

// transientSearchMetadataEngine fails the first SearchMetadata call then
// delegates, modelling a transient index error.
type transientSearchMetadataEngine struct {
	*metadataDocEngine
	failRemaining int
	searchCalls   int
}

func (m *transientSearchMetadataEngine) SearchMetadata(ctx context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	m.searchCalls++
	if m.failRemaining > 0 {
		m.failRemaining--
		return nil, fmt.Errorf("transient metadata search failure")
	}
	return m.metadataDocEngine.SearchMetadata(ctx, req)
}

func setupMetadataBatchDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupServiceTestDB(t)
	db.Create(&entity.Knowledgebase{
		ID: "kb-1", TenantID: "tenant-1", Name: "kb", EmbdID: "embd-1",
		CreatedBy: "user-1", Status: sptr(string(entity.StatusValid)),
	})
	return db
}

// TestGetDocumentMetadataByIDsRetriesTransientFailure is the RED test for #10:
// a single transient search failure must not clear metadata for every doc in
// the batch; a bounded retry should recover it.
func TestGetDocumentMetadataByIDsRetriesTransientFailure(t *testing.T) {
	db := setupMetadataBatchDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	base := newMetadataDocEngine(map[string]map[string]interface{}{
		"doc-1": {"author": "alice"},
		"doc-2": {"author": "bob"},
	}, map[string]string{"doc-1": "kb-1", "doc-2": "kb-1"})
	eng := &transientSearchMetadataEngine{metadataDocEngine: base, failRemaining: 1}

	svc := testDocumentService(t)
	svc.metadataSvc = service.NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), eng)

	got, err := svc.GetDocumentMetadataByIDs(t.Context(), "kb-1", []string{"doc-1", "doc-2"})
	if err != nil {
		t.Fatalf("GetDocumentMetadataByIDs failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("metadata docs = %d (%#v), want 2 after a transient failure", len(got), got)
	}
	if eng.searchCalls < 2 {
		t.Fatalf("SearchMetadata calls = %d, want >= 2 (a retry)", eng.searchCalls)
	}
}

// TestGetDocumentMetadataByIDsRetriesTenantLookupFailure is adversarial: the
// tenant lookup is a separate query from SearchMetadata. A transient failure
// there must also be retried, otherwise every doc in the batch loses metadata.
func TestGetDocumentMetadataByIDsRetriesTenantLookupFailure(t *testing.T) {
	db := setupMetadataBatchDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	// Fail only the first DB query (the tenant lookup) with a transient error.
	failures := 0
	db.Callback().Query().Before("gorm:query").Register("handler:transient_tenant", func(tx *gorm.DB) {
		if failures == 0 {
			failures++
			_ = tx.AddError(fmt.Errorf("transient tenant lookup failure"))
		}
	})

	eng := newMetadataDocEngine(map[string]map[string]interface{}{
		"doc-1": {"author": "alice"},
	}, map[string]string{"doc-1": "kb-1"})
	svc := testDocumentService(t)
	svc.metadataSvc = service.NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), eng)

	got, err := svc.GetDocumentMetadataByIDs(t.Context(), "kb-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("GetDocumentMetadataByIDs failed: %v", err)
	}
	if len(got) != 1 || got["doc-1"]["author"] != "alice" {
		t.Fatalf("metadata = %#v, want doc-1 author=alice after a transient tenant failure", got)
	}
}
