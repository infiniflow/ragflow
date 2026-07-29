//go:build cgo && manual

package table

import (
	"encoding/json"
	"os"
	"path/filepath"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"strings"
	"testing"
)

// TestTableParityWithPythonBoxes reads Python's pre-merge table boxes
// (with R/C annotations) and runs them through Go's constructTable.
// If Go produces the same HTML as Python, the pipeline is correct
// and differences are from the engine layer (pdf_oxide vs pdfplumber).
func TestTableParityWithPythonBoxes(t *testing.T) {
	boxesDir := filepath.Join("testdata", "output", "py", "noocr", "table_boxes")
	entries, err := os.ReadDir(boxesDir)
	if err != nil {
		t.Skipf("Python table_boxes not found — run dump_py_results.py first: %v", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(boxesDir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}

			var pyBoxes []struct {
				X0, X1, Top, Bottom float64
				Text                string
				R, C, H, SP         int
				LayoutType          string
			}
			if err := json.Unmarshal(data, &pyBoxes); err != nil {
				t.Fatal(err)
			}

			// Convert to Go pdf.TextBox
			boxes := make([]pdf.TextBox, len(pyBoxes))
			for i, b := range pyBoxes {
				boxes[i] = pdf.TextBox{
					X0: b.X0, X1: b.X1, Top: b.Top, Bottom: b.Bottom,
					Text: b.Text, R: b.R, C: b.C, H: b.H, SP: b.SP,
					LayoutType: b.LayoutType,
				}
			}

			// Run through Go's constructTable
			item := &pdf.TableItem{}
			html := ConstructTable(nil, boxes, "", item)

			if html == "" {
				t.Error("constructTable returned empty HTML")
				return
			}
			if !strings.Contains(html, "<table>") {
				t.Error("HTML missing <table> tag")
			}

			// Verify structure
			trCount := strings.Count(html, "<tr>")
			tdCount := strings.Count(html, "<td>")
			thCount := strings.Count(html, "<th>")
			if trCount == 0 {
				t.Error("no <tr> rows found")
			}
			if tdCount == 0 && thCount == 0 {
				t.Error("no <td> or <th> cells found")
			}

			// Check no empty rows
			nonEmptyCols := 0
			for _, row := range item.Rows {
				for _, cell := range row {
					if strings.TrimSpace(cell) != "" {
						nonEmptyCols++
					}
				}
			}
			if nonEmptyCols == 0 {
				t.Errorf("all %d cells are empty — R/C path broken", tdCount+thCount)
			}

			t.Logf("%s: %d rows, %d cells (%d th), %d non-empty",
				name, trCount, tdCount+thCount, thCount, nonEmptyCols)
			t.Logf("HTML snippet: %.200s...", html)
		})
	}
}
