//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except under the License.
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package chunker

// Token-count cap for the title-chunker family (GroupTitle / HierarchyTitle),
// mirroring Python title_chunker/common.py:_enforce_token_cap (#18455).
//
// A built chunk that exceeds `chunk_token_cap` tokens is re-split on sentence
// boundaries into <= cap sub-chunks; a single boundary-less run that still
// exceeds the cap is hard-split so the ceiling always holds. Table/image
// chunks are atomic and never split. Every sub-chunk keeps the source chunk's
// merged PDF position matrix so each one still gets its page-region preview
// image and position highlight.

import (
	"strings"
	"unicode/utf8"

	"ragflow/internal/tokenizer"
)

// realNumTokens / realTrimToTokenLimit are the production tokenizer functions.
// They are referenced through package-level vars (numTokens /
// trimToTokenLimit) so tests can swap in a deterministic stub.
var (
	realNumTokens        = tokenizer.NumTokensFromString
	realTrimToTokenLimit = tokenizer.TrimContentToTokenLimit

	numTokens        = realNumTokens
	trimToTokenLimit = realTrimToTokenLimit
)

// titleTokenCount counts tokens for text. numTokensFromString returns 0 when
// the encoder is unavailable; in that case fall back to the rune count so the
// cap is still enforced (mirrors Python #18455's character-count fallback).
func titleTokenCount(text string) int {
	if text == "" {
		return 0
	}
	n := numTokens(text)
	if n == 0 {
		return utf8.RuneCountInString(text)
	}
	return n
}

// titleSentenceSplit splits text on sentence boundaries, keeping each
// delimiter attached to the preceding sentence so the concatenation of the
// returned pieces reproduces the original text exactly. It uses the shared
// sentenceBoundaryRe (Python _sentence_boundary.py SENTENCE_BOUNDARY_RE),
// which includes the English ". " boundary.
func titleSentenceSplit(text string) []string {
	if text == "" {
		return nil
	}
	idxs := sentenceBoundaryRe.FindAllStringIndex(text, -1)
	var out []string
	prev := 0
	for _, idx := range idxs {
		if idx[0] > prev {
			out = append(out, text[prev:idx[1]])
		} else {
			out = append(out, text[idx[0]:idx[1]])
		}
		prev = idx[1]
	}
	if prev < len(text) {
		out = append(out, text[prev:])
	}
	return out
}

// runePrefix returns the first n runes of s (a rune-safe prefix; never splits
// a multi-byte character). Used by the offline/hard-split fallback.
func runePrefix(s string, n int) string {
	if n <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count >= n {
			return s[:i]
		}
		count++
	}
	return s
}

// hardSplitByTokens hard-splits text into pieces whose token count is <= cap.
// It mirrors Python's _hard_split_by_tokens (token_utils.truncate prefix).
// When the tokenizer is unavailable (numTokens returns 0) it degrades to a
// rune-prefix of length cap, keeping the ceiling intact (Python #18455
// character-count fallback).
func hardSplitByTokens(text string, cap int) []string {
	if cap <= 0 {
		if text == "" {
			return nil
		}
		return []string{text}
	}
	var out []string
	rest := text
	for utf8.RuneCountInString(rest) > 0 {
		// A remainder already within the token cap stays whole. This must be
		// checked on TOKENS: for English text runes far exceed tokens, so
		// comparing runes against the cap would over-fragment an in-cap tail.
		if titleTokenCount(rest) <= cap {
			out = append(out, rest)
			break
		}
		head := trimToTokenLimit(rest, cap)
		// No progress (tokenizer unavailable / cannot shrink): fall back to a
		// rune prefix so the loop always makes progress and the ceiling holds.
		if head == "" || head == rest || !strings.HasPrefix(rest, head) {
			head = runePrefix(rest, cap)
			if head == "" || head == rest {
				out = append(out, rest)
				break
			}
		}
		out = append(out, head)
		rest = rest[len(head):]
	}
	return out
}

