package knowledge_compiler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/service/nav"

	"gorm.io/gorm"
)

// mockChat answers the structure variant's three LLM call shapes under the
// Python-aligned items contract: node extraction (entity items), edge
// extraction (relation items constrained to Known Entities), and merge
// judging (Item A / Item B). It scans the prompt for the Greek-letter fixture
// universe (Alpha..Epsilon) so assertions hold regardless of batch packing.
type mockChat struct{}

var greekNames = []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon"}
var greekPairs = [][2]string{{"Alpha", "Beta"}, {"Beta", "Gamma"}, {"Gamma", "Delta"}, {"Delta", "Epsilon"}, {"Alpha", "Delta"}, {"Gamma", "Epsilon"}}

// graphParserConfig declares the entity/relation template shape so the
// structure variant compiles the hypergraph kind (mirrors a Python
// parser_config whose kind aliases "graph" -> hypergraph).
func graphParserConfig() map[string]any {
	return map[string]any{
		"kind": "graph",
		"entity": map[string]any{"fields": []any{
			map[string]any{"type": "project", "description": "a named project"},
		}},
		"relation": map[string]any{"fields": []any{
			map[string]any{"type": "linked", "description": "a link between projects"},
		}},
	}
}

var chunkIDRe = regexp.MustCompile(`\[CHUNK_ID: ([^\]]+)\]`)

func chunkIDsInPrompt(prompt string) []string {
	var ids []string
	for _, m := range chunkIDRe.FindAllStringSubmatch(prompt, -1) {
		ids = append(ids, m[1])
	}
	if len(ids) == 0 {
		ids = []string{"c1"}
	}
	return ids
}

func itemsJSON(items []map[string]any, chunkIDs []string) string {
	for _, item := range items {
		item["source_chunk_ids"] = chunkIDs
	}
	b, _ := json.Marshal(map[string]any{"items": items})
	return string(b)
}

func parseMergePayloads(prompt string) (map[string]any, map[string]any) {
	body := strings.TrimPrefix(prompt, "Item A (existing):\n")
	parts := strings.SplitN(body, "\n\nItem B (incoming):\n", 2)
	if len(parts) != 2 {
		return nil, nil
	}
	var a, b map[string]any
	if err := json.Unmarshal([]byte(parts[0]), &a); err != nil {
		return nil, nil
	}
	if err := json.Unmarshal([]byte(parts[1]), &b); err != nil {
		return nil, nil
	}
	return a, b
}

func (mockChat) Chat(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	if !req.JSONMode {
		// Non-JSON calls (mindmap outline, wiki prose paths) get plain text.
		return &common.ChatResponse{Content: "Outline of: " + firstWords(req.UserPrompt, 24)}, nil
	}
	switch {
	case strings.HasPrefix(req.UserPrompt, "Item A (existing):"):
		a, b := parseMergePayloads(req.UserPrompt)
		if a != nil && b != nil && a["name"] != nil && a["name"] == b["name"] {
			merged, _ := json.Marshal(a)
			return &common.ChatResponse{Content: fmt.Sprintf(`{"duplicated":true,"merged":%s}`, merged)}, nil
		}
		return &common.ChatResponse{Content: `{"duplicated":false,"merged":null}`}, nil
	case strings.Contains(req.SystemPrompt, "## Known Entities:"):
		var items []map[string]any
		for _, pr := range greekPairs {
			if strings.Contains(req.UserPrompt, pr[0]) && strings.Contains(req.UserPrompt, pr[1]) {
				items = append(items, map[string]any{"type": "linked", "source": pr[0], "target": pr[1], "description": pr[0] + " linked to " + pr[1]})
			}
		}
		return &common.ChatResponse{Content: itemsJSON(items, chunkIDsInPrompt(req.UserPrompt))}, nil
	default:
		var items []map[string]any
		for _, n := range greekNames {
			if strings.Contains(req.UserPrompt, n) {
				items = append(items, map[string]any{"type": "project", "name": n, "description": n + " project"})
			}
		}
		return &common.ChatResponse{Content: itemsJSON(items, chunkIDsInPrompt(req.UserPrompt))}, nil
	}
}

// proseChat returns generic, non-empty prose for the non-structure variants
// (wiki/tree/mindmap), which all just need a summary/outline text.
type proseChat struct{}

func (proseChat) Chat(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	if req.JSONMode {
		return &common.ChatResponse{Content: `{"ok":true}`}, nil
	}
	// Echo a deterministic prose reply derived from the prompt.
	return &common.ChatResponse{Content: "Summary of: " + firstWords(req.UserPrompt, 24)}, nil
}

func firstWords(s string, n int) string {
	fields := strings.Fields(s)
	if len(fields) > n {
		fields = fields[:n]
	}
	return strings.Join(fields, " ")
}

func installProseDeps(t *testing.T) {
	t.Helper()
	common.SetDepsResolver(func(tenantID, llmID, embeddingModel string) (common.Deps, error) {
		return common.Deps{Chat: proseChat{}, Embed: mockEmbedder{dim: 8}, TenantID: tenantID}, nil
	})
	t.Cleanup(func() { common.SetDepsResolver(nil) })
}

type mockEmbedder struct{ dim int }

func (m mockEmbedder) Dimensions() int { return m.dim }
func (m mockEmbedder) Encode(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = deterministicVec(t, m.dim)
	}
	return out, nil
}
func deterministicVec(s string, dim int) []float32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	seed := h.Sum32()
	v := make([]float32, dim)
	for j := 0; j < dim; j++ {
		v[j] = float32((seed>>uint(j*5))&0xFF) / 255.0
	}
	return v
}

func installMockDeps(t *testing.T) {
	t.Helper()
	common.SetDepsResolver(func(tenantID, llmID, embeddingModel string) (common.Deps, error) {
		return common.Deps{Chat: mockChat{}, Embed: mockEmbedder{dim: 8}, TenantID: tenantID}, nil
	})
	t.Cleanup(func() { common.SetDepsResolver(nil) })
}

