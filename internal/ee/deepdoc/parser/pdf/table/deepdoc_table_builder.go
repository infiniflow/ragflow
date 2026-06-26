package table

import (
	"context"
	"image"
	"regexp"
	"sort"

	pdf "ragflow/internal/deepdoc/parser/pdf"
	tbl "ragflow/internal/deepdoc/parser/pdf/table"
	pdft "ragflow/internal/deepdoc/parser/pdf/type"
)

// Package ee_pdf provides Enterprise Edition PDF parser extensions.
//
// Files in this package mirror the OSS structure at internal/deepdoc/parser/pdf/
// and register EE-specific implementations via init().

// EE model label taxonomies.
// DLA: 10 classes with duplicates (matching EE Docker TSR endpoint).
var eeDLALabels = []string{
	pdft.LayoutTypeText, pdft.LayoutTypeTitle, pdft.LayoutTypeText, pdft.LayoutTypeReference,
	pdft.LayoutTypeFigure, pdft.DLALabelFigureCaption,
	pdft.LayoutTypeTable, pdft.DLALabelTableCaption, pdft.DLALabelTableCaption,
	pdft.LayoutTypeEquation, pdft.DLALabelFigureCaption,
}

// TSR: 2-class separator lines (v=vertical, h=horizontal).
var eeTSRLabels = []string{"v", "h"}

// DeepDoc label regexes — compiled once at package init.
// These match the TSR label taxonomy returned by the DeepDoc
// table structure recognition service.
var (
	reHeader = regexp.MustCompile(`.*header$`)
	reRowHdr = regexp.MustCompile(`table$|.* (row|header)`)
	// "table$" catches the default TSR label "table" (class 0), matching
	// the behaviour which uses all cells regardless of label.
	reSpan   = regexp.MustCompile(`.*spanning`)
	reColumn = regexp.MustCompile(`table column$`)
)

// gatherTSR filters cells by label regex pattern.
func gatherTSR(cells []pdft.TSRCell, re *regexp.Regexp) []pdft.TSRCell {
	var result []pdft.TSRCell
	for _, c := range cells {
		if re.MatchString(c.Label) {
			result = append(result, c)
		}
	}
	return result
}

// DeepDocTableBuilder implements pdft.TableBuilder using the EE DeepDoc TSR service
// with 2-class label taxonomy and label-aware cell grouping.
type DeepDocTableBuilder struct {
	doc pdft.DocAnalyzer
}

// NewDeepDocTableBuilder creates an EE TableBuilder backed by the DeepDoc service.
// If doc is a *DeepDocClient, its label tables are set to the EE taxonomy.
func NewDeepDocTableBuilder(doc pdft.DocAnalyzer) pdft.TableBuilder {
	if c, ok := doc.(*pdf.InferenceClient); ok {
		c.DLALabels = eeDLALabels
		c.TSRLabels = eeTSRLabels
	}
	return &DeepDocTableBuilder{doc: doc}
}

func (b *DeepDocTableBuilder) Name() string { return "deepdoc" }

func (b *DeepDocTableBuilder) DetectCells(ctx context.Context, cropped image.Image) ([]pdft.TSRCell, error) {
	return b.doc.TSR(ctx, cropped)
}

func (b *DeepDocTableBuilder) GroupCells(cells []pdft.TSRCell) [][]pdft.TSRCell {
	return groupTSRCellsToRowsLabeled(cells)
}

// groupTSRCellsToRowsLabeled groups TSR cells into rows using labels
// (header, row, spanning) instead of just Y proximity. Matching the
// label-aware gather-based approach used by the EE TSR model.
func groupTSRCellsToRowsLabeled(cells []pdft.TSRCell) [][]pdft.TSRCell {
	rows := gatherTSR(cells, reRowHdr)
	spans := gatherTSR(cells, reSpan)
	clmns := gatherTSR(cells, reColumn)

	if len(rows) == 0 && len(spans) == 0 {
		return tbl.GroupTSRCellsToRows(cells)
	}

	tbl.SortYFirstly(rows, 10)
	tbl.SortXFirstly(clmns, 10)

	var grouped [][]pdft.TSRCell
	var curRow []pdft.TSRCell
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
			curRow = []pdft.TSRCell{c}
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
		tbl.SortXFirstly(row, 10)
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
			grouped[i] = append(grouped[i], pdft.TSRCell{X0: lastX, X1: lastX + 1, Y0: rowY0, Y1: rowY1})
		}
	}

	return grouped
}

// init registers the EE TableBuilder factory for ModelEE.
func init() {
	pdf.RegisterTableBuilder(NewDeepDocTableBuilder)
}
