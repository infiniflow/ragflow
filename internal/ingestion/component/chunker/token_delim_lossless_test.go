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

// User-perspective behaviour contract for the TokenChunker delimiter handling.
//
// A delimiter is a HINT for where the chunker MAY break, never a character to
// DELETE. After chunking, concatenating every emitted chunk's text must
// reproduce the original text (lossless): no sentence punctuation such as "。"
// may vanish, and no stray newline may be injected in its place.
//
// These tests assert that contract directly (count of "。" preserved, exact
// reconstruction) rather than hard-coding implementation-shaped strings.

package chunker

import (
	"context"
	"strings"
	"testing"
)

// joinChunks concatenates the text of every emitted chunk the way a downstream
// consumer would see it, for losslessness assertions.
func joinChunks(chunks []map[string]any) string {
	var b strings.Builder
	for _, c := range chunks {
		if s, ok := c["text"].(string); ok {
			b.WriteString(s)
		}
	}
	return b.String()
}

// driveChunker runs the TokenChunker component through its real Invoke path.
func driveChunker(t *testing.T, params map[string]any, input map[string]any) []map[string]any {
	t.Helper()
	comp, err := NewTokenChunker(params)
	if err != nil {
		t.Fatalf("construct TokenChunker: %v", err)
	}
	out, err := comp.Invoke(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("invoke TokenChunker: %v", err)
	}
	if msg, _ := out["_ERROR"].(string); msg != "" {
		t.Fatalf("TokenChunker returned _ERROR: %s", msg)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	return chunks
}

// TestDelimiterKeepsSentencePeriod_JSONPath is the regression for the reported
// bug: with the RAGFlow default delimiter set ("\n!?;。；！？") fed via the JSON
// path, "。" must survive and must NOT be replaced by a newline.
func TestDelimiterKeepsSentencePeriod_JSONPath(t *testing.T) {
	const src = "小明喜欢吃苹果。小红喜欢吃香蕉。小刚喜欢吃橙子。"
	input := map[string]any{
		"name":          "t",
		"output_format": "json",
		"json":          []map[string]any{{"text": src, "doc_type_kwd": "text"}},
	}
	chunks := driveChunker(t, map[string]any{
		"chunk_token_size": float64(512),
		"delimiter":        "\n!?;。；！？",
	}, input)
	got := joinChunks(chunks)
	if want, gotN := strings.Count(src, "。"), strings.Count(got, "。"); want != gotN {
		t.Errorf("sentence period '。' count: want %d (preserved), got %d (dropped into newline?)", want, gotN)
	}
	if !strings.Contains(got, "苹果。小红") {
		t.Errorf("merged text lost the '。' sentence boundary; got %q", got)
	}
}

// TestDelimiterKeepsSentencePeriod_TextPath covers the text/markdown/html path
// with the same default delimiter set: "。" must not be dropped there either.
func TestDelimiterKeepsSentencePeriod_TextPath(t *testing.T) {
	const src = "春眠不觉晓。处处闻啼鸟。夜来风雨声。花落知多少。"
	input := map[string]any{
		"name":          "t",
		"output_format": "text",
		"text":          src,
	}
	chunks := driveChunker(t, map[string]any{
		"chunk_token_size": float64(512),
		"delimiter":        "\n!?;。；！？",
	}, input)
	got := joinChunks(chunks)
	if want, gotN := strings.Count(src, "。"), strings.Count(got, "。"); want != gotN {
		t.Errorf("text path '。' count: want %d, got %d", want, gotN)
	}
	if !strings.Contains(got, "晓。处处") {
		t.Errorf("text path lost sentence boundary; got %q", got)
	}
}

// TestDelimiterLossless_Reconstruction asserts the core user contract: the
// concatenation of all chunk texts reproduces the source exactly (no char
// dropped, no stray char injected) for a mixed delimiter set.
func TestDelimiterLossless_Reconstruction(t *testing.T) {
	src := "第一章。第一节！这是内容；第二句？结尾。\n第二段，逗号不是分隔符，但句号是。"
	for _, delim := range []string{"\n!?;。；！？", "。！？", "\n"} {
		t.Run("delim="+delim, func(t *testing.T) {
			input := map[string]any{
				"name":          "t",
				"output_format": "json",
				"json":          []map[string]any{{"text": src, "doc_type_kwd": "text"}},
			}
			chunks := driveChunker(t, map[string]any{
				"chunk_token_size": float64(2048), // large: isolate delimiter behaviour from token-splitting
				"delimiter":        delim,
			}, input)
			got := joinChunks(chunks)
			if got != src {
				t.Errorf("lossless reconstruction failed:\n got=%q\nwant=%q", got, src)
			}
		})
	}
}

// TestDelimiter_LeadingTrailingAndConsecutive exercises non-happy-path
// delimiter placement: delimiter at the very start, at the very end, and
// consecutive delimiters must not delete or duplicate characters.
func TestDelimiter_LeadingTrailingAndConsecutive(t *testing.T) {
	src := "。开头句。中间。。双标点？结尾句。"
	input := map[string]any{
		"name":          "t",
		"output_format": "json",
		"json":          []map[string]any{{"text": src, "doc_type_kwd": "text"}},
	}
	chunks := driveChunker(t, map[string]any{
		"chunk_token_size": float64(2048),
		"delimiter":        "。？",
	}, input)
	got := joinChunks(chunks)
	if got != src {
		t.Errorf("non-happy-path reconstruction failed:\n got=%q\nwant=%q", got, src)
	}
}

// TestDelimiter_EmptyAndWhitespaceOnly guards against panics / dropped content
// when the source is empty or consists only of delimiter characters.
func TestDelimiter_EmptyAndWhitespaceOnly(t *testing.T) {
	for _, src := range []string{"", "。？。？", "   ", "\n\n"} {
		t.Run("src="+quoteCase(src), func(t *testing.T) {
			input := map[string]any{
				"name":          "t",
				"output_format": "json",
				"json":          []map[string]any{{"text": src, "doc_type_kwd": "text"}},
			}
			chunks := driveChunker(t, map[string]any{
				"chunk_token_size": float64(64),
				"delimiter":        "\n!?;。；！？",
			}, input)
			got := joinChunks(chunks)
			if len(got) > len(src) {
				t.Errorf("output longer than source: got %q (len %d) > src %q (len %d)", got, len(got), src, len(src))
			}
		})
	}
}

func quoteCase(s string) string {
	switch s {
	case "":
		return "empty"
	case "   ":
		return "spaces"
	case "\n\n":
		return `double-newline`
	default:
		return strings.NewReplacer("\n", "\\n").Replace(s)
	}
}
