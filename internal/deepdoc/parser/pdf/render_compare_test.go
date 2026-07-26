//go:build cgo && manual

package pdf

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenderCompare renders PDF pages with Go (pdfium) and compares against
// Python-rendered images (if available). Outputs to testdata/render_compare/.
//
// Usage:
//  1. Run this test to generate Go renders:
//     go test -v -tags=manual -run TestRenderCompare -count=1
//  2. Run the Python script to generate Python renders:
//     python3 testdata/render_compare.py
//  3. Re-run this test — it will compare both and report similarity.
func TestRenderCompare(t *testing.T) {
	const dpi = 216.0
	pdfDir := filepath.Join("testdata", "pdfs")
	goDir := filepath.Join("testdata", "output", "render_compare", "go")
	pyDir := filepath.Join("testdata", "output", "render_compare", "py")
	os.MkdirAll(goDir, 0755)

	entries, err := os.ReadDir(pdfDir)
	if err != nil {
		t.Fatal(err)
	}

	compared := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".pdf") {
			continue
		}
		name := e.Name()
		data, err := os.ReadFile(filepath.Join(pdfDir, name))
		if err != nil {
			t.Logf("%s: read error: %v", name, err)
			continue
		}

		eng, err := NewEngine(data)
		if err != nil {
			t.Logf("%s: engine error: %v", name, err)
			continue
		}

		// Render page 0 with pdfium (Go).
		goImg, err := RenderPageToImage(eng, 0)
		eng.Close()
		if err != nil {
			t.Logf("%s: render error: %v", name, err)
			continue
		}

		// Save Go render.
		goPath := filepath.Join(goDir, name+"_p0.png")
		if err := savePNG(goPath, goImg); err != nil {
			t.Errorf("%s: save: %v", name, err)
			continue
		}

		goBounds := goImg.Bounds()
		t.Logf("%s: Go render %dx%d saved", name, goBounds.Dx(), goBounds.Dy())

		// Compare with Python render if available.
		pyPath := filepath.Join(pyDir, name+"_p0.png")
		pyFile, err := os.Open(pyPath)
		if err != nil {
			continue // Python image not available yet
		}
		pyImg, err := png.Decode(pyFile)
		pyFile.Close()
		if err != nil {
			t.Logf("%s: decode py image: %v", name, err)
			continue
		}

		sim := pixelSimilarity(goImg, pyImg)
		compared++

		pyBounds := pyImg.Bounds()
		sizeMatch := goBounds.Dx() == pyBounds.Dx() && goBounds.Dy() == pyBounds.Dy()

		status := "✅"
		if sim < 90.0 {
			status = "⚠️"
		}
		if sim < 50.0 {
			status = "❌"
		}

		t.Logf("%s %s: similarity=%.1f%% size Go=%dx%d Py=%dx%d sizeMatch=%v",
			status, name, sim, goBounds.Dx(), goBounds.Dy(), pyBounds.Dx(), pyBounds.Dy(), sizeMatch)
	}

	if compared == 0 {
		t.Logf("No Python renders found in %s — run: python3 tools/render_compare.py", pyDir)
	} else {
		t.Logf("Compared %d PDFs", compared)
	}
}

func savePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// pixelSimilarity computes the percentage of pixels that match within tolerance.
// Handles different-sized images by comparing the overlapping region.
func pixelSimilarity(a, b image.Image) float64 {
	ab, bb := a.Bounds(), b.Bounds()
	w := min(ab.Dx(), bb.Dx())
	h := min(ab.Dy(), bb.Dy())
	if w == 0 || h == 0 {
		return 0
	}

	const tolerance = 30 // per-channel tolerance (0-255)
	matching := 0

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r1, g1, b1, _ := a.At(ab.Min.X+x, ab.Min.Y+y).RGBA()
			r2, g2, b2, _ := b.At(bb.Min.X+x, bb.Min.Y+y).RGBA()
			// RGBA() returns 16-bit values; convert to 8-bit.
			dr := math.Abs(float64(r1>>8) - float64(r2>>8))
			dg := math.Abs(float64(g1>>8) - float64(g2>>8))
			db := math.Abs(float64(b1>>8) - float64(b2>>8))
			if dr <= tolerance && dg <= tolerance && db <= tolerance {
				matching++
			}
		}
	}

	// Penalize size mismatch.
	maxArea := max(ab.Dx()*ab.Dy(), bb.Dx()*bb.Dy())
	if maxArea == 0 {
		return 0
	}
	return float64(matching) / float64(maxArea) * 100
}

func colorDiff(a, b color.Color) float64 {
	r1, g1, b1, _ := a.RGBA()
	r2, g2, b2, _ := b.RGBA()
	dr := float64(r1>>8) - float64(r2>>8)
	dg := float64(g1>>8) - float64(g2>>8)
	db := float64(b1>>8) - float64(b2>>8)
	return math.Sqrt(dr*dr + dg*dg + db*db)
}
