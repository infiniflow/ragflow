package task

import (
	"testing"
)

// =============================================================================
// NormalizeChunks
// =============================================================================

func TestNormalizeChunks_ChunksFormat(t *testing.T) {
	input := map[string]any{
		"chunks": []map[string]any{
			{"text": "hello", "doc_type_kwd": "text"},
			{"text": "world", "doc_type_kwd": "text"},
		},
	}
	result := NormalizeChunks(input)
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[0]["text"] != "hello" {
		t.Errorf("result[0][\"text\"] = %q, want \"hello\"", result[0]["text"])
	}
}

func TestNormalizeChunks_JSONFormat(t *testing.T) {
	input := map[string]any{
		"json": []map[string]any{
			{"text": "section 1", "doc_type_kwd": "text"},
		},
	}
	result := NormalizeChunks(input)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	if result[0]["text"] != "section 1" {
		t.Errorf("result[0][\"text\"] = %q, want \"section 1\"", result[0]["text"])
	}
}

func TestNormalizeChunks_JSONFormatFromGenericSlice(t *testing.T) {
	input := map[string]any{
		"json": []any{
			map[string]any{"text": "section 1", "doc_type_kwd": "text"},
		},
	}
	result := NormalizeChunks(input)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	if result[0]["text"] != "section 1" {
		t.Errorf("result[0][\"text\"] = %q, want \"section 1\"", result[0]["text"])
	}
}

func TestNormalizeChunks_MarkdownFormat(t *testing.T) {
	input := map[string]any{
		"markdown": "# Title\n\nContent",
	}
	result := NormalizeChunks(input)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	text, ok := result[0]["text"].(string)
	if !ok {
		t.Fatalf("text should be string for markdown format, got %T", result[0]["text"])
	}
	if text != "# Title\n\nContent" {
		t.Errorf("text = %q, want \"# Title\\n\\nContent\"", text)
	}
}

func TestNormalizeChunks_TextFormat(t *testing.T) {
	input := map[string]any{
		"text": "plain text",
	}
	result := NormalizeChunks(input)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	text, ok := result[0]["text"].(string)
	if !ok {
		t.Fatalf("text should be string for text format, got %T", result[0]["text"])
	}
	if text != "plain text" {
		t.Errorf("text = %q, want \"plain text\"", text)
	}
}

func TestNormalizeChunks_HTMLFormat(t *testing.T) {
	input := map[string]any{
		"html": "<p>Hello</p>",
	}
	result := NormalizeChunks(input)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	text, ok := result[0]["text"].(string)
	if !ok {
		t.Fatalf("text should be string for html format, got %T", result[0]["text"])
	}
	if text != "<p>Hello</p>" {
		t.Errorf("text = %q, want \"<p>Hello</p>\"", text)
	}
}

