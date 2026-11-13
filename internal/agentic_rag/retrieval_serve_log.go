/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package agentic_rag

import (
	"context"
	"strings"
	"unicode/utf8"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"

	"go.uber.org/zap"
)

// Serve-level retrieval logging: what did the model ACTUALLY get?
//
// The generic tool-result log line truncates the payload at 2000 characters, so
// it cannot answer the one question that matters when a benchmark answer is
// wrong: did the sentence carrying the answer ever reach the model? Measured on
// the browsecomp retry batch, 72% of tool-result lines were truncated, which
// makes "the gold string never appears in the log" unable to separate a recall
// miss from a reasoning miss — and the two need opposite fixes.
//
// One debug line per SERVED CHUNK answers it: which chunk and document, how big
// it was, whether it was truncated or a preview, how many of the query's terms
// the snippet contains, and the snippet text itself. It rides the existing debug
// channel — the same `--debug` switch, the same rotation, the same session_id
// correlation — so there is no second log file and no extra env switch to
// forget. Volume is the tool result itself, which is what debug is for.
const serveLogSnippetRunes = 1200

// servedChunkFields builds the debug fields for one served chunk. Split out from
// the logging call so the shape of the record is unit-testable.
func servedChunkFields(ctx context.Context, tool, query string, hit snippetHit) []zap.Field {
	snippet := hit.snippet
	if r := []rune(snippet); len(r) > serveLogSnippetRunes {
		snippet = string(r[:serveLogSnippetRunes]) + "…"
	}
	lower := strings.ToLower(hit.snippet)
	matched := 0
	for _, term := range bm25TermTokens([]string{query}) {
		if term != "" && strings.Contains(lower, strings.ToLower(term)) {
			matched++
		}
	}
	fields := []zap.Field{
		zap.String("tool", tool),
		zap.String("query", query),
		zap.String("chunk_id", hit.chunk.ID),
		zap.String("doc_id", hit.chunk.DocumentID),
		zap.String("doc_name", hit.chunk.DocumentName),
		zap.Int("chunk_runes", utf8.RuneCountInString(hit.chunk.Content)),
		zap.Int("snippet_runes", utf8.RuneCountInString(hit.snippet)),
		zap.Int("query_terms", matched),
		zap.String("snippet", snippet),
	}
	if hit.truncated {
		fields = append(fields, zap.Bool("truncated", true))
	}
	if hit.preview {
		fields = append(fields, zap.Bool("preview", true))
	}
	return fields
}

// logServedChunks writes one debug line per served chunk (a no-op unless the
// logger runs at debug level).
func logServedChunks(ctx context.Context, tool, query string, hits []snippetHit) {
	if len(hits) == 0 || common.Logger == nil || common.Logger.Check(zap.DebugLevel, "agentic_rag: retrieval served chunk") == nil {
		return
	}
	for _, hit := range hits {
		common.DebugCtx(ctx, "agentic_rag: retrieval served chunk", servedChunkFields(ctx, tool, query, hit)...)
	}
}

// serveLogContentRunes bounds the deep-read body copied into the log. Deep-read
// text is the payload itself, not a triage view, so the bound is the segment
// budget rather than the snippet budget.
const serveLogContentRunes = 6000

// servedDeepReadFields builds the debug fields for one deep-read passage that
// actually reached the model (i.e. AFTER the sufficiency filter). It is the
// deep-read twin of servedChunkFields, and it exists for the same reason: the
// generic tool-result line truncates the payload, so without it "the sentence
// carrying the answer was never served" cannot be told apart from "it was
// served and the model still missed it". ref_attr records the [ID:n] the
// passage was stamped with, so a citation can be mapped back to the text.
func servedDeepReadFields(docID string, c runtime.RetrievalChunk, refAttr string) []zap.Field {
	content := c.Content
	if r := []rune(content); len(r) > serveLogContentRunes {
		content = string(r[:serveLogContentRunes]) + "…"
	}
	return []zap.Field{
		zap.String("side", "deep_read"),
		zap.String("doc_id", docID),
		zap.String("chunk_id", c.ID),
		zap.String("doc_name", c.DocumentName),
		zap.Int("chunk_index", c.ChunkIndex),
		zap.String("ref_attr", strings.TrimSpace(refAttr)),
		zap.Int("content_runes", utf8.RuneCountInString(c.Content)),
		zap.String("content", content),
	}
}

// logServedDeepRead writes one debug line per deep-read passage (a no-op unless
// the logger runs at debug level).
func logServedDeepRead(ctx context.Context, docID string, c runtime.RetrievalChunk, refAttr string) {
	if common.Logger == nil || common.Logger.Check(zap.DebugLevel, "agentic_rag: retrieval served deep read") == nil {
		return
	}
	common.DebugCtx(ctx, "agentic_rag: retrieval served deep read", servedDeepReadFields(docID, c, refAttr)...)
}
