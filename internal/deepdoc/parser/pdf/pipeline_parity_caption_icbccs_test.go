//go:build cgo && manual

package pdf

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/deepdoc/parser/pdf/tool"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// replayPipelineTextVariant runs Go's replay pipeline for a custom dataset
// variant (e.g. "ocr_real"), reading Python's chars/DLA/TSR/OCR from the
// variant's data root (BATCH_PARITY_DATA_ROOT). It mirrors the parity harness
// setup for non-default datasets and returns Go's assembled section text,
// Python's text golden (metadata stripped), and the Go sections for
// inspection.
func replayPipelineTextVariant(t *testing.T, variant, name string) (goText, pyText string, sections []pdf.Section) {
	t.Helper()
	dirs := tool.ParityDirsFor(variant)
	engine, err := tool.LoadPythonChars(filepath.Join(dirs.Charspy, name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if v := engine.IsEnglish(); v != nil && *v {
		engine.ClearChars()
	} else if pages, _ := engine.PageCount(); util.DetectEnglish(engine.PageChars(), pages, nil) {
		engine.ClearChars()
	}
	RegisterReplayTableBuilder()
	cfg := pdf.DefaultParserConfig()
	cfg.SortByTop = true
	analyzer := NewPythonIntermediateDocAnalyzer(name, dirs.DLA, dirs.TSRRaw, dirs.OCR, engine.PageDims())
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
	pyData, err := os.ReadFile(filepath.Join(dirs.Text, name+".txt"))
	if err != nil {
		t.Fatalf("read Python text golden: %v", err)
	}
	pyText = tool.StripMeta(string(pyData))
	return goText, pyText, result.Sections
}

// TestPipelineParityIcbccsCaptionEmitted is the TDD guard for the known_diffs
// rule noncell-icbccs-caption-dropped (icbccs deployment.pdf, ocr_real).
//
// The caption text (请求参数 x2, 请求参数枚举值 x1) IS present in the ocr_real
// replay intermediates: the OCR dump has the boxes, and DLA replay types them
// as 'table caption' via AnnotateBoxLayouts. The original gap was a REAL Go bug
// in MergeCaptions.findTables (table/merge_captions.go): a narrow caption
// directly above a much wider table was rejected because dx² exceeded
// maxCaptionGap, so the caption was silently dropped. The fix adds
// maxCaptionVGap so vertically-adjacent captions attach regardless of
// horizontal offset. This test is now a real assertion: it fails if Go does
// not emit <caption>请求参数</caption> (the fixed behavior). See known_diffs
// rule noncell-icbccs-caption-dropped (go_bug, resolved).
//
// Requires BATCH_PARITY_DATA_ROOT (the shared ocr_real dump dir); skips
// otherwise. Run via: build.sh --test-manual with BATCH_PARITY_VARIANT=ocr_real
// and BATCH_PARITY_DATA_ROOT set.
func TestPipelineParityIcbccsCaptionEmitted(t *testing.T) {
	const name = "icbccs deployment.pdf"
	if common.GetEnv(common.EnvBatchParityDataRoot) == "" {
		t.Skip("BATCH_PARITY_DATA_ROOT not set; ocr_real dumps unavailable")
	}
	t.Setenv(common.EnvBatchParityVariant, "ocr_real")
	goText, pyText, sections := replayPipelineTextVariant(t, "ocr_real", name)

	// Diagnostic: surface the layout type Go assigns to the caption text, so
	// the missing caption signal is identifiable.
	for _, s := range sections {
		if strings.Contains(s.Text, "请求参数") {
			t.Logf("DIAG section LayoutType=%q text=%q", s.LayoutType, strings.TrimSpace(s.Text))
		}
	}

	captionRe := regexp.MustCompile(`(?is)<caption>(.*?)</caption>`)
	caps := captionRe.FindAllStringSubmatch(goText, -1)
	if len(caps) == 0 {
		// No <caption> in Go output. The known_diffs rule
		// noncell-icbccs-caption-dropped was a go_bug (resolved by the
		// maxCaptionVGap fix in MergeCaptions.findTables). A missing caption
		// here is a REAL REGRESSION, not the old environmental gap — the
		// caption text IS in the ocr_real replay intermediates. Fail so the
		// guard actually guards.
		t.Errorf("REGRESSION (noncell-icbccs-caption-dropped): Go emits no <caption> for icbccs; the caption text is present in the ocr_real replay intermediates (OCR dump + DLA 'table caption' regions), so this means MergeCaptions dropped it. Python golden has caption: %v", strings.Contains(pyText, "请求参数"))
	}
	var merged strings.Builder
	for _, c := range caps {
		merged.WriteString(strings.TrimSpace(c[1]))
		merged.WriteByte(' ')
	}
	if !strings.Contains(merged.String(), "请求参数") {
		t.Errorf("Go emitted <caption> but text missing 请求参数 (real source caption); got %q", merged.String())
	}
	if !strings.Contains(pyText, "请求参数") {
		t.Errorf("test setup error: Python golden missing 请求参数")
	}
}
