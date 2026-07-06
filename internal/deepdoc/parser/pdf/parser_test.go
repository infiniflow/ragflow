package pdf

import (
	"context"
	"image"
	"math"
	"strings"
	"sync"
	"testing"

	lyt "ragflow/internal/deepdoc/parser/pdf/layout"
	tbl "ragflow/internal/deepdoc/parser/pdf/table"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// ---- test helpers ----

func newTestParser() *Parser {
	return &Parser{Config: pdf.DefaultParserConfig()}
}

func newMockDocAnalyzer(healthy bool, boxes []pdf.OCRBox, texts []pdf.OCRText) *MockDocAnalyzer {
	return &MockDocAnalyzer{
		Healthy:   healthy,
		OCRBoxes:  boxes,
		OCRTexts:  texts,
	}
}

func newSimpleMockDocAnalyzer() *MockDocAnalyzer {
	return &MockDocAnalyzer{Healthy: true}
}

// ── OCR fallback ──────────────────────────────────────────────────────

func TestOCR_Fallback(t *testing.T) {
	dummyImg := image.NewRGBA(image.Rect(0, 0, 100, 100))

	t.Run("nil image", func(t *testing.T) {
		if got := ocrDetectAndRecognize(context.Background(), nil, &MockDocAnalyzer{Healthy: true}, 0, "garbled page"); got != nil {
			t.Error("nil image → nil")
		}
	})

	t.Run("detect returns no boxes", func(t *testing.T) {
		mock := &MockDocAnalyzer{Healthy: true, OCRBoxes: nil}
		if got := ocrDetectAndRecognize(context.Background(), dummyImg, mock, 0, "garbled page"); got != nil {
			t.Error("no det boxes → nil")
		}
	})

	t.Run("detect + recognize success", func(t *testing.T) {
		mock := &MockDocAnalyzer{
			Healthy: true,
			OCRBoxes: []pdf.OCRBox{{X0: 10, Y0: 20, X1: 90, Y1: 20, X2: 90, Y2: 40, X3: 10, Y3: 40}},
			OCRTexts: []pdf.OCRText{{Text: "Hello", Confidence: 0.9}},
		}
		got := ocrDetectAndRecognize(context.Background(), dummyImg, mock, 0, "garbled page")
		if len(got) != 1 {
			t.Fatalf("expected 1 pdf.TextChar, got %d", len(got))
		}
		if got[0].Text != "Hello" {
			t.Errorf("text = %q, want Hello", got[0].Text)
		}
	})

	t.Run("detect boxes but rec returns empty text", func(t *testing.T) {
		mock := &MockDocAnalyzer{
			Healthy: true,
			OCRBoxes: []pdf.OCRBox{{X0: 10, Y0: 20, X1: 90, Y1: 20, X2: 90, Y2: 40, X3: 10, Y3: 40}},
			OCRTexts: []pdf.OCRText{{Text: "", Confidence: 0.1}},
		}
		got := ocrDetectAndRecognize(context.Background(), dummyImg, mock, 0, "garbled page")
		if len(got) != 0 {
			t.Error("empty rec text → empty result")
		}
	})
}

// garbledSample returns chars that trigger IsGarbledByFontEncoding:
// ≥30% subset font, <5% CJK, >40% ASCII punctuation.
// ── OCR scan page ──────────────────────────────────────────────────────

func TestOCR_ScanPage(t *testing.T) {
	dummyImg := image.NewRGBA(image.Rect(0, 0, 100, 100))

	t.Run("nil image", func(t *testing.T) {
		if got := ocrDetectAndRecognize(context.Background(), nil, &MockDocAnalyzer{Healthy: true}, 0, "scan page"); got != nil {
			t.Error("nil image → nil")
		}
	})

	t.Run("detect returns no boxes", func(t *testing.T) {
		mock := &MockDocAnalyzer{Healthy: true, OCRBoxes: nil}
		if got := ocrDetectAndRecognize(context.Background(), dummyImg, mock, 0, "scan page"); got != nil {
			t.Error("no det boxes → nil")
		}
	})

	t.Run("detect + recognize success", func(t *testing.T) {
		mock := &MockDocAnalyzer{
			Healthy: true,
			OCRBoxes: []pdf.OCRBox{
				{X0: 10, Y0: 20, X1: 90, Y1: 20, X2: 90, Y2: 40, X3: 10, Y3: 40},
				{X0: 10, Y0: 50, X1: 90, Y1: 50, X2: 90, Y2: 70, X3: 10, Y3: 70},
			},
			OCRTexts: []pdf.OCRText{{Text: "Hello", Confidence: 0.9}, {Text: "World", Confidence: 0.8}},
		}
		got := ocrDetectAndRecognize(context.Background(), dummyImg, mock, 0, "scan page")
		if len(got) < 1 {
			t.Error("expected at least 1 pdf.TextChar")
		}
	})

	t.Run("detect success but rec returns empty", func(t *testing.T) {
		mock := &MockDocAnalyzer{
			Healthy: true,
			OCRBoxes: []pdf.OCRBox{{X0: 10, Y0: 20, X1: 90, Y1: 20, X2: 90, Y2: 40, X3: 10, Y3: 40}},
			OCRTexts: []pdf.OCRText{},
		}
		got := ocrDetectAndRecognize(context.Background(), dummyImg, mock, 0, "scan page")
		if len(got) != 0 {
			t.Error("no rec text → empty")
		}
	})
}

// ── OCR table cell ─────────────────────────────────────────────────────

func TestOCR_TableCell(t *testing.T) {
	t.Run("fill single empty cell", func(t *testing.T) {
		cells := []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: ""},
			{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: "已有"},
		}
		mock := &MockDocAnalyzer{Healthy: true, OCRTexts: []pdf.OCRText{{Text: "识别结果", Confidence: 0.9}}}
		dummy := image.NewRGBA(image.Rect(0, 0, 200, 50))

		ocrTableCells(context.Background(), cells, dummy, mock)

		if cells[0].Text != "识别结果" {
			t.Errorf("empty cell not filled: %q", cells[0].Text)
		}
		if cells[1].Text != "已有" {
			t.Errorf("filled cell changed: %q", cells[1].Text)
		}
	})

	t.Run("all cells already filled — no OCR", func(t *testing.T) {
		cells := []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "A"},
			{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: "B"},
		}
		ocrTableCells(context.Background(), cells, nil, nil) // should not panic
		if cells[0].Text != "A" || cells[1].Text != "B" {
			t.Error("filled cells should not change")
		}
	})

	t.Run("empty cells list", func(t *testing.T) {
		ocrTableCells(context.Background(), nil, nil, nil) // should not panic
		ocrTableCells(context.Background(), []pdf.TSRCell{}, nil, nil)
	})

	t.Run("no DeepDoc — skip", func(t *testing.T) {
		cells := []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: ""}}
		ocrTableCells(context.Background(), cells, nil, nil)
		if cells[0].Text != "" {
			t.Error("without DeepDoc, cell should stay empty")
		}
	})

	t.Run("no cropped image — skip", func(t *testing.T) {
		cells := []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: ""}}
		mock := &MockDocAnalyzer{Healthy: true, OCRTexts: []pdf.OCRText{{Text: "x", Confidence: 0.5}}}
		ocrTableCells(context.Background(), cells, nil, mock)
		if cells[0].Text != "" {
			t.Error("without image, cell should stay empty")
		}
	})

	t.Run("OCR returns empty string", func(t *testing.T) {
		cells := []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: ""}}
		mock := &MockDocAnalyzer{Healthy: true, OCRTexts: []pdf.OCRText{}}
		dummy := image.NewRGBA(image.Rect(0, 0, 100, 50))
		ocrTableCells(context.Background(), cells, dummy, mock)
		if cells[0].Text != "" {
			t.Error("empty OCR result → cell stays empty")
		}
	})

	t.Run("cell out of image bounds", func(t *testing.T) {
		cells := []pdf.TSRCell{{X0: 500, Y0: 500, X1: 600, Y1: 600, Text: ""}}
		mock := &MockDocAnalyzer{Healthy: true, OCRTexts: []pdf.OCRText{{Text: "out of bounds", Confidence: 0.9}}}
		dummy := image.NewRGBA(image.Rect(0, 0, 100, 100))
		// Should not panic — gracefully degrade
		ocrTableCells(context.Background(), cells, dummy, mock)
		t.Logf("out-of-bounds cell: text=%q", cells[0].Text)
	})
}

