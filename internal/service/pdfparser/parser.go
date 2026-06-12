package pdfparser

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Parser is the main PDF text/layout extraction pipeline.
// It corresponds to RAGFlowPdfParser in pdf_parser.py.
type Parser struct {
	Config Config

	// Post-layout state (accumulated during pipeline)
	boxes         []TextBox
	medianHeights map[int]float64 // page → median char height
	medianWidths  map[int]float64 // page → median char width
	pageFrom      int             // offset for page numbering
}

// PDFEngine abstracts page extraction capabilities.
// Calling code provides the implementation (pdfplumber-rs, etc.).
type PDFEngine interface {
	// ExtractChars returns all characters on a page with position data.
	// pageNum is 0-indexed.
	ExtractChars(pageNum int) ([]TextChar, error)

	// RenderPage renders a page to PNG bytes at the given DPI.
	RenderPage(pageNum int, dpi float64) ([]byte, error)

	// PageCount returns the total number of pages.
	PageCount() (int, error)

	// ExtractTables returns all tables detected on a page (0-indexed).
	// Tables are identified from PDF drawing commands — no OCR needed.
	ExtractTables(pageNum int) ([]TableRegion, error)

	// Close releases resources held by the engine.
	Close() error
}

// Tokenizer provides text tokenization matching rag_tokenizer.
// Used by MergeSameBullet to detect Chinese characters.
type Tokenizer interface {
	Tag(token string) string // POS tag
}

// NewParser creates a new Parser.
func NewParser(cfg Config) *Parser {
	return &Parser{
		Config: cfg,
	}
}

// Parse runs the full PDF extraction pipeline: chars → boxes →
// column assignment → text merge → vertical merge → sections.
//
// Returns sections (text + position tags for NLP merge),
// and rendered page images (map[pageNum]PNG bytes for crop/concat in Go NLP).
func (p *Parser) Parse(engine PDFEngine) (sections []Section, pageImages map[int][]byte, err error) {
	// Initialize
	p.boxes = nil
	p.medianHeights = make(map[int]float64)
	p.medianWidths = make(map[int]float64)
	pageImages = make(map[int][]byte)

	// Normalize page range
	pageCount, err := engine.PageCount()
	if err != nil {
		return nil, pageImages, fmt.Errorf("page count: %w", err)
	}
	toPage := p.Config.ToPage
	if toPage < 0 || toPage >= pageCount {
		toPage = pageCount - 1
	}
	if toPage < p.Config.FromPage {
		return nil, pageImages, nil
	}

	// Per-page extraction
	pageChars := make(map[int][]TextChar) // for language detection
	for pg := p.Config.FromPage; pg <= toPage; pg++ {
		chars, extractErr := engine.ExtractChars(pg)
		if extractErr != nil {
			continue // skip broken pages (like pdfplumber does)
		}

		// Page-level metrics
		p.medianHeights[pg] = MedianCharHeight(chars)
		p.medianWidths[pg] = MedianCharWidth(chars)

		// Render page image (for Go-side crop later)
		if !p.Config.SkipRender {
			png, renderErr := engine.RenderPage(pg, 72*p.Config.Zoom)
			if renderErr == nil && len(png) > 0 {
				pageImages[pg] = png
			}
		}

		pageChars[pg] = chars
		boxes := p.charsToBoxes(chars, pg)
		p.boxes = append(p.boxes, boxes...)
	}

	if len(p.boxes) == 0 {
		return nil, pageImages, nil
	}

	// Detect English (matching Python's is_english per-page majority vote).
	isEnglish := detectEnglish(pageChars)

	// ---- Layout pipeline (matching pdf_parser.py _parse_loaded_window_into_bboxes) ----

	// 1. Assign columns based on x0 clustering (part of _text_merge)
	p.boxes = AssignColumn(p.boxes, p.Config.Zoom)

	// 2. Horizontal text merge (same line, same column) — _text_merge
	p.boxes = TextMerge(p.boxes, p.medianHeights, p.Config.Zoom)

	// 3. Y-first sort (page → top → x0) — _concat_downward
	sortByPageThenY(p.boxes)

	// 4. Naive vertical merge — _naive_vertical_merge
	p.boxes = NaiveVerticalMerge(p.boxes, p.medianHeights, p.medianWidths, isEnglish)

	// 5. Convert boxes → sections with position tags
	sections = p.boxesToSections(p.boxes)

	return sections, pageImages, nil
}

