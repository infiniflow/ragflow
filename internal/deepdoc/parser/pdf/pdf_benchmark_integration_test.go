//go:build cgo && integration

package pdf

import (
	"os"
	"runtime"
	"syscall"
	"testing"

	"ragflow/internal/deepdoc/native"
	infnative "ragflow/internal/deepdoc/parser/pdf/inference/native_analyzer"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func BenchmarkNativePDFParse(b *testing.B) {
	path := os.Getenv("DEEPDOC_BENCH_PDF")
	if path == "" {
		b.Skip("set DEEPDOC_BENCH_PDF")
	}
	modelDir := os.Getenv("DEEPDOC_MODEL_DIR")
	if modelDir == "" {
		b.Skip("set DEEPDOC_MODEL_DIR")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	if err := native.InitORT(); err != nil {
		b.Fatal(err)
	}
	analyzer, err := infnative.NewAnalyzer(modelDir, infnative.DefaultDropScore)
	if err != nil {
		b.Fatal(err)
	}
	parser := NewParser(pdf.DefaultParserConfig())

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for range b.N {
		result, err := parser.Parse(b.Context(), data, analyzer)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Sections) == 0 {
			result.Close()
			b.Fatal("parser returned no sections")
		}
		result.Close()
	}
	b.StopTimer()
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		b.Fatal(err)
	}
	maxrss := float64(usage.Maxrss)
	if runtime.GOOS == "darwin" {
		maxrss /= 1024
	}
	b.ReportMetric(maxrss/1024, "peak-RSS-MiB")
}
