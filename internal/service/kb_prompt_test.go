package service

import (
	"strings"
	"testing"

	"ragflow/internal/tokenizer"
)

func TestKbPrompt_Empty(t *testing.T) {
	if got := KbPrompt(nil, 100); got != "" {
		t.Errorf("expected empty for nil chunks")
	}
	if got := KbPrompt([]SourcedChunk{}, 100); got != "" {
		t.Errorf("expected empty for empty chunks")
	}
	if got := KbPrompt([]SourcedChunk{{Content: "x"}}, 0); got != "" {
		t.Errorf("expected empty for maxTokens=0")
	}
	if got := KbPrompt([]SourcedChunk{{Content: "x"}}, -1); got != "" {
		t.Errorf("expected empty for maxTokens=-1")
	}
}

func TestKbPrompt_Format(t *testing.T) {
	chunks := []SourcedChunk{{
		ID:      "abc",
		Content: "chunk content here",
		DocName: "Test Document",
		URL:     "http://example.com",
	}}
	result := KbPrompt(chunks, 10000)
	if result == "" {
		t.Fatal("expected non-empty prompt")
	}
	// The ID line carries the chunk's position, not its chunk id: the client
	// resolves a citation marker by indexing the reference with that number.
	if !contains(result, "ID: 0") {
		t.Errorf("missing ID line: %s", result)
	}
	if contains(result, "ID: abc") {
		t.Errorf("the block id must be the chunk position, not the chunk id: %s", result)
	}
	// Verify title
	if !contains(result, "Title: Test Document") {
		t.Errorf("missing title: %s", result)
	}
	// Verify URL
	if !contains(result, "URL: http://example.com") {
		t.Errorf("missing URL: %s", result)
	}
	// Verify content
	if !contains(result, "chunk content here") {
		t.Errorf("missing content: %s", result)
	}
	// Verify unicode box-drawing chars
	if !contains(result, "├──") {
		t.Errorf("missing tree drawing: %s", result)
	}
}

func TestKbPrompt_TokenLimit(t *testing.T) {
	chunks := []SourcedChunk{
		{ID: "1", Content: "a very long content that takes many tokens "},
		{ID: "2", Content: "second chunk content here"},
	}
	// Compute limit dynamically so the test works with both the C++
	// tokenizer and the rune-based fallback.
	entryTokens := tokenizer.NumTokensFromString(formatChunkEntry(chunks[0], 0))
	maxToks := int(float64(entryTokens+1) / 0.97) // just enough for first
	result := KbPrompt(chunks, maxToks)
	if !contains(result, "ID: 0") {
		t.Error("first chunk should be included")
	}
	if contains(result, "ID: 1") {
		t.Error("second chunk should be excluded under tight limit")
	}
}

func TestKbPrompt_DocMetadata(t *testing.T) {
	chunks := []SourcedChunk{{
		ID:      "abc",
		Content: "content",
		DocumentMetadata: map[string]interface{}{
			"author": "test author",
			"year":   "2024",
		},
	}}
	result := KbPrompt(chunks, 10000)
	if !contains(result, "author: test author") {
		t.Errorf("missing metadata author: %s", result)
	}
	if !contains(result, "year: 2024") {
		t.Errorf("missing metadata year: %s", result)
	}
}

func TestKbPrompt_NoDocNameOrURL(t *testing.T) {
	chunks := []SourcedChunk{{
		ID:      "simple",
		Content: "plain content",
	}}
	result := KbPrompt(chunks, 10000)
	if contains(result, "Title:") {
		t.Error("should not have title when empty")
	}
	if contains(result, "URL:") {
		t.Error("should not have URL when empty")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestKbPrompt_TokenLimitAccurate(t *testing.T) {
	// Verify truncation uses tokenizer.NumTokensFromString by computing the limit
	// dynamically from the actual token count (works in both fallback
	// and C++ tokenizer environments).
	chunks := []SourcedChunk{
		{ID: "1", Content: "hello"},
		{ID: "2", Content: "world"},
	}
	entryTokens := tokenizer.NumTokensFromString(formatChunkEntry(chunks[0], 0))
	maxToks := int(float64(entryTokens+1) / 0.97) // just enough for first entry
	result := KbPrompt(chunks, maxToks)
	if !contains(result, "ID: 0") {
		t.Error("first chunk should fit")
	}
	if contains(result, "ID: 1") {
		t.Errorf("second chunk should be excluded: result = %q", result)
	}
}

func TestKbPrompt_AllFit(t *testing.T) {
	chunks := []SourcedChunk{
		{ID: "1", Content: "a"},
		{ID: "2", Content: "b"},
	}
	result := KbPrompt(chunks, 1000)
	if !contains(result, "ID: 0") || !contains(result, "ID: 1") {
		t.Error("both chunks should fit under generous limit")
	}
}

// TestKBPromptRendersPythonBlockShape pins the block shape to Python's
// _kb_block() (rag/prompts/generator.py:140): ID, Title, URL, metadata lines and
// Content, with embedded newlines in the single-line fields collapsed to spaces.
func TestKBPromptRendersPythonBlockShape(t *testing.T) {
	svc := &ChatPipelineService{}
	chunks := []map[string]interface{}{
		{
			"id":                  "a",
			"docnm_kwd":           "规范.docx",
			"content_with_weight": "正文",
		},
		{
			"id":                  "b",
			"docnm_kwd":           "多行\n标题.docx",
			"url":                 "https://example.com/x\n",
			"document_metadata":   map[string]interface{}{"author": "a\nb"},
			"content_with_weight": "第二块",
		},
	}

	blocks := svc.kbPrompt(map[string]interface{}{"chunks": chunks}, 8000)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}

	if want := "\nID: 0\n├── Title: 规范.docx\n└── Content:\n正文"; blocks[0] != want {
		t.Errorf("block 0 = %q, want %q", blocks[0], want)
	}

	second := blocks[1]
	for _, want := range []string{
		"\nID: 1\n",
		"├── Title: 多行 标题.docx\n",
		"├── URL: https://example.com/x \n",
		"├── author: a b\n",
		"└── Content:\n第二块",
	} {
		if !strings.Contains(second, want) {
			t.Errorf("block 1 = %q, missing %q", second, want)
		}
	}
	if strings.Contains(second, "\n标题.docx") || strings.Contains(second, "a\nb") {
		t.Errorf("block 1 kept raw newlines in a single-line field: %q", second)
	}
}
