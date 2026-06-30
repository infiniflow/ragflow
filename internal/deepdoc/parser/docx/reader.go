//go:build cgo

package docx

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	officeOxide "github.com/yfedoseev/office_oxide/go"
)

// RawBlock represents a single block (paragraph, heading, or table)
// extracted from a DOCX file in document order.
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

// ExtractRawBlocks opens a DOCX via office_oxide and extracts blocks in
// document order, matching the format produced by python-docx's
// _element.body iteration.
func ExtractRawBlocks(data []byte) ([]RawBlock, error) {
	doc, err := officeOxide.OpenFromBytes(data, "docx")
	if err != nil {
		return nil, fmt.Errorf("office_oxide open: %w", err)
	}
	defer doc.Close()

	irJSON, err := doc.ToIRJSON()
	if err != nil {
		return nil, fmt.Errorf("ToIRJSON: %w", err)
	}

	var ir irDocument
	if err := json.Unmarshal([]byte(irJSON), &ir); err != nil {
		return nil, fmt.Errorf("parse IR JSON: %w", err)
	}

	var blocks []RawBlock
	for _, sec := range ir.Sections {
		for _, el := range sec.Elements {
			block := irElementToBlock(el)
			blocks = append(blocks, block)
		}
	}
	return blocks, nil
}

func irElementToBlock(el irElement) RawBlock {
	switch el.Type {
	case "table":
		rows := make([][]string, len(el.Rows))
		for ri, row := range el.Rows {
			cells := make([]string, len(row.Cells))
			for ci, cell := range row.Cells {
				cells[ci] = joinElements(cell.Content)
			}
			rows[ri] = cells
		}
		return RawBlock{Type: "table", Rows: rows}

	case "heading":
		text := joinRuns(el.Content)
		level := strconv.Itoa(el.Level)
		return RawBlock{
			Type:  "paragraph",
			Text:  text,
			Style: "Heading " + level,
		}

	case "image":
		return RawBlock{
			Type:  "image",
			Image: base64.StdEncoding.EncodeToString(el.Data),
		}

	default: // "paragraph" and anything else
		return RawBlock{
			Type:  "paragraph",
			Text:  joinRuns(el.Content),
			Style: "Normal",
		}
	}
}

func joinRuns(runs []irRun) string {
	var b strings.Builder
	for _, r := range runs {
		if r.Type == "text" {
			b.WriteString(r.Text)
		}
	}
	return b.String()
}

// joinElements extracts plain text from nested irElements (used for table cells).
func joinElements(els []irElement) string {
	var b strings.Builder
	for _, el := range els {
		b.WriteString(joinRuns(el.Content))
	}
	return b.String()
}