// detectEnglish detects whether a PDF is primarily English by per-page
// majority vote, matching Python's is_english logic in __images__
// (pdf_parser.py:1519-1526).
//
// Each page: sample first 100 char texts, join into one string, check if
// there is a run of 30+ consecutive ASCII characters (letters, digits,
// spaces, punctuation).  Pages with such a run vote "English".  Returns
// true when a strict majority of pages vote yes.
func detectEnglish(pageChars map[int][]TextChar) bool {
	if len(pageChars) == 0 {
		return false
	}
	// Python: re.search(r"[ a-zA-Z0-9,/¸;:'\[\]\(\)!@#$%^&*\"?<>._-]{30,}", sample)
	asciiSet := " a-zA-Z0-9,/¸;:'[]()!@#$%^&*\"?<>._-"
	pagesWithSeq := 0
	totalNonEmpty := 0

	for _, chars := range pageChars {
		if len(chars) == 0 {
			continue
		}
		totalNonEmpty++
		n := min(100, len(chars))
		var buf strings.Builder
		for i := 0; i < n; i++ {
			buf.WriteString(chars[i].Text)
		}
		sample := buf.String()
		run := 0
		for _, r := range sample {
			if strings.ContainsRune(asciiSet, r) {
				run++
				if run >= 30 {
					pagesWithSeq++
					break
				}
			} else {
				run = 0
			}
		}
	}

	return totalNonEmpty > 0 && pagesWithSeq > totalNonEmpty/2
}

// charsToBoxes converts raw characters to initial text boxes by grouping
// characters into lines based on vertical overlap.
//
// Python: pdf_parser.__images__ producing self.boxes
func (p *Parser) charsToBoxes(chars []TextChar, pageNum int) []TextBox {
	if len(chars) == 0 {
		return nil
	}

	lines := groupCharsToLines(chars)

	boxes := make([]TextBox, 0, len(lines))
	for _, line := range lines {
		box := lineToTextBox(line)
		box.PageNumber = pageNum
		boxes = append(boxes, box)
	}
	return boxes
}

// boxesToSections converts layout boxes to section format with position tags.
// Position tags are stored in the PositionTag field only, matching Python's
// _parse_loaded_window_into_bboxes which stores _line_tag in b["position_tag"]
// without modifying b["text"].
//
// Python equivalent: output consumed by naive.py::chunk()
func (p *Parser) boxesToSections(boxes []TextBox) []Section {
	sections := make([]Section, 0, len(boxes))
	for _, b := range boxes {
		t := strings.TrimSpace(b.Text)
		if t == "" {
			continue
		}
		posTag := FormatPositionTag(b.PageNumber, b.X0, b.X1, b.Top, b.Bottom)
		sections = append(sections, Section{
			Text:        t,
			PositionTag: posTag,
		})
	}
	return sections
}

// sortByPageThenY sorts boxes by page → top → x0, matching Python's
// _concat_downward which calls Recognizer.sort_Y_firstly(self.boxes, 0).
func sortByPageThenY(boxes []TextBox) {
	sort.Slice(boxes, func(i, j int) bool {
		if boxes[i].PageNumber != boxes[j].PageNumber {
			return boxes[i].PageNumber < boxes[j].PageNumber
		}
		if boxes[i].Top != boxes[j].Top {
			return boxes[i].Top < boxes[j].Top
		}
		return boxes[i].X0 < boxes[j].X0
	})
}

// ---- internal helpers ----

// groupCharsToLines groups characters into horizontal lines based on vertical overlap.
func groupCharsToLines(chars []TextChar) [][]TextChar {
	if len(chars) == 0 {
		return nil
	}

	// Sort by top then x0 using sort.Slice (O(n log n))
	sort.Slice(chars, func(i, j int) bool {
		if chars[i].Top != chars[j].Top {
			return chars[i].Top < chars[j].Top
		}
		return chars[i].X0 < chars[j].X0
	})

	var lines [][]TextChar
	var currentLine []TextChar

	for _, c := range chars {
		if len(currentLine) == 0 {
			currentLine = append(currentLine, c)
			continue
		}
		if verticalOverlap(currentLine[len(currentLine)-1], c) {
			currentLine = append(currentLine, c)
		} else {
			if len(currentLine) > 0 {
				lines = append(lines, currentLine)
			}
			currentLine = []TextChar{c}
		}
	}
	if len(currentLine) > 0 {
		lines = append(lines, currentLine)
	}
	return lines
}

// verticalOverlap checks if two characters are on the same horizontal line.
func verticalOverlap(a, b TextChar) bool {
	mh := math.Max(CharHeight(a), CharHeight(b))
	return math.Abs(a.Top-b.Top) < mh*0.5
}

// lineToTextBox converts a line of characters to a single TextBox.
func lineToTextBox(chars []TextChar) TextBox {
	if len(chars) == 0 {
		return TextBox{}
	}
	box := TextBox{
		X0:     chars[0].X0,
		X1:     chars[0].X1,
		Top:    chars[0].Top,
		Bottom: chars[0].Bottom,
	}
	var textParts []string
	for _, c := range chars {
		box.X0 = math.Min(box.X0, c.X0)
		box.X1 = math.Max(box.X1, c.X1)
		box.Top = math.Min(box.Top, c.Top)
		box.Bottom = math.Max(box.Bottom, c.Bottom)
		textParts = append(textParts, c.Text)
		if c.LayoutType != "" {
			box.LayoutType = c.LayoutType
		}
		if c.LayoutNo != "" {
			box.LayoutNo = c.LayoutNo
		}
	}
	box.Text = strings.Join(textParts, "")
	return box
}