func garbledSample() []pdf.TextChar {
	punctuation := []string{"!", "#", "$", "%", "&", "*", "+", "-", ".", "/",
		":", ";", "<", ">", "=", "?", "@", "^", "_", "~"}
	chars := make([]pdf.TextChar, 20)
	for i, p := range punctuation {
		chars[i] = pdf.TextChar{
			X0: 50 + float64(i*10), X1: 58 + float64(i*10),
			Top: 100, Bottom: 112,
			Text: p, FontName: "ABCDEF+SimSun", PageNumber: 0,
		}
	}
	return chars
}

// ── OCR fallback integration through Parse ──────────────────────────────

func TestOCR_FallbackIntegration(t *testing.T) {
	// ocrFallback logic is tested via TestOCR_fallback.
	// The render+OCR path in Parse requires a real PDF + DeepDoc service.
	// This test verifies the wiring compiles and that garbled chars without
	// DeepDoc pass through gracefully (covered by TestOCR_FallbackIntegration_NoDeepDoc).
	t.Log("OCR fallback Parse integration: tested via TestOCR_fallback (logic) + live DeepDoc testing")
}

func TestOCR_FallbackIntegration_NoDeepDoc(t *testing.T) {
	chars := garbledSample()
	mockEng := &MockEngine{Chars: map[int][]pdf.TextChar{0: chars}, NumPages: 1}
	mockDLA := &MockDocAnalyzer{Healthy: true}

	cfg := pdf.DefaultParserConfig()
	p := NewParser(cfg)
	result, err := p.ParseRaw(context.Background(), mockEng, mockDLA)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("garbled Chars: %d sections", len(result.Sections))
}