func TestKnowledgeCompiler_Structure_EndToEnd(t *testing.T) {
	installMockDeps(t)
	installVariantTemplateResolver(t, "structure")

	c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{
		"compilation_template_id": "tpl-structure", "llm_id": "llm1", "embedding_model": "emb1",
	})
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent: %v", err)
	}

	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"chunks": []any{
			map[string]any{"id": "c1", "text": "Alpha is a Beta"},
			map[string]any{"id": "c2", "text": "Beta related to Gamma"},
		},
		"doc_id":        "d1",
		"tenant_id":     "t1",
		"dataset_id":    "ds1",
		"parser_config": graphParserConfig(),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	chunks, ok := out["chunks"].([]any)
	if !ok {
		t.Fatalf("chunks = %T, want []any", out["chunks"])
	}
	// 2 input chunks + 3 entities + 2 relations + 1 graph = 8 total (the
	// compiled knowledge units are merged into the upstream input chunks).
	if len(chunks) != 8 {
		t.Fatalf("len(chunks) = %d, want 8 (2 input + 3 entities + 2 relations + 1 graph)", len(chunks))
	}

	// Exactly one graph product, parseable to {entities, relations} (mirrors
	// Python's _struct_rebuild_graph_json).
	var graph map[string]any
	for _, c := range chunks {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if kind, _ := cm["kc_kind"].(string); kind == "graph" {
			if err := json.Unmarshal([]byte(cm["text"].(string)), &graph); err != nil {
				t.Fatalf("unmarshal graph: %v", err)
			}
		}
	}
	if graph == nil {
		t.Fatal("no graph chunk emitted")
	}
	entities, _ := graph["entities"].([]any)
	relations, _ := graph["relations"].([]any)
	if len(entities) != 3 {
		t.Fatalf("graph entities = %d, want 3 (Alpha/Beta/Gamma)", len(entities))
	}
	if len(relations) != 2 {
		t.Fatalf("graph relations = %d, want 2", len(relations))
	}
}

func TestKnowledgeCompiler_UnknownVariant(t *testing.T) {
	c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{"compilation_template_id": "nope"})
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent: %v", err)
	}
	_, err = c.Invoke(context.Background(), nil, map[string]any{"chunks": []any{}})
	if !errors.Is(err, common.ErrUnknownVariant) {
		t.Fatalf("err = %v, want ErrUnknownVariant", err)
	}
}

// TestKnowledgeCompiler_Datasetnav_NoVariant locks the removal of the standalone
// datasetnav variant: dataset navigation is NOT an independent compile kind in
// Python (it is a by-product written after tree/page_index compile via
// internal/service datasetnav), so "datasetnav"/"dataset_nav" must now fail as
// an unknown variant rather than silently compile nothing.
func TestKnowledgeCompiler_Datasetnav_NoVariant(t *testing.T) {
	for _, kind := range []string{"datasetnav", "dataset_nav"} {
		_, err := common.KindToVariant(kind)
		if !errors.Is(err, common.ErrUnknownVariant) {
			t.Fatalf("KindToVariant(%q) err = %v, want ErrUnknownVariant", kind, err)
		}
	}
}

// fakeNavSvc records the most recent UpsertDoc call so a test can assert the
// tree by-product hook feeds the nav service the root document summary.
type fakeNavSvc struct {
	mu      sync.Mutex
	called  bool
	summary string
	docID   string
	kbID    string
}

func (f *fakeNavSvc) UpsertDoc(_ context.Context, in nav.UpsertDocInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = true
	f.summary = in.Summary
	f.docID = in.DocID
	f.kbID = in.KbID
	return nil
}
func (f *fakeNavSvc) RemoveDoc(context.Context, string, string, string) error { return nil }
func (f *fakeNavSvc) Search(context.Context, string, string, string, []float32, int) ([]nav.NavHit, error) {
	return nil, nil
}
func (f *fakeNavSvc) ListClusters(context.Context, string, string, int, int) ([]nav.NavNode, int64, error) {
	return nil, 0, nil
}
func (f *fakeNavSvc) ListChildren(context.Context, string, string, string, int, int) ([]nav.NavNode, int64, error) {
	return nil, 0, nil
}

// TestKnowledgeCompiler_TreeNavByProduct asserts the tree compile-complete hook
// writes the dataset-nav by-product: after tree.Run produces a root product,
// upsertTreeNav calls NavService.UpsertDoc with the root summary and the
// doc/dataset context, and never aborts on failure.
func TestKnowledgeCompiler_TreeNavByProduct(t *testing.T) {
	fake := &fakeNavSvc{}
	nav.SetNavService(fake)
	t.Cleanup(func() { nav.SetNavService(nil) })

	deps := common.Deps{
		TenantID:  "t1",
		DatasetID: "kb1",
		Chat:      proseChat{},
		Embed:     mockEmbedder{dim: 8},
	}
	products := []common.Product{
		{Content: "section one body", Meta: map[string]any{"kind": "summary", "level": 0}},
		{Content: "overall doc theme root summary", Vector: []float32{0.1, 0.2}, Meta: map[string]any{"kind": "root", "level": -1}},
	}
	upsertTreeNav(context.Background(), deps, "d1", products)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if !fake.called {
		t.Fatal("upsertTreeNav did not call NavService.UpsertDoc")
	}
	if fake.docID != "d1" {
		t.Errorf("doc_id = %q, want d1", fake.docID)
	}
	if fake.kbID != "kb1" {
		t.Errorf("kb_id = %q, want kb1", fake.kbID)
	}
	if !strings.Contains(fake.summary, "overall doc theme root summary") {
		t.Errorf("summary = %q, want the tree root summary", fake.summary)
	}
}

// TestKnowledgeCompiler_TreeNavByProduct_NilServiceDoesNotAbort asserts the
// by-product hook tolerates a missing NavService without erroring (nav is a
// derived artifact; its absence must never abort compilation).
func TestKnowledgeCompiler_TreeNavByProduct_NilServiceDoesNotAbort(t *testing.T) {
	nav.SetNavService(nil)
	t.Cleanup(func() { nav.SetNavService(nil) })
	// Should not panic and must return normally.
	upsertTreeNav(context.Background(), common.Deps{}, "d1",
		[]common.Product{{Content: "x", Meta: map[string]any{"kind": "root"}}})
}

// TestKnowledgeCompiler_StructureNavByProduct asserts the structure/page_index
// by-product hook summarizes the graph product entities and feeds NavService.
func TestKnowledgeCompiler_StructureNavByProduct(t *testing.T) {
	fake := &fakeNavSvc{}
	nav.SetNavService(fake)
	t.Cleanup(func() { nav.SetNavService(nil) })

	graphJSON := `{"entities":[{"name":"Engine","description":"a propulsion device"},{"name":"Fuel","description":"combustion source"}]}`
	products := []common.Product{
		{Content: graphJSON, Vector: []float32{0.1, 0.2}, Meta: map[string]any{"kind": "graph", "compile_kwd": "page_index"}},
	}
	// The by-product embeds the SUMMARY (not the graph JSON vector), so an
	// embedder must be present.
	upsertStructureNav(context.Background(), common.Deps{TenantID: "t1", DatasetID: "kb1", Embed: mockEmbedder{dim: 8}}, "d1", products)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if !fake.called {
		t.Fatal("upsertStructureNav did not call NavService.UpsertDoc")
	}
	if fake.docID != "d1" {
		t.Errorf("doc_id = %q, want d1", fake.docID)
	}
	if !strings.Contains(fake.summary, "Engine: a propulsion device") {
		t.Errorf("summary = %q, want entity descriptions", fake.summary)
	}
}

// TestPageIndexSummary asserts entity descriptions are concatenated into a
// document summary.
func TestPageIndexSummary(t *testing.T) {
	got := pageIndexSummary(`{"entities":[{"name":"A","description":"one  thing"},{"name":"B","description":""}]}`)
	if !strings.Contains(got, "A: one thing") {
		t.Errorf("summary = %q, want entity A", got)
	}
	if strings.Contains(got, "B") {
		t.Errorf("summary should skip empty-description entity, got %q", got)
	}
}

