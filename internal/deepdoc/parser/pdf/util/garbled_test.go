package util

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
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
		{"subset prefix", "DY1+Z1QDm1-1", true},
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
		chars := make([]pdf.TextChar, 10)
		if IsGarbledByFontEncoding(chars, 20) {
			t.Error("should return false when below minChars threshold")
		}
	})
	t.Run("subset font with ascii — garbled", func(t *testing.T) {
		var chars []pdf.TextChar
		for i := 0; i < 30; i++ {
			chars = append(chars, pdf.TextChar{Text: "!", FontName: "DY1+SimSun"})
		}
		chars = append(chars, pdf.TextChar{Text: "你", FontName: "DY1+SimSun"})
		if !IsGarbledByFontEncoding(chars, 20) {
			t.Error("should detect garbled font encoding")
		}
	})
	t.Run("regular CJK text — not garbled", func(t *testing.T) {
		var chars []pdf.TextChar
		for i := 0; i < 30; i++ {
			chars = append(chars, pdf.TextChar{Text: "测试文本内容", FontName: "SimSun"})
		}
		if IsGarbledByFontEncoding(chars, 20) {
			t.Error("should not flag regular CJK text as garbled")
		}
	})
	t.Run("normal English text — not garbled", func(t *testing.T) {
		var chars []pdf.TextChar
		for i := 0; i < 30; i++ {
			chars = append(chars, pdf.TextChar{Text: "Hello world text content here", FontName: "Times-Roman"})
		}
		if IsGarbledByFontEncoding(chars, 20) {
			t.Error("should not flag regular English text as garbled")
		}
	})
}
