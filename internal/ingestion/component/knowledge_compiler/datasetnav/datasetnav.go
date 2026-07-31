// Package datasetnav implements the "datasetnav" variant of KnowledgeCompiler,
// mirroring Python's dataset_nav.py incremental clustering: each input chunk
// (the in-run equivalent of a document leaf) is embedded and placed into the
// nearest nav_cluster via layered KNN descent — merged when the similarity
// clears _MERGE_THRESHOLD, given a fresh sibling cluster above _MIN_SIM, or
// given a new root-level cluster otherwise. Clusters that exceed the fanout
// or doc-count caps split via the same 2-means procedure as Python.
//
// The Go port keeps the whole tree in memory for one Invoke (no ES reads or
// writes): cross-run incremental upsert/removal, which in Python is an
// ES-backed read-modify-write, is the caller's concern. The per-run algorithm
// (thresholds, LLM merge/summary prompts, readable cluster names, split) is
// faithful to the Python original.
package datasetnav

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// Thresholds and caps, mirrored from dataset_nav.py.
const (
	mergeThresholdDefault = 0.80 // merge doc into cluster
	recurseThreshold      = 0.65 // continue descending into children
	minSim                = 0.50 // minimum similarity to be considered related
	maxFanout             = 64   // max child count before triggering rebalance
	maxDocsPerCluster     = 50   // max docs per leaf cluster before triggering split
)

// datasetnavLock is the minimal locking surface used to serialize dataset-level
// rebuilds. The production Redis client implements this via SET NX + TTL; tests
// inject an in-memory lock (or nil for no-op).
type datasetnavLock interface {
	Acquire(ctx context.Context, key string) (bool, error)
	Release(ctx context.Context, key string) error
}

// navCluster is one internal tree node (Python's nav_cluster row).
type navCluster struct {
	Name   string
	Desc   string
	Parent string // parent cluster name; "root" for depth-0 clusters
	Depth  int
	DocIDs []string
	Vector []float32
}

// navDoc is one document leaf (Python's nav_doc row).
type navDoc struct {
	ChunkID string
	Text    string
	Parent  string // owning cluster name
	Depth   int
	Vector  []float32
}

// navChild is one child reference under a cluster.
type navChild struct {
	name  string // cluster name or doc chunk id
	isDoc bool
	vec   []float32
}

// navTree holds the in-memory tree built over one Invoke.
type navTree struct {
	clusters map[string]*navCluster
	order    []string // cluster names in creation order
	docs     []*navDoc
	children map[string][]navChild // parent cluster name → children
}

func newNavTree() *navTree {
	return &navTree{clusters: map[string]*navCluster{}, children: map[string][]navChild{}}
}

func (t *navTree) addCluster(c *navCluster) {
	t.clusters[c.Name] = c
	t.order = append(t.order, c.Name)
}

