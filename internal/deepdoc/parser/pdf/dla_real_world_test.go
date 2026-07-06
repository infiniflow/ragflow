//go:build cgo && integration

package pdf

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestDLARealWorldCompare runs DLA on fixture PDFs and verifies
// region count, label types, and structural invariants.
func TestDLARealWorldCompare(t *testing.T) {
	client := mustConnectInferenceClient(t)
	outDir := filepath.Join("testdata", "output", "render_compare")
	os.MkdirAll(outDir, 0755)

	type pdfSpec struct {
		name           string
		pages          []int
		wantLabels     []string // must include at least one of these
		wantMinRegions int
	}
	pdfs := []pdfSpec{
		{
			name:           "06_table_content.pdf",
			pages:          []int{0},
			wantLabels:     []string{"text", "table"},
			wantMinRegions: 3,
		},
		{
			name:           "02_chinese_simple.pdf",
			pages:          []int{0},
			wantLabels:     []string{"text", "title"},
			wantMinRegions: 3,
		},
	}

	allLabels := map[string]int{}

	for _, pdf := range pdfs {
		eng := mustOpenEngine(t, pdf.name)
		defer eng.Close()

		for _, pg := range pdf.pages {
			testName := pdf.name + "/page" + string(rune('0'+pg))
			t.Run(testName, func(t *testing.T) {
				pageImg, err := RenderPageToImage(eng, pg)
				if err != nil {
					t.Fatalf("render page %d: %v", pg, err)
				}

				// Save input image for debugging.
				imgPath := filepath.Join(outDir, pdf.name+"_p"+string(rune('0'+pg))+"_dla_input.png")
				savePNGFile(imgPath, pageImg)

				// Call DLA.
				regions, err := client.DLA(context.Background(), pageImg)
				if err != nil {
					t.Fatalf("DLA: %v", err)
				}

				// Save response for debugging.
				goJSON := filepath.Join(outDir, pdf.name+"_p"+string(rune('0'+pg))+"_go_dla.json")
				writeJSON(t, goJSON, regions)

				// ── Assertions ──

				// 1. Must produce regions.
				if len(regions) == 0 {
					t.Fatal("DLA returned 0 regions")
				}
				if len(regions) < pdf.wantMinRegions {
					t.Errorf("expected >= %d regions, got %d", pdf.wantMinRegions, len(regions))
				}

				// 2. Each region must have valid structure.
				labelSet := map[string]int{}
				for i, r := range regions {
					if r.Label == "" {
						t.Errorf("region[%d] has empty label", i)
					}
					if r.X0 >= r.X1 || r.Y0 >= r.Y1 {
						t.Errorf("region[%d] %q: invalid bbox [%.0f %.0f %.0f %.0f]",
							i, r.Label, r.X0, r.Y0, r.X1, r.Y1)
					}
					if r.Confidence <= 0 {
						t.Errorf("region[%d] %q: confidence=%.4f (expected > 0)",
							i, r.Label, r.Confidence)
					}
					labelSet[r.Label]++
					allLabels[r.Label]++
				}

				// 3. Must contain expected label types.
				foundAny := false
				for _, want := range pdf.wantLabels {
					if labelSet[want] > 0 {
						foundAny = true
						break
					}
				}
				if !foundAny {
					t.Errorf("expected at least one of %v labels; got %v",
						pdf.wantLabels, labelSet)
				}

				t.Logf("page %d: %d regions, labels: %v", pg, len(regions), labelSet)
			})
		}
	}

	// Summary of all labels found.
	t.Logf("=== Total label coverage ===")
	for label, count := range allLabels {
		t.Logf("  %s: %d", label, count)
	}
}
