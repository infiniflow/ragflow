//
//  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

package extractor

import (
	"regexp"
	"strings"
)

// Multilingual relation patterns — matching Python MULTILANG_RELATION_PATTERNS.
// Entity groups [A-Z] are case-sensitive; relation keywords use (?i) inline.
// _entWord: uppercase-start word with optional trailing period (e.g. "Inc.", "Corp.")
// Periods between initials are also supported (e.g. "U.S.", "J.K.")
const _entWord = `[A-Za-z][\w']*(?:\.[A-Za-z][\w']*)*\.?`
const _relEntity = `(` + _entWord + `(?:\s+` + _entWord + `)*?)`
const _relEntity2 = `(` + _entWord + `(?:\s+` + _entWord + `){0,1})`

var relationPatterns = map[string][]relPatternEntry{
	"en": {
		{regexp.MustCompile(_relEntity + `\s+(?i:was)\s+(?i:founded)\s+(?i:by)\s+` + _relEntity2), "founded_by"},
		{regexp.MustCompile(_relEntity + `\s+(?i:is)\s+(?i:an?\s+)?(?i:co-)?(?i:founder)\s+(?i:of)\s+` + _relEntity2), "founded_by"},
		{regexp.MustCompile(_relEntity + `\s+(?i:works)\s+(?i:for)\s+` + _relEntity2), "works_for"},
		{regexp.MustCompile(_relEntity + `\s+(?i:is)\s+(?i:an?\s+)?(?i:employee)\s+(?i:of)\s+` + _relEntity2), "works_for"},
		{regexp.MustCompile(_relEntity + `\s+(?i:joined)\s+` + _relEntity2), "works_for"},
		{regexp.MustCompile(_relEntity + `\s+(?i:is)\s+(?i:the\s+)?(?:CEO|CTO|CFO|VP|(?i:director|manager|engineer))\s+(?i:of|at)\s+` + _relEntity2), "works_for"},
		{regexp.MustCompile(_relEntity + `\s+(?i:is)\s+(?i:located|based|headquartered|situated)\s+(?i:in)\s+` + _relEntity2), "located_in"},
		{regexp.MustCompile(_relEntity + `\s+(?i:was)\s+(?i:born)\s+(?i:in|on)\s+` + _relEntity2), "born_in"},
		{regexp.MustCompile(_relEntity + `\s+(?i:born)\s+(?i:in|on)\s+` + _relEntity2), "born_in"},
		{regexp.MustCompile(_relEntity + `\s+(?i:was)\s+(?i:acquired)\s+(?i:by)\s+` + _relEntity2), "acquired"},
		{regexp.MustCompile(_relEntity + `\s+(?i:acquired)\s+` + _relEntity2), "acquired"},
		{regexp.MustCompile(_relEntity + `\s+(?i:is)\s+(?i:the\s+)?(?i:CEO)\s+(?i:of)\s+` + _relEntity2), "ceo_of"},
	},
	"zh": {
		{regexp.MustCompile(`([\p{Han}\w]{2,6})\s*由\s*([\p{Han}\w]{2,4})\s*(?:创立|创建|成立|创办)`), "founded_by"},
		{regexp.MustCompile(`([\p{Han}\w]{2,4})\s*(?:创立|创建|成立|创办)(?:\s*了\s*)?([\p{Han}\w]{2,10})`), "founded_by"},
		{regexp.MustCompile(`([\p{Han}\w]{2,4})\s*(?:是\s*)?([\p{Han}\w]{2,10})\s*(?:创始人|联合创始人)`), "founded_by"},
		{regexp.MustCompile(`([\p{Han}\w]{2,4})\s*(?:任职于|供职于|工作于|就职于)\s*([\p{Han}\w]{2,10})`), "works_for"},
		{regexp.MustCompile(`([\p{Han}\w]{2,4})\s*(?:是\s*)?([\p{Han}\w]{2,10})\s*(?:的员工|的雇员)`), "works_for"},
		{regexp.MustCompile(`([\p{Han}\w]{2,10})\s*(?:位于|坐落于|总部设在|总部位于)\s*([\p{Han}\w]{2,6})`), "located_in"},
		{regexp.MustCompile(`([\p{Han}\w]{2,10})\s*在\s*([\p{Han}\w]{2,6})`), "located_in"},
		{regexp.MustCompile(`([\p{Han}\w]{2,4})\s*(?:出生于|生于)\s*([\p{Han}\w]{2,6})`), "born_in"},
		{regexp.MustCompile(`([\p{Han}\w]{2,10})\s*(?:收购|并购)\s*([\p{Han}\w]{2,10})`), "acquired"},
		{regexp.MustCompile(`([\p{Han}\w]{2,10})\s*被\s*([\p{Han}\w]{2,10})\s*(?:收购|并购)`), "acquired"},
	},
}

