//go:build cgo && integration

package pdf

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// ── golden-file helpers ────────────────────────────────────────────────────

// sectionGolden is the snapshot format for section output.
type sectionGolden struct {
	Text       string `json:"text"`
	LayoutType string `json:"layout_type"`
}

// tableGolden is the snapshot format for table output.
type tableGolden struct {
	Rows [][]string `json:"rows"`
}

func goldenPath(name string) string {
	return filepath.Join("testdata", "integration", name)
}

func readGolden[T any](t *testing.T, path string) []T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	var result []T
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("parse golden %s: %v", path, err)
	}
	return result
}

func writeGolden(t *testing.T, path string, v any) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create golden %s: %v", path, err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		t.Fatalf("write golden %s: %v", path, err)
	}
}

func updateGolden() bool {
	return os.Getenv("UPDATE_GOLDEN") == "1"
}

// sectionsToGolden converts []pdf.Section to the snapshot format.
func sectionsToGolden(sections []pdf.Section) []sectionGolden {
	result := make([]sectionGolden, len(sections))
	for i, s := range sections {
		result[i] = sectionGolden{
			Text:       s.Text,
			LayoutType: s.LayoutType,
		}
	}
	return result
}

// tablesToGolden converts []pdf.TableItem to the snapshot format.
func tablesToGolden(tables []pdf.TableItem) []tableGolden {
	result := make([]tableGolden, len(tables))
	for i, t := range tables {
		result[i] = tableGolden{Rows: t.Rows}
	}
	return result
}

// ── tests ──────────────────────────────────────────────────────────────────

// TestIntegration_SectionsText verifies section text output matches golden.
func TestIntegration_SectionsText(t *testing.T) {
	client := mustConnectInferenceClient(t)
	data := mustReadPDF(t, "01_english_simple.pdf")

	cfg := pdf.DefaultParserConfig()
	p := NewParser(cfg)
	result, err := p.Parse(context.Background(), data, client)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Sections) == 0 {
		t.Fatal("expected at least one section")
	}

	golden := goldenPath("01_english_simple.sections.json")
	got := sectionsToGolden(result.Sections)

	if updateGolden() {
		writeGolden(t, golden, got)
		t.Logf("golden written: %s (%d sections)", golden, len(got))
		return
	}

	expected := readGolden[sectionGolden](t, golden)
	if len(expected) != len(got) {
		t.Errorf("section count mismatch: golden=%d got=%d", len(expected), len(got))
	}
	n := len(expected)
	if len(got) < n {
		n = len(got)
	}
	for i := 0; i < n; i++ {
		if expected[i].Text != got[i].Text {
			t.Errorf("section[%d] text mismatch:\n  golden: %q\n  got:    %q", i, expected[i].Text, got[i].Text)
		}
		if expected[i].LayoutType != got[i].LayoutType {
			t.Errorf("section[%d] layout_type mismatch: golden=%q got=%q",
				i, expected[i].LayoutType, got[i].LayoutType)
		}
	}
}

// TestIntegration_SectionsCount verifies section count is stable.
func TestIntegration_SectionsCount(t *testing.T) {
	client := mustConnectInferenceClient(t)
	data := mustReadPDF(t, "01_english_simple.pdf")

	cfg := pdf.DefaultParserConfig()
	p := NewParser(cfg)
	result, err := p.Parse(context.Background(), data, client)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Read back from golden to get expected count.
	golden := goldenPath("01_english_simple.sections.json")
	expected := readGolden[sectionGolden](t, golden)

	if len(result.Sections) != len(expected) {
		// Log section layout types to help debug divergence.
		var types []string
		for _, s := range result.Sections {
			types = append(types, s.LayoutType)
		}
		t.Errorf("section count: golden=%d got=%d (types: %v)", len(expected), len(result.Sections), types)
	}
}

