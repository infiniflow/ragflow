//go:build manual

package tool

import (
	"path/filepath"
	"testing"

	"ragflow/internal/common"
)

// TestBatchCompareWithPython compares Go output against Python reference
// across 4 dimensions (text, tables, DLA, TSR raw).  It is read-only —
// no generation, no CGO/DeepDoc dependency.  Use PY_OCR_SUFFIX to override
// the Python variant.
func TestBatchCompareWithPython(t *testing.T) {
	prevLogger, prevSugar := common.Logger, common.Sugar
	var prevLevel string
	if common.Logger != nil {
		prevLevel = common.GetLogLevel()
	}
	t.Cleanup(func() {
		common.Logger = prevLogger
		common.Sugar = prevSugar
		if prevLogger != nil {
			_ = common.SetLogLevel(prevLevel)
		}
	})

	level := "info"
	switch common.GetEnv(common.EnvBatchLogLevel) {
	case "debug":
		level = "debug"
	case "warn":
		level = "warn"
	}
	if err := common.InitLogger(level, common.FileOutput{}, ""); err != nil {
		t.Fatalf("init logger: %v", err)
	}

	goVariant := "ocr"
	pyVariant := common.GetEnv(common.EnvPYOCRSuffix)
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
