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

package chunker

import (
	"context"
	"strings"
	"testing"
)

// qaInvoke is a small helper that runs the QA chunker on a upstream-style
// input map and returns the produced chunks as generic maps.
func qaInvoke(t *testing.T, inputs map[string]any) []map[string]any {
	t.Helper()
	c, err := NewQAChunker(map[string]any{})
	if err != nil {
		t.Fatalf("NewQAChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("Invoke did not return []map chunks: %#v", out["chunks"])
	}
	return chunks
}

// TestQAChunker_DefaultLangIsChinese exercises migration diff Chunker-2.13:
// when no language is supplied, Python defaults to Chinese prefixes
// ("问题："/"回答："); the legacy Go code defaulted to English.
func TestQAChunker_DefaultLangIsChinese(t *testing.T) {
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "What is RAG?\tRAG retrieves then generates.",
	}
	chunks := qaInvoke(t, inputs)
	if len(chunks) == 0 {
		t.Fatalf("no chunks produced")
	}
	cw := chunks[0]["content_with_weight"].(string)
	if !contains(cw, "问题：") || !contains(cw, "回答：") {
		t.Errorf("empty lang should default to Chinese prefixes, got %q", cw)
	}
	if contains(cw, "Question:") || contains(cw, "Answer:") {
		t.Errorf("empty lang must not use English prefixes, got %q", cw)
	}
}

// TestRmQAPrefixStripsMultipleSeparators exercises migration diff
// Chunker-2.12: the prefix regex must allow one-or-more separator chars
// (Python uses `[\t:： ]+`), so "Q:: answer" is fully stripped. The legacy
// Go pattern only matched a single separator, leaving ": answer".
func TestRmQAPrefixStripsMultipleSeparators(t *testing.T) {
	if got := rmQAPrefix("Q:: answer"); got != "answer" {
		t.Errorf("multi-separator prefix not fully stripped: got %q", got)
	}
	if got := rmQAPrefix("Question: foo"); got != "foo" {
		t.Errorf("single-separator prefix regression: got %q", got)
	}
	if got := rmQAPrefix("问：答案在此"); got != "答案在此" {
		t.Errorf("CJK prefix regression: got %q", got)
	}
}

// TestQAChunker_SetsTopInt exercises migration diff Chunker-1.8 (top_int):
// each QA chunk must carry the source row index in `top_int`, matching
// Python beAdoc(..., row_num=i).
func TestQAChunker_SetsTopInt(t *testing.T) {
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "Q1\tA1\nQ2\tA2",
	}
	chunks := qaInvoke(t, inputs)
	if len(chunks) != 2 {
		t.Fatalf("want 2 QA chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		raw, ok := c["top_int"]
		if !ok {
			t.Fatalf("chunk %d missing top_int field", i)
		}
		arr, ok := raw.([]any)
		if !ok || len(arr) != 1 {
			t.Fatalf("chunk %d top_int wrong shape: %#v", i, raw)
		}
		if int(arr[0].(float64)) != i {
			t.Errorf("chunk %d top_int = %v, want %d", i, arr[0], i)
		}
	}
}

// TestQAChunker_CarriesImageAndPositions exercises migration diff
// Chunker-1.8 (image / positions): when the upstream JSON item already
// carries an image id and pdf positions, the QA chunk must preserve them
// (Python beAdocPdf sets d["image"] and add_positions).
func TestQAChunker_CarriesImageAndPositions(t *testing.T) {
	inputs := map[string]any{
		"name":          "test.pdf",
		"output_format": "json",
		"json": []map[string]any{
			{
				"text":           "Q\tA",
				"image":          "img-42",
				"_pdf_positions": [][]int{{1, 2, 3, 4, 5}},
			},
		},
	}
	chunks := qaInvoke(t, inputs)
	if len(chunks) != 1 {
		t.Fatalf("want 1 QA chunk, got %d", len(chunks))
	}
	c := chunks[0]
	if c["image"] != "img-42" {
		t.Errorf("QA chunk lost upstream image: %#v", c["image"])
	}
	if _, ok := c["_pdf_positions"]; !ok {
		t.Errorf("QA chunk lost upstream _pdf_positions")
	}
	// The prefix-stripped content must still be present.
	cw, _ := c["content_with_weight"].(string)
	if !contains(cw, "A") {
		t.Errorf("QA content missing answer: %q", cw)
	}
}

// contains is a tiny helper to avoid importing strings in every test.
func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
