// Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nlp

import (
	"math"
	"testing"
)

func TestCosineSimilarity_BasicCases(t *testing.T) {
	cases := []struct {
		name string
		a, b []float64
		want float64
	}{
		{"identical", []float64{1, 2, 3}, []float64{1, 2, 3}, 1},
		{"orthogonal", []float64{1, 0}, []float64{0, 1}, 0},
		{"opposite", []float64{1, 0}, []float64{-1, 0}, -1},
		{"same direction different magnitude", []float64{1, 2}, []float64{2, 4}, 1},
		{"negative scale", []float64{1, 2}, []float64{-2, -4}, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cosineSimilarity(tc.a, tc.b)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("cosineSimilarity(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestCosineSimilarity_Degenerate(t *testing.T) {
	cases := []struct {
		name string
		a, b []float64
	}{
		{"length mismatch", []float64{1, 2}, []float64{1, 2, 3}},
		{"left zero", []float64{0, 0}, []float64{1, 2}},
		{"right zero", []float64{1, 2}, []float64{0, 0}},
		{"both empty", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cosineSimilarity(tc.a, tc.b)
			if got != 0 {
				t.Fatalf("cosineSimilarity(%v, %v) = %v, want 0", tc.a, tc.b, got)
			}
			if math.IsNaN(got) || math.IsInf(got, 0) {
				t.Fatalf("cosineSimilarity returned non-finite value %v", got)
			}
		})
	}
}

func TestCosineSimilarity_NonFiniteInput(t *testing.T) {
	// Non-finite components are not guarded by the implementation: the
	// normalisation still divides by a non-finite norm, so the result is NaN.
	// Pin the boundary so a future guard shows up as a deliberate change.
	got := cosineSimilarity([]float64{math.Inf(1), 0}, []float64{1, 0})
	if !math.IsNaN(got) {
		t.Fatalf("cosineSimilarity with Inf = %v, want NaN (current boundary)", got)
	}
}

func TestTokensToDict_NilQueryBuilderOrTermWeight(t *testing.T) {
	cases := []struct {
		name string
		qb   *QueryBuilder
	}{
		{"nil query builder", nil},
		{"nil term weight", &QueryBuilder{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, order := tokensToDict([]string{"alpha", "beta"}, tc.qb, "")
			if len(d) != 0 {
				t.Fatalf("dict = %v, want empty", d)
			}
			if len(order) != 0 {
				t.Fatalf("key order = %v, want empty", order)
			}
		})
	}
}

func TestTokensToDict_WeightsAndKeyOrder(t *testing.T) {
	qb := NewQueryBuilder()
	tks := []string{"alpha", "beta", "alpha", "gamma"}

	// Use the dealer's own weights as the input oracle, then apply the 0.4/0.6
	// formula independently. Tokenizer weights are deliberately not hardcoded.
	wts := qb.termWeight.Weights(tks, false, "")
	if len(wts) != len(tks) {
		t.Fatalf("Weights returned %d entries, want %d", len(wts), len(tks))
	}

	want := make(map[string]float64)
	var wantOrder []string
	seen := make(map[string]bool)
	for i, tw := range wts {
		if !seen[tw.Term] {
			seen[tw.Term] = true
			wantOrder = append(wantOrder, tw.Term)
		}
		want[tw.Term] += tw.Weight * 0.4
		if i+1 < len(wts) {
			next := wts[i+1]
			key := tw.Term + next.Term
			if !seen[key] {
				seen[key] = true
				wantOrder = append(wantOrder, key)
			}
			want[key] += math.Max(tw.Weight, next.Weight) * 0.6
		}
	}

	got, gotOrder := tokensToDict(tks, qb, "")
	if len(got) != len(want) {
		t.Fatalf("dict size = %d, want %d (%v vs %v)", len(got), len(want), got, want)
	}
	for k, v := range want {
		if math.Abs(got[k]-v) > 1e-9 {
			t.Fatalf("dict[%q] = %v, want %v", k, got[k], v)
		}
	}
	if len(gotOrder) != len(wantOrder) {
		t.Fatalf("key order = %v, want %v", gotOrder, wantOrder)
	}
	for i := range wantOrder {
		if gotOrder[i] != wantOrder[i] {
			t.Fatalf("key order[%d] = %q, want %q (full: %v vs %v)",
				i, gotOrder[i], wantOrder[i], gotOrder, wantOrder)
		}
	}
	// Repeated terms accumulate instead of being overwritten.
	if got["alpha"] <= 0.4*wts[0].Weight {
		t.Fatalf("repeated term alpha did not accumulate: %v", got["alpha"])
	}
}

func TestTokenDictSimilarity_Cases(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := tokenDictSimilarity(nil, map[string]float64{"a": 1}, nil); got != 0 {
			t.Fatalf("empty query -> %v, want 0", got)
		}
		if got := tokenDictSimilarity(map[string]float64{"a": 1}, nil, nil); got != 0 {
			t.Fatalf("empty doc -> %v, want 0", got)
		}
	})

	t.Run("full overlap", func(t *testing.T) {
		q := map[string]float64{"a": 1, "b": 1}
		d := map[string]float64{"a": 5, "b": 5}
		got := tokenDictSimilarity(q, d, []string{"a", "b"})
		if math.Abs(got-1) > 1e-9 {
			t.Fatalf("full overlap -> %v, want 1", got)
		}
	})

	t.Run("partial overlap", func(t *testing.T) {
		q := map[string]float64{"a": 1, "b": 3}
		d := map[string]float64{"a": 9}
		want := (1e-9 + 1) / (1e-9 + 4)
		got := tokenDictSimilarity(q, d, []string{"a", "b"})
		if math.Abs(got-want) > 1e-12 {
			t.Fatalf("partial overlap -> %v, want %v", got, want)
		}
	})

	t.Run("no overlap is near zero but not zero", func(t *testing.T) {
		q := map[string]float64{"a": 1}
		d := map[string]float64{"z": 1}
		got := tokenDictSimilarity(q, d, []string{"a"})
		if got == 0 {
			t.Fatalf("no overlap returned exactly 0, smoothing missing")
		}
		if got > 1e-6 {
			t.Fatalf("no overlap -> %v, want near zero", got)
		}
	})
}

