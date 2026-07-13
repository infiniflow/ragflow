package task

import (
	"math"
	"testing"
	"time"
)

// =============================================================================
// TruncateTexts — Python: tiktoken.cl100k_base token-level truncation
// =============================================================================

func TestTruncateTexts_TokenLevel(t *testing.T) {
	// Python: enc.encode("hello world") → [15339, 1917] (2 tokens)
	// With maxLength=12, safeMax=2, both tokens fit → unchanged
	result := TruncateTexts([]string{"hello world"}, 12)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	if result[0] != "hello world" {
		t.Errorf("with safeMax=2, 'hello world' (2 tokens) should fit, got %q", result[0])
	}
}

func TestTruncateTexts_TruncatesByToken(t *testing.T) {
	// Python: with safeMax=1, keep only 1 token → shorter than original
	result := TruncateTexts([]string{"hello world"}, 11)
	// safeMax = 11-10 = 1, keeps only 1 token
	if len(result[0]) >= len("hello world") || len(result[0]) == 0 {
		t.Errorf("safeMax=1 should produce shorter output, got %q", result[0])
	}
}

func TestTruncateTexts_EmptyText(t *testing.T) {
	result := TruncateTexts([]string{""}, 100)
	if len(result) != 1 || result[0] != "" {
		t.Errorf("empty text should remain empty, got %q", result[0])
	}
}

func TestTruncateTexts_NilInput(t *testing.T) {
	result := TruncateTexts(nil, 100)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

func TestTruncateTexts_EmptySlice(t *testing.T) {
	result := TruncateTexts([]string{}, 100)
	if len(result) != 0 {
		t.Errorf("expected empty slice, got len=%d", len(result))
	}
}

func TestTruncateTexts_MultipleTexts(t *testing.T) {
	texts := []string{"hello world", "short"}
	result := TruncateTexts(texts, 100)
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[0] != "hello world" {
		t.Errorf("first text should be unchanged")
	}
	if result[1] != "short" {
		t.Errorf("second text should be unchanged")
	}
}

// =============================================================================
// SplitQuestions — Python: str.split("\n"), keeps empty strings
// =============================================================================

func TestSplitQuestions_Basic(t *testing.T) {
	result := SplitQuestions("Q1\nQ2\nQ3")
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}
	if result[0] != "Q1" || result[1] != "Q2" || result[2] != "Q3" {
		t.Errorf("got %v, want [Q1 Q2 Q3]", result)
	}
}

func TestSplitQuestions_Empty(t *testing.T) {
	result := SplitQuestions("")
	// Python: "".split("\n") → [""]
	if len(result) != 1 || result[0] != "" {
		t.Errorf("got %v, want [\"\"]", result)
	}
}

func TestSplitQuestions_TrailingNewline(t *testing.T) {
	result := SplitQuestions("Q1\nQ2\n")
	// Python: "Q1\nQ2\n".split("\n") → ["Q1", "Q2", ""]
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3, got %v", len(result), result)
	}
	if result[2] != "" {
		t.Errorf("last element should be empty string, got %q", result[2])
	}
}

func TestSplitQuestions_NoNewline(t *testing.T) {
	result := SplitQuestions("single")
	if len(result) != 1 || result[0] != "single" {
		t.Errorf("got %v, want [single]", result)
	}
}

// =============================================================================
// SplitKeywords — Python: re.split(r"[,，;；、\r\n]+", ...)
// =============================================================================

func TestSplitKeywords_Comma(t *testing.T) {
	result := SplitKeywords("kw1,kw2,kw3")
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}
}

func TestSplitKeywords_ChineseComma(t *testing.T) {
	result := SplitKeywords("kw1，kw2，kw3")
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}
}

func TestSplitKeywords_MixedDelimiters(t *testing.T) {
	result := SplitKeywords("kw1,kw2；kw3")
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3, got %v", len(result), result)
	}
}

func TestSplitKeywords_FiltersEmptyStrings(t *testing.T) {
	result := SplitKeywords("kw1,,kw2")
	// Python: re.split → ["kw1", "", "kw2"] → after filter: ["kw1", "kw2"]
	if len(result) != 2 {
		t.Errorf("empty strings should be filtered: got %v", result)
	}
}

func TestSplitKeywords_Empty(t *testing.T) {
	result := SplitKeywords("")
	// Python: re.split on "" → [""], then filtered by if k.strip()
	if len(result) != 0 {
		t.Errorf("got %v, want empty", result)
	}
}

// =============================================================================
// CreateChunkTime — Python: datetime.now().timestamp() has sub-second precision
// =============================================================================

func TestCreateChunkTime_Format(t *testing.T) {
	timeStr, ts := CreateChunkTime()
	if timeStr == "" {
		t.Error("create_time should not be empty")
	}
	if len(timeStr) != 19 {
		t.Errorf("expected 19 chars (2006-01-02 15:04:05), got %q", timeStr)
	}
	// Python: datetime.now().timestamp() returns float with fractional part
	frac := ts - math.Floor(ts)
	if frac <= 0 {
		t.Errorf("timestamp should have sub-second precision, got frac=%f", frac)
	}
}

