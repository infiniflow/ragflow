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

func TestQAChunker_Registered(t *testing.T) {
	factory, _, _, ok := runtime.DefaultRegistry.Lookup("QAChunker")
	if !ok {
		t.Fatal("QAChunker not found in registry")
	}
	comp, err := factory("QAChunker", nil)
	if err != nil {
		t.Fatalf("factory failed: %v", err)
	}
	if comp == nil {
		t.Fatal("component is nil")
	}
}

func TestQAChunker_DelimiterTab(t *testing.T) {
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "What is Go?\tGo is a programming language.",
	}
	out, err := comp.Invoke(context.Background(), inputs)
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

func TestQAChunker_DelimiterComma(t *testing.T) {
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.csv",
		"output_format": "text",
		"text":          "What is Rust?,Rust is a systems language.",
	}
	out, err := comp.Invoke(context.Background(), inputs)
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

func TestQAChunker_Markdown(t *testing.T) {
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.md",
		"output_format": "markdown",
		"markdown":      "# What is Go?\nGo is a programming language.\n\n# What is Rust?\nRust is a systems language.",
	}
	out, err := comp.Invoke(context.Background(), inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
}

func TestQAChunker_HTMLTable(t *testing.T) {
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.xlsx",
		"output_format": "html",
		"html":          "<table><tr><td>Q1</td><td>A1</td></tr><tr><td>Q2</td><td>A2</td></tr></table>",
	}
	out, err := comp.Invoke(context.Background(), inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
}

func TestQAChunker_RmQAPrefix(t *testing.T) {
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "Question: What is Go?\tAnswer: Go is a language.",
	}
	out, err := comp.Invoke(context.Background(), inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	cww, _ := chunks[0]["content_with_weight"].(string)
	if cww != "Question: What is Go?\tAnswer: Go is a language." {
		t.Fatalf("prefix not stripped: %q", cww)
	}
}

func TestQAChunker_Empty(t *testing.T) {
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "empty.txt",
		"output_format": "text",
		"text":          "",
	}
	out, err := comp.Invoke(context.Background(), inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks, got %d", len(chunks))
	}
}

func TestQAChunker_CaseInsensitivePrefix(t *testing.T) {
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "QUESTION: Hello\tANSWER: World",
	}
	out, err := comp.Invoke(context.Background(), inputs)
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

func TestQAChunker_PrefixRequiresColonOrTab(t *testing.T) {
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "A language model is useful\tQ How does it work",
	}
	out, err := comp.Invoke(context.Background(), inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	cww, _ := chunks[0]["content_with_weight"].(string)
	if cww != "Question: A language model is useful\tAnswer: Q How does it work" {
		t.Fatalf("space-only separator should not strip prefix: %q", cww)
	}
}

func TestQAChunker_HeadingNoTrailingSpace(t *testing.T) {
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.md",
		"output_format": "markdown",
		"markdown":      "#Hello\nWorld\n",
	}
	out, err := comp.Invoke(context.Background(), inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
}

func TestQAChunker_ChineseLang(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "Chinese"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.txt",
		"output_format": "text",
		"text":          "什么是Go？\tGo是一种编程语言。",
	}
	out, err := comp.Invoke(context.Background(), inputs)
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

func TestQAChunker_MarkdownRendersHTML(t *testing.T) {
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "test.md",
		"output_format": "markdown",
		"markdown":      "# Title\nThis is **bold** text.\n",
	}
	out, err := comp.Invoke(context.Background(), inputs)
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
