package parser

import (
	"regexp"
	"strings"
)

// tocHeadingPattern matches TOC heading text — case-insensitive match against
// contents/目录/目次/table of contents/致谢/acknowledge after stripping all
// spaces (ASCII + fullwidth).
// The heading is matched after whitespacePattern has removed its spaces, so a
// multi-word heading is listed without them: "Table of Contents" reads "tableofcontents".
var tocHeadingPattern = regexp.MustCompile(`(?i)^(contents|目录|目次|tableofcontents|致谢|acknowledge)$`)

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
// of parser items.
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
	// Fast path: if no item is a TOC heading, return unchanged without paying
	// for the deletion bitmap or a full-slice self-copy (most documents).
	isHeading := func(text string) bool {
		normalized := whitespacePattern.ReplaceAllString(strings.SplitN(text, "@@", 2)[0], "")
		return tocHeadingPattern.MatchString(normalized)
	}
	hasHeading := false
	for _, item := range items {
		if isHeading(itemText(item)) {
			hasHeading = true
			break
		}
	}
	if !hasHeading {
		return items
	}
	// Logical-deletion view: mark entries as dropped instead of doing an O(n)
	// append-shift per removed entry, then compact once at the end. The loop
	// below reads items[i] through the alive view, preserving the exact
	// index semantics of the original in-place delete loop.
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
		text := itemText(items[i])
		// Strip @@ suffix and whitespace before matching.
		if !isHeading(text) {
			i++
			continue
		}
		// Drop the TOC heading.
		alive[i] = false
		if i = next(i); i >= len(items) {
			break
		}
		// Determine the prefix from the entry right after the heading.
		prefix := tocPrefix(itemText(items[i]), eng)
		for prefix == "" {
			alive[i] = false
			if i = next(i); i >= len(items) {
				break
			}
			prefix = tocPrefix(itemText(items[i]), eng)
		}
		if i >= len(items) || prefix == "" {
			break
		}
		// Drop the first TOC entry (the one that supplied the prefix).
		alive[i] = false
		if i = next(i); i >= len(items) {
			break
		}
		// Drop up to 128 following entries that start with prefix.
		limit := i + 128
		if limit > len(items) {
			limit = len(items)
		}
		for j := i; j < limit; j++ {
			if !alive[j] {
				continue
			}
			if !strings.HasPrefix(itemText(items[j]), prefix) {
				continue
			}
			// Drop entries [i, j).
			for k := i; k < j; k++ {
				alive[k] = false
			}
			break
		}
	}
	// Single write-index compaction preserves the original relative order.
	w := 0
	for i := range items {
		if alive[i] {
			items[w] = items[i]
			w++
		}
	}
	// Clear the vacated tail so removed item maps are not retained by the
	// backing array beyond the returned slice length.
	clear(items[w:])
	return items[:w]
}

// tocPrefix returns the TOC-entry prefix: first 3 chars for CJK,
// first 2 whitespace-separated words for English.
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

// isEnglishTexts reports whether the given texts read as English. It returns
// true when more than 80% of the (up to 200) sampled non-empty segments consist
// solely of ASCII letters, digits, and punctuation.
// The result selects the TOC-entry prefix strategy: a 2-word prefix for English
// content, otherwise a 3-character prefix (used by removeContentsTable / tocPrefix).
func isEnglishTexts(texts []string) bool {
	var sampled []string
	for _, t := range texts {
		if s := strings.TrimSpace(t); s != "" {
			sampled = append(sampled, s)
		}
		if len(sampled) >= 200 {
			break
		}
	}
	if len(sampled) == 0 {
		return false
	}
	pat := regexp.MustCompile("^[" + "`" + "a-zA-Z0-9\\s.,':;/\"?<>!()\\-]+$")
	eng := 0
	for _, t := range sampled {
		if pat.MatchString(t) {
			eng++
		}
	}
	return float64(eng)/float64(len(sampled)) > 0.8
}

// isEnglishItems reports whether the parser items read as English, classifying
// each item's text with isEnglishTexts. It is passed to removeTOCWord /
// removeContentsTable so English documents use the 2-word TOC prefix.
func isEnglishItems(items []map[string]any) bool {
	texts := make([]string, 0, len(items))
	for _, it := range items {
		if t := itemText(it); t != "" {
			texts = append(texts, t)
		}
	}
	return isEnglishTexts(texts)
}
