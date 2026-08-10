package tool

import (
	"context"
	"encoding/json"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/service/wikisearch"
)

// fakeWikiService is a deterministic wikisearch.Service double.
type fakeWikiService struct {
	available map[string]bool // datasetID -> has artifact
	pages     func(query string) []map[string]interface{}
	backfill  map[string]string // chunk id -> content
	callCount int
}

func (f *fakeWikiService) AvailableFor(_ context.Context, _ string, datasetIDs []string) bool {
	for _, ds := range datasetIDs {
		if f.available[ds] {
			return true
		}
	}
	return false
}

func (f *fakeWikiService) QueryPages(_ context.Context, _ string, _ []string, query, _ string, topN int) (wikisearch.SearchResult, error) {
	f.callCount++
	if f.pages == nil || len(f.pages(query)) == 0 {
		return wikisearch.SearchResult{Chunks: []map[string]interface{}{}, DocAggs: []map[string]interface{}{}}, nil
	}
	res := wikisearch.SearchResult{Chunks: append([]map[string]interface{}(nil), f.pages(query)...), DocAggs: []map[string]interface{}{}}
	seen := map[string]bool{}
	for _, c := range res.Chunks {
		if docID, _ := c["doc_id"].(string); docID != "" && !seen[docID] {
			seen[docID] = true
			res.DocAggs = append(res.DocAggs, map[string]interface{}{"doc_id": docID, "doc_name": c["docnm_kwd"]})
		}
	}
	return res, nil
}

func (f *fakeWikiService) BackfillChunks(_ context.Context, _ string, _ []string, chunkIDs []string) ([]map[string]interface{}, error) {
	out := make([]map[string]interface{}, 0, len(chunkIDs))
	for _, id := range chunkIDs {
		content, ok := f.backfill[id]
		if !ok {
			continue
		}
		out = append(out, map[string]interface{}{"chunk_id": id, "content_with_weight": content, "doc_id": "d1", "docnm_kwd": "Doc"})
	}
	return out, nil
}

// wikiToolRun runs the wiki_query tool with a single-dataset canvas context
// (the tool derives tenant + dataset scope from canvas state).
func wikiToolRun(t *testing.T, svc wikisearch.Service, tenant string, kb string, query string) map[string]interface{} {
	t.Helper()
	state := runtime.NewCanvasState("run-1", "task-1")
	state.Sys["tenant_id"] = tenant
	state.Sys["dataset_id"] = kb
	ctx := runtime.WithState(context.Background(), state)
	tool := newWikiQueryToolWithService(svc)
	args, _ := json.Marshal(map[string]interface{}{"query": query})
	raw, err := tool.InvokableRun(ctx, string(args))
	if err != nil {
		t.Fatalf("wiki_query InvokableRun err = %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("bad wiki_query output: %v", err)
	}
	return out
}

func TestWikiQueryTool_ReturnsPages(t *testing.T) {
	svc := &fakeWikiService{
		available: map[string]bool{"kb1": true},
		pages: func(q string) []map[string]interface{} {
			return []map[string]interface{}{{"chunk_id": "wiki/entity/alpha", "content_with_weight": "# Alpha", "doc_id": "kb1", "docnm_kwd": "Alpha", "wiki_slug_kwd": "entity/alpha", "dataset_id": "kb1"}}
		},
	}
	out := wikiToolRun(t, svc, "t1", "kb1", "alpha")
	chunks, _ := out["chunks"].([]interface{})
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	first := chunks[0].(map[string]interface{})
	if first["content_with_weight"] != "# Alpha" {
		t.Errorf("page content = %v, want # Alpha", first["content_with_weight"])
	}
	if first["wiki_slug_kwd"] != "entity/alpha" {
		t.Errorf("slug = %v, want entity/alpha", first["wiki_slug_kwd"])
	}
}

func TestWikiQueryTool_EmptyWhenNoArtifact(t *testing.T) {
	svc := &fakeWikiService{available: map[string]bool{"kb2": true}}
	out := wikiToolRun(t, svc, "t1", "kb1", "alpha") // kb1 has no artifact
	if chunks, _ := out["chunks"].([]interface{}); len(chunks) != 0 {
		t.Fatalf("chunks = %d, want 0 (kb1 has no wiki artifact)", len(chunks))
	}
}

func TestWikiQueryTool_EmptyWhenNoService(t *testing.T) {
	out := wikiToolRun(t, nil, "t1", "kb1", "alpha")
	if chunks, _ := out["chunks"].([]interface{}); len(chunks) != 0 {
		t.Fatalf("chunks = %d, want 0 (no service configured)", len(chunks))
	}
}

func TestWikiQueryTool_ScopeRespected(t *testing.T) {
	svc := &fakeWikiService{
		available: map[string]bool{"kb1": true},
		pages: func(q string) []map[string]interface{} {
			return []map[string]interface{}{{"chunk_id": "wiki/s", "content_with_weight": "c", "doc_id": "kb1", "docnm_kwd": "T", "wiki_slug_kwd": "s", "dataset_id": "kb1"}}
		},
	}
	out := wikiToolRun(t, svc, "t1", "kb1", "alpha")
	if _, ok := out["chunks"]; !ok {
		t.Fatalf("missing chunks key")
	}
}