func TestKnowledgeCompiler_Alias_Mindmap(t *testing.T) {
	installMockDeps(t)
	// "mind_map" is the deprecated alias for "mindmap"; both resolve to the
	// implemented mindmap variant and must run (not ErrUnknownVariant / stub).
	c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{
		"compilation_template_id": "mind_map", "llm_id": "llm1", "embedding_model": "emb1",
	})
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"chunks":    []any{map[string]any{"id": "c1", "text": "Alpha is a Beta"}},
		"doc_id":    "d1",
		"tenant_id": "t1",
	})
	if err != nil {
		t.Fatalf("Invoke mind_map: %v", err)
	}
	chunks, ok := out["chunks"].([]any)
	if !ok || len(chunks) == 0 {
		t.Fatalf("mind_map produced no chunks: %v", out["chunks"])
	}
}

// runVariant is a shared harness that builds a KnowledgeCompiler component for
// the given variant and invokes it over a couple of chunks, returning the merged
// chunk list (upstream input chunks + compiled knowledge-unit chunks).
func runVariant(t *testing.T, variant string, extra map[string]any) []map[string]any {
	t.Helper()
	installVariantTemplateResolver(t, variant)
	params := map[string]any{"compilation_template_id": "tpl-" + variant, "llm_id": "llm1", "embedding_model": "emb1"}
	for k, v := range extra {
		params[k] = v
	}
	c, err := NewKnowledgeCompilerComponent("Compiler", params)
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent(%s): %v", variant, err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"chunks": []any{
			map[string]any{"id": "c1", "text": "The quick brown fox jumps over the lazy dog near the river bank."},
			map[string]any{"id": "c2", "text": "A red fox and a lazy dog rest beside a calm river at dawn."},
			map[string]any{"id": "c3", "text": "Rivers flow through valleys carrying water from the mountains."},
		},
		"doc_id":    "d1",
		"tenant_id": "t1",
	})
	if err != nil {
		t.Fatalf("Invoke(%s): %v", variant, err)
	}
	raw, ok := out["chunks"].([]any)
	if !ok {
		t.Fatalf("Invoke(%s): chunks = %T", variant, out["chunks"])
	}
	if len(raw) == 0 {
		t.Fatalf("Invoke(%s): produced no chunks", variant)
	}
	// Every emitted chunk must carry an id; every compiled knowledge unit must
	// carry its embedding (q_<dim>_vec) and a compile_kwd discriminator.
	chunks := make([]map[string]any, 0, len(raw))
	seenCompiled := false
	for _, r := range raw {
		cm, ok := r.(map[string]any)
		if !ok {
			t.Fatalf("Invoke(%s): chunk not a map: %T", variant, r)
		}
		chunks = append(chunks, cm)
		if cm["id"] == nil || cm["id"] == "" {
			t.Fatalf("Invoke(%s): chunk missing id: %v", variant, cm)
		}
		if ck, _ := cm["compile_kwd"].(string); ck != "" {
			seenCompiled = true
			if _, ok := cm["q_8_vec"]; !ok {
				t.Fatalf("Invoke(%s): compiled chunk missing vector: %v", variant, cm)
			}
		}
	}
	if !seenCompiled {
		t.Fatalf("Invoke(%s): no compiled knowledge-unit chunks emitted", variant)
	}
	return chunks
}

func TestKnowledgeCompiler_Wiki_EndToEnd(t *testing.T) {
	installProseDeps(t)
	chunks := runVariant(t, "wiki", nil)
	// wiki produces a "page" chunk (kind stored as kc_kind).
	foundPage := false
	for _, c := range chunks {
		if kind, _ := c["kc_kind"].(string); kind == "page" {
			foundPage = true
		}
	}
	if !foundPage {
		t.Fatalf("wiki: no 'page' chunk; got %d chunks", len(chunks))
	}
}

func TestKnowledgeCompiler_Tree_EndToEnd(t *testing.T) {
	installProseDeps(t)
	// watershed (default tree_order): zero external clustering dependency.
	chunks := runVariant(t, "tree", nil)
	foundRoot := false
	for _, c := range chunks {
		if kind, _ := c["kc_kind"].(string); kind == "root" {
			foundRoot = true
		}
	}
	if !foundRoot {
		t.Fatalf("tree(default): no 'root' chunk; got %d chunks", len(chunks))
	}

	// A smaller tree_order (more, smaller clusters) must also run and still
	// produce a well-formed tree (root present, chunks non-empty).
	chunksCoarse := runVariant(t, "tree", map[string]any{"extra": map[string]any{"tree_order": 2}})
	if len(chunksCoarse) == 0 {
		t.Fatalf("tree(tree_order=2): produced no chunks")
	}
	foundRootCoarse := false
	for _, c := range chunksCoarse {
		if kind, _ := c["kc_kind"].(string); kind == "root" {
			foundRootCoarse = true
		}
	}
	if !foundRootCoarse {
		t.Fatalf("tree(tree_order=2): no 'root' chunk; got %d chunks", len(chunksCoarse))
	}
}

func TestKnowledgeCompiler_Mindmap_EndToEnd(t *testing.T) {
	installProseDeps(t)
	// The proseChat reply is flat text; parseOutline still yields a root + the
	// reply as a child node, so chunks are non-empty and parent-linked.
	chunks := runVariant(t, "mindmap", nil)
	// Root chunk must have empty parent_kwd and kind "root".
	var root map[string]any
	for _, c := range chunks {
		if kind, _ := c["kc_kind"].(string); kind == "root" {
			root = c
		}
	}
	if root == nil {
		t.Fatalf("mindmap: no root chunk")
	}
	if pid, _ := root["parent_kwd"].(string); pid != "" {
		t.Fatalf("mindmap: root parent_kwd = %q, want empty", pid)
	}
}

