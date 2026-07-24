package parser

import (
	"regexp"
	"strings"
)

// tocHeadingPattern matches TOC heading text. Mirrors Python
// rag/nlp/__init__.py:945 — case-insensitive match against
// contents/目录/目次/table of contents/致谢/acknowledge after
// stripping all spaces (ASCII + fullwidth).
var tocHeadingPattern = regexp.MustCompile(`(?i)^(contents|目录|目次|table of contents|致谢|acknowledge)$`)

// whitespacePattern matches ASCII spaces and fullwidth space (U+3000)
// for normalization before heading match.
var whitespacePattern = regexp.MustCompile("[ \t\u3000]+")

// itemText extracts the text field from a parser item map. Falls
// back to empty string when the field is missing or not a string.
func itemText(item map[string]any) string {
	if s, ok := item["text"].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// removeContentsTable drops the table-of-contents entries from a list
// of parser items. Mirrors Python rag/nlp/__init__.py:937-965
// (remove_contents_table).
//
// Algorithm:
//  1. Scan for a TOC heading (contents/目录/目次/...).
//  2. Drop the heading.
//  3. Take the next entry's prefix (first 3 chars for CJK, first 2
//     words for English) as the TOC entry pattern.
//  4. Drop that entry, then drop up to 128 following entries that
//     start with the prefix.
//  5. Non-matching entries are kept.
func removeContentsTable(items []map[string]any, eng bool) []map[string]any {
	i := 0
	for i < len(items) {
		text := itemText(items[i])
		// Strip @@ suffix and whitespace before matching.
		normalized := whitespacePattern.ReplaceAllString(strings.SplitN(text, "@@", 2)[0], "")
		if !tocHeadingPattern.MatchString(normalized) {
			i++
			continue
		}
		// Drop the TOC heading.
		items = append(items[:i], items[i+1:]...)
		if i >= len(items) {
			break
		}
		// Determine the prefix from the entry right after the heading.
		prefix := tocPrefix(itemText(items[i]), eng)
		for prefix == "" {
			items = append(items[:i], items[i+1:]...)
			if i >= len(items) {
				break
			}
			prefix = tocPrefix(itemText(items[i]), eng)
		}
		if i >= len(items) || prefix == "" {
			break
		}
		// Drop the first TOC entry (the one that supplied the prefix).
		items = append(items[:i], items[i+1:]...)
		if i >= len(items) {
			break
		}
		// Drop up to 128 following entries that start with prefix.
		limit := i + 128
		if limit > len(items) {
			limit = len(items)
		}
		for j := i; j < limit; j++ {
			if !strings.HasPrefix(itemText(items[j]), prefix) {
				continue
			}
			// Drop entries [i, j).
			items = append(items[:i], items[j:]...)
			break
		}
	}
	return items
}

// tocPrefix returns the TOC-entry prefix: first 3 chars for CJK,
// first 2 whitespace-separated words for English. Mirrors Python
// remove_contents_table line 951.
func tocPrefix(text string, eng bool) string {
	if eng {
		words := strings.Fields(text)
		if len(words) >= 2 {
			return words[0] + " " + words[1]
		}
		if len(words) == 1 {
			return words[0]
		}
		return ""
	}
	runes := []rune(text)
	if len(runes) < 3 {
		return string(runes)
	}
	return string(runes[:3])
}