type relPatternEntry struct {
	pattern   *regexp.Regexp
	predicate string
}

// ExtractRelations extracts typed relations between entities.
// Matches the Python RelationExtractor pattern-based approach,
// including cross-sentence filtering via sentence boundary checks.
func ExtractRelations(text string, entities []Entity, lang string) []Relation {
	return extractRelationsWithOpts(text, entities, lang, 100)
}

// extractRelationsWithOpts is the internal version with configurable max distance.
func extractRelationsWithOpts(text string, entities []Entity, lang string, maxDistance int) []Relation {
	patterns, ok := relationPatterns[lang]
	if !ok {
		patterns = relationPatterns["en"]
	}

	entityMap := make(map[string]Entity, len(entities)*2)
	for _, e := range entities {
		entityMap[strings.ToLower(e.Text)] = e
		// Also add punctuation-stripped version
		cleaned := strings.TrimRight(e.Text, ".,;:!?")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned != e.Text {
			entityMap[strings.ToLower(cleaned)] = e
		}
	}

	// Build sentence spans (matching Python's sentence splitting regex)
	hasOffsets := false
	for _, e := range entities {
		if e.StartChar != 0 || e.EndChar != 0 {
			hasOffsets = true
			break
		}
	}
	var sentenceSpans [][2]int
	if hasOffsets {
		sentenceSpans = splitSentences(text)
	}
	sameSentence := func(c1, c2 int) bool {
		if !hasOffsets || len(sentenceSpans) == 0 {
			return true
		}
		for _, sp := range sentenceSpans {
			if sp[0] <= c1 && c1 < sp[1] && sp[0] <= c2 && c2 < sp[1] {
				return true
			}
		}
		return false
	}

	seen := make(map[string]bool)
	var relations []Relation

	// Phase 1: Pattern-based typed relations
	for _, entry := range patterns {
		matches := entry.pattern.FindAllStringSubmatch(text, -1)
		for _, m := range matches {
			if len(m) < 3 {
				continue
			}
			subjText := strings.TrimSpace(m[1])
			objText := strings.TrimSpace(m[2])
			subj := findEntityByText(subjText, entityMap)
			obj := findEntityByText(objText, entityMap)
			if subj.Text == "" || obj.Text == "" {
				continue
			}

			// Cross-sentence filter: skip if entities are in different sentences
			// (matches Python RelationExtractor._extract_with_patterns)
			if !sameSentence(subj.StartChar, obj.StartChar) {
				continue
			}

			key := subj.Text + "|" + entry.predicate + "|" + obj.Text
			if seen[key] {
				continue
			}
			seen[key] = true

			var ctx string
			if len(m) >= 2 {
				ctx = extractContext(text, m[0])
			}
			relations = append(relations, Relation{
				Subject:    subj,
				Predicate:  entry.predicate,
				Object:     obj,
				Confidence: 0.8,
				Context:    ctx,
			})
		}
	}

	// Phase 2: Co-occurrence (standalone, with sentence boundary check)
	for _, r := range extractCooccurrence(text, entities, maxDistance) {
		key := r.Subject.Text + "|related_to|" + r.Object.Text
		if !seen[key] {
			seen[key] = true
			relations = append(relations, r)
		}
	}

	// Multi-hop inference + dedup (matching Python always applies these)
	relations = inferMultiHop(relations)
	relations = dedupRelations(relations)

	return relations
}