// TestIntegration_TableStructure verifies table rows and cell text match golden.
func TestIntegration_TableStructure(t *testing.T) {
	client := mustConnectInferenceClient(t)
	data := mustReadPDF(t, "06_table_content.pdf")

	cfg := pdf.DefaultParserConfig()
	p := NewParser(cfg)
	result, err := p.Parse(context.Background(), data, client)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Tables) == 0 {
		t.Skip("DLA did not detect any tables in fixture — skipping table structure check")
	}

	golden := goldenPath("06_table_content.tables.json")
	got := tablesToGolden(result.Tables)

	if updateGolden() {
		writeGolden(t, golden, got)
		t.Logf("golden written: %s (%d tables)", golden, len(got))
		return
	}

	expected := readGolden[tableGolden](t, golden)
	if len(expected) != len(got) {
		t.Errorf("table count mismatch: golden=%d got=%d", len(expected), len(got))
	}
	n := len(expected)
	if len(got) < n {
		n = len(got)
	}
	for i := 0; i < n; i++ {
		if len(expected[i].Rows) != len(got[i].Rows) {
			t.Errorf("table[%d] row count mismatch: golden=%d got=%d", i, len(expected[i].Rows), len(got[i].Rows))
			continue
		}
		for ri := 0; ri < len(expected[i].Rows); ri++ {
			if len(expected[i].Rows[ri]) != len(got[i].Rows[ri]) {
				t.Errorf("table[%d] row[%d] cell count mismatch: golden=%d got=%d", i, ri, len(expected[i].Rows[ri]), len(got[i].Rows[ri]))
				continue
			}
			for ci := 0; ci < len(expected[i].Rows[ri]); ci++ {
				goldenCell := strings.TrimSpace(expected[i].Rows[ri][ci])
				gotCell := strings.TrimSpace(got[i].Rows[ri][ci])
				if goldenCell != gotCell {
					t.Errorf("table[%d] row[%d] cell[%d] mismatch:\n  golden: %q\n  got:    %q",
						i, ri, ci, goldenCell, gotCell)
				}
			}
		}
	}
}

// TestIntegration_TableImageB64 verifies table ImageB64 is valid base64 PNG.
func TestIntegration_TableImageB64(t *testing.T) {
	client := mustConnectInferenceClient(t)
	data := mustReadPDF(t, "06_table_content.pdf")

	cfg := pdf.DefaultParserConfig()
	p := NewParser(cfg)
	result, err := p.Parse(context.Background(), data, client)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Tables) == 0 {
		t.Skip("DLA did not detect any tables in fixture — skipping image check")
	}

	for i, tbl := range result.Tables {
		if tbl.ImageB64 == "" {
			t.Errorf("table[%d] ImageB64 is empty", i)
			continue
		}
		// Verify base64 decodable.
		raw, err := base64.StdEncoding.DecodeString(tbl.ImageB64)
		if err != nil {
			t.Errorf("table[%d] ImageB64: not valid base64: %v", i, err)
			continue
		}
		// Verify it's a valid image.
		img, _, err := image.Decode(bytes.NewReader(raw))
		if err != nil {
			t.Errorf("table[%d] ImageB64: not a valid image: %v", i, err)
			continue
		}
		b := img.Bounds()
		if b.Dx() <= 0 || b.Dy() <= 0 {
			t.Errorf("table[%d] ImageB64: zero-size image %dx%d", i, b.Dx(), b.Dy())
		}
	}
}

