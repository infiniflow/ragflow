package pdf

import (
	"context"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
	"testing"
)

// TestOCRMergeChars_FullCoverage: embedded chars fill the detect box.
func TestOCRMergeChars_FullCoverage(t *testing.T) {
	mock := &MockDocAnalyzer{
		Healthy: true,
		OCRBoxes: []pdf.OCRBox{
			{X0: 0, Y0: 0, X1: 90, Y1: 0, X2: 90, Y2: 120, X3: 0, Y3: 120},
		},
		OCRTexts: []pdf.OCRText{
			{Text: "OCR text", Confidence: 0.9},
		},
	}

	// Both chars overlap the box (height diff < 0.7) → char text used.
	chars := []pdf.TextChar{
		{X0: 2, X1: 10, Top: 2, Bottom: 35, Text: "Hello"},
		{X0: 12, X1: 28, Top: 2, Bottom: 35, Text: "World"},
	}

	boxes := ocrMergeChars(context.Background(), testPageImg(), chars, mock, 0)
	if len(boxes) != 1 {
		t.Fatalf("expected 1 box, got %d", len(boxes))
	}
	// Char text is more precise than OCR — used when available.
	if boxes[0].Text != "HelloWorld" {
		t.Errorf("expected char text 'HelloWorld', got %q", boxes[0].Text)
	}
}

// TestOCRMergeChars_PartialCoverage: box A has chars, box B is OCR'd.
func TestOCRMergeChars_PartialCoverage(t *testing.T) {
	mock := &MockDocAnalyzer{
		Healthy: true,
		OCRBoxes: []pdf.OCRBox{
			{X0: 0, Y0: 0, X1: 45, Y1: 0, X2: 45, Y2: 60, X3: 0, Y3: 60},
			{X0: 45, Y0: 0, X1: 90, Y1: 0, X2: 90, Y2: 60, X3: 45, Y3: 60},
		},
		OCRTexts: []pdf.OCRText{
			{Text: "OCR-filled", Confidence: 0.9},
		},
	}

	// Char "A" overlaps box A → char text. Box B has no chars → OCR.
	chars := []pdf.TextChar{
		{X0: 2, X1: 12, Top: 2, Bottom: 15, Text: "A"},
	}

	boxes := ocrMergeChars(context.Background(), testPageImg(), chars, mock, 0)
	if len(boxes) != 2 {
		t.Fatalf("expected 2 boxes, got %d", len(boxes))
	}
	// Box A has chars.
	if boxes[0].Text != "A" {
		t.Errorf("box 0: expected 'A', got %q", boxes[0].Text)
	}
	// Box B has no chars → OCR.
	if boxes[1].Text != "OCR-filled" {
		t.Errorf("box 1: expected 'OCR-filled', got %q", boxes[1].Text)
	}
}

// TestOCRMergeChars_NoDetectBoxes: OCRDetect returns nil/empty → ocrMergeChars returns nil.
func TestOCRMergeChars_NoDetectBoxes(t *testing.T) {
	mock := &MockDocAnalyzer{
		Healthy:  true,
		OCRBoxes: nil,
	}

	chars := []pdf.TextChar{
		{X0: 2, X1: 10, Top: 2, Bottom: 8, Text: "Hello"},
	}

	boxes := ocrMergeChars(context.Background(), testPageImg(), chars, mock, 0)
	if boxes != nil {
		t.Errorf("expected nil for no detect boxes, got %d boxes", len(boxes))
	}

	// Also test empty OCRBoxes
	mock.OCRBoxes = []pdf.OCRBox{}
	boxes = ocrMergeChars(context.Background(), testPageImg(), chars, mock, 0)
	if boxes != nil {
		t.Errorf("expected nil for empty detect boxes, got %d boxes", len(boxes))
	}
}

