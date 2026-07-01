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
}

// indexedText pairs text content with its original index in the input slice.
type indexedText struct {
	text string
	idx  int
}

// NaiveMerge merges texts into chunks by token count with delimiter-based
// splitting and overlap. Matches Python naive_merge_with_images().
//
// Parameters:
//   - texts: input text segments (one per parsed section)
//   - chunkTokenNum: max token count per chunk (default 128 in Python)
//   - delimiter: delimiter pattern, supports backtick-quoted custom delimiters
//   - overlappedPercent: overlap percentage (0-100)
func NaiveMerge(texts []string, chunkTokenNum int, delimiter string, overlappedPercent float64) []MergedSegment {
	if len(texts) == 0 {
		return nil
	}

	// Filter out empty texts but track original indices
	var filtered []indexedText
	for i, t := range texts {
		if strings.TrimSpace(t) != "" {
			filtered = append(filtered, indexedText{text: t, idx: i})
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
	// Extract backtick-quoted custom delimiters: `CHAPTER`, `---`, etc.
	re := regexp.MustCompile("`([^`]+)`")
	matches := re.FindAllStringSubmatchIndex(delimiter, -1)

	var normalChars []rune
	lastEnd := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		// Characters before this match are normal delimiters
		normalChars = append(normalChars, []rune(delimiter[lastEnd:start])...)
		// The captured group is a custom delimiter
		custom = append(custom, delimiter[m[2]:m[3]])
		lastEnd = end
	}
	// Remaining characters after last match
	if lastEnd < len(delimiter) {
		normalChars = append(normalChars, []rune(delimiter[lastEnd:])...)
	}

	// Build pattern from normal delimiter characters
	var dels []string
	seen := make(map[string]bool)
	for _, r := range normalChars {
		s := string(r)
		if !seen[s] && s != "" {
			dels = append(dels, regexp.QuoteMeta(s))
			seen[s] = true
		}
	}

	// Sort by length desc (longest match first)
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
	// Build pattern: sort by length desc, escape, join
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
			// Skip parts that are exact matches of the delimiter
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

			seg := MergedSegment{
				Text:           "\n" + part,
				SectionIndices: []int{it.idx},
			}
			result = append(result, seg)
		}
	}
	return result
}

// ── Normal (token-count-based) mode ─────────────────────────────────────────

func mergeByTokenCount(texts []indexedText, delimPattern string, chunkTokenNum int, overlappedPercent float64) []MergedSegment {
	var result []MergedSegment

	// Token threshold for starting a new chunk
	threshold := float64(chunkTokenNum) * (100.0 - overlappedPercent) / 100.0

	for _, it := range texts {
		// Split oversized text at delimiter boundaries
		var subSegments []indexedText
		if delimPattern != "" && tokenizer.NumTokensFromString(it.text) >= chunkTokenNum {
			re := regexp.MustCompile("(" + delimPattern + ")")
			parts := re.Split(it.text, -1)
			for _, part := range parts {
				if part == "" {
					continue
				}
				// Skip delimiter-only parts
				if matched, _ := regexp.MatchString("^("+delimPattern+")$", part); matched {
					continue
				}
				subSegments = append(subSegments, indexedText{text: "\n" + part, idx: it.idx})
			}
		} else {
			subSegments = append(subSegments, indexedText{text: "\n" + it.text, idx: it.idx})
		}

		// Merge sub-segments into chunks
		for _, sub := range subSegments {
			if len(result) == 0 {
				result = append(result, MergedSegment{
					Text:           sub.text,
					SectionIndices: []int{sub.idx},
				})
				continue
			}

			last := &result[len(result)-1]
			lastTk := tokenizer.NumTokensFromString(last.Text)

			// Start new chunk if threshold exceeded
			if lastTk > int(threshold) {
				newText := sub.text
				// Apply overlap from previous chunk
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
				})
			} else {
				// Append to current chunk
				last.Text += sub.text
				last.SectionIndices = appendUnique(last.SectionIndices, sub.idx)
			}
		}
	}

	return result
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func appendUnique(slice []int, val int) []int {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}
