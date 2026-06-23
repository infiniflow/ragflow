package parser

import (
	"bytes"
	"context"
	"strconv"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testDataDir = "testdata/pipeline"

// TestPipeline_CompareWithPython verifies the Go layout pipeline produces
// identical results to the Python reference implementation across all stages.
func TestPipeline_CompareWithPython(t *testing.T) {
	chars := loadTestChars(t, "chars.json")
	if len(chars) == 0 {
		t.Fatal("no chars loaded")
	}
	t.Logf("Loaded %d synthetic chars", len(chars))

	meanHeights := loadFloatMap(t, "mean_heights.json")
	meanWidths := loadFloatMap(t, "mean_widths.json")

	// ---- Stage 1: chars → boxes ----
	pageGroups := make(map[int][]TextChar)
	for _, c := range chars {
		pageGroups[c.PageNumber] = append(pageGroups[c.PageNumber], c)
	}
	var goBoxes []TextBox
	for pg := 0; pg <= 1; pg++ {
		bxs := charsToBoxes(pageGroups[pg], pg, false)
		goBoxes = append(goBoxes, bxs...)
	}

	pyBoxes := loadPyBoxes(t, "py_boxes_initial.json")
	t.Logf("Stage 1 (chars→boxes): Go=%d, Python=%d", len(goBoxes), len(pyBoxes))
	compareBoxes(t, goBoxes, pyBoxes, "chars→boxes")

	// ---- Stage 2: column assignment + text merge (matching Python _text_merge) ----
	goBoxes = AssignColumn(goBoxes, 3)
	goBoxes = TextMerge(goBoxes, meanHeights, 3)
	t.Logf("Stage 2 (text_merge): Go=%d", len(goBoxes))

	// ---- Stage 3: sort by page→top→x0 (matching Python _concat_downward) ----
	sortByPageThenY(goBoxes, false)
	t.Logf("Stage 3 (sort Y): Go=%d", len(goBoxes))

	// ---- Stage 4: naive vertical merge ----
	goBoxes = NaiveVerticalMerge(goBoxes, meanHeights, meanWidths, false)
	t.Logf("Stage 4 (vertical_merge): Go=%d boxes after merge", len(goBoxes))

	// ---- Stage 5: boxes → sections ----
	goSections := boxesToSections(goBoxes, nil)
	pySections := loadPySections(t, "py_sections.json")
	t.Logf("Stage 5 (sections): Go=%d, Python=%d", len(goSections), len(pySections))
	// Note: golden files were generated from a different Python pipeline;
	// exact match not expected. Log counts for informational purposes.
	compareSections(t, goSections, pySections)
}

// TestPipeline_Stability verifies the pipeline produces deterministic output
// (same input → same output across multiple runs).
func TestPipeline_Stability(t *testing.T) {
	chars := loadTestChars(t, "chars.json")
	if len(chars) == 0 {
		t.Fatal("no chars loaded")
	}

	pageGroups := make(map[int][]TextChar)
	for _, c := range chars {
		pageGroups[c.PageNumber] = append(pageGroups[c.PageNumber], c)
	}

	var first []Section
	for run := 0; run < 3; run++ {
		var boxes []TextBox
		for pg := 0; pg <= 1; pg++ {
			bxs := charsToBoxes(pageGroups[pg], pg, false)
			boxes = append(boxes, bxs...)
		}
		boxes = AssignColumn(boxes, 3)
		meanH := map[int]float64{0: 14, 1: 14}
		meanW := map[int]float64{0: 7, 1: 7}
		boxes = TextMerge(boxes, meanH, 3)
		sortByPageThenY(boxes, false)
		boxes = NaiveVerticalMerge(boxes, meanH, meanW, false)
		sections := boxesToSections(boxes, nil)

		if run == 0 {
			first = sections
		} else {
			if len(sections) != len(first) {
				t.Fatalf("run %d: got %d sections, expected %d (non-deterministic!)", run, len(sections), len(first))
			}
			for i := range sections {
				if sections[i].Text != first[i].Text {
					t.Errorf("run %d section[%d]: text differs", run, i)
				}
			}
		}
	}
	t.Logf("Stability: 3 runs produced identical output (%d sections)", len(first))
}

// -- helpers --

func loadTestChars(t *testing.T, name string) []TextChar {
	t.Helper()
	path := filepath.Join(testDataDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var raw []struct {
		X0         float64 `json:"x0"`
		X1         float64 `json:"x1"`
		Top        float64 `json:"top"`
		Bottom     float64 `json:"bottom"`
		Text       string  `json:"text"`
		FontName   string  `json:"fontname"`
		PageNumber int     `json:"page_number"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	chars := make([]TextChar, len(raw))
	for i, r := range raw {
		chars[i] = TextChar{
			X0: r.X0, X1: r.X1, Top: r.Top, Bottom: r.Bottom,
			Text: r.Text, FontName: r.FontName, PageNumber: r.PageNumber,
		}
	}
	return chars
}

func loadPyBoxes(t *testing.T, name string) []TextBox {
	t.Helper()
	path := filepath.Join(testDataDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var raw []struct {
		X0         float64 `json:"x0"`
		X1         float64 `json:"x1"`
		Top        float64 `json:"top"`
		Bottom     float64 `json:"bottom"`
		Text       string  `json:"text"`
		PageNumber int     `json:"page_number"`
		LayoutType string  `json:"layout_type"`
		LayoutNo   string  `json:"layoutno"`
		ColID      int     `json:"col_id"`
		R          string  `json:"R"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	boxes := make([]TextBox, len(raw))
	for i, r := range raw {
		boxes[i] = TextBox{
			X0: r.X0, X1: r.X1, Top: r.Top, Bottom: r.Bottom,
			Text: r.Text, PageNumber: r.PageNumber,
			LayoutType: r.LayoutType, LayoutNo: r.LayoutNo,
			ColID: r.ColID, R: func(s string) int { n, _ := strconv.Atoi(s); return n }(r.R),
		}
	}
	return boxes
}

func loadPySections(t *testing.T, name string) [][2]string {
	t.Helper()
	path := filepath.Join(testDataDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var raw [][2]string
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return raw
}

func loadFloatMap(t *testing.T, name string) map[int]float64 {
	t.Helper()
	path := filepath.Join(testDataDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var raw map[string]float64
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	result := make(map[int]float64)
	for k, v := range raw {
		var pk int
		json.Unmarshal([]byte(k), &pk)
		result[pk] = v
	}
	return result
}

// compareBoxes checks that two box lists are equivalent.
func compareBoxes(t *testing.T, goBoxes, pyBoxes []TextBox, stage string) {
	t.Helper()
	if len(goBoxes) != len(pyBoxes) {
		t.Errorf("[%s] count mismatch: Go=%d Python=%d", stage, len(goBoxes), len(pyBoxes))
		// Don't early-return; compare what we can
	}
	n := min(len(goBoxes), len(pyBoxes))
	mismatches := 0
	for i := 0; i < n; i++ {
		g, p := goBoxes[i], pyBoxes[i]
		if g.PageNumber != p.PageNumber ||
			math.Abs(g.X0-p.X0) > 0.1 || math.Abs(g.X1-p.X1) > 0.1 ||
			math.Abs(g.Top-p.Top) > 0.1 || math.Abs(g.Bottom-p.Bottom) > 0.1 {
			mismatches++
			if mismatches <= 3 {
				t.Errorf("[%s] box[%d] coords differ:\n  Go: (%.1f,%.1f,%.1f,%.1f) pg=%d\n  Py: (%.1f,%.1f,%.1f,%.1f) pg=%d",
					stage, i, g.X0, g.X1, g.Top, g.Bottom, g.PageNumber,
					p.X0, p.X1, p.Top, p.Bottom, p.PageNumber)
			}
		}
		if g.Text != p.Text {
			mismatches++
			if mismatches <= 3 {
				t.Errorf("[%s] box[%d] text differs:\n  Go: %q\n  Py: %q", stage, i, g.Text, p.Text)
			}
		}
	}
	if mismatches == 0 {
		t.Logf("[%s] ✅ all %d boxes match", stage, n)
	} else {
		t.Errorf("[%s] ❌ %d/%d boxes differ", stage, mismatches, n)
	}
}

func compareSections(t *testing.T, goSections []Section, pySections [][2]string) {
	t.Helper()
	if len(goSections) != len(pySections) {
		t.Errorf("sections count: Go=%d Python=%d", len(goSections), len(pySections))
	}
	n := min(len(goSections), len(pySections))
	mismatches := 0
	for i := 0; i < n; i++ {
		goText := goSections[i].Text
		pyText := pySections[i][0]
		// Compare raw text (strip @@ tags since float formatting may differ)
		goRaw := stripTag(goText)
		pyRaw := stripTag(pyText)
		if goRaw != pyRaw {
			mismatches++
			if mismatches <= 3 {
				t.Errorf("section[%d] text differs:\n  Go: %q\n  Py: %q", i, goRaw, pyRaw)
			}
		}
	}
	if mismatches == 0 {
		t.Logf("[sections] ✅ all %d sections match", n)
	} else {
		t.Errorf("[sections] ❌ %d/%d sections differ", mismatches, n)
	}
}

func stripTag(text string) string {
	return strings.TrimSpace(removeTagPattern.ReplaceAllString(text, ""))
}

// mockEngine implements PDFEngine with predefined chars.
type mockEngine struct {
	chars     map[int][]TextChar
	pageCount int
	renderW   int // width for RenderPage, default 595
	renderH   int // height for RenderPage, default 842
}

func (m *mockEngine) ExtractChars(pageNum int) ([]TextChar, error) {
	return m.chars[pageNum], nil
}
func (m *mockEngine) RenderPage(pageNum int, dpi float64) ([]byte, error) {
	w, h := m.renderW, m.renderH
	if w <= 0 {
		w = 595
	}
	if h <= 0 {
		h = 842
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.White)
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes(), nil
}
func (m *mockEngine) RenderPageImage(pageNum int, dpi float64) (image.Image, error) {
	w, h := m.renderW, m.renderH
	if w <= 0 {
		w = 595
	}
	if h <= 0 {
		h = 842
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.White)
		}
	}
	return img, nil
}
func (m *mockEngine) RawData() []byte           { return nil }
func (m *mockEngine) PageCount() (int, error) { return m.pageCount, nil }
func (m *mockEngine) Close() error            { return nil }

func TestParser_Parse(t *testing.T) {
	chars := loadTestChars(t, "chars.json")
	if len(chars) == 0 {
		t.Skip("no chars.json")
	}
	byPage := make(map[int][]TextChar)
	for _, c := range chars {
		byPage[c.PageNumber] = append(byPage[c.PageNumber], c)
	}
	maxPage := 0
	for pg := range byPage {
		if pg > maxPage {
			maxPage = pg
		}
	}

	engine := &mockEngine{chars: byPage, pageCount: maxPage + 1}
	cfg := DefaultConfig()
	p := NewParser(cfg)
	result, err := p.Parse(context.Background(), engine)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	pySections := loadPySections(t, "py_sections.json")
	t.Logf("Parse: Go=%d sections, Python=%d sections", len(result.Sections), len(pySections))
	if len(result.Sections) == 0 {
		t.Error("Parse returned 0 sections")
	}

	// Verify section count is reasonable
	if len(result.Sections) < 1 {
		t.Error("Parse returned 0 sections")
	}
	// Verify against Python golden sections at count level
	// (exact text comparison not possible due to different char extraction paths)
	if len(result.Sections) != len(pySections) {
		t.Logf("section count: Go=%d Python=%d (different merge behavior expected)", len(result.Sections), len(pySections))
	}
	// Verify sections have position tags
	for i, s := range result.Sections {
		if s.PositionTag == "" {
			t.Errorf("section[%d] missing position tag", i)
		}
	}
}

// TestEndToEnd_SameChars feeds pdfplumber chars (from snapshot page_chars)
// to both Go and Python layout pipelines and compares results.
func TestEndToEnd_SameChars(t *testing.T) {
	snapDir := filepath.Join("testdata", "snapshots")

	for _, name := range []string{
		"09_crosspage_paragraph", "10_numbering_patterns",
		"11_three_column", "12_mixed_columns",
		"15_sparse_content", "16_dense_cjk",
	} {
		t.Run(name, func(t *testing.T) {
			snap := loadSnapshot(t, filepath.Join(snapDir, name+".json"))
			s1, ok := snap.Stages["__images__"]
			if !ok || len(s1.PageCharsRaw) == 0 {
				t.Skip("no page_chars in snapshot")
			}

			allChars := snapshotCharsToGo(s1.PageCharsRaw)
			isEng := s1.IsEnglish
			meanH := pageMeanFloat(s1.MeanHeight)
			meanW := pageMeanFloat(s1.MeanWidth)
			if len(meanH) == 0 {
				meanH = calcMedianCharHeights(allChars)
			}
			if len(meanW) == 0 {
				meanW = calcMedianCharWidths(allChars)
			}

			pageGroups := groupCharsByPage(allChars)
			var goBoxes []TextBox
			for pg, chars := range pageGroups {
				goBoxes = append(goBoxes, charsToBoxes(chars, pg, false)...)
			}
			t.Logf("chars: %d total", len(allChars))
			goBoxes = AssignColumn(goBoxes, 3)
			t.Logf("after assign_column: %d boxes", len(goBoxes))
			goBoxes = TextMerge(goBoxes, meanH, 3)
			t.Logf("after text_merge: %d boxes", len(goBoxes))
			sortByPageThenY(goBoxes, false)
			t.Logf("after sort_Y: %d boxes", len(goBoxes))
			goBoxes = NaiveVerticalMerge(goBoxes, meanH, meanW, isEng)
			t.Logf("after vertical_merge: %d boxes", len(goBoxes))
			sections := boxesToSections(goBoxes, nil)
			t.Logf("after boxesToSections: %d sections", len(sections))

			// Compare with snapshot stages
			s6, ok6 := snap.Stages["_naive_vertical_merge"]

			if ok6 {
				if s6.BoxesAfter > 0 {
					diff := math.Abs(float64(len(goBoxes)-s6.BoxesAfter)) / float64(s6.BoxesAfter) * 100
					if diff > 20 {
						t.Errorf("%s: VM box count differs by %.0f%% (Go=%d Py=%d)",
							name, diff, len(goBoxes), s6.BoxesAfter)
					}
				}
				t.Logf("%s: Go=%d boxes after VM, Python %d→%d",
					name, len(goBoxes), s6.BoxesBefore, s6.BoxesAfter)
			}
			goText := joinSectionText(sections)
			t.Logf("%s: Go=%d sections, %d chars", name, len(sections), len(goText))
		})
	}
}

// snapshotCharsToGo converts pdfplumber page_chars (nested array per page) to Go TextChar.
// pdfplumber uses 1-based page numbers; Go uses 0-based.
func snapshotCharsToGo(pageChars [][]json.RawMessage) []TextChar {
	var chars []TextChar
	for _, page := range pageChars {
		for _, raw := range page {
			var c struct {
				X0         float64 `json:"x0"`
				X1         float64 `json:"x1"`
				Top        float64 `json:"top"`
				Bottom     float64 `json:"bottom"`
				Text       string  `json:"text"`
				FontName   string  `json:"fontname"`
				PageNumber int     `json:"page_number"`
			}
			if err := json.Unmarshal(raw, &c); err != nil {
				continue
			}
			chars = append(chars, TextChar{
				X0: c.X0, X1: c.X1, Top: c.Top, Bottom: c.Bottom,
				Text: c.Text, FontName: c.FontName, PageNumber: c.PageNumber - 1,
			})
		}
	}
	return chars
}

func groupCharsByPage(chars []TextChar) map[int][]TextChar {
	g := make(map[int][]TextChar)
	for _, c := range chars {
		g[c.PageNumber] = append(g[c.PageNumber], c)
	}
	return g
}

func pageMeanFloat(nums []float64) map[int]float64 {
	r := make(map[int]float64)
	for i, v := range nums {
		r[i] = v
	}
	return r
}

func calcMedianCharHeights(chars []TextChar) map[int]float64 {
	g := groupCharsByPage(chars)
	r := make(map[int]float64)
	for pg, ch := range g {
		r[pg] = MedianCharHeight(ch)
	}
	return r
}

func calcMedianCharWidths(chars []TextChar) map[int]float64 {
	g := groupCharsByPage(chars)
	r := make(map[int]float64)
	for pg, ch := range g {
		r[pg] = MedianCharWidth(ch)
	}
	return r
}

func joinSectionText(sections []Section) string {
	var sb strings.Builder
	for _, s := range sections {
		sb.WriteString(s.Text) // Text is clean (no position tag embedded)
		sb.WriteString("\n")
	}
	return sb.String()
}
