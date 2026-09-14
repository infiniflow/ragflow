package docx

import (
	"strings"

	"ragflow/internal/deepdoc/parser/pdf/table"
	doctype "ragflow/internal/deepdoc/parser/type"
)

// blocksToSections converts raw DOCX blocks to the shared Section representation
// consumed by the framework layer.  Headings get LayoutType "title", tables get
// DocTypeKwd "table" with a populated TableItem, and everything else is "text".
func blocksToSections(blocks []RawBlock) []doctype.Section {
	sections := make([]doctype.Section, 0, len(blocks))
	for _, b := range blocks {
		sec := blockToSection(b)
		sections = append(sections, sec)
	}
	return sections
}

func blockToSection(b RawBlock) doctype.Section {
	switch b.Type {
	case "table":
		return doctype.Section{
			Text:       table.SimpleRowsToHTML(b.Rows),
			DocTypeKwd: "table",
			TableItem: &doctype.TableItem{
				Rows: b.Rows,
			},
		}
	case "image":
		return doctype.Section{
			DocTypeKwd: "image",
			Image:      b.Image,
		}
	default:
		layoutType := "text"
		if strings.HasPrefix(strings.ToLower(b.Style), "heading") {
			layoutType = "title"
		}
		return doctype.Section{
			Text:       b.Text,
			DocTypeKwd: "text",
			LayoutType: layoutType,
		}
	}
}

// Parse converts a DOCX file (given as bytes) into a doctype.ParseResult.
// It uses office_oxide for raw block extraction, then maps blocks to Sections.
func Parse(data []byte, cfg doctype.ParserConfig) (*doctype.ParseResult, error) {
	blocks, err := ExtractRawBlocks(data)
	if err != nil {
		return nil, err
	}
	return &doctype.ParseResult{
		Sections: blocksToSections(blocks),
	}, nil
}
