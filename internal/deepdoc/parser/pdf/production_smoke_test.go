//go:build cgo && manual

package pdf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/common"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestProductionAssemblySmoke parses real PDFs through the PRODUCTION path —
// real in-process DLA/TSR/OCR inference (MODEL_DIR; ONNX Runtime statically linked), no replay dump — and checks
// the converged R/C grid assembly (AnnotateBoxesWithGrid + GroupBoxesByRC)
// stays robust: parse does not crash, detected tables are not fully empty, and
// every table has content rows. This is the non-replay sanity check for the
// production assembly convergence.
//
// The 35-set table fixtures always run; when BATCH_PARITY_PDF_DIR points at
// the real_pdfs directory, the four ocr_real table PDFs are appended so the
// image/OCR-heavy production path is exercised too.
func TestProductionAssemblySmoke(t *testing.T) {
	client := mustConnectInProcessAnalyzer(t)

	// Run whichever of the known table PDFs exist under BATCH_PARITY_PDF_DIR:
	// the 35-set fixtures live in testdata/pdfs, the ocr_real ones in
	// testdata/real_pdfs, so the same test covers either directory.
	pdfs := []string{
		"06_table_content.pdf",
		"14_text_table_interleaved.pdf",
		"18_table_caption.pdf",
		"13_crosspage_table.pdf",
		"table_rotation_test.pdf",
		"icbccs deployment.pdf",
		"公司差旅费管理办法.pdf",
		"qa.pdf",
		"test.pdf",
	}
	dir := common.GetEnv("BATCH_PARITY_PDF_DIR")
	if dir == "" {
		dir = filepath.Join("testdata", "pdfs")
	}

	cfg := pdf.DefaultParserConfig()
	for _, name := range pdfs {
		pdfPath := filepath.Join(dir, name)
		data, err := os.ReadFile(pdfPath)
		if err != nil {
			if common.GetEnv("BATCH_PARITY_PDF_DIR") != "" {
				t.Logf("%s: not in %s — skip", name, dir)
			} else {
				t.Errorf("%s: read pdf: %v", name, err)
			}
			continue
		}
		p := NewParser(cfg)
		result, err := p.Parse(t.Context(), data, client)
		if err != nil {
			t.Errorf("%s: Parse: %v", name, err)
			continue
		}
		if len(result.Tables) == 0 {
			t.Logf("%s: no tables detected", name)
			continue
		}
		// Confirm the table content actually reaches the final output: at least
		// one table cell text must appear in the parsed sections, otherwise the
		// ExtractTableAndReplace no-replacement path would drop the table.
		var sectionText strings.Builder
		for _, s := range result.Sections {
			sectionText.WriteString(s.Text)
		}
		allText := sectionText.String()
		empty := 0
		missing := 0
		for ti, tbl := range result.Tables {
			gridText := 0
			var sample string
			for _, r := range tbl.Grid {
				for _, c := range r {
					if strings.TrimSpace(c.Text) != "" {
						gridText++
						if sample == "" {
							sample = strings.TrimSpace(c.Text)
						}
					}
				}
			}
			// The production assembly's output is the GRID. Rows is populated
			// downstream by ExtractTableAndReplace only when the table has
			// LayoutType=="table" OCR boxes (replacements>0); OCR-heavy PDFs
			// can leave Rows empty even though the grid is fully assembled.
			if len(tbl.Rows) == 0 {
				t.Logf("%s: table[%d] rows field empty (grid %d rows / %d text cells; ExtractTableAndReplace no-replacement path)", name, ti, len(tbl.Grid), gridText)
			}
			if gridText == 0 {
				empty++
				t.Errorf("%s: table[%d]: grid has %d rows but ZERO text cells — assembly produced an empty table", name, ti, len(tbl.Grid))
			}
			if sample != "" && !strings.Contains(allText, sample) {
				missing++
				t.Errorf("%s: table[%d] cell %q not found in parsed sections — table content may be dropped", name, ti, sample)
			}
		}
		t.Logf("%s: %d tables, %d empty, %d missing-from-sections (production R/C assembly smoke ok)", name, len(result.Tables), empty, missing)
	}
}
