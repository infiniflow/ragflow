//go:build cgo && manual

package pdf

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/deepdoc/parser/pdf/tool"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// TestPipelineParity verifies Go pipeline logic equivalence with Python.
// It loads Python pdfplumber chars (from charspy/), replays Python's DLA +
// TSR intermediates (output/py/ocr/{dla,tsr_raw}) through Go's assembly, and
// compares the resulting sections against Python's output/py/ocr/text/.
//
// Phase 1 (char→section assembly) expects CharSim 100%. Phase 2 (DLA/TSR
// replay) also builds tables from Python's inference; table section HTML may
// differ in whitespace/join formatting, so table PDFs are additionally
// checked by grid (rows) against output/py/ocr/tables/. Phase 3 (OCR replay)
// feeds Python's raw per-page OCR detect results so image-only PDFs get their
// text from the same inference Python used. A PDF without an OCR dump (the
// dump script has not been run with an OCR service available) falls back to
// the char-derived path exactly as before.
//
// Result classification (both sims are always reported):
//   - textSim==100% -> PASS (fully aligned, includes table HTML byte-identical)
//   - table PDF, gridSim==100% AND structSim==100% but textSim<100% ->
//     NONCELL_TEXT: cell content AND structure match Python, the residual gap
//     is non-cell-text HTML-emitter formatting (e.g. header <th> vs <td>,
//     whitespace/newline) — not cell content. In other PDFs it may also be a
//     caption/body paragraph Go omits. Reported separately (not labeled PASS)
//     and not counted as a
//     content failure; classified as go_intentional/go_bug via known_diffs.json.
//   - table PDF, gridSim<100% OR structSim<100% (or non-table textSim<100%) ->
//     FAIL if not exempted; INTENTIONAL if a go_intentional rule names this
//     exact PDF (Go judged at least as good as Python, e.g. table
//     segmentation); IGNORE if an ignore rule names it (neither Go nor Python
//     is the ground truth for this PDF — both sides produce an inaccurate
//     grid, so the gap is not a regression target on either side). Any genuine
//     Go content regression that is not covered by an ignore/go_intentional
//     rule is a FAIL.
func TestPipelineParity(t *testing.T) {
	// Dataset variant: "" (default) or "ocr" resolve to the built-in layout
	// (charspy/, output/py/ocr/...) for the original 35-PDF fixture set; a
	// custom variant such as "ocr_real" reads its own isolated directories
	// (charspy_ocr_real/, output/py/ocr_real/...), so a second dataset (e.g.
	// real_pdfs/) gets its own dumps and verdicts without touching the default
	// set. See tool.ParityDirsFor.
	dirs := tool.ParityDirsFor(common.GetEnv(common.EnvBatchParityVariant))
	charspyDir, pyTextDir := dirs.Charspy, dirs.Text
	dlaDir, tsrDir, ocrDir, tablesDir := dirs.DLA, dirs.TSRRaw, dirs.OCR, dirs.Tables

	entries, err := os.ReadDir(charspyDir)
	if err != nil {
		t.Skipf("charspy/ not found: %v", err)
	}

	filter := common.GetEnv(common.EnvBatchParityFilter)

	// TSR-replay table PDFs that must hold full grid content + structure
	// parity — guards the replay TableIndex json tag / per-page index mapping
	// / cumulative-Y handling / segmentation against regressions.
	// table_rotation_test.pdf is exempted (not in the lock set): its
	// STRUCTURAL divergence (Go 3x6 split vs Python 8x7 merge, structSim<100%
	// though gridSim=100% because cell text is identical) is the table
	// segmentation divergence documented as go_intentional (rule
	// table-rotation-split-vs-merged-grid in known_diffs.json), where Go's
	// split matches the physical PDF and Python's merged grid is a defect.
	lockedGridPDFs := parityLockedGridPDFs()

	// go_intentional rules exempt their applies_to PDFs from the FAIL count:
	// a documented, deliberate divergence where Go is judged at least as good
	// as Python (registry in table/testdata/parity/known_diffs.json). Only a
	// rule that names this exact PDF (no glob, e.g.
	// table-rotation-split-vs-merged-grid) may exempt it; a wildcard rule
	// like table-cell-fill-filled-threshold-0.85 documents a global threshold
	// divergence and must NOT mask a specific PDF's gridSim gap (13/14 stay
	// locked FAILs).
	knownDiffRules := loadPdfKnownDiffs(t)
	intentionalPDF := func(name string) bool {
		for _, r := range knownDiffRules {
			if r.Tag == "go_intentional" && r.matchesExactly(name) {
				return true
			}
		}
		return false
	}
	// ignorePDF reports whether a known_diffs rule marks this PDF as ignore:
	// neither Go nor Python is the ground truth (both are inaccurate), so the
	// divergence is not a regression target on either side and the PDF is
	// exempted from the FAIL count (reported as IGNORE).
	ignorePDF := func(name string) bool {
		for _, r := range knownDiffRules {
			if r.Tag == "ignore" && r.matchesExactly(name) {
				return true
			}
		}
		return false
	}

	// Phase 2: replay Python's DLA + TSR intermediates through Go's assembly
	// so tables are built from Python's inference, not a mock. The factory is
	// keyed on the analyzer type, so non-replay parses are unaffected.
	RegisterReplayTableBuilder()

	total, passed, noncellText, intentional, ignored, failed := 0, 0, 0, 0, 0, 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		if filter != "" && !strings.Contains(e.Name(), filter) {
			continue
		}

		// Load Python chars
		jsonPath := filepath.Join(charspyDir, e.Name())
		engine, err := tool.LoadPythonChars(jsonPath)
		if err != nil {
			t.Errorf("%s: tool.LoadPythonChars: %v", name, err)
			continue
		}

		// English documents: Python (is_english -> chars=[]) consumes pure OCR
		// output, not pdfplumber chars. Mirror that so both sides replay the
		// same input — clear the replayed chars and let processPageBoxes fall
		// through to the OCR-replay path (Phase 3 OCR dump). dims are kept so
		// the placeholder page image still carries the correct size.
		//
		// Use the is_english verdict captured from the SAME Python run that
		// produced the text golden (stable); fall back to DetectEnglish only
		// for older charspy dumps that omit it.
		isEnglish := false
		if v := engine.IsEnglish(); v != nil {
			isEnglish = *v
		} else if pages, _ := engine.PageCount(); util.DetectEnglish(engine.PageChars(), pages, nil) {
			isEnglish = true
		}
		if isEnglish {
			engine.ClearChars()
		}

		// Run Go pipeline with Python's DLA/TSR/OCR replayed.
		cfg := pdf.DefaultParserConfig()
		cfg.SortByTop = true
		analyzer := NewPythonIntermediateDocAnalyzer(name, dlaDir, tsrDir, ocrDir, engine.PageDims())
		p := NewParser(cfg)
		result, err := p.ParseRaw(t.Context(), engine, analyzer)
		if err != nil {
			t.Errorf("%s: Parse: %v", name, err)
			continue
		}

		// Production path now derives per-char R/C itself (AnnotateBoxesWithGrid
		// + GroupBoxesByRC); the replay harness measures that production output
		// directly against Python's golden.

		// Read Python sections
		pyPath := filepath.Join(pyTextDir, name+".txt")
		pyData, err := os.ReadFile(pyPath)
		if err != nil {
			t.Logf("%s: no Python reference at %s — skip", name, pyPath)
			continue
		}

		// Build Go text
		var goText strings.Builder
		for _, s := range result.Sections {
			goText.WriteString(s.Text)
			goText.WriteByte('\n')
		}

		// Debug aid: BATCH_PARITY_DUMP_GO=1 writes the Go output text next to
		// the Python golden so failing PDFs can be diffed case by case.
		if common.GetEnv("BATCH_PARITY_DUMP_GO") != "" {
			dumpDir := dirs.GoText
			_ = os.MkdirAll(dumpDir, 0o755)
			_ = os.WriteFile(filepath.Join(dumpDir, name+".txt"), []byte(goText.String()), 0o644)
		}

		// Compare
		sim := tool.CharSimilarity(goText.String(), tool.StripMeta(string(pyData)))

		// Phase 2: tables are built from Python's replayed TSR, so compare the
		// reconstructed grid content (cell text) rather than the rendered HTML.
		// Grid content parity (gridSim + structSim) isolates Go's GroupCells +
		// FillCellTextFromBoxes assembly. When gridSim AND structSim are 100%
		// but textSim<100%, the residual gap is NON-CELL-TEXT: HTML-emitter
		// formatting (header <th> vs <td>, whitespace/newline) — none of which is
		// a table-cell-assembly bug. (A caption/body paragraph Go omits is also
		// possible in other PDFs.)
		pyRows, pyHasTables := loadPythonTables(t, filepath.Join(tablesDir, name+".json"))
		goRows := goTableRows(result)
		goHasTables := len(goRows) > 0

		total++
		status, detail := "", ""
		switch {
		case sim >= 100.0:
			// True full-output alignment (text, or table HTML byte-identical).
			passed++
			status = "PASS"
			detail = fmt.Sprintf("textSim=%.1f%%", sim)
		case pyHasTables || goHasTables:
			// Table PDFs not textSim=100% are judged on cell content AND
			// structure. gridSim (CharSimilarity over joined cell text) is
			// order- and shape-blind, so a structural divergence (e.g. Go
			// splitting a rotated table into 3x6 while Python merges it into
			// 8x7) reads as gridSim=100% even though the grids are not the
			// same table. gridStructureSimilarity is SHAPE-AWARE: it compares
			// row/column structure ignoring cell text, so it catches exactly
			// that class of divergence.
			gridSim := tool.CharSimilarity(joinGrid(goRows), joinGrid(pyRows))
			structureSim, shapeDetail := gridStructureSimilarity(goRows, pyRows)
			// Full content parity = identical cell text AND identical
			// structure. Only then is the residual textSim gap purely the
			// non-cell-text layer (HTML-emitter formatting outside table cells, e.g.
			// reported as NONCELL_TEXT.
			contentMatch := gridSim >= 100.0 && structureSim >= 100.0
			if contentMatch {
				// Cells + structure match Python, but the full text still
				// diverges — the gap is OUTSIDE table cells: emitter
				// markup/whitespace OR an omitted caption/body paragraph
				// (not a table-cell-assembly bug). Label it noncell-text
				// (not "html-divergent", which wrongly implies content
				// matches) so the divergence is named and not hidden.
				noncellText++
				status = "NONCELL_TEXT"
				detail = fmt.Sprintf("gridSim=%.1f%% structSim=%.1f%% (%s) textSim=%.1f%% (non-cell-text divergence outside table-cell content: emitter markup/whitespace or omitted caption/body text)", gridSim, structureSim, shapeDetail, sim)
			} else if intentionalPDF(name) {
				// go_intentional rule covers this PDF: the divergence (cell
				// content and/or structure) is a documented, deliberate one
				// where Go is judged at least as good as Python (e.g. table
				// segmentation). Reported as INTENTIONAL and not counted as a
				// content failure. Still logged so a reader can see the gap.
				intentional++
				status = "INTENTIONAL"
				detail = fmt.Sprintf("gridSim=%.1f%% structSim=%.1f%% (%s) textSim=%.1f%% (go_intentional: %s)", gridSim, structureSim, shapeDetail, sim, intentionalRuleID(knownDiffRules, name))
			} else if ignorePDF(name) {
				// ignore rule covers this PDF: neither Go nor Python is the
				// ground truth — both produce an inaccurate grid, so the gap
				// is not a regression target on either side. Reported as
				// IGNORE and not counted as a content failure; still logged
				// so the gap remains visible.
				ignored++
				status = "IGNORE"
				detail = fmt.Sprintf("gridSim=%.1f%% structSim=%.1f%% (%s) textSim=%.1f%% (ignore: %s)", gridSim, structureSim, shapeDetail, sim, ignoreRuleID(knownDiffRules, name))
			} else {
				failed++
				status = "FAIL"
				detail = fmt.Sprintf("gridSim=%.1f%% structSim=%.1f%% (%s) textSim=%.1f%% (table grid content/structure differs)", gridSim, structureSim, shapeDetail, sim)
			}
			// Regression guard: a go_intentional PDF whose divergence is
			// STRUCTURAL (Go's table segmentation differs from Python's) must
			// surface as INTENTIONAL, never be hidden under gridSim=100% as
			// html-divergent. This is what caught table_rotation_test.pdf (Go
			// 3x6 split vs Python 8x7 merge) being mislabeled before the
			// structure metric existed.
			if intentionalPDF(name) && structureSim < 100.0 && status != "INTENTIONAL" {
				t.Errorf("REGRESSION %s: go_intentional structural divergence misclassified %s (must be INTENTIONAL)", name, status)
			}
			// Regression lock: TSR-replay table PDFs that reached full grid
			// content + structure parity must stay there (protects against
			// regressions in the replay index/Y mapping or segmentation).
			if lockedGridPDFs[name] && (gridSim < 100.0 || structureSim < 100.0) {
				t.Errorf("LOCKED %s: gridSim=%.1f%% structSim=%.1f%% regressed below 100%%", name, gridSim, structureSim)
			}
		default:
			failed++
			status = "FAIL"
			detail = fmt.Sprintf("textSim=%.1f%% (must be 100%%)", sim)
		}
		t.Logf("%s %s: %s tables:%d boxes:%d->%d->%d->%d",
			status, name, detail, len(goRows), result.Metrics.BoxesInitial, result.Metrics.BoxesTextMerge, result.Metrics.BoxesVertMerge, len(result.Sections))
	}

	if total == 0 {
		t.Skip("no charspy/ files found")
	}
	t.Logf("Pipeline parity: aligned=%d noncell-text=%d intentional=%d ignored=%d failed=%d total=%d", passed, noncellText, intentional, ignored, failed, total)
	if failed > 0 {
		t.Errorf("%d parity failures — Go pipeline content differs from Python (grid/text)", failed)
	}
}

