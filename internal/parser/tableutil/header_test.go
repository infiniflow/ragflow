package tableutil

import (
	"reflect"
	"testing"

	"ragflow/internal/entity"
)

// A data row must not acquire column-header semantics during HTML parsing.
func TestParseTableHTMLPreservesHeaderlessTables(t *testing.T) {
	for _, markup := range []string{
		`<table><tr><td>Alice</td><td>18</td></tr><tr><td>Bob</td><td>20</td></tr></table>`,
		`<table><tr><th scope="row">Alice</th><td>18</td></tr><tr><th scope="row">Bob</th><td>20</td></tr></table>`,
		`<table><tr><td>北京</td><td>上海</td></tr><tr><td>广州</td><td>深圳</td></tr></table>`,
	} {
		td := mustParse(t, markup)
		if td.HeaderRows != 0 {
			t.Errorf("HeaderRows = %d, want 0 for %s", td.HeaderRows, markup)
		}
	}
}

func TestTableHTMLRoundTripPreservesNoHeader(t *testing.T) {
	td := &entity.TableData{Grid: [][]entity.TableCell{
		{{Text: "Name"}, {Text: "Age"}},
		{{Text: "Alice"}, {Text: "18"}},
	}}
	back := mustParse(t, RenderTableHTML(td))
	if back.HeaderRows != 0 || !reflect.DeepEqual(back.CellTexts(), td.CellTexts()) {
		t.Fatalf("round trip changed header decision or cells: %+v", back)
	}
}

func TestDetectHeaderEvidence(t *testing.T) {
	for _, tt := range []struct {
		name     string
		html     string
		evidence HeaderEvidence
		want     Header
	}{
		{"native numeric column names", `<table><tr><th>2024</th><th>2025</th></tr><tr><td>10</td><td>20</td></tr></table>`, HeaderEvidence{Explicit: &Header{Rows: 1}}, Header{Rows: 1}},
		{"explicit absence", `<table><tr><td>Name</td><td>Age</td></tr><tr><td>Alice</td><td>18</td></tr></table>`, HeaderEvidence{Explicit: &Header{}}, Header{}},
		{"uniform style is not a header", `<table><tr><td>Alice</td><td>18</td></tr><tr><td>Bob</td><td>20</td></tr></table>`, HeaderEvidence{Styled: [][]bool{{true, true}, {true, true}}}, Header{}},
		{"predicted data row", `<table><tr><th>Alice</th><th>18</th></tr><tr><td>Bob</td><td>20</td></tr></table>`, HeaderEvidence{PredictedRows: 1}, Header{}},
		{"multi level", `<table><tr><td colspan="2">Sales</td></tr><tr><td>Quantity</td><td>Amount</td></tr><tr><td>2</td><td>30</td></tr></table>`, HeaderEvidence{}, Header{Rows: 2}},
		{"multiple samples reject a one row contrast", `<table><tr><td>alpha</td><td>beta</td></tr><tr><td>1</td><td>2</td></tr><tr><td>gamma</td><td>delta</td></tr><tr><td>epsilon</td><td>zeta</td></tr></table>`, HeaderEvidence{}, Header{}},
		{"single row prediction", `<table><tr><th>Alice</th><th>18</th></tr></table>`, HeaderEvidence{PredictedRows: 1}, Header{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			td := mustParse(t, tt.html)
			if got := DetectHeader(td.Grid, tt.evidence); got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseTableHTMLColumnScopeWithEmptyCorner(t *testing.T) {
	td := mustParse(t, `<table><tr><td></td><th scope="col">2024</th><th scope="col">2025</th></tr><tr><th scope="row">North</th><td>10</td><td>20</td></tr></table>`)
	if td.HeaderRows != 1 {
		t.Fatalf("HeaderRows = %d, want explicit column headers only", td.HeaderRows)
	}
}
