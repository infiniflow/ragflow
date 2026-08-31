//go:build cgo && manual

package pdf

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"ragflow/internal/deepdoc/parser/pdf/tool"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// replayPipelineText runs Go's replay pipeline for name (Python DLA/TSR/OCR
// replayed, mirroring the parity harness) and returns Go's assembled section
// text plus Python's text golden (metadata stripped). It is the shared setup
// for the caption / interleaved-paragraph divergence tests below.
func replayPipelineText(t *testing.T, name string) (goText, pyText string) {
	t.Helper()
	// Python chars are the raw replay input under testdata/charspy/; Python's
	// DLA/TSR/OCR intermediates and text golden live under testdata/output/py/ocr/.
	charspyDir := filepath.Join("testdata", "charspy")
	base := filepath.Join("testdata", "output", "py", "ocr")
	dlaDir := filepath.Join(base, "dla")
	tsrDir := filepath.Join(base, "tsr_raw")
	ocrDir := filepath.Join(base, "ocr")

	engine, err := tool.LoadPythonChars(filepath.Join(charspyDir, name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	// Mirror the harness: English documents clear chars and fall through to
	// the OCR-replay path so both sides replay the same input.
	isEnglish := false
	if v := engine.IsEnglish(); v != nil {
		isEnglish = *v
	} else if pages, _ := engine.PageCount(); util.DetectEnglish(engine.PageChars(), pages, nil) {
		isEnglish = true
	}
	if isEnglish {
		engine.ClearChars()
	}

	RegisterReplayTableBuilder()
	cfg := pdf.DefaultParserConfig()
	cfg.SortByTop = true
	analyzer := NewPythonIntermediateDocAnalyzer(name, dlaDir, tsrDir, ocrDir, engine.PageDims())
	p := NewParser(cfg)
	result, err := p.ParseRaw(t.Context(), engine, analyzer)
	if err != nil {
		t.Fatal(err)
	}

	var b strings.Builder
	for _, s := range result.Sections {
		b.WriteString(s.Text)
		b.WriteByte('\n')
	}
	goText = b.String()

	pyData, err := os.ReadFile(filepath.Join(base, "text", name+".txt"))
	if err != nil {
		t.Fatalf("read Python text golden: %v", err)
	}
	pyText = tool.StripMeta(string(pyData))
	return goText, pyText
}

// TestPipelineParity14InterleavedParagraphDropped is the end-to-end regression
// guard for the resolved go_bug table-text-interleaved-paragraph-dropped:
// 14_text_table_interleaved.pdf has a body paragraph interleaved BEFORE Table 1
// ("... Table 1 shows revenuebycategory."). Python extracts it as a text
// section; Go must keep it too. The paragraph was previously dropped because an
// unanchored "Table N" caption regex mislabeled it a caption and MergeCaptions
// removed it. This test is GREEN now (Go retains the paragraph) and turns RED
// again if that content loss ever regresses.
func TestPipelineParity14InterleavedParagraphDropped(t *testing.T) {
	name := "14_text_table_interleaved.pdf"
	goText, pyText := replayPipelineText(t, name)
	const want = "revenuebycategory"
	if !strings.Contains(pyText, want) {
		t.Fatalf("test setup error: Python golden does not contain %q", want)
	}
	if !strings.Contains(goText, want) {
		t.Errorf("REGRESSION go_bug table-text-interleaved-paragraph-dropped: Go output is missing the interleaved body paragraph containing %q (Python keeps it). Real content loss, not HTML format.", want)
	}
}

// TestPipelineParityCaptionEmitted locks the fix for the content-loss half of
// go_bug table-html-emission-format: Go used to DROP the standalone caption
// section entirely, so the caption text vanished from output. Go now retains
// it by injecting the caption text as a <caption> element inside the target
// table's HTML (MergeCaptions -> injectCaption). This guard fails if the
// retention ever regresses (caption dropped -> no <caption> emitted).
//
// Scope: PDFs that carry DLA "table caption" boxes (06/13/14/18). For those,
// Go now emits <caption>; the emitted text is real PDF content, so it must
// also appear somewhere in Python's full extracted text (proving Go did not
// fabricate it). Go and Python CHOOSE/ORDER caption sentences differently (a
// caption-selection divergence  -  a separate, accepted go_intentional
// residual), so we assert each emitted caption SENTENCE, not the whole
// combined string byte-equally.
//
// Two regression guards:
//  1. VALID HTML: no <table> may contain more than one <caption> (HTML allows
//     only one; extras are dropped by browsers/Markdown converters). A table
//     with caption text both above and below used to emit multiple <caption>
//     elements  -  that is the bug this guard locks against.
//  2. RETENTION: every caption sentence Go extracted must survive in the
//     output AND be real PDF content (present in Python's full text).
func TestPipelineParityCaptionEmitted(t *testing.T) {
	pdfs := []string{
		"06_table_content.pdf",
		"13_crosspage_table.pdf",
		"14_text_table_interleaved.pdf",
		"18_table_caption.pdf",
	}
	// Expected caption sentences Go extracts for each PDF (whitespace collapsed).
	// These are the DLA "table caption" box texts; Go combines them into one
	// <caption> per table. They differ in wording/order from Python's <caption>
	// (selection divergence) but must all be retained as real content. 13's two
	// caption boxes (page 0 title + page 2 footer) now attach to the tall
	// cross-page table after the edge-distance fix.
	expected := map[string][]string{
		"06_table_content.pdf": {
			"Table 1: Quarterly sales by product category (in USD)",
			"The following table summarizes the quarterly sales performance across different product categories.",
		},
		"13_crosspage_table.pdf": {
			"Extended Financial Report",
			"Table: Monthly financial summary FY2024",
		},
		"14_text_table_interleaved.pdf": {
			"Table 1: Revenue",
			"Analysis continues with additional metrics.",
			"Table 2: Satisfaction Analysis continues with additional metrics.",
		},
		"18_table_caption.pdf": {
			"Table 1: Product specification comparison (2024 Q2)",
			"Product Specifications",
		},
	}
	captionRe := regexp.MustCompile(`(?is)<caption>(.*?)</caption>`)
	tableRe := regexp.MustCompile(`(?is)<table>.*?</table>`)
	wsRe := regexp.MustCompile(`\s+`)
	for _, name := range pdfs {
		t.Run(name, func(t *testing.T) {
			goText, pyText := replayPipelineText(t, name)
			// Guard 1: valid HTML  -  at most one <caption> per <table>.
			for _, tbl := range tableRe.FindAllString(goText, -1) {
				if n := strings.Count(tbl, "<caption>"); n > 1 {
					t.Errorf("REGRESSION table-html-emission-format: a <table> contains %d <caption> elements (HTML allows only one; extras are dropped by consumers): %q", n, tbl)
				}
			}
			caps := captionRe.FindAllStringSubmatch(goText, -1)
			if len(caps) == 0 {
				t.Fatalf("REGRESSION table-html-emission-format: Go emitted NO <caption> for %s  -  the standalone caption section was dropped (Python keeps caption text)", name)
			}
			// Guard 2: every expected sentence retained AND real content.
			var allGo strings.Builder
			for _, c := range caps {
				allGo.WriteString(wsRe.ReplaceAllString(strings.TrimSpace(c[1]), " "))
				allGo.WriteByte(' ')
			}
			goMerged := allGo.String()
			for _, w := range expected[name] {
				want := wsRe.ReplaceAllString(strings.TrimSpace(w), " ")
				if !strings.Contains(goMerged, want) {
					t.Errorf("REGRESSION table-html-emission-format: Go dropped caption sentence %q for %s", w, name)
				}
				if !strings.Contains(pyText, want) {
					t.Errorf("REGRESSION table-html-emission-format: Go's caption sentence %q for %s not found in Python text  -  may be fabricated, not real PDF content", w, name)
				}
			}
			t.Logf("%s: Go emits %d <caption> block(s), all sentences retained and real", name, len(caps))
		})
	}
}
