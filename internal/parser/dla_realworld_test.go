//go:build cgo && integration

package parser

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDLARealWorldCompare runs DLA on real PDFs to cover more DLA label types.
func TestDLARealWorldCompare(t *testing.T) {
	client := mustConnectDeepDoc(t)
	outDir := filepath.Join("testdata", "output", "render_compare")
	os.MkdirAll(outDir, 0755)

	type pdfSpec struct {
		name  string
		pages []int
	}
	pdfs := []pdfSpec{
		{"2401.10744.pdf", []int{0, 1, 2, 3, 4}},
		{"fxbaogao.com.pdf", []int{0}},
	}

	allLabels := map[string]int{}

	for _, pdf := range pdfs {
		data, err := os.ReadFile(filepath.Join("testdata", "real_pdfs", pdf.name))
		if err != nil {
			t.Skipf("%s not found: %v", pdf.name, err)
			continue
		}
		eng, err := NewEngine(data)
		if err != nil {
			t.Fatalf("%s engine: %v", pdf.name, err)
		}

		for _, pg := range pdf.pages {
			t.Run(pdf.name+"/page"+string(rune('0'+pg)), func(t *testing.T) {
				pageImg, err := renderPageToImage(eng, pg)
				if err != nil {
					t.Fatalf("render page %d: %v", pg, err)
				}

				// Save input image.
				imgPath := filepath.Join(outDir, pdf.name+"_p"+string(rune('0'+pg))+"_dla_input.png")
				savePNGFile(imgPath, pageImg)

				// Call DLA.
				regions, err := client.DLA(pageImg)
				if err != nil {
					t.Fatalf("DLA: %v", err)
				}

				// Save response.
				goJSON := filepath.Join(outDir, pdf.name+"_p"+string(rune('0'+pg))+"_go_dla.json")
				writeJSON(t, goJSON, regions)

				// Report.
				labels := map[string]int{}
				for _, r := range regions {
					labels[r.Label]++
					allLabels[r.Label]++
				}
				t.Logf("page %d: %d regions, labels: %v", pg, len(regions), labels)
			})
		}
		eng.Close()
	}

	// Summary of all labels found.
	t.Logf("\n=== Total label coverage ===")
	for label, count := range allLabels {
		t.Logf("  %s: %d", label, count)
	}
}
