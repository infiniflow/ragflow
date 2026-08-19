//go:build cgo && manual

package pdf

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ragflow/internal/common"
	lyt "ragflow/internal/deepdoc/parser/pdf/layout"
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
//   - table PDF, gridSim==100% but textSim<100% -> GRID_OK: cell content
//     matches Python, the residual gap is the HTML serialization layer only.
//     Reported separately (not labeled PASS) and not counted as a content
//     failure; classified as go_intentional/go_bug via known_diffs.json.
//   - table PDF, gridSim<100% (or non-table textSim<100%) -> FAIL: Go assembly
//     content genuinely diverges from Python.
func TestPipelineParity(t *testing.T) {
	charspyDir := filepath.Join("testdata", "charspy")
	pyTextDir := filepath.Join("testdata", "output", "py", "ocr", "text")
	dlaDir := filepath.Join("testdata", "output", "py", "ocr", "dla")
	tsrDir := filepath.Join("testdata", "output", "py", "ocr", "tsr_raw")
	ocrDir := filepath.Join("testdata", "output", "py", "ocr", "ocr")
	tablesDir := filepath.Join("testdata", "output", "py", "ocr", "tables")

	entries, err := os.ReadDir(charspyDir)
	if err != nil {
		t.Skipf("charspy/ not found: %v", err)
	}

	filter := common.GetEnv(common.EnvBatchParityFilter)

	// TSR-replay table PDFs that must hold full grid content parity — guards
	// the replay TableIndex json tag / per-page index mapping / cumulative-Y
	// handling against regressions. table_rotation_test.pdf is exempted: its
	// gridSim<100% gap is the table segmentation divergence documented as
	// go_intentional (rule table-rotation-split-vs-merged-grid in
	// known_diffs.json), where Go's split matches the physical PDF and
	// Python's merged 8x7 grid is a segmentation defect.
	lockedGridPDFs := map[string]bool{
		"13_crosspage_table.pdf":        true,
		"14_text_table_interleaved.pdf": true,
		"06_table_content.pdf":          true,
		"18_table_caption.pdf":          true,
	}

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

	// Phase 2: replay Python's DLA + TSR intermediates through Go's assembly
	// so tables are built from Python's inference, not a mock. The factory is
	// keyed on the analyzer type, so non-replay parses are unaffected.
	RegisterReplayTableBuilder()

	total, passed, htmlDivergent, failed := 0, 0, 0, 0
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
			dumpDir := filepath.Join("testdata", "output", "go", "ocr", "text")
			_ = os.MkdirAll(dumpDir, 0o755)
			_ = os.WriteFile(filepath.Join(dumpDir, name+".txt"), []byte(goText.String()), 0o644)
		}

		// Compare
		sim := tool.CharSimilarity(goText.String(), tool.StripMeta(string(pyData)))

		// Phase 2: tables are built from Python's replayed TSR, so compare the
		// reconstructed grid content (cell text) rather than the rendered HTML
		// (which differs in whitespace/join/header markup). Grid content parity
		// isolates Go's GroupCells + FillCellTextFromBoxes assembly; HTML-only
		// differences are expected and not a Go logic bug.
		pyRows, pyHasTables := loadPythonTables(filepath.Join(tablesDir, name+".json"))
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
			// Table PDFs not textSim=100% are judged on cell content first.
			// gridSim==100% means Go's GroupCells + FillCellTextFromBoxes
			// assembly matches Python's grid; the residual textSim gap is the
			// HTML serialization layer (whitespace/join/header markup). That is
			// a real divergence but HTML-only, so it is reported as GRID_OK —
			// never mislabeled PASS — and classified via known_diffs.json.
			gridSim := tool.CharSimilarity(joinGrid(goRows), joinGrid(pyRows))
			if gridSim >= 100.0 {
				htmlDivergent++
				status = "GRID_OK"
				detail = fmt.Sprintf("gridSim=%.1f%% textSim=%.1f%% (HTML-format divergence)", gridSim, sim)
			} else if intentionalPDF(name) {
				// go_intentional rule covers this PDF: the gridSim gap is a
				// documented, deliberate divergence (Go at least as good as
				// Python, e.g. table segmentation), so it is reported as
				// INTENTIONAL and not counted as a content failure. It is
				// still logged so a reader can see the gap.
				status = "INTENTIONAL"
				detail = fmt.Sprintf("gridSim=%.1f%% textSim=%.1f%% (go_intentional: %s)", gridSim, sim, intentionalRuleID(knownDiffRules, name))
			} else {
				failed++
				status = "FAIL"
				detail = fmt.Sprintf("gridSim=%.1f%% textSim=%.1f%% (table grid content differs)", gridSim, sim)
			}
			// Regression lock: TSR-replay table PDFs that reached full grid
			// content parity must stay there (protects against regressions in
			// the replay index/Y mapping).
			if lockedGridPDFs[name] && gridSim < 100.0 {
				t.Errorf("LOCKED %s: gridSim=%.1f%% regressed below 100%%", name, gridSim)
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
	t.Logf("Pipeline parity: aligned=%d html-divergent=%d failed=%d total=%d", passed, htmlDivergent, failed, total)
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
func loadPythonTables(jsonPath string) ([][]string, bool) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, false
	}
	var td pyTableDump
	if err := json.Unmarshal(data, &td); err != nil {
		return nil, false
	}
	var rows [][]string
	for _, r := range td.Results {
		rows = append(rows, r.Rows...)
	}
	return rows, len(rows) > 0
}