// TestKnowledgeCompiler_EmitsChunks verifies that after compiling, the
// component returns the knowledge units merged into the chunk stream (no
// separate products/writer seam). The output shape is the chunker's
// {output_format:"chunks", chunks:[...]}.
func TestKnowledgeCompiler_EmitsChunks(t *testing.T) {
	installMockDeps(t)
	installVariantTemplateResolver(t, "structure")

	c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{
		"compilation_template_id": "tpl-structure", "llm_id": "llm1", "embedding_model": "emb1",
	})
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"chunks": []any{
			map[string]any{"id": "c1", "text": "Alpha is a Beta"},
			map[string]any{"id": "c2", "text": "Beta related to Gamma"},
		},
		"doc_id":        "d1",
		"tenant_id":     "t1",
		"dataset_id":    "ds1",
		"parser_config": graphParserConfig(),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["output_format"].(string); got != "chunks" {
		t.Fatalf("output_format = %v, want chunks", out["output_format"])
	}
	// No legacy products/written keys remain in the output surface.
	if _, ok := out["products"]; ok {
		t.Fatalf("legacy 'products' key still present in output")
	}
	if _, ok := out["written"]; ok {
		t.Fatalf("legacy 'written' key still present in output")
	}
	compiled := 0
	for _, r := range out["chunks"].([]any) {
		cm := r.(map[string]any)
		// Structure rows carry the inferred compile kind (hypergraph here),
		// mirroring Python's per-row autotype stamp.
		if ck, _ := cm["compile_kwd"].(string); ck == "hypergraph" {
			compiled++
			if cm["id"] == nil || cm["id"] == "" {
				t.Fatal("compiled chunk missing id")
			}
			if _, ok := cm["q_8_vec"]; !ok {
				t.Fatal("compiled chunk missing vector")
			}
		}
	}
	if compiled != 6 {
		t.Fatalf("compiled structure chunks = %d, want 6 (3 entities + 2 relations + 1 graph)", compiled)
	}
}

// TestKnowledgeCompiler_TemplateIDsAndProvenance verifies that
// compilation_template_ids passed via the DSL params are stamped onto every
// compiled chunk, and that structure entity rows carry source_chunk_ids
// pointing back at their originating input chunks.
func TestKnowledgeCompiler_TemplateIDsAndProvenance(t *testing.T) {
	installMockDeps(t)
	installVariantTemplateResolver(t, "structure")

	c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{
		"compilation_template_id": "tpl-structure",
		"llm_id":                  "llm1",
		"embedding_model":         "emb1",
	})
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"chunks": []any{
			map[string]any{"id": "c1", "text": "Alpha is a Beta"},
			map[string]any{"id": "c2", "text": "Beta related to Gamma"},
		},
		"doc_id":        "d1",
		"tenant_id":     "t1",
		"parser_config": graphParserConfig(),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	for _, r := range out["chunks"].([]any) {
		cm := r.(map[string]any)
		if ck, _ := cm["compile_kwd"].(string); ck != "hypergraph" {
			continue
		}
		// Every compiled chunk must carry the resolved template id (one per
		// template; the component stamps the producing template's id).
		var tidsCount int
		switch tids := cm["compilation_template_ids"].(type) {
		case []any:
			tidsCount = len(tids)
		case []string:
			tidsCount = len(tids)
		}
		if tidsCount != 1 {
			t.Fatalf("compiled chunk %v: compilation_template_ids = %v, want 1 (the resolved template id)", cm["id"], cm["compilation_template_ids"])
		}
		// Entity rows must carry source_chunk_ids.
		if kg, _ := cm["knowledge_graph_kwd"].(string); kg == "entity" {
			var idsCount int
			switch ids := cm["source_chunk_ids"].(type) {
			case []any:
				idsCount = len(ids)
			case []string:
				idsCount = len(ids)
			}
			if idsCount == 0 {
				t.Fatalf("entity chunk %v: missing source_chunk_ids", cm["id"])
			}
		}
	}

}

// constEmbedder returns one fixed vector for every input text. It is used to
// make historical-dedup deterministic: every compiled product shares the same
// vector, so a historical candidate carrying that vector is a guaranteed
// near-duplicate.
type constEmbedder struct {
	dim int
	vec []float32
}

func (m constEmbedder) Dimensions() int { return m.dim }
func (m constEmbedder) Encode(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = m.vec
	}
	return out, nil
}

// TestKnowledgeCompiler_Tree_DegenerateNoInfiniteLoop is a regression for the
// High-1 bug: the tree builder could recurse forever when a re-cluster returned a single
// label covering all points. The default mockEmbedder emits non-negative
// vectors, so under the watershed default (tree_order=4, ratio 25) every adjacent pair
// has cosine >= 0 and a pathological input can collapse into one cluster; with
// more than one point the old code re-enqueued the identical
// work item at level+1 and hung. This test uses 6 chunks and asserts the run
// terminates with a well-formed root. (If the guard regresses, go test's timeout
// turns the hang into a failure.)
func TestKnowledgeCompiler_Tree_DegenerateNoInfiniteLoop(t *testing.T) {
	installProseDeps(t)
	installVariantTemplateResolver(t, "tree")
	c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{
		"compilation_template_id": "tpl-tree", "llm_id": "llm1", "embedding_model": "emb1",
		"extra": map[string]any{"tree_order": 4},
	})
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"chunks": []any{
			map[string]any{"id": "c1", "text": "alpha beta gamma delta epsilon zeta"},
			map[string]any{"id": "c2", "text": "one two three four five six"},
			map[string]any{"id": "c3", "text": "red green blue yellow white black"},
			map[string]any{"id": "c4", "text": "cat dog bird fish frog snake"},
			map[string]any{"id": "c5", "text": "sun moon star planet comet meteor"},
			map[string]any{"id": "c6", "text": "king queen prince duke earl count"},
		},
		"doc_id":    "d1",
		"tenant_id": "t1",
	})
	if err != nil {
		t.Fatalf("Invoke hung or errored (tree infinite recursion?): %v", err)
	}
	raw, ok := out["chunks"].([]any)
	if !ok {
		t.Fatalf("chunks = %T", out["chunks"])
	}
	foundRoot := false
	for _, r := range raw {
		if cm, ok := r.(map[string]any); ok {
			if k, _ := cm["kc_kind"].(string); k == "root" {
				foundRoot = true
			}
		}
	}
	if !foundRoot {
		t.Fatalf("tree(degenerate): no root chunk; got %d chunks", len(raw))
	}
}

// TestKnowledgeCompiler_Wiki_HistoricalDedupDropsDuplicates is a regression for
// the High-2 bug: EnableHistoricalDedup / the HistoricalCandidates override used
// to be telemetry-only dead code. Here a constant embedder makes every wiki
// product share one vector, and we supply that vector as a historical candidate;
// the run must drop the near-duplicate products so none survive in the output.
func TestKnowledgeCompiler_Wiki_HistoricalDedupDropsDuplicates(t *testing.T) {
	common.SetDepsResolver(func(tenantID, llmID, embeddingModel string) (common.Deps, error) {
		return common.Deps{
			Chat:     proseChat{},
			Embed:    constEmbedder{dim: 8, vec: []float32{1, 0, 0, 0, 0, 0, 0, 0}},
			TenantID: tenantID,
		}, nil
	})
	t.Cleanup(func() { common.SetDepsResolver(nil) })

	installVariantTemplateResolver(t, "wiki")
	c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{
		"compilation_template_id": "tpl-wiki", "llm_id": "llm1", "embedding_model": "emb1",
	})
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"chunks": []any{
			map[string]any{"id": "c1", "text": "The quick brown fox jumps over the lazy dog."},
			map[string]any{"id": "c2", "text": "A red fox and a lazy dog rest beside a calm river."},
		},
		"doc_id":    "d1",
		"tenant_id": "t1",
		// Offline/test override: a historical candidate with the same vector as
		// every compiled product -> all are near-duplicates and must be dropped.
		"historical_candidates": []common.Candidate{
			{ID: "old-artifact", Vector: []float32{1, 0, 0, 0, 0, 0, 0, 0}},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	raw, ok := out["chunks"].([]any)
	if !ok {
		t.Fatalf("chunks = %T", out["chunks"])
	}
	for _, r := range raw {
		cm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if _, has := cm["compile_kwd"].(string); has {
			t.Fatalf("wiki historical dedup failed: a compiled chunk survived despite a matching historical candidate: %v", cm)
		}
	}
}

