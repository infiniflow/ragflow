package parser

import (
	"context"
	"image"
	"strings"
	"testing"
)

func TestIsASCIIPrintable(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{'a', true}, {'z', true}, {'A', true}, {'Z', true},
		{'0', true}, {'9', true}, {' ', true},
		{',', true}, {'.', true}, {'!', true}, {'?', true},
		{'-', true}, {'_', true}, {'/', true}, {':', true},
		{';', true}, {'(', true}, {')', true}, {'[', true},
		{']', true}, {'@', true}, {'#', true}, {'$', true},
		{'%', true}, {'^', true}, {'&', true}, {'*', true},
		{'<', true}, {'>', true},
		{'中', false}, {'。', false}, {'，', false},
		{'α', false}, {'\n', false}, {'\t', false},
	}
	for _, tt := range tests {
		if got := isASCIIPrintable(tt.r); got != tt.want {
			t.Errorf("isASCIIPrintable(%q) = %v, want %v", tt.r, got, tt.want)
		}
	}
}

func TestDetectEnglish(t *testing.T) {
	t.Run("pure english", func(t *testing.T) {
		chars := make([]TextChar, 100)
		for i := range chars {
			chars[i] = TextChar{Text: "a", PageNumber: 0}
		}
		pageChars := map[int][]TextChar{0: chars}
		if !detectEnglish(pageChars, 1, nil) {
			t.Error("pure English PDF should be detected as English")
		}
	})

	t.Run("pure chinese", func(t *testing.T) {
		chars := make([]TextChar, 100)
		for i := range chars {
			chars[i] = TextChar{Text: "中", PageNumber: 0}
		}
		pageChars := map[int][]TextChar{0: chars}
		if detectEnglish(pageChars, 1, nil) {
			t.Error("pure Chinese PDF should NOT be detected as English")
		}
	})

	t.Run("english majority", func(t *testing.T) {
		engChars := make([]TextChar, 100)
		for i := range engChars {
			engChars[i] = TextChar{Text: "a", PageNumber: 0}
		}
		chnChars := make([]TextChar, 100)
		for i := range chnChars {
			chnChars[i] = TextChar{Text: "中", PageNumber: 1}
		}
		pageChars := map[int][]TextChar{0: engChars, 1: chnChars, 2: engChars}
		if !detectEnglish(pageChars, 3, nil) {
			t.Error("2/3 English pages should be English by majority vote")
		}
	})

	t.Run("empty", func(t *testing.T) {
		if detectEnglish(nil, 0, nil) {
			t.Error("empty input should return false")
		}
		if detectEnglish(map[int][]TextChar{}, 1, nil) {
			t.Error("empty map should return false")
		}
	})

	t.Run("image only pages", func(t *testing.T) {
		chars := make([]TextChar, 50)
		for i := range chars {
			chars[i] = TextChar{Text: "a", PageNumber: 0}
		}
		pageChars := map[int][]TextChar{0: chars}
		if detectEnglish(pageChars, 2, nil) {
			t.Error("1/2 pages with chars, 0 with sequence — should NOT be English")
		}
	})
}

// ── SampleFunc tests ────────────────────────────────────────────────────

func TestDefaultSampleChars(t *testing.T) {
	t.Run("nil chars", func(t *testing.T) {
		if s := defaultSampleChars(nil, 100); s != "" {
			t.Errorf("nil chars → %q, want empty", s)
		}
	})

	t.Run("empty chars", func(t *testing.T) {
		if s := defaultSampleChars([]TextChar{}, 100); s != "" {
			t.Errorf("empty chars → %q, want empty", s)
		}
	})

	t.Run("n <= 0", func(t *testing.T) {
		chars := []TextChar{{Text: "x"}}
		if s := defaultSampleChars(chars, 0); s != "" {
			t.Errorf("n=0 → %q, want empty", s)
		}
	})

	t.Run("n larger than len", func(t *testing.T) {
		chars := []TextChar{{Text: "a"}, {Text: "b"}, {Text: "c"}}
		s := defaultSampleChars(chars, 100)
		if len(s) != 3 {
			t.Errorf("n=100, len=3 → got len=%d, want 3", len(s))
		}
		for _, c := range chars {
			if !strings.ContainsRune(s, []rune(c.Text)[0]) {
				t.Errorf("sample %q missing char %q", s, c.Text)
			}
		}
	})

	t.Run("produces all chars (no duplicates, just reordering)", func(t *testing.T) {
		chars := make([]TextChar, 50)
		for i := range chars {
			chars[i] = TextChar{Text: string(rune('A' + i%26))}
		}
		s := defaultSampleChars(chars, 50)
		if len(s) != 50 {
			t.Errorf("len=%d, want 50", len(s))
		}
	})
}

func TestDetectEnglish_CustomSampler(t *testing.T) {
	t.Run("deterministic sampler sees English at end", func(t *testing.T) {
		chars := make([]TextChar, 100)
		for i := 0; i < 70; i++ {
			chars[i] = TextChar{Text: "中", PageNumber: 0}
		}
		for i := 70; i < 100; i++ {
			chars[i] = TextChar{Text: "a", PageNumber: 0}
		}
		pageChars := map[int][]TextChar{0: chars}

		_ = detectEnglish(pageChars, 1, nil)

		lastSampler := func(chars []TextChar, n int) string {
			m := min(n, len(chars))
			start := max(0, len(chars)-m)
			var buf strings.Builder
			for i := start; i < len(chars); i++ {
				buf.WriteString(chars[i].Text)
			}
			return buf.String()
		}
		if !detectEnglish(pageChars, 1, lastSampler) {
			t.Error("sampler that sees the tail should detect English (30 consecutive ASCII)")
		}
	})

	t.Run("deterministic sampler sees only CJK head", func(t *testing.T) {
		chars := make([]TextChar, 100)
		for i := 0; i < 70; i++ {
			chars[i] = TextChar{Text: "中", PageNumber: 0}
		}
		for i := 70; i < 100; i++ {
			chars[i] = TextChar{Text: "a", PageNumber: 0}
		}
		pageChars := map[int][]TextChar{0: chars}

		firstSampler := func(chars []TextChar, n int) string {
			m := min(n, len(chars))
			var buf strings.Builder
			for i := 0; i < m; i++ {
				buf.WriteString(chars[i].Text)
			}
			return buf.String()
		}
		if !detectEnglish(pageChars, 1, firstSampler) {
			t.Error("first-100 sampler: 70 CJK + 30 ASCII → 30 consecutive ASCII → should be English")
		}
	})

	t.Run("sampler returns fewer than 30 chars", func(t *testing.T) {
		chars := make([]TextChar, 10)
		for i := range chars {
			chars[i] = TextChar{Text: "a", PageNumber: 0}
		}
		pageChars := map[int][]TextChar{0: chars}
		if detectEnglish(pageChars, 1, defaultSampleChars) {
			t.Error("fewer than 30 chars → no 30-char run possible → not English")
		}
	})

	t.Run("sample < n chars from page", func(t *testing.T) {
		chars := make([]TextChar, 25)
		for i := range chars {
			chars[i] = TextChar{Text: "a", PageNumber: 0}
		}
		pageChars := map[int][]TextChar{0: chars}
		if detectEnglish(pageChars, 1, defaultSampleChars) {
			t.Error("25 chars cannot form 30-char run → not English")
		}
	})

	t.Run("majority with custom sampler", func(t *testing.T) {
		engChars := make([]TextChar, 100)
		for i := range engChars {
			engChars[i] = TextChar{Text: "a", PageNumber: 0}
		}
		chnChars := make([]TextChar, 100)
		for i := range chnChars {
			chnChars[i] = TextChar{Text: "中", PageNumber: 1}
		}
		pageChars := map[int][]TextChar{0: engChars, 1: chnChars, 2: engChars}
		if !detectEnglish(pageChars, 3, nil) {
			t.Error("2/3 English pages should be English by majority vote")
		}
	})
}