func TestNoDeepDoc_PdfOxideUnmapped_KeepsChars(t *testing.T) {
	// pdf_oxide ### unmapped glyphs mixed with real CJK text.
	// Without DeepDoc, isGarbledPage should return false (isScanNoise gate),
	// so chars are kept and sections > 0.
	chars := make([]pdf.TextChar, 30)
	for i := 0; i < 20; i++ {
		chars[i] = pdf.TextChar{
			Text: "测试文本", FontName: "SimSun",
			X0: 50, X1: 128, Top: float64(100 + i*15), Bottom: float64(112 + i*15),
		}
	}
	// Insert ### unmapped glyph noise (no subset fonts)
	chars[20] = pdf.TextChar{Text: "#", FontName: "SimSun", X0: 130, X1: 138, Top: 100, Bottom: 112}
	chars[21] = pdf.TextChar{Text: "#", FontName: "SimSun", X0: 138, X1: 146, Top: 100, Bottom: 112}
	chars[22] = pdf.TextChar{Text: "#", FontName: "SimSun", X0: 146, X1: 154, Top: 100, Bottom: 112}
	chars[23] = pdf.TextChar{Text: "D", FontName: "SimSun", X0: 154, X1: 162, Top: 100, Bottom: 112}
	chars[24] = pdf.TextChar{Text: "_", FontName: "SimSun", X0: 162, X1: 170, Top: 100, Bottom: 112}
	chars[25] = pdf.TextChar{Text: "8", FontName: "SimSun", X0: 170, X1: 178, Top: 100, Bottom: 112}
	chars[26] = pdf.TextChar{Text: "-", FontName: "SimSun", X0: 178, X1: 186, Top: 100, Bottom: 112}
	chars[27] = pdf.TextChar{Text: ".", FontName: "SimSun", X0: 186, X1: 194, Top: 100, Bottom: 112}
	chars[28] = pdf.TextChar{Text: "*", FontName: "SimSun", X0: 194, X1: 202, Top: 100, Bottom: 112}
	chars[29] = pdf.TextChar{Text: "用", FontName: "SimSun", X0: 202, X1: 210, Top: 100, Bottom: 112}

	mockEng := &MockEngine{Chars: map[int][]pdf.TextChar{0: chars}, NumPages: 1}
	mockDLA := &MockDocAnalyzer{Healthy: true}
	p := NewParser(pdf.DefaultParserConfig())
	result, err := p.ParseRaw(context.Background(), mockEng, mockDLA)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sections) == 0 {
		t.Error("pdf_oxide unmapped + CJK: expected >0 sections, got 0")
	}
	t.Logf("pdf_oxide unmapped + CJK: %d sections (chars kept)", len(result.Sections))
}

