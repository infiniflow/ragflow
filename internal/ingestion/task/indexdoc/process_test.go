package indexdoc

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

func TestProcessChunksForPipeline_SetsDocID(t *testing.T) {
	chunks := []map[string]any{{"text": "hello world"}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}

	if chunks[0]["doc_id"] != "doc-1" {
		t.Errorf("doc_id = %q, want \"doc-1\"", chunks[0]["doc_id"])
	}
	// kb_id is intentionally NOT set here: it is owned by the search engine at
	// the write boundary (ES/Infinity InsertChunks), not by ingestion. See #17371.
	if _, exists := chunks[0]["kb_id"]; exists {
		t.Errorf("kb_id should not be set by ProcessChunksForPipeline, got %v", chunks[0]["kb_id"])
	}
}

func TestProcessChunksForPipeline_SetsDocNameKwd(t *testing.T) {
	chunks := []map[string]any{{"text": "hello"}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", time.Now())
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
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", now)
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
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", time.Now())
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
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}
	id, ok := chunks[0]["id"].(string)
	if !ok || id == "" {
		t.Errorf("id should be generated even for non-string text, got %v", chunks[0]["id"])
	}
}

// TestProcessChunksForPipeline_RemovesInternalPipelineFields pins that
// processChunkPositions prunes the _pdf_positions internal field (the
// parser-emitted position matrix) before indexing. The "image" field is
// no longer dropped here — its lifecycle is owned by the chunker's
// imageUploadDecorator (register.go + image_upload.go), which uploads and
// deletes it at the chunker stage.
func TestProcessChunksForPipeline_RemovesInternalPipelineFields(t *testing.T) {
	chunks := []map[string]any{{
		"text":           "hello",
		"_pdf_positions": []any{[]any{0, 1, 2, 3, 4}},
	}}

	_, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}
	if _, exists := chunks[0]["_pdf_positions"]; exists {
		t.Fatalf("_pdf_positions should be removed before indexing: %v", chunks[0]["_pdf_positions"])
	}
}

func TestProcessChunksForPipeline_PreservesExistingID(t *testing.T) {
	chunks := []map[string]any{{"text": "hello", "id": "existing-id"}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}
	if chunks[0]["id"] != "existing-id" {
		t.Errorf("existing id should be preserved, got %q", chunks[0]["id"])
	}
}

