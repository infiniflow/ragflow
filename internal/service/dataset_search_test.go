package service

import (
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestSearchDatasetRequestToSearchDatasetsRequest(t *testing.T) {
	page := 2
	size := 10
	topK := 8
	useKG := true
	threshold := 0.6
	weight := 0.4
	searchID := "search-1"
	rerankID := "rerank-1"
	keyword := true

	req := SearchDatasetRequest{
		Question:               "hello",
		Page:                   &page,
		Size:                   &size,
		DocIDs:                 []string{"doc-1"},
		UseKG:                  &useKG,
		TopK:                   &topK,
		CrossLanguages:         []string{"zh"},
		SearchID:               &searchID,
		MetadataFilter:         map[string]interface{}{"method": "manual"},
		RerankID:               &rerankID,
		Keyword:                &keyword,
		SimilarityThreshold:    &threshold,
		VectorSimilarityWeight: &weight,
	}

	got := req.ToSearchDatasetsRequest("kb-1")
	if len(got.DatasetIDs) != 1 || got.DatasetIDs[0] != "kb-1" {
		t.Fatalf("datasetIDs = %#v", got.DatasetIDs)
	}
	if got.Question != req.Question || got.Page != req.Page || got.Size != req.Size {
		t.Fatalf("request fields were not preserved: %#v", got)
	}
	if got.SearchID != req.SearchID || got.RerankID != req.RerankID {
		t.Fatalf("pointer fields were not preserved: %#v", got)
	}
}

func TestDatasetServiceSearchDatasetsRejectsMixedEmbeddingModels(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	status := string(entity.StatusValid)
	kbs := []*entity.Knowledgebase{
		{
			ID:         "kb-1",
			TenantID:   "tenant-1",
			Name:       "KB 1",
			EmbdID:     "emb-1",
			Permission: string(entity.TenantPermissionMe),
			CreatedBy:  "tenant-1",
			Status:     &status,
		},
		{
			ID:         "kb-2",
			TenantID:   "tenant-1",
			Name:       "KB 2",
			EmbdID:     "emb-2",
			Permission: string(entity.TenantPermissionMe),
			CreatedBy:  "tenant-1",
			Status:     &status,
		},
	}
	for _, kb := range kbs {
		if err := dao.DB.Create(kb).Error; err != nil {
			t.Fatalf("insert knowledgebase %s: %v", kb.ID, err)
		}
	}

	svc := NewDatasetService()
	_, err := svc.SearchDatasets(&SearchDatasetsRequest{
		DatasetIDs: []string{"kb-1", "kb-2"},
		Question:   "test question",
	}, "tenant-1")
	if err == nil {
		t.Fatal("expected embedding model mismatch error")
	}
	if err.Error() != "Datasets use different embedding models." {
		t.Fatalf("error = %q, want Datasets use different embedding models.", err.Error())
	}
}