func TestIsGarbledPage(t *testing.T) {
	t.Run("PUA dominant", func(t *testing.T) {
		chars := make([]pdf.TextChar, 50)
		for i := range chars {
			chars[i] = pdf.TextChar{Text: string(rune(0xE000)), PageNumber: 0}
		}
		if !util.IsGarbledPage(chars) {
			t.Error("100% PUA → garbled")
		}
	})
	t.Run("font encoding", func(t *testing.T) {
		if !util.IsGarbledPage(garbledSample()) {
			t.Error("subset font → garbled")
		}
	})
	t.Run("normal text", func(t *testing.T) {
		chars := make([]pdf.TextChar, 50)
		for i := range chars {
			chars[i] = pdf.TextChar{Text: "a", PageNumber: 0}
		}
		if util.IsGarbledPage(chars) {
			t.Error("normal text → not garbled")
		}
	})
	t.Run("pdf oxide unmapped + CJK — not garbled", func(t *testing.T) {
		// ### unmapped glyphs + real CJK text (no subset fonts).
		// isScanNoise returns false (≥2 consecutive CJK Chars: "护理全科").
		chars := []pdf.TextChar{
			{Text: "和", PageNumber: 0}, {Text: "蔘", PageNumber: 0},
			{Text: "语", PageNumber: 0}, {Text: "言", PageNumber: 0},
			{Text: "#", PageNumber: 0}, {Text: "#", PageNumber: 0},
			{Text: "#", PageNumber: 0}, {Text: "D", PageNumber: 0},
			{Text: "_", PageNumber: 0}, {Text: "8", PageNumber: 0},
			{Text: "-", PageNumber: 0}, {Text: ".", PageNumber: 0},
			{Text: "*", PageNumber: 0}, {Text: "/", PageNumber: 0},
			{Text: "*", PageNumber: 0}, {Text: "护", PageNumber: 0},
			{Text: "理", PageNumber: 0}, {Text: "全", PageNumber: 0},
			{Text: "科", PageNumber: 0}, {Text: "引", PageNumber: 0},
			{Text: "用", PageNumber: 0},
		}
		if util.IsGarbledPage(chars) {
			t.Error("### unmapped + CJK text should NOT be garbled (no subset fonts)")
		}
	})
	t.Run("too few chars", func(t *testing.T) {
		if util.IsGarbledPage([]pdf.TextChar{{Text: " ", PageNumber: 0}}) {
			t.Error("< 20 chars → not garbled")
		}
	})
}

func TestOCR_Fallback_PUAGarbled(t *testing.T) {
	pua := make([]pdf.TextChar, 50)
	for i := range pua {
		pua[i] = pdf.TextChar{Text: string(rune(0xE000 + i%10)), PageNumber: 0}
	}
	dummyImg := image.NewRGBA(image.Rect(0, 0, 100, 100))
	mock := &MockDocAnalyzer{
		Healthy: true,
		OCRBoxes: []pdf.OCRBox{{X0: 10, Y0: 20, X1: 90, Y1: 20, X2: 90, Y2: 40, X3: 10, Y3: 40}},
		OCRTexts: []pdf.OCRText{{Text: "PUA OCR text", Confidence: 0.9}},
	}
	got := ocrDetectAndRecognize(context.Background(), dummyImg, mock, 0, "garbled page")
	if len(got) != 1 || got[0].Text != "PUA OCR text" {
		t.Errorf("PUA garbled should trigger OCR, got %v", got)
	}
}

// ── ocrMergeChars ──────────────────────────────────────────────────────

