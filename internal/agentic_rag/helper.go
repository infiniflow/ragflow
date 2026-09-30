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
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"

	"go.uber.org/zap"
)

// toolErrorMarker opens the failure notice a tool returns when its
// invocation failed. The model reads the notice (with the root cause and
// severity) and adapts — fix the arguments, switch tools, or finish
// gracefully — while consumeAgentEvents sniffs the same marker to tally
// per-tool failures into the benchmark's run stats.
const toolErrorMarker = "<tool_error"

// toolErrorNotice formats a tool failure as a model-readable result.
func toolErrorNotice(tool string, err error) string {
	msg := err.Error()
	if r := []rune(msg); len(r) > 400 {
		msg = string(r[:400]) + "…"
	}
	return toolErrorXML(tool, "error", msg)
}

// toolErrorXML renders the CANONICAL failure element every tool uses:
//
//	<tool_error tool="NAME" severity="error|warn" key="value">reason</tool_error>
//
// The model reads it and adapts (fix the arguments, switch tools, stop
// retrying a dead backend); consumeAgentEvents tallies the severity="error"
// ones into the benchmark's per-tool failure accounting. severity="warn"
// marks a partial failure the tool already recovered from (e.g. one query
// of several died) — visible, but not an outage. Extra attributes (query,
// failed_queries, ...) carry the failure's scope. The reason is
// XML-escaped; the caller truncates it to keep the result bounded.
func toolErrorXML(tool, severity, msg string, attrs ...[2]string) string {
	var b strings.Builder
	b.WriteString(toolErrorMarker)
	b.WriteString(` tool="`)
	b.WriteString(xmlEscape(tool))
	b.WriteString(`" severity="`)
	b.WriteString(xmlEscape(severity))
	b.WriteString(`"`)
	for _, a := range attrs {
		b.WriteString(` `)
		b.WriteString(a[0])
		b.WriteString(`="`)
		b.WriteString(xmlEscape(a[1]))
		b.WriteString(`"`)
	}
	b.WriteString(">")
	b.WriteString(xmlEscape(msg))
	b.WriteString("</tool_error>")
	return b.String()
}

// guardedToolRun wraps a tool's invokable run. A Go error returned from a
// tool does not reach the model as a result — it aborts the whole ReAct
// loop, so one bad regex or a backend outage ends the turn with nothing
// and the model never learns why. Failures are converted into a
// model-readable <tool_error> result (the loop continues) while the
// operator still gets a warn-level log line with the full error.
func guardedToolRun(ctx context.Context, tool string, run func(context.Context, string) (string, error), args string) (string, error) {
	res, err := run(ctx, args)
	if err == nil {
		return res, nil
	}
	common.WarnCtx(ctx, "agentic_rag: tool failed - converting to model-readable result",
		zap.String("tool", tool), zap.Error(err))
	return toolErrorNotice(tool, err), nil
}

// resolveDatasetScope keeps model-provided dataset ids within the
// conversation's server-bound scope. An omitted request uses the full bound
// scope; an explicit request must be a subset of it.
func resolveDatasetScope(bound, requested []string) ([]string, error) {
	if len(requested) == 0 {
		return bound, nil
	}

	allowed := make(map[string]struct{}, len(bound))
	for _, id := range bound {
		allowed[id] = struct{}{}
	}
	for _, id := range requested {
		if _, ok := allowed[id]; !ok {
			return nil, fmt.Errorf("dataset_id %q is outside the conversation's bound scope", id)
		}
	}
	return requested, nil
}

// clampFloat01 clamps a float into [0, 1], used for similarity weights the model
// may supply out of range.
func clampFloat01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// grepChunksSelectFields is the ES _source fields the retrieval tools request.
// doc_id, page_num_int and chunk_order_int are the mandated reading-order
// identifiers; content_with_weight and the other fields feed the tool output.
// The regexp query matches content_with_weight, so it must stay in the list.
var grepChunksSelectFields = []string{
	"content_with_weight",
	"doc_id",
	"docnm_kwd",
	"kb_id",
	"page_num_int",
	"chunk_order_int",
}

