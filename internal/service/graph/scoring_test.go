package graph

import "testing"

// TestAnalyzeNHopPathsPageRankMaxWins verifies that when the same edge (f, t)
// appears from multiple N-hop paths with different weights, the maximum weight
// is kept (not the last-write-wins value).
//
// Regression test for #15695 — PageRank must use max-wins to match the
// Python equivalent in rag/graphrag/search.py:186.
//
// The two paths deliberately share the (src, mid) edge so the test
// actually exercises the max-wins branch; disjoint paths would only
// exercise first-write-wins semantics and silently pass either way.
func TestAnalyzeNHopPathsPageRankMaxWins(t *testing.T) {
	ents := map[string]*KGEntity{
		"src": {
			Name:       "src",
			Similarity: 0.5,
			NhopEnts: []NhopEntity{
				{
					Path:    []string{"src", "mid", "dst"},
					Weights: []float64{0.3, 0.7}, // shared edge src->mid at weight 0.3
				},
				{
					Path:    []string{"src", "mid", "other"},
					Weights: []float64{0.9, 0.2}, // shared edge src->mid at weight 0.9
				},
			},
		},
	}

	got := AnalyzeNHopPaths(ents)

	// Edge src->mid: contributions 0.3 and 0.9 — max-wins must keep 0.9.
	// The reversed-order case below detects last-wins regressions.
	if v, ok := got[Edge{"src", "mid"}]; !ok {
		t.Fatalf("missing edge src->mid; got map keys: %v", mapKeys(got))
	} else if v.PageRank != 0.9 {
		t.Errorf("edge src->mid PageRank = %v, want 0.9 (max-wins)", v.PageRank)
	}

	// Edge mid->dst: weight 0.7 from the first path only.
	if v, ok := got[Edge{"mid", "dst"}]; !ok {
		t.Fatalf("missing edge mid->dst")
	} else if v.PageRank != 0.7 {
		t.Errorf("edge mid->dst PageRank = %v, want 0.7", v.PageRank)
	}

	// Edge mid->other: weight 0.2 from the second path only.
	if v, ok := got[Edge{"mid", "other"}]; !ok {
		t.Fatalf("missing edge mid->other")
	} else if v.PageRank != 0.2 {
		t.Errorf("edge mid->other PageRank = %v, want 0.2", v.PageRank)
	}
}

// TestAnalyzeNHopPathsPageRankGoOrderingIndependent is the same scenario but
// with the two paths supplied in the opposite order. Under the old last-wins
// code, the value of the shared edge (src, mid) would depend on which path
// was processed last; max-wins makes the result ordering-independent.
func TestAnalyzeNHopPathsPageRankGoOrderingIndependent(t *testing.T) {
	ents := map[string]*KGEntity{
		"src": {
			Name:       "src",
			Similarity: 0.5,
			NhopEnts: []NhopEntity{
				{
					// Stronger src->mid contribution processed first.
					Path:    []string{"src", "mid", "other"},
					Weights: []float64{0.9, 0.2},
				},
				{
					// Weaker src->mid contribution processed second.
					Path:    []string{"src", "mid", "dst"},
					Weights: []float64{0.3, 0.7},
				},
			},
		},
	}

	got := AnalyzeNHopPaths(ents)

	// Shared edge must still be 0.9 regardless of which path ran first.
	if v, ok := got[Edge{"src", "mid"}]; !ok {
		t.Fatalf("missing edge src->mid")
	} else if v.PageRank != 0.9 {
		t.Errorf("edge src->mid PageRank = %v, want 0.9 (max-wins across orderings)", v.PageRank)
	}

	// Edges unique to each path keep their own weights.
	if v, ok := got[Edge{"mid", "other"}]; !ok {
		t.Fatalf("missing edge mid->other")
	} else if v.PageRank != 0.2 {
		t.Errorf("edge mid->other PageRank = %v, want 0.2", v.PageRank)
	}
	if v, ok := got[Edge{"mid", "dst"}]; !ok {
		t.Fatalf("missing edge mid->dst")
	} else if v.PageRank != 0.7 {
		t.Errorf("edge mid->dst PageRank = %v, want 0.7", v.PageRank)
	}
}

func mapKeys(m map[Edge]EdgeScore) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k.From+"->"+k.To)
	}
	return keys
}