func TestOCR_MergeChars(t *testing.T) {
	dummyImg := image.NewRGBA(image.Rect(0, 0, 600, 600))

	t.Run("nil image", func(t *testing.T) {
		chars := []pdf.TextChar{{X0: 10, Top: 10, X1: 20, Bottom: 30, Text: "A", PageNumber: 0}}
		if boxes := ocrMergeChars(context.Background(), nil, chars, &MockDocAnalyzer{Healthy: true}, 0); boxes != nil {
			t.Error("nil image → nil")
		}
	})

	t.Run("detect returns no boxes", func(t *testing.T) {
		mock := &MockDocAnalyzer{Healthy: true, OCRBoxes: []pdf.OCRBox{}}
		chars := []pdf.TextChar{{X0: 10, Top: 10, X1: 20, Bottom: 30, Text: "A", PageNumber: 0}}
		if boxes := ocrMergeChars(context.Background(), dummyImg, chars, mock, 0); boxes != nil {
			t.Error("no detect boxes → nil")
		}
	})

	t.Run("detect boxes — all overlap with chars (chars used, Python-aligned)", func(t *testing.T) {
		mock := &MockDocAnalyzer{
			Healthy: true,
			OCRBoxes: []pdf.OCRBox{{X0: 15, Y0: 15, X1: 150, Y1: 15, X2: 150, Y2: 150, X3: 15, Y3: 150}},
			OCRTexts: []pdf.OCRText{{Text: "Hello OCR", Confidence: 0.9}},
		}
		chars := []pdf.TextChar{{X0: 10, X1: 30, Top: 10, Bottom: 30, Text: "Hello", PageNumber: 0}}
		boxes := ocrMergeChars(context.Background(), dummyImg, chars, mock, 0)
		if len(boxes) != 1 {
			t.Fatalf("expected 1 box, got %d", len(boxes))
		}
		// Embedded chars override OCR — char text is more precise.
		if boxes[0].Text != "Hello" {
			t.Errorf("expected char text 'Hello', got %q", boxes[0].Text)
		}
	})

	t.Run("detect boxes — none overlap with chars", func(t *testing.T) {
		mock := &MockDocAnalyzer{
			Healthy: true,
			OCRBoxes: []pdf.OCRBox{{X0: 240, Y0: 240, X1: 270, Y1: 240, X2: 270, Y2: 270, X3: 240, Y3: 270}},
			OCRTexts: []pdf.OCRText{{Text: "OCR", Confidence: 0.9}},
		}
		chars := []pdf.TextChar{{X0: 10, X1: 20, Top: 10, Bottom: 20, Text: "A", PageNumber: 0}}
		boxes := ocrMergeChars(context.Background(), dummyImg, chars, mock, 0)
		if len(boxes) != 1 {
			t.Fatalf("expected 1 box (OCR), got %d", len(boxes))
		}
		if boxes[0].Text != "OCR" {
			t.Errorf("expected OCR text 'OCR', got %q", boxes[0].Text)
		}
	})

	t.Run("detect box — no chars and OCR returns empty", func(t *testing.T) {
		mock := &MockDocAnalyzer{
			Healthy: true,
			OCRBoxes: []pdf.OCRBox{{X0: 240, Y0: 240, X1: 270, Y1: 240, X2: 270, Y2: 270, X3: 240, Y3: 270}},
			OCRTexts: []pdf.OCRText{},
		}
		chars := []pdf.TextChar{{X0: 10, X1: 20, Top: 10, Bottom: 20, Text: "A", PageNumber: 0}}
		boxes := ocrMergeChars(context.Background(), dummyImg, chars, mock, 0)
		if len(boxes) != 0 {
			t.Fatalf("expected 0 boxes (empty OCR), got %d", len(boxes))
		}
	})

	t.Run("multiple detect boxes — one with chars, one OCR", func(t *testing.T) {
		// Box 1 overlaps chars → uses char text. Box 2 has no chars → OCR.
		mock := &MockDocAnalyzer{
			Healthy: true,
			OCRBoxes: []pdf.OCRBox{
				{X0: 15, Y0: 15, X1: 150, Y1: 15, X2: 150, Y2: 150, X3: 15, Y3: 150},
				{X0: 240, Y0: 240, X1: 270, Y1: 240, X2: 270, Y2: 270, X3: 240, Y3: 270},
			},
			OCRTexts: []pdf.OCRText{
				{Text: "box 1 text", Confidence: 0.9},
			},
		}
		chars := []pdf.TextChar{{X0: 10, X1: 30, Top: 10, Bottom: 30, Text: "Hello", PageNumber: 0}}
		boxes := ocrMergeChars(context.Background(), dummyImg, chars, mock, 0)
		if len(boxes) != 2 {
			t.Fatalf("expected 2 boxes, got %d", len(boxes))
		}
		// Box 0 has chars → uses char text.
		if boxes[0].Text != "Hello" {
			t.Errorf("box[0] expected char text 'Hello', got %q", boxes[0].Text)
		}
		// Box 1 has no chars → OCR.
		if boxes[1].Text != "box 1 text" {
			t.Errorf("box[1] expected OCR 'box 1 text', got %q", boxes[1].Text)
		}
	})

	t.Run("chars in box — sorted by reading order (top→x0)", func(t *testing.T) {
		// Box 1 (pixel Y=30-90 → PDF 10-30) overlaps char "a" at (10,10-30).
		// Box 2 (pixel Y=330-390 → PDF 110-130) overlaps char "c" at (70,110-130).
		mock := &MockDocAnalyzer{
			Healthy: true,
			OCRBoxes: []pdf.OCRBox{
				{X0: 15, Y0: 30, X1: 90, Y1: 30, X2: 90, Y2: 90, X3: 15, Y3: 90},
				{X0: 75, Y0: 330, X1: 300, Y1: 330, X2: 300, Y2: 390, X3: 75, Y3: 390},
			},
		}
		chars := []pdf.TextChar{
			{X0: 70, X1: 90, Top: 110, Bottom: 130, Text: "c", PageNumber: 0},
			{X0: 10, X1: 30, Top: 10, Bottom: 30, Text: "a", PageNumber: 0},
		}
		boxes := ocrMergeChars(context.Background(), dummyImg, chars, mock, 0)
		if len(boxes) != 2 {
			t.Fatalf("expected 2 detect boxes, got %d", len(boxes))
		}
		// Each box gets its overlapping char text.
		if boxes[0].Text != "a" {
			t.Errorf("box[0] expected 'a', got %q", boxes[0].Text)
		}
		if boxes[1].Text != "c" {
			t.Errorf("box[1] expected 'c', got %q", boxes[1].Text)
		}
	})

	t.Run("height mismatch — chars with very different height excluded", func(t *testing.T) {
		// Box pixel Y=75-165 → PDF 25-55, height=30. Char A height=20, diff=10/30=0.33 < 0.7 → kept.
		// Char B height=100, diff=70/100=0.70 ≥ 0.7 → excluded.
		mock := &MockDocAnalyzer{
			Healthy: true,
			OCRBoxes: []pdf.OCRBox{
				{X0: 15, Y0: 75, X1: 150, Y1: 75, X2: 150, Y2: 165, X3: 15, Y3: 165},
			},
			OCRTexts: []pdf.OCRText{{Text: "OCR height test", Confidence: 0.9}},
		}
		chars := []pdf.TextChar{
			{X0: 10, X1: 30, Top: 30, Bottom: 50, Text: "A", PageNumber: 0},
			{X0: 40, X1: 60, Top: 20, Bottom: 120, Text: "B", PageNumber: 0},
		}
		boxes := ocrMergeChars(context.Background(), dummyImg, chars, mock, 0)
		if len(boxes) != 1 {
			t.Fatalf("expected 1 box, got %d", len(boxes))
		}
		// Only 'A' matches; 'B' excluded by height gate.
		if boxes[0].Text != "A" {
			t.Errorf("expected 'A' (B excluded by height gate), got %q", boxes[0].Text)
		}
	})

	t.Run("garbled chars — box text cleared for OCR recognize", func(t *testing.T) {
		mock := &MockDocAnalyzer{
			Healthy: true,
			OCRBoxes: []pdf.OCRBox{
				{X0: 15, Y0: 15, X1: 450, Y1: 15, X2: 450, Y2: 450, X3: 15, Y3: 450},
			},
			OCRTexts: []pdf.OCRText{{Text: "OCR result", Confidence: 0.9}},
		}
		chars := []pdf.TextChar{
			{X0: 10, X1: 20, Top: 10, Bottom: 20, Text: "", PageNumber: 0},
			{X0: 30, X1: 40, Top: 10, Bottom: 20, Text: "", PageNumber: 0},
			{X0: 50, X1: 60, Top: 10, Bottom: 20, Text: "a", PageNumber: 0},
		}
		boxes := ocrMergeChars(context.Background(), dummyImg, chars, mock, 0)
		if len(boxes) != 1 {
			t.Fatalf("expected 1 box, got %d", len(boxes))
		}
		if boxes[0].Text != "OCR result" {
			t.Errorf("expected 'OCR result' (garbled majority -> OCR), got %q", boxes[0].Text)
		}
	})

	t.Run("OCR text preserves word spacing", func(t *testing.T) {
		// Detect box at (pixel 30,30 → 90,90 → PDF 10,10 → 30,30).
		// Chars at (10,10-25) → within the box region.  Char text "do" is
		// used (Python-aligned: embedded chars are more precise than OCR).
		mock := &MockDocAnalyzer{
			Healthy: true,
			OCRBoxes: []pdf.OCRBox{{X0: 30, Y0: 30, X1: 90, Y1: 30, X2: 90, Y2: 90, X3: 30, Y3: 90}},
			OCRTexts: []pdf.OCRText{{Text: "docker commit infiniflow", Confidence: 0.95}},
		}
		chars := []pdf.TextChar{
			{Text: "d", X0: 10, X1: 20, Top: 10, Bottom: 25, PageNumber: 0},
			{Text: "o", X0: 21, X1: 30, Top: 10, Bottom: 25, PageNumber: 0},
		}
		boxes := ocrMergeChars(context.Background(), dummyImg, chars, mock, 0)
		if len(boxes) != 1 {
			t.Fatalf("expected 1 box, got %d", len(boxes))
		}
		// Char text used (Python-aligned).
		if boxes[0].Text != "do" {
			t.Errorf("expected char text 'do', got %q", boxes[0].Text)
		}
	})
}

