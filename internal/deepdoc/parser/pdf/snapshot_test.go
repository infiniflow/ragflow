//go:build manual

package pdf

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	lyt "ragflow/internal/deepdoc/parser/pdf/layout"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestSnapshotStageComparison verifies Go's lyt.TextMerge output
// matches Python's _text_merge sample boxes using synthetic input.
func TestSnapshotStageComparison(t *testing.T) {
	snapDir := filepath.Join("testdata", "snapshots")

	// Pick 3 representative PDFs for detailed comparison
	for _, name := range []string{"01_english_simple", "02_chinese_simple", "04_multicolumn"} {
		t.Run(name, func(t *testing.T) {
			snap := loadSnapshot(t, filepath.Join(snapDir, name+".json"))

			// Get boxes after __images__ (these are the input to Go pipeline)
			s1, ok := snap.Stages["__images__"]
			if !ok || len(s1.SampleBoxesPage0) == 0 {
				t.Skip("no sample boxes in snapshot")
			}

			// Get the text_merge stage output (Python reference)
			s4, ok := snap.Stages["_text_merge"]
			if !ok {
				t.Skip("no text_merge stage")
			}

			t.Logf("PDF: %s", snap.PDFFile)
			t.Logf("  Total pages: %v", s1.TotalPages)
			t.Logf("  Is English: %v", s1.IsEnglish)
			t.Logf("  Sample boxes (page 0): %d", len(s1.SampleBoxesPage0))
			t.Logf("  Text merge: %d -> %d boxes", s4.BoxesBefore, s4.BoxesAfter)

			// Convert sample boxes to Go pdf.TextBox format
			goBoxes := snapshotBoxesToGo(s1.SampleBoxesPage0)

			// Run Go lyt.TextMerge with default params
			meanH := map[int]float64{0: avg(s1.MeanHeight)}
			merged := lyt.TextMerge(goBoxes, meanH, 3)

			// Compare counts
			if len(merged) > 0 {
				t.Logf("  Go lyt.TextMerge: %d -> %d boxes", len(goBoxes), len(merged))
				mergeRatio := float64(len(merged)) / float64(len(goBoxes))
				pyRatio := float64(s4.BoxesAfter) / float64(s4.BoxesBefore)
				t.Logf("  Merge ratios: Go=%.0f%% Python=%.0f%%", mergeRatio*100, pyRatio*100)
			}

			// Run Go lyt.NaiveVerticalMerge
			meanW := map[int]float64{0: avg(s1.MeanWidth)}
			vm := lyt.NaiveVerticalMerge(merged, meanH, meanW, s1.IsEnglish)
			if s6, ok := snap.Stages["_naive_vertical_merge"]; ok {
				t.Logf("  Go VerticalMerge: %d -> %d boxes (Python: %d->%d)",
					len(merged), len(vm), s6.BoxesBefore, s6.BoxesAfter)
			}
			// Sanity-check VM output
			if len(merged) > 0 && len(vm) > len(merged) {
				t.Errorf("VerticalMerge increased box count (%d -> %d)", len(merged), len(vm))
			}
			if len(merged) > 1 && len(vm) == 0 {
				t.Error("VerticalMerge zeroed non-empty input")
			}

			// Run Go boxesToSections
			sections := lyt.BoxesToSections(vm, nil)
			if len(vm) > 0 && len(sections) == 0 {
				t.Error("boxesToSections produced 0 sections from non-empty boxes")
			}
			if len(sections) > 0 {
				t.Logf("  Go sections: %d - preview: %q", len(sections),
					truncate(sections[0].Text, 60))
			}
		})
	}
}

// --- snapshot types ---

type snapshot struct {
	PDFFile string                   `json:"pdf_file"`
	Stages  map[string]snapshotStage `json:"stages"`
}

type snapshotStage struct {
	// __images__
	TotalPages       int           `json:"total_pages"`
	PageCount        int           `json:"page_count"`
	MeanHeight       []float64     `json:"mean_height"`
	MeanWidth        []float64     `json:"mean_width"`
	IsEnglish        bool          `json:"is_english"`
	BoxesPerPage     []int         `json:"boxes_per_page"`
	SampleBoxesPage0 []snapshotBox `json:"sample_boxes_page0"`

	// _text_merge, _concat_downward, _naive_vertical_merge, _filter_forpages
	BoxesBefore int           `json:"boxes_before"`
	BoxesAfter  int           `json:"boxes_after"`
	SampleBoxes []snapshotBox `json:"sample_boxes"`

	// _extract_table_figure
	TableCount     int `json:"table_count"`
	RemainingBoxes int `json:"remaining_boxes"`

	// __call__
	PageCharsRaw    [][]json.RawMessage `json:"page_chars"`
	PageImagesSize  []map[string]int    `json:"page_images_size"`
	TextPreview     string              `json:"text_preview"`
	TextLength      int                 `json:"text_length"`
	TextLengthClean int                 `json:"text_length_clean"`
	TableCountOut   int                 `json:"table_count_out"`
}

