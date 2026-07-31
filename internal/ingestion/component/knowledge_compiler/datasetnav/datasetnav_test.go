package datasetnav

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// scriptedEmbedder maps texts to fixed vectors (fallback: halves).
type scriptedEmbedder struct {
	vecs map[string][]float32
}

func (s scriptedEmbedder) Dimensions() int { return 2 }
func (s scriptedEmbedder) Encode(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		if v, ok := s.vecs[t]; ok {
			out[i] = v
		} else {
			out[i] = []float32{0.5, 0.5}
		}
	}
	return out, nil
}

// navFakeChat answers the three nav LLM calls deterministically: merge keeps
// the existing description, create-summary returns canned JSON, the root
// overview is a fixed string.
type navFakeChat struct {
	mergeCalls   int
	summaryCalls int
}

func (m *navFakeChat) Chat(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	switch {
	case strings.Contains(req.UserPrompt, "Merge the following two descriptions"):
		m.mergeCalls++
		existing := ""
		if rest, ok := strings.CutPrefix(req.UserPrompt, "Merge the following two descriptions of the same topic into a single concise summary (1-3 sentences):\n\nExisting: "); ok {
			existing, _, _ = strings.Cut(rest, "\n\nNew: ")
		}
		return &common.ChatResponse{Content: fmt.Sprintf(`{"merged": %q}`, existing)}, nil
	case strings.Contains(req.UserPrompt, "Given the document excerpts below"):
		m.summaryCalls++
		// Distinct summaries per call: readable cluster names hash the
		// description, so identical canned summaries would collide (the same
		// name-overwrite hazard exists in Python's _nav_cluster_id).
		return &common.ChatResponse{Content: fmt.Sprintf(`{"name": "Topic", "summary": "a topic summary %d"}`, m.summaryCalls)}, nil
	case strings.HasPrefix(req.UserPrompt, "Compose a navigation overview"):
		return &common.ChatResponse{Content: "root overview"}, nil
	default:
		return &common.ChatResponse{Content: "ok"}, nil
	}
}

func TestTwoMeans(t *testing.T) {
	embs := [][]float32{{1, 0}, {0.9, 0.1}, {0, 1}, {0.1, 0.9}}
	labels := twoMeans(embs)
	if len(labels) != 4 {
		t.Fatalf("labels = %v", labels)
	}
	if labels[0] != labels[1] || labels[2] != labels[3] || labels[0] == labels[2] {
		t.Fatalf("expected {x,x,y,y} grouping, got %v", labels)
	}
}

func TestReadableClusterName(t *testing.T) {
	got := readableClusterName("  My   Topic  ", "seed text")
	if !strings.HasPrefix(got, "My Topic ") {
		t.Fatalf("readableClusterName = %q, want cleaned title + hash suffix", got)
	}
	if got := readableClusterName("", "seed"); !strings.HasPrefix(got, "Cluster ") {
		t.Fatalf("empty title must fall back to Cluster: %q", got)
	}
}

func TestFallbackTitle(t *testing.T) {
	if got := fallbackTitle("one two three four five six seven eight"); got != "one two three four five six" {
		t.Fatalf("fallbackTitle = %q, want first 6 words", got)
	}
	if got := fallbackTitle(""); got != "Cluster" {
		t.Fatalf("empty fallbackTitle = %q, want Cluster", got)
	}
}

