package dataset

import (
	"context"
	"strings"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func TestSearchDatasetRequestToSearchDatasetsRequest(t *testing.T) {
	page := 2
	pageSize := 15
	knnTopK := 128
	knnNumCandidates := 256
	useKG := true
	keyword := true
	similarityThreshold := 0.42
	vectorSimilarityWeight := 0.8
	searchID := "search-1"
	rerankID := "rerank-1"
	includeKnowledgeCompilation := false
	req := &service.SearchDatasetRequest{
		Question:               "hello world",
		Page:                   &page,
		PageSize:               &pageSize,
		DocumentIDs:            []string{"doc-1", "doc-2"},
		UseKG:                  &useKG,
		KNNTopK:                &knnTopK,
		KNNNumCandidates:       &knnNumCandidates,
		CrossLanguages:         []string{"en", "zh"},
		SearchID:               &searchID,
		MetadataCondition:      map[string]interface{}{"logic": "and"},
		RerankID:               &rerankID,
		Keyword:                &keyword,
		SimilarityThreshold:    &similarityThreshold,
		VectorSimilarityWeight: &vectorSimilarityWeight,
		IncludeCompiledChunks:  &includeKnowledgeCompilation,
	}

	converted := req.ToSearchDatasetsRequest("dataset-1")
	if len(converted.DatasetIDs) != 1 || converted.DatasetIDs[0] != "dataset-1" {
		t.Fatalf("dataset_ids=%v want [dataset-1]", converted.DatasetIDs)
	}
	if converted.Question != req.Question || converted.Page != req.Page || converted.PageSize != req.PageSize {
		t.Fatalf("converted request did not preserve pagination/question fields: %#v", converted)
	}
	if len(converted.DocumentIDs) != 2 || converted.DocumentIDs[0] != "doc-1" || converted.DocumentIDs[1] != "doc-2" {
		t.Fatalf("document_ids=%v want [doc-1 doc-2]", converted.DocumentIDs)
	}
	if converted.UseKG != req.UseKG || converted.KNNTopK != req.KNNTopK || converted.KNNNumCandidates != req.KNNNumCandidates || converted.SearchID != req.SearchID {
		t.Fatalf("converted request did not preserve optional fields: %#v", converted)
	}
	if converted.MetadataCondition["logic"] != "and" || converted.RerankID != req.RerankID || converted.Keyword != req.Keyword {
		t.Fatalf("converted request did not preserve search config fields: %#v", converted)
	}
	if converted.SimilarityThreshold != req.SimilarityThreshold || converted.VectorSimilarityWeight != req.VectorSimilarityWeight {
		t.Fatalf("converted request did not preserve thresholds: %#v", converted)
	}
	if converted.IncludeCompiledChunks != req.IncludeCompiledChunks {
		t.Fatalf("converted request did not preserve include_knowledge_compilation: %#v", converted)
	}
}

// seedSearchRecord inserts a saved search app row with the given config.
func seedSearchRecord(t *testing.T, db *gorm.DB, id, tenantID string, config map[string]interface{}) {
	t.Helper()
	if err := db.AutoMigrate(&entity.Search{}); err != nil {
		t.Fatalf("migrate search: %v", err)
	}
	status := "1"
	rec := &entity.Search{ID: id, TenantID: tenantID, Name: "app", CreatedBy: tenantID, Status: &status}
	rec.SearchConfig = entity.JSONMap(config)
	if err := db.Create(rec).Error; err != nil {
		t.Fatalf("seed search: %v", err)
	}
}

// TestSearchDatasetsUsesDatasetIDsFromSavedSearch pins the Python parity
// rule: POST /api/v1/retrieval may omit dataset_ids when search_id refers
// to a saved search app whose search_config carries kb_ids. The merged
// dataset set is what gets access-checked.
func TestSearchDatasetsUsesDatasetIDsFromSavedSearch(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	tenantName := "T"
	tenant := &entity.Tenant{ID: "tenant-1", Name: &tenantName}
	if err := db.Create(tenant).Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	own := &entity.Knowledgebase{ID: "ds-own", TenantID: "tenant-1", Name: "own", Permission: "me", EmbdID: "embd-1"}
	foreign := &entity.Knowledgebase{ID: "ds-foreign", TenantID: "tenant-2", Name: "foreign", Permission: "me", EmbdID: "embd-1"}
	for _, kb := range []*entity.Knowledgebase{own, foreign} {
		st := "1"
		kb.Status = &st
		if err := db.Create(kb).Error; err != nil {
			t.Fatalf("seed kb: %v", err)
		}
	}
	svc := &DatasetService{kbDAO: dao.NewKnowledgebaseDAO(), searchService: service.NewSearchService()}

	// Own dataset via config kb_ids: must get past the dataset_ids check
	// and the access check (it fails later on model resolution, which
	// proves the config-supplied ids were used).
	searchID := "search-1"
	seedSearchRecord(t, db, searchID, "tenant-1", map[string]interface{}{"kb_ids": []interface{}{"ds-own"}})
	req := &service.SearchDatasetsRequest{Question: "hello", SearchID: &searchID}
	_, err := svc.SearchDatasets(context.Background(), req, "tenant-1")
	if err == nil {
		t.Fatal("expected a downstream error (no model configured), got nil")
	}
	if strings.Contains(err.Error(), "dataset_ids") || strings.Contains(err.Error(), "invalid search_id") || strings.Contains(err.Error(), "authorized") {
		t.Fatalf("config-supplied dataset_ids were not honored: %v", err)
	}

	// Foreign dataset via config kb_ids: the access check must reject it.
	seedSearchRecord(t, db, "search-2", "tenant-1", map[string]interface{}{"kb_ids": []interface{}{"ds-foreign"}})
	searchID2 := "search-2"
	req2 := &service.SearchDatasetsRequest{Question: "hello", SearchID: &searchID2}
	_, err2 := svc.SearchDatasets(context.Background(), req2, "tenant-1")
	if err2 == nil || !strings.Contains(err2.Error(), "authorized") {
		t.Fatalf("foreign dataset from config must be rejected, got: %v", err2)
	}

	// Another tenant's search app must not resolve at all.
	seedSearchRecord(t, db, "search-3", "tenant-2", map[string]interface{}{"kb_ids": []interface{}{"ds-own"}})
	searchID3 := "search-3"
	req3 := &service.SearchDatasetsRequest{Question: "hello", SearchID: &searchID3}
	_, err3 := svc.SearchDatasets(context.Background(), req3, "tenant-1")
	if err3 == nil || !strings.Contains(err3.Error(), "invalid search_id") {
		t.Fatalf("cross-tenant search_id must be rejected, got: %v", err3)
	}
}
