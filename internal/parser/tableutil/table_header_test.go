package tableutil

import "testing"

func TestParseTableHTMLPreservesHeaderlessRows(t *testing.T) {
	for _, markup := range []string{
		"<table><tr><td>Owner</td><td>RET</td></tr><tr><td>Cycle</td><td>90</td></tr></table>",
		"<table><tr><td></td><td></td></tr><tr><td>Owner</td><td>RET</td></tr></table>",
	} {
		td := mustParse(t, markup)
		if td.HeaderRows != 0 {
			t.Fatalf("header_rows=%d, want 0", td.HeaderRows)
		}
		if len(td.Grid) != 2 {
			t.Fatalf("rows=%d, want 2", len(td.Grid))
		}
	}
}
