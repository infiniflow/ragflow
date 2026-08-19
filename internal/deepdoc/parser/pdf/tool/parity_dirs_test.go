package tool

import (
	"path/filepath"
	"testing"
)

// TestParityDirsForDefaultVariant pins the legacy directory layout: the
// default variant ("" or "ocr") must resolve exactly to the pre-variant
// hardcoded paths so existing dumps (charspy/, output/py/ocr/...) keep
// working untouched.
func TestParityDirsForDefaultVariant(t *testing.T) {
	for _, v := range []string{"", "ocr"} {
		d := ParityDirsFor(v)
		want := ParityDirs{
			Charspy: filepath.Join("testdata", "charspy"),
			Text:    filepath.Join("testdata", "output", "py", "ocr", "text"),
			DLA:     filepath.Join("testdata", "output", "py", "ocr", "dla"),
			TSRRaw:  filepath.Join("testdata", "output", "py", "ocr", "tsr_raw"),
			OCR:     filepath.Join("testdata", "output", "py", "ocr", "ocr"),
			Tables:  filepath.Join("testdata", "output", "py", "ocr", "tables"),
			GoText:  filepath.Join("testdata", "output", "go", "ocr", "text"),
		}
		if d != want {
			t.Errorf("ParityDirsFor(%q) = %+v, want %+v", v, d, want)
		}
	}
}

// TestParityDirsForCustomVariant pins the variant-isolated layout: a custom
// variant moves every artifact under its own subdirectory so a second dataset
// (e.g. real_pdfs) never collides with the legacy 35-PDF set.
func TestParityDirsForCustomVariant(t *testing.T) {
	d := ParityDirsFor("ocr_real")
	want := ParityDirs{
		Charspy: filepath.Join("testdata", "charspy_ocr_real"),
		Text:    filepath.Join("testdata", "output", "py", "ocr_real", "text"),
		DLA:     filepath.Join("testdata", "output", "py", "ocr_real", "dla"),
		TSRRaw:  filepath.Join("testdata", "output", "py", "ocr_real", "tsr_raw"),
		OCR:     filepath.Join("testdata", "output", "py", "ocr_real", "ocr"),
		Tables:  filepath.Join("testdata", "output", "py", "ocr_real", "tables"),
		GoText:  filepath.Join("testdata", "output", "go", "ocr_real", "text"),
	}
	if d != want {
		t.Errorf("ParityDirsFor(\"ocr_real\") = %+v, want %+v", d, want)
	}
}
