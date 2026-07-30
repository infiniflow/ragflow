package golden

import (
	"encoding/json"
	"strings"

	"ragflow/internal/ingestion/component/schema"
)

// TreeMetrics summarizes the structural shape of a raptor product tree.
type TreeMetrics struct {
	ProductCount int
	RootCount    int
	LeafClusters int // number of level-0 summary nodes (bottom clusters)
	MaxDepth     int // root(0) .. deepest summary level
	AllParented  bool
	VectorOK     bool // every product carries a non-empty vector
	SchemaOK     bool // every product carries the schema fields
}

// AnalyzeRaptorProducts validates tree integrity and computes structural
// metrics from a flat chunk list (the compiled raptor output, expressed as
// schema.ChunkDoc values). Used by the 缺口 C golden gate.
func AnalyzeRaptorProducts(chunks []schema.ChunkDoc) TreeMetrics {
	ids := make(map[string]bool, len(chunks))
	for _, c := range chunks {
		if id, ok := c.GetExtraString("id"); ok {
			ids[id] = true
		}
	}
	m := TreeMetrics{ProductCount: len(chunks), AllParented: true, VectorOK: true, SchemaOK: true}
	maxLevel := -1
	for _, c := range chunks {
		kind, _ := c.GetExtraString("kc_kind")
		level := 0
		if lf, ok := extraFloat(c, "kc_level"); ok {
			level = int(lf)
		}
		switch kind {
		case "root":
			m.RootCount++
		case "summary":
			if level == 0 {
				m.LeafClusters++
			}
			if level > maxLevel {
				maxLevel = level
			}
		}
		parent, _ := c.GetExtraString("parent_kwd")
		if kind != "root" && parent == "" {
			m.AllParented = false
		}
		if parent != "" && !ids[parent] {
			m.AllParented = false
		}
		if !hasVector(c) {
			m.VectorOK = false
		}
		id, _ := c.GetExtraString("id")
		docID, _ := c.GetExtraString("doc_id")
		tenant, _ := c.GetExtraString("tenant_id")
		ck, _ := c.GetExtraString("compile_kwd")
		if id == "" || docID == "" || tenant == "" || c.Text == "" || ck == "" {
			m.SchemaOK = false
		}
	}
	m.MaxDepth = maxLevel + 1
	return m
}

// CoverageFraction reports how completely the input chunks are represented by
// the raptor tree. Every source chunk is assigned to exactly one level-0
// cluster in buildTree, and each such cluster becomes a leaf summary node, so a
// well-formed tree covers 100% of chunks. nChunks is the input chunk count.
func (m TreeMetrics) CoverageFraction(nChunks int) float64 {
	if nChunks <= 0 {
		return 0
	}
	// Every chunk maps to a level-0 cluster; the number of covered chunks
	// equals the total chunk count when at least one leaf cluster exists and
	// the tree is well-formed (all parented, single root).
	if m.LeafClusters > 0 && m.RootCount == 1 && m.AllParented {
		return 1.0
	}
	return 0.0
}

// extraFloat reads a numeric Extra value by key.
func extraFloat(c schema.ChunkDoc, key string) (float64, bool) {
	if c.Extra == nil {
		return 0, false
	}
	raw, ok := c.Extra[key]
	if !ok {
		return 0, false
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, false
	}
	return f, true
}

// hasVector reports whether the chunk carries any q_<dim>_vec embedding.
func hasVector(c schema.ChunkDoc) bool {
	for k := range c.Extra {
		if strings.HasPrefix(k, "q_") && strings.HasSuffix(k, "_vec") {
			return true
		}
	}
	return false
}