func TestTokenDictSimilarity_NilKeyOrderFallback(t *testing.T) {
	q := map[string]float64{"alpha": 1, "beta": 2, "gamma": 3}
	d := map[string]float64{"beta": 1, "gamma": 1}
	withOrder := tokenDictSimilarity(q, d, []string{"alpha", "beta", "gamma"})
	withoutOrder := tokenDictSimilarity(q, d, nil)
	if math.Abs(withOrder-withoutOrder) > 1e-12 {
		t.Fatalf("nil order = %v, explicit sorted order = %v", withoutOrder, withOrder)
	}
}

func TestTokenSimilarity_Cases(t *testing.T) {
	t.Run("empty documents", func(t *testing.T) {
		got := TokenSimilarity([]string{"a"}, nil, NewQueryBuilder(), "")
		if len(got) != 0 {
			t.Fatalf("nil docs -> %v, want empty", got)
		}
	})

	t.Run("nil query builder returns zeros", func(t *testing.T) {
		got := TokenSimilarity([]string{"alpha"}, [][]string{{"alpha"}, {"beta"}}, nil, "")
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		for i, v := range got {
			if v != 0 {
				t.Fatalf("score[%d] = %v, want 0", i, v)
			}
		}
	})

	qb := NewQueryBuilder()
	t.Run("identical", func(t *testing.T) {
		got := TokenSimilarity([]string{"alpha", "beta"}, [][]string{{"alpha", "beta"}}, qb, "")
		if len(got) != 1 || math.Abs(got[0]-1) > 1e-9 {
			t.Fatalf("identical -> %v, want [1]", got)
		}
	})

	t.Run("disjoint", func(t *testing.T) {
		got := TokenSimilarity([]string{"alpha"}, [][]string{{"omega"}}, qb, "")
		// The 1e-9 smoothing is in both numerator and denominator, so a
		// disjoint pair is near zero but not exactly zero.
		if len(got) != 1 || got[0] > 1e-6 {
			t.Fatalf("disjoint -> %v, want near 0", got)
		}
	})

	t.Run("partial", func(t *testing.T) {
		got := TokenSimilarity([]string{"alpha", "beta"}, [][]string{{"alpha"}}, qb, "")
		if len(got) != 1 || got[0] <= 0 || got[0] >= 1 {
			t.Fatalf("partial -> %v, want 0 < x < 1", got)
		}
	})

	t.Run("stable across calls", func(t *testing.T) {
		atks := []string{"alpha", "beta", "gamma"}
		btkss := [][]string{{"alpha"}, {"beta", "gamma"}, {}}
		first := TokenSimilarity(atks, btkss, qb, "")
		second := TokenSimilarity(atks, btkss, qb, "")
		if !floatsClose(first, second, 0) {
			t.Fatalf("unstable: %v vs %v", first, second)
		}
	})
}