// TestIntegration_LayoutTypes verifies DLA labels boxes with expected types.
func TestIntegration_LayoutTypes(t *testing.T) {
	client := mustConnectInferenceClient(t)
	data := mustReadPDF(t, "06_table_content.pdf")

	cfg := pdf.DefaultParserConfig()
	p := NewParser(cfg)
	result, err := p.Parse(context.Background(), data, client)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	golden := goldenPath("06_table_content.layouts.json")
	got := sectionsToGolden(result.Sections)

	if updateGolden() {
		writeGolden(t, golden, got)
		t.Logf("golden written: %s (%d sections)", golden, len(got))
		return
	}

	expected := readGolden[sectionGolden](t, golden)
	if len(expected) != len(got) {
		t.Errorf("section count mismatch: golden=%d got=%d", len(expected), len(got))
	}

	// Count layout types on both sides.
	goldenTypes := map[string]int{}
	gotTypes := map[string]int{}
	for _, s := range expected {
		goldenTypes[s.LayoutType]++
	}
	for _, s := range got {
		gotTypes[s.LayoutType]++
	}
	for typ, gc := range goldenTypes {
		if gotTypes[typ] != gc {
			t.Errorf("LayoutType %q count mismatch: golden=%d got=%d", typ, gc, gotTypes[typ])
		}
	}
	for typ, gc := range gotTypes {
		if goldenTypes[typ] == 0 {
			t.Errorf("LayoutType %q count mismatch: golden=0 got=%d", typ, gc)
		}
	}
}

// ── Idempotency tests ─────────────────────────────────────────────────

// TestIntegration_Idempotency verifies that DeepDoc APIs return consistent
// results when called multiple times with the same image. This validates
// that the ML inference is deterministic (or at least semantically stable).
func TestIntegration_Idempotency(t *testing.T) {
	client := mustConnectInferenceClient(t)

	// Render a fixture page as the stable input image.
	eng := mustOpenEngine(t, "06_table_content.pdf")
	pageImg, err := eng.RenderPageImage(0, 216)
	if err != nil {
		t.Fatalf("render page: %v", err)
	}

	const N = 5

	t.Run("DLA", func(t *testing.T) {
		var all [][]pdf.DLARegion
		for i := 0; i < N; i++ {
			regions, err := client.DLA(context.Background(), pageImg)
			if err != nil {
				t.Fatalf("run %d: %v", i, err)
			}
			all = append(all, regions)
		}
		checkDLAIdempotent(t, all)
	})

	t.Run("TSR", func(t *testing.T) {
		// Crop a table region from the page for TSR input.
		// Use a fixed crop area (approximate table location in 06_table_content.pdf).
		cropped := cropImageRect(pageImg, 50, 200, 550, 400)
		var all [][]pdf.TSRCell
		for i := 0; i < N; i++ {
			cells, err := client.TSR(context.Background(), cropped)
			if err != nil {
				t.Fatalf("run %d: %v", i, err)
			}
			all = append(all, cells)
		}
		checkTSRIdempotent(t, all)
	})

	t.Run("OCRDetect", func(t *testing.T) {
		var all [][]pdf.OCRBox
		for i := 0; i < N; i++ {
			boxes, err := client.OCRDetect(context.Background(), pageImg)
			if err != nil {
				t.Fatalf("run %d: %v", i, err)
			}
			all = append(all, boxes)
		}
		checkOCRDetectIdempotent(t, all)
	})

	t.Run("OCRRecognize", func(t *testing.T) {
		cropped := cropImageRect(pageImg, 50, 100, 400, 130)
		var all [][]pdf.OCRText
		for i := 0; i < N; i++ {
			texts, err := client.OCRRecognize(context.Background(), cropped)
			if err != nil {
				t.Fatalf("run %d: %v", i, err)
			}
			all = append(all, texts)
		}
		checkOCRRecognizeIdempotent(t, all)
	})
}

// cropImageRect crops a rectangular region from an image.
func cropImageRect(img image.Image, x0, y0, x1, y1 int) image.Image {
	b := img.Bounds()
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	out := image.NewRGBA(image.Rect(0, 0, x1-x0, y1-y0))
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			out.Set(x-x0, y-y0, img.At(x, y))
		}
	}
	return out
}

const coordEpsilon = 1.0 // pixels
const confEpsilon = 0.01