func TestPlace_MergeSiblingRootBranches(t *testing.T) {
	vecs := map[string][]float32{
		"A":  {1, 0},
		"A2": {0.99, 0.1},  // cosine ≈ 0.995 with A → merge
		"B":  {0.7, 0.714}, // cosine ≈ 0.7 with A → sibling
		"C":  {0, 1},       // cosine 0 with A → new root
	}
	deps := common.Deps{Chat: &navFakeChat{}, Embed: scriptedEmbedder{vecs: vecs}}
	tree := newNavTree()

	if _, err := tree.place(context.Background(), deps, "llm", "cA", "A", vecs["A"], mergeThresholdDefault); err != nil {
		t.Fatal(err)
	}
	if _, err := tree.place(context.Background(), deps, "llm", "cA2", "A2", vecs["A2"], mergeThresholdDefault); err != nil {
		t.Fatal(err)
	}
	if _, err := tree.place(context.Background(), deps, "llm", "cB", "B", vecs["B"], mergeThresholdDefault); err != nil {
		t.Fatal(err)
	}
	if _, err := tree.place(context.Background(), deps, "llm", "cC", "C", vecs["C"], mergeThresholdDefault); err != nil {
		t.Fatal(err)
	}

	if len(tree.order) != 3 {
		t.Fatalf("clusters = %d, want 3 (A-root, B-sibling, C-root): %v", len(tree.order), tree.order)
	}
	// A2 merged into A's cluster.
	var clusterA *navCluster
	for _, name := range tree.order {
		if tree.clusters[name].Depth == 0 && containsString(tree.clusters[name].DocIDs, "cA") {
			clusterA = tree.clusters[name]
		}
	}
	if clusterA == nil || len(clusterA.DocIDs) != 2 || clusterA.DocIDs[1] != "cA2" {
		t.Fatalf("A2 must merge into cluster A (doc_ids=%v)", clusterA)
	}
	// B's cluster: sibling under the virtual root (Python depth quirk: parent
	// depth default 1 → cluster depth 1, parent "root").
	var clusterB *navCluster
	for _, name := range tree.order {
		c := tree.clusters[name]
		if containsString(c.DocIDs, "cB") {
			clusterB = c
		}
	}
	if clusterB == nil || clusterB.Parent != "root" || clusterB.Depth != 1 {
		t.Fatalf("B sibling cluster = %+v, want parent root depth 1", clusterB)
	}
	// C's cluster: a fresh root-level cluster.
	var clusterC *navCluster
	for _, name := range tree.order {
		c := tree.clusters[name]
		if containsString(c.DocIDs, "cC") {
			clusterC = c
		}
	}
	if clusterC == nil || clusterC.Parent != "root" || clusterC.Depth != 0 {
		t.Fatalf("C root cluster = %+v, want parent root depth 0", clusterC)
	}
	if len(tree.docs) != 4 {
		t.Fatalf("nav docs = %d, want 4", len(tree.docs))
	}
}

func TestMaybeSplit(t *testing.T) {
	vecs := map[string][]float32{}
	deps := common.Deps{Chat: &navFakeChat{}, Embed: scriptedEmbedder{vecs: vecs}}
	tree := newNavTree()
	parent := &navCluster{Name: "P", Desc: "parent", Parent: "root", Depth: 0, Vector: []float32{1, 0}}
	tree.addCluster(parent)
	// 51 docs (> maxDocsPerCluster) alternating between two axes.
	for i := 0; i < 51; i++ {
		id := fmt.Sprintf("d%d", i)
		v := []float32{1, 0}
		if i%2 == 1 {
			v = []float32{0, 1}
		}
		parent.DocIDs = append(parent.DocIDs, id)
		tree.addDoc(parent, id, "text "+id, v, 1)
	}
	if err := tree.maybeSplit(context.Background(), deps, "llm", "P"); err != nil {
		t.Fatal(err)
	}
	// Expect two new sub-clusters under P, children reparented, P left with
	// no direct children.
	if len(tree.order) != 3 {
		t.Fatalf("clusters after split = %d, want 3 (P + 2 groups): %v", len(tree.order), tree.order)
	}
	// The split parent must now point at exactly the two new sub-clusters so
	// findBestCluster can descend into the split subtree (M3).
	if got := len(tree.children["P"]); got != 2 {
		t.Fatalf("split parent children = %d, want 2 (the two new sub-clusters)", got)
	}
	subDocs := 0
	for _, name := range tree.order[1:] {
		c := tree.clusters[name]
		if c.Parent != "P" || c.Depth != 1 {
			t.Errorf("sub-cluster %s parent/depth = %s/%d, want P/1", name, c.Parent, c.Depth)
		}
		subDocs += len(c.DocIDs)
	}
	if subDocs != 51 {
		t.Errorf("sub-cluster doc_ids total = %d, want 51", subDocs)
	}
	for _, d := range tree.docs {
		if d.Parent == "P" {
			t.Errorf("doc %s not reparented off P", d.ChunkID)
		}
		if d.Depth != 2 {
			t.Errorf("doc %s depth = %d, want 2", d.ChunkID, d.Depth)
		}
	}
}

