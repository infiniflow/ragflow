// Package doctype provides shared types, interfaces, and constants for the
// deepdoc parser pipeline.  All format-specific parsers (pdf, docx, xlsx, etc.)
// share these definitions.  The package has zero dependencies on sibling
// packages so that any sub-package can import it without circular imports.
package doctype

import (
	"context"
	"image"
	"unicode"
)

// ── Pipeline types ────────────────────────────────────────────────────────

// PipelineMetrics records diagnostic counts at each pipeline stage.
type PipelineMetrics struct {
	BoxesInitial   int
	BoxesTextMerge int
	BoxesVertMerge int
	BoxesFinal     int
	TablesCount    int
}

// ParseResult encapsulates all outputs from a single Parse() call.
type ParseResult struct {
	Sections []Section
	Tables   []TableItem
	// PageHeight and PageWidth record the PDF-point dimensions of
	// each page's winning render. These are computed from the render
	// image's pixel dimensions divided by the per-page zoom factor,
	// so downstream consumers get PDF-point values directly without
	// needing to track a per-page zoom map.
	PageHeight map[int]float64
	PageWidth  map[int]float64
	Metrics    PipelineMetrics
	Outlines   []Outline // PDF outlines/bookmarks extracted from the document

	DLARegions []DLAPageRegions

	// Engine is the native PDF backend used to lazily crop section
	// images on demand (e.g. for markdown figure embeds or downstream
	// chunk-time cropping). It is populated by ParseRaw and carries
	// ownership of the engine: Parse does NOT close the engine, so the
	// caller must release it via Close once the result is fully
	// serialized. The JSON parse path closes it immediately after
	// serialization; the markdown path crops figure images first, then
	// closes. Close is idempotent and safe to call on a nil result.
	Engine PDFEngine
}

// Figures returns all sections with LayoutType "figure".
// Computed on demand from Sections — no stored field.
func (r *ParseResult) Figures() []Section {
	return CollectFigures(r.Sections)
}

// Close releases the native PDF engine held by the result, if any. It is
// safe to call multiple times and on a nil/empty result.
func (r *ParseResult) Close() {
	if r != nil && r.Engine != nil {
		_ = r.Engine.Close()
		r.Engine = nil
	}
}

// DLAPageRegions holds DLA layout regions for one page.
type DLAPageRegions struct {
	Page    int
	Regions []DLARegion
}

// ── Character and text box types ──────────────────────────────────────────

// TextChar represents a single character extracted from a PDF page.
type TextChar struct {
	X0, X1      float64
	Top, Bottom float64
	Text        string
	FontName    string
	FontSize    float64
	PageNumber  int
	LayoutType  string
	LayoutNo    string
	ColID       int
	R           int
}

func (c TextChar) Bounds() (float64, float64, float64, float64) {
	return c.X0, c.Top, c.X1, c.Bottom
}

// TextBox represents a rectangular region of text on a PDF page.
type TextBox struct {
	X0, X1      float64
	Top, Bottom float64
	Text        string
	PageNumber  int
	LayoutType  string
	LayoutNo    string
	ColID       int
	R           int
	// Post-TSR table annotation fields (Python: R/H/C/SP tags)
	RTop, RBott   float64
	HTop, HBott   float64
	HLeft, HRight float64
	H             int
	C             int
	CLeft, CRight float64
	SP            int
}

func (b TextBox) Bounds() (float64, float64, float64, float64) {
	return b.X0, b.Top, b.X1, b.Bottom
}

// ── Position and section types ────────────────────────────────────────────

// Position represents a parsed position tag from @@...## format.
type Position struct {
	PageNumbers []int
	Left        float64
	Right       float64
	Top         float64
	Bottom      float64
}

// Section represents a text segment with its spatial position on a PDF page.
type Section struct {
	Text        string
	PositionTag string
	LayoutType  string
	DocTypeKwd  string // "text"/"table"/"image" — assigned during post-processing
	Positions   []Position
	TableItem   *TableItem
	Image       string // base64-encoded cropped page image
}

// CollectFigures returns all sections with LayoutType "figure".
func CollectFigures(sections []Section) []Section {
	if sections == nil {
		return nil
	}
	figures := make([]Section, 0)
	for _, s := range sections {
		if s.LayoutType == LayoutTypeFigure {
			figures = append(figures, s)
		}
	}
	return figures
}

// ── Table types ───────────────────────────────────────────────────────────

// TableItem represents a detected table or figure region.
type TableItem struct {
	ImageB64  string
	Rows      [][]string
	Cells     []TSRCell
	Positions []Position
	Scale     float64
	CropOffX  float64
	CropOffY  float64
	Caption   string

	RegionLeft, RegionRight, RegionTop, RegionBottom float64
	NoMerge                                          bool
	Grid                                             [][]TSRCell
}

// TSRCell represents one table cell from TSR.
type TSRCell struct {
	X0, Y0, X1, Y1 float64
	Text           string
	Label          string
}

