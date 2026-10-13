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
	"testing"

	"ragflow/internal/agent/runtime"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// fieldMap flattens zap fields for assertions (zap stores each type in its own
// slot, so Interface is nil for the typed constructors).
func fieldMap(fields []zap.Field) map[string]any {
	got := map[string]any{}
	for _, f := range fields {
		switch f.Type {
		case zapcore.StringType:
			got[f.Key] = f.String
		case zapcore.BoolType:
			got[f.Key] = f.Integer == 1
		default:
			got[f.Key] = int(f.Integer) // zap stores ints as int64; compare as int
		}
	}
	return got
}

// TestServedChunkLogFields pins the shape of the serve record: it must answer
// "what reached the model?" without needing the (2000-char truncated) tool
// result log — chunk identity, sizes, truncation/preview flags, how many query
// terms the snippet carries, and the snippet text itself.
func TestServedChunkLogFields(t *testing.T) {
	ctx := context.Background()
	fields := servedChunkFields(ctx, "grep_chunks", "Boston skyline", snippetHit{
		chunk: runtime.RetrievalChunk{
			ID: "c1", DocumentID: "d1", DocumentName: "24653.md",
			Content: strings.Repeat("x ", 400),
		},
		snippet:   "the Boston skyline at dawn",
		truncated: true,
	})
	got := fieldMap(fields)
	if got["tool"] != "grep_chunks" || got["query"] != "Boston skyline" {
		t.Fatalf("tool/query missing: %+v", got)
	}
	if got["chunk_id"] != "c1" || got["doc_id"] != "d1" || got["doc_name"] != "24653.md" {
		t.Fatalf("chunk identity missing: %+v", got)
	}
	if got["chunk_runes"] != 800 || got["snippet_runes"] != 26 {
		t.Fatalf("sizes wrong: chunk_runes=%v snippet_runes=%v", got["chunk_runes"], got["snippet_runes"])
	}
	if got["query_terms"] != 2 {
		t.Fatalf("query_terms = %v, want 2 (Boston, skyline)", got["query_terms"])
	}
	if got["truncated"] != true {
		t.Fatalf("truncation flag missing: %+v", got)
	}
	if got["snippet"] != "the Boston skyline at dawn" {
		t.Fatalf("snippet text missing: %v", got["snippet"])
	}

	// A preview serve (no literal term match) must record zero query terms, and
	// an over-long snippet is capped rather than dropped.
	long := servedChunkFields(ctx, "search_bm25_chunks", "mineralizer", snippetHit{
		chunk:   runtime.RetrievalChunk{ID: "c2", Content: "body"},
		snippet: strings.Repeat("y", serveLogSnippetRunes+50),
		preview: true,
	})
	capped := fieldMap(long)
	if capped["query_terms"] != 0 || capped["preview"] != true {
		t.Fatalf("preview serve recorded wrong: %+v", capped)
	}
	if n := len([]rune(capped["snippet"].(string))); n > serveLogSnippetRunes+1 {
		t.Fatalf("snippet not capped: %d runes", n)
	}
}
