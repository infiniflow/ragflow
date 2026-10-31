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

//go:build integration

package component

import (
	"context"
	"ragflow/internal/ingestion/component/schema"
	"testing"
)

// TestTokenizerComponent_Embedding_SkipsOnlyEmptyTableChunk: in a batch where
// one chunk is nothing but an empty table, only the chunk with text may reach
// the model. Python strips table-structure tags before the emptiness check
// (rag/flow/tokenizer/tokenizer.py:86), so an empty-table chunk contributes no
// embeddable text and the title encode is skipped too (no content to embed).
func TestTokenizerComponent_Embedding_SkipsOnlyEmptyTableChunk(t *testing.T) {
	requireTokenizerPool(t)
	c, stub := withStubEmbedder(t, 2)
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"name":          "doc.pdf",
		"kb_id":         "kb-1",
		"output_format": "chunks",
		"chunks": []map[string]any{
			{"text": "<table><tr><td></td></tr></table>"},
			{"text": "real text"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	// One title encode plus one content encode for the surviving chunk.
	if got := stub.calls.Load(); got != 2 {
		t.Fatalf("embedder calls = %d, want 2 (title + the one chunk with text)", got)
	}
	if len(stub.callInputs) != 2 || len(stub.callInputs[1]) != 1 || stub.callInputs[1][0] != "real text" {
		t.Fatalf("content call inputs = %v, want [real text]", stub.callInputs)
	}
	chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
	if len(chunks) != 2 {
		t.Fatalf("output chunks = %d, want 2 (a skipped chunk stays in the payload)", len(chunks))
	}
	if _, ok := chunks[0]["q_2_vec"]; ok {
		t.Error("the empty-table chunk must not carry a vector")
	}
	if _, ok := chunks[1]["q_2_vec"]; !ok {
		t.Error("the chunk with text must carry a vector")
	}
}

// TestTokenizerComponent_Embedding_TableWithTextIsEmbeddedStripped: what
// reaches the model is the stripped text, not the raw HTML — Python appends
// cleaned_txt to the embedding batch (tokenizer.py:89), so the tags are
// replaced by spaces on the way in.
func TestTokenizerComponent_Embedding_TableWithTextIsEmbeddedStripped(t *testing.T) {
	requireTokenizerPool(t)
	c, stub := withStubEmbedder(t, 2)
	_, err := c.Invoke(context.Background(), nil, map[string]any{
		"name":          "doc.pdf",
		"kb_id":         "kb-1",
		"output_format": "chunks",
		"chunks": []map[string]any{
			{"text": "<table><tr><td>hello</td></tr></table>"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(stub.callInputs) != 2 || stub.callInputs[1][0] != "hello" {
		t.Fatalf("content call inputs = %v, want the stripped text [hello]", stub.callInputs)
	}
}

// TestTokenizerComponent_Embedding_LongAttributeTableTagIsNotStripped pins the
// Python parity quirk: the attribute group in the strip pattern is bounded to
// 12 non-tag characters, so a tag with longer attributes survives the strip —
// only its attribute-free closing tag is replaced by a space.
func TestTokenizerComponent_Embedding_LongAttributeTableTagIsNotStripped(t *testing.T) {
	requireTokenizerPool(t)
	c, stub := withStubEmbedder(t, 2)
	_, err := c.Invoke(context.Background(), nil, map[string]any{
		"name":          "doc.pdf",
		"kb_id":         "kb-1",
		"output_format": "chunks",
		"chunks": []map[string]any{
			{"text": `<table class="a-longer-class-name">cell</table>`},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	want := `<table class="a-longer-class-name">cell`
	if len(stub.callInputs) != 2 || stub.callInputs[1][0] != want {
		t.Fatalf("content call inputs = %v, want [%s]", stub.callInputs, want)
	}
}
