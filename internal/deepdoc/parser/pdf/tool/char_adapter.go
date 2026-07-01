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
	chars map[int][]pdf.TextChar // pageNum → chars
	pages int
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
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("parse charspy json: %w", err)
	}

	chars := make(map[int][]pdf.TextChar, len(wrapper.Pages))
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
	return &PythonCharEngine{chars: chars, pages: len(wrapper.Pages)}, nil
}

// ExtractChars returns all characters for the given page (0-indexed).
func (e *PythonCharEngine) ExtractChars(pageNum int) ([]pdf.TextChar, error) {
	if pageNum < 0 || pageNum >= e.pages {
		return nil, fmt.Errorf("page %d out of range [0, %d)", pageNum, e.pages)
	}
	return e.chars[pageNum], nil
}

// RenderPage returns a 1x1 placeholder PNG (not used in parity tests).
func (e *PythonCharEngine) RenderPage(pageNum int, dpi float64) ([]byte, error) {
	return nil, fmt.Errorf("PythonCharEngine: RenderPage not supported")
}

// RenderPageImage returns a 1x1 placeholder image (not used in parity tests).
func (e *PythonCharEngine) RenderPageImage(pageNum int, dpi float64) (image.Image, error) {
	return nil, fmt.Errorf("PythonCharEngine: RenderPageImage not supported")
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