// TestOCRMergeChars_GarbledChars: chars are majority PUA → text cleared → OCRRecognize triggered.
func TestOCRMergeChars_GarbledChars(t *testing.T) {
	mock := &MockDocAnalyzer{
		Healthy: true,
		OCRBoxes: []pdf.OCRBox{
			{X0: 0, Y0: 0, X1: 90, Y1: 0, X2: 90, Y2: 120, X3: 0, Y3: 120},
		},
		OCRTexts: []pdf.OCRText{
			{Text: "OCR-result", Confidence: 0.95},
		},
	}

	// Char height ~33, box height 40. Diff = 0.175 < 0.7 → not filtered.
	chars := []pdf.TextChar{
		{X0: 2, X1: 10, Top: 2, Bottom: 35, Text: string(rune(0xF0123))},  // PUA
		{X0: 12, X1: 20, Top: 2, Bottom: 35, Text: string(rune(0xF0456))}, // PUA
		{X0: 22, X1: 28, Top: 2, Bottom: 35, Text: "a"},                   // normal
	}

	boxes := ocrMergeChars(context.Background(), testPageImg(), chars, mock, 0)
	if len(boxes) != 1 {
		t.Fatalf("expected 1 box, got %d", len(boxes))
	}
	// Garbled majority → text cleared → OCRRecognize fills
	if boxes[0].Text != "OCR-result" {
		t.Errorf("expected 'OCR-result' from OCRRecognize, got %q", boxes[0].Text)
	}
}

// TestOCRMergeChars_HeightGate: char height differs from box height by >70% → filtered out.
func TestOCRMergeChars_HeightGate(t *testing.T) {
	// Box height in PDF space: 120/3.0 = 40
	mock := &MockDocAnalyzer{
		Healthy: true,
		OCRBoxes: []pdf.OCRBox{
			{X0: 0, Y0: 0, X1: 90, Y1: 0, X2: 90, Y2: 120, X3: 0, Y3: 120},
		},
		OCRTexts: []pdf.OCRText{
			{Text: "height-gated-OCR", Confidence: 0.8},
		},
	}

	// Char height = 1. Box height = 40. Diff = |1-40|/max(1,40) = 39/40 = 0.975 >= 0.7 → filtered.
	chars := []pdf.TextChar{
		{X0: 2, X1: 10, Top: 2, Bottom: 3, Text: "tiny"},
	}

	boxes := ocrMergeChars(context.Background(), testPageImg(), chars, mock, 0)
	if len(boxes) != 1 {
		t.Fatalf("expected 1 box (OCR fallback after height gate), got %d", len(boxes))
	}
	// Height gate filtered the char → box empty → OCRRecognize fills
	if boxes[0].Text != "height-gated-OCR" {
		t.Errorf("expected 'height-gated-OCR', got %q", boxes[0].Text)
	}
}

// TestOCRMergeChars_FontEncodingGarbled verifies Strategy 2 garbled
// detection: subset-font chars clear the box text → OCR fallback.
// Python __ocr: _is_garbled_by_font_encoding(min_chars=5).
func TestOCRMergeChars_FontEncodingGarbled(t *testing.T) {
	mock := &MockDocAnalyzer{
		Healthy: true,
		OCRBoxes: []pdf.OCRBox{
			{X0: 15, Y0: 15, X1: 150, Y1: 15, X2: 150, Y2: 150, X3: 15, Y3: 150},
		},
		OCRTexts: []pdf.OCRText{{Text: "OCR fallback", Confidence: 0.9}},
	}
	// 5+ subset-font chars (font names matching `^[A-Z0-9]{2,6}\+`)
	// trigger font-encoding garbled detection → text cleared → OCR used.
	chars := make([]pdf.TextChar, 5)
	for i := range chars {
		chars[i] = pdf.TextChar{
			X0: 10, X1: 30, Top: float64(10 + i*5), Bottom: float64(25 + i*5),
			Text: "#", FontName: "DY1+SimSun", PageNumber: 0,
		}
	}
	boxes := ocrMergeChars(context.Background(), testPageImg(), chars, mock, 0)
	if len(boxes) != 1 {
		t.Fatalf("expected 1 OCR-fallback box, got %d", len(boxes))
	}
	if boxes[0].Text != "OCR fallback" {
		t.Errorf("font-encoding garbled: expected 'OCR fallback', got %q", boxes[0].Text)
	}
}