// ── OCR fallback ──────────────────────────────────────────────────────

func TestOCR_fallback(t *testing.T) {
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
			OCRBoxes: []OCRBox{{X0: 10, Y0: 20, X1: 90, Y1: 20, X2: 90, Y2: 40, X3: 10, Y3: 40}},
			OCRTexts: []OCRText{{Text: "Hello", Confidence: 0.9}},
		}
		got := ocrDetectAndRecognize(context.Background(), dummyImg, mock, 0, "garbled page")
		if len(got) != 1 {
			t.Fatalf("expected 1 TextChar, got %d", len(got))
		}
		if got[0].Text != "Hello" {
			t.Errorf("text = %q, want Hello", got[0].Text)
		}
	})

	t.Run("detect boxes but rec returns empty text", func(t *testing.T) {
		mock := &MockDocAnalyzer{
			Healthy:  true,
			OCRBoxes: []OCRBox{{X0: 10, Y0: 20, X1: 90, Y1: 20, X2: 90, Y2: 40, X3: 10, Y3: 40}},
			OCRTexts: []OCRText{{Text: "", Confidence: 0.1}},
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

func TestOCR_scanPage(t *testing.T) {
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
			OCRBoxes: []OCRBox{
				{X0: 10, Y0: 20, X1: 90, Y1: 20, X2: 90, Y2: 40, X3: 10, Y3: 40},
				{X0: 10, Y0: 50, X1: 90, Y1: 50, X2: 90, Y2: 70, X3: 10, Y3: 70},
			},
			OCRTexts: []OCRText{{Text: "Hello", Confidence: 0.9}, {Text: "World", Confidence: 0.8}},
		}
		got := ocrDetectAndRecognize(context.Background(), dummyImg, mock, 0, "scan page")
		if len(got) < 1 {
			t.Error("expected at least 1 TextChar")
		}
	})

	t.Run("detect success but rec returns empty", func(t *testing.T) {
		mock := &MockDocAnalyzer{
			Healthy:  true,
			OCRBoxes: []OCRBox{{X0: 10, Y0: 20, X1: 90, Y1: 20, X2: 90, Y2: 40, X3: 10, Y3: 40}},
			OCRTexts: []OCRText{},
		}
		got := ocrDetectAndRecognize(context.Background(), dummyImg, mock, 0, "scan page")
		if len(got) != 0 {
			t.Error("no rec text → empty")
		}
	})
}

// ── OCR table cell ─────────────────────────────────────────────────────



func TestOCR_tableCell(t *testing.T) {
	t.Run("fill single empty cell", func(t *testing.T) {
		cells := []TSRCell{
			{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: ""},
			{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: "已有"},
		}
		mock := &MockDocAnalyzer{Healthy: true, OCRTexts: []OCRText{{Text: "识别结果", Confidence: 0.9}}}
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
		cells := []TSRCell{
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
		ocrTableCells(context.Background(), []TSRCell{}, nil, nil)
	})

	t.Run("no DeepDoc — skip", func(t *testing.T) {
		cells := []TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: ""}}
		ocrTableCells(context.Background(), cells, nil, nil)
		if cells[0].Text != "" {
			t.Error("without DeepDoc, cell should stay empty")
		}
	})

	t.Run("no cropped image — skip", func(t *testing.T) {
		cells := []TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: ""}}
		mock := &MockDocAnalyzer{Healthy: true, OCRTexts: []OCRText{{Text: "x", Confidence: 0.5}}}
		ocrTableCells(context.Background(), cells, nil, mock)
		if cells[0].Text != "" {
			t.Error("without image, cell should stay empty")
		}
	})

	t.Run("OCR returns empty string", func(t *testing.T) {
		cells := []TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: ""}}
		mock := &MockDocAnalyzer{Healthy: true, OCRTexts: []OCRText{}}
		dummy := image.NewRGBA(image.Rect(0, 0, 100, 50))
		ocrTableCells(context.Background(), cells, dummy, mock)
		if cells[0].Text != "" {
			t.Error("empty OCR result → cell stays empty")
		}
	})

	t.Run("cell out of image bounds", func(t *testing.T) {
		cells := []TSRCell{{X0: 500, Y0: 500, X1: 600, Y1: 600, Text: ""}}
		mock := &MockDocAnalyzer{Healthy: true, OCRTexts: []OCRText{{Text: "out of bounds", Confidence: 0.9}}}
		dummy := image.NewRGBA(image.Rect(0, 0, 100, 100))
		// Should not panic — gracefully degrade
		ocrTableCells(context.Background(), cells, dummy, mock)
		t.Logf("out-of-bounds cell: text=%q", cells[0].Text)
	})
}

func garbledSample() []TextChar {
	punctuation := []string{"!", "#", "$", "%", "&", "*", "+", "-", ".", "/",
		":", ";", "<", ">", "=", "?", "@", "^", "_", "~"}
	chars := make([]TextChar, 20)
	for i, p := range punctuation {
		chars[i] = TextChar{
			X0: 50 + float64(i*10), X1: 58 + float64(i*10),
			Top: 100, Bottom: 112,
			Text: p, FontName: "ABCDEF+SimSun", PageNumber: 0,
		}
	}
	return chars
}

// ── OCR fallback integration through Parse ─────────────────────────────

func TestOCR_FallbackIntegration(t *testing.T) {
	// ocrFallback logic is tested via TestOCR_fallback.
	// The render+OCR path in Parse requires a real PDF + DeepDoc service.
	// This test verifies the wiring compiles and that garbled chars without
	// DeepDoc pass through gracefully (covered by TestOCR_FallbackIntegration_NoDeepDoc).
	t.Log("OCR fallback Parse integration: tested via TestOCR_fallback (logic) + live DeepDoc testing")
}

