package table

import (
	"html"
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// SimpleRowsToHTML converts plain string-based table data to an HTML table.
// The first row is treated as a header (<th>).  Used by DOCX, XLSX, PPTX,
// and HTML parsers that produce [][]string directly.
func SimpleRowsToHTML(rows [][]string) string {
	if len(rows) == 0 {
		return "<table></table>"
	}
	nCols := 0
	for _, row := range rows {
		if len(row) > nCols {
			nCols = len(row)
		}
	}
	var b strings.Builder
	b.WriteString("<table>")
	for ri, row := range rows {
		b.WriteString("<tr>")
		tag := "td"
		if ri == 0 {
			tag = "th"
		}
		for ci := 0; ci < nCols; ci++ {
			text := ""
			if ci < len(row) {
				text = row[ci]
			}
			b.WriteString("<")
			b.WriteString(tag)
			b.WriteString(" >")
			b.WriteString(html.EscapeString(text))
			b.WriteString("</")
			b.WriteString(tag)
			b.WriteString(">")
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</table>")
	return b.String()
}

// RowsToStrings converts a grid to a string matrix. Cells covered by a span
// (Label contains "covered") are omitted, matching Python's rendered output,
// which drops covered cells entirely (HTML <td> skipped; desc_table sees
// tbl[r][c] = None) — so a covered cell never appears in the row. Span-free
// grids are unaffected.
func RowsToStrings(rows [][]pdf.TSRCell) [][]string {
	out := make([][]string, len(rows))
	for ri, row := range rows {
		var cells []string
		for _, c := range row {
			if strings.Contains(c.Label, "covered") {
				continue
			}
			cells = append(cells, c.Text)
		}
		out[ri] = cells
	}
	return out
}

func HasText(rows [][]pdf.TSRCell) bool {
	for _, row := range rows {
		for _, c := range row {
			if strings.TrimSpace(c.Text) != "" {
				return true
			}
		}
	}
	return false
}

func HasAnyText(cells []pdf.TSRCell) bool {
	for _, c := range cells {
		if strings.TrimSpace(c.Text) != "" {
			return true
		}
	}
	return false
}