// grepChunksSortFields is the reading-order sort used by grep_chunks and
// list_chunks: by document, then page, then chunk within a page.
var grepChunksSortFields = []string{"doc_id", "page_num_int", "chunk_order_int"}

// readingOrderLess orders chunks by document, then page, then chunk index —
// matching the doc_id / page_num_int / chunk_order_int sort the engine applies.
func readingOrderLess(a, b runtime.RetrievalChunk) bool {
	if a.DocumentID != b.DocumentID {
		return a.DocumentID < b.DocumentID
	}
	if a.PageNum != b.PageNum {
		return a.PageNum < b.PageNum
	}
	return a.ChunkIndex < b.ChunkIndex
}

// Snippet shaping, shared by every locate tool — grep_chunks,
// search_bm25_chunks, search_chunks. One set of knobs governs all three.
const (
	// snippetWholeChunkRunes is the size under which a chunk ships WHOLE. A
	// fixed byte window on a short chunk buys nothing and is exactly how a
	// hit's answer-bearing sentence fell outside the snippet: the model saw the
	// keyword and never the paragraph around it.
	snippetWholeChunkRunes = 600
	// snippetSpanWholeChunkRunes is the same idea for scattered hits: when the
	// outermost matches sit further apart than the snippet budget, shipping the
	// whole chunk (up to this size) beats cutting one of them out.
	snippetSpanWholeChunkRunes = 3000
	// snippetMaxRunes caps one snippet. Beyond it the fragment is a window and
	// is marked truncated so the model knows to deep-read.
	snippetMaxRunes = 1200
	// snippetMinContextRunes is the least context each side of the match span
	// gets before boundary alignment — the hit is never the first or last thing
	// in the fragment.
	snippetMinContextRunes = 240
	// snippetAlignTolerance is how far past snippetMaxRunes alignment may reach
	// to land on a sentence end. A hard cap that cuts mid-sentence is worse
	// than a few dozen extra runes: the cut sentence is exactly where the answer
	// tends to sit.
	snippetAlignTolerance = 240
	// snippetWholeCollapseRatio is the coverage above which a window is
	// pointless: trimming 5% off a chunk and adding ellipses only creates doubt.
	snippetWholeCollapseRatio = 0.9
)

