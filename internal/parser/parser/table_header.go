package parser

import (
	"strings"

	"go.uber.org/zap"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

// spreadsheetTable keeps source row numbers aligned with the original grid.
// Leading text is emitted before the table rather than moved behind its header.
type spreadsheetTable struct {
	data       *entity.TableData
	rowNumbers []int
	leading    []map[string]any
}

func newSpreadsheetTable(records [][]string, rowNumbers []int, evidence tableutil.HeaderEvidence) spreadsheetTable {
	grid := make([][]entity.TableCell, len(records))
	for r, row := range records {
		for _, text := range trimCells(row) {
			grid[r] = append(grid[r], entity.TableCell{Text: text})
		}
	}
	return resolveSpreadsheetTable(grid, rowNumbers, evidence)
}

func resolveSpreadsheetTable(grid [][]entity.TableCell, rowNumbers []int, evidence tableutil.HeaderEvidence) spreadsheetTable {
	if len(rowNumbers) != len(grid) {
		rowNumbers = make([]int, len(grid))
		for i := range rowNumbers {
			rowNumbers[i] = i + 1
		}
	}
	header := tableutil.DetectHeader(grid, evidence)
	var leading []map[string]any
	for r := 0; r < header.Start; r++ {
		// Preserve every preamble value; it is neither discarded nor made
		// into a data row after the detected header.
		var values []string
		for _, cell := range grid[r] {
			if !cell.Covered && strings.TrimSpace(cell.Text) != "" {
				values = append(values, cell.Text)
			}
		}
		if len(values) == 0 {
			continue
		}
		text := strings.Join(values, "\t")
		item := NewTextJSONItem(text)
		item["positions"] = [][]float64{{0, float64(rowNumbers[r]), float64(rowNumbers[r]), 1, float64(len(grid[r]))}}
		leading = append(leading, item)
	}
	dataGrid := grid[header.Start:]
	if header.Start > 0 {
		rows := make([]int, len(grid)-header.Start)
		for r := range rows {
			rows[r] = r + header.Start
		}
		dataGrid = projectTableRows(grid, rows)
	}
	return spreadsheetTable{
		data:       &entity.TableData{Grid: dataGrid, HeaderRows: header.Rows},
		rowNumbers: rowNumbers[header.Start:], leading: leading,
	}
}

// projectTableRows copies ascending source rows and rebuilds their merge origins.
// A covered slot whose origin was removed becomes an empty ordinary cell; the
// removed origin's text is owned by its original segment, never a fabricated one.
func projectTableRows(grid [][]entity.TableCell, rows []int) [][]entity.TableCell {
	rowIndex := make(map[int]int, len(rows))
	for r, source := range rows {
		rowIndex[source] = r
	}
	part := make([][]entity.TableCell, len(rows))
	for r, source := range rows {
		part[r] = append([]entity.TableCell{}, grid[source]...)
		for c := range part[r] {
			cell := &part[r][c]
			if cell.Covered {
				if origin, ok := rowIndex[cell.OriginRow]; ok {
					cell.OriginRow = origin
				} else {
					*cell = entity.TableCell{}
				}
			} else if cell.RowSpan > 1 {
				span := 1
				for next := r + 1; next < len(rows) && rows[next]-source < cell.RowSpan; next++ {
					span++
				}
				cell.RowSpan = span
			}
		}
	}
	return part
}

// NewOCRMarkdownResult retains recognition provenance until raw Markdown is
// normalized. OCR backends must not grant native authority to generated headings.
func NewOCRMarkdownResult(markdown string) ParseResult {
	return ParseResult{OutputFormat: "markdown", Markdown: markdown, predictedTableHeaders: true}
}

// projectTableItem replaces source table markup with structured TableData.
// Callers declare whether heading markup is recognition output or native schema.
// Other items keep their original text; tables render it from their grid.
func projectTableItem(item map[string]any, predicted bool) {
	if item == nil || item["doc_type_kwd"] != "table" {
		return
	}
	text, _ := item["text"].(string)
	if !isTableHTML(text) {
		return
	}
	td, err := tableutil.ParseTableHTML(text)
	if err != nil || td == nil {
		return
	}
	if page := numberValue(item["page_number"]); page > 0 {
		td.Page = int(page)
	}
	resolveTableHeader(td, predicted)
	item["table"] = td
	delete(item, "text")
}

// resolveTableHeader classifies source tables before segmentation.
// Markup returned by recognition backends is a prediction, not native schema.
func resolveTableHeader(td *entity.TableData, predicted bool) {
	if td == nil {
		return
	}
	defer func() {
		common.Debug("document table header decision", zap.String("caption", td.Caption),
			zap.Int("page", td.Page), zap.Bool("predicted", predicted),
			zap.Bool("row_headers_only", td.RowHeadersOnly), zap.Int("header_rows", td.HeaderRows))
	}()
	if td.RowHeadersOnly {
		td.HeaderRows = 0
		return
	}
	evidence := tableutil.HeaderEvidence{}
	if !predicted && td.HeaderRows > 0 {
		evidence.Explicit = &tableutil.Header{Rows: td.HeaderRows}
	}
	h := tableutil.DetectHeader(td.Grid, evidence)
	td.HeaderRows = h.Rows
}

// NormalizeTableHeaders re-evaluates recognition-generated headings after raw
// Markdown/HTML conversion. Native declarations retain their authority. The
// source marker is never serialized to the pipeline.
func (r ParseResult) NormalizeTableHeaders(items []map[string]any) []map[string]any {
	if r.predictedTableHeaders {
		for _, item := range items {
			if td, ok := item["table"].(*entity.TableData); ok {
				resolveTableHeader(td, true)
			}
		}
	}
	return items
}