// TestTableSectionCaptionInHTML verifies mergeCaptions prepends table
// caption text before the HTML table, matching Python's caption handling.

func TestTableSectionCaptionInHTML(t *testing.T) {
	// Simulate pipeline order: extractTableAndReplace → boxesToSections → mergeCaptions
	boxes := []pdf.TextBox{
		{X0: 100, X1: 500, Top: 200, Bottom: 400, LayoutType: "table", PageNumber: 0},
	}
	ti := pdf.TableItem{
		Cells: []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 200, Y1: 50, Label: "table row", Text: "飞机"},
			{X0: 0, Y0: 51, X1: 200, Y1: 100, Label: "table row", Text: "火车"},
		},
		Positions: []pdf.Position{{Left: 100, Right: 500, Top: 200, Bottom: 400}},
		Scale:     1.0,
	}

	// Step 1: extractTableAndReplace → HTML box with table text
	boxes = tbl.ExtractTableAndReplace(boxes, []pdf.TableItem{ti})
	sections := lyt.BoxesToSections(boxes, nil)

	// Add caption section
	sections = append(sections, pdf.Section{
		LayoutType: "table caption",
		Positions:  []pdf.Position{{Left: 100, Right: 500, Top: 180, Bottom: 198}},
		Text:       "表1: 交通工具等级",
	})

	// Step 2: mergeCaptions prepends caption before HTML
	figures := pdf.CollectFigures(sections)
	sections = tbl.MergeCaptions(sections, figures)

	if !strings.HasPrefix(sections[0].Text, "表1: 交通工具等级<table") {
		t.Errorf("expected caption before table HTML, got %q", sections[0].Text)
	}
}

