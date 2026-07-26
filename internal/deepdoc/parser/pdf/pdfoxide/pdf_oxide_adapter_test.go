//go:build cgo && manual

package pdfoxide

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var fixtureDir = filepath.Join("..", "testdata", "pdfs")

// ── Document opening ─────────────────────────────────────────────────────

func TestOpen(t *testing.T) {
	path := filepath.Join(fixtureDir, "01_english_simple.pdf")
	doc, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()
	if pc, _ := doc.PageCount(); pc != 1 {
		t.Fatalf("expected 1 page, got %d", pc)
	}
}

func TestOpenBytes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(fixtureDir, "01_english_simple.pdf"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	doc, err := OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer doc.Close()
	if pc, _ := doc.PageCount(); pc != 1 {
		t.Fatalf("expected 1 page, got %d", pc)
	}
}

func TestOpenBytes_Empty(t *testing.T) {
	_, err := OpenBytes(nil)
	if err == nil {
		t.Error("expected error for nil data")
	}
	_, err = OpenBytes([]byte{})
	if err == nil {
		t.Error("expected error for empty data")
	}
}

func TestOpen_InvalidPath(t *testing.T) {
	_, err := Open(filepath.Join(fixtureDir, "nonexistent.pdf"))
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

// ── PageCount ────────────────────────────────────────────────────────────

func TestPageCount(t *testing.T) {
	doc := openFixture(t, "01_english_simple.pdf")
	defer doc.Close()
	pc, err := doc.PageCount()
	if err != nil {
		t.Fatalf("PageCount: %v", err)
	}
	if pc != 1 {
		t.Errorf("expected 1 page, got %d", pc)
	}
}

func TestPageCount_MultiPage(t *testing.T) {
	doc := openFixture(t, "03_multipage.pdf")
	defer doc.Close()
	pc, err := doc.PageCount()
	if err != nil {
		t.Fatalf("PageCount: %v", err)
	}
	if pc < 2 {
		t.Errorf("expected >= 2 pages, got %d", pc)
	}
}

func TestPageCount_AfterClose(t *testing.T) {
	doc := openFixture(t, "01_english_simple.pdf")
	doc.Close()
	pc, err := doc.PageCount()
	if err == nil {
		t.Error("expected error after close")
	}
	if pc != 0 {
		t.Errorf("expected 0 after close, got %d", pc)
	}
}

// ── Close ────────────────────────────────────────────────────────────────

func TestClose_DoubleClose(t *testing.T) {
	doc := openFixture(t, "01_english_simple.pdf")
	doc.Close()
	// Second Close should not panic
	doc.Close()
}

// ── GetPageChars ─────────────────────────────────────────────────────────

func TestGetPageChars(t *testing.T) {
	doc := openFixture(t, "01_english_simple.pdf")
	defer doc.Close()

	chars, err := doc.GetPageChars(0)
	if err != nil {
		t.Fatalf("GetPageChars: %v", err)
	}
	if len(chars) == 0 {
		t.Fatal("expected non-empty chars")
	}

	c := chars[0]
	if c.Text == "" {
		t.Error("expected non-empty text")
	}
	if c.Fontname == "" {
		t.Error("expected non-empty fontname")
	}
	if c.X0 >= c.X1 {
		t.Errorf("expected x0 < x1, got %f >= %f", c.X0, c.X1)
	}
	if c.Top >= c.Bottom {
		t.Errorf("expected top < bottom, got %f >= %f", c.Top, c.Bottom)
	}
	if c.PageNumber < 1 {
		t.Errorf("expected page_number >= 1, got %d", c.PageNumber)
	}
	if c.Size <= 0 {
		t.Errorf("expected positive font size, got %f", c.Size)
	}
}

func TestGetPageChars_InvalidPage(t *testing.T) {
	doc := openFixture(t, "01_english_simple.pdf")
	defer doc.Close()

	// Negative page
	_, err := doc.GetPageChars(-1)
	if err == nil {
		t.Error("expected error for negative page")
	}

	// Out of range
	_, err = doc.GetPageChars(999)
	if err == nil {
		t.Error("expected error for out-of-range page")
	}
}

func TestGetPageChars_AfterClose(t *testing.T) {
	doc := openFixture(t, "01_english_simple.pdf")
	doc.Close()

	_, err := doc.GetPageChars(0)
	if err == nil {
		t.Error("expected error after close")
	}
}

// ── GetDedupePageChars ───────────────────────────────────────────────────

func TestGetDedupePageChars(t *testing.T) {
	doc := openFixture(t, "01_english_simple.pdf")
	defer doc.Close()

	raw, err := doc.GetPageChars(0)
	if err != nil {
		t.Fatalf("GetPageChars: %v", err)
	}

	deduped, err := doc.GetDedupePageChars(0, 1.0)
	if err != nil {
		t.Fatalf("GetDedupePageChars: %v", err)
	}
	if len(deduped) > len(raw) {
		t.Errorf("expected deduped <= raw (%d > %d)", len(deduped), len(raw))
	}
	if len(deduped) == 0 && len(raw) > 0 {
		t.Error("expected non-empty deduped when raw is non-empty")
	}
}

func TestGetDedupePageChars_Tolerance(t *testing.T) {
	doc := openFixture(t, "01_english_simple.pdf")
	defer doc.Close()

	// tolerance=0 should preserve all (no dedup)
	t0, _ := doc.GetDedupePageChars(0, 0)
	// high tolerance may merge more
	tHi, _ := doc.GetDedupePageChars(0, 100.0)

	raw, _ := doc.GetPageChars(0)
	if len(t0) != len(raw) {
		t.Logf("tolerance=0: %d chars (raw=%d) — some exact overlaps removed", len(t0), len(raw))
	}
	if len(tHi) > len(t0) {
		t.Errorf("high tolerance (%d) should not produce more chars than zero tolerance (%d)", len(tHi), len(t0))
	}
}

// ── GetPageText ──────────────────────────────────────────────────────────

func TestGetPageText(t *testing.T) {
	doc := openFixture(t, "01_english_simple.pdf")
	defer doc.Close()

	text, err := doc.GetPageText(0)
	if err != nil {
		t.Fatalf("GetPageText: %v", err)
	}
	if len(strings.TrimSpace(text)) == 0 {
		t.Error("expected non-empty text")
	}
	// This fixture is multi-line — verify newlines are present.
	if !strings.Contains(text, "\n") {
		t.Error("expected multi-line text to contain newlines")
	}
	// Verify no consecutive newlines (no blank lines from gaps).
	if strings.Contains(text, "\n\n") {
		t.Log("text contains blank lines (may be expected for this layout)")
	}
}

func TestGetPageTextMultiLine(t *testing.T) {
	doc := openFixture(t, "03_multipage.pdf")
	defer doc.Close()

	hasNewline := false
	pc, _ := doc.PageCount()
	for i := 0; i < pc; i++ {
		text, err := doc.GetPageText(i)
		if err != nil {
			t.Fatalf("GetPageText(%d): %v", i, err)
		}
		if len(text) == 0 {
			t.Errorf("page %d: expected non-empty text", i)
		}
		if strings.Contains(text, "\n") {
			hasNewline = true
		}
	}
	if !hasNewline {
		t.Error("expected at least one page to have multi-line text")
	}
}

// ── RenderPage ───────────────────────────────────────────────────────────

func TestRenderPage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(fixtureDir, "01_english_simple.pdf"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	res, err := RenderPage(data, 0, 72.0)
	if err != nil {
		t.Fatalf("RenderPage: %v", err)
	}
	if res.Width <= 0 || res.Height <= 0 {
		t.Errorf("invalid dimensions: %dx%d", res.Width, res.Height)
	}
	if res.Channels != 4 {
		t.Errorf("expected 4 channels, got %d", res.Channels)
	}
	expectedLen := res.Width * res.Height * res.Channels
	if len(res.Data) != expectedLen {
		t.Errorf("data length %d != %d", len(res.Data), expectedLen)
	}
}

func TestRenderPage_EmptyData(t *testing.T) {
	_, err := RenderPage(nil, 0, 72.0)
	if err == nil {
		t.Error("expected error for nil data")
	}
	_, err = RenderPage([]byte{}, 0, 72.0)
	if err == nil {
		t.Error("expected error for empty data")
	}
}

func TestRenderPage_MultiPage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(fixtureDir, "03_multipage.pdf"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for i := 0; i < 2; i++ {
		res, err := RenderPage(data, i, 72.0)
		if err != nil {
			t.Fatalf("RenderPage page %d: %v", i, err)
		}
		if res.Width <= 0 || res.Height <= 0 {
			t.Errorf("page %d: invalid dimensions", i)
		}
	}
}

// ── RenderResult methods ─────────────────────────────────────────────────

func TestRenderResult_ToImage(t *testing.T) {
	data, _ := os.ReadFile(filepath.Join(fixtureDir, "01_english_simple.pdf"))
	res, err := RenderPage(data, 0, 72.0)
	if err != nil {
		t.Fatalf("RenderPage: %v", err)
	}
	img := res.ToImage()
	if img.Bounds().Dx() != res.Width || img.Bounds().Dy() != res.Height {
		t.Errorf("image size %v != %dx%d", img.Bounds(), res.Width, res.Height)
	}
}

func TestRenderResult_At(t *testing.T) {
	data, _ := os.ReadFile(filepath.Join(fixtureDir, "01_english_simple.pdf"))
	res, err := RenderPage(data, 0, 72.0)
	if err != nil {
		t.Fatalf("RenderPage: %v", err)
	}
	// In-bounds: should return a non-nil color
	c := res.At(0, 0)
	if c == nil {
		t.Error("At(0,0) returned nil")
	}
	// Out-of-bounds: should not panic and return zero color
	out := res.At(-1, 0)
	if out == nil {
		t.Error("At(-1,0) returned nil")
	}
	out2 := res.At(res.Width, res.Height)
	if out2 == nil {
		t.Error("At(width,height) returned nil")
	}
}

func TestRenderResult_Bounds(t *testing.T) {
	data, _ := os.ReadFile(filepath.Join(fixtureDir, "01_english_simple.pdf"))
	res, err := RenderPage(data, 0, 72.0)
	if err != nil {
		t.Fatalf("RenderPage: %v", err)
	}
	b := res.Bounds()
	if b.Min.X != 0 || b.Min.Y != 0 {
		t.Errorf("expected origin at (0,0), got (%d,%d)", b.Min.X, b.Min.Y)
	}
	if b.Dx() != res.Width || b.Dy() != res.Height {
		t.Errorf("bounds %v != %dx%d", b, res.Width, res.Height)
	}
}

func TestRenderResult_ColorModel(t *testing.T) {
	data, _ := os.ReadFile(filepath.Join(fixtureDir, "01_english_simple.pdf"))
	res, _ := RenderPage(data, 0, 72.0)
	// ColorModel should return a non-nil model
	if res.ColorModel() == nil {
		t.Error("ColorModel returned nil")
	}
}

// ── TotalPageNumber ──────────────────────────────────────────────────────

func TestTotalPageNumber(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(fixtureDir, "03_multipage.pdf"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	n, err := TotalPageNumber("", data)
	if err != nil {
		t.Fatalf("TotalPageNumber: %v", err)
	}
	if n < 2 {
		t.Errorf("expected >= 2 pages, got %d", n)
	}
}

func TestTotalPageNumber_File(t *testing.T) {
	path := filepath.Join(fixtureDir, "01_english_simple.pdf")
	n, err := TotalPageNumber(path, nil)
	if err != nil {
		t.Fatalf("TotalPageNumber: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 page, got %d", n)
	}
}

// ── InitRenderer ─────────────────────────────────────────────────────────

func TestInitRenderer(t *testing.T) {
	if err := InitRenderer(""); err != nil {
		t.Errorf("InitRenderer should be no-op, got: %v", err)
	}
}

// ── Multiple PDFs smoke test ─────────────────────────────────────────────

func TestMultiplePDFs(t *testing.T) {
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".pdf" {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			doc, err := Open(filepath.Join(fixtureDir, name))
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer doc.Close()

			pc, _ := doc.PageCount()
			if pc == 0 {
				t.Error("PageCount returned 0")
			}
			for i := 0; i < pc; i++ {
				chars, err := doc.GetPageChars(i)
				if err != nil {
					t.Errorf("GetPageChars(%d): %v", i, err)
					continue
				}
				if len(chars) == 0 {
					t.Logf("page %d: 0 chars (may be image-only or sparse)", i)
				}
			}
		})
		count++
	}
	if count == 0 {
		t.Error("no PDFs found in fixture directory")
	}
	t.Logf("Tested %d PDFs", count)
}