// ── known_diffs.json integration (same schema as chunker) ────────────────

const pdfKnownDiffsPath = "table/testdata/parity/known_diffs.json"

// pdfDiffRule is one entry in known_diffs.json: an accepted, documented
// divergence from the Python baseline. Mirrors chunker's diffRule so a PDF
// whose gridSim gap is tagged go_intentional is exempted from the FAIL count
// (reported as INTENTIONAL instead).
type pdfDiffRule struct {
	ID        string   `json:"id"`
	Tag       string   `json:"tag"`
	Kind      string   `json:"kind"`
	AppliesTo []string `json:"applies_to"`
	Fields    []string `json:"fields"`
}

func (r pdfDiffRule) matches(caseID string) bool {
	for _, pattern := range r.AppliesTo {
		if ok, err := path.Match(pattern, caseID); err == nil && ok {
			return true
		}
	}
	return false
}

// matchesExactly reports whether the rule names caseID with a literal (glob-
// free) pattern. Used for the FAIL-exemption: only a rule that explicitly
// names the PDF may waive its gridSim gap; a wildcard rule such as
// applies_to ["*"] documents a global divergence and must not mask a
// specific PDF's failure.
func (r pdfDiffRule) matchesExactly(caseID string) bool {
	for _, pattern := range r.AppliesTo {
		if strings.ContainsAny(pattern, "*?[") {
			continue
		}
		if pattern == caseID {
			return true
		}
	}
	return false
}