// collapseSpaces normalises a chunk body to single-line form so snippets never
// carry raw newlines regardless of the source formatting.
func collapseSpaces(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

// previewForChunk renders a chunk that matched NO literal query term — the
// normal case for a lexical search (BM25 indexes stemmed word forms, so the
// surface term can be absent from the text it scored). Shipping an empty
// snippet left the model holding a chunk_id and no text, which is how a
// document ends up "surfaced but never read": give it the chunk's own opening
// (or the whole body when short) and mark it as a preview, not a keyword window.
func previewForChunk(content string) (string, bool) {
	if content == "" {
		return "", false
	}
	runes := []rune(content)
	if len(runes) <= snippetWholeChunkRunes {
		return collapseSpaces(content), false
	}
	return renderSnippetFragment(runes, 0, snippetMaxRunes), true
}

// snippetForMatches renders the fragment of content that CONTAINS the match
// span [firstByte,lastByte), extended to natural boundaries. It returns the
// fragment and whether anything was left out.
//
// The match span is always inside the returned fragment, so a keyword can never
// be cut out. The rules, in order:
//
//  1. A chunk that fits in snippetWholeChunkRunes ships WHOLE, no ellipsis.
//  2. Scattered hits whose span exceeds the snippet budget ship the whole chunk
//     too, as long as it stays under snippetSpanWholeChunkRunes (a narrower
//     fragment would have to drop a hit).
//  3. Otherwise the span is widened to the enclosing PARAGRAPH (blank-line
//     separated — the corpus is markdown) when that fits in snippetMaxRunes:
//     the complete paragraph around the keyword, which is what the model needs
//     to answer rather than the keyword alone.
//  4. Otherwise it is widened to sentence boundaries within the budget, with at
//     least snippetMinContextRunes on each side.
//
// Reason for the whole dance: a fixed byte window ends mid-sentence and hides
// the sentence right after the keyword — observed as "the document surfaced as
// a snippet, but the paragraph carrying the answer was never read".
func snippetForMatches(content string, firstByte, lastByte int) (string, bool) {
	if content == "" {
		return "", false
	}
	runes := []rune(content)
	if firstByte < 0 {
		firstByte = 0
	}
	if lastByte > len(content) {
		lastByte = len(content)
	}
	if firstByte > lastByte {
		firstByte, lastByte = lastByte, firstByte
	}
	start := utf8.RuneCountInString(content[:firstByte])
	end := utf8.RuneCountInString(content[:lastByte])
	if start >= len(runes) {
		return "", false
	}
	if end > len(runes) {
		end = len(runes)
	}
	if end <= start {
		end = start + 1
	}

	if len(runes) <= snippetWholeChunkRunes {
		return collapseSpaces(content), false
	}
	if end-start >= snippetMaxRunes {
		// The hits themselves are further apart than the budget. Shipping the
		// whole chunk keeps every keyword (rule 2); past that size the fragment
		// keeps the FIRST cluster and the truncation note sends the model to the
		// full chunk — a window that dropped both clusters would be worse.
		if len(runes) <= snippetSpanWholeChunkRunes {
			return collapseSpaces(content), false
		}
		return renderSnippetFragment(runes, start, start+snippetMaxRunes), true
	}

	// Paragraph first: the complete semantic unit around the hit.
	pStart, pEnd := paragraphBounds(runes, start, end)
	if pEnd-pStart <= snippetMaxRunes {
		return renderSnippetFragment(runes, pStart, pEnd), pStart > 0 || pEnd < len(runes)
	}

	// Align each edge, but never let alignment run away with the budget: a
	// sentence-terminator-free tail would otherwise swallow a thousand runes.
	wStart := snapToWordStart(runes, start-snippetMinContextRunes)
	if head := alignForwardToSentenceHead(runes, wStart); head <= start {
		wStart = head
	} else if prev := alignToSentenceStart(runes, wStart); prev < wStart {
		wStart = prev
	}
	wEnd := end + snippetMinContextRunes
	if wEnd > len(runes) {
		wEnd = len(runes)
	}
	if aligned := alignToSentenceEnd(runes, wEnd); aligned-wStart <= snippetMaxRunes+snippetAlignTolerance {
		wEnd = aligned
	}
	if wEnd-wStart > snippetMaxRunes {
		// The match span must survive: pull the start forward to fit the budget
		// (never past the first hit), preferring a sentence head.
		target := wEnd - snippetMaxRunes
		if target < start {
			target = start
		}
		if target > wStart {
			wStart = snapToWordStart(runes, target)
		}
		if head := alignForwardToSentenceHead(runes, wStart); head <= start {
			wStart = head
		}
	}
	// A window that has grown to cover nearly the whole chunk IS the chunk:
	// shipping it whole beats shipping 95% plus two ellipses.
	if float64(wEnd-wStart) >= snippetWholeCollapseRatio*float64(len(runes)) {
		return collapseSpaces(content), false
	}
	return renderSnippetFragment(runes, wStart, wEnd), wStart > 0 || wEnd < len(runes)
}

// renderSnippetFragment renders runes[start:end] with "..." marking truncation
// at either end.
func renderSnippetFragment(runes []rune, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	if start >= end {
		return ""
	}
	prefix, suffix := "", ""
	if start > 0 {
		prefix = "..."
	}
	if end < len(runes) {
		suffix = "..."
	}
	return prefix + collapseSpaces(string(runes[start:end])) + suffix
}

// paragraphBounds widens [start,end) to the blank-line separated block around
// it — the corpus is markdown, so a blank line is a paragraph break.
func paragraphBounds(runes []rune, start, end int) (int, int) {
	pStart := start
	for pStart > 0 {
		if runes[pStart-1] == '\n' && (pStart < 2 || runes[pStart-2] == '\n') {
			break
		}
		pStart--
	}
	pEnd := end
	for pEnd < len(runes) {
		if runes[pEnd] == '\n' && pEnd+1 < len(runes) && runes[pEnd+1] == '\n' {
			break
		}
		pEnd++
	}
	return pStart, pEnd
}

// alignToSentenceStart walks back to the nearest sentence head so the fragment
// never opens mid-sentence.
func alignToSentenceStart(runes []rune, candidate int) int {
	for s := candidate; s > 0; s-- {
		if isSentenceTerminator(runes[s-1]) {
			return s
		}
	}
	return 0
}

// alignToSentenceEnd walks forward to the nearest sentence end so the fragment
// never closes mid-sentence.
func alignToSentenceEnd(runes []rune, candidate int) int {
	for e := candidate; e < len(runes); e++ {
		if isSentenceTerminator(runes[e]) {
			return e + 1
		}
	}
	return len(runes)
}

// snapToWordStart moves candidate forward to the next word boundary, so a
// fragment never opens in the middle of a word (which reads as corruption).
func snapToWordStart(runes []rune, candidate int) int {
	if candidate <= 0 {
		return 0
	}
	if candidate >= len(runes) {
		return len(runes)
	}
	if unicode.IsSpace(runes[candidate-1]) {
		return candidate
	}
	for s := candidate; s < len(runes); s++ {
		if unicode.IsSpace(runes[s]) {
			for s < len(runes) && unicode.IsSpace(runes[s]) {
				s++
			}
			return s
		}
	}
	return candidate
}

// alignForwardToSentenceHead returns the first sentence head at or after
// candidate — the shrink direction, used to bring an over-budget window's start
// back inside the budget without opening mid-sentence.
func alignForwardToSentenceHead(runes []rune, candidate int) int {
	for s := candidate; s < len(runes); s++ {
		if s > 0 && isSentenceTerminator(runes[s-1]) {
			return s
		}
	}
	return candidate
}

func isSentenceTerminator(r rune) bool {
	switch r {
	case '.', '!', '?', ';', '\n', '。', '！', '？', '；':
		return true
	}
	return false
}

// regexMatchSpan returns the earliest match start / latest match end byte
// offsets of re over ALL occurrences in content.
func regexMatchSpan(re *regexp.Regexp, content string) (int, int, bool) {
	locs := re.FindAllStringIndex(content, -1)
	if len(locs) == 0 {
		return 0, 0, false
	}
	first, last := locs[0][0], locs[0][1]
	for _, l := range locs[1:] {
		if l[0] < first {
			first = l[0]
		}
		if l[1] > last {
			last = l[1]
		}
	}
	return first, last, true
}

// termMatchSpan is regexMatchSpan's literal-term counterpart: over every
// case-insensitive occurrence of every term it takes the earliest start and the
// latest end, so multi-keyword queries centre the window on the whole hit set.
func termMatchSpan(terms []string, content string) (int, int, bool) {
	lower := strings.ToLower(content)
	first, last := -1, -1
	for _, t := range terms {
		if t == "" {
			continue
		}
		from := 0
		for {
			i := strings.Index(lower[from:], t)
			if i < 0 {
				break
			}
			lo, hi := from+i, from+i+len(t)
			if first < 0 || lo < first {
				first = lo
			}
			if hi > last {
				last = hi
			}
			from = hi
		}
	}
	if first < 0 {
		return 0, 0, false
	}
	return first, last, true
}

// snippetHit couples a located chunk with its pre-rendered snippet line.
type snippetHit struct {
	chunk   runtime.RetrievalChunk
	snippet string
	// truncated marks a snippet that shows only part of its chunk: the keyword's
	// own paragraph, not the whole body. The XML says so explicitly, and the
	// results carry a hint pointing at list_chunks for the rest — otherwise the
	// model quotes a window as if it were the document.
	truncated bool
	// preview marks a chunk that surfaced WITHOUT a literal term match (BM25
	// scores stemmed forms): the snippet is the chunk's own opening, not the
	// keyword's neighbourhood, and the model must not read it as evidence that
	// the query terms appear there.
	preview bool
}

// formatLocateResultsXML renders the UNIFIED payload shared by every locate
// tool: one compact vocabulary (<search_results> root with a query echo, chunk
// attributes incl. rank/score, one <match_snippet> per chunk). Roots differ
// from list_chunks' <chunks> by design: these are triage views, not deep reads.
func formatLocateResultsXML(ctx context.Context, tool, query string, hits []snippetHit) string {
	// Single serve point for every locate tool: one debug line per served chunk
	// records exactly what went in front of the model (silent unless the logger
	// runs at debug level).
	logServedChunks(ctx, tool, query, hits)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<search_results count=\"%d\" query=\"%s\">\n",
		len(hits), xmlEscape(query)))
	truncatedHits := 0
	previewHits := 0
	for i, h := range hits {
		c := h.chunk
		truncatedAttr := ""
		if h.truncated {
			truncatedAttr = ` truncated="true"`
			truncatedHits++
		}
		matchAttr := ""
		if h.preview {
			matchAttr = ` match="none"`
			previewHits++
		}
		b.WriteString(fmt.Sprintf(
			"<chunk rank=\"%d\" chunk_id=\"%s\" doc_id=\"%s\" page_num=\"%d\" chunk_index=\"%d\" dataset_id=\"%s\" doc_name=\"%s\" score=\"%.3f\" chunk_runes=\"%d\"%s%s>\n",
			i+1,
			xmlEscape(c.ID), xmlEscape(c.DocumentID), c.PageNum, c.ChunkIndex,
			xmlEscape(c.DatasetID), xmlEscape(c.DocumentName), c.Score,
			utf8.RuneCountInString(c.Content), truncatedAttr, matchAttr,
		))
		if h.snippet != "" {
			b.WriteString(fmt.Sprintf("<match_snippet>%s</match_snippet>\n", xmlEscape(h.snippet)))
		}
		b.WriteString("</chunk>\n")
	}
	if truncatedHits > 0 || previewHits > 0 {
		// The model must not quote a fragment as if it were the document.
		b.WriteString(fmt.Sprintf("<snippet_note>%d snippet(s) are TRUNCATED (truncated=\"true\") and %d chunk(s) "+
			"carry NO keyword match (match=\"none\" - a lexical hit whose surface term is absent, e.g. a stemmed form). "+
			"Read the chunk in full with list_chunks (chunk_id + number_neighbors=0) before relying on it: a fragment "+
			"shows the neighbourhood of the match, not everything the chunk says.</snippet_note>\n",
			truncatedHits, previewHits))
	}
	if len(hits) == 0 {
		// Zero hits usually means a WORD-FORM mismatch (the corpus says
		// "mineralization" while the query says "mineralizer"), not corpus
		// absence. Surface the retry discipline here so the model sees it at
		// the exact moment it decides what to query next.
		b.WriteString("<hint>0 hits — the corpus may use a different WORD FORM of your terms. Retry with: (1) derivational variants of the rarest term (mineralizer -> mineralization -> mineralize), singular/plural forms; (2) for grep_chunks, the stem plus a trailing wildcard (mineralizer -> mineraliz.*); (3) the single rarest term ALONE instead of a multi-word phrase (a proper noun, procedure name, or unique date). An exact-phrase 0-hit is not evidence that the corpus lacks the answer.</hint>\n")
		b.WriteString(zeroHitNextStep(tool))
	}
	b.WriteString("</search_results>")
	return b.String()
}

