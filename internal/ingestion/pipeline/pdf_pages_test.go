package pipeline

import (
	"reflect"
	"testing"
)

// TestNormalizeParserConfigPages verifies the walker normalizes the "pages"
// field under every component's filetype setup: writes back the normalized
// value when valid, leaves the original untouched when all-invalid.
func TestNormalizeParserConfigPages(t *testing.T) {
	// f64 builds a JSON-decoded-style []any of []any of float64.
	f64 := func(ranges ...[2]float64) any {
		out := make([]any, 0, len(ranges))
		for _, r := range ranges {
			out = append(out, []any{r[0], r[1]})
		}
		return out
	}

	t.Run("overlap merged and written back", func(t *testing.T) {
		cfg := map[string]any{
			"Parser:X": map[string]any{
				"pdf": map[string]any{"pages": f64([2]float64{1, 200}, [2]float64{111, 333})},
			},
		}
		NormalizeParserConfigPages(cfg)
		pdf := cfg["Parser:X"].(map[string]any)["pdf"].(map[string]any)
		if want := [][]int{{1, 333}}; !reflect.DeepEqual(pdf["pages"], want) {
			t.Errorf("pages = %v, want %v", pdf["pages"], want)
		}
	})

	t.Run("unsorted sorted and written back", func(t *testing.T) {
		cfg := map[string]any{
			"Parser:X": map[string]any{
				"pdf": map[string]any{"pages": f64([2]float64{400, 500}, [2]float64{1, 100})},
			},
		}
		NormalizeParserConfigPages(cfg)
		pdf := cfg["Parser:X"].(map[string]any)["pdf"].(map[string]any)
		if want := [][]int{{1, 100}, {400, 500}}; !reflect.DeepEqual(pdf["pages"], want) {
			t.Errorf("pages = %v, want %v", pdf["pages"], want)
		}
	})

	t.Run("all invalid -> original preserved", func(t *testing.T) {
		original := f64([2]float64{3, 1})
		cfg := map[string]any{
			"Parser:X": map[string]any{
				"pdf": map[string]any{"pages": original},
			},
		}
		NormalizeParserConfigPages(cfg)
		pdf := cfg["Parser:X"].(map[string]any)["pdf"].(map[string]any)
		if !reflect.DeepEqual(pdf["pages"], original) {
			t.Errorf("pages = %v, want original %v preserved", pdf["pages"], original)
		}
	})

	t.Run("non-list pages -> original preserved", func(t *testing.T) {
		cfg := map[string]any{
			"Parser:X": map[string]any{
				"pdf": map[string]any{"pages": "1-100"},
			},
		}
		NormalizeParserConfigPages(cfg)
		pdf := cfg["Parser:X"].(map[string]any)["pdf"].(map[string]any)
		if pdf["pages"] != "1-100" {
			t.Errorf("pages = %v, want original \"1-100\" preserved", pdf["pages"])
		}
	})

	t.Run("no pages key -> unchanged", func(t *testing.T) {
		cfg := map[string]any{
			"Parser:X": map[string]any{
				"pdf": map[string]any{"output_format": "json"},
			},
		}
		NormalizeParserConfigPages(cfg)
		pdf := cfg["Parser:X"].(map[string]any)["pdf"].(map[string]any)
		if _, ok := pdf["pages"]; ok {
			t.Error("pages key should not exist")
		}
		if pdf["output_format"] != "json" {
			t.Error("other fields should not be touched")
		}
	})

	t.Run("non-pdf filetype also normalized (generic walk)", func(t *testing.T) {
		cfg := map[string]any{
			"Parser:X": map[string]any{
				"docx": map[string]any{"pages": f64([2]float64{1, 100})},
			},
		}
		NormalizeParserConfigPages(cfg)
		docx := cfg["Parser:X"].(map[string]any)["docx"].(map[string]any)
		if want := [][]int{{1, 100}}; !reflect.DeepEqual(docx["pages"], want) {
			t.Errorf("docx.pages = %v, want %v", docx["pages"], want)
		}
	})

	t.Run("non-map cpnID value -> unchanged", func(t *testing.T) {
		cfg := map[string]any{"Parser:X": "not a map"}
		NormalizeParserConfigPages(cfg)
		if cfg["Parser:X"] != "not a map" {
			t.Errorf("non-map cpnID value should not be touched, got %v", cfg["Parser:X"])
		}
	})

	t.Run("multiple cpnIDs each normalized", func(t *testing.T) {
		cfg := map[string]any{
			"Parser:A": map[string]any{
				"pdf": map[string]any{"pages": f64([2]float64{1, 200}, [2]float64{111, 333})},
			},
			"Parser:B": map[string]any{
				"pdf": map[string]any{"pages": f64([2]float64{400, 500}, [2]float64{1, 100})},
			},
		}
		NormalizeParserConfigPages(cfg)
		pdfA := cfg["Parser:A"].(map[string]any)["pdf"].(map[string]any)
		pdfB := cfg["Parser:B"].(map[string]any)["pdf"].(map[string]any)
		if want := [][]int{{1, 333}}; !reflect.DeepEqual(pdfA["pages"], want) {
			t.Errorf("Parser:A pages = %v, want %v", pdfA["pages"], want)
		}
		if want := [][]int{{1, 100}, {400, 500}}; !reflect.DeepEqual(pdfB["pages"], want) {
			t.Errorf("Parser:B pages = %v, want %v", pdfB["pages"], want)
		}
	})

	t.Run("nil cfg -> no panic", func(t *testing.T) {
		NormalizeParserConfigPages(nil)
	})
}
