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
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Citation support for agentic deliverables.
//
// The naive pipeline earns its [ID:N] markers two probabilistic ways: a
// prompt asking the model to emit them, and an embedding-similarity pass
// (InsertCitations) guessing which sentence came from which chunk. The
// agentic deliverable needs neither: EVERY matrix/chain line already cites
// its provenance explicitly as `chunk_id: <id>` — fields the answer auditor
// verifies character-for-character against list_chunks output. So the port
// is deterministic: extract the cited ids, and let the caller (chat
// pipeline) fetch those chunks and number them; [ID:N] markers are then
// appended mechanically, N being the chunk's position in the reference
// payload the frontend already resolves ([ID:N] -> reference.chunks[N],
// N 0-based — the same contract naive answers ship).

// citedChunkIDRe matches an explicit `chunk_id: <id>` provenance field for a
// corpus chunk or web passage.
var citedChunkIDRe = regexp.MustCompile(`chunk_id:\s*([A-Za-z0-9_-]+)`)

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

// RewriteProvenance rewrites each provenance tuple IN PLACE: the tuple (or a bare
// `chunk_id: <id>` field) becomes the canonical `[ID:N]` marker at the same position, N
// being the citation key's 0-based position in the reference payload. index maps a key — a
// corpus chunk_id, or a web url — to that position, and the caller builds the payload in
// the same order, so the client resolves the marker as reference.chunks[N].
//
// Nothing of the machine-readable field reaches the user: the whole tuple goes, including
// its doc/snippet columns. A tuple whose key is not in index keeps its text, so an
// unverifiable citation stays visible rather than silently vanishing or pointing at a
// wrong index; that also makes the pass idempotent (a rewritten line no longer holds a
// tuple).
func RewriteProvenance(answer string, index map[string]int) string {
	if answer == "" || len(index) == 0 {
		return answer
	}
	lines := strings.Split(answer, "\n")
	for li, line := range lines {
		if !strings.Contains(line, "chunk_id:") && !strings.Contains(line, "source: web") {
			continue
		}
		lines[li] = replaceProvenance(line, index)
	}
	return strings.Join(lines, "\n")
}

// replaceProvenance rewrites one line's resolvable provenance fields in place. A
// parenthesised tuple is replaced whole (so its doc/snippet columns disappear with it); a
// bare `chunk_id: <id>` field is replaced on its own, so an answer that never followed the
// tuple contract still gets markers.
func replaceProvenance(line string, index map[string]int) string {
	var b strings.Builder
	b.Grow(len(line))
	for i := 0; i < len(line); {
		if line[i] == '(' {
			if end, ok := tupleEnd(line, i); ok {
				if n, ok := tupleMark(line[i:end+1], index); ok {
					writeMarker(&b, n)
				} else {
					b.WriteString(line[i : end+1]) // not a provenance tuple, or unresolvable
				}
				i = end + 1
				continue
			}
		}
		if m := citedChunkIDRe.FindStringSubmatchIndex(line[i:]); m != nil && m[0] == 0 {
			if n, ok := index[line[i+m[2]:i+m[3]]]; ok {
				writeMarker(&b, n)
				i += m[1]
				continue
			}
		}
		b.WriteByte(line[i])
		i++
	}
	return b.String()
}

func writeMarker(b *strings.Builder, n int) {
	b.WriteString("[ID:")
	b.WriteString(strconv.Itoa(n))
	b.WriteString("]")
}

// tupleMark returns the payload position a provenance tuple cites, when it resolves: a
// corpus tuple by its chunk_id, a web tuple by its url.
func tupleMark(group string, index map[string]int) (int, bool) {
	if m := citedChunkIDRe.FindStringSubmatch(group); m != nil {
		n, ok := index[m[1]]
		return n, ok
	}
	if !strings.Contains(group, "source: web") {
		return 0, false
	}
	m := citedWebURLRe.FindStringSubmatch(group)
	if m == nil {
		return 0, false
	}
	n, ok := index[m[1]]
	return n, ok
}

// CitedWebSource is a web passage the answer cites, exactly as the model declared it. The
// turn keeps no record of what a search served, so a web citation is the model's own
// claim: its url, title and snippet all come from the answer's provenance tuple.
type CitedWebSource struct {
	URL     string
	Title   string
	Snippet string
}

var (
	// citedWebURLRe matches the `url:` field of a web provenance tuple; the group is only
	// treated as web when it also says `source: web` (see tupleMark / ExtractCitedWebSources).
	citedWebURLRe = regexp.MustCompile(`url:\s*([^\s,)]+)`)
	// citedWebTitleRe and citedWebSnippetRe carry the other two fields the client shows.
	citedWebTitleRe   = regexp.MustCompile(`title:\s*([^,)]+)`)
	citedWebSnippetRe = regexp.MustCompile(`snippet:\s*"([^"]*)"`)
)

// ExtractCitedWebSources returns the web passages the answer cites, in first-appearance
// order, deduplicated by url.
func ExtractCitedWebSources(final string) []CitedWebSource {
	if final == "" || !strings.Contains(final, "source: web") {
		return nil
	}
	var out []CitedWebSource
	seen := make(map[string]bool)
	for i := 0; i < len(final); {
		if final[i] != '(' {
			i++
			continue
		}
		end, ok := tupleEnd(final, i)
		if !ok {
			i++
			continue
		}
		group := final[i : end+1]
		i = end + 1
		if !strings.Contains(group, "source: web") {
			continue
		}
		u := citedWebURLRe.FindStringSubmatch(group)
		if u == nil || seen[u[1]] {
			continue
		}
		seen[u[1]] = true
		src := CitedWebSource{URL: u[1]}
		if t := citedWebTitleRe.FindStringSubmatch(group); t != nil {
			src.Title = strings.TrimSpace(t[1])
		}
		if s := citedWebSnippetRe.FindStringSubmatch(group); s != nil {
			src.Snippet = s[1]
		}
		out = append(out, src)
	}
	return out
}

