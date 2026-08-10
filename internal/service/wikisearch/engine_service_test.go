package wikisearch

import (
	"context"
	"reflect"
	"testing"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
)

// fakeDocEngine embeds engine.DocEngine (all methods nil) and overrides only the
// chunk-store existence probe, by-id chunk fetch, and search used by the service.
// It records the index/dataset params so tests can assert tenant-scoped index and
// dataset-scoped filters.
type fakeDocEngine struct {
	engine.DocEngine
	// existsDatasets lists dataset ids whose table exists; empty means all probe
	// results follow `exists` (when false, nothing exists).
	exists         bool
	existsDatasets map[string]bool
	chunks         map[string]interface{} // chunk id -> raw row (GetChunk)
	searchRows     []map[string]interface{}
	searchReq      *types.SearchRequest
	gotChunkArgs   [][3]interface{} // [indexName, chunkID, datasetIDs]
	existsArgs     [][2]string      // [indexName, datasetID]
}

func (f *fakeDocEngine) ChunkStoreExists(_ context.Context, indexName, datasetID string) (bool, error) {
	f.existsArgs = append(f.existsArgs, [2]string{indexName, datasetID})
	if f.existsDatasets != nil {
		return f.existsDatasets[datasetID], nil
	}
	return f.exists, nil
}

func (f *fakeDocEngine) GetChunk(_ context.Context, indexName, chunkID string, datasetIDs []string) (interface{}, error) {
	f.gotChunkArgs = append(f.gotChunkArgs, [3]interface{}{indexName, chunkID, datasetIDs})
	return f.chunks[chunkID], nil
}

