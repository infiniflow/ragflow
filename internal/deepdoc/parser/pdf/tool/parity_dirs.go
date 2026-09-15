package tool

import (
	"path/filepath"

	"ragflow/internal/common"
)

// ParityDirs holds every testdata directory the pipeline-parity harness
// reads/writes for one dataset variant. The variant isolates a second set of
// PDFs (e.g. real_pdfs/) from the default 35-PDF fixture set so dumps and
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
	// TableBoxes is Python's per-char R/C table-cell dump (table_boxes/),
	// used by the row-segmentation replay to feed authoritative R/C labels
	// into Go's R/C-based grouping (GroupBoxesByRC).
	TableBoxes string
	// GoText is where the harness dumps Go output text for diffing (BATCH_PARITY_DUMP_GO).
	GoText string
}

// ParityDirsFor maps a dataset variant to its testdata directories. The
// default variant ("" or "ocr") resolves to the built-in layout (charspy/,
// output/py/ocr/...); a custom variant such as "ocr_real" moves every artifact
// under a variant suffix (charspy_ocr_real/, output/py/ocr_real/...) so two
// datasets never share a file.
//
// For a custom variant the root directory can be redirected via
// BATCH_PARITY_DATA_ROOT (e.g. a shared directory outside the worktree so
// multiple worktrees reuse one dump). The default variant always stays under
// the local "testdata".
func ParityDirsFor(variant string) ParityDirs {
	if variant == "" {
		variant = "ocr"
	}
	root := "testdata"
	if variant != "ocr" {
		if r := common.GetEnv(common.EnvBatchParityDataRoot); r != "" {
			root = r
		}
	}
	// The charspy dir is the only artifact whose name does not carry the
	// variant suffix for the default variant (charspy/), matching the layout
	// the dumps were generated with; custom variants append the suffix.
	charspyDir := "charspy"
	if variant != "ocr" {
		charspyDir = "charspy_" + variant
	}
	return ParityDirs{
		Charspy:    filepath.Join(root, charspyDir),
		Text:       filepath.Join(root, "output", "py", variant, "text"),
		DLA:        filepath.Join(root, "output", "py", variant, "dla"),
		TSRRaw:     filepath.Join(root, "output", "py", variant, "tsr_raw"),
		OCR:        filepath.Join(root, "output", "py", variant, "ocr"),
		Tables:     filepath.Join(root, "output", "py", variant, "tables"),
		TableBoxes: filepath.Join(root, "output", "py", variant, "table_boxes"),
		GoText:     filepath.Join(root, "output", "go", variant, "text"),
	}
}
