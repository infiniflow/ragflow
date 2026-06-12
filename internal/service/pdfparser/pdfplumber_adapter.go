// Package pdfparser provides pdfplumber-compatible PDF types and functions.
//
// This file wraps github.com/yfedoseev/pdf_oxide/go (pdf_oxide) to provide
// pdfplumber-style character extraction, page rendering, and RAGFlow-compatible
// utility functions. It is maintained as a standalone adapter layer so that
// the pdfplumber compatibility code can be modified independently of the
// pdf_oxide backend.
//
// Originally derived from github.com/yingfeng/pdfplumber-go.

package pdfparser

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"

	pdfoxide "github.com/yfedoseev/pdf_oxide/go"
)

// ── pdfplumber-compatible types ──────────────────────────────────────────

// Char represents a single character extracted from a PDF page,
// matching pdfplumber's char dict format.
type Char struct {
	Text             string    `json:"text"`
	Fontname         string    `json:"fontname"`
	Size             float64   `json:"size"`
	X0               float64   `json:"x0"`
	X1               float64   `json:"x1"`
	Top              float64   `json:"top"`
	Bottom           float64   `json:"bottom"`
	Width            float64   `json:"width"`
	Height           float64   `json:"height"`
	Doctop           float64   `json:"doctop"`
	Matrix           [6]float64 `json:"matrix"`
	Upright          bool      `json:"upright"`
	StrokingColor    string    `json:"stroking_color"`
	NonStrokingColor string    `json:"non_stroking_color"`
	Ncs              string    `json:"ncs"`
	Adv              float64   `json:"adv"`
	PageNumber       int       `json:"page_number"`
}

// Document wraps pdf_oxide's PdfDocument with pdfplumber-compatible methods.
type Document struct {
	inner *pdfoxide.PdfDocument
}

// RenderResult holds the result of rendering a PDF page.
type RenderResult struct {
	Data     []byte
	Width    int
	Height   int
	Channels int
}

// ── Document methods ─────────────────────────────────────────────────────

// Open opens a PDF file from a file path.
func Open(path string) (*Document, error) {
	doc, err := pdfoxide.Open(path)
	if err != nil {
		return nil, fmt.Errorf("pdfplumber: open %s: %w", path, err)
	}
	return &Document{inner: doc}, nil
}

// OpenBytes opens a PDF from raw bytes in memory.
func OpenBytes(data []byte) (*Document, error) {
	doc, err := pdfoxide.OpenFromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("pdfplumber: open from bytes: %w", err)
	}
	return &Document{inner: doc}, nil
}

// ExtractTables returns all tables on a page (0-indexed) using pdf_oxide's
// native table detection (PDF drawing commands, not OCR).
func (d *Document) ExtractTables(pageIdx int) ([]pdfoxide.Table, error) {
	if d.inner == nil {
		return nil, fmt.Errorf("pdfplumber: document is closed")
	}
	return d.inner.ExtractTables(pageIdx)
}

// ExtractTableCells extracts all tables from a page with cell text populated.
func (d *Document) ExtractTableCells(pageIdx int) ([]TableRegion, error) {
	tables, err := d.ExtractTables(pageIdx)
	if err != nil {
		return nil, err
	}
	result := make([]TableRegion, len(tables))
	for i, t := range tables {
		cells := make([][]string, t.RowCount)
		for r := 0; r < t.RowCount; r++ {
			cells[r] = make([]string, t.ColCount)
			for c := 0; c < t.ColCount; c++ {
				cells[r][c] = t.CellText(r, c)
			}
		}
		result[i] = TableRegion{
			Page:      pageIdx,
			Rows:      t.RowCount,
			Cols:      t.ColCount,
			HasHeader: t.HasHeader,
			Cells:     cells,
		}
	}
	return result, nil
}

// Close releases the document handle.
func (d *Document) Close() {
	if d.inner != nil {
		d.inner.Close()
		d.inner = nil
	}
}

// PageCount returns the number of pages in the document.
func (d *Document) PageCount() (int, error) {
	if d.inner == nil {
		return 0, fmt.Errorf("pdfplumber: document is closed")
	}
	return d.inner.PageCount()
}

