//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package oceanbase

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type textMatch struct {
	start int
	end   int
}

// highlightMarker holds the normalized keywords and compiled patterns for one query.
type highlightMarker struct {
	keywords           []string
	keywordSet         map[string]struct{}
	englishPatterns    []*regexp.Regexp
	nonEnglishPatterns []*regexp.Regexp
}

// newHighlightMarker prepares a reusable text marker for a query's keywords.
func newHighlightMarker(keywords []string) *highlightMarker {
	keywords = normalizeKeywords(keywords)
	marker := &highlightMarker{
		keywords:           keywords,
		keywordSet:         make(map[string]struct{}, len(keywords)),
		englishPatterns:    make([]*regexp.Regexp, 0, len(keywords)),
		nonEnglishPatterns: make([]*regexp.Regexp, 0, len(keywords)),
	}
	for _, keyword := range keywords {
		marker.keywordSet[keyword] = struct{}{}
		quoted := regexp.QuoteMeta(keyword)
		marker.englishPatterns = append(marker.englishPatterns, regexp.MustCompile("(?i)"+quoted))
		marker.nonEnglishPatterns = append(marker.nonEnglishPatterns, regexp.MustCompile(quoted))
	}
	return marker
}

// markText wraps matching terms in em tags. English terms are matched without
// case sensitivity at word boundaries. Non-English text uses tokenizedText
// when available so highlighting follows the indexed token boundaries.
func (m *highlightMarker) markText(text, tokenizedText string) string {
	if m == nil || text == "" || len(m.keywords) == 0 {
		return ""
	}

	var matches []textMatch
	if isMostlyEnglish(text) {
		matches = findPatternMatches(text, m.englishPatterns, true)
	} else if tokenizedText != "" {
		matches = findTokenMatches(text, tokenizedText, m.keywordSet)
	} else {
		matches = findPatternMatches(text, m.nonEnglishPatterns, false)
	}
	if len(matches) == 0 {
		return ""
	}
	return applyMatches(text, matches)
}

func normalizeKeywords(keywords []string) []string {
	seen := make(map[string]struct{}, len(keywords))
	result := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		key := strings.ToLower(keyword)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, keyword)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return len(result[i]) > len(result[j])
	})
	return result
}

func isMostlyEnglish(text string) bool {
	letters := 0
	latinLetters := 0
	for _, r := range text {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if unicode.In(r, unicode.Latin) {
			latinLetters++
		}
	}
	return letters > 0 && latinLetters*2 > letters
}

func findPatternMatches(text string, patterns []*regexp.Regexp, requireBoundary bool) []textMatch {
	candidates := make([]textMatch, 0)
	for _, pattern := range patterns {
		for _, indexes := range pattern.FindAllStringIndex(text, -1) {
			candidate := textMatch{start: indexes[0], end: indexes[1]}
			if requireBoundary && !hasWordBoundaries(text, candidate) {
				continue
			}
			candidates = append(candidates, candidate)
		}
	}
	return selectMatches(candidates)
}

func findTokenMatches(text, tokenizedText string, keywordSet map[string]struct{}) []textMatch {
	tokens := strings.Fields(tokenizedText)
	lastPosition := len(text)
	candidates := make([]textMatch, 0)
	for i := len(tokens) - 1; i >= 0; i-- {
		token := tokens[i]
		position := strings.LastIndex(text[:lastPosition], token)
		if position < 0 {
			continue
		}
		if _, ok := keywordSet[token]; ok {
			candidates = append(candidates, textMatch{start: position, end: position + len(token)})
		}
		lastPosition = position
	}
	return selectMatches(candidates)
}

func hasWordBoundaries(text string, match textMatch) bool {
	if match.start > 0 {
		previous, _ := utf8.DecodeLastRuneInString(text[:match.start])
		if isWordRune(previous) {
			return false
		}
	}
	if match.end < len(text) {
		next, _ := utf8.DecodeRuneInString(text[match.end:])
		if isWordRune(next) {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func selectMatches(candidates []textMatch) []textMatch {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].start != candidates[j].start {
			return candidates[i].start < candidates[j].start
		}
		return candidates[i].end > candidates[j].end
	})
	selected := make([]textMatch, 0, len(candidates))
	lastEnd := -1
	for _, candidate := range candidates {
		if candidate.start < lastEnd {
			continue
		}
		selected = append(selected, candidate)
		lastEnd = candidate.end
	}
	return selected
}

func applyMatches(text string, matches []textMatch) string {
	var result strings.Builder
	position := 0
	for _, match := range matches {
		result.WriteString(text[position:match.start])
		result.WriteString("<em>")
		result.WriteString(text[match.start:match.end])
		result.WriteString("</em>")
		position = match.end
	}
	result.WriteString(text[position:])
	return result.String()
}
