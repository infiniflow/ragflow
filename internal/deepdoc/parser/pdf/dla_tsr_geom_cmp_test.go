//go:build cgo && integration

package pdf

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"ragflow/internal/deepdoc/parser/pdf/tool"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// cmpBox is a bbox in PDF page-point space (1pt = 1/72in).
type cmpBox struct{ x0, y0, x1, y1 float64 }

// MarshalJSON emits the corner coordinates explicitly: the struct fields are
// unexported, so the default json marshaler would skip them and write `{}`.
func (b cmpBox) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		X0, Y0, X1, Y1 float64
	}{b.x0, b.y0, b.x1, b.y1})
}

// TestDLATSRGeomCompareReal measures the geometric divergence between Go's
// PRODUCTION TSR (pdfium render -> DEEPDOC_URL) and Python's production TSR
// (dumped by dump_py_results.py into tsr_raw/) for ONE real PDF, so we can
// answer "do the two sides feed the assembly layer different geometry, and by
// how much?" with measured numbers instead of code-path inference.
//
// Usage (needs a live DEEPDOC_URL inference service; see AGENTS.md `integration` tier):
//
//	TSR_CMP_PDF=/abs/path/to.pdf \
//	TSR_CMP_PY_TSR=/abs/path/to/py_tsr_raw.json \
//	bash build.sh --test-integration ./internal/deepdoc/parser/pdf/ -run TestDLATSRGeomCompareReal
//
// Both sides are mapped into PDF page-point space and matched greedily by
// cell-center proximity. We report cell-count delta, mean/max center offset
// (points), and mean IoU — the actual geometric gap.
func TestDLATSRGeomCompareReal(t *testing.T) {
	pdfPath := os.Getenv("TSR_CMP_PDF")
	pyTSRPath := os.Getenv("TSR_CMP_PY_TSR")
	if pdfPath == "" || pyTSRPath == "" {
		t.Skip("set TSR_CMP_PDF and TSR_CMP_PY_TSR")
	}

	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("read pdf: %v", err)
	}
	eng, err := NewEngine(data)
	if err != nil {
		t.Fatalf("open engine: %v", err)
	}
	defer eng.Close()

	client := mustConnectInferenceClient(t)
	ctx := t.Context()

	pageImg, err := RenderPageToImage(eng, 0)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	regions, err := client.DLA(ctx, pageImg)
	if err != nil {
		t.Fatalf("DLA: %v", err)
	}
	var tr *pdf.DLARegion
	for i := range regions {
		if regions[i].Label == "table" {
			tr = &regions[i]
			break
		}
	}
	if tr == nil {
		t.Fatal("no table region detected by DLA")
	}
	t.Logf("Go DLA table region (full-page px): [%.1f, %.1f, %.1f, %.1f]",
		tr.X0, tr.Y0, tr.X1, tr.Y1)

	cropped := cropImageRect(pageImg,
		int(tr.X0), int(tr.Y0), int(tr.X1), int(tr.Y1))
	goCells, err := client.TSR(ctx, cropped)
	if err != nil {
		t.Fatalf("TSR: %v", err)
	}

	scale := pdf.DlaScale
	var goBoxes []cmpBox
	for _, c := range goCells {
		goBoxes = append(goBoxes, cmpBox{
			(c.X0 + tr.X0) / scale,
			(c.Y0 + tr.Y0) / scale,
			(c.X1 + tr.X0) / scale,
			(c.Y1 + tr.Y0) / scale,
		})
	}

	pyCells, err := tool.LoadPythonTSR(pyTSRPath)
	if err != nil {
		t.Fatalf("load py tsr: %v", err)
	}
	var pyBoxes []cmpBox
	for _, c := range pyCells {
		if c.Page != 0 {
			continue
		}
		pyBoxes = append(pyBoxes, cmpBox{c.X0, c.Y0, c.X1, c.Y1})
	}

	t.Logf("Go cells (page0, mapped to points) = %d", len(goBoxes))
	t.Logf("Python cells (page0) = %d", len(pyBoxes))

	// Greedy nearest-center matching: each Python box -> closest unused Go box.
	pairs := matchNearest(pyBoxes, goBoxes)

	if len(pairs) == 0 {
		t.Fatalf("no matched cells — impossible to compare geometry")
	}
	var sumD, maxD, sumIoU float64
	for _, p := range pairs {
		sumD += p.dCenter
		if p.dCenter > maxD {
			maxD = p.dCenter
		}
		sumIoU += p.iou
	}
	meanD := sumD / float64(len(pairs))
	meanIoU := sumIoU / float64(len(pairs))
	unmatched := len(pyBoxes) + len(goBoxes) - 2*len(pairs)

	t.Logf("Matched pairs        = %d", len(pairs))
	t.Logf("Unmatched cells       = %d", unmatched)
	t.Logf("Mean center offset    = %.2f pt", meanD)
	t.Logf("Max  center offset    = %.2f pt", maxD)
	t.Logf("Mean IoU              = %.3f", meanIoU)

	// The raw offset above includes the DLA table-REGION detection gap (Go's
	// DLA crop may be larger/shifted vs Python's table extent). To isolate the
	// TSR CELL-GRID geometry itself, align each side's overall table bbox to a
	// common origin (translation only — both tables are ~same size) and re-measure.
	goB := bboxOf(goBoxes)
	pyB := bboxOf(pyBoxes)
	t.Logf("Go table extent (pt)    = %.0f x %.0f", goB.x1-goB.x0, goB.y1-goB.y0)
	t.Logf("Py table extent (pt)    = %.0f x %.0f", pyB.x1-pyB.x0, pyB.y1-pyB.y0)
	shiftX, shiftY := pyB.x0-goB.x0, pyB.y0-goB.y0
	var goNorm []cmpBox
	for _, b := range goBoxes {
		goNorm = append(goNorm, cmpBox{b.x0 + shiftX, b.y0 + shiftY, b.x1 + shiftX, b.y1 + shiftY})
	}
	// Re-match in the aligned frame.
	pairsN := matchNearest(pyBoxes, goNorm)
	var sumDN, maxDN, sumIoUN float64
	for _, p := range pairsN {
		sumDN += p.dCenter
		if p.dCenter > maxDN {
			maxDN = p.dCenter
		}
		sumIoUN += p.iou
	}
	t.Logf("--- after aligning table bboxes (isolates TSR cell-grid) ---")
	t.Logf("Aligned mean center offset = %.2f pt", sumDN/float64(len(pairsN)))
	t.Logf("Aligned max  center offset = %.2f pt", maxDN)
	t.Logf("Aligned mean IoU           = %.3f", sumIoUN/float64(len(pairsN)))

	// Surface the worst 5 matched pairs (aligned frame) so the gap is concrete.
	sort.Slice(pairsN, func(i, j int) bool { return pairsN[i].dCenter > pairsN[j].dCenter })
	for i := 0; i < len(pairsN) && i < 5; i++ {
		w := pairsN[i]
		t.Logf("  worst#%d off=%.1fpt  PY=[%.0f,%.0f,%.0f,%.0f]  GO=[%.0f,%.0f,%.0f,%.0f]",
			i+1, w.dCenter,
			w.py.x0, w.py.y0, w.py.x1, w.py.y1,
			w.goC.x0, w.goC.y0, w.goC.x1, w.goC.y1)
	}

	outDir := os.Getenv("TSR_CMP_OUT")
	if outDir == "" {
		outDir = filepath.Join("testdata", "output", "render_compare")
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Logf("warn: mkdir %s: %v", outDir, err)
	}
	if b, err := json.MarshalIndent(goBoxes, "", "  "); err != nil {
		t.Logf("warn: marshal go boxes: %v", err)
	} else if err := os.WriteFile(filepath.Join(outDir, "go_tsr_real_points.json"), b, 0644); err != nil {
		t.Logf("warn: write go_tsr_real_points.json: %v", err)
	}
}

