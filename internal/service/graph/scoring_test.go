package graph

import "testing"

// TestAnalyzeNHopPathsPageRankMaxWins verifies that when the same edge (f, t)
// appears from multiple N-hop paths with different weights, the maximum weight
// is kept (not the last-write-wins value).
//
// Regression test for #15695 — AnalyzeNHopPaths must take the max
// PageRank over all hops for the same edge, not just overwrite it on
// each hop. The earlier overwrite-only behaviour gave later hops
// the final PageRank weight regardless of magnitude.
func TestAnalyzeNHopPathsPageRankMaxWins(t *testing.T) {
	ents := map[string]*KGEntity{
		"src": {
			Name:       "src",
			Similarity: 0.5,
			NhopEnts: []NhopEntity{
				{
					Path:    []string{"src", "mid", "dst"},
					Weights: []float64{0.3, 0.7}, // weak first hop, strong second hop
				},
				{
					Path:    []string{"src", "alt", "dst"},
					Weights: []float64{0.9, 0.1}, // strong first hop, weak second hop
				},
			},
		},
	}

	got := AnalyzeNHopPaths(ents)

	// Edge src->mid: weights 0.3 (first path) and 0.9 (second path, via alt->dst)
	// Max should win: 0.9, not 0.1 (last-wins).
	if v, ok := got[Edge{"src", "mid"}]; !ok {
		t.Fatalf("missing edge src->mid; got map keys: %v", mapKeys(got))
	} else if v.PageRank != 0.9 {
		t.Errorf("edge src->mid PageRank = %v, want 0.9 (max-wins)", v.PageRank)
	}

	// Edge src->alt: only appears once, weight 0.9.
	if v, ok := got[Edge{"src", "alt"}]; !ok {
		t.Fatalf("missing edge src->alt")
	} else if v.PageRank != 0.9 {
		t.Errorf("edge src->alt PageRank = %v, want 0.9", v.PageRank)
	}

	// Edge mid->dst: weight 0.7 (first path only).
	if v, ok := got[Edge{"mid", "dst"}]; !ok {
		t.Fatalf("missing edge mid->dst")
	} else if v.PageRank != 0.7 {
		t.Errorf("edge mid->dst PageRank = %v, want 0.7", v.PageRank)
	}
}

// TestAnalyzeNHopPathsPageRankGoOrderingIndependent is the same scenario but
// with the second (stronger) path encountered first — under the old last-wins
// code this would overwrite to 0.1 and lose the 0.9.
func TestAnalyzeNHopPathsPageRankGoOrderingIndependent(t *testing.T) {
	ents := map[string]*KGEntity{
		"src": {
			Name:       "src",
			Similarity: 0.5,
			NhopEnts: []NhopEntity{
				{
					Path:    []string{"src", "alt", "dst"},
					Weights: []float64{0.9, 0.1}, // processed first
				},
				{
					Path:    []string{"src", "mid", "dst"},
					Weights: []float64{0.3, 0.7}, // processed second; old code would keep 0.3
				},
			},
		},
	}

	got := AnalyzeNHopPaths(ents)
	if v := got[Edge{"src", "mid"}]; v.PageRank != 0.7 {
		t.Errorf("edge src->mid PageRank = %v, want 0.7 (max-wins across orderings)", v.PageRank)
	}
	if v := got[Edge{"src", "alt"}]; v.PageRank != 0.9 {
		t.Errorf("edge src->alt PageRank = %v, want 0.9", v.PageRank)
	}
}

func mapKeys(m map[Edge]EdgeScore) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k.From+"->"+k.To)
	}
	return keys
}