func TestNormalizeChunks_EmptyOutput(t *testing.T) {
	result := NormalizeChunks(map[string]any{})
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestNormalizeChunks_EmptyMarkdown(t *testing.T) {
	result := NormalizeChunks(map[string]any{"markdown": ""})
	if result != nil {
		t.Errorf("expected nil for empty markdown, got %v", result)
	}
}

func TestNormalizeChunks_EmptyText(t *testing.T) {
	result := NormalizeChunks(map[string]any{"text": ""})
	if result != nil {
		t.Errorf("expected nil for empty text, got %v", result)
	}
}

func TestNormalizeChunks_EmptyHTML(t *testing.T) {
	result := NormalizeChunks(map[string]any{"html": ""})
	if result != nil {
		t.Errorf("expected nil for empty html, got %v", result)
	}
}

func TestNormalizeChunks_EmptyChunksList(t *testing.T) {
	result := NormalizeChunks(map[string]any{"chunks": []map[string]any{}})
	if len(result) != 0 {
		t.Errorf("expected empty slice, got len=%d", len(result))
	}
}

func TestNormalizeChunks_EmptyJSONList(t *testing.T) {
	result := NormalizeChunks(map[string]any{"json": []map[string]any{}})
	if len(result) != 0 {
		t.Errorf("expected empty slice, got len=%d", len(result))
	}
}

func TestNormalizeChunks_Priority(t *testing.T) {
	t.Run("chunks over json", func(t *testing.T) {
		input := map[string]any{
			"chunks": []map[string]any{{"text": "from chunks"}},
			"json":   []map[string]any{{"text": "from json"}},
		}
		result := NormalizeChunks(input)
		if result[0]["text"] != "from chunks" {
			t.Errorf("chunks should win: got %q", result[0]["text"])
		}
	})

	t.Run("json over markdown", func(t *testing.T) {
		input := map[string]any{
			"json":     []map[string]any{{"text": "from json"}},
			"markdown": "from markdown",
		}
		result := NormalizeChunks(input)
		if result[0]["text"] != "from json" {
			t.Errorf("json should win: got %q", result[0]["text"])
		}
	})

	t.Run("markdown over text", func(t *testing.T) {
		input := map[string]any{
			"markdown": "from markdown",
			"text":     "from text",
		}
		result := NormalizeChunks(input)
		text, ok := result[0]["text"].(string)
		if !ok {
			t.Fatalf("text should be string, got %T", result[0]["text"])
		}
		if text != "from markdown" {
			t.Errorf("markdown should win: got %q", text)
		}
	})
}

func TestNormalizeChunks_DoesNotMutateInput(t *testing.T) {
	original := []map[string]any{{"text": "original"}}
	input := map[string]any{"chunks": original}
	result := NormalizeChunks(input)
	result[0]["text"] = "modified"
	if original[0]["text"] != "original" {
		t.Error("should deep copy, not mutate input")
	}
}

func TestNormalizeChunks_DeepCopyVectors(t *testing.T) {
	// Python: copy.deepcopy creates fully independent copies.
	// Mutating a slice element in the result must NOT affect the original.
	original_vec := []float64{0.1, 0.2, 0.3}
	original := []map[string]any{{"text": "hello", "q_3_vec": original_vec}}
	input := map[string]any{"chunks": original}
	result := NormalizeChunks(input)
	// Mutate the slice *element* in-place (not replace the slice)
	result[0]["q_3_vec"].([]float64)[0] = 0.9
	// Original must be unchanged
	if original[0]["q_3_vec"].([]float64)[0] != 0.1 {
		t.Error("mutating result vector element should not affect original")
	}
}

func TestNormalizeChunks_NilInput(t *testing.T) {
	result := NormalizeChunks(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

// =============================================================================
// PrepareTextsForDataflowEmbedding
// =============================================================================

func TestPrepareTexts_QuestionsPriority(t *testing.T) {
	chunks := []map[string]any{
		{"questions": "Q1\nQ2", "summary": "a summary", "text": "plain text"},
	}
	result := PrepareTextsForDataflowEmbedding(chunks)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	if result[0] != "Q1\nQ2" {
		t.Errorf("questions should take priority: got %q", result[0])
	}
}

func TestPrepareTexts_SummaryFallback(t *testing.T) {
	chunks := []map[string]any{
		{"summary": "a summary", "text": "plain text"},
	}
	result := PrepareTextsForDataflowEmbedding(chunks)
	if result[0] != "a summary" {
		t.Errorf("summary should be used when no questions: got %q", result[0])
	}
}

func TestPrepareTexts_TextFallback(t *testing.T) {
	chunks := []map[string]any{
		{"text": "plain text"},
	}
	result := PrepareTextsForDataflowEmbedding(chunks)
	if result[0] != "plain text" {
		t.Errorf("text should be used when no questions/summary: got %q", result[0])
	}
}

func TestPrepareTexts_EmptyStringFallback(t *testing.T) {
	chunks := []map[string]any{
		{"text": ""},
	}
	result := PrepareTextsForDataflowEmbedding(chunks)
	if result[0] != "" {
		t.Errorf("expected empty string, got %q", result[0])
	}
}

func TestPrepareTexts_MultipleChunks(t *testing.T) {
	chunks := []map[string]any{
		{"questions": "Q1", "text": "t1"},
		{"summary": "S2", "text": "t2"},
		{"text": "t3"},
	}
	result := PrepareTextsForDataflowEmbedding(chunks)
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}
	if result[0] != "Q1" {
		t.Errorf("result[0] = %q, want \"Q1\"", result[0])
	}
	if result[1] != "S2" {
		t.Errorf("result[1] = %q, want \"S2\"", result[1])
	}
	if result[2] != "t3" {
		t.Errorf("result[2] = %q, want \"t3\"", result[2])
	}
}

func TestPrepareTexts_NilChunks(t *testing.T) {
	result := PrepareTextsForDataflowEmbedding(nil)
	if result != nil {
		t.Errorf("expected nil for nil chunks, got %v", result)
	}
}

func TestPrepareTexts_EmptyChunks(t *testing.T) {
	result := PrepareTextsForDataflowEmbedding([]map[string]any{})
	if len(result) != 0 {
		t.Errorf("expected empty slice, got len=%d", len(result))
	}
}

func TestPrepareTexts_MissingTextKey(t *testing.T) {
	chunks := []map[string]any{
		{"other_key": "value"},
	}
	result := PrepareTextsForDataflowEmbedding(chunks)
	if result[0] != "" {
		t.Errorf("expected empty string for missing text key, got %q", result[0])
	}
}

func TestPrepareTexts_NonStringTextReturnsEmpty(t *testing.T) {
	chunks := []map[string]any{
		{"text": []any{"bad-shape"}},
	}
	result := PrepareTextsForDataflowEmbedding(chunks)
	if result[0] != "" {
		t.Errorf("expected empty string for non-string text, got %q", result[0])
	}
}

func TestMustGetChunkTextString_NonStringReturnsEmpty(t *testing.T) {
	chunk := map[string]any{"text": []string{"bad-shape"}}
	result := MustGetChunkTextString(chunk, "unit-test")
	if result != "" {
		t.Errorf("expected empty string for non-string text, got %q", result)
	}
}

func TestGetChunkTextString_NonStringReturnsError(t *testing.T) {
	chunk := map[string]any{"text": []string{"bad-shape"}}
	_, err := GetChunkTextString(chunk, "unit-test")
	if err == nil {
		t.Error("expected error for non-string text")
	}
}

// =============================================================================
// AttachVectors
// =============================================================================

func TestAttachVectors_Basic(t *testing.T) {
	chunks := []map[string]any{
		{"text": "hello"},
		{"text": "world"},
	}
	vectors := [][]float64{
		{0.1, 0.2, 0.3},
		{0.4, 0.5, 0.6},
	}
	dim := AttachVectors(chunks, vectors)
	if dim != 3 {
		t.Errorf("dim = %d, want 3", dim)
	}
	if len(chunks[0]) == 0 || chunks[0]["q_3_vec"] == nil {
		t.Errorf("expected q_3_vec in chunk[0], got %v", chunks[0])
	}
	vec0 := chunks[0]["q_3_vec"].([]float64)
	if len(vec0) != 3 || vec0[0] != 0.1 {
		t.Errorf("chunk[0] vector = %v, want [0.1 0.2 0.3]", vec0)
	}
	vec1 := chunks[1]["q_3_vec"].([]float64)
	if len(vec1) != 3 || vec1[2] != 0.6 {
		t.Errorf("chunk[1] vector = %v, want [0.4 0.5 0.6]", vec1)
	}
}

func TestAttachVectors_EmptyChunks(t *testing.T) {
	result := AttachVectors(nil, nil)
	if result != 0 {
		t.Errorf("expected 0 for empty chunks, got %d", result)
	}
}

func TestAttachVectors_DifferentDimensions(t *testing.T) {
	chunks := []map[string]any{
		{"text": "a"},
		{"text": "b"},
	}
	vectors := [][]float64{
		{0.1, 0.2},
		{0.3, 0.4, 0.5},
	}
	// Each chunk gets its own vector with key based on its dimension
	AttachVectors(chunks, vectors)
	if chunks[0]["q_2_vec"] == nil {
		t.Errorf("chunk[0] should have q_2_vec, got keys: %v", chunkKeys(chunks[0]))
	}
	if chunks[1]["q_3_vec"] == nil {
		t.Errorf("chunk[1] should have q_3_vec, got keys: %v", chunkKeys(chunks[1]))
	}
}

func TestAttachVectors_MismatchedCount(t *testing.T) {
	chunks := []map[string]any{
		{"text": "hello"},
	}
	vectors := [][]float64{
		{0.1},
		{0.2},
	}
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for mismatched count")
		}
	}()
	AttachVectors(chunks, vectors)
}

func TestAttachVectors_KeyFormat(t *testing.T) {
	chunks := []map[string]any{
		{"text": "hello"},
	}
	vectors := [][]float64{
		{0.1, 0.2, 0.3, 0.4, 0.5},
	}
	AttachVectors(chunks, vectors)
	key := "q_5_vec"
	if chunks[0][key] == nil {
		t.Errorf("expected key %q in chunk, got keys: %v", key, chunkKeys(chunks[0]))
	}
}

func chunkKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
