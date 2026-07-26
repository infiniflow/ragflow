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
		style := el.Style
		if style == "" {
			style = "Normal"
		}
		return RawBlock{
			Type:  "paragraph",
			Text:  joinRuns(el.Content),
			Style: style,
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
// When multiple elements are present, a newline is inserted between each one
// to match python-docx _Cell.text behavior.
func joinElements(els []irElement) string {
	var b strings.Builder
	for i, el := range els {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(joinRuns(el.Content))
	}
	return b.String()
}
