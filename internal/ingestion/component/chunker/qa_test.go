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
	"ragflow/internal/common"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/storage"
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
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
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

func TestQAChunker_DelimiterComma(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
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
	out, err := comp.Invoke(context.Background(), nil, inputs)
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
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
}

func TestQAChunker_RmQAPrefix(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
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
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks, got %d", len(chunks))
	}
}

func TestQAChunker_CaseInsensitivePrefix(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
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

func TestQAChunker_PrefixSpaceSeparatorStrips(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
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
	// a leading "A"/"Q" followed by a space is stripped.
	if cww != "Question: language model is useful\tAnswer: How does it work" {
		t.Fatalf("space-separator prefix not stripped: %q", cww)
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
	out, err := comp.Invoke(context.Background(), nil, inputs)
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

// withQAMemoryStorage swaps the global storage factory for an
// in-memory backend and restores it on cleanup. Mirrors
// withMemoryStorage in internal/ingestion/component/file_test.go.
func withQAMemoryStorage(t *testing.T) *storage.MemoryStorage {
	t.Helper()
	factory := storage.GetStorageFactory()
	prev := factory.GetStorage()
	ms := storage.NewMemoryStorage().(*storage.MemoryStorage)
	factory.SetStorage(ms)
	t.Cleanup(func() { factory.SetStorage(prev) })
	return ms
}

// TestQAChunker_TxtReacquiresRawFromStorage is the regression test for
// txt QA files producing zero chunks: the upstream text parser shreds
// tab-separated Q&A lines into delimiter-less fragments, so the chunker
// must re-read the raw file bytes from storage (mirrors Python qa.py).
func TestQAChunker_TxtReacquiresRawFromStorage(t *testing.T) {
	ms := withQAMemoryStorage(t)
	ctx := context.Background()
	raw := "What is Go?\tGo is a programming language.\nWhat is Rust?\tRust is a systems language.\n"
	if err := ms.Put(ctx, "kb-1", "qa.txt", []byte(raw)); err != nil {
		t.Fatal(err)
	}
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "qa.txt",
		"output_format": "json",
		// What the shredded upstream payload looks like: no delimiter
		// survives in any fragment, so JSON extraction yields nothing.
		"json": []map[string]any{
			{"text": "What is Go?"},
			{"text": " Go is a programming language."},
			{"text": "What is Rust?"},
			{"text": " Rust is a systems language."},
		},
		"bucket": "kb-1",
		"path":   "qa.txt",
	}
	out, err := comp.Invoke(ctx, nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if msg, ok := out["_ERROR"]; ok {
		t.Fatalf("unexpected _ERROR: %v", msg)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	cww, _ := chunks[0]["content_with_weight"].(string)
	if want := "Question: What is Go?\tAnswer: Go is a programming language."; cww != want {
		t.Fatalf("unexpected content: %q, want %q", cww, want)
	}
}

// TestQAChunker_TxtReacquiresRawViaDocID covers the doc_id-driven
// storage resolution path (no explicit bucket/path on the wire).
func TestQAChunker_TxtReacquiresRawViaDocID(t *testing.T) {
	ms := withQAMemoryStorage(t)
	ctx := context.Background()
	if err := ms.Put(ctx, "kb-1", "loc/qa.txt", []byte("Q1\tA1\n")); err != nil {
		t.Fatal(err)
	}
	prev := component.ResolveDocumentStorageOverride
	component.ResolveDocumentStorageOverride = func(docID string) (*component.DocumentStorageRef, error) {
		if docID != "doc-1" {
			t.Errorf("unexpected doc_id: %q", docID)
		}
		return &component.DocumentStorageRef{Bucket: "kb-1", Path: "loc/qa.txt"}, nil
	}
	t.Cleanup(func() { component.ResolveDocumentStorageOverride = prev })

	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "qa.txt",
		"output_format": "json",
		"json":          []map[string]any{{"text": "Q1"}},
		"doc_id":        "doc-1",
	}
	out, err := comp.Invoke(ctx, nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	cww, _ := chunks[0]["content_with_weight"].(string)
	if want := "Question: Q1\tAnswer: A1"; cww != want {
		t.Fatalf("unexpected content: %q, want %q", cww, want)
	}
}

// TestQAChunker_MarkdownReacquiresRawFromStorage covers heading-based
// md QA files: block-split markdown items carry no delimiter, so the
// raw file must be re-read (Python qa.py md branch).
func TestQAChunker_MarkdownReacquiresRawFromStorage(t *testing.T) {
	ms := withQAMemoryStorage(t)
	ctx := context.Background()
	raw := "# What is Go?\nGo is a **programming** language.\n\n# What is Rust?\nRust is a systems language.\n"
	if err := ms.Put(ctx, "kb-1", "qa.md", []byte(raw)); err != nil {
		t.Fatal(err)
	}
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "qa.md",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "What is Go?"},
			{"text": "Go is a **programming** language."},
		},
		"bucket": "kb-1",
		"path":   "qa.md",
	}
	out, err := comp.Invoke(ctx, nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	cww, _ := chunks[0]["content_with_weight"].(string)
	if !strings.Contains(cww, "Question: What is Go?") {
		t.Fatalf("unexpected content: %q", cww)
	}
	if !strings.Contains(cww, "<strong>programming</strong>") &&
		!strings.Contains(cww, "<b>programming</b>") {
		t.Fatalf("markdown answer not rendered to HTML: %q", cww)
	}
}

// TestQAChunker_TxtNoStorageRefFallsBackToPayload verifies that a txt
// input without any storage reference keeps the pre-existing payload
// behavior (direct-fed canvas runs, unit tests).
func TestQAChunker_TxtNoStorageRefFallsBackToPayload(t *testing.T) {
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "qa.txt",
		"output_format": "json",
		"json":          []map[string]any{{"text": "What is Go?\tGo is a programming language."}},
	}
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if msg, ok := out["_ERROR"]; ok {
		t.Fatalf("unexpected _ERROR: %v", msg)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
}

// TestQAChunker_TxtStorageFailureFallsBackToPayload verifies that a
// storage read failure degrades to the upstream payload with a warning
// rather than failing the component.
func TestQAChunker_TxtStorageFailureFallsBackToPayload(t *testing.T) {
	withQAMemoryStorage(t) // object intentionally absent
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "qa.txt",
		"output_format": "json",
		"json":          []map[string]any{{"text": "What is Go?\tGo is a programming language."}},
		"bucket":        "kb-1",
		"path":          "missing.txt",
	}
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if msg, ok := out["_ERROR"]; ok {
		t.Fatalf("unexpected _ERROR: %v", msg)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
}

// TestQAChunker_TxtReacquireStripsBOM verifies a UTF-8 BOM does not
// leak into the first question.
func TestQAChunker_TxtReacquireStripsBOM(t *testing.T) {
	ms := withQAMemoryStorage(t)
	ctx := context.Background()
	if err := ms.Put(ctx, "kb-1", "qa.txt", []byte("\ufeffQ1\tA1\n")); err != nil {
		t.Fatal(err)
	}
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{
		"name":          "qa.txt",
		"output_format": "json",
		"json":          []map[string]any{{"text": "Q1"}},
		"bucket":        "kb-1",
		"path":          "qa.txt",
	}
	out, err := comp.Invoke(ctx, nil, inputs)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	cww, _ := chunks[0]["content_with_weight"].(string)
	if want := "Question: Q1\tAnswer: A1"; cww != want {
		t.Fatalf("unexpected content: %q, want %q", cww, want)
	}
}

// TestQAChunker_DecoratorAssignsDistinctIDs pins the chunk-id regression
// where the registration decorator hashed ck["text"] — absent on QA chunks
// (they carry content_with_weight) — collapsing every QA chunk of a
// document onto the same id, so all but one chunk were silently
// overwritten at index time.
func TestQAChunker_DecoratorAssignsDistinctIDs(t *testing.T) {
	ms := withQAMemoryStorage(t)
	ctx := context.Background()
	raw := "What is Go?\tGo is a programming language.\nWhat is Rust?\tRust is a systems language.\n"
	if err := ms.Put(ctx, testKBID, "qa.txt", []byte(raw)); err != nil {
		t.Fatal(err)
	}
	comp, err := NewQAChunker(map[string]any{"lang": "english"})
	if err != nil {
		t.Fatal(err)
	}
	decorated := &imageUploadDecorator{inner: comp}
	inputs := map[string]any{
		"name":          "qa.txt",
		"kb_id":         testKBID,
		"doc_id":        testDocID,
		"output_format": "json",
		"json":          []map[string]any{{"text": "What is Go?"}}, // shredded upstream payload; raw bytes win
		"bucket":        testKBID,
		"path":          "qa.txt",
	}
	out, err := decorated.Invoke(ctx, nil, inputs)
	if err != nil {
		t.Fatalf("decorated Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	seen := map[string]bool{}
	for i, ck := range chunks {
		id, _ := ck["id"].(string)
		if id == "" {
			t.Fatalf("chunk %d has empty id", i)
		}
		if seen[id] {
			t.Fatalf("chunk %d duplicates id %q — chunks would overwrite each other", i, id)
		}
		seen[id] = true
		cww, _ := ck["content_with_weight"].(string)
		if want := common.ChunkID(testDocID, cww); id != want {
			t.Errorf("chunk %d id = %q, want %q (xxhash of content_with_weight)", i, id, want)
		}
	}
}
