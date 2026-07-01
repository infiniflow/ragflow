//go:build cgo && manual

package pdf

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	inf "ragflow/internal/deepdoc/parser/pdf/inference"
	lyt "ragflow/internal/deepdoc/parser/pdf/layout"
	"ragflow/internal/deepdoc/parser/pdf/tool"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestBatchResults runs Parse() on real PDFs and writes:
//
//	output/go/{variant}/text/{pdf}.txt     — per-section text + #@meta
//	output/go/{variant}/tables/{pdf}.json  — table cells
//	output/go/{variant}/dla/{pdf}.json     — DLA regions (debug)
//	output/go/{variant}/tsr_raw/{pdf}.json — TSR raw cells (debug)
//
// DeepDoc is mandatory (DLA+TSR are inseparable from the pipeline).
//
//	BATCH_SKIP_OCR=1   skip image OCR (DLA+TSR kept)
//	BATCH_COUNT=N      limit to first N PDFs (by file size, smallest first)
//	BATCH_SINGLE=name  process exactly one PDF (full filename)
//
// For read-only comparison, see compare_test.go (no CGO needed).
func TestBatchResults(t *testing.T) {
	setupLogger()

	pdfDir := filepath.Join("testdata", "real_pdfs")
	all := listRealPDFs(t, pdfDir)

	count := countFromEnv("BATCH_COUNT", len(all))
	if single := os.Getenv("BATCH_SINGLE"); single != "" {
		all = filterSingle(all, single, t)
		count = 1
	}
	pdfs := all[:min(count, len(all))]

	ddClient, err := inf.NewClient(os.Getenv("DEEPDOC_URL"))
	if err != nil {
		t.Fatal(err)
	}
	if !ddClient.Health() {
		t.Fatalf("DeepDoc service not available at %s (DLA+TSR required)", ddClient.BaseURL())
	}
	deepDoc := pdf.DocAnalyzer(ddClient)

	variant := variantFromEnv()
	t.Logf("DeepDoc available — DLA+TSR%s enabled (%d PDFs)",
		map[bool]string{true: ", image OCR skipped", false: ", OCR enabled"}[variant == "noocr"], len(pdfs))

	dirs := mkOutputDirs(variant)

	processPDFs(t, pdfDir, pdfs, deepDoc, variant, dirs)
}

// ── helpers ─────────────────────────────────────────────────────────

func setupLogger() {
	level := slog.LevelInfo
	switch os.Getenv("BATCH_LOG_LEVEL") {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
}

func variantFromEnv() string {
	if os.Getenv("BATCH_SKIP_OCR") == "1" {
		return "noocr"
	}
	return "ocr"
}

type outputDirs struct {
	text, tables, dla, tsrRaw string
}

func mkOutputDirs(variant string) outputDirs {
	d := outputDirs{
		text:   filepath.Join("testdata", "output", "go", variant, "text"),
		tables: filepath.Join("testdata", "output", "go", variant, "tables"),
		dla:    filepath.Join("testdata", "output", "go", variant, "dla"),
		tsrRaw: filepath.Join("testdata", "output", "go", variant, "tsr_raw"),
	}
	os.MkdirAll(d.text, 0755)
	os.MkdirAll(d.tables, 0755)
	os.MkdirAll(d.dla, 0755)
	os.MkdirAll(d.tsrRaw, 0755)
	return d
}

func countFromEnv(key string, ceiling int) int {
	if s := os.Getenv(key); s != "" {
		n, err := strconv.Atoi(s)
		if err == nil && n > 0 && n < ceiling {
			return n
		}
	}
	return ceiling
}

func listRealPDFs(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var pdfs []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".pdf") {
			pdfs = append(pdfs, e.Name())
		}
	}
	// Sort by file size, smallest first — fast feedback on small PDFs.
	sort.Slice(pdfs, func(i, j int) bool {
		si, _ := os.Stat(filepath.Join(dir, pdfs[i]))
		sj, _ := os.Stat(filepath.Join(dir, pdfs[j]))
		if si == nil || sj == nil {
			return pdfs[i] < pdfs[j]
		}
		return si.Size() < sj.Size()
	})
	return pdfs
}

