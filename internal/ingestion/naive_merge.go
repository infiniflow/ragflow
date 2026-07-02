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

package ingestion

import (
	"regexp"
	"sort"
	"strings"

	"ragflow/internal/tokenizer"
)

// MergedSegment is a merged text chunk produced by NaiveMerge.
// SectionIndices tracks which input sections contributed to this segment,
// enabling the caller to associate images from those sections.
type MergedSegment struct {
	Text           string `json:"text"`
	SectionIndices []int  `json:"section_indices"`
	Position       string `json:"position,omitempty"`
}

// indexedText pairs text content with its original index and position.
type indexedText struct {
	text string
	idx  int
	pos  string
}

// NaiveMerge merges texts into chunks by token count with delimiter-based
// splitting and overlap. Matches Python naive_merge_with_images().
//
// Parameters:
//   - texts: input text segments (one per parsed section)
//   - positions: optional position strings (one per section, may be nil)
//   - chunkTokenNum: max token count per chunk (default 128 in Python)
//   - delimiter: delimiter pattern, supports backtick-quoted custom delimiters
//   - overlappedPercent: overlap percentage (0-100)
func NaiveMerge(texts []string, positions []string, chunkTokenNum int, delimiter string, overlappedPercent float64) []MergedSegment {
	if len(texts) == 0 {
		return nil
	}

	// Filter out empty texts but track original indices and positions
	var filtered []indexedText
	for i, t := range texts {
		if strings.TrimSpace(t) != "" {
			pos := ""
			if positions != nil && i < len(positions) {
				pos = positions[i]
			}
			filtered = append(filtered, indexedText{text: t, idx: i, pos: pos})
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	// Parse delimiter: extract custom (backtick-quoted) and normal delimiters
	customDels, normalPattern := parseDelimiters(delimiter)

	// Custom delimiter mode: each segment is its own chunk, no merging by token count
	if len(customDels) > 0 {
		return mergeByCustomDelimiters(filtered, customDels, chunkTokenNum)
	}

	// Normal mode: split oversized texts at delimiters, merge by token count
	return mergeByTokenCount(filtered, normalPattern, chunkTokenNum, overlappedPercent)
}

// ── Delimiter parsing ───────────────────────────────────────────────────────

// parseDelimiters extracts custom (backtick-quoted) and normal delimiters.
// Matches Python get_delimiters().
func parseDelimiters(delimiter string) (custom []string, pattern string) {
	re := regexp.MustCompile("`([^`]+)`")
	matches := re.FindAllStringSubmatchIndex(delimiter, -1)

	var normalChars []rune
	lastEnd := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		normalChars = append(normalChars, []rune(delimiter[lastEnd:start])...)
		custom = append(custom, delimiter[m[2]:m[3]])
		lastEnd = end
	}
	if lastEnd < len(delimiter) {
		normalChars = append(normalChars, []rune(delimiter[lastEnd:])...)
	}

	var dels []string
	seen := make(map[string]bool)
	for _, r := range normalChars {
		s := string(r)
		if !seen[s] && s != "" {
			dels = append(dels, regexp.QuoteMeta(s))
			seen[s] = true
		}
	}

	sort.Slice(dels, func(i, j int) bool {
		return len(dels[i]) > len(dels[j])
	})

	if len(dels) > 0 {
		pattern = strings.Join(dels, "|")
	}
	return
}

// ── Custom delimiter mode ───────────────────────────────────────────────────

func mergeByCustomDelimiters(texts []indexedText, customDels []string, chunkTokenNum int) []MergedSegment {
	sort.Slice(customDels, func(i, j int) bool {
		return len(customDels[i]) > len(customDels[j])
	})
	var escaped []string
	for _, d := range customDels {
		escaped = append(escaped, regexp.QuoteMeta(d))
	}
	pattern := regexp.MustCompile("(" + strings.Join(escaped, "|") + ")")

	var result []MergedSegment
	for _, it := range texts {
		parts := pattern.Split(it.text, -1)
		for _, part := range parts {
			if part == "" {
				continue
			}
			isDelimiter := false
			for _, d := range customDels {
				if part == d {
					isDelimiter = true
					break
				}
			}
			if isDelimiter {
				continue
			}

			textSeg := "\n" + part
			pos := resolvePosition(textSeg, it.pos)

			result = append(result, MergedSegment{
				Text:           textSeg,
				SectionIndices: []int{it.idx},
				Position:       pos,
			})
		}
	}
	return result
}

// ── Normal (token-count-based) mode ─────────────────────────────────────────

func mergeByTokenCount(texts []indexedText, delimPattern string, chunkTokenNum int, overlappedPercent float64) []MergedSegment {
	var result []MergedSegment

	threshold := float64(chunkTokenNum) * (100.0 - overlappedPercent) / 100.0

	for _, it := range texts {
		var subSegments []indexedText
		if delimPattern != "" && tokenizer.NumTokensFromString(it.text) >= chunkTokenNum {
			re := regexp.MustCompile("(" + delimPattern + ")")
			parts := re.Split(it.text, -1)
			for _, part := range parts {
				if part == "" {
					continue
				}
				if matched, _ := regexp.MatchString("^("+delimPattern+")$", part); matched {
					continue
				}
				subSegments = append(subSegments, indexedText{
					text: "\n" + part,
					idx:  it.idx,
					pos:  it.pos,
				})
			}
		} else {
			subSegments = append(subSegments, indexedText{
				text: "\n" + it.text,
				idx:  it.idx,
				pos:  it.pos,
			})
		}

		for _, sub := range subSegments {
			tk := tokenizer.NumTokensFromString(sub.text)
			pos := resolvePosition(sub.text, sub.pos)

			if len(result) == 0 {
				result = append(result, MergedSegment{
					Text:           sub.text,
					SectionIndices: []int{sub.idx},
					Position:       pos,
				})
				continue
			}

			last := &result[len(result)-1]
			lastTk := tokenizer.NumTokensFromString(last.Text)

			if lastTk > int(threshold) {
				newText := sub.text
				if overlappedPercent > 0 && lastTk > 0 {
					overlapChars := int(float64(len([]rune(last.Text))) * (100.0 - overlappedPercent) / 100.0)
					if overlapChars > 0 && overlapChars < len([]rune(last.Text)) {
						overlap := string([]rune(last.Text)[overlapChars:])
						newText = overlap + sub.text
					}
				}
				result = append(result, MergedSegment{
					Text:           newText,
					SectionIndices: []int{sub.idx},
					Position:       pos,
				})
			} else {
				last.Text += sub.text
				last.SectionIndices = appendUnique(last.SectionIndices, sub.idx)
				// Position: keep the first non-empty position
				if last.Position == "" && pos != "" {
					last.Position = pos
				}
			}
			_ = tk
		}
	}

	return result
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// resolvePosition applies Python position rules:
//   - if token count < 8 → clear position
//   - if position is not already in text → append to text (done by caller)
// Returns the resolved position string.
func resolvePosition(text, pos string) string {
	if pos == "" {
		return ""
	}
	tk := tokenizer.NumTokensFromString(text)
	if tk < 8 {
		return ""
	}
	return pos
}

func appendUnique(slice []int, val int) []int {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}
