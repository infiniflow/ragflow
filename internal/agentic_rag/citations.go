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

package agentic_rag

import (
	"regexp"
	"strconv"
	"strings"
)

// Citation support for agentic deliverables.
//
// The naive pipeline earns its [ID:N] markers two probabilistic ways: a
// prompt asking the model to emit them, and an embedding-similarity pass
// (InsertCitations) guessing which sentence came from which chunk. The
// agentic answer needs neither: EVERY factual line already cites
// its provenance explicitly as `chunk_id: <id>`, exactly as the tool output
// printed it. So the port
// is deterministic: extract the cited ids, and let the caller (chat
// pipeline) fetch those chunks and number them; [ID:N] markers are then
// appended mechanically, N being the chunk's position in the reference
// payload the frontend already resolves ([ID:N] -> reference.chunks[N],
// N 0-based — the same contract naive answers ship).

// citedChunkIDRe matches the `chunk_id: <id>` provenance field of the FOS
// format (Tested / Eliminated / Retained / Clue lines). Web-sourced lines
// carry no chunk_id by design and are naturally excluded.
var citedChunkIDRe = regexp.MustCompile(`chunk_id:\s*([A-Za-z0-9_-]+)`)

// citationMarkerFollows is what a chunk_id occurrence already carrying a
// citation marker starts with — insertion skips these so repeated calls are
// idempotent.
const citationMarkerFollows = "[ID:"

// ExtractCitedChunkIDs returns the chunk ids the deliverable cites, in
// first-appearance order, deduplicated. Ids that appear on several lines
// (the same chunk backing a Tested and a Clue line) yield ONE entry: the
// reference payload lists each chunk once and every occurrence of the id
// gets the same [ID:N] marker.
func ExtractCitedChunkIDs(final string) []string {
	if final == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, m := range citedChunkIDRe.FindAllStringSubmatch(final, -1) {
		id := m[1]
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// InsertCitationMarkers appends the canonical ` [ID:N]` citation markers for
// every `chunk_id: <id>` occurrence, N being the id's 0-based position in
// chunkIDs. The caller MUST build the reference payload's chunks array in the
// same chunkIDs order — the frontend resolves the marker as
// reference.chunks[N].
//
// Placement is LINE-END, after the line's last backtick: the FOS format wraps
// every matrix/chain line in backticks, and the frontend's citation pass
// skips text inside <code> nodes — a marker inserted next to the chunk_id
// field would render as literal text and never become a clickable citation
// (observed on the 关羽 run: markers present in the SSE payload, invisible in
// the UI). Lines without backticks take the marker right after the last id.
// Ids absent from chunkIDs (e.g. a cited chunk that failed to load) stay
// unmarked rather than pointing at a wrong index, and a line that already
// carries a marker is left untouched, making the rewrite idempotent.
func InsertCitationMarkers(final string, chunkIDs []string) string {
	if final == "" || len(chunkIDs) == 0 {
		return final
	}
	idx := make(map[string]int, len(chunkIDs))
	for i, id := range chunkIDs {
		if _, dup := idx[id]; dup {
			continue
		}
		idx[id] = i
	}
	lines := strings.Split(final, "\n")
	for li, line := range lines {
		if !strings.Contains(line, "chunk_id:") || strings.Contains(line, citationMarkerFollows) {
			continue // nothing to mark / already marked: idempotent
		}
		matches := citedChunkIDRe.FindAllStringSubmatchIndex(line, -1)
		if len(matches) == 0 {
			continue
		}
		var markers strings.Builder
		seen := make(map[int]bool, len(matches))
		lastEnd := 0
		for _, m := range matches {
			lastEnd = m[1]
			n, ok := idx[line[m[2]:m[3]]]
			if !ok || seen[n] {
				continue // unresolvable id: keep the text, no marker
			}
			seen[n] = true
			markers.WriteString(" [ID:" + strconv.Itoa(n) + "]")
		}
		if markers.Len() == 0 {
			continue
		}
		// Outside the code span when the line is backtick-wrapped; the
		// marker annotates the whole line, which cites exactly this chunk.
		insertAt := lastEnd
		if bt := strings.LastIndex(line, "`"); bt >= lastEnd {
			insertAt = bt + 1
		}
		lines[li] = line[:insertAt] + markers.String() + line[insertAt:]
	}
	return strings.Join(lines, "\n")
}
