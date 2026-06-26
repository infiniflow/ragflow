//go:build manual

package tool

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// TestBatchCompareWithPython compares Go output against Python reference
// across 4 dimensions (text, tables, DLA, TSR raw).  It is read-only —
// no generation, no CGO/DeepDoc dependency.  Use BATCH_SKIP_OCR=1 to
// compare the noocr variant; PY_OCR_SUFFIX to override the Python variant.
func TestBatchCompareWithPython(t *testing.T) {
	level := slog.LevelInfo
	if os.Getenv("BATCH_LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	if os.Getenv("BATCH_LOG_LEVEL") == "warn" {
		level = slog.LevelWarn
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	goVariant := "ocr"
	if os.Getenv("BATCH_SKIP_OCR") == "1" {
		goVariant = "noocr"
	}
	pyVariant := os.Getenv("PY_OCR_SUFFIX")
	if pyVariant == "" {
		pyVariant = goVariant
	}
	goTextDir := filepath.Join("testdata", "output", "go", goVariant, "text")
	pyTextDir := filepath.Join("testdata", "output", "py", pyVariant, "text")

	// Read Go text files' #@meta (no aggregate JSON dependency).
	goResults, err := ReadGoTextMeta(goTextDir)
	if err != nil || len(goResults) == 0 {
		t.Fatalf("No Go text files in %s: %v", goTextDir, err)
	}

	// Read Python text files' #@meta
	pyResults, err := ReadPythonTextMeta(pyTextDir)
	if err != nil || len(pyResults) == 0 {
		t.Fatalf("No Python text files in %s: %v", pyTextDir, err)
	}

	t.Logf("Comparing %d Go × %d Python", len(goResults), len(pyResults))
	CompareWithPython(t, goResults, pyResults, goTextDir, pyTextDir)

	// Compare tables.
	goTablesDir := filepath.Join("testdata", "output", "go", goVariant, "tables")
	pyTablesDir2 := filepath.Join("testdata", "output", "py", pyVariant, "tables")
	CompareTablesWithPython(t, goTablesDir, pyTablesDir2)
	// Compare DLA + TSR raw intermediates.
	goDLADir := filepath.Join("testdata", "output", "go", goVariant, "dla")
	pyDLADir := filepath.Join("testdata", "output", "py", pyVariant, "dla")
	CompareDLAWithPython(t, goDLADir, pyDLADir)
	goTSRRawDir := filepath.Join("testdata", "output", "go", goVariant, "tsr_raw")
	pyTSRRawDir := filepath.Join("testdata", "output", "py", pyVariant, "tsr_raw")
	CompareTSRRawWithPython(t, goTSRRawDir, pyTSRRawDir)
}
