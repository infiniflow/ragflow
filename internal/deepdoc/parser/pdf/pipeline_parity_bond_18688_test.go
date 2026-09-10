//go:build cgo && manual

package pdf

import (
	"path/filepath"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/deepdoc/parser/pdf/tool"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// bondName is the ocr_real PDF whose two-page parameter table regressed under
// PR #18688. The table spans page 0 (table_index 0) and page 1 (table_index 1)
// and must be merged into ONE 6-column table matching Python's 51-row golden.
const bondName = "2.中加纯债两年定期开放债券型证券投资基金_参数表（用印版）.pdf"

// replayPipelineGridVariant runs Go's replay pipeline for a custom dataset
// variant (e.g. "ocr_real") and returns the full ParseResult so callers can
// inspect the reconstructed table grids (result.Tables) and compute grid
// structure similarity against Python's golden. It mirrors the per-PDF setup
// of TestPipelineParity Phase 2 for non-default datasets.
func replayPipelineGridVariant(t *testing.T, variant, name string) *pdf.ParseResult {
	t.Helper()
	dirs := tool.ParityDirsFor(variant)
	engine, err := tool.LoadPythonChars(filepath.Join(dirs.Charspy, name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if v := engine.IsEnglish(); v != nil && *v {
		engine.ClearChars()
	} else if pages, _ := engine.PageCount(); util.DetectEnglish(engine.PageChars(), pages, nil) {
		engine.ClearChars()
	}
	RegisterReplayTableBuilder()
	cfg := pdf.DefaultParserConfig()
	cfg.SortByTop = true
	analyzer := NewPythonIntermediateDocAnalyzer(name, dirs.DLA, dirs.TSRRaw, dirs.OCR, engine.PageDims())
	p := NewParser(cfg)
	result, err := p.ParseRaw(t.Context(), engine, analyzer)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// TestPipelineParityBondCrossPageMerge18688 is the TDD guard for the
// regression introduced by PR #18688 (fix(deepdoc): prevent cross-page table
// over-merge from page-local Y coordinates).
//
// Root cause: in table/table_merge.go MergeTablesAcrossPages, #18688 added an
// unconditional `yDis += anchorPageH` (page-absolute Y shift) to the
// cross-page proximity test, intending to stop icbccs-style over-merges where
// two tables merely repeat their page-local Y every page. But for this bond
// PDF the two pages ARE a genuine continuation, and the page-absolute shift
// pushes yDis above the `mh*23` threshold so the merge is wrongly skipped
// (`continue`). Go then emits two separate tables (page 0 = 23x5, page 1 =
// 23x6) instead of one merged 6-column table, diverging from Python's single
// 51-row / 6-column table: structSim drops from ~89.4% (pre-#18688) to 44.7%
// (current HEAD).
//
// This test pins the EXPECTED behavior: the two-page table must be merged into
// a single table whose structure matches Python (structSim >= 89%). It FAILS at
// current HEAD (44.7%), exposing the regression, and passes once the
// over-aggressive `yDis += anchorPageH` is corrected so legitimate
// cross-page continuations still merge.
//
// Requires BATCH_PARITY_DATA_ROOT (the shared ocr_real dump dir); skips
// otherwise. Run via build.sh --test-manual with BATCH_PARITY_VARIANT=ocr_real
// and BATCH_PARITY_DATA_ROOT set.
func TestPipelineParityBondCrossPageMerge18688(t *testing.T) {
	if common.GetEnv(common.EnvBatchParityDataRoot) == "" {
		t.Skip("BATCH_PARITY_DATA_ROOT not set; ocr_real dumps unavailable")
	}
	t.Setenv(common.EnvBatchParityVariant, "ocr_real")
	dirs := tool.ParityDirsFor("ocr_real")

	result := replayPipelineGridVariant(t, "ocr_real", bondName)

	// Diagnostic: the merged (or split) table count directly shows whether
	// the cross-page merge fired. The bond PDF is ONE logical table spanning
	// two pages, so a correct merge yields exactly one TableItem.
	t.Logf("DIAG result.Tables=%d (expected 1 after cross-page merge)", len(result.Tables))

	pyRows, pyHasTables := loadPythonTables(t, filepath.Join(dirs.Tables, bondName+".json"))
	goRows := goTableRows(result)
	gridSim := tool.CharSimilarity(joinGrid(goRows), joinGrid(pyRows))
	structureSim, shape := gridStructureSimilarity(goRows, pyRows)

	// The regression is a structural (column-count) divergence, not cell-text
	// loss: gridSim stays ~100% while structSim collapses. Pin structSim.
	if structureSim < 89.0 {
		t.Errorf("REGRESSION (#18688 cross-page over-reject): bond PDF structSim=%.1f%% (<89%%); "+
			"Python merges the two-page parameter table into one 6-column table (51 rows) but Go leaves it split "+
			"(structSim 44.7%% at HEAD). shape=%s gridSim=%.1f%% pyTables=%v goTables=%d",
			structureSim, shape, gridSim, pyHasTables, len(result.Tables))
	}
}