func (f *fakeDocEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	f.searchReq = req
	rows := f.searchRows
	// Honor the compile_kwd filter so availability semantics are exercised
	// faithfully (only rows carrying the requested compile_kwd match).
	if kwd, ok := req.Filter["compile_kwd"].(string); ok {
		filtered := make([]map[string]interface{}, 0, len(rows))
		for _, r := range rows {
			if r["compile_kwd"] == kwd {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}
	return &types.SearchResult{Chunks: rows, Total: int64(len(rows))}, nil
}

// realEngineRow returns a wiki page row in the engine's actual shape: id
// (shimmed _id), kb_id (dataset), compile_kwd, doc_id, docnm_kwd, slug_kwd,
// source_chunk_ids.
func realEngineRow(id, kbID, docID, name, slug string, source []string) map[string]interface{} {
	row := map[string]interface{}{
		"id": id, "kb_id": kbID, "compile_kwd": "wiki_page",
		"doc_id": docID, "docnm_kwd": name,
		"content_with_weight": "content of " + id, "slug_kwd": slug,
	}
	if len(source) > 0 {
		row["source_chunk_ids"] = source
	}
	return row
}

func TestEngineService_QueryPages_NormalizesEngineRowShape(t *testing.T) {
	eng := &fakeDocEngine{searchRows: []map[string]interface{}{
		realEngineRow("wiki/alpha", "kb1", "d1", "Alpha", "entity/alpha", []string{"c1", "c2"}),
	}}
	svc := NewEngineService(eng)
	res, err := svc.QueryPages(context.Background(), "t1", []string{"kb1"}, "alpha", "", 5)
	if err != nil {
		t.Fatalf("QueryPages err = %v", err)
	}
	if len(res.Chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(res.Chunks))
	}
	// id -> chunk_id, kb_id -> dataset_id must be normalized (Finding 2).
	if res.Chunks[0]["chunk_id"] != "wiki/alpha" {
		t.Errorf("chunk_id = %v, want wiki/alpha (from engine id)", res.Chunks[0]["chunk_id"])
	}
	if res.Chunks[0]["dataset_id"] != "kb1" {
		t.Errorf("dataset_id = %v, want kb1 (from engine kb_id)", res.Chunks[0]["dataset_id"])
	}
	if res.Chunks[0]["wiki_slug_kwd"] != "entity/alpha" {
		t.Errorf("wiki_slug_kwd = %v, want entity/alpha", res.Chunks[0]["wiki_slug_kwd"])
	}
	src, ok := res.Chunks[0]["source_chunk_ids"].([]string)
	if !ok || len(src) != 2 {
		t.Fatalf("source_chunk_ids missing: %#v", res.Chunks[0])
	}
	// The engine Search must be scoped to the tenant index + dataset filter and
	// filtered to compiled wiki pages.
	if eng.searchReq == nil {
		t.Fatalf("no SearchRequest issued")
	}
	if !reflect.DeepEqual(eng.searchReq.IndexNames, []string{"ragflow_t1"}) {
		t.Errorf("IndexNames = %v, want [ragflow_t1]", eng.searchReq.IndexNames)
	}
	if !reflect.DeepEqual(eng.searchReq.KbIDs, []string{"kb1"}) {
		t.Errorf("KbIDs = %v, want [kb1]", eng.searchReq.KbIDs)
	}
	if f, ok := eng.searchReq.Filter["compile_kwd"].(string); !ok || f != "wiki_page" {
		t.Errorf("compile_kwd filter = %v, want wiki_page", eng.searchReq.Filter["compile_kwd"])
	}
	// SelectFields must project slug_kwd + source_chunk_ids (Infinity's default
	// projection omits them), otherwise the page slug and P7 provenance are lost.
	projected := map[string]bool{}
	for _, f := range eng.searchReq.SelectFields {
		projected[f] = true
	}
	for _, f := range []string{"id", "kb_id", "doc_id", "docnm_kwd", "content_with_weight", "slug_kwd", "source_chunk_ids"} {
		if !projected[f] {
			t.Errorf("SelectFields missing %q: %v", f, eng.searchReq.SelectFields)
		}
	}
}

// TestEngineService_QueryPages_ArrayShapedFields locks firstString handling of
// array-shaped engine keyword fields: the document engine returns slug_kwd /
// docnm_kwd as arrays, and those must not be dropped.
func TestEngineService_QueryPages_ArrayShapedFields(t *testing.T) {
	eng := &fakeDocEngine{searchRows: []map[string]interface{}{
		{
			"id": "wiki/alpha", "kb_id": "kb1", "compile_kwd": "wiki_page",
			"doc_id": "d1", "docnm_kwd": []string{"Alpha"},
			"content_with_weight": "content of wiki/alpha",
			"slug_kwd":            []string{"entity/alpha"},
			"source_chunk_ids":    []string{"c1"},
		},
	}}
	svc := NewEngineService(eng)
	res, err := svc.QueryPages(context.Background(), "t1", []string{"kb1"}, "alpha", "", 5)
	if err != nil {
		t.Fatalf("QueryPages err = %v", err)
	}
	if len(res.Chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(res.Chunks))
	}
	if res.Chunks[0]["wiki_slug_kwd"] != "entity/alpha" {
		t.Errorf("wiki_slug_kwd = %v, want entity/alpha (array-shaped slug_kwd)", res.Chunks[0]["wiki_slug_kwd"])
	}
	if res.Chunks[0]["docnm_kwd"] != "Alpha" {
		t.Errorf("docnm_kwd = %v, want Alpha (array-shaped docnm_kwd)", res.Chunks[0]["docnm_kwd"])
	}
	if len(res.DocAggs) != 1 || res.DocAggs[0]["doc_name"] != "Alpha" {
		t.Errorf("DocAggs doc_name = %v, want Alpha", res.DocAggs)
	}
}

func TestEngineService_QueryPages_DegradesEmpty(t *testing.T) {
	svc := NewEngineService(nil)
	res, err := svc.QueryPages(context.Background(), "t1", []string{"kb1"}, "alpha", "", 5)
	if err != nil || len(res.Chunks) != 0 {
		t.Fatalf("got chunks=%d err=%v, want empty/noerr (no engine)", len(res.Chunks), err)
	}
	svc2 := NewEngineService(&fakeDocEngine{searchRows: []map[string]interface{}{
		realEngineRow("x", "kb1", "d", "D", "s", nil),
	}})
	res2, _ := svc2.QueryPages(context.Background(), "t1", []string{"kb1"}, "  ", "", 5)
	if len(res2.Chunks) != 0 {
		t.Fatalf("chunks = %d, want 0 for blank query", len(res2.Chunks))
	}
}

