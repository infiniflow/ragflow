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
