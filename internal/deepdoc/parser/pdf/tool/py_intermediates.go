package tool

import (
	"encoding/json"
	"fmt"
	"os"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// ── Python intermediate loaders ────────────────────────────────────────────
//
// These parse the JSON dumps produced by dump_py_results.py
// (output/py/ocr/{dla,tsr_raw}/{name}.pdf.json). The coordinates in those
// dumps are in PDF-point space (÷DlaScale); replay adapters multiply by
// pdf.DlaScale before feeding Go, which expects image-pixel space.

// PythonDLAPage holds one page's DLA layout regions.
type PythonDLAPage struct {
	Page    int
	Regions []PythonDLARegion
}

// PythonDLARegion mirrors a single DLA region in the dump.
// Coordinates are PDF points (top/left origin), NOT image pixels.
type PythonDLARegion struct {
	Type   string
	X0     float64
	X1     float64
	Top    float64
	Bottom float64
}

// LoadPythonDLA parses output/py/ocr/dla/{name}.pdf.json into per-page
// DLA regions. The file name uses the .pdf.json suffix produced by the
// dump script (name already includes the .pdf extension).
func LoadPythonDLA(jsonPath string) ([]PythonDLAPage, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("read dla json: %w", err)
	}
	var pages []PythonDLAPage
	if err := json.Unmarshal(data, &pages); err != nil {
		return nil, fmt.Errorf("parse dla json: %w", err)
	}
	return pages, nil
}

// PythonTSRCell mirrors one raw TSR component in the dump (table / table
// column / table row / table column header / table spanning cell ...).
// Coordinates are PDF points (top/left origin), NOT image pixels.
//
// Note: Y coordinates are PAGE-CUMULATIVE — Python's
// _map_tsr_component_to_page_space (pdf_parser.py:572-573) adds
// page_cum_height[page] to top/bottom. Replay adapters must subtract the
// cumulative offset (derived from charspy page dims) before mapping into
// Go's crop space.
type PythonTSRCell struct {
	TableIndex int     `json:"table_index"`
	Page       int     `json:"page"`
	Label      string  `json:"label"`
	X0         float64 `json:"x0"`
	Y0         float64 `json:"y0"`
	X1         float64 `json:"x1"`
	Y1         float64 `json:"y1"`
	Text       string  `json:"text"`
	// Score is the detection confidence. Python's layouts_cleanup keeps the
	// higher-score line when two overlap (recognizer.py:141); it is required
	// to reproduce the exact structure-line cleanup.
	Score float64 `json:"score"`
}

// LoadPythonTSR parses output/py/ocr/tsr_raw/{name}.pdf.json into the raw
// TSR component list. The replay TableBuilder filters by (page, table_index)
// and maps each component into Go's crop space.
func LoadPythonTSR(jsonPath string) ([]PythonTSRCell, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("read tsr json: %w", err)
	}
	var cells []PythonTSRCell
	if err := json.Unmarshal(data, &cells); err != nil {
		return nil, fmt.Errorf("parse tsr json: %w", err)
	}
	return cells, nil
}

// ToDLARegion converts a Python PDF-point DLA region into a Go image-pixel
// DLARegion (×DlaScale). Confidence is pinned high so the replay does not
// trip the garbage-layout gate that Python never applied to these dumps.
func (r PythonDLARegion) ToDLARegion() pdf.DLARegion {
	return pdf.DLARegion{
		X0:         r.X0 * pdf.DlaScale,
		Y0:         r.Top * pdf.DlaScale,
		X1:         r.X1 * pdf.DlaScale,
		Y1:         r.Bottom * pdf.DlaScale,
		Label:      r.Type,
		Confidence: 1.0,
	}
}

// ToTSRCell converts a Python PDF-point TSR component into a Go TSRCell in
// crop space. Python's Y is page-cumulative (page_cum_height added in
// _map_tsr_component_to_page_space), so it must first be reduced by
// cumOffsetPx — the sum of prior page image heights in image pixels, i.e.
// page_cum_height × DlaScale — to land in page-local points, then ×DlaScale
// and shifted by the crop origin (image pixels) so it shares the frame of
// Go's boxInCrop.
func (c PythonTSRCell) ToTSRCell(cropOffX, cropOffY, cumOffsetPx float64) pdf.TSRCell {
	return pdf.TSRCell{
		X0:    c.X0*pdf.DlaScale - cropOffX,
		Y0:    c.Y0*pdf.DlaScale - cumOffsetPx - cropOffY,
		X1:    c.X1*pdf.DlaScale - cropOffX,
		Y1:    c.Y1*pdf.DlaScale - cumOffsetPx - cropOffY,
		Label: c.Label,
		Text:  c.Text,
		Score: c.Score,
	}
}

// ── Phase 3: OCR replay ────────────────────────────────────────────────────

// PythonAllBox is one entry of the table_boxes/{name}.all_boxes.json snapshot:
// a page box (table or non-table) present at R/C-annotation time, carrying
// only the coordinates Python's layouts_cleanup area branch needs.
type PythonAllBox struct {
	X0         float64 `json:"x0"`
	X1         float64 `json:"x1"`
	Top        float64 `json:"top"`
	Bottom     float64 `json:"bottom"`
	PageNumber int     `json:"page_number"`
}