// TestSameImageTSRConsistency isolates the ONE variable the user asked about:
// "for the SAME image sent to the inference service, do Go and Python get
// consistent TSR?" Go's client.TSR internally does util.EncodePNG(cropped)
// then POSTs multipart(field "request", file "tsr.png") to /predict/tsr. We
// save that EXACT PNG the Go client would send, then a Python caller POSTs the
// same bytes to the same endpoint; the bboxes must be identical (the service
// is stateless + deterministic, caller-agnostic). Any real-PDF Go-vs-Python
// TSR gap therefore comes from the INPUT IMAGE differing (pdfium vs
// pdfplumber), not from the service treating callers differently.
//
// Usage (needs a live DEEPDOC_URL inference service; see AGENTS.md `integration` tier):
//
//	TSR_CMP_PDF=/abs/path/to.pdf \
//	bash build.sh --test-integration ./internal/deepdoc/parser/pdf/ -run TestSameImageTSRConsistency
//
// then run tools-py/cmp_same_image_tsr.py (reads the saved PNG + go json).
func TestSameImageTSRConsistency(t *testing.T) {
	pdfPath := os.Getenv("TSR_CMP_PDF")
	if pdfPath == "" {
		t.Skip("set TSR_CMP_PDF")
	}
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("read pdf: %v", err)
	}
	eng, err := NewEngine(data)
	if err != nil {
		t.Fatalf("open engine: %v", err)
	}
	defer eng.Close()

	client := mustConnectInferenceClient(t)
	ctx := t.Context()
	pageImg, err := RenderPageToImage(eng, 0)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	regions, err := client.DLA(ctx, pageImg)
	if err != nil {
		t.Fatalf("DLA: %v", err)
	}
	var tr *pdf.DLARegion
	for i := range regions {
		if regions[i].Label == "table" {
			tr = &regions[i]
			break
		}
	}
	if tr == nil {
		t.Fatal("no table region detected by DLA")
	}
	cropped := cropImageRect(pageImg, int(tr.X0), int(tr.Y0), int(tr.X1), int(tr.Y1))

	// Output dir: override with TSR_CMP_OUT (the default testdata dir may not
	// be writable under the current user); the Python comparator reads the same.
	outDir := os.Getenv("TSR_CMP_OUT")
	if outDir == "" {
		outDir = filepath.Join("testdata", "output", "render_compare")
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Logf("warn: mkdir %s: %v", outDir, err)
	}

	// The exact bytes Go's client.TSR sends to the service.
	pngBytes, err := util.EncodePNG(cropped)
	if err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "tsr_same_input.png"), pngBytes, 0644); err != nil {
		t.Fatalf("write png: %v", err)
	}
	t.Logf("Saved exact PNG Go sends (%d bytes, %dx%d) → %s/tsr_same_input.png",
		len(pngBytes), cropped.Bounds().Dx(), cropped.Bounds().Dy(), outDir)

	// Go's TSR result in the PNG's own pixel space (no coordinate transform).
	goCells, err := client.TSR(ctx, cropped)
	if err != nil {
		t.Fatalf("TSR: %v", err)
	}
	type rawCell struct {
		X0, Y0, X1, Y1 float64
		Label          string
		Score          float64
	}
	out := make([]rawCell, 0, len(goCells))
	for _, c := range goCells {
		out = append(out, rawCell{c.X0, c.Y0, c.X1, c.Y1, c.Label, c.Score})
	}
	writeJSON(t, filepath.Join(outDir, "go_tsr_same_px.json"), out)
	t.Logf("Go TSR cells (PNG px) = %d → %s/go_tsr_same_px.json", len(out), outDir)
	t.Logf("Now run: python3 internal/deepdoc/parser/pdf/tool-py/cmp_same_image_tsr.py %s", outDir)
}

