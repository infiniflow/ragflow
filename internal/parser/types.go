// Package pdfparser provides Go equivalents of RAGFlow's deepdoc/parser/pdf_parser.py
// layout analysis and text extraction logic.
//
// Each exported function documents its corresponding Python original with
// file:line references to pdf_parser.py.
package parser

import (
	"context"
	"image"
)

// PipelineMetrics records diagnostic counts at each pipeline stage.
// Used for Go-vs-Python parity comparison and logging.
type PipelineMetrics struct {
	BoxesInitial   int
	BoxesTextMerge int
	BoxesVertMerge int
	BoxesFinal     int
	TablesCount    int
}

// ParseResult encapsulates all outputs from a single Parse() call.
// Parser itself is stateless and safe to reuse across documents.
type ParseResult struct {
	Sections   []Section
	Tables     []TableItem
	PageImages map[int]image.Image
	Figures    []Section
	Metrics    PipelineMetrics

	// Debug intermediates for DLA/TSR comparison with Python.
	// Populated only during fresh Parse, not from cached results.
	DLADebug []DLAPageRegions
	TSRDebug []TSRRawCell
}

// DLAPageRegions holds DLA layout regions for one page.
type DLAPageRegions struct {
	Page    int
	Regions []DLARegion
}

// TSRRawCell holds a raw TSR cell before row/column grouping.
type TSRRawCell struct {
	TableIndex int     `json:"table_index"`
	Page       int     `json:"page"`
	Label      string  `json:"label"`
	X0         float64 `json:"x0"`
	Y0         float64 `json:"y0"`
	X1         float64 `json:"x1"`
	Y1         float64 `json:"y1"`
	Text       string  `json:"text"`
}

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
	X0, X1      float64 // horizontal bounds in PDF points
	Top, Bottom float64 // vertical bounds in PDF points
	Text        string  // single character (or small text run)
	FontName    string  // e.g. "ABCDE+SimSun"
	FontSize    float64
	PageNumber  int
	LayoutType  string // "text", "table", "figure", "equation"
	LayoutNo    string // layout identifier
	ColID       int    // column ID assigned by _assign_column
	R           int    // rotation/orientation marker
}

func (c TextChar) Bounds() (float64, float64, float64, float64) {
	return c.X0, c.Top, c.X1, c.Bottom
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
	R           int
	// Post-TSR table annotation fields (Python: R/H/C/SP tags)
	RTop, RBott   float64 // row top/bottom
	HTop, HBott   float64 // header top/bottom
	HLeft, HRight float64 // header left/right
	H             int     // header index
	C             int     // column index
	CLeft, CRight float64 // column left/right
	SP            int     // spanning cell index
}

func (b TextBox) Bounds() (float64, float64, float64, float64) {
	return b.X0, b.Top, b.X1, b.Bottom
}

// Position represents a parsed position tag from @@...## format.
//
// Python: pdf_parser.py:1872 extract_positions()
//
// Format: @@{page_range}\t{left}\t{right}\t{top}\t{bottom}##
// Example: "@@0-1\t50.0\t300.0\t200.0\t400.0##"
type Position struct {
	PageNumbers []int // e.g. [0, 1] for cross-page content
	Left        float64
	Right       float64
	Top         float64
	Bottom      float64
}

// Section represents a text segment with its spatial position on a PDF page.
// This is the primary output of layout analysis, consumed by NLP merge/split.
//
// Python equivalent: sections elements in naive.py::chunk()
//
//	[(text_with_tags, position_tag_string), ...]
type Section struct {
	Text        string     // text content
	PositionTag string     // "@@page-left-right-top-bottom##" format
	LayoutType  string     // "text", "table", "title", "figure", ...
	Positions   []Position // parsed from PositionTag
	TableItem   *TableItem // non-nil when this section is a table
	Image       string     // base64-encoded PNG of the cropped region (Python: b["image"])
}

// CollectFigures returns all sections with LayoutType "figure".
// Returns nil if the input is nil, empty slice if no figures found.
func CollectFigures(sections []Section) []Section {
	if sections == nil {
		return nil
	}
	figures := make([]Section, 0)
	for _, s := range sections {
		if s.LayoutType == "figure" {
			figures = append(figures, s)
		}
	}
	return figures
}

