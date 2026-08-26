//go:build cgo && manual

package pdf

import (
	"encoding/json"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"testing"

	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// TestWarpAlignGo renders a real PDF page with the production pdfium render
// path (the exact image the OCR detector receives) and, once a quads.json has
// been produced by tools/render_diff/warp_align.py's detect phase, also writes
// the Go WarpCrop and FastCrop crops for every detected quad.
//
// The crops land under WARP_OUT (default /tmp/render_diff/align) so nothing is
// committed and the worktree stays isolated from the rest of the repo.
//
// Run (two passes):
//
//	# pass 1: render the page (WARP_PDF is required by the Go test)
//	WARP_PDF=test/benchmark/test_docs/Doc1.pdf \
//	bash build.sh --test-manual ./internal/deepdoc/parser/pdf/ \
//	    -run TestWarpAlignGo
//	# derive quads + reference crops from the rendered page
//	.venv/bin/python tools/render_diff/warp_align.py genquads \
//	    --pdf test/benchmark/test_docs/Doc1.pdf --go-page-png /tmp/render_diff/align/page0.png
//	# pass 2: now quads.json exists -> write the Go crops
//	WARP_PDF=test/benchmark/test_docs/Doc1.pdf \
//	bash build.sh --test-manual ./internal/deepdoc/parser/pdf/ \
//	    -run TestWarpAlignGo
//	.venv/bin/python tools/render_diff/warp_align.py compare
//
// Env:
//
//	WARP_PDF   (required on pass 1) input PDF path
//	WARP_PAGE  (default 0) 0-based page index
//	WARP_OUT   (default /tmp/render_diff/align) output directory
//	WARP_QUADS (default <WARP_OUT>/quads.json) detect-phase output
type warpAlignBox struct {
	Quad []float64 `json:"quad"` // [x0,y0,x1,y1,x2,y2,x3,y3]
}

type warpAlignQuads struct {
	Boxes []warpAlignBox `json:"boxes"`
}

func TestWarpAlignGo(t *testing.T) {
	pdfPath := os.Getenv("WARP_PDF")
	if pdfPath == "" {
		t.Skip("WARP_PDF not set; nothing to render")
	}
	page := 0
	if v := os.Getenv("WARP_PAGE"); v != "" {
		fmt.Sscanf(v, "%d", &page)
	}
	out := os.Getenv("WARP_OUT")
	if out == "" {
		out = filepath.Join("/tmp", "render_diff", "align")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("read pdf: %v", err)
	}
	engine, err := NewEngine(data)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()
	img, err := RenderPageToImage(engine, page)
	if err != nil {
		t.Fatalf("render page %d: %v", page, err)
	}
	if err := writeAlignPNG(filepath.Join(out, fmt.Sprintf("page%d.png", page)), img); err != nil {
		t.Fatalf("write page: %v", err)
	}
	t.Logf("wrote page%d.png (%dx%d)", page, img.Bounds().Dx(), img.Bounds().Dy())

	quadsPath := os.Getenv("WARP_QUADS")
	if quadsPath == "" {
		quadsPath = filepath.Join(out, "quads.json")
	}
	raw, err := os.ReadFile(quadsPath)
	if err != nil {
		t.Skipf("quads not found at %s; run warp_align.py detect first", quadsPath)
	}
	var q warpAlignQuads
	if err := json.Unmarshal(raw, &q); err != nil {
		t.Fatalf("parse quads: %v", err)
	}
	for i, b := range q.Boxes {
		if len(b.Quad) != 8 {
			t.Fatalf("box %d: bad quad len %d", i, len(b.Quad))
		}
		pts := [4]util.Pt{
			{X: b.Quad[0], Y: b.Quad[1]},
			{X: b.Quad[2], Y: b.Quad[3]},
			{X: b.Quad[4], Y: b.Quad[5]},
			{X: b.Quad[6], Y: b.Quad[7]},
		}
		warp := util.WarpCrop(img, pts)
		if err := writeAlignPNG(filepath.Join(out, fmt.Sprintf("go_warp_%d.png", i)), warp); err != nil {
			t.Fatalf("write warp %d: %v", i, err)
		}
		x0 := int(math.Min(b.Quad[0], math.Min(b.Quad[2], math.Min(b.Quad[4], b.Quad[6]))))
		y0 := int(math.Min(b.Quad[1], math.Min(b.Quad[3], math.Min(b.Quad[5], b.Quad[7]))))
		x1 := int(math.Max(b.Quad[0], math.Max(b.Quad[2], math.Max(b.Quad[4], b.Quad[6]))))
		y1 := int(math.Max(b.Quad[1], math.Max(b.Quad[3], math.Max(b.Quad[5], b.Quad[7]))))
		fc := util.FastCrop(img, x0, y0, x1, y1)
		if err := writeAlignPNG(filepath.Join(out, fmt.Sprintf("go_fastcrop_%d.png", i)), fc); err != nil {
			t.Fatalf("write fastcrop %d: %v", i, err)
		}
	}
	t.Logf("wrote %d go crops (warp + fastcrop)", len(q.Boxes))
}

func writeAlignPNG(path string, img image.Image) error {
	b, err := util.EncodePNG(img)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
