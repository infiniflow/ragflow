package chunker

import (
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
)

// hardSplitBenchText builds a realistic oversized boundary-less run: a long
// unbroken sequence without sentence delimiters, mixing CJK and ASCII like a
// dense paragraph of a scanned/OCR'd document. This is exactly the input that
// hardSplitPiece must token-split, and the reason the function exists. Sized
// so a single hardSplitPiece call takes on the order of a second under the
// reference implementation: large enough for the O(n^2/target) encode work to
// dominate the measurement, small enough that the benchmark completes quickly
// even with the un-optimized code stashed.
func hardSplitBenchText() string {
	var b strings.Builder
	const sentence = "企业知识库检索系统需要高效处理海量非结构化文档的解析切分与向量化入库RAGFlow整合了文档理解文本切分混合检索与重排等多个环节Knowledgebase ingestion splits long passages into token-bounded chunks for embedding. "
	for b.Len() < 12_000 {
		b.WriteString(sentence)
	}
	return b.String()
}

// benchHardSplit runs hardSplitPiece and reports both the elapsed time and the
// number of produced pieces, so the benchmark stays honest about output size
// (a regression that halves the pieces would mask a speedup).
func benchHardSplit(b *testing.B, text string) {
	b.Helper()
	var pieces int
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pieces = len(hardSplitPiece(text, "", 512))
	}
	b.ReportMetric(float64(pieces), "pieces/op")
}

func BenchmarkHardSplitPiece_Text(b *testing.B) {
	text := hardSplitBenchText()
	if n := tokenizeStr(text); n <= 512 {
		b.Fatalf("benchmark input only has %d tokens; it must exceed the 512-token target", n)
	}
	benchHardSplit(b, text)
}

func BenchmarkHardSplitPiece_CJK(b *testing.B) {
	text := strings.Repeat("无句读长文知识库检索切分处理效率优化", 250)
	benchHardSplit(b, text)
}

// hardSplitPieceRef is the exact pre-optimization algorithm, preserved so the
// differential test can lock the refactor to byte-identical output. It is kept
// deliberately slow (a full tokenizeStr(rest) guard per iteration) because it
// is a reference, not a runtime path.
func hardSplitPieceRef(text string, docType string, target int) []schema.ChunkDoc {
	var out []schema.ChunkDoc
	rest := text
	for tokenizeStr(rest) > target {
		head := tokenizer.TrimContentToTokenLimit(rest, target)
		if head == "" || head == rest {
			break // cannot shrink further; avoid an infinite loop
		}
		if !strings.HasPrefix(rest, head) {
			head = strings.TrimRight(head, "�")
			if head == "" {
				break
			}
		}
		cut := adjustCutPastTag(rest, len(head), target)
		if cut <= 0 || cut >= len(rest) {
			if rest != "" {
				out = append(out, schema.ChunkDoc{Text: rest, DocType: docType, TKNums: intPtr(tokenizeStr(rest)), CKType: "text"})
			}
			rest = ""
			break
		}
		head = rest[:cut]
		out = append(out, schema.ChunkDoc{Text: head, DocType: docType, TKNums: intPtr(tokenizeStr(head)), CKType: "text"})
		rest = rest[cut:]
	}
	if rest != "" {
		out = append(out, schema.ChunkDoc{Text: rest, DocType: docType, TKNums: intPtr(tokenizeStr(rest)), CKType: "text"})
	}
	return out
}

// TestHardSplitPieceMatchesReference pins the one invariant this optimization
// is allowed to preserve: the output must be byte-for-byte identical to the
// pre-optimization algorithm. It runs over a corpus that mixes plain text,
// CJK, PDF coordinate tags, emoji, and edge-case inputs, at several token
// targets. This is a stricter lock than "every piece <= target", which the
// base algorithm itself violates: cl100k is byte-level, so a token boundary
// can land mid-multibyte-char and the re-encoded continuation piece can carry
// one extra token (confirmed on the unmodified base code).
func TestHardSplitPieceMatchesReference(t *testing.T) {
	inputs := []string{
		strings.Repeat("boundary-less run of text with no sentence delimiter ", 500),
		strings.Repeat("中文中文中文无句读", 1000),
		strings.Repeat("prelude @@12,34## body @@56,78## tail ", 300),
		strings.Repeat("mid-char🎉emoji🎉run-", 400),
		strings.Repeat("a", 3000),
		"single-token",
		"",
		"\n\n\n",
	}
	for _, target := range []int{32, 64, 128, 512} {
		for _, in := range inputs {
			got := hardSplitPiece(in, "", target)
			want := hardSplitPieceRef(in, "", target)
			if len(got) != len(want) {
				t.Fatalf("target=%d in=%d bytes: piece count %d != %d", target, len(in), len(got), len(want))
			}
			var joined strings.Builder
			for i := range got {
				if got[i].Text != want[i].Text {
					t.Fatalf("target=%d in=%d bytes piece[%d]: %q != %q", target, len(in), i, got[i].Text, want[i].Text)
				}
				joined.WriteString(got[i].Text)
			}
			if joined.String() != in {
				t.Fatalf("target=%d in=%d bytes: pieces do not reproduce input (%d != %d bytes)",
					target, len(in), joined.Len(), len(in))
			}
		}
	}
}