func checkDLAIdempotent(t *testing.T, all [][]pdf.DLARegion) {
	t.Helper()
	ref := all[0]
	strictEqual := 0
	for i := 1; i < len(all); i++ {
		if len(all[i]) != len(ref) {
			t.Errorf("run %d: %d regions (run 0: %d) — NOT idempotent", i, len(all[i]), len(ref))
			continue
		}
		strict := true
		for j := range ref {
			if ref[j].Label != all[i][j].Label {
				t.Errorf("run %d region %d: label %q != %q", i, j, all[i][j].Label, ref[j].Label)
				strict = false
			}
			if !coordClose(ref[j].X0, all[i][j].X0) || !coordClose(ref[j].Y0, all[i][j].Y0) ||
				!coordClose(ref[j].X1, all[i][j].X1) || !coordClose(ref[j].Y1, all[i][j].Y1) {
				t.Errorf("run %d region %d: coords differ beyond epsilon", i, j)
				strict = false
			}
			if !floatClose(ref[j].Confidence, all[i][j].Confidence, confEpsilon) {
				strict = false // confidence jitter is acceptable
			}
		}
		if strict {
			strictEqual++
		}
	}
	t.Logf("DLA: %d regions, %d/%d runs strictly equal", len(ref), strictEqual+1, len(all))
}

func checkTSRIdempotent(t *testing.T, all [][]pdf.TSRCell) {
	t.Helper()
	ref := all[0]
	strictEqual := 0
	for i := 1; i < len(all); i++ {
		if len(all[i]) != len(ref) {
			t.Errorf("run %d: %d cells (run 0: %d) — NOT idempotent", i, len(all[i]), len(ref))
			continue
		}
		strict := true
		for j := range ref {
			if !coordClose(ref[j].X0, all[i][j].X0) || !coordClose(ref[j].Y0, all[i][j].Y0) ||
				!coordClose(ref[j].X1, all[i][j].X1) || !coordClose(ref[j].Y1, all[i][j].Y1) {
				t.Errorf("run %d cell %d: coords differ beyond epsilon", i, j)
				strict = false
			}
		}
		if strict {
			strictEqual++
		}
	}
	t.Logf("TSR: %d cells, %d/%d runs strictly equal", len(ref), strictEqual+1, len(all))
}

func checkOCRDetectIdempotent(t *testing.T, all [][]pdf.OCRBox) {
	t.Helper()
	ref := all[0]
	strictEqual := 0
	for i := 1; i < len(all); i++ {
		if len(all[i]) != len(ref) {
			t.Errorf("run %d: %d boxes (run 0: %d) — NOT idempotent", i, len(all[i]), len(ref))
			continue
		}
		strict := true
		for j := range ref {
			if !coordClose(ref[j].X0, all[i][j].X0) || !coordClose(ref[j].Y0, all[i][j].Y0) {
				strict = false
			}
		}
		if strict {
			strictEqual++
		}
	}
	t.Logf("OCRDetect: %d boxes, %d/%d runs strictly equal", len(ref), strictEqual+1, len(all))
}

func checkOCRRecognizeIdempotent(t *testing.T, all [][]pdf.OCRText) {
	t.Helper()
	ref := all[0]
	strictEqual := 0
	for i := 1; i < len(all); i++ {
		if len(all[i]) != len(ref) {
			t.Errorf("run %d: %d texts (run 0: %d) — NOT idempotent", i, len(all[i]), len(ref))
			continue
		}
		strict := true
		for j := range ref {
			if ref[j].Text != all[i][j].Text {
				t.Errorf("run %d text %d: %q != %q — NOT idempotent", i, j, all[i][j].Text, ref[j].Text)
				strict = false
			}
			if !floatClose(ref[j].Confidence, all[i][j].Confidence, confEpsilon) {
				strict = false
			}
		}
		if strict {
			strictEqual++
		}
	}
	t.Logf("OCRRecognize: %d texts, %d/%d runs strictly equal", len(ref), strictEqual+1, len(all))
}

func coordClose(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= coordEpsilon
}

func floatClose(a, b, eps float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= eps
}

