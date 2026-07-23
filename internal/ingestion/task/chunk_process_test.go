package task

import (
	"testing"
	"time"
)

// =============================================================================
// RenameTextToContentWithWeight - Python processChunks logic
// =============================================================================

func TestRenameTextToContentWithWeight_Basic(t *testing.T) {
	chunk := map[string]any{"text": "hello world"}
	RenameTextToContentWithWeight(chunk)
	if _, exists := chunk["text"]; exists {
		t.Error("text key should be removed")
	}
	if chunk["content_with_weight"] != "hello world" {
		t.Errorf("content_with_weight = %q, want \"hello world\"", chunk["content_with_weight"])
	}
}

func TestRenameTextToContentWithWeight_PreservesExisting(t *testing.T) {
	chunk := map[string]any{"content_with_weight": "already set", "text": "hello"}
	RenameTextToContentWithWeight(chunk)
	if chunk["content_with_weight"] != "already set" {
		t.Errorf("preserved value should not be overwritten")
	}
	if _, exists := chunk["text"]; exists {
		t.Error("text should still be removed")
	}
}

func TestRenameTextToContentWithWeight_NoTextKey(t *testing.T) {
	chunk := map[string]any{"other": "value"}
	RenameTextToContentWithWeight(chunk)
	if _, exists := chunk["content_with_weight"]; exists {
		t.Error("should not add content_with_weight when no text key")
	}
}

// =============================================================================
// ProcessChunksForPipeline - Python: processChunks()
// =============================================================================

func TestProcessChunksForPipeline_SetsDocIDAndKBID(t *testing.T) {
	chunks := []map[string]any{{"text": "hello world"}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}

	if chunks[0]["doc_id"] != "doc-1" {
		t.Errorf("doc_id = %q, want \"doc-1\"", chunks[0]["doc_id"])
	}
	if kbIDs, ok := chunks[0]["kb_id"].([]string); ok {
		if len(kbIDs) != 1 || kbIDs[0] != "kb-1" {
			t.Errorf("kb_id = %v, want [\"kb-1\"]", chunks[0]["kb_id"])
		}
	} else {
		t.Errorf("kb_id should be []string, got %T", chunks[0]["kb_id"])
	}
}

func TestProcessChunksForPipeline_SetsDocNameKwd(t *testing.T) {
	chunks := []map[string]any{{"text": "hello"}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}
	if chunks[0]["docnm_kwd"] != "test-doc.pdf" {
		t.Errorf("docnm_kwd = %q, want \"test-doc.pdf\"", chunks[0]["docnm_kwd"])
	}
}

func TestProcessChunksForPipeline_SetsTimeFields(t *testing.T) {
	now := time.Now()
	chunks := []map[string]any{{"text": "hello"}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", now)
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}

	if timeStr, ok := chunks[0]["create_time"].(string); ok {
		if timeStr != now.Format("2006-01-02 15:04:05") {
			t.Errorf("create_time = %q, want %q", timeStr, now.Format("2006-01-02 15:04:05"))
		}
	} else {
		t.Errorf("create_time should be string, got %T", chunks[0]["create_time"])
	}
	if ts, ok := chunks[0]["create_timestamp_flt"].(float64); ok {
		expected := float64(now.UnixMicro()) / 1e6
		if ts != expected {
			t.Errorf("create_timestamp_flt = %f, want %f", ts, expected)
		}
	} else {
		t.Errorf("create_timestamp_flt should be float64, got %T", chunks[0]["create_timestamp_flt"])
	}
}

func TestProcessChunksForPipeline_GeneratesID(t *testing.T) {
	chunks := []map[string]any{{"text": "hello"}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}
	id, ok := chunks[0]["id"].(string)
	if !ok || id == "" {
		t.Errorf("id should be non-empty string, got %v", chunks[0]["id"])
	}
}

// TestProcessChunksForPipeline_GeneratesIDOnNonStringText pins the id fallback:
// when ck["id"] is absent and ck["text"] is a non-string (e.g. from a
// malformed input), the type assertion silently yields "" and
// component.ChunkID computes a valid id from empty text, rather than erroring.
func TestProcessChunksForPipeline_GeneratesIDOnNonStringText(t *testing.T) {
	chunks := []map[string]any{{"text": []any{"bad-shape"}}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}
	id, ok := chunks[0]["id"].(string)
	if !ok || id == "" {
		t.Errorf("id should be generated even for non-string text, got %v", chunks[0]["id"])
	}
}

