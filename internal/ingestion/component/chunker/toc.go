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

package chunker

import (
	"regexp"
	"strings"
)

// tocTitlePattern mirrors the Python regex:
//
//	re.match(r"(contents|目录|目次|table of contents|致谢|acknowledge)$", ...)
//
// Used to detect section headers that introduce a table-of-contents block.
var tocTitlePattern = regexp.MustCompile(`(?i)^(contents|目录|目次|table of contents|致谢|acknowledge)$`)

// tocCollapseSpaceRe collapses consecutive whitespace characters (space,
// ideographic space, fullwidth space). Mirrors Python's:
//
//	re.sub(r"( | |\u3000)+", "", get(i).split("@@")[0])
var tocCollapseSpaceRe = regexp.MustCompile(`[ \x{3000}]+`)

// isEnglishTextPattern mirrors Python's is_english character pattern:
//
//	r"[`a-zA-Z0-9\s.,':;/\"?<>!\(\)\-]+"
//
// A text that full-matches this consists entirely of ASCII alphanumeric,
// whitespace, and common punctuation. The backtick is omitted as it is
// irrelevant for section texts.
var isEnglishTextPattern = regexp.MustCompile(`^[a-zA-Z0-9\s.,':;"/?<>!()\-]+$`)

// removeContentsTable mirrors Python's rag/nlp/__init__.py:remove_contents_table.
// It removes table-of-contents entries from the record stream before heading
// detection, preventing TOC entries (like "第一章", "1.1" etc.) from being
// misidentified as real section headings and fragmenting the chunk output.
//
// Algorithm:
//  1. Scan for a line matching the TOC title pattern.
//  2. Pop the TOC header and the next non-empty line (first TOC entry), which
//     provides the prefix pattern.
//  3. For non-English text, the prefix is the first 3 characters; for English,
//     the prefix is the first 2 words (matching Python's `eng` branch).
//  4. Scan forward (up to 128 records) to find the next line starting with the
//     same prefix — this marks the end of the TOC block.
//  5. Remove all entries from the TOC header position to just before the found
//     line, leaving the non-TOC content intact.
func removeContentsTable(records []lineRecord) []lineRecord {
	eng := isEnglishRecords(records)
	i := 0
	for i < len(records) {
		text := tocText(records[i])
		if !tocTitlePattern.MatchString(tocCollapseSpaceRe.ReplaceAllString(text, "")) {
			i++
			continue
		}
		// Pop the TOC header line.
		records = append(records[:i], records[i+1:]...)
		if i >= len(records) {
			break
		}
		// Get the prefix from the next line (first TOC entry).
		prefix := tocPrefix(records[i], eng)
		for prefix == "" {
			records = append(records[:i], records[i+1:]...)
			if i >= len(records) {
				break
			}
			prefix = tocPrefix(records[i], eng)
		}
		if i >= len(records) || prefix == "" {
			break
		}
		// Pop the first TOC entry (used only to determine the prefix).
		records = append(records[:i], records[i+1:]...)
		if i >= len(records) || prefix == "" {
			break
		}
		// Scan forward to find a line that starts with the same prefix.
		// When found, remove all TOC entries between the header and that line.
		for j := i; j < len(records) && j < i+128; j++ {
			if !strings.HasPrefix(tocText(records[j]), prefix) {
				continue
			}
			records = append(records[:i], records[j:]...)
			break
		}
	}
	return records
}

// tocText extracts the clean text from a lineRecord, stripping the "@@"
// suffix and whitespace. Mirrors Python's inline `get(i)` helper:
//
//	(sections[i] if isinstance(sections[i], type("")) else sections[i][0]).strip()
//
// and the subsequent `.split("@@")[0]` call.
func tocText(r lineRecord) string {
	text := r.text
	if idx := strings.Index(text, "@@"); idx >= 0 {
		text = text[:idx]
	}
	return strings.TrimSpace(text)
}

// tocPrefix computes the prefix used to identify TOC entries within a
// block. Mirrors Python's:
//
//	prefix = get(i)[:3] if not eng else " ".join(get(i).split()[:2])
func tocPrefix(r lineRecord, eng bool) string {
	text := tocText(r)
	if !eng {
		// Use rune slicing to match Python's character-level [:3].
		// Go's default byte-slicing would return only one
		// 3-byte CJK character instead of three characters.
		runes := []rune(text)
		if len(runes) < 3 {
			return text
		}
		return string(runes[:3])
	}
	words := strings.Fields(text)
	if len(words) >= 2 {
		return strings.Join(words[:2], " ")
	}
	return text
}

// isEnglishRecords samples up to 200 text records and returns true when
// >80% of sampled records consist entirely of ASCII alphanumeric,
// whitespace, and punctuation characters. Mirrors Python's:
//
//	is_english(random_choices([t for t, _ in sections], k=200))
func isEnglishRecords(records []lineRecord) bool {
	const sampleSize = 200
	count := 0
	eng := 0
	for _, r := range records {
		t := strings.TrimSpace(r.text)
		if t == "" {
			continue
		}
		count++
		if isEnglishTextPattern.MatchString(t) {
			eng++
		}
		if count >= sampleSize {
			break
		}
	}
	if count == 0 {
		return false
	}
	return float64(eng)/float64(count) > 0.8
}