// Run executes the datasetnav variant.
func Run(ctx context.Context, deps common.Deps, param common.Param, inputs common.Inputs) (common.Outputs, error) {
	if deps.Embed == nil {
		return common.Outputs{}, fmt.Errorf("datasetnav: embedder required")
	}
	docID := firstNonEmpty(inputs.DocID, deps.DatasetID)
	if docID == "" {
		docID = "unknown"
	}
	llmID := firstNonEmpty(param.LLMID, inputs.LLMID)
	tenantID := deps.TenantID

	// The rebuild lock is scoped to the dataset, not the document (mirrors
	// Python's _nav_lock_key(kb_id)): concurrent runs against the same dataset
	// must share one lock. Fall back to docID only when no dataset id is known.
	datasetID := firstNonEmpty(deps.DatasetID, docID)

	texts, chunkIDs := chunkTexts(inputs.Chunks)
	if len(texts) == 0 {
		return common.Outputs{}, nil
	}

	if l, ok := deps.Redis.(datasetnavLock); ok && l != nil {
		lockKey := "datasetnav:" + tenantID + ":" + datasetID
		acquired, err := l.Acquire(ctx, lockKey)
		if err != nil {
			return common.Outputs{}, err
		}
		if !acquired {
			return common.Outputs{}, fmt.Errorf("datasetnav: lock not acquired for %s", lockKey)
		}
		defer l.Release(ctx, lockKey)
	}

	// Embed every chunk text once (Python embeds each doc summary on upsert).
	vectors, err := deps.Embed.Encode(ctx, texts)
	if err != nil {
		return common.Outputs{}, err
	}
	if len(vectors) != len(texts) {
		return common.Outputs{}, fmt.Errorf("datasetnav: embedding count mismatch (%d vs %d)", len(vectors), len(texts))
	}

	// Sequential placement, mirroring the lock-serialized per-document upserts
	// in Python (also what makes the in-run tree deterministic).
	tree := newNavTree()
	var clusterDescs []string
	mergeThreshold := mergeThresholdFor(param)
	for i, text := range texts {
		if err := ctx.Err(); err != nil {
			return common.Outputs{}, err
		}
		desc, err := tree.place(ctx, deps, llmID, chunkIDs[i], text, vectors[i], mergeThreshold)
		if err != nil {
			return common.Outputs{}, err
		}
		clusterDescs = append(clusterDescs, desc...)
	}

	// Emit cluster rows in creation order, then nav_doc leaves in placement
	// order, then the root overview node (a Go-side convenience: Python's tree
	// has no root row, but downstream consumers expect one overview product).
	var products []common.Product
	clusterProductID := map[string]string{}
	// Pre-compute every cluster id (deterministic from StableRowID) before
	// resolving parent edges, so a child emitted before its (later-created)
	// parent still gets the correct ParentID instead of "" (reparenting bug).
	// Cluster ids are tenant-scoped (as the nav_doc ids already are) so two
	// tenants whose datasetID falls back to the same docID cannot collide.
	for _, name := range tree.order {
		clusterProductID[name] = common.StableRowID("dataset_nav", tenantID, datasetID, "cluster", name)
	}
	for _, name := range tree.order {
		c := tree.clusters[name]
		pid := ""
		if c.Parent != "" && c.Parent != "root" {
			pid = clusterProductID[c.Parent]
		}
		id := clusterProductID[c.Name]
		products = append(products, common.Product{
			ID:       id,
			DocID:    docID,
			TenantID: tenantID,
			Variant:  common.VariantDatasetnav,
			Content:  payloadJSON(map[string]any{"type": "nav_cluster", "description": c.Desc}),
			Vector:   c.Vector,
			ParentID: pid,
			Meta: map[string]any{
				"kind":        "nav_cluster",
				"type":        "nav_cluster",
				"name":        c.Name,
				"parent_name": c.Parent,
				"depth":       c.Depth,
				"doc_ids":     append([]string{}, c.DocIDs...),
				"size":        len(c.DocIDs),
			},
		})
	}
	for _, d := range tree.docs {
		products = append(products, common.Product{
			// Scope nav_doc ids by tenant and dataset (not just chunk id) so
			// synthetic/positional chunk ids from different docs/datasets do not
			// collide and overwrite each other across tenants (M5).
			ID:       common.StableRowID("dataset_nav", tenantID, datasetID, "doc", d.ChunkID),
			DocID:    docID,
			TenantID: tenantID,
			Variant:  common.VariantDatasetnav,
			Content:  payloadJSON(map[string]any{"type": "nav_doc", "description": d.Text}),
			Vector:   d.Vector,
			ParentID: clusterProductID[d.Parent],
			Meta: map[string]any{
				"kind":        "nav_doc",
				"type":        "nav_doc",
				"name":        navDocName(d.Parent, d.Text),
				"parent_name": d.Parent,
				"depth":       d.Depth,
				"doc_ids":     []string{d.ChunkID},
			},
		})
	}

	// Root overview built from EVERY cluster description (all products are
	// buffered in the same slice, so every node contributes to the root).
	rootSummary, err := summarize(ctx, deps, llmID, "Compose a navigation overview from these section summaries:\n\n"+formatNavSummaries(clusterDescs))
	if err == nil && rootSummary != "" {
		emb, e2 := deps.Embed.Encode(ctx, []string{rootSummary})
		if e2 == nil && len(emb) > 0 {
			products = append(products, common.Product{
				ID:       common.StableRowID(tenantID, docID, string(common.VariantDatasetnav), "root"),
				DocID:    docID,
				TenantID: tenantID,
				Variant:  common.VariantDatasetnav,
				Content:  rootSummary,
				Vector:   emb[0],
				Meta: map[string]any{
					"kind":  "root",
					"type":  "nav_cluster",
					"name":  "root",
					"depth": 0,
					"size":  len(tree.docs),
				},
			})
		}
	}

	out := common.Outputs{
		Products: products,
	}
	return out, nil
}

