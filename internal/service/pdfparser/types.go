// Package pdfparser provides Go equivalents of RAGFlow's deepdoc/parser/pdf_parser.py
// layout analysis and text extraction logic.
//
// Each exported function documents its corresponding Python original with
// file:line references to pdf_parser.py.
package pdfparser

// TextChar represents a single character extracted from a PDF page.
// Corresponds to pdfplumber page.chars dict elements in pdf_parser.py.
//
// Python equivalent:
//
//	c = {"x0": 100.5, "x1": 108.2, "top": 200.0, "bottom": 212.0,
//	     "text": "A", "fontname": "ABCDE+SimSun", "page_number": 3}
//
// Example:
//
//	c := TextChar{X0: 100.5, X1: 108.2, Top: 200.0, Bottom: 212.0,
//	              Text: "A", FontName: "ABCDE+SimSun", PageNumber: 3}
type TextChar struct {
	X0, X1          float64 // horizontal bounds in PDF points
	Top, Bottom     float64 // vertical bounds in PDF points
	Text            string  // single character (or small text run)
	FontName        string  // e.g. "ABCDE+SimSun"
	FontSize        float64
	PageNumber      int
	NCS             string  // non-stroking color space (pdfplumber field)
	StrokingColor   []float64
	NonStrokingColor []float64
	LayoutType      string  // "text", "table", "figure", "equation"
	LayoutNo        string  // layout identifier
	ColID           int     // column ID assigned by _assign_column
	R               string  // rotation/orientation marker
	InRow           int     // number of sibling boxes in same row
}

// TextBox represents a rectangular region of text on a PDF page,
// typically a line or paragraph fragment. Created by layout analysis
// (e.g. _assign_column, _text_merge).
//
// Python equivalent:
//
//	b = {"x0": 50.0, "x1": 550.0, "top": 100.0, "bottom": 112.0,
//	     "text": "第三章 财务分析", "page_number": 3, "layout_type": "text"}
type TextBox struct {
	X0, X1      float64
	Top, Bottom float64
	Text        string
	PageNumber  int
	LayoutType  string // "text", "table", "figure", "equation"
	LayoutNo    string
	ColID       int
	R           string
	InRow       int
}

// Position represents a parsed position tag from @@...## format.
//
// Python: pdf_parser.py:1872 extract_positions()
//
// Format: @@{page_range}\t{left}\t{right}\t{top}\t{bottom}##
// Example: "@@0-1\t50.0\t300.0\t200.0\t400.0##"
type Position struct {
	PageNumbers []int   // e.g. [0, 1] for cross-page content
	Left        float64
	Right       float64
	Top         float64
	Bottom      float64
}

// TableRegion represents a detected table on a PDF page.
type TableRegion struct {
	Page      int
	Rows      int
	Cols      int
	HasHeader bool
	Cells     [][]string // [row][col] cell text
}

// Section represents a text segment with its spatial position on a PDF page.
// This is the primary output of layout analysis, consumed by NLP merge/split.
//
// Python equivalent: sections elements in naive.py::chunk()
//
//	[(text_with_tags, position_tag_string), ...]
type Section struct {
	Text        string // text content with embedded @@ position tags
	PositionTag string // "@@page-left-right-top-bottom##" format
}

// TableItem represents a detected table or figure region.
//
// Python equivalent: tables elements in naive.py::chunk()
//
//	[((img, rows), positions), ...]
type TableItem struct {
	ImageB64  string     // base64-encoded PNG of the table/figure region
	Rows      [][]string // table rows, each row is a list of cell strings
	Positions []Position // spatial positions
}

// Config holds parser configuration.
//
// Python equivalent: kwargs merged with parser_config in task_executor.py
type Config struct {
	Zoom               float64 // zoom factor for page rendering, default 3
	FromPage            int     // 0-based start page
	ToPage              int     // 0-based end page (-1 = all)
	TableContextSize    int     // tokens of surrounding context for tables
	ImageContextSize    int     // tokens of surrounding context for images
	AutoRotateTables    *bool   // enable auto table rotation detection
	SeparateTablesFigs   bool    // separate tables and figures
	SkipRender          bool    // skip page image rendering (tests, char-only pipelines)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Zoom:              3,
		FromPage:           0,
		ToPage:             -1,
		TableContextSize:   0,
		ImageContextSize:   0,
		SeparateTablesFigs: false,
	}
}

// HasColor checks if a character has visible color (not invisible white-on-white).
//
// Python: pdf_parser.py:190 _has_color()
//
// Non-stroking/stroking color is not available from the PDF engine, so this function
// returns true by default. The original logic was:
//
//	if o["ncs"] == "DeviceGray" and all channels are 1:
//	    if text matches [a-zT_...] pattern → likely invisible → return False
//	return True
//
// All extracted chars are assumed visible since
// the PDF engine handles rendering internally.
func HasColor(c TextChar) bool {
	return true
}

// DetectGarbled returns true if a page's text is likely garbled due to
// font encoding issues, indicating OCR is needed.
//
// This is a convenience wrapper around IsGarbledByFontEncoding.
//
// Python: pdf_parser.py:264 _is_garbled_by_font_encoding()
func DetectGarbled(chars []TextChar) bool {
	return IsGarbledByFontEncoding(chars, 20)
}