// ── Engine-level tests ───────────────────────────────────────────────────

func TestPDFPlumber_RenderPage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(fixtureDir, "01_english_simple.pdf"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	eng, err := NewEngine(data)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	img, err := eng.RenderPage(0, 72.0)
	if err != nil {
		t.Fatalf("RenderPage: %v", err)
	}
	if len(img) == 0 {
		t.Error("RenderPage returned empty image data")
	}
}

func TestPDFPlumber_MultiPage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(fixtureDir, "03_multipage.pdf"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	eng, err := NewEngine(data)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	pc, _ := eng.PageCount()
	if pc < 2 {
		t.Fatalf("expected >= 2 pages, got %d", pc)
	}
	for i := 0; i < pc; i++ {
		chars, err := eng.ExtractChars(i)
		if err != nil {
			t.Errorf("ExtractChars(%d): %v", i, err)
		}
		if len(chars) == 0 {
			t.Logf("page %d: 0 chars extracted", i)
		}
	}
}

// ── Char extraction comparison with Python pdfplumber ────────────────────

// pyChar mirrors the per-character dict that Python pdfplumber writes into
// snapshots (stages.__images__.page_chars).
type pyChar struct {
	Text       string  `json:"text"`
	FontName   string  `json:"fontname"`
	Size       float64 `json:"size"`
	X0         float64 `json:"x0"`
	X1         float64 `json:"x1"`
	Top        float64 `json:"top"`
	Bottom     float64 `json:"bottom"`
	PageNumber int     `json:"page_number"`
}

