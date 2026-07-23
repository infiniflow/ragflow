package utility

import (
	"reflect"
	"testing"
)

// TestNormalizePDFPages verifies normalization of the 1-indexed inclusive
// "pages" ranges: type coercion, illegal-range dropping, sorting, and
// overlap/adjacency merging.
func TestNormalizePDFPages(t *testing.T) {
	// f64 builds a JSON-decoded-style []any of []any of float64.
	f64 := func(ranges ...[2]float64) any {
		out := make([]any, 0, len(ranges))
		for _, r := range ranges {
			out = append(out, []any{r[0], r[1]})
		}
		return out
	}

	cases := []struct {
		name string
		raw  any
		want [][]int
	}{
		{"nil -> nil", nil, nil},
		{"not a slice -> nil", "1-100", nil},
		{"empty slice -> nil", []any{}, nil},
		{"single range float64", f64([2]float64{1, 100}), [][]int{{1, 100}}},
		{"two disjoint ranges", f64([2]float64{1, 100}, [2]float64{400, 500}), [][]int{{1, 100}, {400, 500}}},
		{"overlap merged", f64([2]float64{1, 200}, [2]float64{111, 333}), [][]int{{1, 333}}},
		{"adjacent merged", f64([2]float64{1, 100}, [2]float64{101, 200}), [][]int{{1, 200}}},
		{"unsorted input sorted", f64([2]float64{400, 500}, [2]float64{1, 100}), [][]int{{1, 100}, {400, 500}}},
		{"from>to dropped", f64([2]float64{5, 3}), nil},
		{"from<1 dropped", f64([2]float64{0, 5}), nil},
		{"from<1 dropped (negative)", f64([2]float64{-3, 5}), nil},
		{"mixed valid and invalid", f64([2]float64{1, 10}, [2]float64{9, 2}, [2]float64{20, 30}), [][]int{{1, 10}, {20, 30}}},
		{"wrong arity dropped", []any{[]any{float64(1)}}, nil},
		{"non-numeric dropped", []any{[]any{"a", "b"}}, nil},
		{"int accepted", []any{[]any{int(1), int(100)}}, [][]int{{1, 100}}},
		{"int64 accepted", []any{[]any{int64(1), int64(100)}}, [][]int{{1, 100}}},
		{"single page range", f64([2]float64{5, 5}), [][]int{{5, 5}}},
		{"all invalid -> nil", f64([2]float64{5, 3}, [2]float64{0, 2}), nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizePDFPages(c.raw)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("NormalizePDFPages(%v) = %v, want %v", c.raw, got, c.want)
			}
		})
	}

	// Idempotency: feeding the output back in yields the same result.
	t.Run("idempotent", func(t *testing.T) {
		once := NormalizePDFPages(f64([2]float64{1, 200}, [2]float64{111, 333}))
		// Convert [][]int to []any of []any for the second pass.
		raw2 := make([]any, 0, len(once))
		for _, r := range once {
			raw2 = append(raw2, []any{r[0], r[1]})
		}
		twice := NormalizePDFPages(raw2)
		if !reflect.DeepEqual(once, twice) {
			t.Errorf("not idempotent: once=%v twice=%v", once, twice)
		}
	})
}