func loadPdfKnownDiffs(t *testing.T) []pdfDiffRule {
	t.Helper()
	raw, err := os.ReadFile(pdfKnownDiffsPath)
	if err != nil {
		t.Fatalf("read %s: %v", pdfKnownDiffsPath, err)
	}
	var registry struct {
		Version int           `json:"version"`
		Rules   []pdfDiffRule `json:"rules"`
	}
	if err := json.Unmarshal(raw, &registry); err != nil {
		t.Fatalf("parse %s: %v", pdfKnownDiffsPath, err)
	}
	return registry.Rules
}

// intentionalRuleID returns the id of the first go_intentional rule naming
// this exact PDF, for the INTENTIONAL status detail line.
func intentionalRuleID(rules []pdfDiffRule, name string) string {
	for _, r := range rules {
		if r.Tag == "go_intentional" && r.matchesExactly(name) {
			return r.ID
		}
	}
	return "unknown"
}

// ignoreRuleID returns the id of the first ignore rule naming this exact PDF,
// for the IGNORE status detail line.
func ignoreRuleID(rules []pdfDiffRule, name string) string {
	for _, r := range rules {
		if r.Tag == "ignore" && r.matchesExactly(name) {
			return r.ID
		}
	}
	return "unknown"
}

// ── table-grid parity helpers ──────────────────────────────────────────