func filterSingle(pdfs []string, name string, t *testing.T) []string {
	t.Helper()
	for _, n := range pdfs {
		if n == name {
			return []string{n}
		}
	}
	t.Fatalf("BATCH_SINGLE: %s not found in real_pdfs/", name)
	return nil
}

// extractPageStats returns (charCount, boxCount) for all pages in engine.
func extractPageStats(eng pdf.PDFEngine) (chars, boxes int) {
	np, _ := eng.PageCount()
	for pg := 0; pg < np; pg++ {
		pgChars, err := eng.ExtractChars(pg)
		if err != nil {
			continue
		}
		chars += len(pgChars)
		boxes += len(lyt.CharsToBoxes(pgChars, pg, false))
	}
	return
}

func textLenFromOutput(data []byte) int {
	s := string(data)
	if idx := strings.LastIndex(s, "\n#@meta"); idx >= 0 {
		s = s[:idx]
	}
	return utf8.RuneCountInString(s)
}

// ── main processing loop ────────────────────────────────────────────

func processPDFs(t *testing.T, pdfDir string, pdfs []string, deepDoc pdf.DocAnalyzer, variant string, dirs outputDirs) []tool.BatchResult {
	t.Helper()
	var results []tool.BatchResult
	totalChars := 0
	skipOCR := os.Getenv("BATCH_SKIP_OCR") == "1"

	for i, name := range pdfs {
		label := fmt.Sprintf("[%d/%d] %s", i+1, len(pdfs), name)

		// ── cached? ──
		if cached := tryLoadCached(dirs, name); cached != nil {
			results = append(results, *cached)
			totalChars += cached.TextLen
			t.Logf("%s %s — SKIP (cached, %d chars, %d sections)",
				time.Now().Format("15:04:05"), label, cached.TextLen, cached.Sections)
			continue
		}

		// ── parse ──
		res, err := parseOne(pdfDir, name, deepDoc, skipOCR)
		if err != nil {
			results = append(results, tool.BatchResult{File: name, Error: err.Error()})
			t.Logf("%s — %v", label, err)
			continue
		}

		writeOutputs(dirs, name, &res.result, res)
		results = append(results, res.BatchResult)
		totalChars += res.TextLen

		t.Logf("%s %s — chars=%d boxes:%d→%d→%d→%d text=%d (%.1fs)",
			time.Now().Format("15:04:05"), label, res.Chars,
			res.BoxesInitial, res.BoxesTextMerg, res.BoxesVertMerg, res.Sections,
			res.TextLen, res.TimeS)
	}

	t.Logf("\nDone. %d PDFs, %d chars. Output: %s/", len(results), totalChars, dirs.text)
	return results
}

type parseOneResult struct {
	tool.BatchResult
	result pdf.ParseResult
}

func parseOne(pdfDir, name string, deepDoc pdf.DocAnalyzer, skipOCR bool) (*parseOneResult, error) {
	data, err := os.ReadFile(filepath.Join(pdfDir, name))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	eng, err := NewEngine(data)
	if err != nil {
		return nil, fmt.Errorf("engine: %w", err)
	}
	defer eng.Close()

	pageCount, _ := eng.PageCount()
	chars, _ := extractPageStats(eng)

	cfg := pdf.DefaultParserConfig()
	cfg.SkipOCR = skipOCR
	p := NewParser(cfg)
	t0 := time.Now()
	parsed, err := p.ParseRaw(context.Background(), eng, deepDoc)
	elapsed := time.Since(t0).Seconds()
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	textLen := 0
	for _, s := range parsed.Sections {
		textLen += utf8.RuneCountInString(s.Text)
	}

	return &parseOneResult{
		BatchResult: tool.BatchResult{
			File:          name,
			Pages:         pageCount,
			Chars:         chars,
			BoxesInitial:  parsed.Metrics.BoxesInitial,
			BoxesTextMerg: parsed.Metrics.BoxesTextMerge,
			BoxesVertMerg: parsed.Metrics.BoxesVertMerge,
			Sections:      len(parsed.Sections),
			TextLen:       textLen,
			TimeS:         math.Round(elapsed*100) / 100,
		},
		result: *parsed,
	}, nil
}