// extractCooccurrence generates related_to relations for entity pairs
// within maxDistance characters in the same sentence.
func extractCooccurrence(text string, entities []Entity, maxDistance int) []Relation {
	if len(entities) < 2 {
		return nil
	}
	hasOffsets := false
	for _, e := range entities {
		if e.StartChar != 0 || e.EndChar != 0 {
			hasOffsets = true
			break
		}
	}
	var sentenceSpans [][2]int
	if hasOffsets {
		sentenceSpans = splitSentences(text)
	}
	sameSentence := func(c1, c2 int) bool {
		if !hasOffsets || len(sentenceSpans) == 0 {
			return true
		}
		for _, sp := range sentenceSpans {
			if sp[0] <= c1 && c1 < sp[1] && sp[0] <= c2 && c2 < sp[1] {
				return true
			}
		}
		return false
	}
	var relations []Relation
	for i := 0; i < len(entities); i++ {
		for j := i + 1; j < len(entities); j++ {
			e1, e2 := entities[i], entities[j]
			if !sameSentence(e1.StartChar, e2.StartChar) {
				continue
			}
			dist := abs(e2.StartChar - e1.EndChar)
			if dist > maxDistance {
				continue
			}
			relations = append(relations, Relation{
				Subject:    e1,
				Predicate:  "related_to",
				Object:     e2,
				Confidence: 0.4,
				Context:    extractContextSimple(text, e1, e2),
				Metadata:   map[string]interface{}{"method": "cooccurrence"},
			})
		}
	}
	return relations
}

// findEntityByText matches Python's _find_entity logic.
// Strips trailing punctuation and handles "and/or" postfix.
func findEntityByText(raw string, entityMap map[string]Entity) Entity {
	text := strings.TrimSpace(raw)
	// Strip trailing punctuation
	for len(text) > 0 && strings.ContainsAny(text[len(text)-1:], ".,;:!?") {
		text = strings.TrimSpace(text[:len(text)-1])
	}
	key := strings.ToLower(text)
	if ent, ok := entityMap[key]; ok {
		return ent
	}
	// Try removing trailing " and ..." / " or ..."
	for _, sep := range []string{" and ", " or ", ", "} {
		if idx := strings.Index(key, sep); idx > 0 {
			candidate := key[:idx]
			if ent, ok := entityMap[candidate]; ok {
				return ent
			}
		}
	}
	return Entity{}
}

func extractContext(text string, matchStr string) string {
	idx := strings.Index(text, matchStr)
	if idx < 0 {
		return ""
	}
	start := idx - 30
	if start < 0 {
		start = 0
	}
	end := idx + len(matchStr) + 30
	if end > len(text) {
		end = len(text)
	}
	return text[start:end]
}

func extractContextSimple(text string, e1, e2 Entity) string {
	start := min(e1.StartChar, e2.StartChar) - 20
	if start < 0 {
		start = 0
	}
	end := max(e1.EndChar, e2.EndChar) + 20
	if end > len(text) {
		end = len(text)
	}
	return text[start:end]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// splitSentences splits text into sentence spans [start, end),
// matching Python's: re.finditer(r'[^.!?]+(?:[.!?](?=\s|$))+', text)
// Uses Go-compatible regex (no lookahead) with manual space check.
func splitSentences(text string) [][2]int {
	re := regexp.MustCompile(`[^.!?]+[.!?]+`)
	matches := re.FindAllStringIndex(text, -1)
	var spans [][2]int
	for _, m := range matches {
		end := m[1]
		// Punctuation must be followed by space or end-of-string
		if end == len(text) || text[end] == ' ' {
			spans = append(spans, [2]int{m[0], end})
		}
	}
	if len(spans) == 0 && len(text) > 0 {
		spans = append(spans, [2]int{0, len(text)})
	}
	return spans
}
