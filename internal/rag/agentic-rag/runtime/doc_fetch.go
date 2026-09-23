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

package runtime

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"ragflow/internal/common"
	"ragflow/internal/rag/prompts"
	"ragflow/internal/tokenizer"
)

const (
	// docFetchPageSize is the 128-chunk page size.
	docFetchPageSize = 128
	// docHeadScanChunks is how many chunks of a document's FIRST page HeadChunksOfDocument
	// inspects before it picks the opening. Reading order is chunk_order_int, and a page also
	// carries rows without one (compiled tree / navigation nodes), so the smallest-ordered rows
	// are picked from a page wide enough to hold the document's own opening.
	docHeadScanChunks = 40
	// docFetchMaxChunks is the hard 10000-chunk cap.
	docFetchMaxChunks = 10000
	// docFetchFallbackTokens bounds the fetch when the caller supplies no model
	// window. Callers usually pass one, so this only guards a caller that does not.
	docFetchFallbackTokens = 8192
	// estimateCharsPerToken approximates the tokenizer when no model encoder is
	// loaded; estimateTokens falls back to it only when tokenizer.NumTokensFromString
	// returns 0 (encoder unavailable).
	estimateCharsPerToken = 4
)

// documentReadAllowed is THE guard every document read passes: a session may read a document
// only if it is inside the bound datasets, and inside the session's document scope when one is
// set. fetchFullDocument and the paging reader both call it, so adding paging cannot widen what
// a session is allowed to see.
func documentReadAllowed(ctx context.Context, deps SearchDeps, docID string) bool {
	if deps.DocChunks == nil || docID == "" || len(deps.KbIDs) == 0 {
		common.Warn("fetch full document: skipped", zap.String("doc_id", docID), zap.Int("datasets", len(deps.KbIDs)))
		return false
	}
	// a session-wide document scope is authoritative.
	if len(deps.DocScope) > 0 && !containsStr(deps.DocScope, docID) {
		common.Warn("fetch full document: doc_id is outside the session document scope", zap.String("doc_id", docID))
		return false
	}
	// never read a document that is not in the bound datasets.
	if belongs, verified := docInDatasets(ctx, deps, docID); verified && !belongs {
		common.Warn("fetch full document: doc_id is not in any bound dataset, refusing to fetch", zap.String("doc_id", docID))
		return false
	}
	return true
}

// fetchDocumentPage reads ONE page of a document for the PAGING reader (list_chunks).
//
// Paging is what makes a document finitely readable: the document a session can read is
// bounded by the model window, so a long one used to be readable only up to that window, with
// nothing telling the model that it had stopped early or how to go on. The page request asks
// for want+1 chunks because the DocChunkLister contract answers both questions in one call —
// "a short page means the document is exhausted" — so no count query is needed.
//
// Returns the page and whether the document has more chunks after it.
func fetchDocumentPage(ctx context.Context, deps SearchDeps, docID string, offset, want int) ([]map[string]any, bool) {
	if want <= 0 {
		want = docFetchPageSize
	}
	if offset < 0 {
		offset = 0
	}
	if !documentReadAllowed(ctx, deps, docID) {
		return nil, false
	}
	page, err := deps.DocChunks.DocChunks(ctx, DocChunksRequest{
		DocID:      docID,
		DatasetIDs: deps.KbIDs,
		TenantID:   deps.TenantID,
		Offset:     offset,
		Limit:      want + 1,
	})
	if err != nil {
		common.Warn("fetch document page: page failed", zap.Int("offset", offset), zap.Error(err))
		return nil, false
	}
	if len(page) <= want {
		return page, false
	}
	return page[:want], true
}