// TestSortCharsYFirstly verifies the fuzzy Y-sort used in ocrMergeChars
// matches Python Recognizer.sort_Y_firstly.
func TestSortCharsYFirstly(t *testing.T) {
	t.Run("same line — fuzzy group by X", func(t *testing.T) {
		// Chars on the same line with slightly different Top values.
		// Threshold=10 covers all Top diffs → should sort by X only.
		chars := []pdf.TextChar{
			{X0: 50, Top: 12, Text: "C"},
			{X0: 30, Top: 16, Text: "B"},
			{X0: 10, Top: 10, Text: "A"},
		}
		sortCharsYFirstly(chars, 10)
		if chars[0].Text != "A" || chars[1].Text != "B" || chars[2].Text != "C" {
			t.Errorf("expected A,B,C (X-order), got %v,%v,%v", chars[0].Text, chars[1].Text, chars[2].Text)
		}
	})

	t.Run("different lines — sort by Y", func(t *testing.T) {
		// Chars on clearly different lines → sort by Y only.
		chars := []pdf.TextChar{
			{X0: 50, Top: 100, Text: "C"},
			{X0: 30, Top: 10, Text: "A"},
			{X0: 10, Top: 50, Text: "B"},
		}
		sortCharsYFirstly(chars, 10)
		if chars[0].Text != "A" || chars[1].Text != "B" || chars[2].Text != "C" {
			t.Errorf("expected A,B,C (Y-order), got %v,%v,%v", chars[0].Text, chars[1].Text, chars[2].Text)
		}
	})

	t.Run("mixed — same-line group with different-line", func(t *testing.T) {
		// A and B on line 1 (Top ~10), C on line 2 (Top ~100).
		chars := []pdf.TextChar{
			{X0: 50, Top: 100, Text: "C"},
			{X0: 30, Top: 14, Text: "B"},
			{X0: 10, Top: 10, Text: "A"},
		}
		sortCharsYFirstly(chars, 10)
		// A and B same line → X-order: A(10) before B(30).
		// C on different line → after A and B.
		if chars[0].Text != "A" || chars[1].Text != "B" || chars[2].Text != "C" {
			t.Errorf("expected A,B,C, got %v,%v,%v", chars[0].Text, chars[1].Text, chars[2].Text)
		}
	})
}

// TestOCRMergeChars_MixedFontSizes verifies that ocrMergeChars uses
// fuzzy Y-sort — chars on the same line with different font sizes
// (different Top values) are sorted by X, not by strict Top.
func TestOCRMergeChars_MixedFontSizes(t *testing.T) {
	// Simulate mixed font sizes on the same line.
	// "小" has higher Top (smaller font sits higher on the baseline)
	// but is physically to the left of "大" and "号".
	// Strict Top-sort would put "小" first ("小" Top=10 > "大" Top=5).
	// Fuzzy Y-sort groups them as same-line → X-order: "小大号" (correct).
	//
	// Box height: detect box Y2=120 at scale=3 → PDF-space height=40pt.
	// Chars need height >0.3*boxH to pass height gate.
	mock := &MockDocAnalyzer{
		Healthy: true,
		OCRBoxes: []pdf.OCRBox{
			{X0: 0, Y0: 0, X1: 90, Y1: 0, X2: 90, Y2: 120, X3: 0, Y3: 120},
		},
	}
	chars := []pdf.TextChar{
		{X0: 3, X1: 12, Top: 10, Bottom: 30, Text: "小"}, // smaller font, higher baseline
		{X0: 12, X1: 24, Top: 5, Bottom: 35, Text: "大"}, // larger font, lower baseline
		{X0: 24, X1: 36, Top: 5, Bottom: 35, Text: "号"}, // same size as 大, rightmost
	}
	boxes := ocrMergeChars(context.Background(), testPageImg(), chars, mock, 0)
	if len(boxes) != 1 {
		t.Fatalf("expected 1 box, got %d", len(boxes))
	}
	// X-order: 小(x0=3), 大(x0=15), 号(x0=30).
	if boxes[0].Text != "小大号" {
		t.Errorf("expected '小大号' (X-order with fuzzy Y-group), got %q", boxes[0].Text)
	}
}

