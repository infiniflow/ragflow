package service

import (
	"testing"
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
	// Verify ID appears
	if !contains(result, "ID: abc") {
		t.Errorf("missing ID line: %s", result)
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
	entryTokens := NumTokensFromString(formatChunkEntry(chunks[0]))
	maxToks := int(float64(entryTokens+1) / 0.97) // just enough for first
	result := KbPrompt(chunks, maxToks)
	if !contains(result, "ID: 1") {
		t.Error("first chunk should be included")
	}
	if contains(result, "ID: 2") {
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



func TestNumTokensFromString_Empty(t *testing.T) {
	if got := NumTokensFromString(""); got != 0 {
		t.Errorf("expected 0 for empty string, got %d", got)
	}
}

func TestNumTokensFromString_Positive(t *testing.T) {
	// Either the C++ tokenizer or the fallback must return > 0 for
	// non-empty text.  The exact count depends on the environment.
	for _, s := range []string{"hello world", "你好世界"} {
		if got := NumTokensFromString(s); got <= 0 {
			t.Errorf("NumTokensFromString(%q) = %d, want >0", s, got)
		}
	}
}

func TestKbPrompt_TokenLimitAccurate(t *testing.T) {
	// Verify truncation uses NumTokensFromString by computing the limit
	// dynamically from the actual token count (works in both fallback
	// and C++ tokenizer environments).
	chunks := []SourcedChunk{
		{ID: "1", Content: "hello"},
		{ID: "2", Content: "world"},
	}
	entryTokens := NumTokensFromString(formatChunkEntry(chunks[0]))
	maxToks := int(float64(entryTokens+1) / 0.97) // just enough for first entry
	result := KbPrompt(chunks, maxToks)
	if !contains(result, "ID: 1") {
		t.Error("first chunk should fit")
	}
	if contains(result, "ID: 2") {
		t.Errorf("second chunk should be excluded: result = %q", result)
	}
}

func TestKbPrompt_AllFit(t *testing.T) {
	chunks := []SourcedChunk{
		{ID: "1", Content: "a"},
		{ID: "2", Content: "b"},
	}
	result := KbPrompt(chunks, 1000)
	if !contains(result, "ID: 1") || !contains(result, "ID: 2") {
		t.Error("both chunks should fit under generous limit")
	}
}


