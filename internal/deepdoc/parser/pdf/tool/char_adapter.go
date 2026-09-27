package tool

import (
	"encoding/json"
	"fmt"
	"image"
	"os"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// PythonCharEngine implements pdf.PDFEngine by loading chars from a
// charspy/{pdf}.json file exported by dump_py_results.py.
// It is used for pipeline parity testing — same input chars as Python,
// so any difference in pipeline output is a Go pipeline logic bug.
type PythonCharEngine struct {
	chars     map[int][]pdf.TextChar // pageNum → chars
	dims      map[int][2]int         // pageNum → [w,h] in ZM/image pixels (optional)
	isEnglish *bool                  // document-level verdict captured from the same Python run
	pages     int
}

// LoadPythonChars loads chars from a charspy/{name}.json file.
func LoadPythonChars(jsonPath string) (*PythonCharEngine, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("read charspy json: %w", err)
	}
	var wrapper struct {
		Pages [][]struct {
			Text     string  `json:"text"`
			X0       float64 `json:"x0"`
			X1       float64 `json:"x1"`
			Top      float64 `json:"top"`
			Bottom   float64 `json:"bottom"`
			FontName string  `json:"fontname"`
			Size     float64 `json:"size"`
		} `json:"pages"`
		// dims carries per-page image dimensions in ZM/image pixels, written
		// by the Phase 3 dump script so image-only pages (no embedded chars)
		// get a correctly sized placeholder page image. REQUIRED: a missing
		// dim would fall back to a 1x1 placeholder, corrupting pageHeight and
		// pushing every box off-page — LoadPythonChars fails fast instead.
		Dims [][]int `json:"dims"`
		// IsEnglish is the document-level english verdict from the SAME Python
		// run that produced the text golden. Capturing it (instead of
		// re-deriving via random sampling) keeps the replay input identical to
		// Python's. Optional: older dumps omit it.
		IsEnglish *bool `json:"is_english"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("parse charspy json: %w", err)
	}
	// dims are required (1:1 with pages) so image-only pages never fall back
	// to a degenerate placeholder that corrupts pageHeight. Fail fast on a bad
	// dump instead of silently degrading parity.
	if len(wrapper.Dims) != len(wrapper.Pages) {
		return nil, fmt.Errorf("charspy json dims count %d != pages %d (regenerate with dump_py_results.py)", len(wrapper.Dims), len(wrapper.Pages))
	}
	for i, d := range wrapper.Dims {
		if len(d) < 2 || d[0] <= 0 || d[1] <= 0 {
			return nil, fmt.Errorf("charspy json page %d invalid dims: %v", i, d)
		}
	}

	chars := make(map[int][]pdf.TextChar, len(wrapper.Pages))
	dims := make(map[int][2]int, len(wrapper.Dims))
	for pg, pageChars := range wrapper.Pages {
		result := make([]pdf.TextChar, len(pageChars))
		for i, c := range pageChars {
			result[i] = pdf.TextChar{
				Text:       c.Text,
				X0:         c.X0,
				X1:         c.X1,
				Top:        c.Top,
				Bottom:     c.Bottom,
				FontName:   c.FontName,
				FontSize:   c.Size,
				PageNumber: pg,
			}
		}
		chars[pg] = result
	}
	for pg, d := range wrapper.Dims {
		if len(d) >= 2 {
			dims[pg] = [2]int{d[0], d[1]}
		}
	}
	return &PythonCharEngine{chars: chars, dims: dims, isEnglish: wrapper.IsEnglish, pages: len(wrapper.Pages)}, nil
}

// ExtractChars returns all characters for the given page (0-indexed).
func (e *PythonCharEngine) ExtractChars(pageNum int) ([]pdf.TextChar, error) {
	if pageNum < 0 || pageNum >= e.pages {
		return nil, fmt.Errorf("page %d out of range [0, %d)", pageNum, e.pages)
	}
	return e.chars[pageNum], nil
}

// PageChars returns the loaded per-page chars so callers can run document-level
// detection (e.g. util.DetectEnglish) before the pipeline consumes them.
func (e *PythonCharEngine) PageChars() map[int][]pdf.TextChar { return e.chars }

// IsEnglish returns the document-level english verdict captured from the same
// Python run that produced the dump, or nil when the dump omits it (older
// charspy files). Prefer it over re-deriving via random sampling, which is
// unstable on documents with small english runs.
func (e *PythonCharEngine) IsEnglish() *bool { return e.isEnglish }

// PageDims returns per-page image dimensions [w,h] in ZM/image pixels. They
// size the placeholder page image for image-only pages AND let TSR replay
// derive Python's page_cum_height (sum of prior page image heights), which
// the tsr_raw dump does not carry.
func (e *PythonCharEngine) PageDims() map[int][2]int { return e.dims }

// ClearChars empties every page's chars while keeping page count and dims.
// Python clears chars for is_english documents (pdf_parser.py:1687) and feeds
// pure OCR output instead, so replaying an English document must do the same —
// processPageBoxes then falls through to ocrDetectAndRecognize, which consumes
// the Phase 3 OCR dump. dims are untouched so image-only pages still get a
// correctly sized placeholder page image.
func (e *PythonCharEngine) ClearChars() {
	for pg := range e.chars {
		e.chars[pg] = nil
	}
}

// RenderPage is unused for parity — this engine supplies pre-loaded chars
// only. Return nil bytes (not an error) so callers that treat a nil render
// as "skip rendering" behave correctly.
func (e *PythonCharEngine) RenderPage(pageNum int, dpi float64) ([]byte, error) {
	return nil, nil
}

// RenderPageImage returns a blank placeholder image (no error) sized from the
// dumped per-page image dims. This engine has no PDF bytes to rasterize; the
// DocAnalyzer ignores the pixels anyway, but processPage derives
// pageHeight/pageWidth from the placeholder size, so a wrong size would
// confound the assembly-parity measurement with bogus geometry. dims are
// required (LoadPythonChars fails fast when a dump omits them); a missing page
// here is a hard error, never a degenerate 1x1 placeholder.
func (e *PythonCharEngine) RenderPageImage(pageNum int, dpi float64) (image.Image, error) {
	d, ok := e.dims[pageNum]
	if !ok || d[0] <= 0 || d[1] <= 0 {
		return nil, fmt.Errorf("page %d has no valid image dims (regenerate dump with dump_py_results.py)", pageNum)
	}
	return image.NewRGBA(image.Rect(0, 0, d[0], d[1])), nil
}

// PageCount returns the number of pages.
func (e *PythonCharEngine) PageCount() (int, error) {
	return e.pages, nil
}

// RawData returns nil — this engine only supplies pre-loaded chars
// for pipeline parity tests and does not hold PDF bytes.
func (e *PythonCharEngine) RawData() []byte { return nil }

func (e *PythonCharEngine) Outlines() ([]pdf.Outline, error) { return nil, nil }

// Close is a no-op.
func (e *PythonCharEngine) Close() error {
	return nil
}
