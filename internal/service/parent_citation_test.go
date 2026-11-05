package service

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/service/nlp"
)

type parentCitationEngine struct {
	engine.DocEngine
	missing bool
	fetch   *types.SearchRequest
}

func (e *parentCitationEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if req.IncludeUnavailable {
		return &types.SearchResult{Chunks: []map[string]interface{}{{
			"id": "parent", "doc_id": "doc", "kb_id": "kb",
			"content_with_weight": "The warranty lasts twelve months. Contact support for repairs.",
		}}}, nil
	}
	e.fetch = req
	if !e.missing && reflect.DeepEqual(req.Filter["id"], []string{"child"}) {
		return &types.SearchResult{Chunks: []map[string]interface{}{{"id": "child", "q_2_vec": []float64{1, 0}}}}, nil
	}
	return &types.SearchResult{}, nil
}

type warrantyEmbedder struct{}

func (warrantyEmbedder) Encode(context.Context, []string) ([][]float64, error) {
	return [][]float64{{1, 0}}, nil
}

func TestParentExpansionAutomaticCitation(t *testing.T) {
	for _, missing := range []bool{false, true} {
		name := "available-child"
		if missing {
			name = "missing-child"
		}
		t.Run(name, func(t *testing.T) {
			e := &parentCitationEngine{missing: missing}
			chunks := nlp.RetrievalByChildren([]map[string]interface{}{{
				"chunk_id": "child", "mom_id": "parent", "doc_id": "doc", "kb_id": "kb",
				"vector": []float64{0, 0}, "similarity": 0.9,
			}}, []string{"tenant"}, e, t.Context())
			info := map[string]interface{}{"chunks": chunks}
			hits, err := HydrateChunkVectors(t.Context(), info, []string{"tenant"}, []string{"kb"}, 0, e)
			if err != nil {
				t.Fatal(err)
			}
			if e.fetch == nil || !reflect.DeepEqual(e.fetch.Filter["id"], []string{"child"}) || !reflect.DeepEqual(e.fetch.SelectFields, []string{"id", "q_2_vec"}) {
				t.Fatalf("hydration must fetch child at actual dimension: %#v", e.fetch)
			}
			answer, cited := InsertCitations(t.Context(), "The warranty lasts twelve months.", NewSourcedChunks(chunks), warrantyEmbedder{}, [][]float64{chunkVector(chunks[0])})
			if missing {
				if hits != 0 || len(cited) != 0 {
					t.Fatalf("missing child produced unsupported citation: %q", answer)
				}
			} else if hits != 1 || len(cited) != 1 || !strings.Contains(answer, "[ID:0]") {
				t.Fatalf("parent lost automatic citation: hits=%d answer=%q cited=%v", hits, answer, cited)
			}
			if chunks[0]["chunk_id"] != "parent" {
				t.Fatal("citation content identity changed")
			}
			formatted := chunksFormat(chunks)
			if formatted[0]["id"] != "parent" || formatted[0]["document_id"] != "doc" {
				t.Fatalf("public citation lost parent identity: %#v", formatted)
			}
			if _, present := formatted[0]["vector_chunk_id"]; present {
				t.Fatal("public citation contains internal vector provenance")
			}
		})
	}
}