func TestEngineService_BackfillChunks_ByIDScoped(t *testing.T) {
	eng := &fakeDocEngine{chunks: map[string]interface{}{
		"c1": map[string]interface{}{"id": "c1", "content_with_weight": "raw 1", "doc_id": "d1", "docnm_kwd": "D", "kb_id": "kb1"},
		"c2": map[string]interface{}{"id": "c2", "content_with_weight": "raw 2", "doc_id": "d1", "docnm_kwd": "D", "kb_id": "kb1"},
	}}
	svc := NewEngineService(eng)
	out, err := svc.BackfillChunks(context.Background(), "t1", []string{"kb1", "kb2"}, []string{"c1", "c2", "c1", "missing"})
	if err != nil {
		t.Fatalf("BackfillChunks err = %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("backfill = %d, want 2 (deduped c1,c2; missing skipped)", len(out))
	}
	if out[0]["chunk_id"] != "c1" || out[1]["chunk_id"] != "c2" {
		t.Errorf("backfill ids = %#v, want c1,c2", out)
	}
	// kb_id -> dataset_id must be normalized on evidence rows too (Low finding).
	if out[0]["dataset_id"] != "kb1" || out[1]["dataset_id"] != "kb1" {
		t.Errorf("backfill dataset_id = %v / %v, want kb1 for both (from engine kb_id)",
			out[0]["dataset_id"], out[1]["dataset_id"])
	}
	// Every GetChunk must be against the tenant index and the dataset scope.
	for _, args := range eng.gotChunkArgs {
		if args[0] != "ragflow_t1" {
			t.Errorf("GetChunk index = %v, want ragflow_t1", args[0])
		}
		ds, _ := args[2].([]string)
		if !reflect.DeepEqual(ds, []string{"kb1", "kb2"}) {
			t.Errorf("GetChunk datasetIDs = %v, want [kb1 kb2]", ds)
		}
	}
}

func TestEngineService_BackfillChunks_DegradesNoEngine(t *testing.T) {
	svc := NewEngineService(nil)
	out, err := svc.BackfillChunks(context.Background(), "t1", []string{"kb1"}, []string{"c1"})
	if err != nil || len(out) != 0 {
		t.Fatalf("got %d err=%v, want empty/noerr (no engine)", len(out), err)
	}
}

func TestEngineService_AvailableFor_BoundedExistenceSearch(t *testing.T) {
	// A KB carrying wiki pages => AvailableFor true, and the request must be a
	// bounded (Limit=1) search on the tenant index, filtered to
	// compile_kwd="wiki_page" and the dataset KBs.
	eng := &fakeDocEngine{searchRows: []map[string]interface{}{
		realEngineRow("wiki/p1", "kb1", "d1", "P", "p1", nil),
	}}
	svc := NewEngineService(eng)
	if !svc.AvailableFor(context.Background(), "t1", []string{"kb1"}) {
		t.Errorf("AvailableFor should be true when a wiki page row exists")
	}
	if eng.searchReq == nil {
		t.Fatalf("no existence Search issued")
	}
	if !reflect.DeepEqual(eng.searchReq.IndexNames, []string{"ragflow_t1"}) {
		t.Errorf("existence IndexNames = %v, want [ragflow_t1]", eng.searchReq.IndexNames)
	}
	if eng.searchReq.Limit != 1 {
		t.Errorf("existence Limit = %d, want 1 (bounded)", eng.searchReq.Limit)
	}
	if f, ok := eng.searchReq.Filter["compile_kwd"].(string); !ok || f != "wiki_page" {
		t.Errorf("existence compile_kwd filter = %v, want wiki_page", eng.searchReq.Filter["compile_kwd"])
	}
	if !reflect.DeepEqual(eng.searchReq.KbIDs, []string{"kb1"}) {
		t.Errorf("existence KbIDs = %v, want [kb1]", eng.searchReq.KbIDs)
	}
	// No engine / empty tenant / no datasets -> false.
	if NewEngineService(nil).AvailableFor(context.Background(), "t1", []string{"kb1"}) {
		t.Errorf("AvailableFor should be false with no engine")
	}
	if NewEngineService(eng).AvailableFor(context.Background(), "", []string{"kb1"}) {
		t.Errorf("AvailableFor should be false with empty tenant")
	}
	if NewEngineService(eng).AvailableFor(context.Background(), "t1", nil) {
		t.Errorf("AvailableFor should be false with no datasets")
	}
	// No wiki page rows (only ordinary chunks) -> false, even though a chunk
	// store exists. This is the "wiki pages exist, not just a table" gate.
	svcNoWiki := NewEngineService(&fakeDocEngine{searchRows: []map[string]interface{}{
		{"id": "c1", "kb_id": "kb1", "content_with_weight": "plain chunk"}, // no compile_kwd
	}})
	if svcNoWiki.AvailableFor(context.Background(), "t1", []string{"kb1"}) {
		t.Errorf("AvailableFor should be false for a KB with no wiki page rows")
	}
}

func TestTenantIndexName(t *testing.T) {
	if got := tenantIndexName("t1"); got != "ragflow_t1" {
		t.Errorf("tenantIndexName(t1) = %q, want ragflow_t1", got)
	}
}
