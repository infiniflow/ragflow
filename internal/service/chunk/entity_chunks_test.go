package chunk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
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
	sources []string
	matches []string
	queries []*types.SearchRequest
}

func (e *entityChunksTestEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.queries = append(e.queries, req)
	if req.Filter["knowledge_graph_kwd"] == "entity" {
		return &types.SearchResult{Chunks: []map[string]interface{}{e.entity}}, nil
	}
	ids := e.sources
	if filterIDs, ok := req.Filter["id"].([]string); ok {
		ids = filterIDs
	}
	if len(req.MatchExprs) == 0 && req.OrderBy == nil {
		ids = ids[:min(len(ids), 128)] // An unsorted Infinity scan stops after its first batch.
	}
	if len(req.MatchExprs) > 0 {
		if e.matches != nil {
			ids = slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return !slices.Contains(e.matches, id) })
		}
		match := req.MatchExprs[0].(*types.MatchTextExpr)
		ids = ids[:min(len(ids), match.TopN)]
	}
	total := int64(len(ids))
	start := min(req.Offset, len(ids))
	end := min(start+req.Limit, len(ids))
	if req.Offset > 0 && start == len(ids) {
		total = 0 // Infinity loses total when Offset skips every candidate.
	}
	chunks := make([]map[string]interface{}, end-start)
	for i := range chunks {
		chunks[i] = map[string]interface{}{"id": ids[start+i]}
	}
	return &types.SearchResult{Chunks: chunks, Total: total}, nil
}

func TestListKeywordChunksCompletePagination(t *testing.T) {
	pushChunkTestDB(t, setupChunkTestDB(t))
	insertChunkTestUserTenant(t, "user-1", "tenant-1")
	insertChunkTestKB(t, "kb-1", "tenant-1")
	insertChunkTestDoc(t, "doc-1", "kb-1")
	setChunkTestStats(t, "doc-1", "kb-1", 0, 1, 0, 1) // Deliberately stale document count.
	ids := make([]string, 500)
	for i := range ids {
		ids[i] = fmt.Sprintf("%016x", i+1)
	}
	payload, _ := json.Marshal(map[string]interface{}{"source_chunk_ids": ids})
	for _, tt := range []struct {
		name, entityID string
		page           int
		sources        []string
	}{
		{"entity first page", "entity-1", 1, ids},
		{"entity last page", "entity-1", 5, ids},
		{"entity beyond last page", "entity-1", 6, ids},
		{"document count", "", 1, ids},
		{"empty document", "", 1, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			engine := &entityChunksTestEngine{
				entity: map[string]interface{}{
					"id": "entity-1", "kb_id": "kb-1", "doc_id": "doc-1", "type_kwd": "entity",
					"content_with_weight": string(payload),
				},
				sources: tt.sources,
				matches: ids[:450],
			}
			svc := &ChunkService{docEngine: engine, kbDAO: dao.NewKnowledgebaseDAO()}
			size, available := 100, 0
			response, err := svc.List(t.Context(), &service.ListChunksRequest{
				DatasetID: "kb-1", DocID: "doc-1", EntityID: tt.entityID, Page: &tt.page, Size: &size,
				Keywords: "invoice", AvailableInt: &available,
			}, "user-1")
			if err != nil {
				t.Fatal(err)
			}
			wantTotal := min(len(tt.sources), 450)
			start := min((tt.page-1)*size, wantTotal)
			end := min(start+size, wantTotal)
			if response.Total != int64(wantTotal) || len(response.Chunks) != end-start {
				t.Fatalf("total=%d chunks=%d, want %d/%d", response.Total, len(response.Chunks), wantTotal, end-start)
			}
			for i, chunk := range response.Chunks {
				if chunk["chunk_id"] != ids[start+i] {
					t.Errorf("chunk %d = %v, want %s", i, chunk["chunk_id"], ids[start+i])
				}
			}
			if wantTotal == 0 && (len(engine.queries) != 1 || len(engine.queries[0].MatchExprs) != 0) {
				t.Fatalf("empty scope queried keyword candidates: %#v", engine.queries)
			}
			for _, query := range engine.queries {
				if query.Filter["doc_id"] != "doc-1" || !reflect.DeepEqual(query.KbIDs, []string{"kb-1"}) {
					t.Fatalf("query escaped document/dataset: %#v", query)
				}
				if query.Filter["knowledge_graph_kwd"] != "entity" &&
					(query.Filter["available_int"] != 0 || query.Filter["must_not"].(map[string]interface{})["exists"] != "compile_kwd") {
					t.Errorf("query changed chunk filters: %#v", query.Filter)
				}
			}
		})
	}
}