// TableItem represents a detected table or figure region.
//
// Python equivalent: tables elements in naive.py::chunk()
//
//	[((img, rows), positions), ...]
type TableItem struct {
	ImageB64  string     // base64-encoded PNG of the table/figure region
	Rows      [][]string // DEPRECATED: replaced by Cells; kept for batch output compat
	Cells     []TSRCell  // raw TSR cells in crop pixel space
	Positions []Position // spatial positions (PDF points, pre-merge)
	Scale     float64    // zoom factor for coordinate conversion
	CropOffX  float64    // crop origin X in pixel space
	CropOffY  float64    // crop origin Y in pixel space
	Caption   string     // caption text merged from adjacent caption box

	// DLA table region boundaries in PDF point space (72 DPI).
	// Matches Python's cropout using DLA layout region boundaries
	// instead of text box anchor coordinates.
	RegionLeft, RegionRight, RegionTop, RegionBottom float64

	// NoMerge prevents cross-page merging for this table.  Python's
	// _extract_table_figure adds table keys to nomerge_lout_no when
	// the next box is a caption/title/reference, indicating the table
	// group ended and should not merge with its continuation.
	NoMerge bool

	// Grid is the row-column grid produced by TableBuilder.GroupCells.
	// Consumed by constructTable Path 1 and annotateTableBoxes.
	// Nil for tables without TSR cells (fallback paths use boxes instead).
	Grid [][]TSRCell
}

// ParserConfig holds parser configuration.
//
// Python equivalent: kwargs merged with parser_config in task_executor.py
type ParserConfig struct {
	Zoom               float64      // zoom factor for page rendering, default 3
	FromPage           int          // 0-based start page
	ToPage             int          // 0-based end page (-1 = all)
	TableContextSize   int          // tokens of surrounding context for tables
	ImageContextSize   int          // tokens of surrounding context for images
	AutoRotateTables   *bool        // enable auto table rotation detection
	SeparateTablesFigs bool         // separate tables and figures
	SortByTop          bool         // true = Top-based sort (parity tests); false = Bottom (production)
	ChunkSize          int          // pages per chunk (0 = default 50, matching Python batch_size)
	SkipOCR            bool         // true = DLA+TSR only, no image OCR (matching Python SKIP_OCR=1)
	MaxOCRConcurrency  int          // max concurrent OCR pages (0 = sequential); matches Python PARALLEL_DEVICES
	TableBuilder       TableBuilder // TSR model adapter; injected by caller via NewTableBuilderFor
}

// DefaultParserConfig returns a ParserConfig with sensible defaults.
func DefaultParserConfig() ParserConfig {
	return ParserConfig{
		Zoom:               3,
		FromPage:           0,
		ToPage:             -1,
		ChunkSize:          50,
		TableContextSize:   0,
		ImageContextSize:   0,
		SeparateTablesFigs: false,
	}
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

// HasColor checks if a character has visible color (not invisible white-on-white).
//
// Python: pdf_parser.py:190 _has_color()
//
// All extracted chars are assumed visible since the PDF engine handles
// rendering internally.
func HasColor(c TextChar) bool {
	return true
}

// ── DeepDoc interfaces (shared between cgo and non-cgo builds) ──────────

// ModelType identifies the DeepDoc TSR model flavour.
type ModelType string

const (
	ModelSaas ModelType = "saas" // cpu DeepDoc — cell-level TSR output
	ModelOSS  ModelType = "oss"  // oss DeepDoc — column/row line TSR output
)

// DocAnalyzer abstracts DeepDoc vision operations so the Parser can
// work with either a live service or a test mock.
// I/O methods accept a context for cancellation and deadline propagation.
type DocAnalyzer interface {
	DLA(ctx context.Context, pageImage image.Image) ([]DLARegion, error)
	TSR(ctx context.Context, cropped image.Image) ([]TSRCell, error)
	OCRDetect(ctx context.Context, cropped image.Image) ([]OCRBox, error)
	OCRRecognize(ctx context.Context, cropped image.Image) ([]OCRText, error)
	OCRRecognizeBatch(ctx context.Context, cropped []image.Image) ([][]OCRText, []error)
	Health() bool
	ModelType() ModelType
}

// OCRBox represents a detected text region from DeepDoc OCR detection.
// DeepDoc /predict/ocr?operator=det returns:
//
//	{"output": [[[[[x0,y0],[x1,y1],[x2,y2],[x3,y3]], ...]]]}
type OCRBox struct {
	X0, Y0, X1, Y1, X2, Y2, X3, Y3 float64
}

// OCRText represents recognized text with confidence from DeepDoc OCR rec.
// DeepDoc /predict/ocr?operator=rec returns:
//
//	{"output": [[[["text", confidence], ...]]]}
type OCRText struct {
	Text       string
	Confidence float64
}

// DLARegion represents one detected layout region.
type DLARegion struct {
	X0, Y0, X1, Y1 float64
	Label          string
	Confidence     float64
}

func (r DLARegion) Bounds() (float64, float64, float64, float64) {
	return r.X0, r.Y0, r.X1, r.Y1
}

// TSRCell represents one table cell from TSR.
type TSRCell struct {
	X0, Y0, X1, Y1 float64
	Text           string
	Label          string // "table", "table row", "table column", etc.
}

func (c TSRCell) Bounds() (float64, float64, float64, float64) {
	return c.X0, c.Y0, c.X1, c.Y1
}
