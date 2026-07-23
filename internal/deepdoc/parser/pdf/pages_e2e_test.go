package pdf

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"ragflow/internal/utility"
)

// TestPagesEndToEnd_NormalizeThenParse verifies the full pages path: a
// frontend-submitted (JSON-decoded, possibly dirty) pages value is normalized
// by utility.NormalizePDFPages, fed into ParserConfig.Pages, and the deepdoc
// parser only processes the resulting page ranges.
//
// This ties together step 3 (API-layer normalization) and step 1 (deepdoc
// page filtering) end-to-end at the parser level.
func TestPagesEndToEnd_NormalizeThenParse(t *testing.T) {
	// jsonPages builds a JSON-decoded-style []any of []any of float64, mimicking
	// what arrives over the wire from the frontend.
	jsonPages := func(ranges ...[2]float64) any {
		out := make([]any, 0, len(ranges))
		for _, r := range ranges {
			out = append(out, []any{r[0], r[1]})
		}
		return out
	}

	t.Run("overlapping and unsorted ranges normalized then filtered", func(t *testing.T) {
		// Frontend submitted [1,3],[2,5],[8,10] (overlap 1-3 & 2-5, unsorted).
		raw := jsonPages([2]float64{1, 3}, [2]float64{2, 5}, [2]float64{8, 10})
		normalized := utility.NormalizePDFPages(raw) // -> [[1,5],[8,10]]

		wantNorm := [][]int{{1, 5}, {8, 10}}
		if !reflect.DeepEqual(normalized, wantNorm) {
			t.Fatalf("NormalizePDFPages = %v, want %v", normalized, wantNorm)
		}

		cfg := pdf.DefaultParserConfig()
		cfg.Pages = normalized
		p := NewParser(cfg)

		eng := makePageTaggedEngine(10)
		result, err := p.ParseRaw(context.Background(), eng, &MockDocAnalyzer{Healthy: true})
		if err != nil {
			t.Fatalf("ParseRaw: %v", err)
		}

		// [1,5] -> 0-based 0..4 ; [8,10] -> 7..9
		wantPages := map[int]struct{}{0: {}, 1: {}, 2: {}, 3: {}, 4: {}, 7: {}, 8: {}, 9: {}}
		if got := pageHeightKeys(result.PageHeight); !reflect.DeepEqual(got, wantPages) {
			t.Errorf("PageHeight keys = %v, want %v", got, wantPages)
		}

		// Sections must not carry text from skipped pages (5, 6).
		combined := combineSectionText(result.Sections)
		for _, pg := range []int{5, 6} {
			if strings.Contains(combined, fmt.Sprintf("p%d", pg)) {
				t.Errorf("expected p%d to be skipped; combined=%q", pg, combined)
			}
		}
	})

	t.Run("invalid ranges dropped during normalization", func(t *testing.T) {
		// [1,3],[3,1](from>to),[8,10] -> normalize drops [3,1] -> [[1,3],[8,10]]
		raw := jsonPages([2]float64{1, 3}, [2]float64{3, 1}, [2]float64{8, 10})
		normalized := utility.NormalizePDFPages(raw)

		wantNorm := [][]int{{1, 3}, {8, 10}}
		if !reflect.DeepEqual(normalized, wantNorm) {
			t.Fatalf("NormalizePDFPages = %v, want %v", normalized, wantNorm)
		}

		cfg := pdf.DefaultParserConfig()
		cfg.Pages = normalized
		p := NewParser(cfg)

		eng := makePageTaggedEngine(10)
		result, err := p.ParseRaw(context.Background(), eng, &MockDocAnalyzer{Healthy: true})
		if err != nil {
			t.Fatalf("ParseRaw: %v", err)
		}

		wantPages := map[int]struct{}{0: {}, 1: {}, 2: {}, 7: {}, 8: {}, 9: {}}
		if got := pageHeightKeys(result.PageHeight); !reflect.DeepEqual(got, wantPages) {
			t.Errorf("PageHeight keys = %v, want %v", got, wantPages)
		}
	})

	t.Run("all invalid -> nil -> parse all pages (regression guard)", func(t *testing.T) {
		// [3,1],[0,2] both invalid -> nil -> parse all pages.
		raw := jsonPages([2]float64{3, 1}, [2]float64{0, 2})
		normalized := utility.NormalizePDFPages(raw)
		if normalized != nil {
			t.Fatalf("NormalizePDFPages = %v, want nil", normalized)
		}

		cfg := pdf.DefaultParserConfig()
		cfg.Pages = normalized // nil
		p := NewParser(cfg)

		eng := makePageTaggedEngine(10)
		result, err := p.ParseRaw(context.Background(), eng, &MockDocAnalyzer{Healthy: true})
		if err != nil {
			t.Fatalf("ParseRaw: %v", err)
		}

		wantPages := map[int]struct{}{}
		for i := 0; i < 10; i++ {
			wantPages[i] = struct{}{}
		}
		if got := pageHeightKeys(result.PageHeight); !reflect.DeepEqual(got, wantPages) {
			t.Errorf("PageHeight keys = %v, want all 10 pages", got)
		}
	})

	t.Run("range clamped to document page count", func(t *testing.T) {
		// [1,1000000] on a 5-page doc -> clamped to all 5 pages.
		raw := jsonPages([2]float64{1, 1000000})
		normalized := utility.NormalizePDFPages(raw)

		cfg := pdf.DefaultParserConfig()
		cfg.Pages = normalized
		p := NewParser(cfg)

		eng := makePageTaggedEngine(5)
		result, err := p.ParseRaw(context.Background(), eng, &MockDocAnalyzer{Healthy: true})
		if err != nil {
			t.Fatalf("ParseRaw: %v", err)
		}

		wantPages := map[int]struct{}{0: {}, 1: {}, 2: {}, 3: {}, 4: {}}
		if got := pageHeightKeys(result.PageHeight); !reflect.DeepEqual(got, wantPages) {
			t.Errorf("PageHeight keys = %v, want all 5 pages", got)
		}
	})
}
