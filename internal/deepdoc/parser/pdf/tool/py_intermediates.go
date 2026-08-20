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
	}
}

// ── Phase 3: OCR replay ────────────────────────────────────────────────────

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