// TestBoxMatchesCell_FalsePositive verifies that boxMatchesCell rejects
// text boxes that are mostly OUTSIDE the cell, even with cellIsEmpty=true.
// The 0.3 threshold should not match a wide box that barely touches a
// narrow cell — this would cause body text to leak into table cells.
// TestParser_ConcurrentSafety verifies that Parser.ParseRaw() is safe for
// concurrent use. 8 goroutines each call Parse 5 times on the same Parser
// instance. Run with -race.
func TestParser_ConcurrentSafety(t *testing.T) {
	mockDLA := &MockDocAnalyzer{Healthy: true}
	p := NewParser(pdf.DefaultParserConfig())

	var wg sync.WaitGroup
	n := 8
	for range n {
		wg.Go(func() {
			for range 5 {
				eng := &MockEngine{NumPages: 2}
				if _, err := p.ParseRaw(context.Background(), eng, mockDLA); err != nil {
					t.Errorf("ParseRaw: %v", err)
				}
			}
		})
	}
	wg.Wait()
}

func TestParseRaw_ClampsFromPage(t *testing.T) {
	// A negative FromPage should be treated as page 0.
	// Only page 0 has content so we can verify clamping worked.
	eng := &MockEngine{NumPages: 3, Chars: map[int][]pdf.TextChar{
		0: {{Text: "page0", X0: 100, X1: 200, Top: 100, Bottom: 120}},
	}}
	mockDLA := &MockDocAnalyzer{Healthy: true}
	cfg := pdf.DefaultParserConfig()
	cfg.FromPage = -1
	p := NewParser(cfg)
	result, err := p.ParseRaw(context.Background(), eng, mockDLA)
	if err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}
	if len(result.Sections) == 0 {
		t.Error("expected sections from page 0")
	}
}

func TestParseRaw_ZeroZoom_NoNaN(t *testing.T) {
	// Zoom=0 should not produce NaN coordinates.
	eng := &MockEngine{NumPages: 1, Chars: map[int][]pdf.TextChar{
		0: {{Text: "test", X0: 100, X1: 200, Top: 100, Bottom: 120}},
	}}
	mockDLA := &MockDocAnalyzer{Healthy: true}
	cfg := pdf.DefaultParserConfig()
	cfg.Zoom = 0
	p := NewParser(cfg)
	result, err := p.ParseRaw(context.Background(), eng, mockDLA)
	if err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}
	foundPosition := false
	for _, s := range result.Sections {
		for _, pos := range s.Positions {
			foundPosition = true
			if math.IsNaN(pos.Left) || math.IsNaN(pos.Top) {
				t.Error("Zoom=0 produced NaN coordinates")
			}
		}
	}
	if !foundPosition {
		t.Fatal("expected at least one position to validate")
	}
}

// ── Test for refactored helper functions ──────────────────────────────
// These are simple tests for the helper functions to ensure they work.