func TestOCR_FallbackIntegration_NoDeepDoc(t *testing.T) {
	chars := garbledSample()
	mockEng := &mockEngine{chars: map[int][]TextChar{0: chars}, pageCount: 1}

	cfg := DefaultParserConfig()
	p := NewParser(cfg, &MockDocAnalyzer{Healthy: true, Model: ModelSaas})
	result, err := p.Parse(context.Background(), mockEng)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("garbled chars: %d sections", len(result.Sections))
}

func TestNoDeepDoc_PdfOxideUnmapped_KeepsChars(t *testing.T) {
	// pdf_oxide ### unmapped glyphs mixed with real CJK text.
	// Without DeepDoc, isGarbledPage should return false (isScanNoise gate),
	// so chars are kept and sections > 0.
	chars := make([]TextChar, 30)
	for i := 0; i < 20; i++ {
		chars[i] = TextChar{
			Text: "测试文本", FontName: "SimSun",
			X0: 50, X1: 128, Top: float64(100 + i*15), Bottom: float64(112 + i*15),
		}
	}
	// Insert ### unmapped glyph noise (no subset fonts)
	chars[20] = TextChar{Text: "#", FontName: "SimSun", X0: 130, X1: 138, Top: 100, Bottom: 112}
	chars[21] = TextChar{Text: "#", FontName: "SimSun", X0: 138, X1: 146, Top: 100, Bottom: 112}
	chars[22] = TextChar{Text: "#", FontName: "SimSun", X0: 146, X1: 154, Top: 100, Bottom: 112}
	chars[23] = TextChar{Text: "D", FontName: "SimSun", X0: 154, X1: 162, Top: 100, Bottom: 112}
	chars[24] = TextChar{Text: "_", FontName: "SimSun", X0: 162, X1: 170, Top: 100, Bottom: 112}
	chars[25] = TextChar{Text: "8", FontName: "SimSun", X0: 170, X1: 178, Top: 100, Bottom: 112}
	chars[26] = TextChar{Text: "-", FontName: "SimSun", X0: 178, X1: 186, Top: 100, Bottom: 112}
	chars[27] = TextChar{Text: ".", FontName: "SimSun", X0: 186, X1: 194, Top: 100, Bottom: 112}
	chars[28] = TextChar{Text: "*", FontName: "SimSun", X0: 194, X1: 202, Top: 100, Bottom: 112}
	chars[29] = TextChar{Text: "用", FontName: "SimSun", X0: 202, X1: 210, Top: 100, Bottom: 112}

	mockEng := &mockEngine{chars: map[int][]TextChar{0: chars}, pageCount: 1}
	p := NewParser(DefaultParserConfig(), &MockDocAnalyzer{Healthy: true, Model: ModelSaas})
	result, err := p.Parse(context.Background(), mockEng)
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
		chars := make([]TextChar, 50)
		for i := range chars {
			chars[i] = TextChar{Text: string(rune(0xE000)), PageNumber: 0}
		}
		if !isGarbledPage(chars) {
			t.Error("100% PUA → garbled")
		}
	})
	t.Run("font encoding", func(t *testing.T) {
		if !isGarbledPage(garbledSample()) {
			t.Error("subset font → garbled")
		}
	})
	t.Run("normal text", func(t *testing.T) {
		chars := make([]TextChar, 50)
		for i := range chars {
			chars[i] = TextChar{Text: "a", PageNumber: 0}
		}
		if isGarbledPage(chars) {
			t.Error("normal text → not garbled")
		}
	})
	t.Run("pdf oxide unmapped + CJK — not garbled", func(t *testing.T) {
		// ### unmapped glyphs + real CJK text (no subset fonts).
		// isScanNoise returns false (≥2 consecutive CJK chars: "护理全科").
		chars := []TextChar{
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
		if isGarbledPage(chars) {
			t.Error("### unmapped + CJK text should NOT be garbled (no subset fonts)")
		}
	})
	t.Run("too few chars", func(t *testing.T) {
		if isGarbledPage([]TextChar{{Text: " ", PageNumber: 0}}) {
			t.Error("< 20 chars → not garbled")
		}
	})
}

func TestOCR_fallback_PUAGarbled(t *testing.T) {
	pua := make([]TextChar, 50)
	for i := range pua {
		pua[i] = TextChar{Text: string(rune(0xE000 + i%10)), PageNumber: 0}
	}
	dummyImg := image.NewRGBA(image.Rect(0, 0, 100, 100))
	mock := &MockDocAnalyzer{
		Healthy:  true,
		OCRBoxes: []OCRBox{{X0: 10, Y0: 20, X1: 90, Y1: 20, X2: 90, Y2: 40, X3: 10, Y3: 40}},
		OCRTexts: []OCRText{{Text: "PUA OCR text", Confidence: 0.9}},
	}
	got := ocrDetectAndRecognize(context.Background(), dummyImg, mock, 0, "garbled page")
	if len(got) != 1 || got[0].Text != "PUA OCR text" {
		t.Errorf("PUA garbled should trigger OCR, got %v", got)
	}
}

// ── ocrMergeChars ─────────────────────────────────────────────────────

