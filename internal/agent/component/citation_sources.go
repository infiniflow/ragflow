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

package component

import (
	"regexp"
	"strings"

	"ragflow/internal/agent/component/prompts"
	"ragflow/internal/tokenizer"
)

// citationGroundingSourceTokenBudget caps the sources injected into the
// post-stream citation prompt. A multi-search Agent run can accumulate dozens
// of chunks; without a cap the second LLM call eventually overflows the
// model's context window.
const citationGroundingSourceTokenBudget = 30000

// citationMarkerMaxLength bounds a citation marker's full text length. It is
// far above [ID:<chunk id>] for any real chunk id, so a candidate longer than
// this is not a marker. The stream filter stops holding at this bound and the
// final filter leaves such text literal, keeping the streamed and saved
// content identical.
const citationMarkerMaxLength = 128

// citationGroundingMarkerCaptureRe matches a canonical [ID:<id>] marker and
// captures the id. Whitespace after "ID:" is tolerated and normalized away.
// The id cannot contain "[": an unclosed [ID: must not swallow the text up to
// a later ] (for example a Markdown link that follows it).
var citationGroundingMarkerCaptureRe = regexp.MustCompile(`\[ID:\s*([^\[\]]*)\]`)

// limitCitationSources keeps sources in order until the next one would exceed
// the token budget. Empty sources are skipped. The first valid source is
// always kept so a single oversized chunk still gets a grounding attempt.
func limitCitationSources(sources []prompts.CitationSource, budget int) []prompts.CitationSource {
	if budget <= 0 {
		return sources
	}
	kept := make([]prompts.CitationSource, 0, len(sources))
	used := 0
	for _, source := range sources {
		id := strings.TrimSpace(source.ID)
		if id == "" || strings.TrimSpace(source.Content) == "" {
			continue
		}
		tokens := tokenizer.NumTokensFromString(id) + tokenizer.NumTokensFromString(source.Content)
		if len(kept) > 0 && used+tokens > budget {
			break
		}
		kept = append(kept, prompts.CitationSource{ID: id, Content: source.Content})
		used += tokens
	}
	return kept
}

// filterCitationMarkers drops [ID:<id>] markers whose id is not in
// allowedIDs and normalizes the whitespace of the ones that remain.
func filterCitationMarkers(content string, allowedIDs []string) string {
	allowed := make(map[string]struct{}, len(allowedIDs))
	for _, id := range allowedIDs {
		if id = strings.TrimSpace(id); id != "" {
			allowed[id] = struct{}{}
		}
	}
	return filterCitationMarkersWithSet(content, allowed)
}

func filterCitationMarkersWithSet(content string, allowed map[string]struct{}) string {
	if content == "" {
		return ""
	}
	return citationGroundingMarkerCaptureRe.ReplaceAllStringFunc(content, func(marker string) string {
		// A marker longer than the cap is not a real citation id; keep the text
		// visible instead of silently dropping it.
		if len(marker) > citationMarkerMaxLength {
			return marker
		}
		match := citationGroundingMarkerCaptureRe.FindStringSubmatch(marker)
		if len(match) < 2 {
			return ""
		}
		id := strings.TrimSpace(match[1])
		if _, ok := allowed[id]; !ok {
			return ""
		}
		return "[ID:" + id + "]"
	})
}

// citationMarkerStreamFilter applies the same marker filtering to a streamed
// response. A marker can be split across deltas, so trailing text that could
// still become a marker is buffered until the next delta or Flush.
type citationMarkerStreamFilter struct {
	allowed map[string]struct{}
	pending string
}

func newCitationMarkerStreamFilter(allowedIDs []string) *citationMarkerStreamFilter {
	allowed := make(map[string]struct{}, len(allowedIDs))
	for _, id := range allowedIDs {
		if id = strings.TrimSpace(id); id != "" {
			allowed[id] = struct{}{}
		}
	}
	return &citationMarkerStreamFilter{allowed: allowed}
}

func (f *citationMarkerStreamFilter) Push(delta string) string {
	if delta == "" {
		return ""
	}
	text := f.pending + delta
	f.pending = ""
	if hold := citationMarkerHoldStart(text); hold >= 0 && len(text)-hold <= citationMarkerMaxLength {
		f.pending = text[hold:]
		text = text[:hold]
	}
	return filterCitationMarkersWithSet(text, f.allowed)
}

func (f *citationMarkerStreamFilter) Flush() string {
	text := f.pending
	f.pending = ""
	if text == "" {
		return ""
	}
	// Apply the same filter as the final (non-stream) path: a well-formed
	// marker is filtered or kept, while an unclosed [ID:... stays literal
	// instead of swallowing the text that follows it.
	return filterCitationMarkersWithSet(text, f.allowed)
}

// citationMarkerHoldStart returns the index of the trailing text that must be
// buffered because it may still turn into a citation marker.
func citationMarkerHoldStart(text string) int {
	if idx := strings.LastIndex(text, "[ID:"); idx >= 0 && !strings.Contains(text[idx:], "]") {
		return idx
	}
	for _, prefix := range []string{"[ID:", "[ID", "[I", "["} {
		if strings.HasSuffix(text, prefix) {
			return len(text) - len(prefix)
		}
	}
	return -1
}
