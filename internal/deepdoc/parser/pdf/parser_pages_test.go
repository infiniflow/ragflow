package pdf

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestResolvePagesToProcess verifies the 1-indexed inclusive ranges are
// converted to a sorted, de-duplicated, clamped 0-indexed page list.
func TestResolvePagesToProcess(t *testing.T) {
	// allPages returns [0..n-1] for assertion convenience.
	allPages := func(n int) []int {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	}

	cases := []struct {
		name      string
		ranges    [][]int
		pageCount int
		want      []int
	}{
		{"nil ranges -> all pages", nil, 10, allPages(10)},
		{"empty ranges -> all pages", [][]int{}, 10, allPages(10)},
		{"single range", [][]int{{1, 3}}, 10, []int{0, 1, 2}},
		{"two disjoint ranges", [][]int{{1, 3}, {8, 10}}, 10, []int{0, 1, 2, 7, 8, 9}},
		{"unbounded upper clamped to page count", [][]int{{1, 1000000}}, 5, allPages(5)},
		{"range entirely beyond doc -> empty", [][]int{{8, 10}}, 5, []int{}},
		{"overlapping ranges de-duplicated", [][]int{{1, 3}, {2, 4}}, 10, []int{0, 1, 2, 3}},
		{"from > to skipped", [][]int{{3, 1}}, 10, []int{}},
		{"wrong arity skipped", [][]int{{1}}, 10, []int{}},
		{"unsorted input sorted", [][]int{{8, 10}, {1, 3}}, 10, []int{0, 1, 2, 7, 8, 9}},
		{"single page range", [][]int{{5, 5}}, 10, []int{4}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolvePagesToProcess(c.ranges, c.pageCount)
			if !slices.Equal(got, c.want) {
				t.Errorf("resolvePagesToProcess(%v, %d) = %v, want %v", c.ranges, c.pageCount, got, c.want)
			}
		})
	}
}

// makePageTaggedEngine builds a MockEngine with `numPages` pages, each carrying
// a single char whose text is "pN" (N = 0-based page number), so parsed pages
// can be identified in the resulting sections.
func makePageTaggedEngine(numPages int) *MockEngine {
	chars := make(map[int][]pdf.TextChar, numPages)
	for i := 0; i < numPages; i++ {
		chars[i] = []pdf.TextChar{
			{Text: fmt.Sprintf("p%d", i), X0: 10, X1: 50, Top: 10, Bottom: 30, PageNumber: i},
		}
	}
	return &MockEngine{NumPages: numPages, Chars: chars, RenderW: 100, RenderH: 100}
}

// pageHeightKeys returns the set of page numbers that were actually processed,
// extracted from result.PageHeight (keyed by 0-based page number).
func pageHeightKeys(m map[int]float64) map[int]struct{} {
	out := make(map[int]struct{}, len(m))
	for k := range m {
		out[k] = struct{}{}
	}
	return out
}

// TestParseRaw_PageRanges_Applied verifies that Config.Pages restricts parsing
// to the specified pages (1-indexed inclusive) and that pages outside the
// ranges are not processed.
func TestParseRaw_PageRanges_Applied(t *testing.T) {
	eng := makePageTaggedEngine(10)
	p := NewParser(pdf.ParserConfig{Pages: [][]int{{1, 3}, {8, 10}}})

	result, err := p.ParseRaw(context.Background(), eng, &MockDocAnalyzer{Healthy: true})
	if err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}

	wantPages := map[int]struct{}{0: {}, 1: {}, 2: {}, 7: {}, 8: {}, 9: {}}
	if got := pageHeightKeys(result.PageHeight); !reflect.DeepEqual(got, wantPages) {
		t.Errorf("PageHeight keys = %v, want %v", got, wantPages)
	}

	// Sections must reference only parsed pages and must not contain text from
	// skipped pages (p3..p6).
	combined := combineSectionText(result.Sections)
	for _, pg := range []int{0, 1, 2, 7, 8, 9} {
		if !strings.Contains(combined, fmt.Sprintf("p%d", pg)) {
			t.Errorf("expected section text to contain p%d; combined=%q", pg, combined)
		}
	}
	for _, pg := range []int{3, 4, 5, 6} {
		if strings.Contains(combined, fmt.Sprintf("p%d", pg)) {
			t.Errorf("expected section text NOT to contain p%d (page skipped); combined=%q", pg, combined)
		}
	}
}

// TestParseRaw_NoPages_AllPagesParsed is the regression guard: with Pages nil
// (the default), every page is parsed exactly as before this feature.
func TestParseRaw_NoPages_AllPagesParsed(t *testing.T) {
	eng := makePageTaggedEngine(10)
	p := NewParser(pdf.DefaultParserConfig())

	result, err := p.ParseRaw(context.Background(), eng, &MockDocAnalyzer{Healthy: true})
	if err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}

	wantPages := map[int]struct{}{}
	for i := 0; i < 10; i++ {
		wantPages[i] = struct{}{}
	}
	if got := pageHeightKeys(result.PageHeight); !reflect.DeepEqual(got, wantPages) {
		t.Errorf("PageHeight keys = %v, want all 10 pages %v", got, wantPages)
	}
}

// TestParseRaw_PageRanges_BeyondDoc verifies that a range fully beyond the
// document page count yields an empty parse (no pages processed).
func TestParseRaw_PageRanges_BeyondDoc(t *testing.T) {
	eng := makePageTaggedEngine(5)
	p := NewParser(pdf.ParserConfig{Pages: [][]int{{8, 10}}})

	result, err := p.ParseRaw(context.Background(), eng, &MockDocAnalyzer{Healthy: true})
	if err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}
	if len(result.PageHeight) != 0 {
		t.Errorf("expected 0 pages parsed for fully out-of-range pages, got %d (%v)",
			len(result.PageHeight), pageHeightKeys(result.PageHeight))
	}
}

// combineSectionText concatenates all section text for content assertions.
func combineSectionText(sections []pdf.Section) string {
	var b strings.Builder
	for _, s := range sections {
		b.WriteString(s.Text)
	}
	return b.String()
}
