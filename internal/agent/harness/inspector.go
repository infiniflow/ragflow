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

package harness

import (
	"crypto/md5"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Inspector tools (mirrors Python harness/tools/inspector.py): operate on the
// chunks already accumulated in the shared Kbinfos, rather than issuing new
// retrieval. They are stateless reads of the in-memory evidence pool.

// InspectorOpenContext expands context around a chunk (mirrors open_context):
// the chunk plus its 2 neighbours on each side.
func InspectorOpenContext(chunks []map[string]interface{}, chunkID string) []map[string]interface{} {
	idx := findChunkIndex(chunks, chunkID)
	if idx < 0 {
		return nil
	}
	start := idx - 2
	if start < 0 {
		start = 0
	}
	end := idx + 2
	if end > len(chunks) {
		end = len(chunks)
	}
	return append([]map[string]interface{}(nil), chunks[start:end]...)
}

// InspectorCompareSources returns the chunks matching the given ids (mirrors
// compare_sources).
func InspectorCompareSources(chunks []map[string]interface{}, chunkIDs []string) []map[string]interface{} {
	if len(chunkIDs) == 0 {
		return nil
	}
	want := map[string]bool{}
	for _, id := range chunkIDs {
		want[id] = true
	}
	var out []map[string]interface{}
	for _, c := range chunks {
		if want[chunkIDOf(c)] {
			out = append(out, c)
		}
	}
	return out
}

// InspectorGrepWithin narrows chunks of one document to their keyword-bearing
// sentences (mirrors grep_within + _narrow_by_keywords). It returns copies so the
// shared kbinfos citation pool is never mutated.
func InspectorGrepWithin(chunks []map[string]interface{}, docID, pattern string) []map[string]interface{} {
	kwds := keywordList(pattern)
	var scoped []map[string]interface{}
	for _, c := range chunks {
		if docIDOf(c) == docID {
			scoped = append(scoped, c)
		}
	}
	if len(kwds) == 0 || len(scoped) == 0 {
		return scoped
	}

	var out []map[string]interface{}
	dedup := map[string]bool{}
	for _, c := range scoped {
		cp := cloneChunk(c)
		content := chunkText(cp)
		narrowed := narrowContent(content, kwds)
		if narrowed == "" {
			continue
		}
		key := md5.Sum([]byte(narrowed))
		if dedup[fmt.Sprintf("%x", key)] {
			continue
		}
		dedup[fmt.Sprintf("%x", key)] = true
		cp["content_with_weight"] = narrowed
		if _, ok := cp["content"]; ok {
			cp["content"] = narrowed
		}
		delete(cp, "highlight")
		out = append(out, cp)
	}
	return out
}

// InspectorRequestAdjacent returns count neighbours before or after a chunk
// (mirrors request_adjacent).
func InspectorRequestAdjacent(chunks []map[string]interface{}, chunkID, direction string, count int) []map[string]interface{} {
	idx := findChunkIndex(chunks, chunkID)
	if idx < 0 {
		return nil
	}
	if count <= 0 {
		count = 3
	}
	var start, end int
	if direction == "prev" {
		start = idx - count
		if start < 0 {
			start = 0
		}
		end = idx
	} else {
		start = idx + 1
		end = start + count
		if end > len(chunks) {
			end = len(chunks)
		}
	}
	if start >= len(chunks) || start >= end {
		return nil
	}
	return append([]map[string]interface{}(nil), chunks[start:end]...)
}

// findChunkIndex mirrors Python _find_chunk_index.
func findChunkIndex(chunks []map[string]interface{}, chunkID string) int {
	for i, c := range chunks {
		if chunkIDOf(c) == chunkID {
			return i
		}
	}
	return -1
}

// chunkIDOf mirrors Python _chunk_id: chunk_id or id.
func chunkIDOf(c map[string]interface{}) string {
	if v, ok := c["chunk_id"].(string); ok && v != "" {
		return v
	}
	if v, ok := c["id"].(string); ok && v != "" {
		return v
	}
	return ""
}

// docIDOf returns the chunk's doc_id.
func docIDOf(c map[string]interface{}) string {
	if v, ok := c["doc_id"].(string); ok {
		return v
	}
	return ""
}

// keywordList mirrors _narrow_by_keywords' keyword parsing: comma-separated; when
// fewer than 3 comma terms, split on spaces into bigrams.
func keywordList(keywords string) []string {
	comma := splitTrim(keywords, ",")
	if len(comma) < 3 {
		words := splitTrim(keywords, " ")
		var bigrams []string
		for i := 0; i+1 < len(words); i++ {
			bigrams = append(bigrams, words[i]+" "+words[i+1])
		}
		return bigrams
	}
	return comma
}

func splitTrim(s, sep string) []string {
	var out []string
	for _, part := range strings.Split(s, sep) {
		part = strings.ToLower(strings.TrimSpace(part))
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// narrowContent mirrors Python _narrow_content: keep keyword-bearing sentences
// (+/- 1 neighbour), highlight matches, wrap in "...". Returns "" when no
// keyword occurs anywhere in the content.
func narrowContent(content string, kwds []string) string {
	sents := splitSentences(content)
	if len(sents) == 0 {
		return ""
	}
	keep := map[int]bool{}
	matched := false
	for i, s := range sents {
		low := strings.ToLower(s)
		for _, kw := range kwds {
			if strings.Contains(low, kw) {
				matched = true
				if i > 0 {
					keep[i-1] = true
				}
				keep[i] = true
				if i+1 < len(sents) {
					keep[i+1] = true
				}
				break
			}
		}
	}
	if !matched {
		return ""
	}
	idx := make([]int, 0, len(keep))
	for i := range keep {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	var b strings.Builder
	for _, i := range idx {
		b.WriteString(sents[i])
	}
	narrowed := strings.TrimSpace(b.String())
	return "..." + highlightKeywords(narrowed, kwds) + "..."
}

// highlightKeywords mirrors Python _highlight_keywords: wrap the longest keyword
// matches in *asterisks*, case-insensitive.
func highlightKeywords(text string, kwds []string) string {
	terms := make([]string, 0, len(kwds))
	seen := map[string]bool{}
	for _, k := range kwds {
		if k != "" && !seen[k] {
			seen[k] = true
			terms = append(terms, k)
		}
	}
	if len(terms) == 0 {
		return text
	}
	sort.Slice(terms, func(i, j int) bool { return len(terms[i]) > len(terms[j]) })
	parts := make([]string, len(terms))
	for i, t := range terms {
		parts[i] = regexp.QuoteMeta(t)
	}
	re := regexp.MustCompile("(?i)" + strings.Join(parts, "|"))
	return re.ReplaceAllString(text, "*$0*")
}

// cloneChunk returns a shallow copy of a chunk map so inspector narrowing never
// mutates the shared kbinfos citation pool.
func cloneChunk(c map[string]interface{}) map[string]interface{} {
	cp := make(map[string]interface{}, len(c))
	for k, v := range c {
		cp[k] = v
	}
	return cp
}