// place inserts one document (chunk) into the tree, mirroring
// upsert_dataset_nav_doc's placement logic. It returns the descriptions of
// any clusters created for this document (for the root overview).
func (t *navTree) place(ctx context.Context, deps common.Deps, llmID, chunkID, text string, vec []float32, mergeThreshold float64) ([]string, error) {
	bestName, bestParent, sim := t.findBestCluster(vec)

	switch {
	case bestName != "" && sim >= mergeThreshold:
		// ── Merge into the best cluster (mirrors _llm_merge + re-embed) ──
		c := t.clusters[bestName]
		newDesc, err := llmMerge(ctx, deps, llmID, c.Desc, text)
		if err != nil {
			return nil, err
		}
		if newDesc != c.Desc {
			c.Desc = newDesc
			emb, err := deps.Embed.Encode(ctx, []string{newDesc})
			if err != nil {
				return nil, err
			}
			if len(emb) > 0 {
				c.Vector = emb[0]
			}
		}
		if !containsString(c.DocIDs, chunkID) {
			c.DocIDs = append(c.DocIDs, chunkID)
		}
		t.addDoc(c, chunkID, text, vec, c.Depth+1)
		if err := t.maybeSplit(ctx, deps, llmID, bestName); err != nil {
			return nil, err
		}
		return nil, nil

	case bestName != "" && sim >= minSim:
		// ── Create a sibling/child cluster (mirrors the _MIN_SIM branch) ──
		parentForNew := bestParent
		if parentForNew == "" {
			parentForNew = bestName
		}
		parentDepth := 1 // Python's default when the parent row is absent
		if p, ok := t.clusters[parentForNew]; ok {
			parentDepth = p.Depth
		}
		title, desc, err := llmCreateSummary(ctx, deps, llmID, []string{text})
		if err != nil {
			return nil, err
		}
		name := readableClusterName(title, text)
		nc := &navCluster{
			Name:   name,
			Desc:   desc,
			Parent: parentForNew,
			// Python stamps the new cluster at the PARENT's depth (not +1);
			// the nav_doc goes one deeper. Mirrored, quirk included.
			Depth:  parentDepth,
			DocIDs: []string{chunkID},
			Vector: vec,
		}
		t.addCluster(nc)
		t.children[parentForNew] = append(t.children[parentForNew], navChild{name: name, vec: vec})
		t.addDoc(nc, chunkID, text, vec, parentDepth+1)
		return []string{desc}, nil

	default:
		// ── Create a root-level cluster (mirrors the else branch) ──
		title, desc, err := llmCreateSummary(ctx, deps, llmID, []string{text})
		if err != nil {
			return nil, err
		}
		name := readableClusterName(title, text)
		nc := &navCluster{
			Name:   name,
			Desc:   desc,
			Parent: "root",
			Depth:  0,
			DocIDs: []string{chunkID},
			Vector: vec,
		}
		t.addCluster(nc)
		t.addDoc(nc, chunkID, text, vec, 1)
		return []string{desc}, nil
	}
}

func (t *navTree) addDoc(c *navCluster, chunkID, text string, vec []float32, depth int) {
	t.docs = append(t.docs, &navDoc{ChunkID: chunkID, Text: text, Parent: c.Name, Depth: depth, Vector: vec})
	t.children[c.Name] = append(t.children[c.Name], navChild{name: chunkID, isDoc: true, vec: vec})
}