// tupleEnd returns the index of the ')' closing the '(' at start, ignoring parentheses
// inside a double-quoted span (a snippet routinely contains them). ok is false when the
// group is never closed, in which case the caller must not treat it as a tuple.
func tupleEnd(s string, start int) (int, bool) {
	inQuote := false
	for i := start + 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if inQuote {
				i++ // an escaped char inside the snippet cannot close the quote
			}
		case '"':
			inQuote = !inQuote
		case ')':
			if !inQuote {
				return i, true
			}
		}
	}
	return 0, false
}

//
// The tool results the model reads carry a `ref="n"` attribute on every passage
// (see EvidenceRegistry), and the prompt asks it to end each corpus-backed line
// with that handle: `[ID:n]`. The number names the passage the model was SHOWN,
// so it is checkable against what actually reached the context — unlike a chunk
// id the model could invent. The two helpers below turn those handles into the
// reference list the client resolves.

// citedRefRE matches the canonical `[ID:n]` handle and the bare `[n]` form
// accepted by the agentic citation contract. Bare numeric markers are bounded
// by the evidence registry before they can resolve, so ordinary bracketed
// numbers such as years remain untouched when they do not name a passage.
var citedRefRE = regexp.MustCompile(`(?i)\[(?:id\s*:\s*)?(\d+)\]`)

// commaCitationRE matches a handle group the model compressed into ONE marker
// ("[ID:47, ID:115]") instead of writing one marker per passage.
var commaCitationRE = regexp.MustCompile(`\[\s*ID\s*:\s*[0-9]+(?:\s*,\s*ID\s*:\s*[0-9]+)+\s*\]`)

var handleNumberRE = regexp.MustCompile(`[0-9]+`)

// ExpandCommaCitations splits a merged handle marker into one canonical marker
// per passage, so both this package's scanner and the client's citation pass
// can resolve it.
//
// A grouped marker matches neither `\[ID:(\d+)\]` nor the client's marker
// regex — both stop at the comma — so it survives every citation pass and
// reaches the UI as literal text instead of a citation. A handle naming a
// passage the registry never published (n >= maxIndex) is dropped rather than
// left pointing out of range. maxIndex <= 0 means the caller has no registry
// to bound against, and the text is left untouched.
func ExpandCommaCitations(answer string, maxIndex int) string {
	if answer == "" || maxIndex <= 0 || !strings.Contains(answer, ",") {
		return answer
	}
	return commaCitationRE.ReplaceAllStringFunc(answer, func(m string) string {
		var b strings.Builder
		for _, num := range handleNumberRE.FindAllString(m, -1) {
			n, err := strconv.Atoi(num)
			if err != nil || n < 0 || n >= maxIndex {
				continue
			}
			b.WriteString("[ID:")
			b.WriteString(strconv.Itoa(n))
			b.WriteString("]")
		}
		return b.String()
	})
}

// CitedIDsFromMarkers maps the answer's [ID:n] handles back to chunk ids, in
// first-citation order, deduplicated. A handle naming a passage the registry
// never published is ignored. Returns nil when the answer carries no handle
// (or no registry is available), which is the caller's signal to fall back to
// the raw `chunk_id:` path.
func CitedIDsFromMarkers(answer string, registry *EvidenceRegistry) []string {
	if answer == "" || registry == nil {
		return nil
	}
	ids := registry.IDs()
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, m := range citedRefRE.FindAllStringSubmatch(answer, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 0 || n >= len(ids) {
			continue
		}
		id := ids[n]
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// ResolveCitations rewrites the answer's [ID:n] handles so they index the
// reference list the caller publishes, and returns the cited ids in that order.
//
// publishable is the id list (in order) the caller will ship as
// reference.chunks — already narrowed to the passages that resolved. The first
// passage the answer cites becomes [ID:0], the next new one [ID:1], and so on;
// a handle naming a passage absent from publishable is dropped rather than left
// pointing at a wrong index. This is the mechanical compaction that lets a run
// stamp run-level handles ("[ID:265]") while the answer ships "[ID:0]".
func ResolveCitations(answer string, registry *EvidenceRegistry, publishable []string) (string, []string) {
	if answer == "" || registry == nil || len(publishable) == 0 {
		return answer, nil
	}
	ids := registry.IDs()
	pos := make(map[string]int, len(publishable))
	for i, id := range publishable {
		if _, dup := pos[id]; !dup {
			pos[id] = i
		}
	}
	seen := make(map[int]struct{})
	var cited []string
	out := citedRefRE.ReplaceAllStringFunc(answer, func(m string) string {
		sub := citedRefRE.FindStringSubmatch(m)
		n, err := strconv.Atoi(sub[1])
		if err != nil || n < 0 || n >= len(ids) {
			if strings.Contains(strings.ToLower(m), "id") {
				return ""
			}
			return m
		}
		k, ok := pos[ids[n]]
		if !ok {
			if strings.Contains(strings.ToLower(m), "id") {
				return ""
			}
			return m
		}
		if _, dup := seen[k]; !dup {
			seen[k] = struct{}{}
			cited = append(cited, ids[n])
		}
		return fmt.Sprintf("[ID:%d]", k)
	})
	return out, cited
}
