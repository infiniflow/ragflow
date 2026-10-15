package parser

import (
	"strings"

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
		dataGrid = make([][]entity.TableCell, len(grid)-header.Start)
		for r, row := range grid[header.Start:] {
			dataGrid[r] = append([]entity.TableCell{}, row...)
			for c := range dataGrid[r] {
				if dataGrid[r][c].Covered {
					dataGrid[r][c].OriginRow -= header.Start
				}
			}
		}
	}
	return spreadsheetTable{
		data:       &entity.TableData{Grid: dataGrid, HeaderRows: header.Rows},
		rowNumbers: rowNumbers[header.Start:], leading: leading,
	}
}

// resolveTableHeader runs once on source HTML, never on rendered chunks.
// Markup returned by recognition backends is a prediction, not native schema.
func resolveTableHeader(td *entity.TableData, predicted bool) {
	if td == nil {
		return
	}
	evidence := tableutil.HeaderEvidence{}
	if predicted {
		evidence.PredictedRows = td.HeaderRows
	} else if td.HeaderRows > 0 {
		evidence.Explicit = &tableutil.Header{Rows: td.HeaderRows}
	}
	h := tableutil.DetectHeader(td.Grid, evidence)
	td.HeaderRows = h.Rows
}

// NormalizeTableHeaders preserves the source's evidence level after a backend
// response is converted from Markdown/HTML to structured JSON. Native document
// declarations retain their semantics; recognition output is checked by the
// shared detector. The source marker is never serialized to the pipeline.
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