// findBestCluster mirrors _find_best_cluster: top-1 root cluster by cosine,
// then descend into the best-matching child while similarity stays above
// _RECURSE_THRESHOLD. Returns (best cluster name, its parent name, sim).
func (t *navTree) findBestCluster(vec []float32) (string, string, float64) {
	var best *navCluster
	bestSim := 0.0
	for _, c := range t.clusters {
		if c.Depth != 0 {
			continue
		}
		s := cosine(vec, c.Vector)
		if best == nil || s > bestSim {
			best, bestSim = c, s
		}
	}
	if best == nil {
		return "", "", 0.0
	}
	bestName, bestParent, sim := best.Name, best.Parent, bestSim
	visited := map[string]bool{bestName: true}
	for sim >= recurseThreshold {
		var child *navCluster
		childSim := 0.0
		for _, k := range t.children[bestName] {
			if k.isDoc || visited[k.name] {
				continue
			}
			c := t.clusters[k.name]
			if c == nil {
				continue
			}
			if s := cosine(vec, c.Vector); child == nil || s > childSim {
				child, childSim = c, s
			}
		}
		if child == nil || childSim < recurseThreshold {
			break
		}
		bestParent = best.Parent // the (old) best's parent, mirroring Python
		bestName = child.Name
		sim = childSim
		best = child
		visited[bestName] = true
	}
	return bestName, bestParent, sim
}

// maybeSplit mirrors _maybe_split_cluster: when a cluster exceeds the fanout
// or doc-count caps, split its children into two 2-means groups and reparent
// each group under a fresh sub-cluster.
func (t *navTree) maybeSplit(ctx context.Context, deps common.Deps, llmID, clusterName string) error {
	kids := t.children[clusterName]
	var clusterKids, docKids []navChild
	for _, k := range kids {
		if k.isDoc {
			docKids = append(docKids, k)
		} else {
			clusterKids = append(clusterKids, k)
		}
	}
	if len(clusterKids)+len(docKids) <= maxFanout && len(docKids) <= maxDocsPerCluster {
		return nil
	}
	if len(kids) < 4 {
		return nil // Python requires >= 4 embeddings to attempt a split
	}

	embs := make([][]float32, len(kids))
	for i, k := range kids {
		embs[i] = k.vec
	}
	labels := twoMeans(embs)

	parent := t.clusters[clusterName]
	depth := 0
	if parent != nil {
		depth = parent.Depth + 1
	}
	// navChild collects the new sub-cluster names so the split parent keeps a
	// children entry pointing at them; without it findBestCluster can no longer
	// descend into the split subtree (M3).
	var subChildren []navChild
	for gi := 0; gi < 2; gi++ {
		var kidIdx []int
		for i, lb := range labels {
			if lb == gi {
				kidIdx = append(kidIdx, i)
			}
		}
		if len(kidIdx) == 0 {
			continue
		}
		var docIDs, descs []string
		for _, i := range kidIdx {
			k := kids[i]
			if k.isDoc {
				if !containsString(docIDs, k.name) {
					docIDs = append(docIDs, k.name)
				}
				for _, d := range t.docs {
					if d.ChunkID == k.name {
						descs = append(descs, d.Text)
					}
				}
				continue
			}
			if c := t.clusters[k.name]; c != nil {
				for _, d := range c.DocIDs {
					if !containsString(docIDs, d) {
						docIDs = append(docIDs, d)
					}
				}
				descs = append(descs, c.Desc)
			}
		}
		var title, desc string
		if len(descs) > 0 {
			var err error
			title, desc, err = llmCreateSummary(ctx, deps, llmID, descs)
			if err != nil {
				return err
			}
		} else {
			title, desc = fmt.Sprintf("Group %d", gi+1), fmt.Sprintf("Group %d", gi+1)
		}
		gname := readableClusterName(title, desc)
		emb, err := deps.Embed.Encode(ctx, []string{desc})
		if err != nil {
			return err
		}
		var gvec []float32
		if len(emb) > 0 {
			gvec = emb[0]
		}
		nc := &navCluster{Name: gname, Desc: desc, Parent: clusterName, Depth: depth, DocIDs: docIDs, Vector: gvec}
		t.addCluster(nc)
		subChildren = append(subChildren, navChild{name: gname})

		// Reparent the group's children to the new sub-cluster.
		for _, i := range kidIdx {
			k := kids[i]
			if k.isDoc {
				for _, d := range t.docs {
					if d.ChunkID == k.name {
						d.Parent = gname
						d.Depth = depth + 1
					}
				}
			} else if c := t.clusters[k.name]; c != nil {
				c.Parent = gname
				c.Depth = depth + 1
			}
			t.children[gname] = append(t.children[gname], k)
		}
	}
	// The split parent's children become the new sub-clusters so findBestCluster
	// can still descend into the split subtree (M3). When no sub-cluster was
	// actually created the parent keeps an empty children list.
	t.children[clusterName] = subChildren
	return nil
}