// lexicalLocateTools match the query's WORDS: grep_chunks by regex over chunk
// text, search_bm25_chunks by token ranking. A zero-hit result from one of them
// is evidence about the caller's vocabulary, not about the corpus.
var lexicalLocateTools = map[string]struct{}{
	"grep_chunks":        {},
	"search_bm25_chunks": {},
}

// zeroHitNextStep is the recovery step appended to a 0-hit locate result,
// chosen by which leg produced the zero. The distinction matters and is the
// whole point of the message:
//
//   - a LEXICAL tool that found nothing has proved that the corpus does not
//     state the thing in the caller's words — the one condition the
//     meaning-based leg exists for. Rewriting the same words (the natural next
//     move, and the one a benchmark run repeated 28 times on one question) adds
//     nothing, so the hint asks for a DESCRIPTION instead, and for a new
//     CATEGORY when the first description misses.
//   - a SEMANTIC tool that found nothing has a different problem: nothing in the
//     corpus MEANS that, so the description — typically the question's own
//     abstract wording ("business", "manufacturing", "award") — is what has to
//     change.
//
// Both branches name the next move in one sentence: the model reads this at the
// moment it decides what to query, which is where a rule stated once in the
// system prompt gets forgotten.
func zeroHitNextStep(tool string) string {
	if _, lexical := lexicalLocateTools[tool]; lexical {
		return "<hint_next>THIS IS THE LEXICAL DEAD-END SIGNAL, not a corpus gap: your words are not how the corpus states this. Do NOT rewrite the same terms a third time — run ONE search_semantic_chunks query that DESCRIBES the thing you are looking for in plain language (what it is and what is true about it), and if that comes back off-target, describe a DIFFERENT CATEGORY of thing (restaurant -> hotel -> winery -> manufacturer).</hint_next>\n"
	}
	return "<hint_next>A MEANING search returned nothing, so the problem is the DESCRIPTION rather than the vocabulary: nothing in the corpus means what you asked for. Drop the question's abstract words (\"business\", \"manufacturing\", \"an award\") and describe the concrete thing instead, or describe a different CATEGORY of thing (restaurant -> hotel -> winery -> manufacturer), then retry ONCE.</hint_next>\n"
}