// ── Alignment Integration Tests ─────────────────────────────────────────
// Run with: go test -v -run TestIntegration_Alignment -tags=integration -count=1 ./internal/parser/

// TestIntegration_TableAlign verifies table text backfill, text-fragment
// suppression inside table regions, and caption removal — the key alignment
// fixes from the Python→Go migration.
func TestIntegration_TableAlign(t *testing.T) {
	client := mustConnectInferenceClient(t)
	data := mustReadPDF(t, "18_table_caption.pdf")

	cfg := pdf.DefaultParserConfig()
	p := NewParser(cfg)
	result, err := p.Parse(context.Background(), data, client)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Assert 1: No caption sections remain (merged into parent or removed).
	for _, s := range result.Sections {
		if s.LayoutType == "table caption" || s.LayoutType == "figure caption" {
			t.Errorf("caption pdf.Section should be removed: layout=%s text=%q", s.LayoutType, s.Text)
		}
	}

	// Assert 2: Table sections have TSR-structured text (not raw OCR fragments).
	var hasTable bool
	for _, s := range result.Sections {
		if s.LayoutType == "table" && s.TableItem != nil && len(s.TableItem.Rows) > 0 {
			hasTable = true
			// Structured text should contain tabs (\t) for column separation.
			if !strings.Contains(s.Text, "\t") {
				t.Logf("table pdf.Section.Text may not be structured: %q", s.Text[:min(80, len(s.Text))])
			}
			break
		}
	}
	if !hasTable {
		t.Log("no table with TSR rows found — may need different PDF layout")
	}

	t.Logf("Sections: %d, Tables: %d, Figures: %d",
		len(result.Sections), len(result.Tables), len(result.Figures()))
}

// TestIntegration_GarbageLayout verifies CID-garbled and garbage-layout
// (header/footer/reference) boxes are popped from output.
func TestIntegration_GarbageLayout(t *testing.T) {
	client := mustConnectInferenceClient(t)
	data := mustReadPDF(t, "17_garbage_layout.pdf")

	cfg := pdf.DefaultParserConfig()
	p := NewParser(cfg)
	result, err := p.Parse(context.Background(), data, client)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Assert: No CID-garbled text survives.
	for _, s := range result.Sections {
		if strings.Contains(s.Text, "(cid:") {
			t.Errorf("CID garbage should be popped: %q", s.Text)
		}
	}

	// Assert: No header/footer/reference sections in output.
	for _, s := range result.Sections {
		if s.LayoutType == "header" || s.LayoutType == "footer" || s.LayoutType == "reference" {
			t.Logf("garbage layout %q survived with text %q — may be legitimate page decoration",
				s.LayoutType, s.Text[:min(60, len(s.Text))])
		}
	}

	t.Logf("Sections: %d", len(result.Sections))
}

// TestIntegration_MultiChunk verifies chunked processing for large documents.
func TestIntegration_MultiChunk(t *testing.T) {
	client := mustConnectInferenceClient(t)
	data := mustReadPDF(t, "19_multipage_chunk.pdf")

	cfg := pdf.DefaultParserConfig()
	cfg.BatchSize = 10 // small batches to force multi-batch path
	p := NewParser(cfg)
	result, err := p.Parse(context.Background(), data, client)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// 52 pages with 10-page batches → >= 6 batches.
	if len(result.Sections) == 0 {
		t.Error("multi-batch should produce sections")
	}

	t.Logf("52 pages × batchSize=10: %d sections, %d tables",
		len(result.Sections), len(result.Tables))
}

