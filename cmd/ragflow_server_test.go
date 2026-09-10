package main

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func kb(embdID string, tenantEmbdID string) *entity.Knowledgebase {
	kb := &entity.Knowledgebase{EmbdID: embdID}
	if tenantEmbdID != "" {
		kb.TenantEmbdID = &tenantEmbdID
	}
	return kb
}

// TestHasEmbedderForMirrorsPython pins dialog_service.py:362 —
// `embd_mdl = LLMBundle(...) if kbs and kbs[0].embd_id else None`. The gate is
// the FIRST dataset's embd_id, so a dialog whose leading dataset has no model
// yields no embedding model even when a later one does.
func TestHasEmbedderForMirrorsPython(t *testing.T) {
	cases := []struct {
		name string
		kbs  []*entity.Knowledgebase
		want bool
	}{
		{"no datasets", nil, false},
		{"single dataset with model", []*entity.Knowledgebase{kb("bge-m3", "")}, true},
		{"single dataset without model", []*entity.Knowledgebase{kb("", "")}, false},
		// Python reads kbs[0].embd_id only; it does not scan for any dataset
		// carrying a model.
		{"first without, later with", []*entity.Knowledgebase{kb("", ""), kb("bge-m3", "")}, false},
		{"all with the same model", []*entity.Knowledgebase{kb("bge-m3", ""), kb("bge-m3", "")}, true},
	}
	for _, c := range cases {
		if got := hasEmbedderFor(c.kbs); got != c.want {
			t.Errorf("%s: hasEmbedderFor = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestValidateDatasetEmbeddingModels pins knowledgebase_service.py:62-94.
// Mixing is a hard error upstream (get_models raises at dialog_service.py:358) —
// NOT a silent fallback to keyword-only retrieval.
func TestValidateDatasetEmbeddingModels(t *testing.T) {
	t.Run("all without a model is allowed", func(t *testing.T) {
		if err := validateDatasetEmbeddingModels(context.Background(), []*entity.Knowledgebase{
			kb("", ""), kb("", ""),
		}); err != nil {
			t.Errorf("all-without must be allowed, got %v", err)
		}
	})

	t.Run("all same model is allowed", func(t *testing.T) {
		if err := validateDatasetEmbeddingModels(context.Background(), []*entity.Knowledgebase{
			kb("bge-m3", ""), kb("bge-m3", ""),
		}); err != nil {
			t.Errorf("same model must be allowed, got %v", err)
		}
	})

	t.Run("some with some without is an error", func(t *testing.T) {
		if err := validateDatasetEmbeddingModels(context.Background(), []*entity.Knowledgebase{
			kb("bge-m3", ""), kb("", ""),
		}); err == nil {
			t.Error("mixing present/absent embedding models must error (Python raises)")
		}
	})

	t.Run("different models is an error", func(t *testing.T) {
		if err := validateDatasetEmbeddingModels(context.Background(), []*entity.Knowledgebase{
			kb("bge-m3", ""), kb("text-embedding-3", ""),
		}); err == nil {
			t.Error("different embedding models must error (Python raises)")
		}
	})

	t.Run("dangling tenant_model id falls back to embd_id base name", func(t *testing.T) {
		// Python silently skips ids that no longer resolve
		// (tenant_model_service.py:557) and falls back to
		// _base_model_name(embd_id) — it must not fail the request.
		if err := validateDatasetEmbeddingModels(context.Background(), []*entity.Knowledgebase{
			kb("bge-m3@inst@prov", "gone-1"), kb("bge-m3@inst2@prov2", "gone-2"),
		}); err != nil {
			t.Errorf("two dangling refs with the same base name must be allowed, got %v", err)
		}
	})
}

// TestBaseModelNameMirrorsPython pins knowledgebase_service.py:33:
// rsplit("@", 2)[0] — the model name, with @instance@provider stripped.
func TestBaseModelNameMirrorsPython(t *testing.T) {
	cases := []struct{ in, want string }{
		{"bge-m3@inst@prov", "bge-m3"},
		{"bge-m3@inst", "bge-m3"},
		{"bge-m3", "bge-m3"},
		{"", ""},
	}
	for _, c := range cases {
		if got := baseModelName(c.in); got != c.want {
			t.Errorf("baseModelName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestHarnessWebSearcherNilAndWrapped pins the adapter: a nil pipeline callback
// must yield a nil harness.WebSearcher (which hides the web_search tool), and a
// non-nil one must be callable through the seam.
func TestHarnessWebSearcherNilAndWrapped(t *testing.T) {
	if got := harnessWebSearcher(nil); got != nil {
		t.Error("nil callback must yield a nil WebSearcher")
	}
	called := false
	searcher := harnessWebSearcher(func(_ context.Context, queries []string) ([]string, error) {
		called = true
		return queries, nil
	})
	if searcher == nil {
		t.Fatal("non-nil callback must yield a WebSearcher")
	}
	got, err := searcher.Search(context.Background(), []string{"q"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !called {
		t.Error("callback was not invoked")
	}
	if len(got) != 1 || got[0] != "q" {
		t.Errorf("Search = %v, want [q]", got)
	}
}

// errStubEncode stands in for a provider-side embedding failure.
var errStubEncode = errors.New("stub encode failure")

// recordingEngine captures the SearchRequest so the tests can pin the adapter's
// mapping without a live document engine.
type recordingEngine struct {
	last *types.SearchRequest
	res  *types.SearchResult
}

func (e *recordingEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.last = req
	if e.res != nil {
		return e.res, nil
	}
	return &types.SearchResult{}, nil
}

// TestEngineCompiledStoreSearchCompiledMapsRequest pins the mapping from the
// harness' compiled-row request onto the engine search: tenant index, kb scope,
// numeric available_int, the synthesis-page doc-scope column (source_doc_ids),
// and the keyword match leg over content_ltks/content_sm_ltks.
func TestEngineCompiledStoreSearchCompiledMapsRequest(t *testing.T) {
	eng := &recordingEngine{}
	// No embedder wired → the keyword leg stands alone.
	store := newEngineCompiledStore(eng, nil, "")
	_, err := store.SearchCompiled(context.Background(), "kb1", "t1", []string{"d1"}, map[string][]string{
		"compile_kwd":   {"wiki_page"},
		"available_int": {"1"},
	}, "culdcept", 7)
	if err != nil {
		t.Fatalf("SearchCompiled: %v", err)
	}
	req := eng.last
	if req == nil {
		t.Fatal("engine was not called")
	}
	if len(req.IndexNames) != 1 || req.IndexNames[0] != "ragflow_t1" {
		t.Errorf("IndexNames = %v, want [ragflow_t1]", req.IndexNames)
	}
	if len(req.KbIDs) != 1 || req.KbIDs[0] != "kb1" {
		t.Errorf("KbIDs = %v, want [kb1]", req.KbIDs)
	}
	if req.Limit != 7 {
		t.Errorf("Limit = %d, want 7", req.Limit)
	}
	// available_int is a numeric column: "1" must reach the engine as an int.
	if got, ok := req.Filter["available_int"].([]int); !ok || len(got) != 1 || got[0] != 1 {
		t.Errorf("available_int = %#v, want []int{1}", req.Filter["available_int"])
	}
	if got, ok := req.Filter["compile_kwd"].([]string); !ok || len(got) != 1 || got[0] != "wiki_page" {
		t.Errorf("compile_kwd = %#v, want []string{wiki_page}", req.Filter["compile_kwd"])
	}
	// Synthesis pages are scoped by source_doc_ids, not doc_id.
	if _, ok := req.Filter["source_doc_ids"]; !ok {
		t.Errorf("doc scope key = %v, want source_doc_ids present", req.Filter)
	}
	if _, ok := req.Filter["doc_id"]; ok {
		t.Errorf("synthesis rows must not use doc_id scope: %v", req.Filter)
	}
	if len(req.MatchExprs) != 1 {
		t.Fatalf("MatchExprs = %#v, want one keyword expr", req.MatchExprs)
	}
	mt, ok := req.MatchExprs[0].(*types.MatchTextExpr)
	if !ok {
		t.Fatalf("MatchExprs[0] = %T, want *types.MatchTextExpr", req.MatchExprs[0])
	}
	if mt.MatchingText != "culdcept" || mt.TopN != 7 {
		t.Errorf("keyword expr = %+v, want text=culdcept topN=7", mt)
	}
	if len(mt.Fields) != 2 || mt.Fields[0] != "content_ltks" || mt.Fields[1] != "content_sm_ltks" {
		t.Errorf("keyword fields = %v, want [content_ltks content_sm_ltks]", mt.Fields)
	}
}

// TestCompiledDocScopeKey pins Python's column choice: entity/relation rows use
// doc_id; the three synthesis kinds use source_doc_ids.
func TestCompiledDocScopeKey(t *testing.T) {
	cases := []struct {
		name    string
		filters map[string][]string
		want    string
	}{
		{"entity rows", map[string][]string{"knowledge_graph_kwd": {"entity"}}, "doc_id"},
		{"tree rows", map[string][]string{"knowledge_graph_kwd": {"entity"}, "compile_kwd": {"tree"}}, "doc_id"},
		{"wiki pages", map[string][]string{"compile_kwd": {"wiki_page"}}, "source_doc_ids"},
		{"artifact pages", map[string][]string{"compile_kwd": {"artifact_page"}}, "source_doc_ids"},
		{"essence", map[string][]string{"compile_kwd": {"essence"}}, "source_doc_ids"},
	}
	for _, c := range cases {
		if got := compiledDocScopeKey(c.filters); got != c.want {
			t.Errorf("%s: compiledDocScopeKey = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestEngineCompiledStoreLoadChunks pins the id-scoped fetch used to promote
// source chunks into evidence.
func TestEngineCompiledStoreLoadChunks(t *testing.T) {
	eng := &recordingEngine{res: &types.SearchResult{Chunks: []map[string]interface{}{{"id": "c1"}}}}
	store := newEngineCompiledStore(eng, nil, "")
	rows, err := store.LoadChunks(context.Background(), "kb1", "t1", []string{"c1", "c2"})
	if err != nil {
		t.Fatalf("LoadChunks: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if eng.last.Limit != 2 {
		t.Errorf("Limit = %d, want 2", eng.last.Limit)
	}
	ids, ok := eng.last.Filter["id"].([]string)
	if !ok || len(ids) != 2 || ids[0] != "c1" || ids[1] != "c2" {
		t.Errorf("id filter = %#v, want [c1 c2]", eng.last.Filter["id"])
	}
}

// TestNewDatasetCompiledStore pins the store wiring: no engine yields no store;
// datasets without an embedding model yield a keyword-only store; a dataset
// embedding model wires the query encoder with THAT dataset's model and owner
// tenant (not a tenant default or the request tenant).
func TestNewDatasetCompiledStore(t *testing.T) {
	eng := &recordingEngine{}
	if got := newDatasetCompiledStore(nil, nil, nil); got != nil {
		t.Error("nil engine must disable the compiled store")
	}
	if got := newDatasetCompiledStore(eng, nil, []*entity.Knowledgebase{{ID: "kb1", TenantID: "t1"}}); got == nil {
		t.Error("engine without an embedder must still build a keyword-only store")
	}
	store := newDatasetCompiledStore(eng, service.NewModelProviderService(),
		[]*entity.Knowledgebase{{ID: "kb1", EmbdID: "bge-m3@inst@prov", TenantID: "owner-tenant"}})
	es, ok := store.(*engineCompiledStore)
	if !ok {
		t.Fatalf("store = %T, want *engineCompiledStore", store)
	}
	if es.embed == nil {
		t.Fatal("the dataset's embedding model must wire the query encoder")
	}
	if es.embedTenantID != "owner-tenant" {
		t.Errorf("embedTenantID = %q, want owner-tenant (the dataset owner, not the request tenant)", es.embedTenantID)
	}
}

// stubQueryEncoder records the query-encoding calls the dense leg makes.
type stubQueryEncoder struct {
	vecs       [][]float32
	err        error
	calls      int
	lastTenant string
}

func (s *stubQueryEncoder) EncodeQueries(_ context.Context, tenantID string, _ []string) ([][]float32, error) {
	s.calls++
	s.lastTenant = tenantID
	return s.vecs, s.err
}

// TestEngineCompiledStoreUsesDenseSeedLeg pins the parity fix: when an embedder
// is wired, the compiled-row search goes out as a DENSE expr on the query
// vector's q_<dim>_vec column with Python's num_candidates floor and similarity
// threshold — and carries NO keyword expr alongside it (Python appends exactly
// one). The encoding runs under the embedder's OWN tenant (Python
// embd_owner_tenant_id = kbs[0].tenant_id), not the searched scope's tenant.
func TestEngineCompiledStoreUsesDenseSeedLeg(t *testing.T) {
	eng := &recordingEngine{}
	enc := &stubQueryEncoder{vecs: [][]float32{{1, 2, 3}}}
	// Scope tenant differs from the embedder's owner tenant on purpose.
	store := newEngineCompiledStore(eng, enc, "owner-tenant")

	if _, err := store.SearchCompiled(context.Background(), "kb1", "scope-tenant", nil,
		map[string][]string{"knowledge_graph_kwd": {"entity"}}, "culdcept", 5); err != nil {
		t.Fatalf("SearchCompiled: %v", err)
	}
	if enc.calls != 1 {
		t.Fatalf("encode calls = %d, want 1", enc.calls)
	}
	if enc.lastTenant != "owner-tenant" {
		t.Errorf("encode tenant = %q, want the embedder's owner tenant owner-tenant", enc.lastTenant)
	}
	if len(eng.last.MatchExprs) != 1 {
		t.Fatalf("MatchExprs = %#v, want exactly one dense expr", eng.last.MatchExprs)
	}
	dense, ok := eng.last.MatchExprs[0].(*types.MatchDenseExpr)
	if !ok {
		t.Fatalf("MatchExprs[0] = %T, want *types.MatchDenseExpr", eng.last.MatchExprs[0])
	}
	if dense.VectorColumnName != "q_3_vec" {
		t.Errorf("VectorColumnName = %q, want q_3_vec", dense.VectorColumnName)
	}
	if dense.EmbeddingDataType != "float" || dense.DistanceType != "cosine" || dense.TopN != 5 {
		t.Errorf("dense expr = %+v, want float/cosine/topN=5", dense)
	}
	if dense.ExtraOptions["similarity"] != 0.1 {
		t.Errorf("similarity = %v, want 0.1", dense.ExtraOptions["similarity"])
	}
	// max(topN, 256) — a small topN is floored to the HNSW candidate count.
	if dense.ExtraOptions["num_candidates"] != 256 {
		t.Errorf("num_candidates = %v, want 256 (max(5,256))", dense.ExtraOptions["num_candidates"])
	}

	// A topN above the floor is passed through unchanged.
	if _, err := store.SearchCompiled(context.Background(), "kb1", "scope-tenant", nil,
		map[string][]string{"knowledge_graph_kwd": {"entity"}}, "culdcept", 300); err != nil {
		t.Fatalf("SearchCompiled(300): %v", err)
	}
	dense300 := eng.last.MatchExprs[0].(*types.MatchDenseExpr)
	if dense300.ExtraOptions["num_candidates"] != 300 {
		t.Errorf("num_candidates = %v, want 300", dense300.ExtraOptions["num_candidates"])
	}
}

// TestEngineCompiledStoreDenseFallsBackToKeyword pins Python's `except Exception`
// around get_vector: an encode failure (or an empty vector) must degrade to the
// keyword leg rather than dropping the expansion.
func TestEngineCompiledStoreDenseFallsBackToKeyword(t *testing.T) {
	for _, c := range []struct {
		name string
		enc  *stubQueryEncoder
	}{
		{"encode error", &stubQueryEncoder{err: errStubEncode}},
		{"empty vector", &stubQueryEncoder{vecs: [][]float32{{}}}},
		{"no vectors", &stubQueryEncoder{vecs: nil}},
	} {
		t.Run(c.name, func(t *testing.T) {
			eng := &recordingEngine{}
			store := newEngineCompiledStore(eng, c.enc, "t1")
			if _, err := store.SearchCompiled(context.Background(), "kb1", "t1", nil,
				map[string][]string{"knowledge_graph_kwd": {"entity"}}, "culdcept", 5); err != nil {
				t.Fatalf("SearchCompiled: %v", err)
			}
			if c.enc.calls != 1 {
				t.Errorf("encode calls = %d, want 1", c.enc.calls)
			}
			if len(eng.last.MatchExprs) != 1 {
				t.Fatalf("MatchExprs = %#v, want one keyword expr", eng.last.MatchExprs)
			}
			mt, ok := eng.last.MatchExprs[0].(*types.MatchTextExpr)
			if !ok {
				t.Fatalf("MatchExprs[0] = %T, want *types.MatchTextExpr keyword fallback", eng.last.MatchExprs[0])
			}
			if mt.MatchingText != "culdcept" {
				t.Errorf("keyword text = %q, want culdcept", mt.MatchingText)
			}
		})
	}
}