// GetPageChars returns all characters on a page (0-indexed).
func (d *Document) GetPageChars(pageIdx int) ([]Char, error) {
	if d.inner == nil {
		return nil, fmt.Errorf("pdfplumber: document is closed")
	}
	n, err := d.PageCount()
	if err != nil {
		return nil, fmt.Errorf("pdfplumber: page count: %w", err)
	}
	if pageIdx < 0 || pageIdx >= n {
		return nil, fmt.Errorf("pdfplumber: page index %d out of range (pages: %d)", pageIdx, n)
	}
	raw, err := d.inner.ExtractChars(pageIdx)
	if err != nil {
		return nil, fmt.Errorf("pdfplumber: extract chars page %d: %w", pageIdx, err)
	}
	chars := make([]Char, len(raw))
	for i, c := range raw {
		x0 := float64(c.X)
		top := float64(c.Y)
		w := float64(c.Width)
		h := float64(c.Height)
		fs := float64(c.FontSize)
		chars[i] = Char{
			Text:             string(c.Char),
			Fontname:         c.FontName,
			Size:             fs,
			X0:               x0,
			X1:               x0 + w,
			Top:              top,
			Bottom:           top + h,
			Width:            w,
			Height:           h,
			Doctop:           top,
			Matrix:           [6]float64{fs, 0, 0, fs, x0, top},
			Upright:          true,
			StrokingColor:    "",
			NonStrokingColor: "",
			Ncs:              "",
			Adv:              fs * 0.5,
			PageNumber:       pageIdx + 1,
		}
	}
	return chars, nil
}

// GetDedupePageChars returns deduplicated characters on a page (0-indexed).
// tolerance controls how close two chars must be to be considered duplicates.
func (d *Document) GetDedupePageChars(pageIdx int, tolerance float64) ([]Char, error) {
	chars, err := d.GetPageChars(pageIdx)
	if err != nil {
		return nil, err
	}
	return dedupeChars(chars, tolerance), nil
}

// GetPageText extracts plain text from a page (0-indexed), in reading order (top → x0).
func (d *Document) GetPageText(pageIdx int) (string, error) {
	chars, err := d.GetPageChars(pageIdx)
	if err != nil {
		return "", err
	}
	if len(chars) == 0 {
		return "", nil
	}
	sorted := make([]Char, len(chars))
	copy(sorted, chars)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Top != sorted[j].Top {
			return sorted[i].Top < sorted[j].Top
		}
		return sorted[i].X0 < sorted[j].X0
	})
	var b strings.Builder
	for i, c := range sorted {
		b.WriteString(c.Text)
		if i+1 < len(sorted) {
			next := sorted[i+1]
			if next.Top == c.Top {
				gap := next.X0 - c.X1
				if gap > c.Width*0.3 {
					b.WriteByte(' ')
				}
			} else {
				b.WriteByte('\n')
			}
		}
	}
	return b.String(), nil
}

// ── Deduplication ────────────────────────────────────────────────────────

func dedupeChars(chars []Char, tolerance float64) []Char {
	if len(chars) == 0 {
		return nil
	}

	// Sort by X0 so we only need a sliding window of nearby chars.
	sorted := make([]Char, len(chars))
	copy(sorted, chars)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].X0 < sorted[j].X0 })

	result := make([]Char, 0, len(sorted))
	// maxCharWidth is the maximum X-span we've seen; chars further apart
	// than this cannot overlap. Update as we go.
	maxCharWidth := 0.0

	for _, ch := range sorted {
		cw := ch.X1 - ch.X0
		if cw > maxCharWidth {
			maxCharWidth = cw
		}

		dup := false
		// Only scan backwards within maxCharWidth; chars further away
		// cannot possibly overlap.
		for i := len(result) - 1; i >= 0; i-- {
			existing := &result[i]
			if ch.X0-existing.X1 > maxCharWidth {
				break // too far left to overlap
			}
			ox := math.Max(0, math.Min(ch.X1, existing.X1)-math.Max(ch.X0, existing.X0))
			oy := math.Max(0, math.Min(ch.Bottom, existing.Bottom)-math.Max(ch.Top, existing.Top))
			oa := ox * oy
			if oa <= 0 {
				continue
			}
			ca := cw * (ch.Bottom - ch.Top)
			ea := (existing.X1 - existing.X0) * (existing.Bottom - existing.Top)
			maxA := math.Max(ca, ea)
			ratio := oa / maxA
			sameFont := ch.Fontname == existing.Fontname
			sameSize := math.Abs(ch.Size-existing.Size) <= tolerance
			if ratio > 0.5 && sameFont && sameSize {
				dup = true
				break
			}
		}
		if !dup {
			result = append(result, ch)
		}
	}
	return result
}

