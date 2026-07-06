package table

import (
	"fmt"
	"html"
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func RowsToHTML(rows [][]pdf.TSRCell, caption string, headerRows map[int]bool, spanInfo map[[2]int][2]int, covered map[[2]int]bool) string {
	var b strings.Builder
	b.WriteString("<table>")
	if caption != "" {
		b.WriteString("<caption>")
		b.WriteString(html.EscapeString(caption))
		b.WriteString("</caption>")
	}
	for ri, row := range rows {
		b.WriteString("<tr>")
		for ci, cell := range row {
			if covered[[2]int{ri, ci}] { continue }
			tag := "td"
			if headerRows[ri] { tag = "th" }
			b.WriteString("<")
			b.WriteString(tag)
			sp := ""
			if s, ok := spanInfo[[2]int{ri, ci}]; ok {
				if s[0] > 1 {
					sp = fmt.Sprintf("colspan=%d", s[0])
				}
				if s[1] > 1 {
					if sp != "" { sp += " " }
					sp += fmt.Sprintf("rowspan=%d", s[1])
				}
			}
			if sp != "" {
				b.WriteString(" ")
				b.WriteString(sp)
			}
			b.WriteString(" >")
			b.WriteString(html.EscapeString(cell.Text))
			b.WriteString("</")
			b.WriteString(tag)
			b.WriteString(">")
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</table>")
	return b.String()
}

// SimpleRowsToHTML converts plain string-based table data to an HTML table.
// The first row is treated as a header (<th>).  Used by DOCX, XLSX, PPTX,
// and HTML parsers that produce [][]string directly.
func SimpleRowsToHTML(rows [][]string) string {
	if len(rows) == 0 {
		return "<table></table>"
	}
	nCols := 0
	for _, row := range rows {
		if len(row) > nCols { nCols = len(row) }
	}
	var b strings.Builder
	b.WriteString("<table>")
	for ri, row := range rows {
		b.WriteString("<tr>")
		tag := "td"
		if ri == 0 { tag = "th" }
		for ci := 0; ci < nCols; ci++ {
			text := ""
			if ci < len(row) { text = row[ci] }
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

func RowsToStrings(rows [][]pdf.TSRCell) [][]string {
	out := make([][]string, len(rows))
	for ri, row := range rows {
		out[ri] = make([]string, len(row))
		for ci, c := range row {
			out[ri][ci] = c.Text
		}
	}
	return out
}

func HasText(rows [][]pdf.TSRCell) bool {
	for _, row := range rows {
		for _, c := range row {
			if strings.TrimSpace(c.Text) != "" { return true }
		}
	}
	return false
}

func HasAnyText(cells []pdf.TSRCell) bool {
	for _, c := range cells {
		if strings.TrimSpace(c.Text) != "" { return true }
	}
	return false
}
