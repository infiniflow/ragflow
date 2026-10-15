package chunk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/service"
)

func TestListEntityChunks(t *testing.T) {
	pushChunkTestDB(t, setupChunkTestDB(t))
	insertChunkTestUserTenant(t, "user-1", "tenant-1")
	insertChunkTestKB(t, "kb-1", "tenant-1")
	insertChunkTestDoc(t, "doc-1", "kb-1")
	ids := make([]string, 125)
	for i := range ids {
		ids[i] = fmt.Sprintf("%016x", len(ids)-i)
	}
	payload, _ := json.Marshal(map[string]interface{}{"source_chunk_ids": ids})
	for _, tt := range []struct {
		name, payload, document, kind string
		wantNotFound                  bool
	}{
		{"page two", string(payload), "doc-1", "entity", false},
		{"empty sources", `{"source_chunk_ids":[]}`, "doc-1", "entity", false},
		{"other document", string(payload), "other-doc", "entity", true},
		{"relation", string(payload), "doc-1", "relation", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			engine := &entityChunksTestEngine{entity: map[string]interface{}{
				"id": "entity-1", "kb_id": "kb-1", "doc_id": tt.document, "type_kwd": tt.kind,
				"content_with_weight": tt.payload, "source_chunk_ids": []string{"column-only"},
			}}
			svc := &ChunkService{docEngine: engine, kbDAO: dao.NewKnowledgebaseDAO()}
			page, size, available := 2, 100, 0
			response, err := svc.List(t.Context(), &service.ListChunksRequest{
				DatasetID: "kb-1", DocID: "doc-1", EntityID: "entity-1", Page: &page, Size: &size,
				Keywords: "invoice", AvailableInt: &available,
			}, "user-1")
			if tt.wantNotFound {
				if !errors.Is(err, common.ErrNotFound) || len(engine.queries) != 1 {
					t.Fatalf("error = %v, queries = %d; want not found after scoped lookup", err, len(engine.queries))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			lookup := engine.queries[0]
			if lookup.Filter["id"] != "entity-1" || lookup.Filter["doc_id"] != "doc-1" || lookup.Filter["knowledge_graph_kwd"] != "entity" || !reflect.DeepEqual(lookup.KbIDs, []string{"kb-1"}) {
				t.Errorf("entity lookup scope = %#v", lookup)
			}
			if tt.name == "empty sources" {
				if len(engine.queries) != 1 || response.Total != 0 || response.Chunks == nil || len(response.Chunks) != 0 || response.Doc["id"] != "doc-1" {
					t.Errorf("empty page = %#v, queries = %d", response, len(engine.queries))
				}
				return
			}
			if len(engine.queries) != 2 || response.Total != 125 || len(response.Chunks) != 25 {
				t.Fatalf("response = %#v, queries = %d", response, len(engine.queries))
			}
			chunks := engine.queries[1]
			if chunks.Offset != 100 || chunks.Limit != 100 || !reflect.DeepEqual(chunks.Filter["id"], ids) || chunks.Filter["available_int"] != 0 || chunks.Filter["doc_id"] != "doc-1" || len(chunks.MatchExprs) != 1 {
				t.Errorf("entity chunk pagination/filter = %#v", chunks)
			}
		})
	}
}

type entityChunksTestEngine struct {
	parseTestDocEngine
	entity  map[string]interface{}
	queries []*types.SearchRequest
}

func (e *entityChunksTestEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.queries = append(e.queries, req)
	if len(e.queries) == 1 {
		return &types.SearchResult{Chunks: []map[string]interface{}{e.entity}}, nil
	}
	chunks := make([]map[string]interface{}, 25)
	for i := range chunks {
		chunks[i] = map[string]interface{}{"id": fmt.Sprintf("c%d", i)}
	}
	return &types.SearchResult{Chunks: chunks, Total: 125}, nil
}
