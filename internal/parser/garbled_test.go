package parser

import (
	"testing"
)

func TestIsGarbledChar(t *testing.T) {
	tests := []struct {
		name string
		ch   string
		want bool
	}{
		{"empty", "", false},
		{"normal ascii", "A", false},
		{"normal chinese", "你", false},
		{"PUA char E000", "", true},
		{"PUA char F8FF", "", true},
		{"replacement char", "�", true},
		{"null control", "\x00", true},
		{"tab", "\t", false},
		{"newline", "\n", false},
		{"C1 control", "", true},
		{"C1 control 9F", "", true},
		{"normal single byte", "z", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsGarbledChar(tt.ch)
			if got != tt.want {
				t.Errorf("IsGarbledChar(%q) = %v, want %v", tt.ch, got, tt.want)
			}
		})
	}
}

func TestIsGarbledText(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		threshold float64
		want      bool
	}{
		{"empty", "", 0.5, false},
		{"normal text", "正常文本", 0.5, false},
		{"cid pattern", "(cid:123)", 0.5, true},
		{"all garbled", "", 0.5, true},
		{"one garbled in many", "ABDEFGHI", 0.5, false},
		{"half garbled strict", "AB", 0.5, true},
		{"half garbled loose", "AB", 0.7, false},
		{"english text", "Hello World", 0.5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsGarbledText(tt.text, tt.threshold)
			if got != tt.want {
				t.Errorf("IsGarbledText(%q, %v) = %v, want %v", tt.text, tt.threshold, got, tt.want)
			}
		})
	}
}

func TestHasSubsetFontPrefix(t *testing.T) {
	tests := []struct {
		name     string
		fontName string
		want     bool
	}{
		{"subset prefix", "DY1+ZLQDm1-1", true},
		{"short subset", "AB+SimSun", true},
		{"no prefix", "SimSun", false},
		{"empty", "", false},
		{"just plus", "+SimSun", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasSubsetFontPrefix(tt.fontName)
			if got != tt.want {
				t.Errorf("HasSubsetFontPrefix(%q) = %v, want %v", tt.fontName, got, tt.want)
			}
		})
	}
}

func TestIsGarbledByFontEncoding(t *testing.T) {
	t.Run("too few chars", func(t *testing.T) {
		chars := make([]TextChar, 10)
		if IsGarbledByFontEncoding(chars, 20) {
			t.Error("should return false when below minChars threshold")
		}
	})

	t.Run("subset font with ascii — garbled", func(t *testing.T) {
		// Simulate CJK PDF with broken font encoding: all chars have subset font prefix,
		// virtually no CJK, almost all ASCII punctuation
		var chars []TextChar
		for i := 0; i < 30; i++ {
			chars = append(chars, TextChar{
				Text:     "!",
				FontName: "DY1+SimSun",
			})
		}
		// Add some CJK (but below 5%)
		chars = append(chars, TextChar{Text: "你", FontName: "DY1+SimSun"})
		if !IsGarbledByFontEncoding(chars, 20) {
			t.Error("should detect garbled font encoding")
		}
	})

	t.Run("regular CJK text — not garbled", func(t *testing.T) {
		var chars []TextChar
		for i := 0; i < 30; i++ {
			chars = append(chars, TextChar{
				Text:     "测试文本内容",
				FontName: "SimSun",
			})
		}
		if IsGarbledByFontEncoding(chars, 20) {
			t.Error("should not flag regular CJK text as garbled")
		}
	})

	t.Run("fullwidth chars from subset font — not garbled", func(t *testing.T) {
			// Fullwidth characters (U+FF01-U+FF5E) are legitimate CJK typographic forms.
			// They should count as cjkLike, preventing false garbled detection.
			var chars []TextChar
			for i := 0; i < 30; i++ {
				chars = append(chars, TextChar{
					Text:     "ＡＢＣＤＥＦ", // U+FF21-U+FF26 fullwidth uppercase
					FontName: "DY1+SimSun",
				})
			}
			if IsGarbledByFontEncoding(chars, 20) {
				t.Error("fullwidth chars from subset font should NOT be garbled")
			}
		})

		t.Run("normal English text — not garbled", func(t *testing.T) {
		var chars []TextChar
		for i := 0; i < 30; i++ {
			chars = append(chars, TextChar{
				Text:     "Hello world text content here",
				FontName: "Times-Roman",
			})
		}
		if IsGarbledByFontEncoding(chars, 20) {
			t.Error("should not flag regular English text as garbled")
		}
	})
}

func TestDetectGarbled(t *testing.T) {
	// Normal CJK text
	chars := make([]TextChar, 30)
	for i := range chars {
		chars[i] = TextChar{Text: "正常文本", FontName: "SimSun"}
	}
	if DetectGarbled(chars) {
		t.Error("normal CJK should not be garbled")
	}

	// Subset font with punctuation
	var garbled []TextChar
	for i := 0; i < 30; i++ {
		garbled = append(garbled, TextChar{Text: "!", FontName: "DY1+SimSun"})
	}
	if !DetectGarbled(garbled) {
		t.Error("subset font with punctuation should be garbled")
	}
}

// ── pdf_oxide ### detection tests ─────────────────────────────────────

func TestPdfOxideUnmappedGarbled_Empty(t *testing.T) {
	if pdfOxideUnmappedGarbled("") {
		t.Error("empty text should not be garbled")
	}
}

func TestPdfOxideUnmappedGarbled_NormalText(t *testing.T) {
	if pdfOxideUnmappedGarbled("这是一段正常的中文文本没有任何问题") {
		t.Error("normal Chinese text should not be garbled")
	}
}

func TestPdfOxideUnmappedGarbled_SingleHash(t *testing.T) {
	// A single # is not enough (could be a phone number or reference).
	if pdfOxideUnmappedGarbled("参考 #123 的文献") {
		t.Error("single # should not be garbled")
	}
}

func TestPdfOxideUnmappedGarbled_TripleHashCluster(t *testing.T) {
	// Two ### sequences => garbled.
	if !pdfOxideUnmappedGarbled("我信###D_8-.###$#(") {
		t.Error("two ### clusters should be garbled")
	}
}

func TestPdfOxideUnmappedGarbled_QuadHash(t *testing.T) {
	// One #### counts as one ### cluster. Need two for trigger.
	// But density may also be high enough.
	if !pdfOxideUnmappedGarbled("text####abc####def") {
		t.Error("two #### clusters should be garbled")
	}
}

func TestPdfOxideUnmappedGarbled_SingleTriple(t *testing.T) {
	// Single ### cluster => garbled.  In a 200-char sample "###" is impossible
	// in normal text (URLs/markdown use at most "##").
	if !pdfOxideUnmappedGarbled("hello###world normal text here") {
		t.Error("single ### cluster should be garbled")
	}
}

func TestPdfOxideUnmappedGarbled_HighDensity(t *testing.T) {
	// 10 # chars mixed among 40+ non-space chars = 25% → garbled.
	text := "#a#b#c#d#e#f#g#h#i#j" + " extra normal chars padding to reach minimum"
	if !pdfOxideUnmappedGarbled(text) {
		t.Error("high # density should be garbled")
	}
}

func TestPdfOxideUnmappedGarbled_RealWorldGarbled(t *testing.T) {
	// Simulates the garbled page from 1例3个月...pdf:
	// Chinese text mixed with ###D_ style unmapped glyph patterns.
	garbled := "和蔘语言###D_8-.*/*护理全科##%&$ 80引用\"\"###$#(点向患儿"
	if !pdfOxideUnmappedGarbled(garbled) {
		t.Error("real-world garbled text with ### clusters should be detected")
	}
}