func (c TSRCell) Bounds() (float64, float64, float64, float64) {
	return c.X0, c.Y0, c.X1, c.Y1
}

// ── DeepDoc vision types ─────────────────────────────────────────────────

// DLARegion represents one detected layout region.
type DLARegion struct {
	X0, Y0, X1, Y1 float64
	Label          string
	Confidence     float64
}

func (r DLARegion) Bounds() (float64, float64, float64, float64) {
	return r.X0, r.Y0, r.X1, r.Y1
}

// OCRBox represents a detected text region from DeepDoc OCR detection.
type OCRBox struct {
	X0, Y0, X1, Y1, X2, Y2, X3, Y3 float64
}

// OCRText represents recognized text with confidence from DeepDoc OCR rec.
type OCRText struct {
	Text       string
	Confidence float64
}

// ── Parser configuration ──────────────────────────────────────────────────

// ParserConfig holds parser configuration.
type ParserConfig struct {
	Zoom               float64
	TableContextSize   int
	ImageContextSize   int
	AutoRotateTables   *bool
	SeparateTablesFigs bool
	SortByTop          bool
	SkipOCR            bool
	// Pages restricts parsing to these 1-indexed inclusive page ranges.
	// nil/empty means parse all pages. Ranges beyond the document are clamped
	// at parse time; fully out-of-range ranges are skipped.
	Pages [][]int
}

// DefaultParserConfig returns a ParserConfig with sensible defaults.
func DefaultParserConfig() ParserConfig {
	return ParserConfig{
		Zoom:               3,
		TableContextSize:   0,
		ImageContextSize:   0,
		SeparateTablesFigs: false,
	}
}

// DlaDPI is the DPI used for rendering page images for DeepDoc DLA/OCR.
const DlaDPI = 216

// DlaScale is the scale factor from PDF points (72 DPI) to DLA image space.
const DlaScale = DlaDPI / 72.0

// ── Layout type constants ─────────────────────────────────────────────────

const (
	LayoutTypeText      = "text"
	LayoutTypeTable     = "table"
	LayoutTypeFigure    = "figure"
	LayoutTypeEquation  = "equation"
	LayoutTypeTitle     = "title"
	LayoutTypeReference = "reference"
	LayoutTypeFooter    = "footer"
	LayoutTypeHeader    = "header"

	DLALabelFigureCaption = "figure caption"
	DLALabelTableCaption  = "table caption"
)

// ── Interfaces ────────────────────────────────────────────────────────────

// DocAnalyzer abstracts DeepDoc vision operations.
type DocAnalyzer interface {
	DLA(ctx context.Context, pageImage image.Image) ([]DLARegion, error)
	TSR(ctx context.Context, cropped image.Image) ([]TSRCell, error)
	OCRDetect(ctx context.Context, cropped image.Image) ([]OCRBox, error)
	OCRRecognize(ctx context.Context, cropped image.Image) ([]OCRText, error)
	Health() bool
}

// ── Outline ────────────────────────────────────────────────────────────

// Outline represents one entry in a PDF's document outline (table of contents).
// Python: extract_pdf_outlines() in deepdoc/parser/utils.py
type Outline struct {
	Title      string
	Level      int
	PageNumber int // 1-indexed, matching Python
}

// PDFEngine abstracts page extraction capabilities.
type PDFEngine interface {
	// ExtractChars returns the per-glyph text of one page. pageNum is a
	// 0-based page index. Each returned TextChar carries its bounding box
	// in PDF-point space (post-/Rotate and CropBox-corrected) and is the
	// finest-grained text unit: it is one glyph, not a word or line.
	// Downstream processPageBoxes / CharsToBoxes merge these chars into
	// line/word-level TextBoxes before DLA/TSR, so ExtractChars output is
	// never fed to layout analysis as-is.
	ExtractChars(pageNum int) ([]TextChar, error)
	RenderPage(pageNum int, dpi float64) ([]byte, error)
	RenderPageImage(pageNum int, dpi float64) (image.Image, error)
	RawData() []byte
	PageCount() (int, error)
	Outlines() ([]Outline, error)
	Close() error
}

// Tokenizer provides text tokenization matching rag_tokenizer.
type Tokenizer interface {
	Tag(token string) string
}

// SampleFunc samples up to n characters from a page's chars.
type SampleFunc func(chars []TextChar, n int) string

// TableBuilder encapsulates TSR model-specific cell detection and grouping.
type TableBuilder interface {
	Name() string
	DetectCells(ctx context.Context, cropped image.Image) ([]TSRCell, error)
	GroupCells(cells []TSRCell) [][]TSRCell
}

// Rectangular is any 2D axis-aligned rectangle that can report its bounds.
type Rectangular interface {
	Bounds() (x0, y0, x1, y1 float64)
}

// IsCJK reports whether r is a CJK character.
func IsCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}
