//go:build cgo && manual

package pdf

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tbl "ragflow/internal/deepdoc/parser/pdf/table"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// TestCropOrientationAB is the "Go OCR" column of the 2x2 parity experiment,
// now exercising the FIXED orientation path.
//
// It runs the real Go pipeline on table_rotation_test.pdf page 0 (pdfium
// render + DLA crop + EvaluateTableOrientation). EvaluateTableOrientation now
// mirrors Python's _evaluate_table_orientation: it calls OCRDetect to find the
// table's text lines, warps each line, and recognizes it individually, then
// scores by the per-line mean confidence. This replaces the previous bug where
// the whole table was sent to a single rec call (no detection) and collapsed
// to one line.
//
// For each detected table it:
//   - saves go_crop_<i>.png (the crop the Go pipeline actually feeds OCR)
//   - logs Go-on-Go orientation (page 0 tables are upright → expected 0°)
//   - if py_crop_<i>.png exists (produced by the Python script), logs
//     Go-on-Py orientation, holding the Go OCR mode fixed so the only
//     variable is the crop.
//
// Prereqs: in-process DeepDoc backend (MODEL_DIR set; ONNX Runtime statically linked) + CGO native libs.
// Run: bash build.sh --test-native ./internal/deepdoc/parser/pdf/ -run TestCropOrientationAB -v
const abOutDir = "/tmp/orient_ab"

func TestCropOrientationAB(t *testing.T) {
	if err := os.MkdirAll(abOutDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pdfPath := filepath.Join("testdata", "pdfs", "table_rotation_test.pdf")
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		t.Skipf("test PDF not found: %s", pdfPath)
	}

	dd := mustConnectInProcessAnalyzer(t)

	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	eng, err := NewEngine(data)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	ctx := t.Context()
	pageImg, err := RenderPageToImage(eng, 0)
	if err != nil {
		t.Fatal(err)
	}

	regions, err := dd.DLA(ctx, pageImg)
	if err != nil {
		t.Fatal(err)
	}

	ti := 0
	for _, r := range regions {
		if r.Label != "table" {
			continue
		}
		ti++
		cropped, err := util.CropImageRegion(pageImg, r)
		if err != nil {
			t.Errorf("crop table %d: %v", ti, err)
			continue
		}
		goPath := filepath.Join(abOutDir, "go_crop_"+strconv.Itoa(ti)+".png")
		f, e := os.Create(goPath)
		if e != nil {
			t.Logf("create %s: %v", goPath, e)
		} else {
			if err := png.Encode(f, cropped); err != nil {
				t.Logf("encode %s: %v", goPath, err)
			}
			if err := f.Close(); err != nil {
				t.Logf("close %s: %v", goPath, err)
			}
		}
		angle, _, scores := tbl.EvaluateTableOrientation(ctx, cropped, dd)
		t.Logf("[Go-on-Go] crop#%d %dx%d saved=%s: angle=%d scores 0=%.3f 90=%.3f 180=%.3f 270=%.3f",
			ti, cropped.Bounds().Dx(), cropped.Bounds().Dy(), goPath,
			angle, scores[0], scores[90], scores[180], scores[270])
		// Page 0 tables are upright; the fixed orientation path must not
		// mis-rotate them. Surface a regression instead of only logging.
		if angle != 0 {
			t.Errorf("[Go-on-Go] crop#%d: got angle %d, want 0", ti, angle)
		}
	}
	if ti == 0 {
		t.Error("DLA detected no table regions on the regression page; the Go-on-Go probe exercised nothing")
	}

	// Go-on-Py: read Python's crops, hold the Go OCR mode fixed.
	entries, _ := os.ReadDir(abOutDir)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "py_crop_") || !strings.HasSuffix(name, ".png") {
			continue
		}
		f, err := os.Open(filepath.Join(abOutDir, name))
		if err != nil {
			continue
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Errorf("decode %s: %v", name, err)
			continue
		}
		angle, _, scores := tbl.EvaluateTableOrientation(ctx, img.(image.Image), dd)
		t.Logf("[Go-on-Py] %s %dx%d: angle=%d scores 0=%.3f 90=%.3f 180=%.3f 270=%.3f",
			name, img.Bounds().Dx(), img.Bounds().Dy(),
			angle, scores[0], scores[90], scores[180], scores[270])
	}
}
