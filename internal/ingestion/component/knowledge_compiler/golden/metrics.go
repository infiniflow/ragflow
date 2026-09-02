package golden

import (
	"encoding/json"
	"strings"

	"ragflow/internal/ingestion/component/schema"
)

// TreeMetrics summarizes the structural shape of a tree product tree.
type TreeMetrics struct {
	ProductCount int
	RootCount    int
	LeafClusters int // number of level-0 summary nodes (bottom clusters)
	MaxDepth     int // root(0) .. deepest summary level
	AllParented  bool
	VectorOK     bool // every product carries a non-empty vector
	SchemaOK     bool // every product carries the schema fields
	// CoveredSources is the number of distinct source chunk IDs referenced by
	// level-0 leaf clusters via their source_chunk_ids meta. It measures how
	// completely the input corpus is represented by the tree, independent of
	// the tree's structural well-formedness.
	CoveredSources int
	// covered is the working set of distinct source chunk IDs seen so far.
	covered map[string]bool
}

// AnalyzeTreeProducts validates tree integrity and computes structural
// metrics from a flat chunk list (the compiled tree output, expressed as
// schema.ChunkDoc values). Used by the 缺口 C golden gate.
//
// validSourceIDs, when provided, limits coverage counting to source chunk IDs
// that actually belong to the input corpus. This prevents an untrusted
// source_chunk_ids (e.g. a leaked/garbage ID) from inflating CoveredSources
// past nChunks and pushing CoverageFraction above 1.0. When empty, all
// source_chunk_ids are counted (backward compatible for unit tests that build
// synthetic trees).
func AnalyzeTreeProducts(chunks []schema.ChunkDoc, validSourceIDs ...string) TreeMetrics {
	ids := make(map[string]bool, len(chunks))
	for _, c := range chunks {
		if id, ok := c.GetExtraString("id"); ok {
			ids[id] = true
		}
	}
	validSet := make(map[string]bool, len(validSourceIDs))
	for _, id := range validSourceIDs {
		validSet[id] = true
	}
	checkValid := len(validSet) > 0
	m := TreeMetrics{AllParented: true, VectorOK: true, SchemaOK: true, covered: make(map[string]bool)}
	maxLevel := -1
	for _, c := range chunks {
		kind, _ := c.GetExtraString("kc_kind")
		// Auxiliary tree-graph rows (kind entity/relation/graph, from the
		// structure-graph projection) are not part of the RAPTOR tree structure,
		// so they are excluded from the structural metrics (ProductCount,
		// AllParented, SchemaOK, ...). The tree itself is the root/summary set.
		if kind != "root" && kind != "summary" {
			continue
		}
		m.ProductCount++
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
				// Accumulate the distinct source chunk IDs this leaf cluster
				// was built from. Every input chunk is assigned to exactly one
				// level-0 cluster in buildTree, so the union of these sets is
				// the set of covered source chunks. Only IDs that belong to the
				// input corpus count, so an unknown ID cannot inflate coverage.
				if src, ok := c.GetExtraStringSlice("source_chunk_ids"); ok {
					for _, id := range src {
						if checkValid && !validSet[id] {
							continue
						}
						if !m.covered[id] {
							m.covered[id] = true
							m.CoveredSources++
						}
					}
				}
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
// the tree. It is the ratio of distinct source chunk IDs referenced by the
// level-0 leaf clusters (CoveredSources) to the total input chunk count
// (nChunks). This detects dropped source chunks: a structurally well-formed
// tree that silently omits input chunks will score below 1.0.
func (m TreeMetrics) CoverageFraction(nChunks int) float64 {
	if nChunks <= 0 {
		return 0
	}
	if m.CoveredSources <= 0 {
		return 0
	}
	// Coverage can never exceed 1.0: a source chunk is covered at most once,
	// and only corpus IDs are counted, so CoveredSources <= nChunks.
	frac := float64(m.CoveredSources) / float64(nChunks)
	if frac > 1.0 {
		return 1.0
	}
	return frac
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
