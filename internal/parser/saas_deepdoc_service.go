package parser

import (
	"context"
	"image"
	"regexp"
	"sort"
)

// SaaS model label taxonomies.
// DLA: 10 classes with duplicates (matching SaaS Docker TSR endpoint).
var saasDLALabels = []string{
	"title", "text", "reference", "figure", "figure caption",
	"table", "table caption", "table caption", "equation", "figure caption",
}

// TSR: 2-class separator lines (v=vertical, h=horizontal).
var saasTSRLabels = []string{"v", "h"}

// DeepDoc label regexes — compiled once at package init.
// These match the TSR label taxonomy returned by the Python DeepDoc
// table structure recognition service.
var (
	reHeader = regexp.MustCompile(`.*header$`)
	reRowHdr = regexp.MustCompile(`table$|.* (row|header)`)
	// "table$" catches the default TSR label "table" (class 0), matching
	// Python's behavior which uses all cells regardless of label.
	reSpan   = regexp.MustCompile(`.*spanning`)
	reColumn = regexp.MustCompile(`table column$`)
)

// gatherTSR filters cells by label regex pattern.
func gatherTSR(cells []TSRCell, re *regexp.Regexp) []TSRCell {
	var result []TSRCell
	for _, c := range cells {
		if re.MatchString(c.Label) {
			result = append(result, c)
		}
	}
	return result
}

// SaasDeepDocService implements TableBuilder and DocAnalyzer using the
// Python DeepDoc TSR service.
type SaasDeepDocService struct {
	doc DocAnalyzer
}

// NewSaasDeepDocService creates a service backed by the SaaS DeepDoc service.
// If doc is a *DeepDocClient, its DLALabels/TSRLabels are set to the SaaS
// taxonomy.
func NewSaasDeepDocService(doc DocAnalyzer) *SaasDeepDocService {
	if c, ok := doc.(*DeepDocClient); ok {
		c.DLALabels = saasDLALabels
		c.TSRLabels = saasTSRLabels
	}
	return &SaasDeepDocService{doc: doc}
}

func (b *SaasDeepDocService) Name() string { return "deepdoc" }

func (b *SaasDeepDocService) DetectCells(ctx context.Context, cropped image.Image) ([]TSRCell, error) {
	return b.doc.TSR(ctx, cropped)
}

func (b *SaasDeepDocService) GroupCells(cells []TSRCell) [][]TSRCell {
	return groupTSRCellsToRowsLabeled(cells)
}

// groupTSRCellsToRowsLabeled groups TSR cells into rows using labels
// (header, row, spanning) instead of just Y proximity. Matching Python's
// gather-based approach.
func groupTSRCellsToRowsLabeled(cells []TSRCell) [][]TSRCell {
	rows := gatherTSR(cells, reRowHdr)
	spans := gatherTSR(cells, reSpan)
	clmns := gatherTSR(cells, reColumn)

	if len(rows) == 0 && len(spans) == 0 {
		return groupTSRCellsToRows(cells)
	}

	sortYFirstly(rows, 10)
	sortXFirstly(clmns, 10)

	var grouped [][]TSRCell
	var curRow []TSRCell
	curY := 0.0
	rowThreshold := 0.0
	if len(rows) > 0 {
		heights := make([]float64, len(rows))
		for i, r := range rows {
			heights[i] = r.Y1 - r.Y0
		}
		sort.Float64s(heights)
		rowThreshold = heights[len(heights)/2] * 0.5
		if rowThreshold <= 0 {
			rowThreshold = 10
		}
	}

	for _, c := range rows {
		if len(curRow) == 0 {
			curRow = append(curRow, c)
			curY = c.Y0
			continue
		}
		if c.Y0-curY > rowThreshold {
			grouped = append(grouped, curRow)
			curRow = []TSRCell{c}
			curY = c.Y0
		} else {
			curRow = append(curRow, c)
		}
	}
	if len(curRow) > 0 {
		grouped = append(grouped, curRow)
	}

	for _, s := range spans {
		for ri, row := range grouped {
			if len(row) > 0 && s.Y0 <= row[0].Y1 && s.Y1 >= row[0].Y0 {
				grouped[ri] = append(grouped[ri], s)
				break
			}
		}
	}

	for _, row := range grouped {
		sortXFirstly(row, 10)
	}

	maxCols := 0
	for _, row := range grouped {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	for i := range grouped {
		if len(grouped[i]) == 0 {
			continue // no real cells → cannot derive valid coordinates for padding
		}
		for len(grouped[i]) < maxCols {
			lastX := grouped[i][len(grouped[i])-1].X1 + 10
			rowY0 := grouped[i][0].Y0
			rowY1 := grouped[i][0].Y1
			grouped[i] = append(grouped[i], TSRCell{X0: lastX, X1: lastX + 1, Y0: rowY0, Y1: rowY1})
		}
	}

	return grouped
}