// enforceTitleTokenCap applies the hard token ceiling to every text chunk
// after build_chunks. Mirrors Python BaseTitleChunker._enforce_token_cap:
//   - cap <= 0 disables the ceiling (chunks returned unchanged);
//   - non-text chunks (table/image) are atomic and skipped;
//   - a text chunk already within cap is kept;
//   - an over-cap text chunk is re-split via splitTitleChunkByCap.
func enforceTitleTokenCap(chunks []map[string]any, cap int) []map[string]any {
	if cap <= 0 {
		return chunks
	}
	out := make([]map[string]any, 0, len(chunks))
	for _, chunk := range chunks {
		if toStringOrDefault(chunk["doc_type_kwd"], "text") != "text" {
			out = append(out, chunk)
			continue
		}
		text := toString(chunk["text"])
		if text == "" || titleTokenCount(text) <= cap {
			out = append(out, chunk)
			continue
		}
		out = append(out, splitTitleChunkByCap(chunk, cap)...)
	}
	return out
}

// splitTitleChunkByCap re-splits one oversized text chunk into <= cap
// sub-chunks. Sentences are tried first (greedy grouping); any remaining
// over-cap segment is hard-split. Every sub-chunk keeps the source chunk's
// merged PDF position matrix (the coordinates are coarse, source-chunk
// level), so the on-demand crop pass can attach a preview image to each
// sub-chunk.
func splitTitleChunkByCap(chunk map[string]any, cap int) []map[string]any {
	text := toString(chunk["text"])
	sentences := titleSentenceSplit(text)
	if len(sentences) == 0 {
		return []map[string]any{chunk}
	}
	groups := []string{}
	current := ""
	for _, s := range sentences {
		cand := s
		if current != "" {
			cand = current + s
		}
		if current != "" && titleTokenCount(cand) > cap {
			groups = append(groups, current)
			current = s
		} else {
			current = cand
		}
	}
	if current != "" {
		groups = append(groups, current)
	}
	finalGroups := []string{}
	for _, g := range groups {
		if titleTokenCount(g) <= cap {
			finalGroups = append(finalGroups, g)
		} else {
			finalGroups = append(finalGroups, hardSplitByTokens(g, cap)...)
		}
	}
	if len(finalGroups) == 0 {
		return []map[string]any{chunk}
	}

	// Proportionally slice PDF positions so each sub-chunk's screenshot
	// crops only its own vertical region (fix for chunk-screenshot mismatch).
	// When no positions are present the old shallow-copy behaviour is kept.
	_, hasPDF := chunk["_pdf_positions"]
	_, hasPos := chunk["positions"]
	if !hasPDF && !hasPos {
		out := make([]map[string]any, 0, len(finalGroups))
		for _, g := range finalGroups {
			sub := shallowCopyChunk(chunk)
			sub["text"] = g
			out = append(out, sub)
		}
		return out
	}
	total := totalRunes(finalGroups)
	if total == 0 {
		out := make([]map[string]any, 0, len(finalGroups))
		for _, g := range finalGroups {
			sub := shallowCopyChunk(chunk)
			sub["text"] = g
			out = append(out, sub)
		}
		return out
	}
	out := make([]map[string]any, 0, len(finalGroups))
	cum := 0
	for _, g := range finalGroups {
		pcRunes := utf8.RuneCountInString(g)
		startRatio := float64(cum) / float64(total)
		endRatio := float64(cum+pcRunes) / float64(total)
		sub := shallowCopyChunk(chunk)
		sub["text"] = g
		if hasPDF {
			if sliced := sliceAnyPositions(chunk["_pdf_positions"], startRatio, endRatio); sliced != nil {
				sub["_pdf_positions"] = sliced
			}
		}
		if hasPos {
			if sliced := sliceAnyPositions(chunk["positions"], startRatio, endRatio); sliced != nil {
				sub["positions"] = sliced
			}
		}
		out = append(out, sub)
		cum += pcRunes
	}
	return out
}

// shallowCopyChunk returns a shallow copy of c. Coordinate matrices
// ([][]float64) are deliberately shared by reference between the source
// chunk and its cap-split sub-chunks: nothing in the split or the downstream
// read-only passes (on-demand crop, position indexing) mutates a matrix, so
// the sharing is intentional and safe.
func shallowCopyChunk(c map[string]any) map[string]any {
	m := make(map[string]any, len(c))
	for k, v := range c {
		m[k] = v
	}
	return m
}

// toStringOrDefault returns the string value of v, or def when v is absent /
// nil / not a string. Used to treat a missing doc_type_kwd as "text".
func toStringOrDefault(v any, def string) string {
	if v == nil {
		return def
	}
	if s, ok := v.(string); ok {
		return s
	}
	return def
}
