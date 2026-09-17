package docx

// RawBlock represents a single block extracted from a DOCX file in document order.
// Type is one of "paragraph", "table", or "image". Headings are represented as
// Type "paragraph" with a Style of "Heading N".
type RawBlock struct {
	Type  string     `json:"type"`            // "paragraph" or "table"
	Text  string     `json:"text"`            // paragraph text; empty for tables
	Style string     `json:"style"`           // Word style name (e.g. "Normal", "Heading 1")
	Image string     `json:"image,omitempty"` // base64-encoded image data
	Rows  [][]string `json:"rows,omitempty"`  // table rows; nil for paragraphs
}

// ── office_oxide IR JSON types ────────────────────────────────────────

type irElement struct {
	Type    string  `json:"type"`    // "paragraph", "heading", "table", "image"
	Level   int     `json:"level"`   // heading level (1-6)
	Style   string  `json:"style"`   // Word style name (e.g. "Normal", "Caption", "Heading 1")
	Content []irRun `json:"content"` // rich text runs
	Data    []byte  `json:"data"`    // raw image bytes (for "image" type)
	Rows    []irRow `json:"rows"`    // table rows
}

type irRun struct {
	Type    string      `json:"type"`    // "text", "image"
	Text    string      `json:"text"`    // plain text content
	Content []irElement `json:"content"` // nested elements (used in table cells)
}

type irRow struct {
	Cells []irCell `json:"cells"`
}

type irCell struct {
	Content []irElement `json:"content"` // nested paragraphs inside table cell
}

type irSection struct {
	Title    string      `json:"title"`
	Elements []irElement `json:"elements"`
}

type irDocument struct {
	Sections []irSection `json:"sections"`
}
