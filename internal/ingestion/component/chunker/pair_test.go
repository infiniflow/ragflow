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

	"ragflow/internal/agent/runtime"
)

func TestPairChunker_Registered(t *testing.T) {
	factory, _, _, ok := runtime.DefaultRegistry.Lookup("PairChunker")
	if !ok {
		t.Fatal("PairChunker not found in registry")
	}
	comp, err := factory("PairChunker", nil)
	if err != nil {
		t.Fatalf("factory failed: %v", err)
	}
	if comp == nil {
		t.Fatal("component is nil")
	}
}

func TestPairChunker_DelimiterTab(t *testing.T) {
	comp, err := NewPairChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "What is Go?\tGo is a programming language.",
	}
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	chunk := chunks[0]
	cww, _ := chunk["content_with_weight"].(string)
	if cww != "Question: What is Go?\tAnswer: Go is a programming language." {
		t.Fatalf("unexpected content: %q", cww)
	}
}

func TestPairChunker_DelimiterComma(t *testing.T) {
	comp, err := NewPairChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.csv",
		"output_format": "text",
		"text":          "What is Rust?,Rust is a systems language.",
	}
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	chunk := chunks[0]
	cww, _ := chunk["content_with_weight"].(string)
	if cww != "Question: What is Rust?\tAnswer: Rust is a systems language." {
		t.Fatalf("unexpected content: %q", cww)
	}
}

func TestPairChunker_Markdown(t *testing.T) {
	comp, err := NewPairChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.md",
		"output_format": "markdown",
		"markdown":      "# What is Go?\nGo is a programming language.\n\n# What is Rust?\nRust is a systems language.",
	}
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
}

func TestPairChunker_HTMLTable(t *testing.T) {
	comp, err := NewPairChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.xlsx",
		"output_format": "html",
		"html":          "<table><tr><td>Q1</td><td>A1</td></tr><tr><td>Q2</td><td>A2</td></tr></table>",
	}
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
}

func TestPairChunker_RmQAPrefix(t *testing.T) {
	comp, err := NewPairChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "Question: What is Go?\tAnswer: Go is a language.",
	}
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	cww, _ := chunks[0]["content_with_weight"].(string)
	if cww != "Question: What is Go?\tAnswer: Go is a language." {
		t.Fatalf("prefix not stripped: %q", cww)
	}
}

func TestPairChunker_Empty(t *testing.T) {
	comp, err := NewPairChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "empty.txt",
		"output_format": "text",
		"text":          "",
	}
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks, got %d", len(chunks))
	}
}

func TestPairChunker_CaseInsensitivePrefix(t *testing.T) {
	comp, err := NewPairChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "QUESTION: Hello\tANSWER: World",
	}
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	cww, _ := chunks[0]["content_with_weight"].(string)
	if cww != "Question: Hello\tAnswer: World" {
		t.Fatalf("case-insensitive prefix not stripped: %q", cww)
	}
}

func TestPairChunker_PrefixSpaceSeparatorStrips(t *testing.T) {
	comp, err := NewPairChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "A language model is useful\tQ How does it work",
	}
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	cww, _ := chunks[0]["content_with_weight"].(string)
	// Python qa.py:241 uses `[\t:： ]+`, so a space is a valid separator:
	// a leading "A"/"Q" followed by a space is stripped. .
	if cww != "Question: language model is useful\tAnswer: How does it work" {
		t.Fatalf("space-separator prefix not stripped: %q", cww)
	}
}

func TestPairChunker_HeadingNoTrailingSpace(t *testing.T) {
	comp, err := NewPairChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.md",
		"output_format": "markdown",
		"markdown":      "#Hello\nWorld\n",
	}
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
}

func TestPairChunker_ChineseLang(t *testing.T) {
	comp, err := NewPairChunker(map[string]any{"lang": "Chinese"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "什么是Go？\tGo是一种编程语言。",
	}
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	cww, _ := chunks[0]["content_with_weight"].(string)
	if want := "问题：什么是Go？\t回答：Go是一种编程语言。"; cww != want {
		t.Fatalf("unexpected content: %q, want %q", cww, want)
	}
}

func TestPairChunker_ModeDefaultsToQA(t *testing.T) {
	// mode is optional and defaults to "qa": constructing without a mode
	// yields the canonical question/answer behaviour.
	comp, err := NewPairChunker(nil)
	if err != nil {
		t.Fatalf("NewPairChunker with no mode: %v", err)
	}
	if got := comp.(*PairChunkerComponent).param.Mode; got != "qa" {
		t.Fatalf("default mode = %q, want %q", got, "qa")
	}
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "What is Go?\tGo is a programming language.",
	}
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	cww, _ := chunks[0]["content_with_weight"].(string)
	if cww != "问题：What is Go?\t回答：Go is a programming language." {
		t.Fatalf("unexpected content: %q", cww)
	}
}

func TestPairChunker_ModeRejectsUnknown(t *testing.T) {
	// The generic two-column chunker exposes a `mode` selector; today only
	// "qa" is implemented, so any other mode must fail fast at construction
	// instead of silently producing QA output. An explicit empty mode
	// defaults to "qa".
	for _, mode := range []string{"", "tag", "Table"} {
		params := map[string]any{"mode": mode}
		_, err := NewPairChunker(params)
		if mode == "" {
			if err != nil {
				t.Fatalf("empty mode should default to qa, got error: %v", err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("mode %q should be rejected, got no error", mode)
		}
	}
}

func TestPairChunker_MarkdownRendersHTML(t *testing.T) {
	comp, err := NewPairChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.md",
		"output_format": "markdown",
		"markdown":      "# Title\nThis is **bold** text.\n",
	}
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	cww, _ := chunks[0]["content_with_weight"].(string)
	if !strings.Contains(cww, "<strong>bold</strong>") &&
		!strings.Contains(cww, "<b>bold</b>") {
		t.Fatalf("markdown not rendered to HTML: %q", cww)
	}
}