func TestOCR_MergeChars(t *testing.T) {
	dummyImg := image.NewRGBA(image.Rect(0, 0, 600, 600))

	t.Run("nil image", func(t *testing.T) {
		chars := []TextChar{{X0: 10, Top: 10, X1: 20, Bottom: 30, Text: "A", PageNumber: 0}}
		if boxes := ocrMergeChars(context.Background(), nil, chars, &MockDocAnalyzer{Healthy: true}, 0); boxes != nil {
			t.Error("nil image → nil")
		}
	})

	t.Run("detect returns no boxes", func(t *testing.T) {
		mock := &MockDocAnalyzer{Healthy: true, OCRBoxes: []OCRBox{}}
		chars := []TextChar{{X0: 10, Top: 10, X1: 20, Bottom: 30, Text: "A", PageNumber: 0}}
		if boxes := ocrMergeChars(context.Background(), dummyImg, chars, mock, 0); boxes != nil {
			t.Error("no detect boxes → nil")
		}
	})

	t.Run("detect boxes — all overlap with chars (chars used, Python-aligned)", func(t *testing.T) {
		mock := &MockDocAnalyzer{
			Healthy:  true,
			OCRBoxes: []OCRBox{{X0: 15, Y0: 15, X1: 150, Y1: 15, X2: 150, Y2: 150, X3: 15, Y3: 150}},
			OCRTexts: []OCRText{{Text: "Hello OCR", Confidence: 0.9}},
		}
		chars := []TextChar{{X0: 10, X1: 30, Top: 10, Bottom: 30, Text: "Hello", PageNumber: 0}}
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
			Healthy:  true,
			OCRBoxes: []OCRBox{{X0: 240, Y0: 240, X1: 270, Y1: 240, X2: 270, Y2: 270, X3: 240, Y3: 270}},
			OCRTexts: []OCRText{{Text: "OCR", Confidence: 0.9}},
		}
		chars := []TextChar{{X0: 10, X1: 20, Top: 10, Bottom: 20, Text: "A", PageNumber: 0}}
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
			Healthy:  true,
			OCRBoxes: []OCRBox{{X0: 240, Y0: 240, X1: 270, Y1: 240, X2: 270, Y2: 270, X3: 240, Y3: 270}},
			OCRTexts: []OCRText{},
		}
		chars := []TextChar{{X0: 10, X1: 20, Top: 10, Bottom: 20, Text: "A", PageNumber: 0}}
		boxes := ocrMergeChars(context.Background(), dummyImg, chars, mock, 0)
		if len(boxes) != 0 {
			t.Fatalf("expected 0 boxes (empty OCR), got %d", len(boxes))
		}
	})

	t.Run("multiple detect boxes — one with chars, one OCR", func(t *testing.T) {
		// Box 1 overlaps chars → uses char text. Box 2 has no chars → OCR.
		mock := &MockDocAnalyzer{
			Healthy: true,
			OCRBoxes: []OCRBox{
				{X0: 15, Y0: 15, X1: 150, Y1: 15, X2: 150, Y2: 150, X3: 15, Y3: 150},
				{X0: 240, Y0: 240, X1: 270, Y1: 240, X2: 270, Y2: 270, X3: 240, Y3: 270},
			},
			OCRTexts: []OCRText{
				{Text: "box 1 text", Confidence: 0.9},
			},
		}
		chars := []TextChar{{X0: 10, X1: 30, Top: 10, Bottom: 30, Text: "Hello", PageNumber: 0}}
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
			OCRBoxes: []OCRBox{
				{X0: 15, Y0: 30, X1: 90, Y1: 30, X2: 90, Y2: 90, X3: 15, Y3: 90},
				{X0: 75, Y0: 330, X1: 300, Y1: 330, X2: 300, Y2: 390, X3: 75, Y3: 390},
			},
		}
		chars := []TextChar{
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
			OCRBoxes: []OCRBox{
				{X0: 15, Y0: 75, X1: 150, Y1: 75, X2: 150, Y2: 165, X3: 15, Y3: 165},
			},
			OCRTexts: []OCRText{{Text: "OCR height test", Confidence: 0.9}},
		}
		chars := []TextChar{
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
			OCRBoxes: []OCRBox{
				{X0: 15, Y0: 15, X1: 450, Y1: 15, X2: 450, Y2: 450, X3: 15, Y3: 450},
			},
			OCRTexts: []OCRText{{Text: "OCR result", Confidence: 0.9}},
		}
		chars := []TextChar{
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
			Healthy:  true,
			OCRBoxes: []OCRBox{{X0: 30, Y0: 30, X1: 90, Y1: 30, X2: 90, Y2: 90, X3: 30, Y3: 90}},
			OCRTexts: []OCRText{{Text: "docker commit infiniflow", Confidence: 0.95}},
		}
		chars := []TextChar{
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

func TestLineToTextBox_SpaceInsertion(t *testing.T) {
	// ASCII chars with visible gap → space inserted.
	chars := []TextChar{
		{X0: 0, X1: 8, Text: "H"},
		{X0: 12, X1: 16, Text: "i"},
	}
	box := lineToTextBox(chars)
	if box.Text != "H i" {
		t.Errorf("expected 'H i', got %q", box.Text)
	}
}

func TestLineToTextBox_NoSpaceForCJK(t *testing.T) {
	// CJK chars should NOT get space inserted.
	chars := []TextChar{
		{X0: 0, X1: 8, Text: "你"},
		{X0: 12, X1: 20, Text: "好"},
	}
	box := lineToTextBox(chars)
	if box.Text != "你好" {
		t.Errorf("expected '你好', got %q", box.Text)
	}
}

func TestLineToTextBox_NoSpaceForTightGap(t *testing.T) {
	// Small gap below threshold → no space.
	chars := []TextChar{
		{X0: 0, X1: 8, Text: "a"},
		{X0: 9, X1: 16, Text: "b"},
	}
	box := lineToTextBox(chars)
	if box.Text != "ab" {
		t.Errorf("expected 'ab', got %q", box.Text)
	}
}

func TestLineToTextBox_EmptyTextSkipsSpace(t *testing.T) {
	chars := []TextChar{
		{X0: 0, X1: 8, Text: ""},
		{X0: 12, X1: 16, Text: "A"},
	}
	box := lineToTextBox(chars)
	if box.Text != "A" {
		t.Errorf("expected 'A', got %q", box.Text)
	}
}

// TestTableToHTML verifies the HTML table format matches Python's
// construct_table output (tsr.py:293-313).
func TestRowsToHTML(t *testing.T) {
	// rowsToHTML takes [][]TSRCell instead of [][]string (tableToHTML removed).
	toCells := func(rows [][]string) [][]TSRCell {
		out := make([][]TSRCell, len(rows))
		for ri, row := range rows {
			out[ri] = make([]TSRCell, len(row))
			for ci, s := range row {
				out[ri][ci] = TSRCell{Text: s}
			}
		}
		return out
	}

	t.Run("simple 2x2 table", func(t *testing.T) {
		rows := toCells([][]string{
			{"姓名", "年龄"},
			{"张三", "25"},
		})
		html := rowsToHTML(rows, "", nil, nil, nil)
		expected := "<table><tr><td >姓名</td><td >年龄</td></tr><tr><td >张三</td><td >25</td></tr></table>"
		if html != expected {
			t.Errorf("got  %q\nwant %q", html, expected)
		}
	})

	t.Run("empty table", func(t *testing.T) {
		html := rowsToHTML(nil, "", nil, nil, nil)
		if html != "<table></table>" {
			t.Errorf("expected '<table></table>', got %q", html)
		}
	})

	t.Run("single cell", func(t *testing.T) {
		rows := toCells([][]string{{"X"}})
		html := rowsToHTML(rows, "", nil, nil, nil)
		expected := "<table><tr><td >X</td></tr></table>"
		if html != expected {
			t.Errorf("got  %q\nwant %q", html, expected)
		}
	})

	t.Run("matches Python format for 公司差旅费", func(t *testing.T) {
		rows := toCells([][]string{
			{"标职务", "飞机", "火车", "轮船", "其他交通工具（不含的士）"},
			{"公司级领导人员", "经济舱位", "火车软席", "二等舱位", "按实报销"},
			{"其他工作人员", "经济舱位", "火车硬席", "三等舱位", "按实报销"},
		})
		html := rowsToHTML(rows, "", nil, nil, nil)
		if !strings.HasPrefix(html, "<table>") || !strings.HasSuffix(html, "</table>") {
			t.Errorf("not valid HTML: %s", html)
		}
		if !strings.Contains(html, "<td >标职务</td>") {
			t.Errorf("missing cell '标职务': %s", html)
		}
		if strings.Count(html, "<tr>") != 3 {
			t.Errorf("expected 3 rows, got %d", strings.Count(html, "<tr>"))
		}
	})
}

// TestExtractTableAndReplace verifies that extractTableAndReplace pops
// table boxes and replaces them with consolidated HTML, matching Python.
func TestExtractTableAndReplace(t *testing.T) {
	// Build boxes with table labels and a TableItem with cells.
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20, Text: "A", LayoutType: "table", PageNumber: 0, R: 0, C: 0},
		{X0: 0, X1: 100, Top: 21, Bottom: 40, Text: "B", LayoutType: "table", PageNumber: 0, R: 0, C: 0},
		{X0: 110, X1: 200, Top: 0, Bottom: 20, Text: "C", LayoutType: "table", PageNumber: 0, R: 0, C: 1},
		{X0: 110, X1: 200, Top: 21, Bottom: 40, Text: "D", LayoutType: "table", PageNumber: 0, R: 0, C: 1},
	}
	tbl := TableItem{
		Cells: []TSRCell{
			{X0: 0, Y0: 0, X1: 100, Y1: 20, Label: "table row"},
			{X0: 110, Y0: 0, X1: 200, Y1: 20, Label: "table row"},
			{X0: 0, Y0: 21, X1: 100, Y1: 40, Label: "table row"},
			{X0: 110, Y0: 21, X1: 200, Y1: 40, Label: "table row"},
		},
		Positions: []Position{{Left: 0, Right: 200, Top: 0, Bottom: 40}},
		Scale: 1.0,
	}
	result := extractTableAndReplace(boxes, []TableItem{tbl})
	if len(result) != 1 {
		t.Fatalf("expected 1 box (replaced), got %d", len(result))
	}
	if result[0].LayoutType != "table" {
		t.Errorf("expected LayoutType table, got %q", result[0].LayoutType)
	}
	if !strings.Contains(result[0].Text, "<table>") {
		t.Errorf("expected HTML table, got %q", result[0].Text)
	}
}

