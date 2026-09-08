package knowledge_compiler

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/ingestion/component/knowledge_compiler/golden"
	"ragflow/internal/ingestion/component/schema"
)

// chunkHasVector reports whether a compiled chunk carries any q_<dim>_vec embedding.
func chunkHasVector(c schema.ChunkDoc) bool {
	for k := range c.Extra {
		if strings.HasPrefix(k, "q_") && strings.HasSuffix(k, "_vec") {
			return true
		}
	}
	return false
}

// signedEmbedder is a deterministic embedder whose vectors span both signs.
// The default mockEmbedder emits non-negative vectors in [0,1], which makes
// every pairwise cosine >= 0; under PSI's threshold=0 merge rule that collapses
// the whole corpus into a single cluster and triggers unbounded recursion in
// buildTree. Signed vectors give the clustering real signal so the golden
// corpus separates into multiple small clusters (the production path uses real
// centered embeddings and does not hit this).
type signedEmbedder struct{ dim int }

func (s signedEmbedder) Dimensions() int { return s.dim }
func (s signedEmbedder) Encode(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = signedVec(t, s.dim)
	}
	return out, nil
}
func signedVec(str string, dim int) []float32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(str))
	seed := h.Sum32()
	v := make([]float32, dim)
	for j := 0; j < dim; j++ {
		x := int32(seed>>uint(j*3)) % 100
		v[j] = float32(x)/50.0 - 1.0
	}
	return v
}

// installSignedProseDeps wires a prose LLM + signed deterministic embedder.
func installSignedProseDeps(t *testing.T) {
	t.Helper()
	common.SetDepsResolver(func(tenantID, llmID, embeddingModel string) (common.Deps, error) {
		return common.Deps{Chat: proseChat{}, Embed: signedEmbedder{dim: 8}, TenantID: tenantID}, nil
	})
	t.Cleanup(func() { common.SetDepsResolver(nil) })
}

// runVariantChunks builds the component for variant, runs it over the fixed
// golden corpus, and returns the compiled chunks (schema.ChunkDoc) that were
// merged into the output chunks list.
func runVariantChunks(t *testing.T, variant string, extra map[string]any) []schema.ChunkDoc {
	t.Helper()
	return runVariantChunksWithInputs(t, variant, extra, nil)
}

// runVariantChunksWithInputs is runVariantChunks plus extra Invoke-level
// inputs (e.g. parser_config for the structure variant's template shape).
func runVariantChunksWithInputs(t *testing.T, variant string, extra, inputsExtra map[string]any) []schema.ChunkDoc {
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
	invokeInputs := map[string]any{
		"chunks":    golden.ChunksToAny(golden.FixedCorpus()),
		"doc_id":    "d1",
		"tenant_id": "t1",
	}
	for k, v := range inputsExtra {
		invokeInputs[k] = v
	}
	out, err := c.Invoke(context.Background(), nil, invokeInputs)
	if err != nil {
		t.Fatalf("Invoke(%s): %v", variant, err)
	}
	raw, ok := out["chunks"].([]any)
	if !ok {
		t.Fatalf("Invoke(%s): chunks = %T", variant, out["chunks"])
	}
	docs, ok, err := schema.ChunkDocsFromAny(raw)
	if !ok || err != nil {
		t.Fatalf("Invoke(%s): ChunkDocsFromAny = %v err=%v", variant, ok, err)
	}
	// The component merges the compiled knowledge units into the upstream input
	// chunks; the golden gates only analyze the compiled units (those carrying
	// the compile_kwd discriminator), mirroring the old out["products"] surface.
	compiled := make([]schema.ChunkDoc, 0, len(docs))
	for _, d := range docs {
		if _, has := d.GetExtraString("compile_kwd"); has {
			compiled = append(compiled, d)
		}
	}
	return compiled
}

func loadBaseline(t *testing.T, name string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("golden", "fixtures", name))
	if err != nil {
		t.Fatalf("read baseline %s: %v", name, err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal baseline %s: %v", name, err)
	}
	return m
}