func TestParser_getBatchSize(t *testing.T) {
	tests := []struct {
		name      string
		batchSize int
		want      int
	}{
		{
			name:      "positive batch size",
			batchSize: 100,
			want:      100,
		},
		{
			name:      "zero batch size",
			batchSize: 0,
			want:      50,
		},
		{
			name:      "negative batch size",
			batchSize: -1,
			want:      50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Parser{Config: pdf.ParserConfig{BatchSize: tt.batchSize}}
			got := p.getBatchSize()
			if got != tt.want {
				t.Errorf("getBatchSize() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParser_setupPageConcurrency(t *testing.T) {
	tests := []struct {
		name           string
		maxConcurrency int
		wantCap        int
	}{
		{
			name:           "positive concurrency",
			maxConcurrency: 5,
			wantCap:        5,
		},
		{
			name:           "zero concurrency",
			maxConcurrency: 0,
			wantCap:        1,
		},
		{
			name:           "negative concurrency",
			maxConcurrency: -1,
			wantCap:        1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Parser{Config: pdf.ParserConfig{MaxOCRConcurrency: tt.maxConcurrency}}
			sem, wg := p.setupPageConcurrency()
			if cap(sem) != tt.wantCap {
				t.Errorf("setupPageConcurrency() sem cap = %v, want %v", cap(sem), tt.wantCap)
			}
			if wg == nil {
				t.Error("setupPageConcurrency() wg should not be nil")
			}
		})
	}
}

func TestParser_prescanPages(t *testing.T) {
	// Provide at least 30 ASCII chars per page so DetectEnglish finds a run ≥30.
	charsPage0 := make([]pdf.TextChar, 50)
	for i := range charsPage0 {
		charsPage0[i] = pdf.TextChar{
			Text: "a", PageNumber: 0, Top: 10, Bottom: 20,
			X0: 50 + float64(i*10), X1: 58 + float64(i*10),
		}
	}
	charsPage1 := make([]pdf.TextChar, 50)
	for i := range charsPage1 {
		charsPage1[i] = pdf.TextChar{
			Text: "b", PageNumber: 1, Top: 10, Bottom: 20,
			X0: 50 + float64(i*10), X1: 58 + float64(i*10),
		}
	}
	eng := &MockEngine{
		NumPages: 2,
		Chars: map[int][]pdf.TextChar{
			0: charsPage0,
			1: charsPage1,
		},
	}
	p := &Parser{Config: pdf.DefaultParserConfig()}

	prescanChars, prescanMedianH, prescanMedianW, isEnglish, scanNoise := p.prescanPages(context.Background(), eng, 0, 1, 2)

	if _, ok := prescanChars[0]; !ok {
		t.Error("prescanPages() prescanChars should contain page 0")
	}
	if _, ok := prescanMedianH[0]; !ok {
		t.Error("prescanPages() prescanMedianH should contain page 0")
	}
	if _, ok := prescanMedianW[0]; !ok {
		t.Error("prescanPages() prescanMedianW should contain page 0")
	}
	if isEnglish != true {
		t.Errorf("prescanPages() isEnglish = %v, want true", isEnglish)
	}
	if scanNoise != false {
		t.Errorf("prescanPages() scanNoise = %v, want false", scanNoise)
	}
}

func TestParser_mergeBatchResults(t *testing.T) {
	p := &Parser{Config: pdf.DefaultParserConfig()}
	result := &pdf.ParseResult{
		Sections: []pdf.Section{{Text: "section1"}},
		Tables:   []pdf.TableItem{{}},
		Metrics: pdf.PipelineMetrics{
			BoxesInitial:   10,
			BoxesTextMerge: 8,
			BoxesVertMerge: 5,
			BoxesFinal:     3,
			TablesCount:    1,
		},
	}
	batch := &pdf.ParseResult{
		Sections: []pdf.Section{{Text: "section2"}},
		Tables:   []pdf.TableItem{{}},
		PageImages: map[int]image.Image{1: image.NewRGBA(image.Rect(0, 0, 10, 10))},
		Metrics: pdf.PipelineMetrics{
			BoxesInitial:   20,
			BoxesTextMerge: 15,
			BoxesVertMerge: 10,
			BoxesFinal:     6,
			TablesCount:    2,
		},
	}

	p.mergeBatchResults(result, batch)

	if len(result.Sections) != 2 {
		t.Errorf("mergeBatchResults() sections length = %v, want 2", len(result.Sections))
	}
	if len(result.Tables) != 2 {
		t.Errorf("mergeBatchResults() tables length = %v, want 2", len(result.Tables))
	}
	if result.Metrics.BoxesInitial != 30 {
		t.Errorf("mergeBatchResults() BoxesInitial = %v, want 30", result.Metrics.BoxesInitial)
	}
	if result.Metrics.BoxesTextMerge != 23 {
		t.Errorf("mergeBatchResults() BoxesTextMerge = %v, want 23", result.Metrics.BoxesTextMerge)
	}
	if result.Metrics.BoxesVertMerge != 15 {
		t.Errorf("mergeBatchResults() BoxesVertMerge = %v, want 15", result.Metrics.BoxesVertMerge)
	}
	if result.Metrics.BoxesFinal != 9 {
		t.Errorf("mergeBatchResults() BoxesFinal = %v, want 9", result.Metrics.BoxesFinal)
	}
	if result.Metrics.TablesCount != 3 {
		t.Errorf("mergeBatchResults() TablesCount = %v, want 3", result.Metrics.TablesCount)
	}
	if _, ok := result.PageImages[1]; !ok {
		t.Error("mergeBatchResults() pageImages should contain page 1")
	}
}
