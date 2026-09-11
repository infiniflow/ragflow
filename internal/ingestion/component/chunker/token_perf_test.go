package chunker

import (
	"strings"
	"testing"
)

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
