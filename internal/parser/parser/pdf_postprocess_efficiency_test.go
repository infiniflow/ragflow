package parser

import (
	"fmt"
	"testing"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

// buildWideSectionResult builds a layout whose sections carry many positions
// with no page numbers. firstSectionPage therefore scans every position on
// each call, so the O(n^2) comparison loop in reorderPDFMultiColumn pays an
// O(positions) cost per comparison. The fixture makes that cost measurable.
func buildWideSectionResult(nSections, nPositions int) *deepdoctype.ParseResult {
	sections := make([]deepdoctype.Section, nSections)
	for i := range sections {
		positions := make([]deepdoctype.Position, nPositions)
		left := float64((i % 4) * 40) // adjacent columns 40 apart (< threshold 50)
		top := float64((i * 37) % 1000)
		for j := range positions {
			positions[j] = deepdoctype.Position{
				Left:   left,
				Right:  left + 100, // width 100 -> median 100 -> threshold 50
				Top:    top,
				Bottom: top + 20,
				// PageNumbers intentionally nil so firstSectionPage scans all.
			}
		}
		sections[i] = deepdoctype.Section{
			Text:       fmt.Sprintf("s%d", i),
			LayoutType: deepdoctype.LayoutTypeText,
			Positions:  positions,
		}
	}
	return &deepdoctype.ParseResult{Sections: sections}
}

// BenchmarkReorderPDFMultiColumn guards the multi-column hot path against
// reintroducing per-comparison position scans. A single layout is reused
// across iterations so a long run never retains b.N copies of the fixture.
func BenchmarkReorderPDFMultiColumn(b *testing.B) {
	base := buildWideSectionResult(800, 200).Sections
	layout := &deepdoctype.ParseResult{Sections: cloneSections(base)}
	for i := 0; i < b.N; i++ {
		// Restore the original order outside the timed region so every
		// iteration measures the same input. Positions are shared but only
		// read by reorderPDFMultiColumn.
		b.StopTimer()
		copy(layout.Sections, base)
		b.StartTimer()
		reorderPDFMultiColumn(layout, 600, 1)
	}
}

func cloneSections(sections []deepdoctype.Section) []deepdoctype.Section {
	out := make([]deepdoctype.Section, len(sections))
	copy(out, sections)
	return out
}

// TestReorderPDFMultiColumn_PreservesOrder pins the CURRENT byte-for-byte
// output. The chained-column case is the adversarial one: a global sort by
// (page,left,top) would emit A,B,C while the existing conditional bubble emits
// a different order, so any rewrite must keep the bubble's exact semantics.
func TestReorderPDFMultiColumn_PreservesOrder(t *testing.T) {
	tests := []struct {
		name      string
		sections  []deepdoctype.Section
		pageWidth float64
		want      []string
	}{
		{
			name: "chained columns within threshold",
			sections: []deepdoctype.Section{
				makePDFSection("A", "text", 0, 0, 100, 10, 20),
				makePDFSection("B", "text", 0, 40, 140, 5, 15),
				makePDFSection("C", "text", 0, 80, 180, 0, 10),
			},
			pageWidth: 600,
			want:      []string{"A", "C", "B"},
		},
		{
			name: "two pages keep page order",
			sections: []deepdoctype.Section{
				makePDFSection("p0b", "text", 0, 0, 100, 50, 60),
				makePDFSection("p0a", "text", 0, 0, 100, 10, 20),
				makePDFSection("p1a", "text", 1, 0, 100, 10, 20),
			},
			pageWidth: 600,
			want:      []string{"p0a", "p0b", "p1a"},
		},
		{
			name: "single section",
			sections: []deepdoctype.Section{
				makePDFSection("only", "text", 0, 0, 100, 10, 20),
			},
			pageWidth: 600,
			want:      []string{"only"},
		},
		{
			name: "empty positions section participates with zero geometry",
			sections: []deepdoctype.Section{
				{Text: "nopos", LayoutType: "text"},
				makePDFSection("real", "text", 0, 10, 110, 30, 40),
			},
			pageWidth: 600,
			want:      []string{"nopos", "real"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &deepdoctype.ParseResult{Sections: cloneSections(tt.sections)}
			reorderPDFMultiColumn(result, tt.pageWidth, 1)
			got := orderTexts(result.Sections)
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Fatalf("order = %v, want %v", got, tt.want)
			}
		})
	}
}