func TestKnowledgeCompiler_Wiki_UpdateMergesExistingPage(t *testing.T) {
	common.SetDepsResolver(func(tenantID, llmID, embeddingModel string) (common.Deps, error) {
		return common.Deps{
			Chat:     wikiUpdateChat{},
			Embed:    mockEmbedder{dim: 8},
			TenantID: tenantID,
			WikiPages: wikiStoreTestStub{page: &common.WikiPageCandidate{
				ID:           "existing-1",
				Slug:         "entity/alpha",
				Title:        "Alpha",
				PageType:     "entity",
				Topic:        "Alpha",
				ContentMD:    "# Alpha\n\nExisting fact.\n",
				ContentMDRaw: "# Alpha\n\nExisting fact.\n",
				EntityNames:  []string{"Alpha"},
				Score:        0.85,
			}},
		}, nil
	})
	t.Cleanup(func() { common.SetDepsResolver(nil) })

	installVariantTemplateResolver(t, "wiki")
	c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{
		"compilation_template_id": "tpl-wiki", "llm_id": "llm1", "embedding_model": "emb1",
	})
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"chunks": []any{
			map[string]any{"id": "c1", "text": "Alpha launched a new process in 2026."},
		},
		"doc_id":     "d1",
		"dataset_id": "ds1",
		"tenant_id":  "t1",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	raw, ok := out["chunks"].([]any)
	if !ok {
		t.Fatalf("chunks = %T", out["chunks"])
	}
	var page map[string]any
	for _, r := range raw {
		cm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if cm["compile_kwd"] == "wiki_page" && cm["kc_kind"] == "page" && cm["slug_kwd"] == "entity/alpha" {
			page = cm
			break
		}
	}
	if page == nil {
		t.Fatalf("no merged wiki page found in output: %#v", raw)
	}
	text, _ := page["text"].(string)
	if !strings.Contains(text, "Existing fact.") || !strings.Contains(text, "Alpha launched a new process in 2026.") {
		t.Fatalf("merged page text = %q, want both existing and incoming facts", text)
	}
	if outlinks, ok := page["outlinks_kwd"].([]any); ok && len(outlinks) == 0 {
		t.Fatalf("outlinks_kwd should include rewritten see-also link: %#v", page)
	}
}

// fakeHistoricalKNN records the datasetID it was queried with and optionally
// returns a hit for every lookup, so a test can assert both the scope of the
// KNN query and that near-duplicate products are dropped.
type fakeHistoricalKNN struct {
	mu     sync.Mutex
	lastDS string
	hit    bool
}

func (f *fakeHistoricalKNN) TopKHistory(_ context.Context, _ string, datasetID, _ string, _ []float32, _ int, _ float64) ([]common.HistoricalHit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastDS = datasetID
	if f.hit {
		return []common.HistoricalHit{{ID: "old", Score: 1.0}}, nil
	}
	return nil, nil
}

// TestKnowledgeCompiler_Wiki_HistoricalDedupScopedByDataset is a regression for
// the Medium bug: the historical KNN lookup used to be scoped by doc_id, so
// cross-document dedup within the same dataset did not work. This test supplies
// both a doc_id ("d1") and a dataset_id ("ds1") and asserts the lookup is
// scoped to the dataset, not the document.
func TestKnowledgeCompiler_Wiki_HistoricalDedupScopedByDataset(t *testing.T) {
	knn := &fakeHistoricalKNN{hit: true} // every product is a near-dup -> dropped
	common.SetDepsResolver(func(tenantID, llmID, embeddingModel string) (common.Deps, error) {
		return common.Deps{
			Chat:          proseChat{},
			Embed:         constEmbedder{dim: 8, vec: []float32{1, 0, 0, 0, 0, 0, 0, 0}},
			TenantID:      tenantID,
			HistoricalKNN: knn,
		}, nil
	})
	t.Cleanup(func() { common.SetDepsResolver(nil) })

	installVariantTemplateResolver(t, "wiki")
	c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{
		"compilation_template_id": "tpl-wiki", "llm_id": "llm1", "embedding_model": "emb1",
		"enable_historical_dedup": true,
	})
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"chunks": []any{
			map[string]any{"id": "c1", "text": "The quick brown fox jumps over the lazy dog."},
			map[string]any{"id": "c2", "text": "A red fox and a lazy dog rest beside a calm river."},
		},
		"doc_id":     "d1",
		"dataset_id": "ds1",
		"tenant_id":  "t1",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	knn.mu.Lock()
	ds := knn.lastDS
	knn.mu.Unlock()
	if ds != "ds1" {
		t.Fatalf("historical KNN queried with datasetID=%q, want %q (lookup must be scoped to the dataset, not the document)", ds, "ds1")
	}
	// With hit=true the near-duplicate products must be dropped, so no compiled
	// chunks survive.
	raw, ok := out["chunks"].([]any)
	if !ok {
		t.Fatalf("chunks = %T", out["chunks"])
	}
	for _, r := range raw {
		if cm, ok := r.(map[string]any); ok {
			if _, has := cm["compile_kwd"]; has {
				t.Fatalf("wiki historical dedup failed: a compiled chunk survived despite a matching historical hit: %v", cm)
			}
		}
	}
}

// fencedChat wraps otherwise-valid extraction JSON in a ```json ... ``` fence,
// the most common way LLMs wrap JSON even when JSONMode is requested.
type fencedChat struct{}

func (fencedChat) Chat(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	return &common.ChatResponse{Content: "```json\n" +
		`{"items":[{"type":"project","name":"Alpha","description":"Alpha project","source_chunk_ids":["c1"]}]}` +
		"\n```"}, nil
}

// proseOnlyChat returns genuine prose with no JSON at all — a model that
// ignored the JSON contract entirely.
type proseOnlyChat struct{}

func (proseOnlyChat) Chat(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	return &common.ChatResponse{Content: "I could not extract structured facts from this passage."}, nil
}

type wikiUpdateChat struct{}

