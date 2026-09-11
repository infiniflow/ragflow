//go:build cgo && integration

package native

import (
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestNativeLoadsOrtModels proves the Go in-process DeepDoc backend consumes the
// FlatBuffer (.ort) model format, while the Python DeepDoc service keeps the
// legacy .onnx files. This is the contract: Go loads .ort, Python loads .onnx.
//
// It stages a model directory containing ONLY the .ort weights (mirroring a
// Go-only deployment where the Python .onnx copy is absent) and runs every
// recognizer against it. Each recognizer MUST succeed: before the loader is
// switched to .ort, get*Session opens det.onnx/layout.onnx/tsr.onnx/rec.onnx —
// which are absent, so the run errors and this test is RED until the switch
// lands. After the switch the loader opens det.ort/layout.ort/tsr.ort/rec.ort,
// which are present, so every recognizer runs to completion and the test goes
// GREEN.
func TestNativeLoadsOrtModels(t *testing.T) {
	skipIfNoModels(t)
	src := os.Getenv("MODEL_DIR")

	ortOnly := t.TempDir()
	for _, name := range []string{"det.ort", "layout.ort", "tsr.ort", "rec.ort", "ocr.res"} {
		if err := copyFile(filepath.Join(src, name), filepath.Join(ortOnly, name)); err != nil {
			t.Fatalf("stage %s: %v", name, err)
		}
	}

	if err := InitORT(); err != nil {
		t.Fatalf("InitORT: %v", err)
	}

	img := solidImage(256, 256)
	nimg, err := FromImage(img)
	if err != nil {
		t.Fatalf("FromImage: %v", err)
	}

	// Det must load det.ort (not det.onnx) and run to completion.
	if _, e := RunDet(t.Context(), ortOnly, nimg); e != nil {
		t.Fatalf("RunDet must load det.ort and succeed on the .ort-only directory, got: %v", e)
	}
	// Layout/DLA must load layout.ort.
	if _, e := RunDLA(t.Context(), ortOnly, nimg); e != nil {
		t.Fatalf("RunDLA must load layout.ort and succeed on the .ort-only directory, got: %v", e)
	}
	// TSR must load tsr.ort.
	if _, e := RunTSR(t.Context(), ortOnly, nimg); e != nil {
		t.Fatalf("RunTSR must load tsr.ort and succeed on the .ort-only directory, got: %v", e)
	}
	// OCR-rec must load rec.ort.
	if _, e := RunOCRRec(t.Context(), ortOnly, nimg); e != nil {
		t.Fatalf("RunOCRRec must load rec.ort and succeed on the .ort-only directory, got: %v", e)
	}
}

// solidImage returns a w×h mid-gray RGBA image so preprocessing has
// non-degenerate input without depending on fetched test fixtures.
func solidImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.Gray{Y: 128})
		}
	}
	return img
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
