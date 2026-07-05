
package table

import (
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func TestRowsToHTML(t *testing.T) {
	// rowsToHTML takes [][]pdf.TSRCell instead of [][]string (tableToHTML removed).
	toCells := func(rows [][]string) [][]pdf.TSRCell {
		out := make([][]pdf.TSRCell, len(rows))
		for ri, row := range rows {
			out[ri] = make([]pdf.TSRCell, len(row))
			for ci, s := range row {
				out[ri][ci] = pdf.TSRCell{Text: s}
			}
		}
		return out
	}

	t.Run("simple 2x2 table", func(t *testing.T) {
		rows := toCells([][]string{
			{"姓名", "年龄"},
			{"张三", "25"},
		})
		html := RowsToHTML(rows, "", nil, nil, nil)
		expected := "<table><tr><td >姓名</td><td >年龄</td></tr><tr><td >张三</td><td >25</td></tr></table>"
		if html != expected {
			t.Errorf("got  %q\nwant %q", html, expected)
		}
	})

	t.Run("empty table", func(t *testing.T) {
		html := RowsToHTML(nil, "", nil, nil, nil)
		if html != "<table></table>" {
			t.Errorf("expected '<table></table>', got %q", html)
		}
	})

	t.Run("single cell", func(t *testing.T) {
		rows := toCells([][]string{{"X"}})
		html := RowsToHTML(rows, "", nil, nil, nil)
		expected := "<table><tr><td >X</td></tr></table>"
		if html != expected {
			t.Errorf("got  %q\nwant %q", html, expected)
		}
	})

	t.Run("matches Python format for 公司差旅费", func(t *testing.T) {
		rows := toCells([][]string{
			{"标职务", "飞机", "火车", "轮船", "其他交通工具（不含的士）"},
			{"公司级领导人员", "经济舱位", "火车软席", "二等舱位", "按实报销"},
			{"其他工作人员", "经济舱位", "火车硬席", "三等舱位", "按实报销"},
		})
		html := RowsToHTML(rows, "", nil, nil, nil)
		if !strings.HasPrefix(html, "<table>") || !strings.HasSuffix(html, "</table>") {
			t.Errorf("not valid HTML: %s", html)
		}
		if !strings.Contains(html, "<td >标职务</td>") {
			t.Errorf("missing cell '标职务': %s", html)
		}
		if strings.Count(html, "<tr>") != 3 {
			t.Errorf("expected 3 rows, got %d", strings.Count(html, "<tr>"))
		}
	})
}

func TestRowsToHTML_HeaderRows(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "Name", Label: "table column header"},
		{X0: 101, Y0: 0, X1: 200, Y1: 30, Text: "Age", Label: "table column header"},
		{X0: 0, Y0: 35, X1: 100, Y1: 65, Text: "John", Label: "table row"},
		{X0: 101, Y0: 35, X1: 200, Y1: 65, Text: "30", Label: "table row"},
	}
	// constructTable should produce <th > for header row.
	item := &pdf.TableItem{}
	html := ConstructTable(cells, nil, "", item)
	// Header row should use <th >, data row <td >.
	if !strings.Contains(html, "<th >") {
		t.Errorf("expected <th > for header row. HTML: %s", html)
	}
	if strings.Count(html, "<th ") != 2 {
		t.Errorf("expected 2 <th > cells, got %d. HTML: %s", strings.Count(html, "<th "), html)
	}
	if strings.Count(html, "<td ") != 2 {
		t.Errorf("expected 2 <td > cells (data row), got %d", strings.Count(html, "<td "))
	}
}

func TestRowsToHTML_Colspan(t *testing.T) {
	// Box spanning 2 columns: SP annotation with HLeft/HRight covering cols 0-1.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "Name", R: 0, C: 0, H: 1, HLeft: 10, HRight: 190},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "", R: 0, C: 1, SP: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "John", R: 1, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "30", R: 1, C: 1},
	}
	rows := GroupBoxesByRC(boxes)
	spans, covered := CalSpans(rows)
	html := RowsToHTML(rows, "", nil, spans, covered)
	if !strings.Contains(html, "colspan") {
		t.Errorf("expected colspan attribute, got: %s", html)
	}
	t.Logf("HTML: %s", html)
}