func (wikiUpdateChat) Chat(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	switch {
	case req.JSONMode && strings.Contains(req.UserPrompt, "Extract all knowledge"):
		return &common.ChatResponse{Content: `{
  "entities": [{"name":"Alpha","type":"person","aliases":["A"],"source_chunk_id":"c1"}],
  "concepts": [{"term":"Alpha Protocol","definition_excerpt":"Alpha Protocol is a rollout process","source_chunk_id":"c1"}],
  "claims": [{"statement":"Alpha launched a new process in 2026.","subject":"Alpha","confidence":"explicit","source_chunk_id":"c1"}],
  "relations": [],
  "topics": ["Alpha"]
}`}, nil
	case req.JSONMode && strings.Contains(req.UserPrompt, "Return a JSON compilation plan"):
		return &common.ChatResponse{Content: `{
  "pages":[{"action":"CREATE","slug":"entity/alpha","title":"Alpha","page_type":"entity","topic":"Alpha","entity_names":["Alpha"],"related_kb_pages":[],"priority":1,"lead":"Alpha overview","sections":[{"heading":"Overview","points":["Alpha overview"]}]}],
  "estimated_page_count":1,
  "compilation_notes":"ok"
}`}, nil
	case strings.Contains(req.SystemPrompt, "wiki page merger"):
		return &common.ChatResponse{Content: "# Alpha\n\nExisting fact.\n\nAlpha launched a new process in 2026.\n\n## See also\n\n[Alpha Protocol](artifact/ds1/concept/alpha-protocol)"}, nil
	case strings.Contains(req.UserPrompt, "Existing page content"):
		return &common.ChatResponse{Content: "# Alpha\n\nAlpha launched a new process in 2026.\n\n## See also\n\n[[concept/alpha-protocol|Alpha Protocol]]"}, nil
	default:
		return &common.ChatResponse{Content: `{"action":"UPDATE","slug":"entity/alpha","reason":"same entity"}`}, nil
	}
}

type wikiStoreTestStub struct {
	page *common.WikiPageCandidate
}

func (s wikiStoreTestStub) FindSimilarPages(_ context.Context, _, _ string, _ []float32, _ int) ([]common.WikiPageCandidate, error) {
	if s.page == nil {
		return nil, nil
	}
	return []common.WikiPageCandidate{*s.page}, nil
}

func (s wikiStoreTestStub) GetPageBySlug(_ context.Context, _, _, slug string) (*common.WikiPageCandidate, error) {
	if s.page != nil && s.page.Slug == slug {
		return s.page, nil
	}
	return nil, nil
}

// TestKnowledgeCompiler_Structure_FencedJSONNotDropped is a regression for the
// Medium bug: GenJSON used to return {"_raw":...} on any parse failure, which
// parseExtractResult silently treated as "empty extraction", dropping every
// entity/relation. A fenced ```json ... ``` reply is now unwrapped and parsed,
// so the extraction still yields its entities.
func TestKnowledgeCompiler_Structure_FencedJSONNotDropped(t *testing.T) {
	common.SetDepsResolver(func(tenantID, llmID, embeddingModel string) (common.Deps, error) {
		return common.Deps{Chat: fencedChat{}, Embed: mockEmbedder{dim: 8}, TenantID: tenantID}, nil
	})
	t.Cleanup(func() { common.SetDepsResolver(nil) })

	installVariantTemplateResolver(t, "structure")
	c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{
		"compilation_template_id": "tpl-structure", "llm_id": "llm1", "embedding_model": "emb1",
	})
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"chunks":    []any{map[string]any{"id": "c1", "text": "Alpha is a Beta"}},
		"doc_id":    "d1",
		"tenant_id": "t1",
	})
	if err != nil {
		t.Fatalf("Invoke (fenced JSON should parse): %v", err)
	}
	var sawAlphaEntity bool
	for _, r := range out["chunks"].([]any) {
		cm := r.(map[string]any)
		if cm["compile_kwd"] == nil {
			continue
		}
		// name_kwd is lowercased (mirrors Python's _struct_to_doc_storage_doc).
		if cm["name_kwd"] == "alpha" {
			sawAlphaEntity = true
		}
	}
	if !sawAlphaEntity {
		t.Fatalf("fenced-JSON reply was dropped (no Alpha entity extracted); GenJSON must unwrap the fence, not treat it as empty extraction")
	}
}

// TestKnowledgeCompiler_Structure_MalformedJSONFailsLoud is the companion
// regression: when the reply is genuinely unparseable (not just fenced), the
// component must fail loudly instead of silently emitting zero knowledge units.
func TestKnowledgeCompiler_Structure_MalformedJSONFailsLoud(t *testing.T) {
	common.SetDepsResolver(func(tenantID, llmID, embeddingModel string) (common.Deps, error) {
		return common.Deps{Chat: proseOnlyChat{}, Embed: mockEmbedder{dim: 8}, TenantID: tenantID}, nil
	})
	t.Cleanup(func() { common.SetDepsResolver(nil) })

	installVariantTemplateResolver(t, "structure")
	c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{
		"compilation_template_id": "tpl-structure", "llm_id": "llm1", "embedding_model": "emb1",
	})
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent: %v", err)
	}
	_, err = c.Invoke(context.Background(), nil, map[string]any{
		"chunks":    []any{map[string]any{"id": "c1", "text": "Alpha is a Beta"}},
		"doc_id":    "d1",
		"tenant_id": "t1",
	})
	if err == nil {
		t.Fatalf("Invoke succeeded on an unparseable LLM reply; it must fail loudly rather than silently drop the extraction")
	}
}

// TestKnowledgeCompiler_PassThroughEnvelope is a regression for the Medium bug:
// the advertised output schema promises name / tenant_id / kb_id are carried
// forward from upstream (pass-through), but mergeChunks only emitted
// output_format + chunks. Headless / manual chaining reads those identity keys
// from the component output (the Tokenizer falls back to globals only in a full
// pipeline), so they must be forwarded when present.
func TestKnowledgeCompiler_PassThroughEnvelope(t *testing.T) {
	installMockDeps(t)
	installVariantTemplateResolver(t, "structure")

	c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{
		"compilation_template_id": "tpl-structure", "llm_id": "llm1", "embedding_model": "emb1",
	})
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"chunks":    []any{map[string]any{"id": "c1", "text": "Alpha is a Beta"}},
		"doc_id":    "d1",
		"tenant_id": "t1",
		"name":      "doc.pdf",
		"kb_id":     "kb9",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out["name"] != "doc.pdf" {
		t.Fatalf("output missing pass-through name: %v", out["name"])
	}
	if out["tenant_id"] != "t1" {
		t.Fatalf("output missing pass-through tenant_id: %v", out["tenant_id"])
	}
	if out["kb_id"] != "kb9" {
		t.Fatalf("output missing pass-through kb_id: %v", out["kb_id"])
	}
	if out["output_format"] != "chunks" {
		t.Fatalf("output_format = %v, want chunks", out["output_format"])
	}
	// Absent keys must NOT be invented.
	if _, ok := out["dataset_id"]; ok {
		t.Fatalf("output must not invent absent pass-through keys")
	}
}

// groupResolverStub maps every requested group id to a fixed pair of template
// ids, standing in for the production DB-backed group service.
func groupResolverStub(_ context.Context, _ *gorm.DB, _ string, groupIDs []string) ([]string, error) {
	var out []string
	for _, g := range groupIDs {
		out = append(out, "tpl-"+g+"-a", "tpl-"+g+"-b")
	}
	return out, nil
}