func tryLoadCached(dirs outputDirs, name string) *tool.BatchResult {
	textPath := filepath.Join(dirs.text, name+".txt")
	tablesPath := filepath.Join(dirs.tables, name+".json")
	if !tool.FileExists(textPath) || !tool.FileExists(tablesPath) {
		return nil
	}
	data, err := os.ReadFile(textPath)
	if err != nil {
		return nil
	}
	var r tool.BatchResult
	r.File = name
	if idx := strings.LastIndex(string(data), "\n#@meta"); idx >= 0 {
		if json.Unmarshal(data[idx+7:], &r) == nil {
			// TextLen must be recalculated from text-only portion (excludes #@meta line).
			r.TextLen = textLenFromOutput(data)
			return &r
		}
	}
	return nil
}

// htmlToRows extracts cell text rows from an HTML <table> string,
// matching Python's html_to_rows in dump_py_results.py.
func htmlToRows(html string) [][]string {
	var rows [][]string
	re := regexp.MustCompile(`<tr>(.*?)</tr>`)
	td := regexp.MustCompile(`<t[dh][^>]*>(.*?)</t[dh]>`)
	for _, tr := range re.FindAllStringSubmatch(html, -1) {
		var cells []string
		for _, m := range td.FindAllStringSubmatch(tr[1], -1) {
			cells = append(cells, m[1])
		}
		rows = append(rows, cells)
	}
	return rows
}

func writeOutputs(dirs outputDirs, name string, parsed *pdf.ParseResult, res *parseOneResult) {
	// ── text + #@meta ──
	var sb strings.Builder
	for _, s := range parsed.Sections {
		sb.WriteString(s.Text)
		sb.WriteByte('\n')
	}
	if b, _ := json.Marshal(res.BatchResult); b != nil {
		sb.WriteString("#@meta")
		sb.Write(b)
		sb.WriteByte('\n')
	}
	os.WriteFile(filepath.Join(dirs.text, name+".txt"), []byte(sb.String()), 0644)

	// ── tables JSON — extract rows from section HTML (matching Python html_to_rows) ──
	type slimTable struct {
		Rows      [][]string     `json:"rows"`
		Positions []pdf.Position `json:"positions,omitempty"`
	}
	// Collect all table sections in order (index-matched to TableItems).
	var tableSections []pdf.Section
	for _, s := range parsed.Sections {
		if s.LayoutType == "table" && strings.HasPrefix(s.Text, "<table>") {
			tableSections = append(tableSections, s)
		}
	}
	slim := make([]slimTable, len(parsed.Tables))
	for j, t := range parsed.Tables {
		slim[j].Rows = t.Rows
		slim[j].Positions = t.Positions
		// Fallback: extract rows from section HTML (index-matched).
		if len(slim[j].Rows) == 0 && j < len(tableSections) {
			slim[j].Rows = htmlToRows(tableSections[j].Text)
		}
	}
	if b, _ := json.MarshalIndent(slim, "", "  "); b != nil {
		os.WriteFile(filepath.Join(dirs.tables, name+".json"), b, 0644)
	}

	// ── DLA + TSR debug intermediates ──
	if parsed.DLADebug != nil {
		if b, _ := json.MarshalIndent(parsed.DLADebug, "", "  "); b != nil {
			os.WriteFile(filepath.Join(dirs.dla, name+".json"), b, 0644)
		}
	}
	if parsed.TSRDebug != nil {
		if b, _ := json.MarshalIndent(parsed.TSRDebug, "", "  "); b != nil {
			os.WriteFile(filepath.Join(dirs.tsrRaw, name+".json"), b, 0644)
		}
	}
}