// dedupStrings returns s in first-occurrence order without duplicates.
func dedupStrings(s []string) []string {
	seen := make(map[string]struct{}, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// formatChunksXML serialises the ordered chunks of a single document into the
// compact XML shape consumed by the model. anchorMeta (may be empty) appends
// anchored-read annotations to the root element. Terminology is uniform with
// the other retrieval tools: dataset_id (dataset), doc_id (document).
// formatChunksXML renders one deep-read result set. notice, when non-empty, is
// emitted as a <notice> element right after the opening tag: it carries
// findings that must NOT abort the caller's turn (e.g. anchors that did not
// resolve), so the model can see and judge them instead of losing the whole
// ReAct turn to an error.
func formatChunksXML(docID string, chunks []runtime.RetrievalChunk, anchorMeta, notice string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<chunks doc_id=\"%s\" fetched=\"%d\"%s>\n",
		xmlEscape(docID), len(chunks), anchorMeta))
	if notice != "" {
		b.WriteString(fmt.Sprintf("<notice>%s</notice>\n", xmlEscape(notice)))
	}
	for _, c := range chunks {
		b.WriteString(fmt.Sprintf(
			"<chunk chunk_id=\"%s\" doc_id=\"%s\" page_num=\"%d\" chunk_index=\"%d\" dataset_id=\"%s\" doc_name=\"%s\">\n",
			xmlEscape(c.ID), xmlEscape(c.DocumentID), c.PageNum, c.ChunkIndex,
			xmlEscape(c.DatasetID), xmlEscape(c.DocumentName)))
		if c.Content != "" {
			b.WriteString(fmt.Sprintf("<content>%s</content>\n", xmlEscape(c.Content)))
		}
		b.WriteString("</chunk>\n")
	}
	b.WriteString("</chunks>")
	return b.String()
}