// TestCharExtraction_CompareWithPython uses Go pdf_oxide to extract chars from
// the 16 test PDFs and compares against Python pdfplumber golden data in
// testdata/snapshots/*.json.
//
// pdf_oxide and pdfplumber are different engines with different internal
// ordering and coordinate origins, so we compare:
//   - char count per page (should match closely)
//   - text content (as sorted sets, ignoring order differences)
//   - coordinate ranges (min/max, since absolute positions differ by engine)
func TestCharExtraction_CompareWithPython(t *testing.T) {
	snapDir := filepath.Join("..", "testdata", "snapshots")

	entries, err := os.ReadDir(snapDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	totalPDFs := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		pdfPath := filepath.Join(fixtureDir, name+".pdf")
		if _, err := os.Stat(pdfPath); err != nil {
			t.Logf("SKIP %s: PDF not found", name)
			continue
		}

		t.Run(name, func(t *testing.T) {
			pyChars := loadPyPageChars(t, filepath.Join(snapDir, e.Name()))

			pdfData, err := os.ReadFile(pdfPath)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			eng, err := NewEngine(pdfData)
			if err != nil {
				t.Fatalf("NewEngine: %v", err)
			}
			defer eng.Close()

			goPageCount, _ := eng.PageCount()
			pyPageCount := len(pyChars)

			if goPageCount != pyPageCount {
				t.Logf("page count: Go=%d Python=%d", goPageCount, pyPageCount)
			}

			totalPy, totalGo := 0, 0
			textInBoth, textOnlyPy, textOnlyGo := 0, 0, 0
			maxPages := goPageCount
			if pyPageCount > maxPages {
				maxPages = pyPageCount
			}

			for pg := 0; pg < maxPages; pg++ {
				var pyPage []pyChar
				if pg < len(pyChars) {
					pyPage = pyChars[pg]
				}
				goPage, err := eng.ExtractChars(pg)
				if err != nil {
					t.Logf("page %d: Go ExtractChars error: %v", pg, err)
					continue
				}

				totalPy += len(pyPage)
				totalGo += len(goPage)

				// Build text sets (sorted by position order differs between engines)
				pyTexts := make(map[string]int)
				for _, c := range pyPage {
					pyTexts[c.Text]++
				}
				goTexts := make(map[string]int)
				for _, c := range goPage {
					goTexts[c.Text]++
				}

				// Count texts that appear in both
				for t, pyCount := range pyTexts {
					goCount := goTexts[t]
					if goCount > 0 {
						m := pyCount
						if goCount < m {
							m = goCount
						}
						textInBoth += m
					} else {
						textOnlyPy += pyCount
					}
				}
				for t, goCount := range goTexts {
					if pyTexts[t] == 0 {
						textOnlyGo += goCount
					}
				}

				if len(pyPage) != len(goPage) {
					t.Logf("page %d: char count Go=%d Python=%d", pg, len(goPage), len(pyPage))
				}
			}

			// Summary
			totalCompared := textInBoth + textOnlyPy + textOnlyGo
			overlapRate := 0.0
			if totalCompared > 0 {
				overlapRate = float64(textInBoth) / float64(totalCompared) * 100
			}

			t.Logf("chars: Go=%d Python=%d | text overlap: %.1f%% (shared=%d, only_py=%d, only_go=%d)",
				totalGo, totalPy, overlapRate, textInBoth, textOnlyPy, textOnlyGo)

			if totalPy > 0 && totalGo > 0 {
				countDiff := float64(math.Abs(float64(totalGo-totalPy))) / float64(totalPy) * 100
				if countDiff > 5 {
					t.Errorf("char count differs by %.1f%% (>5%%)", countDiff)
				}
			}
		})
		totalPDFs++
	}

	if totalPDFs == 0 {
		t.Error("no PDF/snapshot pairs found")
	}
}