// TestOCRMergeChars_BoxOrder verifies detect boxes are sorted top-down
// (matching Python's sort_Y_firstly) before char matching.
func TestOCRMergeChars_BoxOrder(t *testing.T) {
	// 3 detect boxes in reverse Y order. After sorting, output should be top-down.
	mock := &MockDocAnalyzer{
		Healthy: true,
		OCRBoxes: []pdf.OCRBox{
			{X0: 0, Y0: 90, X1: 90, Y1: 90, X2: 90, Y2: 120, X3: 0, Y3: 120}, // bottom
			{X0: 0, Y0: 45, X1: 90, Y1: 45, X2: 90, Y2: 60, X3: 0, Y3: 60},   // middle
			{X0: 0, Y0: 0, X1: 90, Y1: 0, X2: 90, Y2: 30, X3: 0, Y3: 30},     // top
		},
		OCRTexts: []pdf.OCRText{{Text: "OCR", Confidence: 0.9}},
	}
	// Chars in PDF space (72 DPI). Detect boxes are at 216 DPI,
	// scaled down by 3 in ocrMergeChars.
	// Box1 PDF: y0=0,y1=10. Box2 PDF: y0=15,y1=20. Box3 PDF: y0=30,y1=40.
	chars := []pdf.TextChar{
		{X0: 2, X1: 10, Top: 2, Bottom: 7, Text: "A"},   // box 1 (top)
		{X0: 2, X1: 10, Top: 16, Bottom: 19, Text: "B"}, // box 2 (middle)
		{X0: 2, X1: 10, Top: 32, Bottom: 37, Text: "C"}, // box 3 (bottom)
	}
	boxes := ocrMergeChars(context.Background(), testPageImg(), chars, mock, 0)
	if len(boxes) != 3 {
		t.Fatalf("expected 3 boxes, got %d", len(boxes))
	}
	// Sorted top-down: A(top~2), B(top~47), C(top~92).
	if boxes[0].Text != "A" || boxes[1].Text != "B" || boxes[2].Text != "C" {
		t.Errorf("expected top-down A,B,C, got %q,%q,%q",
			boxes[0].Text, boxes[1].Text, boxes[2].Text)
	}
}