// TestKnowledgeCompiler_GroupIDsResolvedToTemplateIDs is a regression: a
// compilation_template_group_id is resolved via the installed GroupResolver to
// its concrete template ids, and each template is compiled as its own spec and
// stamped with its producing template id. Group-only configs must not silently
// miss compilation_template_ids.
func TestKnowledgeCompiler_GroupIDsResolvedToTemplateIDs(t *testing.T) {
	installMockDeps(t)
	// The group resolves to two concrete template ids; each is compiled as its
	// own spec, so the TemplateResolver must return a valid kind for them.
	// Override the package stub and restore it afterward.
	common.SetTemplateResolver(func(ctx context.Context, db *gorm.DB, tenantID, templateID string) (common.TemplateInfo, error) {
		kind := templateID
		if strings.HasPrefix(templateID, "tpl-grp1") {
			kind = "structure"
		}
		return common.TemplateInfo{ID: templateID, Kind: kind, Config: map[string]any{}}, nil
	})
	t.Cleanup(func() { common.SetTemplateResolver(testTemplateResolver) })

	common.SetGroupResolver(groupResolverStub)
	t.Cleanup(func() { common.SetGroupResolver(testGroupResolver) })

	// compilation_template_group_id (not the obsolete plural list) selects the
	// group; compilation_template_id is absent so the group path is taken.
	c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{
		"compilation_template_group_id": "grp1",
		"llm_id":                        "llm1",
		"embedding_model":               "emb1",
	})
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"chunks":    []any{map[string]any{"id": "c1", "text": "Alpha is a Beta"}},
		"doc_id":    "d1",
		"tenant_id": "t1",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	want := map[string]bool{"tpl-grp1-a": true, "tpl-grp1-b": true}
	seen := map[string]bool{}
	checked := 0
	for _, r := range out["chunks"].([]any) {
		cm := r.(map[string]any)
		// No parser_config is supplied, so InferType returns "list" and the
		// structure variant stamps compile_kwd="list" (not "structure").
		if cm["compile_kwd"] != "list" {
			continue
		}
		checked++
		var got []string
		switch v := cm["compilation_template_ids"].(type) {
		case []any:
			for _, e := range v {
				got = append(got, e.(string))
			}
		case []string:
			got = v
		}
		// Each product is stamped with the single template id that produced it.
		if len(got) != 1 || !want[got[0]] {
			t.Fatalf("compiled chunk %v: compilation_template_ids = %v, want exactly one of %v", cm["id"], got, want)
		}
		seen[got[0]] = true
	}
	if checked == 0 {
		t.Fatalf("no compiled chunks inspected (compile_kwd=list expected); assertion was vacuous")
	}
	// Both resolved template ids must appear across the products.
	for id := range want {
		if !seen[id] {
			t.Fatalf("resolved template id %q was never stamped on any product", id)
		}
	}
}

// TestKnowledgeCompiler_GroupIDsWithoutResolverFailsLoud is the companion
// regression: a group-only config with no GroupResolver installed must fail
// loudly (surfacing the misconfiguration) rather than silently emitting rows
// that miss compilation_template_ids.
func TestKnowledgeCompiler_GroupIDsWithoutResolverFailsLoud(t *testing.T) {
	installMockDeps(t)
	common.SetGroupResolver(nil) // ensure no resolver is installed

	c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{
		"compilation_template_group_id": "grp1",
		"llm_id":                        "llm1",
		"embedding_model":               "emb1",
	})
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent: %v", err)
	}
	_, err = c.Invoke(context.Background(), nil, map[string]any{
		"chunks":    []any{map[string]any{"id": "c1", "text": "Alpha is a Beta"}},
		"doc_id":    "d1",
		"tenant_id": "t1",
	})
	if err == nil {
		t.Fatalf("Invoke succeeded with group ids but no GroupResolver; it must fail loudly instead of dropping compilation_template_ids")
	}
	if !strings.Contains(err.Error(), "GroupResolver") {
		t.Fatalf("error %q does not mention GroupResolver", err.Error())
	}
}

// testTemplateResolver / testGroupResolver are the package-wide stub resolvers
// installed by TestMain. The stub maps the template id 1:1 onto the template
// kind, which lets tests resolve arbitrary synthetic ids (e.g. "tpl-structure")
// to a kind. Tests must pass a TEMPLATE ID, never a variant name, as
// compilation_template_id; use installVariantTemplateResolver(t, variant) to
// register a synthetic "tpl-<variant>" id that maps to the desired kind.
// Production wiring installs the real DB-backed resolvers (see
// internal/ingestion/task/knowledge_compiler_wiring.go).
var testTemplateResolver common.TemplateResolver = func(ctx context.Context, db *gorm.DB, tenantID, templateID string) (common.TemplateInfo, error) {
	return common.TemplateInfo{ID: templateID, Kind: templateID, Config: map[string]any{}}, nil
}

// installVariantTemplateResolver wires a template resolver so the synthetic
// template id "tpl-<variant>" resolves to Kind <variant>. This lets a test
// select a compiler variant through the production id -> kind -> variant path
// (resolveTemplateSpecs -> KindToVariant) instead of misusing the variant name
// as the compilation_template_id. Any other template id is delegated to the
// package default; the default is restored on cleanup.
func installVariantTemplateResolver(t *testing.T, variant string) {
	t.Helper()
	prev := testTemplateResolver
	common.SetTemplateResolver(func(ctx context.Context, db *gorm.DB, tenantID, templateID string) (common.TemplateInfo, error) {
		if templateID == "tpl-"+variant {
			return common.TemplateInfo{ID: templateID, Kind: variant, Config: map[string]any{}}, nil
		}
		return prev(ctx, db, tenantID, templateID)
	})
	t.Cleanup(func() { common.SetTemplateResolver(testTemplateResolver) })
}

var testGroupResolver common.GroupResolver = func(ctx context.Context, db *gorm.DB, tenantID string, groupIDs []string) ([]string, error) {
	return groupIDs, nil
}

// TestKnowledgeCompiler_RegistryResolvesUnifiedName locks the unified-name
// contract: the knowledge-compilation node is registered under "Compiler"
// (matching the Python side rag/flow/compiler/compiler.py component_name), so
// both a Python-saved canvas and Go's built-in ingestion templates resolve to
// the same KnowledgeCompilerComponent through runtime.DefaultRegistry.
func TestKnowledgeCompiler_RegistryResolvesUnifiedName(t *testing.T) {
	factory, category, _, ok := runtime.DefaultRegistry.Lookup("Compiler")
	if !ok {
		t.Fatal("runtime registry has no component \"Compiler\"; the Python canvas and Go templates both use this name")
	}
	if category != runtime.CategoryIngestion {
		t.Fatalf("component \"Compiler\" category = %q, want %q", category, runtime.CategoryIngestion)
	}
	c, err := factory("Compiler", map[string]any{"compilation_template_id": "tree", "llm_id": "llm1", "embedding_model": "emb1"})
	if err != nil {
		t.Fatalf("factory(\"Compiler\"): %v", err)
	}
	if _, ok := c.(*KnowledgeCompilerComponent); !ok {
		t.Fatalf("factory(\"Compiler\") produced %T, want *KnowledgeCompilerComponent", c)
	}
}