func TestMaybeSplit_BelowCaps(t *testing.T) {
	tree := newNavTree()
	parent := &navCluster{Name: "P", Desc: "p", Parent: "root", Depth: 0, Vector: []float32{1, 0}}
	tree.addCluster(parent)
	for i := 0; i < 3; i++ {
		tree.addDoc(parent, fmt.Sprintf("d%d", i), "t", []float32{1, 0}, 1)
	}
	if err := tree.maybeSplit(context.Background(), common.Deps{}, "llm", "P"); err != nil {
		t.Fatal(err)
	}
	if len(tree.order) != 1 {
		t.Fatalf("below caps must not split: %v", tree.order)
	}
}

func TestRun_EndToEnd(t *testing.T) {
	vecs := map[string][]float32{
		"alpha one": {1, 0},
		"alpha two": {0.99, 0.1},
		"beta one":  {0, 1},
	}
	deps := common.Deps{Chat: &navFakeChat{}, Embed: scriptedEmbedder{vecs: vecs}, TenantID: "t1", DatasetID: "ds1"}
	p := common.Param{}.Defaults()
	p.Variant = common.VariantDatasetnav
	inputs := common.Inputs{
		DocID: "d1",
		Chunks: []common.Chunk{
			{ID: "c1", Text: "alpha one"},
			{ID: "c2", Text: "alpha two"},
			{ID: "c3", Text: "beta one"},
		},
	}
	out, err := Run(context.Background(), deps, p, inputs)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var clusters, docs, roots int
	parentByID := map[string]string{}
	for _, p := range out.Products {
		switch p.Meta["kind"] {
		case "nav_cluster":
			clusters++
			parentByID[p.Meta["name"].(string)] = p.Meta["parent_name"].(string)
		case "nav_doc":
			docs++
			if p.ParentID == "" {
				t.Errorf("nav_doc must link to its cluster product")
			}
		case "root":
			roots++
		}
	}
	if clusters != 2 || docs != 3 || roots != 1 {
		t.Fatalf("products = %d clusters + %d docs + %d roots, want 2+3+1", clusters, docs, roots)
	}
}

func TestLLMMergeFallbacks(t *testing.T) {
	// Non-JSON text reply is used verbatim (mirrors gen_json's str path).
	deps := common.Deps{Chat: chatFunc(func(req common.ChatRequest) (*common.ChatResponse, error) {
		return &common.ChatResponse{Content: "merged text"}, nil
	})}
	got, err := llmMerge(context.Background(), deps, "llm", "old", "new")
	if err != nil || got != "merged text" {
		t.Fatalf("llmMerge text path = %q, %v", got, err)
	}
	// Empty reply keeps the old description.
	deps2 := common.Deps{Chat: chatFunc(func(req common.ChatRequest) (*common.ChatResponse, error) {
		return &common.ChatResponse{Content: ""}, nil
	})}
	got, _ = llmMerge(context.Background(), deps2, "llm", "old", "new")
	if got != "old" {
		t.Fatalf("empty reply must keep old desc, got %q", got)
	}
	// Nil chat keeps the old description without a call.
	got, _ = llmMerge(context.Background(), common.Deps{}, "llm", "old", "new")
	if got != "old" {
		t.Fatalf("nil chat must keep old desc, got %q", got)
	}
}

type chatFunc func(common.ChatRequest) (*common.ChatResponse, error)

func (f chatFunc) Chat(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	return f(req)
}