func TestProcessChunksForPipeline_RemovesInternalPipelineFields(t *testing.T) {
	chunks := []map[string]any{{
		"text":           "hello",
		"_pdf_positions": []any{[]any{0, 1, 2, 3, 4}},
		"image":          "data:image/png;base64,abc",
	}}

	_, err := ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}
	if _, exists := chunks[0]["_pdf_positions"]; exists {
		t.Fatalf("_pdf_positions should be removed before indexing: %v", chunks[0]["_pdf_positions"])
	}
	if _, exists := chunks[0]["image"]; exists {
		t.Fatalf("image should be removed before indexing: %v", chunks[0]["image"])
	}
}

func TestProcessChunksForPipeline_PreservesExistingID(t *testing.T) {
	chunks := []map[string]any{{"text": "hello", "id": "existing-id"}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}
	if chunks[0]["id"] != "existing-id" {
		t.Errorf("existing id should be preserved, got %q", chunks[0]["id"])
	}
}

func TestProcessChunksForPipeline_QuestionsProcessing(t *testing.T) {
	chunks := []map[string]any{{"text": "hello", "questions": "Q1\nQ2\nQ3"}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}

	if _, exists := chunks[0]["questions"]; exists {
		t.Error("questions key should be removed")
	}
	kwd, ok := chunks[0]["question_kwd"].([]string)
	if !ok {
		t.Fatalf("question_kwd should be []string, got %T", chunks[0]["question_kwd"])
	}
	if len(kwd) != 3 {
		t.Errorf("question_kwd len = %d, want 3", len(kwd))
	}
	if _, ok := chunks[0]["question_tks"]; ok {
		t.Errorf("question_tks must NOT be produced by executor (owned by Tokenizer), got %T", chunks[0]["question_tks"])
	}
}

func TestProcessChunksForPipeline_KeywordsProcessing(t *testing.T) {
	chunks := []map[string]any{{"text": "hello", "keywords": "kw1,kw2;kw3"}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}

	if _, exists := chunks[0]["keywords"]; exists {
		t.Error("keywords key should be removed")
	}
	kwd, ok := chunks[0]["important_kwd"].([]string)
	if !ok || len(kwd) == 0 {
		t.Errorf("important_kwd should be non-empty []string, got %v", chunks[0]["important_kwd"])
	}
	if _, ok := chunks[0]["important_tks"]; ok {
		t.Errorf("important_tks must NOT be produced by executor (owned by Tokenizer), got %T", chunks[0]["important_tks"])
	}
}

func TestProcessChunksForPipeline_SummaryProcessing(t *testing.T) {
	chunks := []map[string]any{{"text": "hello", "summary": "This is a summary."}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}

	if _, exists := chunks[0]["summary"]; exists {
		t.Error("summary key should be removed")
	}
	if _, ok := chunks[0]["content_ltks"]; ok {
		t.Errorf("content_ltks must NOT be produced by executor (owned by Tokenizer), got %T", chunks[0]["content_ltks"])
	}
	if _, ok := chunks[0]["content_sm_ltks"]; ok {
		t.Errorf("content_sm_ltks must NOT be produced by executor (owned by Tokenizer), got %T", chunks[0]["content_sm_ltks"])
	}
}

