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

// TestPipelineParityCaptionOmission documents the go_intentional divergence
// table-html-emission-format: Go's RowsToHTML does NOT emit a <caption>
// element, while Python embeds a descriptive <caption> inside each table. The
// test is a REGRESSION GUARD — it passes today (Go omits caption text) and
// flips if Go ever starts emitting caption text unexpectedly. The divergence
// is non-cell-text (cell content + structure still match, gridSim/structSim
// 100%), so it is deliberate, not a table-assembly bug.
func TestPipelineParityCaptionOmission(t *testing.T) {
	pdfs := []string{
		"06_table_content.pdf",
		"13_crosspage_table.pdf",
		"14_text_table_interleaved.pdf",
		"18_table_caption.pdf",
	}
	captionRe := regexp.MustCompile(`(?is)<caption>(.*?)</caption>`)
	for _, name := range pdfs {
		t.Run(name, func(t *testing.T) {
			goText, pyText := replayPipelineText(t, name)
			caps := captionRe.FindAllStringSubmatch(pyText, -1)
			if len(caps) == 0 {
				t.Fatalf("test setup error: Python golden has no <caption> for %s", name)
			}
			for _, c := range caps {
				caption := strings.TrimSpace(c[1])
				if caption == "" {
					continue
				}
				if strings.Contains(goText, caption) {
					t.Errorf("REGRESSION go_intentional table-html-emission-format: Go now emits caption text %q for %s — caption-omission divergence closed unexpectedly", caption, name)
				}
			}
			t.Logf("%s: Python has %d <caption> block(s); Go omits caption text (go_intentional)", name, len(caps))
		})
	}
}