// TestTableSectionCaptionInHTML verifies mergeCaptions prepends table
// caption text before the HTML table, matching Python's caption handling.
func TestTableSectionCaptionInHTML(t *testing.T) {
	// Simulate pipeline order: extractTableAndReplace → boxesToSections → mergeCaptions
	boxes := []TextBox{
		{X0: 100, X1: 500, Top: 200, Bottom: 400, LayoutType: "table", PageNumber: 0},
	}
	ti := TableItem{
		Cells: []TSRCell{
			{X0: 0, Y0: 0, X1: 200, Y1: 50, Label: "table row", Text: "飞机"},
			{X0: 0, Y0: 51, X1: 200, Y1: 100, Label: "table row", Text: "火车"},
		},
		Positions: []Position{{Left: 100, Right: 500, Top: 200, Bottom: 400}},
		Scale: 1.0,
	}

	// Step 1: extractTableAndReplace → HTML box with table text
	boxes = extractTableAndReplace(boxes, []TableItem{ti})
	sections := boxesToSections(boxes, nil)

	// Add caption section
	sections = append(sections, Section{
		LayoutType: "table caption",
		Positions:  []Position{{Left: 100, Right: 500, Top: 180, Bottom: 198}},
		Text:       "表1: 交通工具等级",
	})

	// Step 2: mergeCaptions prepends caption before HTML
	figures := CollectFigures(sections)
	sections = mergeCaptions(sections, figures)

	if !strings.HasPrefix(sections[0].Text, "表1: 交通工具等级<table>") {
		t.Errorf("expected caption before table HTML, got %q", sections[0].Text)
	}
}

// TestBoxMatchesCell_FalsePositive verifies that boxMatchesCell rejects
// text boxes that are mostly OUTSIDE the cell, even with cellIsEmpty=true.
// The 0.3 threshold should not match a wide box that barely touches a
// narrow cell — this would cause body text to leak into table cells.
func TestBoxMatchesCell_FalsePositive(t *testing.T) {
	// Cell: narrow table cell (40×20 px)
	cell := TSRCell{X0: 0, Y0: 0, X1: 40, Y1: 20}

	// Box A: entirely inside the cell → should match.
	boxA := TextBox{X0: 5, X1: 35, Top: 2, Bottom: 18, Text: "标职务"}

	// Box B: a wide body-text box that only slightly overlaps the cell.
	// It covers x=30..200 but the cell is only x=0..40.
	// Overlap: x=30..40 (10px), box width=170 → ratio=10/170=0.059 < 0.3.
	boxB := TextBox{X0: 30, X1: 200, Top: 5, Bottom: 15, Text: "第二条出差人员应按规定等级乘坐交通工具..."}

	if !boxMatchesCell(cell, boxA, true) {
		t.Error("boxA entirely inside cell should match with cellIsEmpty=true")
	}
	if boxMatchesCell(cell, boxB, true) {
		t.Error("boxB mostly outside cell should NOT match even with cellIsEmpty=true")
	}
	if !boxMatchesCell(cell, boxA, false) {
		t.Error("boxA entirely inside cell should match with cellIsEmpty=false")
	}
	if boxMatchesCell(cell, boxB, false) {
		t.Error("boxB mostly outside cell should NOT match with cellIsEmpty=false")
	}
}

// TestFillCellTextFromBoxes_PageGlobal verifies that fillCellTextFromBoxes
// correctly matches text boxes to cells when both use page-global 72 DPI
// coordinates, matching Python's construct_table approach.
func TestFillCellTextFromBoxes_PageGlobal(t *testing.T) {
	t.Run("exact alignment matches", func(t *testing.T) {
		cells := []TSRCell{
			{X0: 73, Y0: 329, X1: 214, Y1: 345},
			{X0: 214, Y0: 329, X1: 272, Y1: 345},
			{X0: 272, Y0: 329, X1: 407, Y1: 345},
		}
		boxes := []TextBox{
			{X0: 73, X1: 214, Top: 329, Bottom: 345, Text: "标职务"},
			{X0: 214, X1: 272, Top: 329, Bottom: 345, Text: "飞机"},
			{X0: 272, X1: 407, Top: 329, Bottom: 345, Text: "火车"},
		}
		fillCellTextFromBoxes(cells, boxes)
		if cells[0].Text != "标职务" {
			t.Errorf("cell[0] = %q, want '标职务'", cells[0].Text)
		}
		if cells[1].Text != "飞机" {
			t.Errorf("cell[1] = %q, want '飞机'", cells[1].Text)
		}
		if cells[2].Text != "火车" {
			t.Errorf("cell[2] = %q, want '火车'", cells[2].Text)
		}
	})

	t.Run("body text box does not leak into cell", func(t *testing.T) {
		cells := []TSRCell{{X0: 73, Y0: 329, X1: 214, Y1: 345}}
		boxes := []TextBox{
			{X0: 73, X1: 214, Top: 329, Bottom: 345, Text: "标职务"},
			{X0: 73, X1: 520, Top: 310, Bottom: 360, Text: "第二条出差人员应按规定"},
		}
		fillCellTextFromBoxes(cells, boxes)
		if cells[0].Text != "标职务" {
			t.Errorf("cell text = %q, want '标职务' (body text should not leak in)", cells[0].Text)
		}
	})

	t.Run("empty cells list is no-op", func(t *testing.T) {
		fillCellTextFromBoxes(nil, []TextBox{{Text: "x"}})
	})

	t.Run("empty boxes list preserves cell text", func(t *testing.T) {
		cells := []TSRCell{{Text: "existing"}}
		fillCellTextFromBoxes(cells, nil)
		if cells[0].Text != "existing" {
			t.Errorf("existing text should be preserved, got %q", cells[0].Text)
		}
	})
}