// goTableRows flattens every Go result table's grid (cell text) into rows.
func goTableRows(result *pdf.ParseResult) [][]string {
	var rows [][]string
	for _, t := range result.Tables {
		for _, r := range t.Grid {
			row := make([]string, len(r))
			for j, c := range r {
				row[j] = strings.TrimSpace(c.Text)
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

// TestVMWhitespaceGapBridge reproduces the exact RAG PDF divergence
// with synthetic boxes.  A whitespace box (width > 0, gap just below
// threshold) gets merged into a content box, extending its bottom by
// the whitespace height.  This flips the next gap from reject to merge,
// creating a cascade that reduces the section count by 1.
//
// Go's whitespace pre-filter removes this box before VM, so the
// bottom extension never happens and the cascade fails to start.
func TestVMWhitespaceGapBridge(t *testing.T) {
	// Coordinates extracted from RAG PDF charspy data, "服务体系" region.
	boxes := []pdf.TextBox{
		// Content A: merged result of 3 preceding lines
		{X0: 37.6, X1: 491.0, Top: 339.35, Bottom: 382.39,
			Text: "生成文本再用standard分词建立索引", PageNumber: 1},
		// Whitespace: U+00A0 non-breaking space, has non-zero width
		{X0: 37.6, X1: 40.3, Top: 396.39, Bottom: 406.79,
			Text: " ", PageNumber: 1},
		// Content B: would be rejected without whitespace gap bridge
		{X0: 37.6, X1: 543.3, Top: 420.16, Bottom: 431.19,
			Text: "直接用rag分词建立索引", PageNumber: 1},
		// Content C: cascades after B merges
		{X0: 37.6, X1: 526.4, Top: 436.16, Bottom: 447.20,
			Text: "是在原文中并没有这样的文字", PageNumber: 1},
	}

	mh := 9.361 // RAG PDF char median
	thr := mh * 1.5

	// Run VM with whitespace PRESENT (Python-like, no pre-filter).
	// Python's while/pop merges whitespace at b_ position into b
	// (extending b.bottom), then compares same b against next content.
	// We simulate this by letting whitespace through gap/xov checks
	// and absorbing it into prev when the checks pass.
	vWithWS := func() int {
		bxs := make([]pdf.TextBox, len(boxes))
		copy(bxs, boxes)
		sort.Slice(bxs, func(i, j int) bool {
			if bxs[i].Top != bxs[j].Top {
				return bxs[i].Top < bxs[j].Top
			}
			return bxs[i].X0 < bxs[j].X0
		})
		out := make([]pdf.TextBox, 0, len(bxs))
		for i := 0; i < len(bxs); i++ {
			b := bxs[i]
			isWS := strings.TrimSpace(b.Text) == ""
			// Whitespace in b position (current box): pop (skip).
			// In Python: bxs.pop(i); continue; i stays.
			if isWS && len(out) == 0 {
				continue // nothing to extend
			}
			if isWS && len(out) > 0 {
				prev := &out[len(out)-1]
				gap := b.Top - prev.Bottom
				ov := util.OverlapX(prev, &b)
				// Python: gap passes AND xov passes → whitespace merged
				// into prev, extending bottom.  i advances (Go for-loop).
				if gap <= thr && ov >= 0.3 {
					prev.Bottom = b.Bottom
				}
				continue
			}
			if len(out) == 0 {
				out = append(out, b)
				continue
			}
			prev := &out[len(out)-1]
			if prev.LayoutNo != b.LayoutNo {
				out = append(out, b)
				continue
			}
			gap := b.Top - prev.Bottom
			ov := util.OverlapX(prev, &b)
			if gap > thr {
				out = append(out, b)
				continue
			}
			if ov < 0.3 {
				out = append(out, b)
				continue
			}
			pt := strings.TrimSpace(prev.Text)
			bt := strings.TrimSpace(b.Text)
			prev.Text = strings.TrimSpace(strings.TrimRight(pt, " \t") + " " + strings.TrimLeft(bt, " \t"))
			prev.Bottom = b.Bottom
			if prev.X0 > b.X0 {
				prev.X0 = b.X0
			}
			if prev.X1 < b.X1 {
				prev.X1 = b.X1
			}
		}
		return len(out)
	}

	// Run VM with whitespace PRE-FILTERED (Go current behavior).
	vNoWS := func() int {
		bxs := make([]pdf.TextBox, 0, len(boxes))
		for _, b := range boxes {
			if strings.TrimSpace(b.Text) != "" {
				bxs = append(bxs, b)
			}
		}
		sort.Slice(bxs, func(i, j int) bool {
			if bxs[i].Top != bxs[j].Top {
				return bxs[i].Top < bxs[j].Top
			}
			return bxs[i].X0 < bxs[j].X0
		})
		out := make([]pdf.TextBox, 0, len(bxs))
		for i := 0; i < len(bxs); i++ {
			b := bxs[i]
			if len(out) == 0 {
				out = append(out, b)
				continue
			}
			prev := &out[len(out)-1]
			if prev.LayoutNo != b.LayoutNo {
				out = append(out, b)
				continue
			}
			gap := b.Top - prev.Bottom
			ov := util.OverlapX(prev, &b)
			if gap > thr {
				out = append(out, b)
				continue
			}
			if ov < 0.3 {
				out = append(out, b)
				continue
			}
			pt := strings.TrimSpace(prev.Text)
			bt := strings.TrimSpace(b.Text)
			prev.Text = strings.TrimSpace(strings.TrimRight(pt, " \t") + " " + strings.TrimLeft(bt, " \t"))
			prev.Bottom = b.Bottom
			if prev.X0 > b.X0 {
				prev.X0 = b.X0
			}
			if prev.X1 < b.X1 {
				prev.X1 = b.X1
			}
		}
		return len(out)
	}

	nWS := vWithWS()
	nNoWS := vNoWS()
	t.Logf("With whitespace (Python-like): %d sections", nWS)
	t.Logf("Without whitespace (Go pre-filter): %d sections", nNoWS)
	t.Logf("Gap without bridge: 420.16 - 382.39 = %.2f > %.2f = REJECT", 420.16-382.39, thr)
	t.Logf("Gap with bridge:    420.16 - 406.79 = %.2f < %.2f = MERGE", 420.16-406.79, thr)

	// The manual vWithWS (Python-like) and vNoWS (old Go pre-filter) still
	// differ — the mechanism is real.  But production lyt.NaiveVerticalMerge now
	// handles whitespace inline (gap bridge), matching Python.
	if nWS == nNoWS {
		t.Error("Manual implementations should differ — the gap bridge mechanism is real")
	}

	// Verify production lyt.NaiveVerticalMerge matches vWithWS (Python behavior).
	mhMap := map[int]float64{1: mh}
	mwMap := map[int]float64{1: 5}
	vmResult := lyt.NaiveVerticalMerge(boxes, mhMap, mwMap, nil)
	t.Logf("lyt.NaiveVerticalMerge (production): %d sections", len(vmResult))
	if len(vmResult) != nWS {
		t.Errorf("lyt.NaiveVerticalMerge produced %d sections, want %d (Python-like with gap bridge)", len(vmResult), nWS)
	}
}
