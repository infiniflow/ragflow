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

// TestNormalizeRerankScores_OutOfRange_Rescaled covers the central bug fix:
// uncalibrated reranker output (e.g. NVIDIA logits) is min-max rescaled
// onto [0, 1] so a negative logit weighted by vtWeight=0.7 cannot sink a
// relevant chunk below pure keyword matches.
func TestNormalizeRerankScores_OutOfRange_Rescaled(t *testing.T) {
	cases := []struct {
		name string
		in   []float64
		want []float64
	}{
		{"unbounded mixed-sign logits", []float64{10.0, -3.0, 0.0}, []float64{1.0, 0.0, 3.0 / 13.0}},
		{"large positive logits", []float64{100.0, 50.0, 75.0}, []float64{1.0, 0.0, 0.5}},
		{"negative-only logits", []float64{-1.0, -5.0, -3.0}, []float64{1.0, 0.0, 0.5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeRerankScores(tc.in)
			if !floatsClose(got, tc.want, 1e-9) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
			if minOf(got) < 0.0 || maxOf(got) > 1.0 {
				t.Errorf("scores escaped [0, 1]: %v", got)
			}
		})
	}
}

// TestNormalizeRerankScores_InRange_Preserved pins the calibrated-provider
// guarantee: Cohere/Jina/Voyage-style scores in [0, 1] are returned verbatim,
// so similarity_threshold semantics and the reported vector_similarity keep
// their absolute magnitudes.
func TestNormalizeRerankScores_InRange_Preserved(t *testing.T) {
	cases := []struct {
		name string
		in   []float64
	}{
		{"spread relevance", []float64{0.9, 0.1, 0.5}},
		{"all-equal but valid", []float64{0.8, 0.8, 0.8}},
		{"single candidate", []float64{1.0}},
		{"already spanning the full range", []float64{0.0, 1.0, 0.42}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeRerankScores(tc.in)
			if !floatsClose(got, tc.in, 1e-9) {
				t.Errorf("got %v, want %v (must be preserved)", got, tc.in)
			}
		})
	}
}

// TestNormalizeRerankScores_PreservesOrdering ensures rescaling does not
// scramble the relative ranking; this is the property downstream code relies
// on when sorting by rerank score.
func TestNormalizeRerankScores_PreservesOrdering(t *testing.T) {
	in := []float64{-5.0, 12.0, 3.0, -1.0}
	got := NormalizeRerankScores(in)
	wantOrder := argsortDesc(in)
	gotOrder := argsortDesc(got)
	if !intsEqual(wantOrder, gotOrder) {
		t.Errorf("ordering changed: want %v, got %v", wantOrder, gotOrder)
	}
}

// TestNormalizeRerankScores_SpreadlessOutOfRange_Clamped covers the
// degenerate but realistic case of a single rerank candidate or a flat
// batch of out-of-range values: clamped per element, never zeroed, never
// NaN. A lone high logit would otherwise be silently dropped and
// contaminate the blend with NaN if divided by ~0.
func TestNormalizeRerankScores_SpreadlessOutOfRange_Clamped(t *testing.T) {
	cases := []struct {
		name string
		in   []float64
		want []float64
	}{
		{"single out-of-range high", []float64{5.0}, []float64{1.0}},
		{"single out-of-range negative", []float64{-3.0}, []float64{0.0}},
		{"flat out-of-range high batch", []float64{5.0, 5.0, 5.0}, []float64{1.0, 1.0, 1.0}},
		{"flat out-of-range low batch", []float64{-2.0, -2.0, -2.0}, []float64{0.0, 0.0, 0.0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeRerankScores(tc.in)
			if !floatsClose(got, tc.want, 1e-9) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
			for _, s := range got {
				if math.IsNaN(s) {
					t.Fatalf("NaN leaked into normalized scores: %v", got)
				}
			}
		})
	}
}

// TestNormalizeRerankScores_Empty covers the empty-input contract: returned
// verbatim, no allocation, no panic.
func TestNormalizeRerankScores_Empty(t *testing.T) {
	got := NormalizeRerankScores(nil)
	if len(got) != 0 {
		t.Errorf("nil in -> expected empty out, got %v", got)
	}
	got = NormalizeRerankScores([]float64{})
	if len(got) != 0 {
		t.Errorf("[] in -> expected empty out, got %v", got)
	}
}

// TestNormalizeRerankScores_InPlace pins the in-place guarantee: the input
// slice's backing array is what gets returned, so the RerankByModel call
// site stays allocation-free.
func TestNormalizeRerankScores_InPlace(t *testing.T) {
	in := []float64{10.0, -3.0, 0.0}
	got := NormalizeRerankScores(in)
	if &got[0] != &in[0] {
		t.Errorf("NormalizeRerankScores must mutate in place; got a new backing array")
	}
}

func floatsClose(a, b []float64, tol float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(a[i]-b[i]) > tol {
			return false
		}
	}
	return true
}

func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func minOf(s []float64) float64 {
	if len(s) == 0 {
		return 0
	}
	m := s[0]
	for _, v := range s[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func maxOf(s []float64) float64 {
	if len(s) == 0 {
		return 0
	}
	m := s[0]
	for _, v := range s[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

// argsortDesc returns the indices of s sorted by value in descending order,
// matching how a downstream consumer would compare rerank scores.
func argsortDesc(s []float64) []int {
	idx := make([]int, len(s))
	for i := range idx {
		idx[i] = i
	}
	// Insertion sort keeps it dependency-free; len is small (batch size).
	for i := 1; i < len(idx); i++ {
		for j := i; j > 0 && s[idx[j]] > s[idx[j-1]]; j-- {
			idx[j], idx[j-1] = idx[j-1], idx[j]
		}
	}
	return idx
}
