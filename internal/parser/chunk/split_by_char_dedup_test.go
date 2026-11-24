package chunk

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// splitByCharReference is the pre-optimization oracle: every emitted char
// chunk gets DetectLanguage over the FULL REMAINING text (O(n^2)). The
// optimized version must produce byte-identical output including the subtle
// per-chunk language flips that arise when the remaining suffix's CJK ratio
// crosses the 0.3 threshold.
func splitByCharReference(text string) []ChunkData {
	var chunks []ChunkData
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		chunks = append(chunks, ChunkData{
			Content:  string(r),
			Index:    len(chunks),
			Metadata: map[string]interface{}{"language": DetectLanguage(text)},
		})
		text = text[size:]
	}
	return chunks
}

func runSplitByChar(text string) []ChunkData {
	op := &SplitOperator{}
	return op.splitByChar(text)
}

func TestSplitByCharSuffixEquivalence(t *testing.T) {
	cases := []struct {
		name, text string
	}{
		{"pure-cjk", "中文测试文本语言检测"},
		{"pure-en", "hello world this is english"},
		// Adversarial: a leading English run followed by CJK. The remaining
		// suffix's CJK ratio crosses 0.3 partway, so early chunks are "en" and
		// late chunks are "zh" — a single whole-string DetectLanguage would
		// collapse them all and break equivalence.
		{"en-then-cjk", "abcdefghijklmnop" + "中文测试文本"},
		{"cjk-then-en", "中文测试" + "hello world"},
		{"emoji", "a😀b😀c中文"},
		{"empty", ""},
		{"single", "中"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := splitByCharReference(tc.text)
			got := runSplitByChar(tc.text)
			if len(got) != len(want) {
				t.Fatalf("len: got %d want %d", len(got), len(want))
			}
			for i := range got {
				if got[i].Content != want[i].Content || got[i].Index != want[i].Index {
					t.Fatalf("chunk %d content/index: got %q/%d want %q/%d", i, got[i].Content, got[i].Index, want[i].Content, want[i].Index)
				}
				if got[i].Metadata["language"] != want[i].Metadata["language"] {
					t.Fatalf("chunk %d language: got %v want %v", i, got[i].Metadata["language"], want[i].Metadata["language"])
				}
			}
		})
	}
}

func BenchmarkSplitByCharSuffix(b *testing.B) {
	// ~300 chars so the O(n^2) full-suffix scan is visible.
	text := strings.Repeat("hello world ", 10) + strings.Repeat("中文字符内容重复", 30)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		runSplitByChar(text)
	}
}

// The en-then-cjk adversarial fixture must actually cross the 0.3 threshold:
// early chunks "en", late chunks "zh". This guards against a fixture that
// silently stops exercising the per-suffix language flip.
func TestSplitByCharLanguageFlipFixture(t *testing.T) {
	got := splitByCharReference("abcdefghijklmnop" + "中文测试文本")
	sawEN, sawZH := false, false
	prev := ""
	for _, c := range got {
		lang := c.Metadata["language"]
		if lang != prev {
			t.Logf("flip at %q: %s", c.Content, lang)
		}
		if lang == "en" {
			sawEN = true
		}
		if lang == "zh" {
			sawZH = true
		}
		prev = lang.(string)
	}
	if !sawEN || !sawZH {
		t.Fatalf("fixture must produce both en and zh chunks; sawEN=%v sawZH=%v", sawEN, sawZH)
	}
}
