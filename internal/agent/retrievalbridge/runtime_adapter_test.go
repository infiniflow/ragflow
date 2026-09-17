package retrievalbridge

import (
	"context"
	"testing"

	agentrunt "ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"

	"gorm.io/gorm"
)

type captureRetrievalService struct {
	req agenttool.RetrievalRequest
}

func (s *captureRetrievalService) Search(_ context.Context, _ *gorm.DB, req agenttool.RetrievalRequest) ([]agenttool.RetrievalChunk, error) {
	s.req = req
	return nil, nil
}

func TestRuntimeAdapterDisablesDenseFallback(t *testing.T) {
	previous := agenttool.GetRetrievalService()
	service := &captureRetrievalService{}
	agenttool.SetRetrievalService(service)
	t.Cleanup(func() { agenttool.SetRetrievalService(previous) })
	_, err := NewRuntimeAdapter().Search(t.Context(), nil, agentrunt.RetrievalRequest{Query: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if service.req.AllowDenseFallback == nil || *service.req.AllowDenseFallback {
		t.Fatal("Agentic runtime retrieval must disable dense fallback")
	}
}

// TestRuntimeAdapterKeepsRankFeatureStatesApart: the harness sends an absent
// rank feature for the hybrid/vector/bm25 legs (Python omits the argument, so
// the retriever's pagerank default applies) and an explicitly empty one for the
// retrieve leg with no tag vocabulary (Python passes None, which suppresses
// that default). Collapsing the two into one nil made retrieval() re-enable a
// rank-feature clause Python never sends.
func TestRuntimeAdapterKeepsRankFeatureStatesApart(t *testing.T) {
	previous := agenttool.GetRetrievalService()
	service := &captureRetrievalService{}
	agenttool.SetRetrievalService(service)
	t.Cleanup(func() { agenttool.SetRetrievalService(previous) })

	adapter := NewRuntimeAdapter()
	if _, err := adapter.Search(t.Context(), nil, agentrunt.RetrievalRequest{Query: "legs"}); err != nil {
		t.Fatal(err)
	}
	if service.req.RankFeature != nil {
		t.Fatalf("omitted rank feature = %v, want nil", service.req.RankFeature)
	}

	if _, err := adapter.Search(t.Context(), nil, agentrunt.RetrievalRequest{
		Query:       "retrieve leg",
		RankFeature: map[string]float64{},
	}); err != nil {
		t.Fatal(err)
	}
	if service.req.RankFeature == nil || len(*service.req.RankFeature) != 0 {
		t.Fatalf("explicit empty rank feature = %v, want non-nil empty map", service.req.RankFeature)
	}
}