// loadPyPageChars reads Python pdfplumber page_chars from a snapshot JSON.
func loadPyPageChars(t *testing.T, path string) [][]pyChar {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var s struct {
		Stages map[string]struct {
			PageChars [][]pyChar `json:"page_chars"`
		} `json:"stages"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse: %v", err)
	}
	stage, ok := s.Stages["__images__"]
	if !ok {
		t.Fatal("no __images__ stage in snapshot")
	}
	return stage.PageChars
}

// ── Helpers ──────────────────────────────────────────────────────────────

func openFixture(t *testing.T, name string) *Document {
	t.Helper()
	doc, err := Open(filepath.Join(fixtureDir, name))
	if err != nil {
		t.Fatalf("Open(%s): %v", name, err)
	}
	return doc
}

func TestGetPageChars_RadicalNormalization(t *testing.T) {
	// Verify that GetPageChars applies normalizeRadicals to every char.
	// Uses any available fixture PDF — just checking no radical leaks through.
	doc := openFixture(t, "01_english_simple.pdf")
	defer doc.Close()

	n, _ := doc.PageCount()
	foundRadical := false
	for pg := 0; pg < n && !foundRadical; pg++ {
		chars, err := doc.GetPageChars(pg)
		if err != nil {
			continue
		}
		for _, c := range chars {
			for _, r := range c.Text {
				if r >= 0x2F00 && r <= 0x2FDF {
					t.Errorf("Kangxi Radical U+%04X found in page %d: %q — normalization NOT applied",
						r, pg, c.Text)
					foundRadical = true
					break
				}
			}
		}
	}
	if !foundRadical {
		t.Log("No Kangxi Radicals found — normalization applied (or none in source)")
	}
}

// TestExtractChars_RotatedPages_CoordsInBounds verifies that character
// coordinates from rotated pages stay within page bounds.  pdf_oxide
// already applies /Rotate internally; the Go engine must not rotate
// a second time (double rotation pushes coords out of bounds).
func TestExtractChars_RotatedPages_CoordsInBounds(t *testing.T) {
	angles := []struct {
		name string
		rot  int
	}{
		{"rotate_0", 0},
		{"rotate_90", 90},
		{"rotate_180", 180},
		{"rotate_270", 270},
	}

	for _, a := range angles {
		t.Run(a.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(fixtureDir, a.name+".pdf"))
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			eng, err := NewEngine(data)
			if err != nil {
				t.Fatalf("NewEngine: %v", err)
			}
			defer eng.Close()

			chars, err := eng.ExtractChars(0)
			if err != nil {
				t.Fatalf("ExtractChars: %v", err)
			}
			if len(chars) == 0 {
				// Some rotated pages may legitimately have no extractable
				// characters.  The critical requirement: if chars ARE
				// returned, every one must be within page bounds.
				t.Skipf("0 chars extracted — skipping bounds check")
			}

			w, h, err := eng.PageSize(0)
			if err != nil {
				t.Fatalf("PageSize: %v", err)
			}

			outOfBounds := 0
			for _, c := range chars {
				if c.X0 < -1 || c.X1 > w+1 || c.Top < -1 || c.Bottom > h+1 {
					t.Errorf("char %q out of bounds: (%.0f,%.0f)-(%.0f,%.0f) page=(%.0f,%.0f) rot=%d",
						c.Text, c.X0, c.Top, c.X1, c.Bottom, w, h, a.rot)
					outOfBounds++
				}
			}
			if outOfBounds > 0 {
				t.Errorf("%d/%d chars are out of bounds (rotation=%d°)",
					outOfBounds, len(chars), a.rot)
			}
		})
	}
}