// TestProcessChunksForPipeline_PreservesTokenizerProducedFields documents the
// Tokenizer-terminated contract: when the upstream Tokenizer already produced
// the _tks/_ltks/_kwd fields, the executor preserves them untouched and only
// strips the consumed source fields. The executor never re-tokenizes or
// overwrites Tokenizer output.
func TestProcessChunksForPipeline_PreservesTokenizerProducedFields(t *testing.T) {
	chunks := []map[string]any{{
		"text":            "hello",
		"questions":       "Q1\nQ2",
		"question_tks":    "tokenizer-output-tks",
		"question_kwd":    []string{"preset-q-kwd"},
		"keywords":        "kw1,kw2",
		"important_tks":   "tokenizer-output-itks",
		"important_kwd":   []string{"preset-i-kwd"},
		"summary":         "a summary",
		"content_ltks":    "tokenizer-output-ltks",
		"content_sm_ltks": "tokenizer-output-smltks",
	}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}

	// Consumed source fields are stripped.
	for _, k := range []string{"questions", "keywords", "summary"} {
		if _, exists := chunks[0][k]; exists {
			t.Errorf("%s should be removed (consumed by Tokenizer)", k)
		}
	}
	// Tokenizer-produced fields are preserved verbatim (not overwritten).
	if chunks[0]["question_tks"] != "tokenizer-output-tks" {
		t.Errorf("question_tks overwritten: %v", chunks[0]["question_tks"])
	}
	if chunks[0]["important_tks"] != "tokenizer-output-itks" {
		t.Errorf("important_tks overwritten: %v", chunks[0]["important_tks"])
	}
	if chunks[0]["content_ltks"] != "tokenizer-output-ltks" {
		t.Errorf("content_ltks overwritten: %v", chunks[0]["content_ltks"])
	}
	if chunks[0]["content_sm_ltks"] != "tokenizer-output-smltks" {
		t.Errorf("content_sm_ltks overwritten: %v", chunks[0]["content_sm_ltks"])
	}
	// Preset _kwd arrays are preserved (executor does not overwrite).
	if kwd, ok := chunks[0]["question_kwd"].([]string); !ok || len(kwd) != 1 || kwd[0] != "preset-q-kwd" {
		t.Errorf("question_kwd preset not preserved: %v", chunks[0]["question_kwd"])
	}
	if kwd, ok := chunks[0]["important_kwd"].([]string); !ok || len(kwd) != 1 || kwd[0] != "preset-i-kwd" {
		t.Errorf("important_kwd preset not preserved: %v", chunks[0]["important_kwd"])
	}
}

func TestProcessChunksForPipeline_TextRenamed(t *testing.T) {
	chunks := []map[string]any{{"text": "hello world"}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}

	if _, exists := chunks[0]["text"]; exists {
		t.Error("text key should be removed")
	}
	if chunks[0]["content_with_weight"] != "hello world" {
		t.Errorf("content_with_weight = %q, want \"hello world\"", chunks[0]["content_with_weight"])
	}
}

func TestProcessChunksForPipeline_PreservesContentWithWeight(t *testing.T) {
	chunks := []map[string]any{{"content_with_weight": "already set", "text": "hello"}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}
	if chunks[0]["content_with_weight"] != "already set" {
		t.Errorf("content_with_weight = %q, want \"already set\"", chunks[0]["content_with_weight"])
	}
}

func TestProcessChunkPositions_FlatFloat64(t *testing.T) {
	chunk := map[string]any{
		"positions": []float64{0, 100, 50, 200, 150},
	}
	processChunkPositions(chunk)

	if _, exists := chunk["positions"]; exists {
		t.Fatal("positions key must be removed")
	}
	pageNum := chunk["page_num_int"].([]int)
	if len(pageNum) != 1 || pageNum[0] != 1 {
		t.Errorf("page_num_int = %v, want [1]", pageNum)
	}
}

func TestProcessChunkPositions_2DFloat64(t *testing.T) {
	chunk := map[string]any{
		"positions": [][]float64{
			{0, 100, 50, 200, 150},
			{1, 200, 60, 300, 250},
		},
	}
	processChunkPositions(chunk)

	if _, exists := chunk["positions"]; exists {
		t.Fatal("positions key must be removed")
	}
	pageNum := chunk["page_num_int"].([]int)
	if len(pageNum) != 2 || pageNum[0] != 1 || pageNum[1] != 2 {
		t.Errorf("page_num_int = %v, want [1 2]", pageNum)
	}
	top := chunk["top_int"].([]int)
	if len(top) != 2 || top[0] != 200 || top[1] != 300 {
		t.Errorf("top_int = %v, want [200 300]", top)
	}
}

func TestProcessChunkPositions_NoPositions(t *testing.T) {
	chunk := map[string]any{"text": "hello"}
	processChunkPositions(chunk)
	if _, exists := chunk["page_num_int"]; exists {
		t.Error("page_num_int must not be set when positions is missing")
	}
}