// HeadChunksOfDocument returns the FIRST `want` chunks of one document, in READING order.
//
// It is the reading door into a document the caller ALREADY chose — for a member's own document
// named by title (see the entity-title channel) this is how its head is read, because the member's
// fact is a table row there and no ranking reaches it: measured 2026-09-22 on the ten-nominee Oscar
// question, "Children: 3" sits in chunk_order_int 0 of the Brendan Fraser document, and the chunk's
// searchable tokens (content_ltks) carry NO "children" at all — the table's cell text is not in any
// indexed field, so a metadata-scoped hybrid leg can rank that document's references and prose
// forever and never return its infobox.
//
// Reading is bounded by the caller's `want` and by the session's own document guard (unbound
// dataset, or a document outside the session scope, yields nil).
func HeadChunksOfDocument(ctx context.Context, deps SearchDeps, docID string, want int) []map[string]any {
	if want <= 0 {
		want = 1
	}
	// The opening is picked by READING ORDER, and by the fact that a document's TABLE is the part
	// a question asks about: an infobox or a standings table carries the fields, while the prose
	// around it carries the narrative. Neither signal alone is enough —
	//
	//   - rows with no chunk_order_int at all lead the page (the compiled tree/navigation nodes a
	//     parser writes), and without a table-first rule the reader returns one of them;
	//   - a re-parsed copy of the SAME document can carry no order at all (measured 2026-09-22:
	//     one person, two documents — one ordered with its infobox at order 0, one unordered with
	//     its infobox somewhere in the middle), so order alone picks "whatever came first", which
	//     was prose.
	//
	// So the table-bearing chunks lead, and the reading order orders them (and everything else)
	// within its group.
	page, _ := fetchDocumentPage(ctx, deps, docID, 0, docHeadScanChunks)
	if len(page) == 0 {
		return nil
	}
	tables := make([]map[string]any, 0, len(page))
	text := make([]map[string]any, 0, len(page))
	for _, c := range page {
		if containsHTMLTable(c) {
			tables = append(tables, c)
			continue
		}
		text = append(text, c)
	}
	sortChunksByReadingOrder(tables)
	sortChunksByReadingOrder(text)
	page = append(tables, text...)
	if len(page) > want {
		page = page[:want]
	}
	return page
}

// containsHTMLTable reports whether a chunk's text carries an HTML table — the shape a
// document's facts are written in (see renderTables). Narrower than isTableChunk, which also
// accepts pipe-row tables.
func containsHTMLTable(c map[string]any) bool {
	return strings.Contains(strings.ToLower(ChunkTextOf(c)), "<table")
}

// sortChunksByReadingOrder sorts in place by chunk_order_int, with the rows that carry none last
// and their own page order otherwise intact.
func sortChunksByReadingOrder(chunks []map[string]any) {
	sort.SliceStable(chunks, func(i, j int) bool {
		a, aOK := chunkOrderOf(chunks[i])
		b, bOK := chunkOrderOf(chunks[j])
		switch {
		case aOK && bOK:
			return a < b
		case aOK:
			return true
		case bOK:
			return false
		}
		return false
	})
}

// chunkOrderOf reads a chunk's 0-based reading-order index within its document
// (the engine's chunk_order_int). ok=false means the row carries none — a compiled
// tree/navigation row rather than a piece of the document's text.
func chunkOrderOf(c map[string]any) (int, bool) {
	v, ok := c["chunk_order_int"]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	text := strings.TrimSpace(fmt.Sprint(v))
	if text == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(text); err == nil {
		return n, true
	}
	return 0, false
}

// fetchFullDocument
// (agentic_rag.py:fetch_full_document): read a document end-to-end in reading order, in pages,
// stopping before the model window would overflow.
//
// Returns (chunks, docAggs). Both are nil when the document is unavailable —
// unbound datasets, a document outside the session scope, or an empty read.
func fetchFullDocument(ctx context.Context, deps SearchDeps, docID string, maxTokens int) ([]map[string]any, []map[string]any) {
	if !documentReadAllowed(ctx, deps, docID) {
		return nil, nil
	}

	budget := maxTokens
	if budget <= 0 {
		budget = docFetchFallbackTokens
	}

	var chunks []map[string]any
	tokens := 0
	budgetHit := false
	// NOTE: the budget check breaks the OUTER loop, so a page
	// that overruns the window stops paging entirely. Kept as-is: the budget is
	// a hard stop, and continuing would only add chunks that get dropped.
	for offset := 0; offset < docFetchMaxChunks && !budgetHit; offset += docFetchPageSize {
		page, err := deps.DocChunks.DocChunks(ctx, DocChunksRequest{
			DocID:      docID,
			DatasetIDs: deps.KbIDs,
			TenantID:   deps.TenantID,
			Offset:     offset,
			Limit:      docFetchPageSize,
		})
		if err != nil {
			common.Warn("fetch full document: page failed", zap.Int("offset", offset), zap.Error(err))
			break
		}
		if len(page) == 0 {
			break
		}
		for _, ck := range page {
			n := estimateTokens(ChunkTextOf(ck))
			if tokens+n > budget {
				budgetHit = true
				break
			}
			tokens += n
			chunks = append(chunks, ck)
		}
		if len(page) < docFetchPageSize {
			break // document exhausted
		}
	}
	if len(chunks) == 0 {
		common.Info("fetch full document: no chunks", zap.String("doc_id", docID))
		return nil, nil
	}

	docName := ""
	for _, c := range chunks {
		if t := DocTitleOf(c); t != "" {
			docName = t
			break
		}
	}
	aggs := []map[string]any{
		{"doc_name": docName, "doc_id": docID, "count": len(chunks)},
	}
	return chunks, aggs
}

