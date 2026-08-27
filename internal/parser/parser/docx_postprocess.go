package parser

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
)

// docxOutline is a heading outline entry extracted from the
// office_oxide IR. Mirrors Python (title, level, None) tuples from
// rag/flow/parser/utils.py:77-89 extract_word_outlines.
type docxOutline struct {
	Title string `json:"title"`
	Level int    `json:"level"`
}

// dotPageNumberPattern matches ".... 123" / "·····5" style TOC
// leader lines. Mirrors Python utils.py:285 regex
// (\.{2,}|…{2,}|·{2,}|[ ]{2,})\s*\d+\s*$.
var dotPageNumberPattern = regexp.MustCompile(`(\.{2,}|…{2,}|·{2,}|[ ]{2,})\s*\d+\s*$`)

// extractDOCXOutlines parses the office_oxide IR JSON and returns
// heading outlines (title + 0-based level). Mirrors Python
// rag/flow/parser/utils.py:77-89 extract_word_outlines.
func extractDOCXOutlines(irJSON string) []docxOutline {
	var ir docxIRDocument
	if err := json.Unmarshal([]byte(irJSON), &ir); err != nil {
		return nil
	}
	var outlines []docxOutline
	for _, sec := range ir.Sections {
		for _, el := range sec.Elements {
			if el.Type != "heading" {
				continue
			}
			title := strings.TrimSpace(joinDOCXIRRuns(el.contentRuns()))
			if title == "" {
				continue
			}
			outlines = append(outlines, docxOutline{
				Title: title,
				Level: el.Level - 1, // 0-based, matching Python
			})
		}
	}
	return outlines
}

// removeTOCWord filters TOC entries from docx items using heading
// outlines. Mirrors Python rag/flow/parser/utils.py:115-144
// remove_toc_word.
//
// Algorithm:
//  1. No outlines → delegate to removeContentsTable (text heuristic).
//  2. With outlines → locate the "目录/Contents" heading, drop it,
//     then drop following entries that match an outline-title prefix
//     or the "dots + page number" regex.
//  3. Finally run removeContentsTable as a fallback sweep.
func removeTOCWord(items []map[string]any, outlines []docxOutline, eng bool) []map[string]any {
	if len(outlines) == 0 {
		return removeContentsTable(items, eng)
	}

	outlineTitles := make([]string, 0, len(outlines))
	for _, o := range outlines {
		t := strings.ToLower(strings.TrimSpace(o.Title))
		if t != "" {
			outlineTitles = append(outlineTitles, t)
		}
	}

	if len(outlineTitles) > 0 {
		i := 0
		for i < len(items) {
			text := strings.ToLower(strings.TrimSpace(itemText(items[i])))
			normalized := whitespacePattern.ReplaceAllString(strings.SplitN(text, "@@", 2)[0], "")
			if !tocHeadingPattern.MatchString(normalized) {
				i++
				continue
			}
			// Drop the TOC heading.
			items = append(items[:i], items[i+1:]...)
			// Drop following entries matching outline-title prefix
			// or dot+page-number pattern.
			for i < len(items) {
				raw := itemText(items[i])
				normalized := strings.ToLower(strings.TrimSpace(strings.SplitN(raw, "@@", 2)[0]))
				if normalized == "" {
					items = append(items[:i], items[i+1:]...)
					continue
				}
				matched := false
				for _, title := range outlineTitles {
					if strings.HasPrefix(normalized, title) || strings.HasPrefix(title, normalized) {
						matched = true
						break
					}
				}
				if matched || dotPageNumberPattern.MatchString(raw) {
					items = append(items[:i], items[i+1:]...)
					continue
				}
				break
			}
			break
		}
	}

	return removeContentsTable(items, eng)
}

// whitespaceCollapsePattern collapses runs of whitespace (spaces,
// tabs, newlines) into a single space — mirrors Python
// re.sub(r"\s+", " ", text).strip() used in
// extract_docx_header_footer_texts (utils.py:210,229).
var whitespaceCollapsePattern = regexp.MustCompile(`\s+`)

// extractDOCXHeaderFooterTexts opens the docx ZIP, reads every
// word/header*.xml and word/footer*.xml part, and returns the set
// of normalized text strings found inside <w:t> elements. Mirrors
// Python rag/flow/parser/utils.py:38-53
// extract_docx_header_footer_texts (python-docx sections[].header/
// footer paragraphs + tables).
func extractDOCXHeaderFooterTexts(data []byte) map[string]bool {
	texts := make(map[string]bool)
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return texts
	}
	for _, f := range zr.File {
		name := f.Name
		if !strings.HasPrefix(name, "word/") {
			continue
		}
		base := name[len("word/"):]
		if !strings.HasPrefix(base, "header") && !strings.HasPrefix(base, "footer") {
			continue
		}
		if !strings.HasSuffix(base, ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		text := extractWtTextFromXML(rc)
		rc.Close()
		normalized := strings.TrimSpace(whitespaceCollapsePattern.ReplaceAllString(text, " "))
		if normalized != "" {
			texts[normalized] = true
		}
	}
	return texts
}

// extractWtTextFromXML decodes the XML stream and concatenates the
// content of every <w:t> element (the WordprocessingML text node).
func extractWtTextFromXML(r io.Reader) string {
	dec := xml.NewDecoder(r)
	var sb strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "t" {
			continue
		}
		// The next token inside <w:t>...</w:t> is CharData.
		for {
			inner, err := dec.Token()
			if err != nil {
				return sb.String()
			}
			switch it := inner.(type) {
			case xml.CharData:
				sb.Write(it)
			case xml.EndElement:
				if it.Name.Local == "t" {
					goto nextToken
				}
			}
		}
	nextToken:
	}
	return sb.String()
}

// removeDOCXHeaderFooterSections drops items whose normalized text
// exactly matches a header/footer text string. Mirrors Python
// rag/flow/parser/utils.py:56-67 remove_header_footer_docx_sections.
func removeDOCXHeaderFooterSections(items []map[string]any, hfTexts map[string]bool) []map[string]any {
	if len(hfTexts) == 0 {
		return items
	}
	filtered := items[:0]
	for _, item := range items {
		text := itemText(item)
		normalized := strings.TrimSpace(whitespaceCollapsePattern.ReplaceAllString(text, " "))
		if normalized != "" && hfTexts[normalized] {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}
