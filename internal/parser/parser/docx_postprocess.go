package parser

import (
	"encoding/json"
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
		// Logical-deletion view: mark instead of O(n) append-shift, compact once.
		alive := make([]bool, len(items))
		for i := range alive {
			alive[i] = true
		}
		next := func(from int) int {
			for from < len(items) && !alive[from] {
				from++
			}
			return from
		}
		i := 0
		for i < len(items) {
			i = next(i)
			if i >= len(items) {
				break
			}
			text := strings.ToLower(strings.TrimSpace(itemText(items[i])))
			normalized := whitespacePattern.ReplaceAllString(strings.SplitN(text, "@@", 2)[0], "")
			if !tocHeadingPattern.MatchString(normalized) {
				i++
				continue
			}
			// Drop the TOC heading.
			alive[i] = false
			// Drop following entries matching outline-title prefix
			// or dot+page-number pattern.
			for i = next(i); i < len(items); {
				raw := itemText(items[i])
				normalized := strings.ToLower(strings.TrimSpace(strings.SplitN(raw, "@@", 2)[0]))
				if normalized == "" {
					alive[i] = false
					i = next(i)
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
					alive[i] = false
					i = next(i)
					continue
				}
				break
			}
			break
		}
		w := 0
		for i := range items {
			if alive[i] {
				items[w] = items[i]
				w++
			}
		}
		// Clear the vacated tail so removed item maps are not retained by the
		// backing array beyond the surviving slice length.
		clear(items[w:])
		items = items[:w]
	}

	return removeContentsTable(items, eng)
}