// twoMeans mirrors the runtime k-means-like split in _maybe_split_cluster:
// centroids seeded with the first and middle embeddings, 10 refinement
// rounds, squared-euclidean assignment, then a final relabel pass.
func twoMeans(embs [][]float32) []int {
	n := len(embs)
	if n == 0 {
		return nil
	}
	centroids := [][]float32{append([]float32{}, embs[0]...), append([]float32{}, embs[n/2]...)}
	assign := func(e []float32) int {
		if sqDist(e, centroids[0]) < sqDist(e, centroids[1]) {
			return 0
		}
		return 1
	}
	for iter := 0; iter < 10; iter++ {
		var groups [2][][]float32
		for _, e := range embs {
			g := assign(e)
			groups[g] = append(groups[g], e)
		}
		for gi := 0; gi < 2; gi++ {
			if len(groups[gi]) == 0 {
				continue
			}
			dim := len(groups[gi][0])
			avg := make([]float32, dim)
			for _, e := range groups[gi] {
				for d := 0; d < dim; d++ {
					avg[d] += e[d]
				}
			}
			for d := range avg {
				avg[d] /= float32(len(groups[gi]))
			}
			centroids[gi] = avg
		}
	}
	labels := make([]int, n)
	for i, e := range embs {
		labels[i] = assign(e)
	}
	return labels
}

func sqDist(a, b []float32) float64 {
	var s float64
	for i := 0; i < len(a) && i < len(b); i++ {
		d := float64(a[i] - b[i])
		s += d * d
	}
	return s
}

func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// ---- LLM helpers (prompts mirrored verbatim from dataset_nav.py) ----

var navLLMTemperature = 0.1

// llmMerge mirrors _llm_merge: fuse the existing cluster description with the
// new doc summary. The reply may be a JSON object ({"merged"|"result"}) or
// bare text; anything unusable keeps the old description.
func llmMerge(ctx context.Context, deps common.Deps, llmID, clusterDesc, docSummary string) (string, error) {
	if deps.Chat == nil {
		return clusterDesc, nil
	}
	prompt := "Merge the following two descriptions of the same topic into " +
		"a single concise summary (1-3 sentences):\n\n" +
		"Existing: " + clusterDesc + "\n\n" +
		"New: " + docSummary + "\n\n" +
		"Return ONLY the merged text, no commentary."
	resp, err := deps.Chat.Chat(ctx, common.ChatRequest{LLMID: llmID, UserPrompt: prompt, Temperature: &navLLMTemperature})
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(resp.Content)
	if m := parseWholeJSON(text); m != nil {
		if s, ok := m["merged"].(string); ok && strings.TrimSpace(s) != "" {
			return s, nil
		}
		if s, ok := m["result"].(string); ok && strings.TrimSpace(s) != "" {
			return s, nil
		}
		return clusterDesc, nil
	}
	if text != "" {
		return text, nil
	}
	return clusterDesc, nil
}