func num(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

// TestGolden_Structure_ProductCount is the 缺口 E gate for the structure
// variant: on the fixed corpus with the deterministic mock LLM, the Go product
// count must equal the locked golden baseline. The gate feeds a graph-kind
// parser_config so the hypergraph path (two-stage entity → relation
// extraction + LLM merge dedup) is what gets locked.
func TestGolden_Structure_ProductCount(t *testing.T) {
	installMockDeps(t)
	prods := runVariantChunksWithInputs(t, "structure", nil, map[string]any{
		"parser_config": graphParserConfig(),
	})

	// Schema invariants: every product carries the schema fields and a vector.
	for _, p := range prods {
		id, _ := p.GetExtraString("id")
		docID, _ := p.GetExtraString("doc_id")
		tenant, _ := p.GetExtraString("tenant_id")
		if id == "" || docID == "" || tenant == "" || p.Text == "" {
			t.Fatalf("structure: product missing schema fields: %+v", p)
		}
		if !chunkHasVector(p) {
			t.Fatalf("structure: product %s missing vector", id)
		}
	}

	// Exactly one graph product summarizing {nodes, edges}.
	graphCount, nodeCount, edgeCount := 0, 0, 0
	for _, p := range prods {
		kind, _ := p.GetExtraString("kc_kind")
		switch kind {
		case "graph":
			graphCount++
		case "entity":
			nodeCount++
		case "relation":
			edgeCount++
		}
	}
	if graphCount != 1 {
		t.Fatalf("structure: graphCount = %d, want 1", graphCount)
	}
	if nodeCount == 0 || edgeCount == 0 {
		t.Fatalf("structure: expected entities+relations, got nodes=%d edges=%d", nodeCount, edgeCount)
	}

	baseline := loadBaseline(t, "structure_wiki_baseline.json")
	t.Logf("structure: %d products", len(prods))
	want := num(baseline, "structure_products")
	if want != 0 && len(prods) != want {
		t.Errorf("structure product count = %d, golden baseline = %d (review contract if intentional)", len(prods), want)
	}
}

// TestGolden_Wiki_ProductCount is the 缺口 E gate for the wiki variant.
func TestGolden_Wiki_ProductCount(t *testing.T) {
	installProseDeps(t)
	prods := runVariantChunks(t, "wiki", nil)

	for _, p := range prods {
		id, _ := p.GetExtraString("id")
		docID, _ := p.GetExtraString("doc_id")
		tenant, _ := p.GetExtraString("tenant_id")
		if id == "" || docID == "" || tenant == "" || p.Text == "" {
			t.Fatalf("wiki: product missing schema fields: %+v", p)
		}
		if !chunkHasVector(p) {
			t.Fatalf("wiki: product %s missing vector", id)
		}
	}

	foundPage := false
	for _, p := range prods {
		if kind, _ := p.GetExtraString("kc_kind"); kind == "page" {
			foundPage = true
		}
	}
	if !foundPage {
		t.Fatalf("wiki: no 'page' product; got %d products", len(prods))
	}

	baseline := loadBaseline(t, "structure_wiki_baseline.json")
	t.Logf("wiki: %d products", len(prods))
	want := num(baseline, "wiki_products")
	if want != 0 && len(prods) != want {
		t.Errorf("wiki product count = %d, golden baseline = %d (review contract if intentional)", len(prods), want)
	}
}

// TestGolden_Tree_Structure is the 缺口 C gate for the tree variant: on the
// fixed corpus, the watershed builder (tree/raptor.go::watershed) must produce a
// well-formed tree (single root, fully parented, vectors + schema intact, full
// chunk coverage, bounded depth/cluster count) across several tree_order
// settings. The structural contract is what a regression would violate; the
// exact cluster counts are locked in tree_baseline.json.
func TestGolden_Tree_Structure(t *testing.T) {
	installSignedProseDeps(t)
	baseline := loadBaseline(t, "tree_baseline.json")
	nChunks := len(golden.FixedCorpus())

	cases := []struct {
		name   string
		order  int // <=0 means default (omit extra)
		minKey string
		maxKey string
	}{
		{"default", 0, "default_leaf_clusters_min", "default_leaf_clusters_max"},
		{"fine", 2, "fine_leaf_clusters_min", "fine_leaf_clusters_max"},
		{"coarse", 10, "coarse_leaf_clusters_min", "coarse_leaf_clusters_max"},
	}
	for _, tc := range cases {
		var prods []schema.ChunkDoc
		if tc.order <= 0 {
			prods = runVariantChunks(t, "tree", nil)
		} else {
			prods = runVariantChunks(t, "tree", map[string]any{
				"extra": map[string]any{"tree_order": tc.order},
			})
		}
		m := golden.AnalyzeTreeProducts(prods, goldenCorpusIDs()...)
		t.Logf("tree(%s): products=%d root=%d leafClusters=%d maxDepth=%d coverage=%.2f",
			tc.name, m.ProductCount, m.RootCount, m.LeafClusters, m.MaxDepth, m.CoverageFraction(nChunks))

		if m.RootCount != 1 {
			t.Errorf("tree(%s): rootCount=%d, want 1", tc.name, m.RootCount)
		}
		if !m.AllParented {
			t.Errorf("tree(%s): tree has dangling parent_id references", tc.name)
		}
		if !m.VectorOK {
			t.Errorf("tree(%s): some products missing vectors", tc.name)
		}
		if !m.SchemaOK {
			t.Errorf("tree(%s): some products missing schema fields", tc.name)
		}
		if cov := m.CoverageFraction(nChunks); cov < numFloat(baseline, "leaf_coverage_min") {
			t.Errorf("tree(%s): leaf coverage %.2f < baseline %.2f", tc.name, cov, numFloat(baseline, "leaf_coverage_min"))
		}
		if m.LeafClusters < num(baseline, tc.minKey) || m.LeafClusters > num(baseline, tc.maxKey) {
			t.Errorf("tree(%s): leafClusters=%d outside [%d,%d]", tc.name, m.LeafClusters, num(baseline, tc.minKey), num(baseline, tc.maxKey))
		}
		if m.MaxDepth > num(baseline, "max_depth_max") {
			t.Errorf("tree(%s): maxDepth=%d > baseline %d", tc.name, m.MaxDepth, num(baseline, "max_depth_max"))
		}
	}
}

func numFloat(m map[string]any, key string) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return 0
}

// goldenCorpusIDs returns the IDs of every chunk in the fixed golden corpus.
// Passed to AnalyzeTreeProducts so coverage only counts source chunk IDs that
// belong to the input corpus (a leaked/unknown ID must not inflate coverage).
func goldenCorpusIDs() []string {
	corpus := golden.FixedCorpus()
	ids := make([]string, 0, len(corpus))
	for _, c := range corpus {
		ids = append(ids, c.ID)
	}
	return ids
}