// =============================================================================
// RenameTextToContentWithWeight — Python processChunks logic
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
// ProcessChunksForPipeline — Python: processChunks()
// =============================================================================

func TestProcessChunksForPipeline_SetsDocIDAndKBID(t *testing.T) {
	chunks := []map[string]any{{"text": "hello world"}}
	_ = ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())

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
	_ = ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	if chunks[0]["docnm_kwd"] != "test-doc.pdf" {
		t.Errorf("docnm_kwd = %q, want \"test-doc.pdf\"", chunks[0]["docnm_kwd"])
	}
}

func TestProcessChunksForPipeline_SetsTimeFields(t *testing.T) {
	now := time.Now()
	chunks := []map[string]any{{"text": "hello"}}
	_ = ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", now)

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
	_ = ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	id, ok := chunks[0]["id"].(string)
	if !ok || id == "" {
		t.Errorf("id should be non-empty string, got %v", chunks[0]["id"])
	}
}

func TestProcessChunksForPipeline_NoPanicOnListText(t *testing.T) {
	chunks := []map[string]any{{"text": []any{"bad-shape"}}}
	res := ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	if res == nil {
		t.Errorf("should return valid result")
	}
}

func TestProcessChunksForPipeline_RemovesInternalPipelineFields(t *testing.T) {
	chunks := []map[string]any{{
		"text":           "hello",
		"_pdf_positions": []any{[]any{0, 1, 2, 3, 4}},
		"image":          "data:image/png;base64,abc",
	}}

	_ = ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	if _, exists := chunks[0]["_pdf_positions"]; exists {
		t.Fatalf("_pdf_positions should be removed before indexing: %v", chunks[0]["_pdf_positions"])
	}
	if _, exists := chunks[0]["image"]; exists {
		t.Fatalf("image should be removed before indexing: %v", chunks[0]["image"])
	}
}

func TestProcessChunksForPipeline_PreservesExistingID(t *testing.T) {
	chunks := []map[string]any{{"text": "hello", "id": "existing-id"}}
	_ = ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	if chunks[0]["id"] != "existing-id" {
		t.Errorf("existing id should be preserved, got %q", chunks[0]["id"])
	}
}

func TestProcessChunksForPipeline_QuestionsProcessing(t *testing.T) {
	chunks := []map[string]any{{"text": "hello", "questions": "Q1\nQ2\nQ3"}}
	_ = ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())

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
	if _, ok := chunks[0]["question_tks"].(string); !ok {
		t.Errorf("question_tks should be string, got %T", chunks[0]["question_tks"])
	}
}

func TestProcessChunksForPipeline_KeywordsProcessing(t *testing.T) {
	chunks := []map[string]any{{"text": "hello", "keywords": "kw1,kw2;kw3"}}
	_ = ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())

	if _, exists := chunks[0]["keywords"]; exists {
		t.Error("keywords key should be removed")
	}
	kwd, ok := chunks[0]["important_kwd"].([]string)
	if !ok || len(kwd) == 0 {
		t.Errorf("important_kwd should be non-empty []string, got %v", chunks[0]["important_kwd"])
	}
	if _, ok := chunks[0]["important_tks"].(string); !ok {
		t.Errorf("important_tks should be string, got %T", chunks[0]["important_tks"])
	}
}

func TestProcessChunksForPipeline_SummaryProcessing(t *testing.T) {
	chunks := []map[string]any{{"text": "hello", "summary": "This is a summary."}}
	_ = ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())

	if _, exists := chunks[0]["summary"]; exists {
		t.Error("summary key should be removed")
	}
	if _, ok := chunks[0]["content_ltks"].(string); !ok {
		t.Errorf("content_ltks should be string, got %T", chunks[0]["content_ltks"])
	}
	if _, ok := chunks[0]["content_sm_ltks"].(string); !ok {
		t.Errorf("content_sm_ltks should be string, got %T", chunks[0]["content_sm_ltks"])
	}
}

func TestProcessChunksForPipeline_TextRenamed(t *testing.T) {
	chunks := []map[string]any{{"text": "hello world"}}
	_ = ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())

	if _, exists := chunks[0]["text"]; exists {
		t.Error("text key should be removed")
	}
	if chunks[0]["content_with_weight"] != "hello world" {
		t.Errorf("content_with_weight = %q, want \"hello world\"", chunks[0]["content_with_weight"])
	}
}

func TestProcessChunksForPipeline_PreservesContentWithWeight(t *testing.T) {
	chunks := []map[string]any{{"content_with_weight": "already set", "text": "hello"}}
	_ = ProcessChunksForPipeline(chunks, "doc-1", "kb-1", "test-doc.pdf", time.Now())
	if chunks[0]["content_with_weight"] != "already set" {
		t.Errorf("content_with_weight = %q, want \"already set\"", chunks[0]["content_with_weight"])
	}
}
