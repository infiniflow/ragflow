package table

import (
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

// ToTableData builds the canonical entity.TableData directly from a TSR cell
// grid. ConstructTable pairs it with tableutil.RenderTableHTML so a table is
// serialized by the single canonical renderer (RenderTableHTML) instead of the
// former second, compact RowsToHTML emitter. ToTableData reuses
// tableutil.BuildTableData's normalization (header count, span clamp,
// ragged-no-pad, trailing-empty trim, Covered reconstruction), so the grid it
// produces is identical to what the legacy RowsToHTML -> ParseTableHTML round
// trip produced.
//
// Covered cells are skipped here (their engulfed positions are reconstructed as
// Covered markers from the origin's span), mirroring how the legacy emitter
// never produced a cell for a covered slot. headerRows may flag non-leading rows
// as headers; only the leading consecutive run is counted (the same
// leading-prefix semantics ParseTableHTML applies to <th>).
func ToTableData(rows [][]pdf.TSRCell, caption string, headerRows map[int]bool, spanInfo map[[2]int][2]int, covered map[[2]int]bool) *entity.TableData {
	inputs := make([][]tableutil.CellInput, len(rows))
	headerFlags := make([]bool, len(rows))
	for ri, row := range rows {
		headerFlags[ri] = headerRows[ri]
		for ci, cell := range row {
			if covered[[2]int{ri, ci}] {
				continue
			}
			cs, rs := 1, 1
			if s, ok := spanInfo[[2]int{ri, ci}]; ok {
				if s[0] > 1 {
					cs = s[0]
				}
				if s[1] > 1 {
					rs = s[1]
				}
			}
			inputs[ri] = append(inputs[ri], tableutil.CellInput{
				Text:    cell.Text,
				ColSpan: cs,
				RowSpan: rs,
			})
		}
	}
	return tableutil.BuildTableData(inputs, headerFlags, caption)
}