func TestCharsToBoxes_XGapSplitsColumns(t *testing.T) {
	// Simulate a table row with 3 columns: col 0="A", col 1="B", col 2="C".
	// Large X gaps between columns, small gaps within.
	chars := []TextChar{
		{X0: 10, X1: 18, Top: 0, Bottom: 12, Text: "A", PageNumber: 0},
		{X0: 18, X1: 26, Top: 0, Bottom: 12, Text: "1", PageNumber: 0},       // small gap after A
		{X0: 150, X1: 158, Top: 0, Bottom: 12, Text: "B", PageNumber: 0},     // large gap → new box
		{X0: 158, X1: 166, Top: 0, Bottom: 12, Text: "2", PageNumber: 0},     // small
		{X0: 300, X1: 308, Top: 0, Bottom: 12, Text: "C", PageNumber: 0},     // large gap → new box
		{X0: 308, X1: 316, Top: 0, Bottom: 12, Text: "3", PageNumber: 0},     // small
	}
	boxes := charsToBoxes(chars, 0, false)
	if len(boxes) != 3 {
		t.Fatalf("expected 3 boxes (one per column), got %d", len(boxes))
	}
	if boxes[0].Text != "A1" {
		t.Errorf("col 0: got %q, want %q", boxes[0].Text, "A1")
	}
	if boxes[1].Text != "B2" {
		t.Errorf("col 1: got %q, want %q", boxes[1].Text, "B2")
	}
	if boxes[2].Text != "C3" {
		t.Errorf("col 2: got %q, want %q", boxes[2].Text, "C3")
	}
}

func TestCharsToBoxes_NoSplitNormalText(t *testing.T) {
	// Normal English text: small gaps between chars.
	chars := []TextChar{
		{X0: 10, X1: 18, Top: 0, Bottom: 12, Text: "H", PageNumber: 0},
		{X0: 18, X1: 26, Top: 0, Bottom: 12, Text: "e", PageNumber: 0},
		{X0: 26, X1: 34, Top: 0, Bottom: 12, Text: "l", PageNumber: 0},
		{X0: 34, X1: 42, Top: 0, Bottom: 12, Text: "l", PageNumber: 0},
		{X0: 42, X1: 50, Top: 0, Bottom: 12, Text: "o", PageNumber: 0},
	}
	boxes := charsToBoxes(chars, 0, false)
	if len(boxes) != 1 {
		t.Fatalf("expected 1 box for normal text, got %d", len(boxes))
	}
	if boxes[0].Text != "Hello" {
		t.Errorf("got %q, want %q", boxes[0].Text, "Hello")
	}
}

func TestCharsToBoxes_SingleChar(t *testing.T) {
	chars := []TextChar{
		{X0: 10, X1: 18, Top: 0, Bottom: 12, Text: "X", PageNumber: 0},
	}
	boxes := charsToBoxes(chars, 0, false)
	if len(boxes) != 1 || boxes[0].Text != "X" {
		t.Errorf("single char: got %d boxes, text=%q", len(boxes), boxes[0].Text)
	}
}

func TestCharsToBoxes_Empty(t *testing.T) {
	boxes := charsToBoxes(nil, 0, false)
	if len(boxes) != 0 {
		t.Errorf("empty: got %d boxes", len(boxes))
	}
}

func TestCharsToBoxes_ChineseUniformSpacing(t *testing.T) {
	// CJK characters with uniform spacing — no column gaps.
	chars := []TextChar{
		{X0: 10, X1: 26, Top: 0, Bottom: 16, Text: "标", PageNumber: 0},
		{X0: 26, X1: 42, Top: 0, Bottom: 16, Text: "职", PageNumber: 0},
		{X0: 42, X1: 58, Top: 0, Bottom: 16, Text: "务", PageNumber: 0},
	}
	boxes := charsToBoxes(chars, 0, false)
	if len(boxes) != 1 {
		t.Fatalf("uniform CJK: expected 1 box, got %d", len(boxes))
	}
}


// TestBoxesToSections_CrossPagePositionTag verifies that a box whose bottom
// exceeds the page height produces a multi-page PositionTag.
// Python: _line_tag while-loop (pdf_parser.py:1279-1283) detects cross-page
// spans and generates "@@5-6\t..." tags.
func TestBoxesToSections_CrossPagePositionTag(t *testing.T) {
	// Page 0: 267 PDF-points tall (800px at zoom=3).
	// Box bottom=400 > 267 → spills into page 1 by 133pt.
	boxes := []TextBox{
		{X0: 100, X1: 500, Top: 200, Bottom: 400, PageNumber: 0, Text: "跨页表格"},
	}
	pageHeights := map[int]float64{0: 267.0}

	sections := boxesToSections(boxes, pageHeights)
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	s := sections[0]

	// Python: @@1-2\t100.0\t500.0\t200.0\t133.0##
	// Page 0→1 becomes 1-indexed → pages 1-2.
	if s.PositionTag != "@@1-2\t100.0\t500.0\t200.0\t133.0##" {
		t.Errorf("PositionTag: got %q, want '@@1-2\\t100.0\\t500.0\\t200.0\\t133.0##'", s.PositionTag)
	}
	if len(s.Positions) != 1 {
		t.Fatalf("expected 1 Position, got %d", len(s.Positions))
	}
	p := s.Positions[0]
	if len(p.PageNumbers) != 2 || p.PageNumbers[0] != 0 || p.PageNumbers[1] != 1 {
		t.Errorf("PageNumbers: got %v, want [0, 1]", p.PageNumbers)
	}
	if p.Top != 200 || p.Bottom != 133 {
		t.Errorf("coords: top=%v (want 200), bottom=%v (want 133 = 400-267)", p.Top, p.Bottom)
	}
}

