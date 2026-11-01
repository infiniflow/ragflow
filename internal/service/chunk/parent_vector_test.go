package chunk

import (
	"context"
	"reflect"
	"testing"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/service/nlp"
)

type parentVectorEngine struct {
	engine.DocEngine
	fetch *types.SearchRequest
}

func (e *parentVectorEngine) GetType() string { return "elasticsearch" }

func (e *parentVectorEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if req.IncludeUnavailable {
		return &types.SearchResult{Chunks: []map[string]interface{}{{
			"id": "parent", "doc_id": "doc", "kb_id": "kb",
			"content_with_weight": "The warranty lasts twelve months.",
		}}}, nil
	}
	e.fetch = req
	if reflect.DeepEqual(req.Filter["id"], []interface{}{"child"}) {
		return &types.SearchResult{Chunks: []map[string]interface{}{{
			"id": "child", "q_2_vec": []float64{1, 0},
		}}}, nil
	}
	return &types.SearchResult{}, nil
}

func TestHydrateChunkVectorsPreservesRetrievalIdentity(t *testing.T) {
	for _, parent := range []bool{false, true} {
		name, wantID := "flat", "child"
		if parent {
			name, wantID = "parent", "parent"
		}
		t.Run(name, func(t *testing.T) {
			e := &parentVectorEngine{}
			chunks := []map[string]interface{}{{
				"chunk_id": "child", "doc_id": "doc", "kb_id": "kb",
				"vector": []float64{0, 0}, "similarity": 0.9,
			}}
			if parent {
				chunks[0]["mom_id"] = "parent"
				chunks = nlp.RetrievalByChildren(chunks, []string{"tenant"}, e, t.Context())
			}
			hydrateChunkVectors(t.Context(), e, chunks, []string{"kb"}, []string{"tenant"})
			if !reflect.DeepEqual(chunks[0]["vector"], []float64{1, 0}) {
				t.Fatalf("retrieved chunk lost its source vector: %#v", chunks)
			}
			if chunks[0]["chunk_id"] != wantID || chunks[0]["doc_id"] != "doc" {
				t.Fatalf("hydration changed reference identity: %#v", chunks)
			}
			if e.fetch == nil || !reflect.DeepEqual(e.fetch.SelectFields, []string{"q_2_vec"}) || !reflect.DeepEqual(e.fetch.KbIDs, []string{"kb"}) {
				t.Fatalf("wrong vector projection or dataset scope: %#v", e.fetch)
			}
		})
	}
}
