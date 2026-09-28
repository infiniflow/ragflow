package retrievalbridge

import (
	"context"
	"errors"
	"reflect"
	"testing"

	agentrunt "ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"

	"gorm.io/gorm"
)

type captureRetrievalService struct {
	req    agenttool.RetrievalRequest
	chunks []agenttool.RetrievalChunk
	err    error
}

func (s *captureRetrievalService) Search(_ context.Context, _ *gorm.DB, req agenttool.RetrievalRequest) ([]agenttool.RetrievalChunk, error) {
	s.req = req
	return s.chunks, s.err
}

func TestRuntimeAdapterDisablesDenseFallback(t *testing.T) {
	service := &captureRetrievalService{}
	_, err := NewRuntimeAdapter(service).Search(t.Context(), nil, agentrunt.RetrievalRequest{Query: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if service.req.AllowDenseFallback == nil || *service.req.AllowDenseFallback {
		t.Fatal("Agentic runtime retrieval must disable dense fallback")
	}
}

// TestRuntimeAdapterDoesNotResolveItsTargetFromTheSharedRegistry reproduces the
// server's boot order (cmd/ragflow_server.go): the NLP adapter is registered,
// then the bridge REPLACES it in the same shared registry
// (agenttool.SetRetrievalService and runtime.SetRetrievalService write one
// singleton). The bridge must still reach the adapter it was built with;
// resolving the registry on every call made Search call itself, which is a stack
// overflow — so a regression here crashes this test rather than failing it.
func TestRuntimeAdapterDoesNotResolveItsTargetFromTheSharedRegistry(t *testing.T) {
	previous := agenttool.GetRetrievalService()
	t.Cleanup(func() { agenttool.SetRetrievalService(previous) })

	service := &captureRetrievalService{chunks: []agenttool.RetrievalChunk{{ID: "from-adapter"}}}
	agenttool.SetRetrievalService(service)
	bridge := NewRuntimeAdapter(service)
	// boot order, second half: the bridge itself becomes the registry's value
	agenttool.SetRetrievalService(bridge)

	chunks, err := bridge.Search(t.Context(), nil, agentrunt.RetrievalRequest{Query: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 || chunks[0].ID != "from-adapter" {
		t.Fatalf("bridge did not reach the adapter it was built with: %+v", chunks)
	}
}

// TestRuntimeAdapterPreservesEveryChunkField pins the field-preservation contract
// at the bridge. A hand-written field list is how ChunkIndex/PageNum were
// silently dropped, and those two are what let the deep-read tools sort a
// document into reading order.
func TestRuntimeAdapterPreservesEveryChunkField(t *testing.T) {
	want := agenttool.RetrievalChunk{
		ID:               "c1",
		Content:          "content",
		DocumentID:       "doc",
		DocumentName:     "doc.md",
		DatasetID:        "kb",
		ImageID:          "img",
		DocType:          "text",
		URL:              "http://example.invalid",
		Positions:        []any{1.0, 2.0},
		ChunkIndex:       7,
		PageNum:          3,
		MomID:            "mom",
		Score:            0.9,
		TermSimilarity:   0.5,
		VectorSimilarity: 0.4,
	}
	service := &captureRetrievalService{chunks: []agenttool.RetrievalChunk{want}}

	got, err := NewRuntimeAdapter(service).Search(t.Context(), nil, agentrunt.RetrievalRequest{Query: "q"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d chunks, want 1", len(got))
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Fatalf("chunk lost fields across the bridge:\n got %+v\nwant %+v", got[0], want)
	}
}

func TestRuntimeAdapterWithoutServiceReportsTheRegistrySentinel(t *testing.T) {
	_, err := NewRuntimeAdapter(nil).Search(t.Context(), nil, agentrunt.RetrievalRequest{Query: "q"})
	if !errors.Is(err, agenttool.ErrRetrievalServiceMissing) {
		t.Fatalf("want ErrRetrievalServiceMissing, got %v", err)
	}
}
