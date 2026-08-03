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
//  The above copyright notice and this permission notice shall be
//  included in all copies or substantial portions of the Software.
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
// cross-item merge against Python's _merge_text_chunks_by_token_size.
//
// Python merges adjacent text chunks across JSON items into a single global
// token budget, so two items that jointly fit chunk_token_size come back as
// ONE chunk. Go historically merged within each item separately, emitting one
// chunk per item. The oracle (chunk count = 1) is Python's behaviour and must
// hold.
//
// Self-contained: it drives NewTokenChunker directly (no .venv, no golden
// harness), so it runs in the default unit tier.
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
		t.Fatalf("want 1 chunk (Python global merge across items), got %d", len(chunks))
	}
	text, _ := chunks[0]["text"].(string)
	if !strings.Contains(text, "alpha beta gamma") || !strings.Contains(text, "another long item") {
		t.Errorf("merged chunk does not contain both items:\n%q", text)
	}
}

// TestJSONSingleItemNotSubSplitMatchesPython pins the TokenChunker json
// path's handling of a single item that exceeds chunk_token_size. Python's
// json path keeps each over-budget item whole (no sub-split), so one input
// item yields exactly one chunk. Go historically sub-split it via
// splitOversizedUnit, emitting two chunks for one logical item.
func TestJSONSingleItemNotSubSplitMatchesPython(t *testing.T) {
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
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk (Python keeps over-budget single item whole), got %d", len(chunks))
	}
}