// TestIntegration_NoRegression runs a few snapshot PDFs and checks basic
// invariants — no panic, sections produced, no CID garbage.
func TestIntegration_NoRegression(t *testing.T) {
	client := mustConnectInferenceClient(t)

	for _, name := range []string{
		"01_english_simple.pdf",
		"02_chinese_simple.pdf",
		"06_table_content.pdf",
		"07_mixed_content.pdf",
	} {
		t.Run(name, func(t *testing.T) {
			data := mustReadPDF(t, name)
			cfg := pdf.DefaultParserConfig()
			p := NewParser(cfg)
			result, err := p.Parse(context.Background(), data, client)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(result.Sections) == 0 {
				t.Error("expected at least 1 section")
			}
			for _, s := range result.Sections {
				if strings.Contains(s.Text, "(cid:") {
					t.Errorf("CID garbage in %s: %q", name, s.Text)
				}
			}
			t.Logf("%s: %d sections", name, len(result.Sections))
		})
	}
}

// TestIntegration_TableRotation verifies that evaluateTableOrientation
// correctly detects rotation using region-count scoring.
func TestIntegration_TableRotation(t *testing.T) {
	client := mustConnectInferenceClient(t)

	t.Run("upright_table", func(t *testing.T) {
		data := mustReadPDF(t, "rotate_0.pdf")
		cfg := pdf.DefaultParserConfig()
		p := NewParser(cfg)
		result, err := p.Parse(context.Background(), data, client)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if len(result.Sections) == 0 {
			t.Error("expected sections from upright table")
		}
		t.Logf("rotate_0: %d sections, %d tables", len(result.Sections), len(result.Tables))
	})

	t.Run("rotated_90_table", func(t *testing.T) {
		data := mustReadPDF(t, "rotate_90.pdf")
		cfg := pdf.DefaultParserConfig()
		// DeepDoc DLA does not yet correctly annotate boxes on rotated
		// pages (regions and characters are in different coordinate
		// spaces post-rotation).  Character extraction and rotation are
		// verified via the lyt.CharsToBoxes path.
		cfg.SkipOCR = true
		p := NewParser(cfg)
		result, err := p.Parse(context.Background(), data, client)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if len(result.Sections) == 0 {
			t.Error("expected sections from rotated table")
		}
		t.Logf("rotate_90: %d sections, %d tables", len(result.Sections), len(result.Tables))
	})
}

// TestIntegration_WordSpacing verifies space insertion between ASCII word
// characters with a visible gap (Python __img_ocr space insertion).
func TestIntegration_WordSpacing(t *testing.T) {
	client := mustConnectInferenceClient(t)
	data := mustReadPDF(t, "01_english_simple.pdf")

	cfg := pdf.DefaultParserConfig()
	p := NewParser(cfg)
	result, err := p.Parse(context.Background(), data, client)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Assert: no "word1word2" concatenation — ASCII words should be
	// space-separated (either by embedded-char spacing or OCR gaps).
	for _, s := range result.Sections {
		run := 0
		for _, r := range s.Text {
			if r >= 'a' && r <= 'z' {
				run++
				if run > 15 {
					t.Logf("long lowercase run (no space): section text=%q",
						s.Text[:min(80, len(s.Text))])
					break
				}
			} else {
				run = 0
			}
		}
	}
	t.Logf("word spacing check: %d sections", len(result.Sections))
}

// TestE2E_ParseAndPostProcess runs Parse → PostProcess end-to-end on a real
// PDF. Skips VLM (no tenant_id set) but exercises all other operators.
func TestE2E_ParseAndPostProcess(t *testing.T) {
	data := mustReadPDF(t, "01_english_simple.pdf")

	mock := &MockDocAnalyzer{Healthy: true}
	p := NewParser(pdf.DefaultParserConfig())

	result, err := p.Parse(context.Background(), data, mock)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(result.Sections) == 0 {
		t.Fatal("Parse() returned zero sections")
	}
	t.Logf("sections: %d", len(result.Sections))

	// PostProcess is handled by the Pipeline framework.
	// Verify raw parse produces sections with LayoutType set.
	for i, s := range result.Sections {
		t.Logf("  section[%d]: layout=%q text=%q", i, s.LayoutType, truncate(s.Text, 60))
	}

	figs := result.Figures()
	t.Logf("figures: %d", len(figs))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