type pyTableDump struct {
	Results []struct {
		Positions []struct {
			Page   int     `json:"page"`
			Left   float64 `json:"left"`
			Right  float64 `json:"right"`
			Top    float64 `json:"top"`
			Bottom float64 `json:"bottom"`
		} `json:"positions"`
		Rows [][]string `json:"rows"`
	} `json:"results"`
}

// loadPythonTables returns the flattened concatenation of every table's row
// grid from output/py/ocr/tables/{name}.json, plus whether any table exists.
// loadPythonTables reads Python's dumped table grid for a PDF. A missing golden
// and a corrupt golden are reported distinctly so neither silently masquerades
// as a Go content failure (both return (nil, false) so the table branch treats
// them as 'no Python tables').
func loadPythonTables(t *testing.T, jsonPath string) ([][]string, bool) {
	t.Helper()
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Errorf("%s: missing Python tables golden (environment gap, NOT a Go content fail): %v", jsonPath, err)
		return nil, false
	}
	var td pyTableDump
	if err := json.Unmarshal(data, &td); err != nil {
		t.Errorf("%s: corrupt Python tables golden (bad input, NOT a Go content fail): %v", jsonPath, err)
		return nil, false
	}
	var rows [][]string
	for _, r := range td.Results {
		rows = append(rows, r.Rows...)
	}
	return rows, len(rows) > 0
}

// goTableRows flattens every Go result table's grid (cell text) into rows.
// Cells covered by a spanning cell (marked "table covered" by ConstructTable)
// are dropped, matching Python's golden rows, which come from the rendered
// HTML and omit covered <td> entirely — so per-row column counts match
// Python's variable-width grids (e.g. 11x[3,2,3,...]).
func goTableRows(result *pdf.ParseResult) [][]string {
	var rows [][]string
	for _, t := range result.Tables {
		for _, r := range t.Grid {
			var row []string
			for _, c := range r {
				if strings.Contains(c.Label, "covered") {
					continue
				}
				row = append(row, strings.TrimSpace(c.Text))
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// joinGrid renders a grid as newline-joined, space-joined cell text so
// CharSimilarity can compare two grids' content ignoring row/cell order.
func joinGrid(grid [][]string) string {
	var b strings.Builder
	for _, row := range grid {
		b.WriteString(strings.Join(row, " "))
		b.WriteByte('\n')
	}
	return b.String()
}
