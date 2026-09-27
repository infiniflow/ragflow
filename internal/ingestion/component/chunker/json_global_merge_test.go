//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied,
//  including without limitation the rights to use, copy, modify, merge,
//  publish, distribute, sublicense, and/or sell copies of the Software,
//  and to permit persons to whom the Software is furnished to do so,
//  subject to the following conditions:
//
//  The above copyright notice and this permission notice shall be included
//  in all copies or substantial portions of the Software.
//
//  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
//  EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
//  MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
//  NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
//  LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
//  OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
//  WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package chunker

// TestJSONGlobalMergeMatchesPython pins the TokenChunker json path's
// cross-item merge: adjacent text chunks across JSON items collapse into one
// chunk when they jointly fit chunk_token_size, matching Python's
// _merge_text_chunks_by_token_size.
import (
	"context"
	"strings"
	"testing"
)

func TestJSONGlobalMergeMatchesPython(t *testing.T) {
	const budget = 128
	comp, err := NewTokenChunker(map[string]any{
		"chunk_token_size": float64(budget),
	})
	if err != nil {
		t.Fatalf("construct TokenChunker: %v", err)
	}

	input := map[string]any{
		"name":          "t",
		"output_format": "json",
		"json": []map[string]any{
			{
				"text":         "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho sigma tau upsilon phi chi psi omega",
				"doc_type_kwd": "text",
			},
			{
				"text":         "another long item with many words that should be merged when the token budget allows merging across items on the python side but stays separate on the go side",
				"doc_type_kwd": "text",
			},
		},
	}

	out, err := comp.Invoke(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("invoke TokenChunker: %v", err)
	}
	if msg, _ := out["_ERROR"].(string); msg != "" {
		t.Fatalf("TokenChunker returned _ERROR: %s", msg)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk (global merge across items), got %d", len(chunks))
	}
	text, _ := chunks[0]["text"].(string)

	first := "alpha beta gamma"
	second := "another long item"
	i1 := strings.Index(text, first)
	i2 := strings.Index(text, second)
	if i1 < 0 || i2 < 0 {
		t.Errorf("merged chunk does not contain both items:\n%q", text)
	}
	// The two items must appear in source order within the single merged chunk.
	if i1 >= i2 {
		t.Errorf("merged chunk reordered items (want %q before %q):\n%q", first, second, text)
	}
}

// TestJSONSingleItemSubSplitUnderHardCap pins the TokenChunker json path's
// hard-cap handling of a single item that exceeds chunk_token_size: the
// over-budget item is re-split into <= budget chunks (sentence boundaries
// first, hard token-split fallback) whose concatenated text reproduces the
// input verbatim. This replaces the former #17799 "kept whole" contract.
func TestJSONSingleItemSubSplitUnderHardCap(t *testing.T) {
	const budget = 128
	comp, err := NewTokenChunker(map[string]any{
		"chunk_token_size": float64(budget),
	})
	if err != nil {
		t.Fatalf("construct TokenChunker: %v", err)
	}

	// A single item far over the token budget (well above 128 tokens).
	long := strings.Repeat("word ", 200)
	input := map[string]any{
		"name":          "t",
		"output_format": "json",
		"json": []map[string]any{
			{"text": long, "doc_type_kwd": "text"},
		},
	}

	out, err := comp.Invoke(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("invoke TokenChunker: %v", err)
	}
	if msg, _ := out["_ERROR"].(string); msg != "" {
		t.Fatalf("TokenChunker returned _ERROR: %s", msg)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) < 2 {
		t.Fatalf("over-budget item must be split, got %d chunk(s)", len(chunks))
	}
	var joined string
	for i, ck := range chunks {
		text, _ := ck["text"].(string)
		if n := tokenizeStr(text); n > budget {
			t.Errorf("chunk %d exceeds budget: tokens=%d (cap=%d)", i, n, budget)
		}
		joined += strings.TrimSpace(text)
	}
	if strings.ReplaceAll(joined, " ", "") != strings.ReplaceAll(strings.TrimSpace(long), " ", "") {
		t.Errorf("split not lossless:\n got=%q\nwant=%q", joined, strings.TrimSpace(long))
	}
}
