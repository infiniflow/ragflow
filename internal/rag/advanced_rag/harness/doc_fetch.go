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
	"context"

	"ragflow/internal/rag/prompts"
	"ragflow/internal/tokenizer"
)

const (
	// docFetchPageSize is Python's 128-chunk page.
	docFetchPageSize = 128
	// docFetchMaxChunks is Python's hard 10000-chunk cap (max_count > 10000).
	docFetchMaxChunks = 10000
	// docFetchFallbackTokens bounds the fetch when the caller supplies no model
	// window. Python always passes one, so this only guards a Go-only caller.
	docFetchFallbackTokens = 8192
	// estimateCharsPerToken approximates the tokenizer when no model encoder is
	// loaded; estimateTokens falls back to it only when tokenizer.NumTokensFromString
	// returns 0 (encoder unavailable). Python uses num_tokens_from_string directly.
	estimateCharsPerToken = 4
)

// fetchFullDocument mirrors Python RAGTools.fetch_full_document
// (agentic_rag.py:fetch_full_document): read a document end-to-end in reading order, in pages,
// stopping before the model window would overflow.
//
// Returns (chunks, docAggs). Both are nil when the document is unavailable —
// unbound datasets, a document outside the session scope, or an empty read.
func fetchFullDocument(ctx context.Context, deps SearchDeps, docID string, maxTokens int) ([]map[string]any, []map[string]any) {
	if deps.DocChunks == nil || docID == "" || len(deps.KbIDs) == 0 {
		_LOG.Printf("[Fetch full document] skipped (doc_id=%q, datasets=%d)", docID, len(deps.KbIDs))
		return nil, nil
	}
	// Python :765 — a session-wide document scope is authoritative.
	if len(deps.DocScope) > 0 && !containsStr(deps.DocScope, docID) {
		_LOG.Printf("[Fetch full document] doc_id %q is outside the session document scope", docID)
		return nil, nil
	}
	// Python :768 — never read a document that is not in the bound datasets.
	if belongs, verified := docInDatasets(ctx, deps, docID); verified && !belongs {
		_LOG.Printf("[Fetch full document] doc_id %q is not in any bound dataset — refusing to fetch", docID)
		return nil, nil
	}

	budget := maxTokens
	if budget <= 0 {
		budget = docFetchFallbackTokens
	}

	var chunks []map[string]any
	tokens := 0
	budgetHit := false
	// Python :769-776 — NOTE: the budget check breaks the OUTER loop, so a page
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
			_LOG.Printf("[Fetch full document] page at offset %d failed: %v", offset, err)
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
		_LOG.Printf("[Fetch full document] no chunks for doc_id %q", docID)
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

// summarizeDocument mirrors Python RAGTools.summarize_document
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

	// Python :942-948 — the document becomes part of the evidence set, so its
	// [ID]s stay citable; only the blocks added here are returned.
	startIdx := 0
	if deps.KB != nil {
		startIdx = len(deps.KB.Chunks)
		deps.KB.Merge(chunks, aggs)
	} else {
		deps.KB = &Kbinfos{}
		deps.KB.Merge(chunks, aggs)
	}
	blocks := prompts.KBPrompt(deps.KB.Chunks, budget)
	if startIdx >= len(blocks) {
		return nil
	}
	fresh := blocks[startIdx:]

	// Python :952-959 — without do_refer the model is told not to cite, so the
	// rules must not be handed to it.
	if !deps.DoRefer {
		return fresh
	}
	header := "# Citation rules\nApply the following rules VERBATIM to your final answer.\n\n" +
		prompts.CitationPrompt(deps.CiteRules) + "\n\n----\n\n"
	return append([]string{header}, fresh...)
}

// SummarizeDocument is the exported, Python-RAGTools.summarize_document
// equivalent used by the outer react loop (rag_agent) as a non-terminal tool.
// It reads the whole document identified by docID into the evidence set and
// returns the prompt blocks to feed back to the model. do_refer is taken from
// deps.DoRefer (Python lets the RAGTools context decide whether the model may
// cite the freshly read document), so callers control citation behavior.
// Returns nil when the document has no readable chunks (deps.DocChunks unset
// or the doc is unavailable).
func SummarizeDocument(ctx context.Context, deps SearchDeps, docID string, maxTokens int) []string {
	if deps.DocChunks == nil {
		return nil
	}
	return summarizeDocument(ctx, deps, docID, maxTokens)
}

// estimateTokens returns the token count of s. It prefers the precise
// tokenizer (tokenizer.NumTokensFromString, mirroring Python's
// num_tokens_from_string); when no encoder is loaded that returns 0, so we
// fall back to a character/estimateCharsPerToken approximation for a rough
// budget, like the previous Go-only behavior.
func estimateTokens(s string) int {
	if n := tokenizer.NumTokensFromString(s); n > 0 {
		return n
	}
	return (len([]rune(s)) + estimateCharsPerToken - 1) / estimateCharsPerToken
}