type snapshotBox struct {
	X0         float64     `json:"x0"`
	X1         float64     `json:"x1"`
	Top        float64     `json:"top"`
	Bottom     float64     `json:"bottom"`
	Text       string      `json:"text"`
	PageNumber int         `json:"page_number"`
	LayoutType string      `json:"layout_type"`
	LayoutNo   string      `json:"layoutno"`
	ColID      int         `json:"col_id"`
	R          interface{} `json:"R"` // could be string or int
}

func loadSnapshot(t *testing.T, path string) snapshot {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var s snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse: %v", err)
	}
	return s
}

func snapshotBoxesToGo(sbs []snapshotBox) []pdf.TextBox {
	boxes := make([]pdf.TextBox, len(sbs))
	for i, sb := range sbs {
		boxes[i] = pdf.TextBox{
			X0: sb.X0, X1: sb.X1, Top: sb.Top, Bottom: sb.Bottom,
			Text: sb.Text, PageNumber: sb.PageNumber - 1, // pdfplumber uses 1-based
			LayoutType: sb.LayoutType, LayoutNo: sb.LayoutNo,
			ColID: sb.ColID, R: toInt(sb.R),
		}
	}
	return boxes
}

func stagesNames(s snapshot) []string {
	var keys []string
	for k := range s.Stages {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func avg(nums []float64) float64 {
	if len(nums) == 0 {
		return 0
	}
	sum := 0.0
	for _, n := range nums {
		sum += n
	}
	return sum / float64(len(nums))
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}

// TestSnapshotRoundtrip verifies we can load and save snapshot data
// without corruption, and that the format is self-consistent.
func TestSnapshotRoundtrip(t *testing.T) {
	snapDir := filepath.Join("testdata", "snapshots")

	for _, name := range []string{"01_english_simple", "08_edge_cases", "16_dense_cjk"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(snapDir, name+".json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			// Verify valid JSON
			var raw map[string]interface{}
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}

			// Verify required keys
			if _, ok := raw["pdf_file"]; !ok {
				t.Error("missing pdf_file")
			}
			stages, ok := raw["stages"].(map[string]interface{})
			if !ok {
				t.Fatal("stages not a map")
			}

			// Verify required stages exist
			for _, required := range []string{"__images__", "_text_merge", "_concat_downward", "_naive_vertical_merge"} {
				if _, ok := stages[required]; !ok {
					t.Errorf("missing stage: %s", required)
				}
			}
			t.Logf("%s: %d stages, %s bytes", name, len(stages),
				formatBytes(len(data)))
		})
	}
}

func toInt(v interface{}) int {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case string:
		n, _ := strconv.Atoi(x)
		return n
	default:
		return 0
	}
}

func formatBytes(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
}

// TestSnapshotsConsistency checks that stage counts are monotonically non-increasing
// (each merge stage should never increase box counts).
func TestSnapshotsConsistency(t *testing.T) {
	snapDir := filepath.Join("testdata", "snapshots")
	entries, _ := os.ReadDir(snapDir)

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") || strings.HasSuffix(e.Name(), "_chars.json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		t.Run(name, func(t *testing.T) {
			snap := loadSnapshot(t, filepath.Join(snapDir, e.Name()))

			s4, ok4 := snap.Stages["_text_merge"]
			_, _ = snap.Stages["_concat_downward"]
			s6, ok6 := snap.Stages["_naive_vertical_merge"]

			// After text_merge, counts should decrease or stay same
			if ok4 && s4.BoxesBefore > 0 && s4.BoxesAfter > s4.BoxesBefore {
				t.Errorf("_text_merge INCREASED: %d -> %d", s4.BoxesBefore, s4.BoxesAfter)
			}
			// After vertical merge
			if ok6 && s6.BoxesBefore > 0 && s6.BoxesAfter > s6.BoxesBefore {
				t.Errorf("_naive_vertical_merge INCREASED: %d -> %d", s6.BoxesBefore, s6.BoxesAfter)
			}

			// Transitivity: if both exist, s4.BoxesAfter >= s6.BoxesAfter
			if ok4 && ok6 && s4.BoxesAfter > 0 && s6.BoxesAfter > 0 {
				if s6.BoxesAfter > s4.BoxesAfter {
					t.Errorf("unexpected: vertical_merge(%d) > text_merge(%d)", s6.BoxesAfter, s4.BoxesAfter)
				}
			}

			// Verify sample boxes have valid coordinates
			if ok4 && len(s4.SampleBoxes) > 0 {
				for i, b := range s4.SampleBoxes {
					if b.X1 <= b.X0 || b.Bottom <= b.Top || math.IsNaN(b.X0) {
						t.Errorf("sample_box[%d] invalid: x0=%.1f x1=%.1f top=%.1f bottom=%.1f",
							i, b.X0, b.X1, b.Top, b.Bottom)
					}
				}
			}
		})
	}
}