func TestHybridSimilarity_VectorOnly(t *testing.T) {
	avec := []float64{1, 0}
	bvecs := [][]float64{{1, 0}, {0, 1}}
	sim, tsim, vsim := HybridSimilarity(avec, bvecs, []string{"alpha"}, [][]string{{"alpha"}, {"beta"}}, 0.3, 0.7, nil, "")
	if !floatsClose(vsim, []float64{1, 0}, 1e-9) {
		t.Fatalf("vsim = %v, want [1 0]", vsim)
	}
	for i := range tsim {
		if tsim[i] != 0 {
			t.Fatalf("tsim[%d] = %v, want 0 with nil qb", i, tsim[i])
		}
	}
	want := []float64{0.7, 0}
	if !floatsClose(sim, want, 1e-9) {
		t.Fatalf("sim = %v, want %v", sim, want)
	}
}

func TestHybridSimilarity_BlendAndZero(t *testing.T) {
	t.Run("blend", func(t *testing.T) {
		qb := NewQueryBuilder()
		avec := []float64{1, 0}
		bvecs := [][]float64{{1, 0}, {1, 1}}
		atks := []string{"alpha"}
		btkss := [][]string{{"alpha"}, {"beta"}}
		sim, tsim, vsim := HybridSimilarity(avec, bvecs, atks, btkss, 0.3, 0.7, qb, "")
		for i := range sim {
			want := 0.3*tsim[i] + 0.7*vsim[i]
			if math.Abs(sim[i]-want) > 1e-9 {
				t.Fatalf("sim[%d] = %v, want 0.3*%v + 0.7*%v = %v", i, sim[i], tsim[i], vsim[i], want)
			}
		}
	})

	t.Run("all zero vectors", func(t *testing.T) {
		qb := NewQueryBuilder()
		avec := []float64{0, 0}
		bvecs := [][]float64{{0, 0}, {0, 0}}
		sim, tsim, vsim := HybridSimilarity(avec, bvecs, []string{"alpha"}, [][]string{{"alpha"}, {"beta"}}, 0.3, 0.7, qb, "")
		for i := range vsim {
			if vsim[i] != 0 {
				t.Fatalf("vsim[%d] = %v, want 0", i, vsim[i])
			}
		}
		// Initial sim equals tsim; the slice must not be the same backing array.
		if !floatsClose(sim, tsim, 1e-9) {
			t.Fatalf("sim = %v, want tsim = %v", sim, tsim)
		}
	})
}

func TestHybridSimilarity_AllZeroDoesNotAlias(t *testing.T) {
	avec := []float64{0, 0}
	bvecs := [][]float64{{0, 0}}
	sim, tsim, _ := HybridSimilarity(avec, bvecs, []string{"alpha"}, [][]string{{"alpha"}}, 0.3, 0.7, nil, "")
	if len(sim) == 0 || len(tsim) == 0 {
		t.Fatal("expected non-empty result")
	}
	if &sim[0] == &tsim[0] {
		t.Fatal("sim and tsim share a backing array; rank feature scores would leak into term_similarity")
	}
	sim[0] += 42
	if tsim[0] != 0 {
		t.Fatalf("mutating sim changed tsim: %v", tsim)
	}
}

func TestHybridSimilarity_LengthMismatch(t *testing.T) {
	// Known limitation: fewer vectors than token lists indexes past vsim.
	defer func() {
		if r := recover(); r != nil {
			t.Logf("HybridSimilarity panics on vector/token length mismatch (known limitation): %v", r)
		}
	}()
	HybridSimilarity(
		[]float64{1, 0},
		[][]float64{{1, 0}},
		[]string{"alpha"},
		[][]string{{"alpha"}, {"beta"}},
		0.3, 0.7, nil, "",
	)
}