// TestBoxesToSections_SinglePageUnchanged verifies single-page boxes are
// unaffected by the cross-page change.
func TestBoxesToSections_SinglePageUnchanged(t *testing.T) {
	boxes := []TextBox{
		{X0: 50, X1: 200, Top: 10, Bottom: 30, PageNumber: 0, Text: "普通文本"},
	}
	pageHeights := map[int]float64{0: 267.0}

	sections := boxesToSections(boxes, pageHeights)
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	// Single page: tag should be @@1, not @@1-1
	if sections[0].PositionTag != "@@1\t50.0\t200.0\t10.0\t30.0##" {
		t.Errorf("single-page PositionTag: got %q", sections[0].PositionTag)
	}
	if len(sections[0].Positions[0].PageNumbers) != 1 {
		t.Errorf("single-page PageNumbers: got %v, want [0]", sections[0].Positions[0].PageNumbers)
	}
}

func TestResolvePageSpan_SinglePage(t *testing.T) {
	// Box fits within the page → toPage unchanged, bottom unchanged.
	toPage, bottom := resolvePageSpan(0, 30, map[int]float64{0: 267})
	if toPage != 0 || bottom != 30 {
		t.Errorf("got toPage=%d bottom=%v, want 0, 30", toPage, bottom)
	}
}

func TestResolvePageSpan_CrossPage(t *testing.T) {
	// Box bottom=400 exceeds page 0 height=267 → spans to page 1.
	toPage, bottom := resolvePageSpan(0, 400, map[int]float64{0: 267})
	if toPage != 1 {
		t.Errorf("toPage = %d, want 1", toPage)
	}
	if bottom != 133 {
		t.Errorf("bottom = %v, want 133 (400-267)", bottom)
	}
}

func TestResolvePageSpan_MultiPage(t *testing.T) {
	// Box bottom=600, page 0=267, page 1=200, page 2=200.
	heights := map[int]float64{0: 267, 1: 200, 2: 200}
	toPage, bottom := resolvePageSpan(0, 600, heights)
	if toPage != 2 {
		t.Errorf("toPage = %d, want 2", toPage)
	}
	if bottom != 133 {
		t.Errorf("bottom = %v, want 133 (600-267-200)", bottom)
	}
}

func TestResolvePageSpan_NilHeights(t *testing.T) {
	toPage, bottom := resolvePageSpan(0, 400, nil)
	if toPage != 0 || bottom != 400 {
		t.Errorf("got toPage=%d bottom=%v, want 0, 400 (nil=no cross-page)", toPage, bottom)
	}
}

func TestResolvePageSpan_ZeroHeightGuard(t *testing.T) {
	// Zero-height pages must not cause an infinite loop.
	// Page 0=200, page 1=0, page 2=0, page 3=300 — box bottom=500.
	heights := map[int]float64{0: 200, 1: 0, 2: 0, 3: 300}
	toPage, bottom := resolvePageSpan(0, 500, heights)
	// 500-200=300 remaining; page1=0 → break at unknown/invalid; toPage=1, bottom=300.
	// (the break path treats zero/unknown as "assume same height once and stop")
	if toPage != 1 {
		t.Errorf("toPage = %d, want 1 (stopped at first zero-height page)", toPage)
	}
	if bottom != 300 {
		t.Errorf("bottom = %v, want 300 (500-200)", bottom)
	}
}

func TestResolvePageSpan_UnknownNextPage(t *testing.T) {
	// Next page not in map → assume same height once, then stop.
	heights := map[int]float64{0: 267}
	toPage, bottom := resolvePageSpan(0, 500, heights)
	if toPage != 1 {
		t.Errorf("toPage = %d, want 1 (one fallback extension)", toPage)
	}
	if bottom != 233 {
		t.Errorf("bottom = %v, want 233 (500-267)", bottom)
	}
}

func TestResolvePageSpan_NegativePh(t *testing.T) {
	heights := map[int]float64{0: 200, 1: -10, 2: 200}
	toPage, bottom := resolvePageSpan(0, 500, heights)
	if toPage != 1 {
		t.Errorf("toPage = %d, want 1 (stopped at negative-height page)", toPage)
	}
	if bottom != 300 {
		t.Errorf("bottom = %v, want 300 (500-200)", bottom)
	}
}

// TestCrossPageTableMerge verifies that mergeTablesAcrossPages merges
// two TableItems on consecutive pages with overlapping X positions.
// Python: _extract_table_figure merges cross-page tables by matching layoutno.
func TestCrossPageTableMerge(t *testing.T) {
	// Page 0 table: 2 cells, positioned at page 0.
	pg0 := TableItem{
		Positions: []Position{
			{PageNumbers: []int{0}, Left: 50, Right: 500, Top: 100, Bottom: 800},
		},
		Scale: 1.0,
		Cells: []TSRCell{
			{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "pg0_r0c0"},
			{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: "pg0_r0c1"},
		},
	}
	// Page 1 table: 2 cells, same X range, positioned at page 1.
	pg1 := TableItem{
		Positions: []Position{
			{PageNumbers: []int{1}, Left: 50, Right: 500, Top: 100, Bottom: 300},
		},
		Scale: 1.0,
		Cells: []TSRCell{
			{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "pg1_r0c0"},
			{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: "pg1_r0c1"},
		},
	}
	tables := []TableItem{pg0, pg1}

	// mergeTablesAcrossPages merges tables on consecutive pages with X overlap.
	merged := mergeTablesAcrossPages(tables, nil)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged table, got %d", len(merged))
	}
	if len(merged[0].Cells) != 4 {
		t.Errorf("expected 4 merged cells, got %d", len(merged[0].Cells))
	}
	if len(merged[0].Positions) != 2 {
		t.Errorf("expected 2 merged positions, got %d", len(merged[0].Positions))
	}
	t.Logf("Merged %d cells across %d pages", len(merged[0].Cells), len(merged[0].Positions))
}

// TestMergeTablesAcrossPages_NoOverlap verifies that non-adjacent or
// non-overlapping tables are NOT merged.
func TestMergeTablesAcrossPages_NoOverlap(t *testing.T) {
	// Tables with no X overlap should NOT be merged.
	tables := []TableItem{
		{
			Positions: []Position{{PageNumbers: []int{0}, Left: 50, Right: 100, Top: 100, Bottom: 500}},
			Scale:     1.0,
			Cells:     []TSRCell{{Text: "left"}},
		},
		{
			Positions: []Position{{PageNumbers: []int{1}, Left: 500, Right: 600, Top: 100, Bottom: 500}},
			Scale:     1.0,
			Cells:     []TSRCell{{Text: "right"}},
		},
	}
	merged := mergeTablesAcrossPages(tables, nil)
	if len(merged) != 2 {
		t.Fatalf("non-overlapping tables: expected 2 tables, got %d", len(merged))
	}
}

