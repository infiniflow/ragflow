package utility

import (
	"reflect"
	"testing"
)

// TestNormalizePDFPages verifies normalization of the 1-indexed inclusive
// "pages" ranges under fail-fast semantics: valid ranges are normalized
// (sorted/merged/deduplicated), any invalid range rejects the whole input,
// and nil/empty means "no value" (parse all pages).
func TestNormalizePDFPages(t *testing.T) {
	// f64 builds a JSON-decoded-style []any of []any of float64.
	f64 := func(ranges ...[2]float64) any {
		out := make([]any, 0, len(ranges))
		for _, r := range ranges {
			out = append(out, []any{r[0], r[1]})
		}
		return out
	}

	// valid cases: expect (normalized, nil)
	validCases := []struct {
		name string
		raw  any
		want [][]int
	}{
		{"single range float64", f64([2]float64{1, 100}), [][]int{{1, 100}}},
		{"two disjoint ranges", f64([2]float64{1, 100}, [2]float64{400, 500}), [][]int{{1, 100}, {400, 500}}},
		{"overlap merged", f64([2]float64{1, 200}, [2]float64{111, 333}), [][]int{{1, 333}}},
		{"adjacent merged", f64([2]float64{1, 100}, [2]float64{101, 200}), [][]int{{1, 200}}},
		{"unsorted input sorted", f64([2]float64{400, 500}, [2]float64{1, 100}), [][]int{{1, 100}, {400, 500}}},
		{"int accepted", []any{[]any{int(1), int(100)}}, [][]int{{1, 100}}},
		{"int64 accepted", []any{[]any{int64(1), int64(100)}}, [][]int{{1, 100}}},
		{"single page range", f64([2]float64{5, 5}), [][]int{{5, 5}}},
	}
	for _, c := range validCases {
		t.Run("valid/"+c.name, func(t *testing.T) {
			got, err := NormalizePDFPages(c.raw)
			if err != nil {
				t.Fatalf("NormalizePDFPages(%v) unexpected error: %v", c.raw, err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("NormalizePDFPages(%v) = %v, want %v", c.raw, got, c.want)
			}
		})
	}

	// no-value cases: expect (nil, nil)
	noValueCases := []struct {
		name string
		raw  any
	}{
		{"nil", nil},
		{"empty slice", []any{}},
	}
	for _, c := range noValueCases {
		t.Run("no-value/"+c.name, func(t *testing.T) {
			got, err := NormalizePDFPages(c.raw)
			if err != nil {
				t.Fatalf("NormalizePDFPages(%v) unexpected error: %v", c.raw, err)
			}
			if got != nil {
				t.Errorf("NormalizePDFPages(%v) = %v, want nil", c.raw, got)
			}
		})
	}

	// invalid cases: expect (nil, error) — fail-fast, no partial dropping
	invalidCases := []struct {
		name string
		raw  any
	}{
		{"not a slice", "1-100"},
		{"from>to", f64([2]float64{5, 3})},
		{"from<1", f64([2]float64{0, 5})},
		{"from<1 negative", f64([2]float64{-3, 5})},
		{"mixed valid and invalid", f64([2]float64{1, 10}, [2]float64{9, 2}, [2]float64{20, 30})},
		{"wrong arity", []any{[]any{float64(1)}}},
		{"non-numeric", []any{[]any{"a", "b"}}},
		{"non-integral float", f64([2]float64{1.5, 3})},
		{"all invalid", f64([2]float64{5, 3}, [2]float64{0, 2})},
	}
	for _, c := range invalidCases {
		t.Run("invalid/"+c.name, func(t *testing.T) {
			got, err := NormalizePDFPages(c.raw)
			if err == nil {
				t.Fatalf("NormalizePDFPages(%v) expected error, got nil (result=%v)", c.raw, got)
			}
			if got != nil {
				t.Errorf("NormalizePDFPages(%v) = %v, want nil on error", c.raw, got)
			}
		})
	}

	// Idempotency: feeding the output back in yields the same result.
	t.Run("idempotent", func(t *testing.T) {
		once, err := NormalizePDFPages(f64([2]float64{1, 200}, [2]float64{111, 333}))
		if err != nil {
			t.Fatalf("first pass: %v", err)
		}
		// Convert [][]int to []any of []any for the second pass.
		raw2 := make([]any, 0, len(once))
		for _, r := range once {
			raw2 = append(raw2, []any{r[0], r[1]})
		}
		twice, err := NormalizePDFPages(raw2)
		if err != nil {
			t.Fatalf("second pass: %v", err)
		}
		if !reflect.DeepEqual(once, twice) {
			t.Errorf("not idempotent: once=%v twice=%v", once, twice)
		}
	})
}