// llmCreateSummary mirrors _llm_create_summary: derive a readable (name,
// summary) pair from doc summaries, with the same fallbacks.
func llmCreateSummary(ctx context.Context, deps common.Deps, llmID string, docSummaries []string) (string, string, error) {
	fallbackSummary := ""
	if len(docSummaries) > 0 {
		fallbackSummary = docSummaries[0]
	}
	if deps.Chat == nil {
		return fallbackTitle(fallbackSummary), fallbackSummary, nil
	}
	prompt := "Given the document excerpts below, produce a short human-readable topic " +
		"name and a concise description of their common topic.\n\n" +
		strings.Join(docSummaries, "\n---\n") + "\n\n" +
		`Return ONLY JSON: {"name": "<2-6 word topic title>", "summary": "<1-3 sentence description>"}`
	resp, err := deps.Chat.Chat(ctx, common.ChatRequest{LLMID: llmID, UserPrompt: prompt, Temperature: &navLLMTemperature})
	if err != nil {
		return "", "", err
	}
	text := strings.TrimSpace(resp.Content)
	if m := parseWholeJSON(text); m != nil {
		summary := fallbackSummary
		if s, ok := m["summary"].(string); ok && strings.TrimSpace(s) != "" {
			summary = strings.TrimSpace(s)
		} else if s, ok := m["result"].(string); ok && strings.TrimSpace(s) != "" {
			summary = strings.TrimSpace(s)
		}
		name := cleanTitle(firstStringOf(m["name"]))
		if name == "" {
			name = fallbackTitle(summary)
		}
		return name, summary, nil
	}
	if text != "" {
		return fallbackTitle(text), text, nil
	}
	return fallbackTitle(fallbackSummary), fallbackSummary, nil
}

// parseWholeJSON parses the response ONLY when it is a complete JSON object
// (mirrors gen_json's dict path; bare text takes the string fallback path).
func parseWholeJSON(s string) map[string]any {
	if !strings.HasPrefix(s, "{") {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	return m
}

// cleanTitle mirrors _clean_title: whitespace-normalized, capped at 48 chars.
func cleanTitle(title string) string {
	return truncateRunes(strings.Join(strings.Fields(title), " "), 48)
}

// fallbackTitle mirrors _fallback_title: first 6 words, else "Cluster".
func fallbackTitle(summary string) string {
	words := strings.Fields(summary)
	if len(words) > 6 {
		words = words[:6]
	}
	if t := strings.Join(words, " "); t != "" {
		return t
	}
	return "Cluster"
}

// readableClusterName mirrors _readable_cluster_name: "<title> <8-hex>".
func readableClusterName(title, seed string) string {
	t := cleanTitle(title)
	if t == "" {
		t = "Cluster"
	}
	return t + " " + common.ContentHash(seed)[:8]
}

// navDocName mirrors _make_nav_doc_row's name field:
// f"{parent_kwd}_{xxh64(summary)[:12]}".
func navDocName(parent, summary string) string {
	return parent + "_" + common.ContentHash(summary)[:12]
}

// summarize is the plain-text LLM helper used for the root overview.
func summarize(ctx context.Context, deps common.Deps, llmID, text string) (string, error) {
	if deps.Chat == nil {
		return "", nil
	}
	resp, err := deps.Chat.Chat(ctx, common.ChatRequest{
		LLMID:        llmID,
		SystemPrompt: navSystemPrompt,
		UserPrompt:   text,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}

// formatNavSummaries renders cluster descriptions as a bullet list for the
// root overview prompt.
func formatNavSummaries(summaries []string) string {
	var b strings.Builder
	for _, s := range summaries {
		b.WriteString("- ")
		b.WriteString(s)
		b.WriteString("\n")
	}
	return b.String()
}

// mergeThresholdFor returns the merge threshold (nav_radius override kept for
// tuning/tests; default _MERGE_THRESHOLD).
func mergeThresholdFor(param common.Param) float64 {
	if t, ok := param.Extra["nav_radius"].(float64); ok && t > 0 {
		return t
	}
	return mergeThresholdDefault
}

// chunkTexts returns the non-empty chunk texts and their ids in parallel
// order (Python skips docs without a summary; we skip empty chunks).
func chunkTexts(chunks []common.Chunk) ([]string, []string) {
	var texts, ids []string
	for i, c := range chunks {
		t := firstNonEmpty(c.Text, c.Content)
		if strings.TrimSpace(t) == "" {
			continue
		}
		id := c.ID
		if id == "" {
			id = fmt.Sprintf("chunk-%d", i)
		}
		texts = append(texts, t)
		ids = append(ids, id)
	}
	return texts, ids
}

func payloadJSON(v map[string]any) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "{}"
	}
	return strings.TrimSpace(b.String())
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func firstStringOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

const navSystemPrompt = `You are a navigation assistant. Summarize the provided text into a concise label and overview that helps a user navigate to the relevant content. Output the summary only.`