// TestOCRMergeChars_OverlappingBoxes verifies char-perspective matching:
// when two detect boxes overlap and a char falls in the overlap zone,
// it is assigned to only ONE box (the best match), not duplicated across both.
// The old box-perspective collectOverlapChars would duplicate the char;
// the new char-perspective code (matching Python's find_overlapped) does not.
func TestOCRMergeChars_OverlappingBoxes(t *testing.T) {
	// Box A: PDF x=0..20, y=0..20. Box B: PDF x=10..30, y=0..20.
	// Overlap zone: x=10..20.
	// Char "Y" at PDF x=2..8 → Box A only.
	// Char "X" at PDF x=12..18 → overlap zone (both boxes).
	// Char "Z" at PDF x=22..28 → Box B only.
	//
	// Old box-perspective: Box A gets [Y,X], Box B gets [X,Z].
	// New char-perspective: Box A gets [Y,X] (best overlap), Box B gets [Z].
	mock := &MockDocAnalyzer{
		Healthy: true,
		OCRBoxes: []pdf.OCRBox{
			{X0: 0, Y0: 0, X1: 60, Y1: 0, X2: 60, Y2: 60, X3: 0, Y3: 60},   // Box A
			{X0: 30, Y0: 0, X1: 90, Y1: 0, X2: 90, Y2: 60, X3: 30, Y3: 60}, // Box B
		},
	}
	chars := []pdf.TextChar{
		{X0: 2, X1: 8, Top: 2, Bottom: 12, Text: "甲"},   // Box A only
		{X0: 12, X1: 18, Top: 2, Bottom: 12, Text: "乙"}, // overlap zone
		{X0: 22, X1: 28, Top: 2, Bottom: 12, Text: "丙"}, // Box B only
	}
	boxes := ocrMergeChars(context.Background(), testPageImg(), chars, mock, 0)
	if len(boxes) != 2 {
		t.Fatalf("expected 2 boxes, got %d", len(boxes))
	}
	// Tie on equal overlap → later box wins (matching Python's >=).
	// "乙" goes to Box B (both overlap=1.0, Box B checked later).
	// Box A → "甲", Box B → "乙丙" (sorted by X).
	if boxes[0].Text != "甲" {
		t.Errorf("box A: expected '甲', got %q", boxes[0].Text)
	}
	if boxes[1].Text != "乙丙" {
		t.Errorf("box B: expected '乙丙', got %q", boxes[1].Text)
	}
}

// ── pdf_oxide ### detection tests ─────────────────────────────────────

func TestPdfOxideUnmappedGarbled_Empty(t *testing.T) {
	if util.PdfOxideUnmappedGarbled("") {
		t.Error("empty text should not be garbled")
	}
}

func TestPdfOxideUnmappedGarbled_NormalText(t *testing.T) {
	if util.PdfOxideUnmappedGarbled("这是一段正常的中文文本没有任何问题") {
		t.Error("normal Chinese text should not be garbled")
	}
}

func TestPdfOxideUnmappedGarbled_SingleHash(t *testing.T) {
	// A single # is not enough (could be a phone number or reference).
	if util.PdfOxideUnmappedGarbled("参考 #123 的文献") {
		t.Error("single # should not be garbled")
	}
}

func TestPdfOxideUnmappedGarbled_TripleHashCluster(t *testing.T) {
	// Two ### sequences => garbled.
	if !util.PdfOxideUnmappedGarbled("我信###D_8-.###$#(") {
		t.Error("two ### clusters should be garbled")
	}
}

func TestPdfOxideUnmappedGarbled_QuadHash(t *testing.T) {
	// One #### counts as one ### cluster. Need two for trigger.
	// But density may also be high enough.
	if !util.PdfOxideUnmappedGarbled("text####abc####def") {
		t.Error("two #### clusters should be garbled")
	}
}

func TestPdfOxideUnmappedGarbled_SingleTriple(t *testing.T) {
	// Single ### cluster => garbled.  In a 200-char sample "###" is impossible
	// in normal text (URLs/markdown use at most "##").
	if !util.PdfOxideUnmappedGarbled("hello###world normal text here") {
		t.Error("single ### cluster should be garbled")
	}
}

func TestPdfOxideUnmappedGarbled_HighDensity(t *testing.T) {
	// 10 # chars mixed among 40+ non-space chars = 25% → garbled.
	text := "#a#b#c#d#e#f#g#h#i#j" + " extra normal chars padding to reach minimum"
	if !util.PdfOxideUnmappedGarbled(text) {
		t.Error("high # density should be garbled")
	}
}

func TestPdfOxideUnmappedGarbled_RealWorldGarbled(t *testing.T) {
	// Simulates the garbled page from 1例3个月...pdf:
	// Chinese text mixed with ###D_ style unmapped glyph patterns.
	garbled := "和蔘语言###D_8-.*/*护理全科##%&$ 80引用\"\"###$#(点向患儿"
	if !util.PdfOxideUnmappedGarbled(garbled) {
		t.Error("real-world garbled text with ### clusters should be detected")
	}
}