// ── Rendering ────────────────────────────────────────────────────────────

// RenderPage renders a PDF page to RGBA pixels using pdf_oxide.
// pdfData must be the raw PDF bytes, pageIdx is 0-based, dpi is the resolution.
// Prefer Document.RenderPage when you already have an open Document to avoid re-parsing.
func RenderPage(pdfData []byte, pageIdx int, dpi float64) (*RenderResult, error) {
	if len(pdfData) == 0 {
		return nil, fmt.Errorf("pdfplumber: empty PDF data for rendering")
	}
	doc, err := pdfoxide.OpenFromBytes(pdfData)
	if err != nil {
		return nil, fmt.Errorf("pdfplumber: open for render: %w", err)
	}
	defer doc.Close()

	return renderPageFromDoc(doc, pageIdx, dpi)
}

// RenderPage renders a single page using the already-open document.
// Unlike the standalone RenderPage function, this reuses the open handle
// and does not re-parse the PDF on every call.
func (d *Document) RenderPage(pageIdx int, dpi float64) (*RenderResult, error) {
	if d.inner == nil {
		return nil, fmt.Errorf("pdfplumber: document is closed")
	}
	return renderPageFromDoc(d.inner, pageIdx, dpi)
}

// renderPageFromDoc is the shared rendering core: calls RenderPageRaw and
// converts premultiplied alpha to straight alpha.
func renderPageFromDoc(doc *pdfoxide.PdfDocument, pageIdx int, dpi float64) (*RenderResult, error) {
	pixmap, err := doc.RenderPageRaw(pageIdx, int(math.Round(dpi)))
	if err != nil {
		return nil, fmt.Errorf("pdfplumber: render page %d: %w", pageIdx, err)
	}

	data := make([]byte, len(pixmap.Data))
	for i := 0; i < len(pixmap.Data); i += 4 {
		a := pixmap.Data[i+3]
		if a == 0 {
			data[i], data[i+1], data[i+2], data[i+3] = 0, 0, 0, 0
		} else {
			data[i] = uint8(math.Min(255, float64(pixmap.Data[i])*255/float64(a)))
			data[i+1] = uint8(math.Min(255, float64(pixmap.Data[i+1])*255/float64(a)))
			data[i+2] = uint8(math.Min(255, float64(pixmap.Data[i+2])*255/float64(a)))
			data[i+3] = a
		}
	}
	return &RenderResult{Data: data, Width: pixmap.Width, Height: pixmap.Height, Channels: 4}, nil
}

// InitRenderer is a no-op for pdf_oxide (renderer is initialized internally).
func InitRenderer(path string) error { return nil }

// ToImage converts a RenderResult to an image.RGBA.
func (r *RenderResult) ToImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, r.Width, r.Height))
	copy(img.Pix, r.Data)
	return img
}

// ColorModel implements image.Image.
func (r *RenderResult) ColorModel() color.Model { return color.RGBAModel }

// Bounds implements image.Image.
func (r *RenderResult) Bounds() image.Rectangle { return image.Rect(0, 0, r.Width, r.Height) }

// At implements image.Image.
func (r *RenderResult) At(x, y int) color.Color {
	if x < 0 || x >= r.Width || y < 0 || y >= r.Height {
		return color.RGBA{}
	}
	idx := (y*r.Width + x) * r.Channels
	if r.Channels >= 4 {
		return color.RGBA{R: r.Data[idx], G: r.Data[idx+1], B: r.Data[idx+2], A: r.Data[idx+3]}
	}
	return color.RGBA{R: r.Data[idx], G: r.Data[idx+1], B: r.Data[idx+2], A: 255}
}

// ── Utility ──────────────────────────────────────────────────────────────

// TotalPageNumber opens a PDF and returns the page count.
func TotalPageNumber(path string, data []byte) (int, error) {
	var doc *Document
	var err error
	if data != nil {
		doc, err = OpenBytes(data)
	} else {
		doc, err = Open(path)
	}
	if err != nil {
		return 0, err
	}
	defer doc.Close()
	return doc.PageCount()
}