// TestKnowledgeCompiler_TenantFromGlobals locks the tenant-resolution contract:
// in the production canvas run the run-level tenant_id lives in the shared
// CanvasState.Globals bag (seeded by the pipeline at run start), not necessarily
// in the KnowledgeCompiler's own input map. The component must resolve it through
// globals.GlobalOrInput; otherwise the template/group lookup gets an empty tenant
// and fails with "compilation_template_group ... not found for tenant".
func TestKnowledgeCompiler_TenantFromGlobals(t *testing.T) {
	// Attach a CanvasState to the context and seed the run-level tenant id into
	// the global bag, as the pipeline does at run start.
	ctx := runtime.WithState(context.Background(), runtime.NewCanvasState("run-id", "sess-id"))
	globals.SeedIngestionGlobals(ctx, map[string]any{"tenant_id": "tenant-from-globals"})

	// Install a template resolver that records the tenant id it is called with.
	var gotTenant string
	prev := testTemplateResolver
	common.SetTemplateResolver(func(ctx context.Context, db *gorm.DB, tenantID, templateID string) (common.TemplateInfo, error) {
		gotTenant = tenantID
		return common.TemplateInfo{ID: templateID, Kind: "structure", Config: map[string]any{}}, nil
	})
	t.Cleanup(func() { common.SetTemplateResolver(prev) })

	c, err := NewKnowledgeCompilerComponent("Compiler", map[string]any{"compilation_template_id": "tpl-x"})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}

	// Invoke without tenant_id in the input map; the tenant must come from the
	// global bag. The template resolution (and thus gotTenant) happens early in
	// Invoke, before the variant's LLM/embedding deps are exercised — which is
	// all this test needs to assert.
	_, _ = c.Invoke(ctx, nil, map[string]any{
		"llm_id":          "llm1",
		"chunks":          []any{map[string]any{"id": "c1", "content_with_weight": "alpha beta", "text": "alpha beta"}},
		"embedding_model": "emb1",
	})
	if gotTenant != "tenant-from-globals" {
		t.Fatalf("template resolver saw tenant %q, want %q (tenant_id must be read from CanvasState.Globals)", gotTenant, "tenant-from-globals")
	}
}

// TestKnowledgeCompiler_BuildInputsAcceptsMapSliceChunks locks the chunk-carrier
// contract: the upstream chunker hands chunks over as []map[string]any (see the
// chunk map shape {"id","text","ck_type","doc_type_kwd","tk_nums"} observed from
// the running pipeline), not as []any of map. buildInputs must accept both
// shapes; a strict []any assertion alone would silently drop every chunk and
// leave the knowledge compiler with empty input (compiling nothing yet still
// reporting success).
func TestKnowledgeCompiler_BuildInputsAcceptsMapSliceChunks(t *testing.T) {
	in, err := buildInputs(map[string]any{
		"chunks": []map[string]any{
			{"id": "c1", "text": "《三国演义》", "ck_type": "text"},
			{"id": "c2", "text": "滚滚长江东逝水", "ck_type": "text"},
		},
	}, common.Param{})
	if err != nil {
		t.Fatalf("buildInputs: %v", err)
	}
	if len(in.Chunks) != 2 {
		t.Fatalf("buildInputs produced %d chunks, want 2 (upstream sends []map[string]any)", len(in.Chunks))
	}
	if in.Chunks[0].ID != "c1" || in.Chunks[0].Text != "《三国演义》" {
		t.Fatalf("chunk[0] = %+v, want id=c1 text=《三国演义》", in.Chunks[0])
	}

	// The legacy []any-of-map shape must still work.
	in2, err := buildInputs(map[string]any{
		"chunks": []any{map[string]any{"id": "x", "text": "t"}},
	}, common.Param{})
	if err != nil {
		t.Fatalf("buildInputs ([]any): %v", err)
	}
	if len(in2.Chunks) != 1 || in2.Chunks[0].ID != "x" {
		t.Fatalf("buildInputs ([]any) produced %+v, want 1 chunk id=x", in2.Chunks)
	}
}

// TestProductsToChunkDocs_PageVsSectionCompileKWD locks the page/section
// discriminator: a wiki page product is stamped compile_kwd="wiki_page" and a
// wiki section product compile_kwd="wiki_section", so a page search on
// compile_kwd="wiki_page" (engine_service / kcWikiPageStore) returns pages only.
func TestProductsToChunkDocs_PageVsSectionCompileKWD(t *testing.T) {
	page := common.Product{
		ID: "page-id", DocID: "d1", TenantID: "t1", Variant: common.VariantWiki,
		Content: "# Alpha\n\nBody", ParentID: "",
		Meta: map[string]any{"kind": "page", "slug": "entity/alpha", "title": "Alpha", "page_type": "entity", "source_chunk_ids": []string{"c1"}},
	}
	section := common.Product{
		ID: "section-id", DocID: "d1", TenantID: "t1", Variant: common.VariantWiki,
		Content: "Section body", ParentID: "page-id",
		Meta: map[string]any{"kind": "section", "slug": "overview", "page_slug": "entity/alpha", "section_level": 1, "source_chunk_ids": []string{"c1"}},
	}
	docs, err := productsToChunkDocs([]common.Product{page, section})
	if err != nil {
		t.Fatalf("productsToChunkDocs: %v", err)
	}
	var pageKWD, sectionKWD string
	var sectionParent string
	for _, d := range docs {
		// Product.Meta is preserved under the kc_* round-trip keys; the page/
		// section kind lives at "kc_kind".
		kind, _ := d.GetExtraString("kc_kind")
		if kind == "page" {
			pageKWD, _ = d.GetExtraString("compile_kwd")
		}
		if kind == "section" {
			sectionKWD, _ = d.GetExtraString("compile_kwd")
			sectionParent, _ = d.GetExtraString("parent_kwd")
		}
	}
	if pageKWD != "wiki_page" {
		t.Errorf("page compile_kwd = %q, want wiki_page", pageKWD)
	}
	if sectionKWD != "wiki_section" {
		t.Errorf("section compile_kwd = %q, want wiki_section (schema-backed page/section discriminator)", sectionKWD)
	}
	if sectionParent != "page-id" {
		t.Errorf("section parent_kwd = %q, want page-id", sectionParent)
	}
}

// TestMain installs the stub resolvers for the variant unit tests.
func TestMain(m *testing.M) {
	common.SetTemplateResolver(testTemplateResolver)
	common.SetGroupResolver(testGroupResolver)
	os.Exit(m.Run())
}
