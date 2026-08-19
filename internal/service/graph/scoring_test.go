package graph

import "testing"

// TestAnalyzeNHopPathsPageRankMaxWins asserts that when the same edge
// appears from multiple N-hop paths with different weights, the larger
// weight wins regardless of the iteration order of the source map.
//
// The Go runtime randomizes map iteration order, so the test runs the
// assertion in a loop — a single order-dependent failure here would only
// catch the bug ~once-in-N runs without the loop.
func TestAnalyzeNHopPathsPageRankMaxWins(t *testing.T) {
	// Two distinct query entities (qA, qB) each reach mid->dst along a
	// different N-hop path with a different weight. The bug under test
	// is that the SECOND-processed path overwrites the first's PageRank
	// (last-wins). Under the fix, the larger weight always wins.
	const wantPageRank = 0.7 // max(0.4, 0.7)

	for i := 0; i < 50; i++ {
		ents := map[string]*KGEntity{
			"qA": {
				Name:       "qA",
				Similarity: 0.5,
				NhopEnts: []NhopEntity{
					{
						Path:    []string{"qA", "mid", "dst"},
						Weights: []float64{0.3, 0.4}, // mid->dst weight 0.4
					},
				},
			},
			"qB": {
				Name:       "qB",
				Similarity: 0.5,
				NhopEnts: []NhopEntity{
					{
						Path:    []string{"qB", "mid", "dst"},
						Weights: []float64{0.2, 0.7}, // mid->dst weight 0.7
					},
				},
			},
		}

		got := AnalyzeNHopPaths(ents)
		es, ok := got[Edge{"mid", "dst"}]
		if !ok {
			t.Fatalf("iteration %d: missing edge mid->dst; got keys: %v", i, mapKeys(got))
		}
		if es.PageRank != wantPageRank {
			t.Errorf("iteration %d: edge mid->dst PageRank = %v, want %v (max-wins; the lower weight must never overwrite the higher one)", i, es.PageRank, wantPageRank)
		}
	}
}

// TestAnalyzeNHopPathsPageRankPathLongerThanWeights pins down the
// `i < len(weights)` guard: when the path is longer than the weights
// slice, the code must NOT panic on `weights[i]`, and the partial-edge
// PageRank is whatever the last in-range weight was (or zero if the
// edge never got a weight at all).
func TestAnalyzeNHopPathsPageRankPathLongerThanWeights(t *testing.T) {
	t.Run("no panic when path outruns weights", func(t *testing.T) {
		// Path "a -> b -> c -> d" has indices 0,1,2 for edges a->b, b->c, c->d.
		// Weights of length 2 means i=2 (c->d) is out-of-range; the guard
		// must skip the PageRank assignment instead of indexing weights[2].
		ents := map[string]*KGEntity{
			"src": {
				Name:       "src",
				Similarity: 0.5,
				NhopEnts: []NhopEntity{
					{
						Path:    []string{"a", "b", "c", "d"},
						Weights: []float64{0.9, 0.1}, // length 2, path length 4
					},
				},
			},
		}

		// Just exercising the path without a panic is the primary
		// assertion of this test case.
		got := AnalyzeNHopPaths(ents)
		es, ok := got[Edge{"c", "d"}]
		if !ok {
			t.Fatalf("missing edge c->d; got keys: %v", mapKeys(got))
		}
		// No PageRank was assigned for c->d (weights out of range), so
		// it stays at the zero value of float64.
		if es.PageRank != 0 {
			t.Errorf("edge c->d PageRank = %v, want 0 (no in-range weight)", es.PageRank)
		}
	})

	t.Run("in-range weights still apply", func(t *testing.T) {
		// Sanity: when the path is shorter than (or equal to) Weights,
		// the in-range weights still apply (this is the path covered by
		// TestAnalyzeNHopPathsPageRankMaxWins, but spelled out here).
		ents := map[string]*KGEntity{
			"src": {
				Name:       "src",
				Similarity: 0.5,
				NhopEnts: []NhopEntity{
					{
						Path:    []string{"a", "b", "c"},
						Weights: []float64{0.9, 0.1}, // length 2, path length 3
					},
				},
			},
		}
		got := AnalyzeNHopPaths(ents)
		if es, ok := got[Edge{"a", "b"}]; !ok || es.PageRank != 0.9 {
			t.Errorf("edge a->b PageRank = %v, want 0.9", es.PageRank)
		}
		if es, ok := got[Edge{"b", "c"}]; !ok || es.PageRank != 0.1 {
			t.Errorf("edge b->c PageRank = %v, want 0.1", es.PageRank)
		}
	})
}

func mapKeys(m map[Edge]EdgeScore) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k.From+"->"+k.To)
	}
	return keys
}
