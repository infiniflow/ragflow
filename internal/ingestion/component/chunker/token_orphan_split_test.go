// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chunker

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

// TestSplitOversizedTextDoesNotCreateInteriorOrphan covers the leading-newline
// case where adding the delimiter after hard splitting used to leave a one-
// token chunk between otherwise full chunks.
func TestSplitOversizedTextDoesNotCreateInteriorOrphan(t *testing.T) {
	const target = 32
	text := "\n" + strings.Repeat("word ", 400)

	pieces := splitOversizedText(schema.ChunkDoc{
		Text:   text,
		CKType: "text",
	}, target)
	if len(pieces) < 3 {
		t.Fatalf("fixture should produce a first piece and multiple hard-split pieces, got %d", len(pieces))
	}

	var joined strings.Builder
	for i, piece := range pieces {
		tokens := tokenizeStr(piece.Text)
		joined.WriteString(piece.Text)
		if tokens > target {
			t.Errorf("piece %d has %d tokens, exceeds target %d: %q", i, tokens, target, piece.Text)
		}
		if got := intValue(piece.TKNums); got != tokens {
			t.Errorf("piece %d records %d tokens, want %d", i, got, tokens)
		}
		if i > 0 && i < len(pieces)-1 && tokens < target/2 {
			t.Errorf("piece %d is an interior orphan with only %d tokens: %q", i, tokens, piece.Text)
		}
	}
	if got := joined.String(); got != text {
		t.Fatalf("split is not lossless:\n got %q\nwant %q", got, text)
	}
}

// TestTokenChunkerJSONOversizeUnitSplitNoOrphan covers the same regression
// through the JSON ingestion path, where a blank line is folded into the
// oversized item before it reaches splitOversizedText.
func TestTokenChunkerJSONOversizeUnitSplitNoOrphan(t *testing.T) {
	const target = 32
	text := "Title\n\n" + strings.Repeat("word ", 400)

	chunker, err := NewTokenChunker(map[string]any{
		"chunk_token_size": float64(target),
	})
	if err != nil {
		t.Fatalf("construct TokenChunker: %v", err)
	}
	out, err := chunker.Invoke(context.Background(), nil, map[string]any{
		"name":          "document.json",
		"output_format": "json",
		"json": []map[string]any{
			{"text": text, "doc_type_kwd": "text"},
		},
	})
	if err != nil {
		t.Fatalf("invoke TokenChunker: %v", err)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks missing or wrong type: %T", out["chunks"])
	}
	if len(chunks) < 3 {
		t.Fatalf("oversized JSON item must produce a title and split body, got %d chunks", len(chunks))
	}

	var joined strings.Builder
	for i, chunk := range chunks {
		piece, ok := chunk["text"].(string)
		if !ok {
			t.Fatalf("chunk %d text = %T, want string", i, chunk["text"])
		}
		tokens := tokenizeStr(piece)
		if tokens > target {
			t.Errorf("chunk %d exceeds token cap: got %d, want <= %d", i, tokens, target)
		}
		if i > 0 && i < len(chunks)-1 && tokens < target/2 {
			t.Errorf("chunk %d is an interior orphan: got %d tokens", i, tokens)
		}
		joined.WriteString(piece)
	}
	if joined.String() != text {
		t.Errorf("JSON chunking is not lossless:\n got %q\nwant %q", joined.String(), text)
	}
}
