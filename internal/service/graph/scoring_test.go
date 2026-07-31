package graph

import "testing"

// TestAnalyzeNHopPathsPageRankMaxWins verifies that when the same edge (f, t)
// appears from multiple N-hop paths with different weights, the maximum weight
// is kept (not the last-write-wins value).
//
// Regression test for #15695 — PageRank must use max-wins to match the
// Python equivalent in rag/graphrag/search.py:186.
//
// Fixture: both paths share the (mid, dst) edge so the second NhopEntity
// produces a competing weight for it. Max-wins should keep 0.7 (from path 1)
// over 0.5 (from path 2). Under the old last-wins code the value would be 0.5.
func TestAnalyzeNHopPathsPageRankMaxWins(t *testing.T) {
	ents := map[string]*KGEntity{
		"src": {
			Name:       "src",
			Similarity: 0.5,
			NhopEnts: []NhopEntity{
				{
					Path:    []string{"src", "mid", "dst"},
					Weights: []float64{0.3, 0.7}, // mid->dst at i=1, weight 0.7
				},
				{
					Path:    []string{"alt", "mid", "dst"},
					Weights: []float64{0.4, 0.5}, // mid->dst at i=1, weight 0.5
				},
			},
		},
	}

	got := AnalyzeNHopPaths(ents)

	// Repeated edge mid->dst: weights 0.7 (path 1) and 0.5 (path 2).
	// Max-wins should keep 0.7, not the last-wins 0.5.
	if v, ok := got[Edge{"mid", "dst"}]; !ok {
		t.Fatalf("missing edge mid->dst; got keys: %v", mapKeys(got))
	} else if v.PageRank != 0.7 {
		t.Errorf("edge mid->dst PageRank = %v, want 0.7 (max-wins)", v.PageRank)
	}

	// Sanity-check edges that appear in only one path.
	if v, ok := got[Edge{"src", "mid"}]; !ok {
		t.Fatalf("missing edge src->mid; got keys: %v", mapKeys(got))
	} else if v.PageRank != 0.3 {
		t.Errorf("edge src->mid PageRank = %v, want 0.3", v.PageRank)
	}
	if v, ok := got[Edge{"alt", "mid"}]; !ok {
		t.Fatalf("missing edge alt->mid; got keys: %v", mapKeys(got))
	} else if v.PageRank != 0.4 {
		t.Errorf("edge alt->mid PageRank = %v, want 0.4", v.PageRank)
	}
}

// TestAnalyzeNHopPathsPageRankGoOrderingIndependent reuses the same shared
// (mid, dst) edge with the NhopEnts order reversed — under the old
// last-wins code the mid->dst PageRank would flip to 0.5 (last write).
// Max-wins must yield 0.7 regardless of path iteration order.
func TestAnalyzeNHopPathsPageRankGoOrderingIndependent(t *testing.T) {
	ents := map[string]*KGEntity{
		"src": {
			Name:       "src",
			Similarity: 0.5,
			NhopEnts: []NhopEntity{
				{
					Path:    []string{"alt", "mid", "dst"},
					Weights: []float64{0.4, 0.5}, // processed first
				},
				{
					Path:    []string{"src", "mid", "dst"},
					Weights: []float64{0.3, 0.7}, // processed second; old code would overwrite to 0.5
				},
			},
		},
	}

	got := AnalyzeNHopPaths(ents)
	if v, ok := got[Edge{"mid", "dst"}]; !ok {
		t.Fatalf("missing edge mid->dst; got keys: %v", mapKeys(got))
	} else if v.PageRank != 0.7 {
		t.Errorf("edge mid->dst PageRank = %v, want 0.7 (max-wins across orderings)", v.PageRank)
	}
}

func mapKeys(m map[Edge]EdgeScore) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k.From+"->"+k.To)
	}
	return keys
}
