package tool

import (
	"path/filepath"
	"testing"
)

// TestParityDirsForDefaultVariant pins the default directory layout: the
// default variant ("" or "ocr") must resolve exactly to the built-in paths
// (charspy/, output/py/ocr/...).
func TestParityDirsForDefaultVariant(t *testing.T) {
	for _, v := range []string{"", "ocr"} {
		d := ParityDirsFor(v)
		want := ParityDirs{
			Charspy:    filepath.Join("testdata", "charspy"),
			Text:       filepath.Join("testdata", "output", "py", "ocr", "text"),
			DLA:        filepath.Join("testdata", "output", "py", "ocr", "dla"),
			TSRRaw:     filepath.Join("testdata", "output", "py", "ocr", "tsr_raw"),
			OCR:        filepath.Join("testdata", "output", "py", "ocr", "ocr"),
			Tables:     filepath.Join("testdata", "output", "py", "ocr", "tables"),
			TableBoxes: filepath.Join("testdata", "output", "py", "ocr", "table_boxes"),
			GoText:     filepath.Join("testdata", "output", "go", "ocr", "text"),
		}
		if d != want {
			t.Errorf("ParityDirsFor(%q) = %+v, want %+v", v, d, want)
		}
	}
}

// TestParityDirsForCustomVariant pins the variant-isolated layout: a custom
// variant moves every artifact under its own subdirectory so a second dataset
// (e.g. real_pdfs) never collides with the default 35-PDF set.
func TestParityDirsForCustomVariant(t *testing.T) {
	// Isolate from any ambient BATCH_PARITY_DATA_ROOT so this test pins the
	// local testdata layout regardless of the environment it runs in.
	t.Setenv("BATCH_PARITY_DATA_ROOT", "")
	d := ParityDirsFor("ocr_real")
	want := ParityDirs{
		Charspy:    filepath.Join("testdata", "charspy_ocr_real"),
		Text:       filepath.Join("testdata", "output", "py", "ocr_real", "text"),
		DLA:        filepath.Join("testdata", "output", "py", "ocr_real", "dla"),
		TSRRaw:     filepath.Join("testdata", "output", "py", "ocr_real", "tsr_raw"),
		OCR:        filepath.Join("testdata", "output", "py", "ocr_real", "ocr"),
		Tables:     filepath.Join("testdata", "output", "py", "ocr_real", "tables"),
		TableBoxes: filepath.Join("testdata", "output", "py", "ocr_real", "table_boxes"),
		GoText:     filepath.Join("testdata", "output", "go", "ocr_real", "text"),
	}
	if d != want {
		t.Errorf("ParityDirsFor(\"ocr_real\") = %+v, want %+v", d, want)
	}
}

// TestParityDirsForCustomVariantDataRoot pins the shared-data-root redirect:
// when BATCH_PARITY_DATA_ROOT is set, a custom variant resolves every
// artifact under that root (so multiple worktrees reuse one dump). The
// default variant must ignore the env and stay under local testdata.
func TestParityDirsForCustomVariantDataRoot(t *testing.T) {
	t.Setenv("BATCH_PARITY_DATA_ROOT", "/srv/parity-data")
	d := ParityDirsFor("ocr_real")
	want := ParityDirs{
		Charspy:    filepath.Join("/srv/parity-data", "charspy_ocr_real"),
		Text:       filepath.Join("/srv/parity-data", "output", "py", "ocr_real", "text"),
		DLA:        filepath.Join("/srv/parity-data", "output", "py", "ocr_real", "dla"),
		TSRRaw:     filepath.Join("/srv/parity-data", "output", "py", "ocr_real", "tsr_raw"),
		OCR:        filepath.Join("/srv/parity-data", "output", "py", "ocr_real", "ocr"),
		Tables:     filepath.Join("/srv/parity-data", "output", "py", "ocr_real", "tables"),
		TableBoxes: filepath.Join("/srv/parity-data", "output", "py", "ocr_real", "table_boxes"),
		GoText:     filepath.Join("/srv/parity-data", "output", "go", "ocr_real", "text"),
	}
	if d != want {
		t.Errorf("ParityDirsFor(\"ocr_real\") with BATCH_PARITY_DATA_ROOT = %+v, want %+v", d, want)
	}

	// Default variant must NOT be redirected by the env.
	def := ParityDirsFor("")
	if def.Charspy != filepath.Join("testdata", "charspy") {
		t.Errorf("default variant Charspy = %q, want %q (env must be ignored)", def.Charspy, filepath.Join("testdata", "charspy"))
	}
}
