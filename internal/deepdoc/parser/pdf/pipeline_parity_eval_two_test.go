//go:build cgo && manual

package pdf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/deepdoc/parser/pdf/tool"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// runReplayText runs Go's pipeline for a PDF with Python's DLA/TSR/OCR
// replayed (mirroring the parity harness) and returns the assembled section
// text.
func runReplayText(t *testing.T, name string) string {
	t.Helper()
	base := filepath.Join("testdata", "output", "py", "ocr")
	engine, err := tool.LoadPythonChars(filepath.Join("testdata", "charspy", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
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
	analyzer := NewPythonIntermediateDocAnalyzer(name, filepath.Join(base, "dla"), filepath.Join(base, "tsr_raw"), filepath.Join(base, "ocr"), engine.PageDims())
	p := NewParser(cfg)
	result, err := p.ParseRaw(t.Context(), engine, analyzer)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, s := range result.Sections {
		sb.WriteString(s.Text)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// loadPyGoldenText returns the Python golden section text (meta line
// stripped) for a PDF.
func loadPyGoldenText(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "output", "py", "ocr", "text", name+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	return stripMetaLine(string(raw))
}

func stripMetaLine(s string) string {
	if i := strings.LastIndex(s, "\n#@meta"); i >= 0 {
		return s[:i]
	}
	return s
}

// assertTextParity asserts Go's assembled section text is structurally and
// content-identical to Python's golden: textSim==100% (same non-space char
// multiset, i.e. no content lost or added) AND same section count (line
// structure preserved). Both sides consume the same replayed intermediate, so
// equivalent processing logic must yield identical output.
func assertTextParity(t *testing.T, name string) {
	t.Helper()
	goText := runReplayText(t, name)
	pyText := stripMetaLine(loadPyGoldenText(t, name))

	goSections := nonEmptyLines(goText)
	pySections := nonEmptyLines(pyText)
	sim := tool.CharSimilarity(goText, pyText)

	if sim < 100 || len(goSections) != len(pySections) {
		t.Errorf("OPEN go_bug (eval_two_* content/structure loss): %s sections GO=%d PY=%d textSim=%.1f%%",
			name, len(goSections), len(pySections), sim)
		t.Logf("GO text:\n%s", goText)
		t.Logf("PY golden:\n%s", pyText)
	}
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// TestPipelineParityEvalTwoWideGutter: PDF = two columns (16 lines each).
// Go's text assembly DROPS 2 real lines (one '...Xx' and one '...xXx',
// 30 vs 32 line-fragments), while Python keeps all 32. Tracks the content
// loss reported by the harness (textSim 96.7%).
func TestPipelineParityEvalTwoWideGutter(t *testing.T) {
	assertTextParity(t, "eval_two_wide_gutter.pdf")
}

// TestPipelineParityEvalTwoIndentedFirstPara: PDF = left column 8 independent
// lines (overlapping middle text), right column 16. Go COLLAPSES the left
// column's 8 lines into ONE section (3 vs 16 sections, 23 vs 24
// line-fragments), losing the per-line structure Python preserves. Tracks
// the harness textSim 97.9% gap.
func TestPipelineParityEvalTwoIndentedFirstPara(t *testing.T) {
	assertTextParity(t, "eval_two_indented_first_para.pdf")
}