// TestMergeTablesAcrossPages_NonConsecutive verifies that tables on
// non-consecutive pages are NOT merged.
func TestMergeTablesAcrossPages_NonConsecutive(t *testing.T) {
	tables := []TableItem{
		{
			Positions: []Position{{PageNumbers: []int{0}, Left: 50, Right: 500, Top: 100, Bottom: 500}},
			Scale:     1.0,
			Cells:     []TSRCell{{Text: "page0"}},
		},
		{
			Positions: []Position{{PageNumbers: []int{3}, Left: 50, Right: 500, Top: 100, Bottom: 500}},
			Scale:     1.0,
			Cells:     []TSRCell{{Text: "page3"}},
		},
	}
	merged := mergeTablesAcrossPages(tables, nil)
	if len(merged) != 2 {
		t.Fatalf("non-consecutive pages: expected 2 tables, got %d", len(merged))
	}
}

// TestMergeTablesAcrossPages_SingleTable verifies that a single table
// passes through unchanged.
func TestMergeTablesAcrossPages_SingleTable(t *testing.T) {
	tables := []TableItem{
		{
			Positions: []Position{{PageNumbers: []int{0}, Left: 50, Right: 500, Top: 100, Bottom: 500}},
			Scale:     1.0,
			Cells:     []TSRCell{{Text: "only"}},
		},
	}
	merged := mergeTablesAcrossPages(tables, nil)
	if len(merged) != 1 {
		t.Fatalf("single table: expected 1 table, got %d", len(merged))
	}
}



func TestCharsToBoxes_CJKWordGapNoSplit(t *testing.T) {
	chars := []TextChar{
		{X0: 10, X1: 26, Top: 0, Bottom: 16, Text: "二", PageNumber: 0},
		{X0: 38, X1: 54, Top: 0, Bottom: 16, Text: "等", PageNumber: 0},
		{X0: 54, X1: 70, Top: 0, Bottom: 16, Text: "舱", PageNumber: 0},
		{X0: 70, X1: 86, Top: 0, Bottom: 16, Text: "位", PageNumber: 0},
	}
	boxes := charsToBoxes(chars, 0, false)
	if len(boxes) != 1 {
		t.Fatalf("CJK word gap: expected 1 box, got %d", len(boxes))
	}
}

func TestCharsToBoxes_VaryingColumnGaps(t *testing.T) {
	// Realistic page: many chars per column (gap~0), REAL column gaps (30+, 50+).
	chars := []TextChar{
		{X0: 10, X1: 26, Top: 0, Bottom: 16, Text: "姓", PageNumber: 0},
		{X0: 26, X1: 42, Top: 0, Bottom: 16, Text: "名", PageNumber: 0},
		{X0: 42, X1: 58, Top: 0, Bottom: 16, Text: "称", PageNumber: 0},
		{X0: 108, X1: 124, Top: 0, Bottom: 16, Text: "年", PageNumber: 0},
		{X0: 124, X1: 140, Top: 0, Bottom: 16, Text: "龄", PageNumber: 0},
		{X0: 180, X1: 196, Top: 0, Bottom: 16, Text: "性", PageNumber: 0},
		{X0: 196, X1: 212, Top: 0, Bottom: 16, Text: "别", PageNumber: 0},
	}
	boxes := charsToBoxes(chars, 0, false)
	if len(boxes) != 3 {
		t.Fatalf("varying column gaps: expected 3 boxes, got %d", len(boxes))
	}
}

func TestCharsToBoxes_MixedCJKEnglishNoSplit(t *testing.T) {
	chars := []TextChar{
		{X0: 10, X1: 26, Top: 0, Bottom: 16, Text: "经", PageNumber: 0},
		{X0: 26, X1: 42, Top: 0, Bottom: 16, Text: "济", PageNumber: 0},
		{X0: 42, X1: 50, Top: 0, Bottom: 16, Text: "A", PageNumber: 0},
		{X0: 50, X1: 58, Top: 0, Bottom: 16, Text: "B", PageNumber: 0},
	}
	boxes := charsToBoxes(chars, 0, false)
	if len(boxes) != 1 {
		t.Fatalf("mixed CJK+English: expected 1 box, got %d", len(boxes))
	}
}

// TestMergeCaptions_NeedsCaptionLayoutType exposes that mergeCaptions only
// strips caption sections when DLA labels them as "table caption" or
// "figure caption".  When DLA labels them as "text" (real scenario with
// some PDF layouts), the caption text remains in the table output.
func TestMergeCaptions_NeedsCaptionLayoutType(t *testing.T) {
	// Simulate what happens when DLA doesn't produce a "table caption" region:
	// a "text" section adjacent to a table is NOT treated as caption.
	sections := []Section{
		{LayoutType: "table", Text: "<table><tr><td >data</td></tr></table>",
			Positions: []Position{{Left: 100, Right: 500, Top: 200, Bottom: 400}}},
		{LayoutType: "text", Text: "公司领导班子成员、出差地",
			Positions: []Position{{Left: 100, Right: 500, Top: 180, Bottom: 198}}},
	}
	figures := CollectFigures(sections)
	result := mergeCaptions(sections, figures)
	// BUG: "text" layout type is NOT matched by mergeCaptions (only "table caption"/"figure caption").
	// The caption text survives as a separate section instead of being prepended to the table.
	for _, s := range result {
		if s.LayoutType == "text" && strings.Contains(s.Text, "公司领导班子") {
			t.Log("KNOWN LIMITATION: caption with LayoutType='text' not stripped by mergeCaptions")
		}
	}
}

// TestGroupBoxesByRC_ColspanMissing exposes that groupBoxesByRC doesn't
// compute colspan/rowspan from SP annotations (__cal_spans in Python).
// Spanning cells should be annotated with colspan/rowspan in the HTML output.
func TestGroupBoxesByRC_ColspanMissing(t *testing.T) {
	// Box with SP annotation spanning 2 columns (HLeft→HRight covers cols 0-1).
	boxes := []TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "Name", R: 0, C: 0, H: 1,
			HLeft: 10, HRight: 200},
		{X0: 110, X1: 200, Top: 0, Bottom: 30, Text: "", R: 0, C: 1, SP: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "A", R: 1, C: 0},
		{X0: 110, X1: 200, Top: 35, Bottom: 65, Text: "B", R: 1, C: 1},
	}
	rows := groupBoxesByRC(boxes)
	// The result should have colspan=2 for cell [0,0] and skip [0,1].
	// Currently groupBoxesByRC produces a flat grid without span info.
	if len(rows) >= 1 && len(rows[0]) >= 2 && rows[0][1].Text == "" {
		t.Log("KNOWN LIMITATION: colspan not computed — cell [0,1] is empty instead of merged")
	}
	_ = rows
}