// geomPair is one greedily-matched Python↔Go table cell.
type geomPair struct {
	dCenter float64
	iou     float64
	py, goC cmpBox
}

// matchNearest greedily matches each Python box to the closest unused Go box
// (by cell-center distance) and returns the matched pairs. Both slices must be
// expressed in the same coordinate space before calling.
func matchNearest(pyBoxes, goBoxes []cmpBox) []geomPair {
	var pairs []geomPair
	used := make([]bool, len(goBoxes))
	for _, pb := range pyBoxes {
		pcx, pcy := boxCenter(pb)
		best := -1
		var bestD float64
		for j, gb := range goBoxes {
			if used[j] {
				continue
			}
			gcx, gcy := boxCenter(gb)
			d := math.Hypot(pcx-gcx, pcy-gcy)
			if best == -1 || d < bestD {
				best = j
				bestD = d
			}
		}
		if best == -1 {
			continue
		}
		used[best] = true
		pairs = append(pairs, geomPair{
			dCenter: bestD,
			iou:     boxIoU(pb, goBoxes[best]),
			py:      pb,
			goC:     goBoxes[best],
		})
	}
	return pairs
}

func boxCenter(b cmpBox) (float64, float64) {
	return (b.x0 + b.x1) / 2, (b.y0 + b.y1) / 2
}

// bboxOf returns the overall bounding box enclosing all boxes.
func bboxOf(boxes []cmpBox) cmpBox {
	b := boxes[0]
	for _, x := range boxes[1:] {
		if x.x0 < b.x0 {
			b.x0 = x.x0
		}
		if x.y0 < b.y0 {
			b.y0 = x.y0
		}
		if x.x1 > b.x1 {
			b.x1 = x.x1
		}
		if x.y1 > b.y1 {
			b.y1 = x.y1
		}
	}
	return b
}

func boxIoU(a, b cmpBox) float64 {
	ix0 := math.Max(a.x0, b.x0)
	iy0 := math.Max(a.y0, b.y0)
	ix1 := math.Min(a.x1, b.x1)
	iy1 := math.Min(a.y1, b.y1)
	iw := ix1 - ix0
	ih := iy1 - iy0
	if iw <= 0 || ih <= 0 {
		return 0
	}
	inter := iw * ih
	areaA := (a.x1 - a.x0) * (a.y1 - a.y0)
	areaB := (b.x1 - b.x0) * (b.y1 - b.y0)
	return inter / (areaA + areaB - inter)
}
