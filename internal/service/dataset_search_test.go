package service

import "testing"

func TestSearchDatasetRequestToSearchDatasetsRequest(t *testing.T) {
	page := 2
	size := 15
	topK := 128
	useKG := true
	keyword := true
	similarityThreshold := 0.42
	vectorSimilarityWeight := 0.8
	searchID := "search-1"
	rerankID := "rerank-1"
	req := &SearchDatasetRequest{
		Question:               "hello world",
		Page:                   &page,
		Size:                   &size,
		DocIDs:                 []string{"doc-1", "doc-2"},
		UseKG:                  &useKG,
		TopK:                   &topK,
		CrossLanguages:         []string{"en", "zh"},
		SearchID:               &searchID,
		MetadataFilter:         map[string]interface{}{"method": "manual"},
		RerankID:               &rerankID,
		Keyword:                &keyword,
		SimilarityThreshold:    &similarityThreshold,
		VectorSimilarityWeight: &vectorSimilarityWeight,
	}

	converted := req.ToSearchDatasetsRequest("dataset-1")
	if len(converted.DatasetIDs) != 1 || converted.DatasetIDs[0] != "dataset-1" {
		t.Fatalf("dataset_ids=%v want [dataset-1]", converted.DatasetIDs)
	}
	if converted.Question != req.Question || converted.Page != req.Page || converted.Size != req.Size {
		t.Fatalf("converted request did not preserve pagination/question fields: %#v", converted)
	}
	if len(converted.DocIDs) != 2 || converted.DocIDs[0] != "doc-1" || converted.DocIDs[1] != "doc-2" {
		t.Fatalf("doc_ids=%v want [doc-1 doc-2]", converted.DocIDs)
	}
	if converted.UseKG != req.UseKG || converted.TopK != req.TopK || converted.SearchID != req.SearchID {
		t.Fatalf("converted request did not preserve optional fields: %#v", converted)
	}
	if converted.MetadataFilter["method"] != "manual" || converted.RerankID != req.RerankID || converted.Keyword != req.Keyword {
		t.Fatalf("converted request did not preserve search config fields: %#v", converted)
	}
	if converted.SimilarityThreshold != req.SimilarityThreshold || converted.VectorSimilarityWeight != req.VectorSimilarityWeight {
		t.Fatalf("converted request did not preserve thresholds: %#v", converted)
	}
}