// LoadPythonAllBoxes parses output/py/{variant}/table_boxes/{name}.all_boxes.json
// (the full page box set at R/C-annotation time) into []pdf.TextBox. A missing
// file is reported via error so callers can fall back to the table boxes only.
func LoadPythonAllBoxes(jsonPath string) ([]pdf.TextBox, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}
	var dumped []PythonAllBox
	if err := json.Unmarshal(data, &dumped); err != nil {
		return nil, err
	}
	boxes := make([]pdf.TextBox, 0, len(dumped))
	for _, b := range dumped {
		boxes = append(boxes, pdf.TextBox{
			X0:         b.X0,
			X1:         b.X1,
			Top:        b.Top,
			Bottom:     b.Bottom,
			PageNumber: b.PageNumber,
		})
	}
	return boxes, nil
}

// PythonOCRPage holds one page's final OCR-derived text boxes (the box list
// __ocr appends to parser.boxes). The coordinates are PAGE-POINTS but
// PAGE-CUMULATIVE — Python's __ocr runs on page images stacked at
// page_cum_height offsets, so box Y includes the sum of prior page heights.
// Replay adapters must subtract the cumulative offset (derived from charspy
// page dims) before mapping into Go's page-local image space.
type PythonOCRPage struct {
	Page  int
	Boxes []PythonOCRBox
}

// ── Per-char R/C dump (table_boxes/) ──────────────────────────────────────
// table_boxes/{name}.json carries each table cell box WITH its TSR-assigned
// R/C/H/SP annotations (the authoritative per-char row/column assignment
// Python's construct_table groups by). This is the signal Go's line-based
// GroupCells cross-product ignores; the replay harness feeds it to
// GroupBoxesByRC so Go's assembly matches Python's R/C view.
//
// The dump is a FLAT list of boxes (one object per table cell), not
// page-wrapped. Each box carries page_number (1-based) and layoutno (the
// per-page table key, e.g. "table-0") so callers can split boxes back into
// per-table groups.

// PythonTableBox mirrors one table-cell box with per-char R/C/H/SP labels.
// Field names follow the dump's JSON keys exactly.
type PythonTableBox struct {
	X0         float64 `json:"x0"`
	X1         float64 `json:"x1"`
	Top        float64 `json:"top"`
	Bottom     float64 `json:"bottom"`
	Text       string  `json:"text"`
	PageNumber int     `json:"page_number"`
	LayoutNo   string  `json:"layoutno"`
	R          int     `json:"R"`
	C          int     `json:"C"`
	H          int     `json:"H"`
	SP         int     `json:"SP"`
	LayoutType string  `json:"layout_type"`
}

// LoadPythonTableBoxes parses output/py/{variant}/table_boxes/{name}.json
// (a flat box list) into a []pdf.TextBox with R/C/H/SP carried over. A
// missing file is reported via error so callers can treat "no R/C dump" as a
// no-op (e.g. PDFs without tables, or the old ocr_real dump before the R/C
// capture was added).
func LoadPythonTableBoxes(jsonPath string) ([]pdf.TextBox, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}
	var dumped []PythonTableBox
	if err := json.Unmarshal(data, &dumped); err != nil {
		return nil, err
	}
	boxes := make([]pdf.TextBox, 0, len(dumped))
	for _, b := range dumped {
		boxes = append(boxes, pdf.TextBox{
			X0:         b.X0,
			X1:         b.X1,
			Top:        b.Top,
			Bottom:     b.Bottom,
			Text:       b.Text,
			R:          b.R,
			C:          b.C,
			H:          b.H,
			SP:         b.SP,
			PageNumber: b.PageNumber,
			LayoutNo:   b.LayoutNo,
			LayoutType: b.LayoutType,
		})
	}
	return boxes, nil
}

// PythonOCRBox mirrors one final OCR text box: axis-aligned bbox in
// page-cumulative points, plus the assembled recognized text. Confidence is
// not preserved in the dump (the assembled boxes do not carry it), so replay
// uses 0.
type PythonOCRBox struct {
	X0   float64
	Y0   float64
	X1   float64
	Y1   float64
	Text string
	Conf float64
}

// LoadPythonOCR parses output/py/ocr/ocr/{name}.pdf.json into per-page final
// OCR text boxes.
func LoadPythonOCR(jsonPath string) ([]PythonOCRPage, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("read ocr json: %w", err)
	}
	var pages []PythonOCRPage
	if err := json.Unmarshal(data, &pages); err != nil {
		return nil, fmt.Errorf("parse ocr json: %w", err)
	}
	return pages, nil
}

// ToOCRBox converts a Python page-cumulative-point OCR box into a Go OCRBox
// quad in page-local image-pixel space. The cumulative offset (sum of prior
// page image heights in image pixels, i.e. page_cum_height × DlaScale) is
// subtracted from Y first so boxes land on the current page, then ×DlaScale.
// The quad is the axis-aligned rectangle itself, so WarpCrop receives an
// identity de-skew and the emitted bounds match the dump's bbox after the
// /DlaScale conversion in ocrDetectAndRecognize.
func (b PythonOCRBox) ToOCRBox(cumOffsetPx float64) pdf.OCRBox {
	return pdf.OCRBox{
		X0: b.X0 * pdf.DlaScale, Y0: b.Y0*pdf.DlaScale - cumOffsetPx,
		X1: b.X1 * pdf.DlaScale, Y1: b.Y0*pdf.DlaScale - cumOffsetPx,
		X2: b.X1 * pdf.DlaScale, Y2: b.Y1*pdf.DlaScale - cumOffsetPx,
		X3: b.X0 * pdf.DlaScale, Y3: b.Y1*pdf.DlaScale - cumOffsetPx,
	}
}
