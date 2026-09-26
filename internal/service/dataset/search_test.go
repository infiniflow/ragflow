package dataset

import (
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func TestSearchDatasetsRejectsDocumentsOutsideDatasets(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	if err := db.Create(&entity.Knowledgebase{
		ID:         "kb-1",
		TenantID:   "tenant-1",
		CreatedBy:  "tenant-1",
		Name:       "Dataset",
		Permission: string(entity.TenantPermissionMe),
		Status:     sptr(string(entity.StatusValid)),
	}).Error; err != nil {
		t.Fatalf("insert test dataset: %v", err)
	}

	_, err := (&DatasetService{
		kbDAO:       dao.NewKnowledgebaseDAO(),
		documentDAO: dao.NewDocumentDAO(),
	}).SearchDatasets(t.Context(), &service.SearchDatasetsRequest{
		Question:    "question",
		DatasetIDs:  []string{"kb-1"},
		DocumentIDs: []string{"not-owned"},
	}, "tenant-1")
	if err == nil || err.Error() != "The datasets don't own the document not-owned" {
		t.Fatalf("error=%v want document ownership error", err)
	}
}

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

func TestSelectMetadataFilteredDocIDs(t *testing.T) {
	tests := []struct {
		name                 string
		current              []string
		filtered             []string
		hasMetadataCondition bool
		filterReturnedEmpty  bool
		want                 []string
	}{
		{name: "metadata filter narrows explicit document ids", current: []string{"doc-1"}, filtered: []string{"doc-1"}, want: []string{"doc-1"}},
		{name: "metadata filter cannot widen explicit document ids", current: []string{"doc-1"}, filtered: []string{}, want: []string{}},
		{name: "empty generated filter keeps explicit document ids", current: []string{"doc-1"}, filtered: nil, filterReturnedEmpty: true, want: []string{"doc-1"}},
		{name: "metadata condition uses definitive empty result", current: []string{"doc-1"}, filtered: nil, hasMetadataCondition: true, filterReturnedEmpty: true, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectMetadataFilteredDocIDs(tt.current, tt.filtered, tt.hasMetadataCondition, tt.filterReturnedEmpty)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}
