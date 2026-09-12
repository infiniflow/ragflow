package dataset

import (
	"testing"

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

func TestApplySavedSearchConfig_RequestFieldsWin(t *testing.T) {
	st := 0.9
	vsw := 0.7
	topK := 512
	rcc := 256
	useKG := false
	keyword := true
	rerank := "my-rerank"
	req := &service.SearchDatasetsRequest{
		SimilarityThreshold:    &st,
		VectorSimilarityWeight: &vsw,
		KNNTopK:                &topK,
		RerankCandidatesCount:  &rcc,
		UseKG:                  &useKG,
		Keyword:                &keyword,
		RerankID:               &rerank,
		CrossLanguages:         []string{"en"},
	}
	c := searchRunConfig{
		rerankCandidatesCount:  rcc,
		similarityThreshold:    st,
		vectorSimilarityWeight: vsw,
		knnTopK:                topK,
		useKG:                  useKG,
		crossLanguages:         []string{"en"},
		keyword:                keyword,
		rerankID:               rerank,
	}
	c.applySavedSearchConfig(req, map[string]interface{}{
		"similarity_threshold":     0.1,
		"vector_similarity_weight": 0.2,
		"top_k":                    32.0,
		"rerank_candidates_count":  42,
		"use_kg":                   true,
		"keyword":                  false,
		"rerank_id":                "other",
		"cross_languages":          []interface{}{"zh"},
		"chat_id":                  "chat-1",
	})
	if c.similarityThreshold != 0.9 {
		t.Errorf("explicit similarity_threshold overridden: got %v", c.similarityThreshold)
	}
	if c.vectorSimilarityWeight != 0.7 {
		t.Errorf("explicit vector_similarity_weight overridden: got %v", c.vectorSimilarityWeight)
	}
	if c.knnTopK != 512 {
		t.Errorf("explicit knn_top_k overridden: got %v", c.knnTopK)
	}
	if c.rerankCandidatesCount != 256 {
		t.Errorf("explicit rerank_candidates_count overridden: got %v", c.rerankCandidatesCount)
	}
	if c.useKG != false {
		t.Errorf("explicit use_kg=false overridden: got %v", c.useKG)
	}
	if c.keyword != true {
		t.Errorf("explicit keyword overridden: got %v", c.keyword)
	}
	if c.rerankID != "my-rerank" {
		t.Errorf("explicit rerank_id overridden: got %v", c.rerankID)
	}
	if len(c.crossLanguages) != 1 || c.crossLanguages[0] != "en" {
		t.Errorf("explicit cross_languages overridden: got %v", c.crossLanguages)
	}
	if c.chatID != "chat-1" {
		t.Errorf("chat_id not picked up from saved config: got %q", c.chatID)
	}
}

func TestApplySavedSearchConfig_FillsUnsetFields(t *testing.T) {
	req := &service.SearchDatasetsRequest{}
	c := searchRunConfig{
		rerankCandidatesCount:  64,
		similarityThreshold:    0.2,
		vectorSimilarityWeight: 0.3,
		knnTopK:                1024,
	}
	c.applySavedSearchConfig(req, map[string]interface{}{
		"similarity_threshold":     0.1,
		"vector_similarity_weight": 0.15,
		"top_k":                    4096.0,
		"rerank_candidates_count":  42,
		"use_kg":                   true,
		"cross_languages":          []interface{}{"zh", "en"},
	})
	if c.similarityThreshold != 0.1 {
		t.Errorf("saved similarity_threshold not applied: got %v", c.similarityThreshold)
	}
	if c.vectorSimilarityWeight != 0.15 {
		t.Errorf("saved vector_similarity_weight not applied: got %v", c.vectorSimilarityWeight)
	}
	if c.knnTopK != 2048 {
		t.Errorf("saved top_k not applied with clamp: got %v", c.knnTopK)
	}
	if c.rerankCandidatesCount != 42 {
		t.Errorf("saved rerank_candidates_count not applied: got %v", c.rerankCandidatesCount)
	}
	if !c.useKG {
		t.Errorf("saved use_kg not applied: got %v", c.useKG)
	}
	if len(c.crossLanguages) != 2 || c.crossLanguages[0] != "zh" {
		t.Errorf("saved cross_languages not applied: got %v", c.crossLanguages)
	}
}

func TestApplySavedSearchConfig_DefaultRerankCandidates(t *testing.T) {
	// No request value and no saved value: Python's setdefault leaves 100.
	req := &service.SearchDatasetsRequest{}
	c := searchRunConfig{rerankCandidatesCount: 64}
	c.applySavedSearchConfig(req, map[string]interface{}{})
	if c.rerankCandidatesCount != 100 {
		t.Errorf("expected setdefault 100, got %v", c.rerankCandidatesCount)
	}
}

func TestApplySavedSearchConfig_MetadataConditionBeatsSavedFilter(t *testing.T) {
	savedFilter := map[string]interface{}{"method": "manual"}
	req := &service.SearchDatasetsRequest{
		MetadataCondition: map[string]interface{}{"conditions": []interface{}{}},
	}
	c := searchRunConfig{}
	c.applySavedSearchConfig(req, map[string]interface{}{"meta_data_filter": savedFilter})
	if c.metadataFilter != nil {
		t.Errorf("request metadata_condition should beat saved meta_data_filter, got %v", c.metadataFilter)
	}

	req2 := &service.SearchDatasetsRequest{}
	c2 := searchRunConfig{}
	c2.applySavedSearchConfig(req2, map[string]interface{}{"meta_data_filter": savedFilter})
	if c2.metadataFilter == nil {
		t.Error("saved meta_data_filter not applied when request has no filter")
	}
}