func TestProcessChunksForPipeline_QuestionsProcessing(t *testing.T) {
	chunks := []map[string]any{{"text": "hello", "questions": "Q1\nQ2\nQ3"}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", time.Now())
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

// TestProcessChunksForPipeline_MetadataMapAggregated pins the normal contract:
// ck["metadata"] produced by the Extractor (the merge of enable_metadata +
// field_name="metadata") is a map[string]any and is aggregated into the
// returned doc-level metadata.
func TestProcessChunksForPipeline_MetadataMapAggregated(t *testing.T) {
	chunks := []map[string]any{
		{"text": "hello", "metadata": map[string]any{"category": "finance", "region": "east"}},
	}
	metadata, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}
	if metadata["category"] != "finance" {
		t.Errorf("category = %v, want finance", metadata["category"])
	}
	if metadata["region"] != "east" {
		t.Errorf("region = %v, want east", metadata["region"])
	}
	// The consumed metadata key must not leak onto the persisted chunk.
	if _, exists := chunks[0]["metadata"]; exists {
		t.Error("metadata key should be removed from the chunk after aggregation")
	}
}

// TestProcessChunksForPipeline_MetadataNonMapDropped pins the strict contract:
// ck["metadata"] is Extractor-owned and always a map[string]any. A non-map
// value (e.g. a JSON string, as field_name="metadata" used to emit before the
// extractor unified to map) is a contract violation — it is dropped with a
// warning, never guess-parsed, so an upstream bug surfaces instead of silently
// producing document metadata.
func TestProcessChunksForPipeline_MetadataNonMapDropped(t *testing.T) {
	for name, value := range map[string]any{
		"json_string": `{"category":"finance","region":"east"}`,
		"fenced":      "```json\n{\"category\":\"law\"}\n```",
		"not_json":    "this is not json",
	} {
		t.Run(name, func(t *testing.T) {
			chunks := []map[string]any{{"text": "hello", "metadata": value}}
			metadata, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", time.Now())
			if err != nil {
				t.Fatalf("ProcessChunksForPipeline: %v", err)
			}
			if len(metadata) != 0 {
				t.Errorf("metadata = %v, want empty (non-map metadata dropped)", metadata)
			}
			if _, exists := chunks[0]["metadata"]; exists {
				t.Error("metadata key should be removed from the chunk after aggregation")
			}
		})
	}
}

func TestProcessChunksForPipeline_KeywordsProcessing(t *testing.T) {
	chunks := []map[string]any{{"text": "hello", "keywords": "kw1,kw2;kw3"}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", time.Now())
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
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", time.Now())
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
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", time.Now())
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
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", time.Now())
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
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}
	if chunks[0]["content_with_weight"] != "already set" {
		t.Errorf("content_with_weight = %q, want \"already set\"", chunks[0]["content_with_weight"])
	}
}

func TestProcessChunkPositions_FlatFloat64(t *testing.T) {
	chunk := map[string]any{
		// positions is 1-indexed (parser normalized before we see it)
		"positions": []float64{1, 100, 50, 200, 150},
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
			{1, 100, 50, 200, 150},
			{2, 200, 60, 300, 250},
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
	// _pdf_positions is pruned unconditionally, even on the early-return path
	// where "positions" is absent, since the two fields are independent.
	chunk := map[string]any{
		"text":           "hello",
		"_pdf_positions": []any{[]any{0, 1, 2, 3, 4}},
	}
	processChunkPositions(chunk)
	if _, exists := chunk["page_num_int"]; exists {
		t.Error("page_num_int must not be set when positions is missing")
	}
	if _, exists := chunk["_pdf_positions"]; exists {
		t.Error("_pdf_positions must be pruned even when positions is missing")
	}
}

// TestCleanupConsumedChunkFields_ImportantKwdMultiDelimiter pins the executor
// fallback's important_kwd materialization. When the Tokenizer component did
// NOT pre-produce important_kwd, the executor falls back to
// utility.SplitKeywords, which splits on the full delimiter set
// (ASCII + CJK comma/semicolon/ideographic-comma/newline) and DROPS empty
// parts. This is intentionally different from the Tokenizer component path
// (internal/ingestion/component/tokenizer.go:690), which splits on the ENGLISH
// COMMA ONLY and PRESERVES empty elements to match the DSL
// (rag/flow/tokenizer/tokenizer.py:153 `keywords.split(",")`).
//
// The two layers deliberately diverge: the component aligns to the DSL keyword
// contract ("delimited by ENGLISH COMMA"); the executor fallback mirrors
// Python task_executor.run_dataflow:879 and tolerates mixed delimiters from
// older upstream producers. Neither side should be "unified" to the other —
// changing one without the other silently breaks the documented parity
// boundary. The component-side half of this contract is locked by
// TestTokenizerComponent_ImportantKwd_CommaOnly in the component package.
func TestCleanupConsumedChunkFields_ImportantKwdMultiDelimiter(t *testing.T) {
	ck := map[string]any{"text": "hello", "keywords": "kw1,kw2;kw3，kw4"}

	cleanupConsumedChunkFields(ck)

	kwd, ok := ck["important_kwd"].([]string)
	if !ok {
		t.Fatalf("important_kwd should be []string, got %T", ck["important_kwd"])
	}
	// Executor fallback splits on comma/semicolon/CJK-comma and drops empties:
	// "kw1,kw2;kw3，kw4" -> ["kw1","kw2","kw3","kw4"], NOT the component's
	// ["kw1","kw2;kw3，kw4"].
	want := []string{"kw1", "kw2", "kw3", "kw4"}
	if len(kwd) != len(want) {
		t.Fatalf("executor important_kwd = %v, want %v (multi-delimiter, empties dropped)", kwd, want)
	}
	for i := range want {
		if kwd[i] != want[i] {
			t.Errorf("executor important_kwd[%d] = %q, want %q", i, kwd[i], want[i])
		}
	}
	if _, exists := ck["keywords"]; exists {
		t.Error("keywords source field should be consumed/removed")
	}
}

// TestCleanupConsumedChunkFields_ImportantKwdDropsEmptyParts documents that the
// executor fallback drops empty parts (e.g. the middle empty token in
// "a,,b"), diverging from the component path which PRESERVES it as ["a","","b"].
// Together with the component CommaOnly test this locks the intentional
// divergence: same input, different important_kwd arrays per layer.
func TestCleanupConsumedChunkFields_ImportantKwdDropsEmptyParts(t *testing.T) {
	ck := map[string]any{"text": "hello", "keywords": "a,,b"}

	cleanupConsumedChunkFields(ck)

	kwd, ok := ck["important_kwd"].([]string)
	if !ok {
		t.Fatalf("important_kwd should be []string, got %T", ck["important_kwd"])
	}
	// Executor drops the empty middle part: ["a","b"], NOT ["a","","b"].
	want := []string{"a", "b"}
	if len(kwd) != len(want) || kwd[0] != "a" || kwd[1] != "b" {
		t.Fatalf("executor important_kwd = %v, want %v (empty parts dropped)", kwd, want)
	}
}
