package knowledge_compiler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
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
// (wiki/raptor/mindmap/datasetnav), which all just need a summary/outline text.
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

	c, err := NewKnowledgeCompilerComponent("KnowledgeCompiler", map[string]any{
		"variant": "structure", "llm_id": "llm1", "embedding_model": "emb1",
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
	c, err := NewKnowledgeCompilerComponent("KnowledgeCompiler", map[string]any{"variant": "nope"})
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent: %v", err)
	}
	_, err = c.Invoke(context.Background(), nil, map[string]any{"chunks": []any{}})
	if !errors.Is(err, common.ErrUnknownVariant) {
		t.Fatalf("err = %v, want ErrUnknownVariant", err)
	}
}

func TestKnowledgeCompiler_Alias_Mindmap(t *testing.T) {
	installMockDeps(t)
	// "mind_map" is the deprecated alias for "mindmap"; both resolve to the
	// implemented mindmap variant and must run (not ErrUnknownVariant / stub).
	c, err := NewKnowledgeCompilerComponent("KnowledgeCompiler", map[string]any{
		"variant": "mind_map", "llm_id": "llm1", "embedding_model": "emb1",
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
	params := map[string]any{"variant": variant, "llm_id": "llm1", "embedding_model": "emb1"}
	for k, v := range extra {
		params[k] = v
	}
	c, err := NewKnowledgeCompilerComponent("KnowledgeCompiler", params)
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

func TestKnowledgeCompiler_Raptor_EndToEnd(t *testing.T) {
	installProseDeps(t)
	// watershed (default tree_order): zero external clustering dependency.
	chunks := runVariant(t, "raptor", nil)
	foundRoot := false
	for _, c := range chunks {
		if kind, _ := c["kc_kind"].(string); kind == "root" {
			foundRoot = true
		}
	}
	if !foundRoot {
		t.Fatalf("raptor(default): no 'root' chunk; got %d chunks", len(chunks))
	}

	// A smaller tree_order (more, smaller clusters) must also run and still
	// produce a well-formed tree (root present, chunks non-empty).
	chunksCoarse := runVariant(t, "raptor", map[string]any{"extra": map[string]any{"tree_order": 2}})
	if len(chunksCoarse) == 0 {
		t.Fatalf("raptor(tree_order=2): produced no chunks")
	}
	foundRootCoarse := false
	for _, c := range chunksCoarse {
		if kind, _ := c["kc_kind"].(string); kind == "root" {
			foundRootCoarse = true
		}
	}
	if !foundRootCoarse {
		t.Fatalf("raptor(tree_order=2): no 'root' chunk; got %d chunks", len(chunksCoarse))
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

func TestKnowledgeCompiler_Datasetnav_EndToEnd(t *testing.T) {
	installProseDeps(t)
	chunks := runVariant(t, "datasetnav", nil)
	foundRoot := false
	for _, c := range chunks {
		if kind, _ := c["kc_kind"].(string); kind == "root" {
			foundRoot = true
		}
	}
	if !foundRoot {
		t.Fatalf("datasetnav: no 'root' chunk; got %d chunks", len(chunks))
	}
}

// TestKnowledgeCompiler_EmitsChunks verifies that after compiling, the
// component returns the knowledge units merged into the chunk stream (no
// separate products/writer seam). The output shape is the chunker's
// {output_format:"chunks", chunks:[...]}.
func TestKnowledgeCompiler_EmitsChunks(t *testing.T) {
	installMockDeps(t)

	c, err := NewKnowledgeCompilerComponent("KnowledgeCompiler", map[string]any{
		"variant": "structure", "llm_id": "llm1", "embedding_model": "emb1",
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

	c, err := NewKnowledgeCompilerComponent("KnowledgeCompiler", map[string]any{
		"variant":                  "structure",
		"llm_id":                   "llm1",
		"embedding_model":          "emb1",
		"compilation_template_ids": []any{"tpl-A", "tpl-B"},
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
		// Every compiled chunk must carry the resolved template ids.
		var tidsCount int
		switch tids := cm["compilation_template_ids"].(type) {
		case []any:
			tidsCount = len(tids)
		case []string:
			tidsCount = len(tids)
		}
		if tidsCount != 2 {
			t.Fatalf("compiled chunk %v: compilation_template_ids = %v, want 2", cm["id"], cm["compilation_template_ids"])
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

// TestKnowledgeCompiler_Raptor_DegenerateNoInfiniteLoop is a regression for the
// High-1 bug: RAPTOR could recurse forever when a re-cluster returned a single
// label covering all points. The default mockEmbedder emits non-negative
// vectors, so under the watershed default (tree_order=4, ratio 25) every adjacent pair
// has cosine >= 0 and a pathological input can collapse into one cluster; with
// more than one point the old code re-enqueued the identical
// work item at level+1 and hung. This test uses 6 chunks and asserts the run
// terminates with a well-formed root. (If the guard regresses, go test's timeout
// turns the hang into a failure.)
func TestKnowledgeCompiler_Raptor_DegenerateNoInfiniteLoop(t *testing.T) {
	installProseDeps(t)
	c, err := NewKnowledgeCompilerComponent("KnowledgeCompiler", map[string]any{
		"variant": "raptor", "llm_id": "llm1", "embedding_model": "emb1",
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
		t.Fatalf("Invoke hung or errored (RAPTOR infinite recursion?): %v", err)
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
		t.Fatalf("raptor(degenerate): no root chunk; got %d chunks", len(raw))
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

	c, err := NewKnowledgeCompilerComponent("KnowledgeCompiler", map[string]any{
		"variant": "wiki", "llm_id": "llm1", "embedding_model": "emb1",
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

	c, err := NewKnowledgeCompilerComponent("KnowledgeCompiler", map[string]any{
		"variant": "wiki", "llm_id": "llm1", "embedding_model": "emb1",
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
		if cm["compile_kwd"] == "artifact_page" && cm["kc_kind"] == "page" && cm["slug_kwd"] == "entity/alpha" {
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

	c, err := NewKnowledgeCompilerComponent("KnowledgeCompiler", map[string]any{
		"variant": "wiki", "llm_id": "llm1", "embedding_model": "emb1",
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

// fakeDatasetnavLock records the key it was asked to acquire, so a test can
// assert the datasetnav rebuild lock is scoped to the dataset, not the document.
type fakeDatasetnavLock struct {
	mu      sync.Mutex
	lastKey string
}

func (f *fakeDatasetnavLock) Acquire(_ context.Context, key string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastKey = key
	return true, nil
}
func (f *fakeDatasetnavLock) Release(_ context.Context, _ string) error { return nil }

// TestKnowledgeCompiler_Datasetnav_LockKeyedByDataset is a regression for the
// Medium bug: the distributed rebuild lock used to be keyed by doc_id, so two
// runs against the same dataset (with different per-document doc_id values)
// took different locks and could interleave. This test supplies both doc_id
// ("d1") and dataset_id ("ds1") and asserts the lock key uses the dataset.
func TestKnowledgeCompiler_Datasetnav_LockKeyedByDataset(t *testing.T) {
	lk := &fakeDatasetnavLock{}
	common.SetDepsResolver(func(tenantID, llmID, embeddingModel string) (common.Deps, error) {
		return common.Deps{
			Chat:     proseChat{},
			Embed:    mockEmbedder{dim: 8},
			TenantID: tenantID,
			Redis:    lk,
		}, nil
	})
	t.Cleanup(func() { common.SetDepsResolver(nil) })

	c, err := NewKnowledgeCompilerComponent("KnowledgeCompiler", map[string]any{
		"variant": "datasetnav", "llm_id": "llm1", "embedding_model": "emb1",
	})
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent: %v", err)
	}
	if _, err := c.Invoke(context.Background(), nil, map[string]any{
		"chunks": []any{
			map[string]any{"id": "c1", "text": "The quick brown fox jumps over the lazy dog near the river bank."},
			map[string]any{"id": "c2", "text": "A red fox and a lazy dog rest beside a calm river at dawn."},
		},
		"doc_id":     "d1",
		"dataset_id": "ds1",
		"tenant_id":  "t1",
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	lk.mu.Lock()
	key := lk.lastKey
	lk.mu.Unlock()
	want := "datasetnav:t1:ds1"
	if key != want {
		t.Fatalf("datasetnav lock key = %q, want %q (lock must be scoped to the dataset, not the document)", key, want)
	}
}

// recordingNavChat echoes each nav-group summary back verbatim (so the child
// summaries are identifiable) and records the root-synthesis user prompt so a
// test can assert which child summaries reached the root overview.
type recordingNavChat struct {
	mu         sync.Mutex
	rootPrompt string
}

func (r *recordingNavChat) Chat(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	if strings.HasPrefix(req.UserPrompt, "Compose a navigation overview") {
		r.mu.Lock()
		r.rootPrompt = req.UserPrompt
		r.mu.Unlock()
		return &common.ChatResponse{Content: "root overview"}, nil
	}
	// Nav-group summary: echo the group text so the marker survives into the
	// root prompt when the root is built from all child summaries.
	return &common.ChatResponse{Content: strings.TrimSpace(req.UserPrompt)}, nil
}

// TestKnowledgeCompiler_Datasetnav_RootIncludesAllSummaries is a regression for
// root-overview completeness: the root synthesis must be built from ALL child
// summaries, not from a partial buffer. The Go implementation buffers every
// product in Outputs.Products (no streaming sink), so the root always sees the
// full child set even when the result set is large. Here nav_radius>1 forces
// each chunk into its own nav node, and we assert every child marker appears in
// the recorded root-synthesis prompt.
func TestKnowledgeCompiler_Datasetnav_RootIncludesAllSummaries(t *testing.T) {
	chat := &recordingNavChat{}
	common.SetDepsResolver(func(tenantID, llmID, embeddingModel string) (common.Deps, error) {
		return common.Deps{
			Chat:     chat,
			Embed:    mockEmbedder{dim: 8},
			TenantID: tenantID,
		}, nil
	})
	t.Cleanup(func() { common.SetDepsResolver(nil) })

	c, err := NewKnowledgeCompilerComponent("KnowledgeCompiler", map[string]any{
		"variant": "datasetnav", "llm_id": "llm1", "embedding_model": "emb1",
		// nav_radius > 1 guarantees no two chunks group together (cosine <= 1),
		// so each chunk becomes its own nav node.
		"extra": map[string]any{"nav_radius": 1.1},
	})
	if err != nil {
		t.Fatalf("NewKnowledgeCompilerComponent: %v", err)
	}

	markers := []string{"NAVMARKA", "NAVMARKB", "NAVMARKC", "NAVMARKD", "NAVMARKE"}
	chunks := make([]any, len(markers))
	for i, m := range markers {
		chunks[i] = map[string]any{"id": m, "text": m + " unique section body text"}
	}
	if _, err := c.Invoke(context.Background(), nil, map[string]any{
		"chunks":    chunks,
		"doc_id":    "d1",
		"tenant_id": "t1",
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	chat.mu.Lock()
	root := chat.rootPrompt
	chat.mu.Unlock()
	if root == "" {
		t.Fatalf("root synthesis prompt was never recorded (root node not built)")
	}
	for _, m := range markers {
		if !strings.Contains(root, m) {
			t.Fatalf("root overview is missing child summary %q; the root must be built from ALL child summaries.\nroot prompt:\n%s", m, root)
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

	c, err := NewKnowledgeCompilerComponent("KnowledgeCompiler", map[string]any{
		"variant": "structure", "llm_id": "llm1", "embedding_model": "emb1",
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

	c, err := NewKnowledgeCompilerComponent("KnowledgeCompiler", map[string]any{
		"variant": "structure", "llm_id": "llm1", "embedding_model": "emb1",
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

	c, err := NewKnowledgeCompilerComponent("KnowledgeCompiler", map[string]any{
		"variant": "structure", "llm_id": "llm1", "embedding_model": "emb1",
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
func groupResolverStub(_ context.Context, _ string, groupIDs []string) ([]string, error) {
	var out []string
	for _, g := range groupIDs {
		out = append(out, "tpl-"+g+"-a", "tpl-"+g+"-b")
	}
	return out, nil
}

// TestKnowledgeCompiler_GroupIDsResolvedToTemplateIDs is a regression for the
// open question: compilation_template_group_ids were parsed into Param.GroupIDs
// but never resolved or stamped, so group-only configs silently missed
// compilation_template_ids. The component now resolves groups via the installed
// GroupResolver and merges them into the stamped template-id list.
func TestKnowledgeCompiler_GroupIDsResolvedToTemplateIDs(t *testing.T) {
	installMockDeps(t)
	common.SetGroupResolver(groupResolverStub)
	t.Cleanup(func() { common.SetGroupResolver(nil) })

	c, err := NewKnowledgeCompilerComponent("KnowledgeCompiler", map[string]any{
		"variant":                        "structure",
		"llm_id":                         "llm1",
		"embedding_model":                "emb1",
		"compilation_template_group_ids": []any{"grp1"},
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
		if len(got) != 2 || !want[got[0]] || !want[got[1]] {
			t.Fatalf("compiled chunk %v: compilation_template_ids = %v, want %v (group ids must resolve to template ids)", cm["id"], got, want)
		}
	}
	if checked == 0 {
		t.Fatalf("no compiled chunks inspected (compile_kwd=list expected); assertion was vacuous")
	}
}

// TestKnowledgeCompiler_GroupIDsWithoutResolverFailsLoud is the companion
// regression: a group-only config with no GroupResolver installed must fail
// loudly (surfacing the misconfiguration) rather than silently emitting rows
// that miss compilation_template_ids.
func TestKnowledgeCompiler_GroupIDsWithoutResolverFailsLoud(t *testing.T) {
	installMockDeps(t)
	common.SetGroupResolver(nil) // ensure no resolver is installed

	c, err := NewKnowledgeCompilerComponent("KnowledgeCompiler", map[string]any{
		"variant":                        "structure",
		"llm_id":                         "llm1",
		"embedding_model":                "emb1",
		"compilation_template_group_ids": []any{"grp1"},
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