// summarizeDocument
// (agentic_rag.py:rag): load the whole document, fold it into the evidence the
// citation rules refer to, and return the newly rendered blocks.
func summarizeDocument(ctx context.Context, deps SearchDeps, docID string, maxTokens int) []string {
	chunks, aggs := fetchFullDocument(ctx, deps, docID, maxTokens)
	if len(chunks) == 0 {
		return nil
	}
	budget := maxTokens
	if budget <= 0 {
		budget = docFetchFallbackTokens
	}

	// the document becomes part of the evidence set, so its
	// [ID]s stay citable; only the blocks of the chunks read here are returned.
	//
	// The blocks are picked by the SOURCE CHUNK, not by the pre-merge chunk
	// count: KBPrompt skips a chunk with no content and stops when the token
	// budget is exhausted, so block N is not chunk N — a slice taken at the
	// pre-merge count can drop readable blocks, or point past the end and return
	// nil for a document that was read fine — and this is easy to hit, because Merge
	// deduplicates a document
	// chunk that is already pooled, so the pre-merge count can even equal the
	// post-merge count). Merge reports the pool position of every chunk this
	// fetch contributed — deduplicated ones included — which makes the mapping
	// exact; iterating it keeps the document's reading order.
	if deps.KB == nil {
		deps.KB = &Kbinfos{}
	}
	added := deps.KB.Merge(chunks, aggs)
	// Pool positions, not rendered positions, are the block ids — 0-based, like every
	// other evidence render. These are the tool's evidence, not the final answer's:
	// the compose re-renders the pool it wants to cite with its own numbering
	// (CiteChunkIDs) and the chat pipeline resolves the answer's markers against that
	// list. What the ids must do here is stay addressable, which a pool position does
	// whatever the render skipped.
	blocks, sources := prompts.KBPromptPoolIndexed(deps.KB.Chunks, budget)
	blockAt := make(map[int]string, len(sources))
	for i, src := range sources {
		if _, seen := blockAt[src]; !seen {
			blockAt[src] = blocks[i]
		}
	}
	// One block per POOL POSITION. `added` is positional per OCCURRENCE: a chunk
	// the reader served twice (offset paging over an unstable order) or a chunk
	// already pooled is reported at the same position again, so appending per
	// occurrence would hand the model the same block — and its tokens — twice.
	fresh := make([]string, 0, len(added))
	seenPositions := make(map[int]struct{}, len(added))
	for _, pos := range added {
		if _, seen := seenPositions[pos]; seen {
			continue
		}
		seenPositions[pos] = struct{}{}
		if block, ok := blockAt[pos]; ok {
			fresh = append(fresh, block)
		}
	}
	// Nothing of this document survived the budget: the pool is already at the
	// model window, so there is no block to hand back.
	if len(fresh) == 0 {
		return nil
	}

	// without do_refer the model is told not to cite, so the
	// rules must not be handed to it.
	if !deps.DoRefer {
		return fresh
	}
	// The blocks are numbered by pool position: were this path ever reached with
	// do_refer=true, the rules would need the same 0-based sentence the compose adds
	// (agentic_rag.zeroBasedEvidenceRule) — CitationPrompt itself cannot carry it,
	// the canvas renders hash ids.
	header := "# Citation rules\nApply the following rules VERBATIM to your final answer.\n\n" +
		prompts.CitationPrompt(deps.CiteRules) + "\n\n----\n\n"
	return append([]string{header}, fresh...)
}

// SummarizeDocument is the exported summarize_document tool used by the outer react loop
// (rag_agent) as a non-terminal tool. It reads the whole document identified by docID into the
// evidence set and returns the prompt blocks to feed back to the model. do_refer is taken from
// deps.DoRefer (the run config decides whether the model may cite the freshly read document),
// so callers control citation behavior.
// Returns nil when the document has no readable chunks (deps.DocChunks unset
// or the doc is unavailable).
func SummarizeDocument(ctx context.Context, deps SearchDeps, docID string, maxTokens int) []string {
	if deps.DocChunks == nil {
		return nil
	}
	return summarizeDocument(ctx, deps, docID, maxTokens)
}

// estimateTokens returns the token count of s. It prefers the precise
// tokenizer (tokenizer.NumTokensFromString); when no encoder is loaded that returns 0, so we
// fall back to a character/estimateCharsPerToken approximation for a rough
// budget, like the previous Go-only behavior.
func estimateTokens(s string) int {
	if n := tokenizer.NumTokensFromString(s); n > 0 {
		return n
	}
	return (len([]rune(s)) + estimateCharsPerToken - 1) / estimateCharsPerToken
}
