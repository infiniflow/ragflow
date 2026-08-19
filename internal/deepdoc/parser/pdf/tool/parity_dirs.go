package tool

import "path/filepath"

// ParityDirs holds every testdata directory the pipeline-parity harness
// reads/writes for one dataset variant. The variant isolates a second set of
// PDFs (e.g. real_pdfs/) from the legacy 35-PDF fixture set so dumps and
// parity verdicts never collide.
type ParityDirs struct {
	// Charspy is the Python pdfplumber chars dump directory.
	Charspy string
	// Text is Python's per-box text golden.
	Text string
	// DLA is Python's layout intermediate replay input.
	DLA string
	// TSRRaw is Python's table-structure-recognition replay input.
	TSRRaw string
	// OCR is Python's raw OCR detect replay input.
	OCR string
	// Tables is Python's table grid golden.
	Tables string
	// GoText is where the harness dumps Go output text for diffing (BATCH_PARITY_DUMP_GO).
	GoText string
}

// ParityDirsFor maps a dataset variant to its testdata directories. The
// default variant ("" or "ocr") resolves to the legacy hardcoded layout
// (charspy/, output/py/ocr/...), so existing dumps keep working untouched; a
// custom variant such as "ocr_real" moves every artifact under a variant
// suffix (charspy_ocr_real/, output/py/ocr_real/...) so two datasets never
// share a file.
func ParityDirsFor(variant string) ParityDirs {
	if variant == "" {
		variant = "ocr"
	}
	d := ParityDirs{
		Charspy: filepath.Join("testdata", "charspy"),
		Text:    filepath.Join("testdata", "output", "py", variant, "text"),
		DLA:     filepath.Join("testdata", "output", "py", variant, "dla"),
		TSRRaw:  filepath.Join("testdata", "output", "py", variant, "tsr_raw"),
		OCR:     filepath.Join("testdata", "output", "py", variant, "ocr"),
		Tables:  filepath.Join("testdata", "output", "py", variant, "tables"),
		GoText:  filepath.Join("testdata", "output", "go", variant, "text"),
	}
	if variant != "ocr" {
		d.Charspy = filepath.Join("testdata", "charspy_"+variant)
	}
	return d
}
