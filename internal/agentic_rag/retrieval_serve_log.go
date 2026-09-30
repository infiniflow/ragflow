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
		zap.String("content", hit.chunk.Content),
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
	if len(hits) == 0 {
		return
	}
	for _, hit := range hits {
		common.DebugCtx(ctx, "agentic_rag: retrieval served chunk", servedChunkFields(ctx, tool, query, hit)...)
	}
}
