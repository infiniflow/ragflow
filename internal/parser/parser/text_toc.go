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

// tocRules carries the format-specific matching knobs for TOC removal. The
// zero value matches plain-text items as emitted by HTML, DOC and DOCX parsers.
type tocRules struct {
	// clean strips format-specific markup before heading, prefix and entry
	// matching. nil matches the item text as-is.
	clean func(string) string
	// heading overrides tocHeadingPattern when non-nil.
	heading *regexp.Regexp
	// asciiExtra lists characters the format adds to English body text
	// (Markdown markup) so isEnglishItems classifies it as English.
	asciiExtra string
}

// plainTOCRules keeps the plain-text contract that HTML, DOC and DOCX share.
var plainTOCRules = tocRules{heading: tocHeadingPattern}

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
func removeContentsTable(items []map[string]any, eng bool, rules tocRules) []map[string]any {
	heading := rules.heading
	if heading == nil {
		heading = tocHeadingPattern
	}
	clean := rules.clean
	if clean == nil {
		clean = func(s string) string { return s }
	}
	i := 0
	for i < len(items) {
		text := itemText(items[i])
		// Strip @@ suffix and whitespace before matching.
		normalized := whitespacePattern.ReplaceAllString(clean(strings.SplitN(text, "@@", 2)[0]), "")
		if !heading.MatchString(normalized) {
			i++
			continue
		}
		// Drop the TOC heading.
		items = append(items[:i], items[i+1:]...)
		if i >= len(items) {
			break
		}
		// Determine the prefix from the entry right after the heading.
		prefix := tocPrefix(clean(itemText(items[i])), eng)
		for prefix == "" {
			items = append(items[:i], items[i+1:]...)
			if i >= len(items) {
				break
			}
			prefix = tocPrefix(clean(itemText(items[i])), eng)
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
			if !strings.HasPrefix(clean(itemText(items[j])), prefix) {
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
// solely of ASCII letters/digits/punctuation. asciiExtra lists characters a
// format adds to its body text (Markdown markup) so they do not flip the
// classification; the shared callers pass an empty string.
// The result selects the TOC-entry prefix strategy: a 2-word prefix for English
// content, otherwise a 3-character prefix (used by removeContentsTable / tocPrefix).
func isEnglishTexts(texts []string, asciiExtra string) bool {
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
	pat := regexp.MustCompile("^[" + "`" + "a-zA-Z0-9\\s.,':;/\"?<>!()" + asciiExtra + "\\-]+$")
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
func isEnglishItems(items []map[string]any, asciiExtra string) bool {
	texts := make([]string, 0, len(items))
	for _, it := range items {
		if t := itemText(it); t != "" {
			texts = append(texts, t)
		}
	}
	return isEnglishTexts(texts, asciiExtra)
}
