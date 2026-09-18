package indexdoc

import (
	"reflect"
	"testing"
	"time"

	"ragflow/internal/common"
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
	if chunk["content_with_weight"] != "hello" {
		t.Errorf("text must be authoritative at the storage boundary, got %q", chunk["content_with_weight"])
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

// TestProcessChunksForPipeline_RejectsNonStringText pins the strict contract:
// non-string text must fail before chunk-id generation.
func TestProcessChunksForPipeline_RejectsNonStringText(t *testing.T) {
	chunks := []map[string]any{{"text": []any{"bad-shape"}}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", time.Now())
	if err == nil {
		t.Fatal("ProcessChunksForPipeline should reject non-string text")
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

func TestProcessChunksForPipeline_TextAuthoritativeAtRename(t *testing.T) {
	chunks := []map[string]any{{"content_with_weight": "already set", "text": "hello"}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}
	if chunks[0]["content_with_weight"] != "hello" {
		t.Errorf("content_with_weight = %q, want %q", chunks[0]["content_with_weight"], "hello")
	}
}

func TestProcessChunksForPipeline_RejectsMissingText(t *testing.T) {
	chunks := []map[string]any{{"content_with_weight": "already set"}}
	_, err := ProcessChunksForPipeline(chunks, "doc-1", "test-doc.pdf", time.Now())
	if err == nil {
		t.Fatal("ProcessChunksForPipeline should reject chunks without text")
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

// TestProcessChunksForPipeline_StripsPipelineOnlyFields pins the index boundary
// against the parser/chunker bookkeeping keys: none of them is a chunk-store
// column, and a strict engine rejects the whole insert over one of them
// ("Column ck_type not found in table", InfinityException 3013). ES only
// swallowed them because its mapping is dynamic.
func TestProcessChunksForPipeline_StripsPipelineOnlyFields(t *testing.T) {
	ck := map[string]any{
		"text": "hello",
		// Every bookkeeping key the chunkers/parsers can leave on a chunk.
		"ck_type": "text", "tk_nums": 3, "layout": "text", "layout_type": "text",
		"layoutno": "0", "image": "data:image/png;base64,AAAA",
		"context_above": "above", "context_below": "below", "page_number": 2,
		"table_id": "t1", "sheet": "s1", "sheet_index": 0,
		"headers": []string{"h"}, "cells": []string{"c"},
		"row_start": 0, "row_end": 1, "col_start": 0, "col_end": 1,
	}

	if _, err := ProcessChunksForPipeline([]map[string]any{ck}, "doc-1", "Doc", time.Now()); err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}

	for _, key := range pipelineOnlyFields {
		if _, exists := ck[key]; exists {
			t.Errorf("%q must be stripped before persist (no chunk column; a strict engine rejects the insert)", key)
		}
	}
	if ck["content_with_weight"] != "hello" {
		t.Errorf("content_with_weight = %v, want the chunk text (the strip must not touch persist fields)", ck["content_with_weight"])
	}
	if ck["doc_id"] != "doc-1" {
		t.Errorf("doc_id = %v, want doc-1 (the strip must not touch persist fields)", ck["doc_id"])
	}
}

// =============================================================================
// Table Column Mode & Metadata Aggregation Tests
// =============================================================================

func TestResolveTableProfile_RootTier(t *testing.T) {
	cfg := map[string]interface{}{
		"table_column_mode":  "manual",
		"table_column_roles": map[string]interface{}{"col1": "indexing", "col2": "metadata"},
		"table_column_names": []interface{}{"col1", "col2"},
	}
	profile := ResolveTableProfile(cfg)
	if profile == nil {
		t.Fatal("want a profile from the root keys")
	}
	if profile.Mode != common.TableColumnModeManual {
		t.Errorf("mode = %q, want manual", profile.Mode)
	}
	if len(profile.Roles) != 2 {
		t.Errorf("roles = %#v, want 2", profile.Roles)
	}
	if len(profile.Columns) != 2 {
		t.Errorf("columns = %#v, want 2", profile.Columns)
	}
}

func TestResolveTableProfile_RootWriterShapes(t *testing.T) {
	cfg := map[string]interface{}{
		"table_column_mode":  "auto",
		"table_column_roles": map[string]string{"col1": "both", "col2": "Both"},
		"table_column_names": []string{"col1", "col2"},
	}
	profile := ResolveTableProfile(cfg)
	if profile == nil {
		t.Fatal("want a profile from the root keys")
	}
	got := profile.ToRolesInterfaceMap()
	if len(got) != 2 ||
		got["col1"] != string(common.ColumnRoleBoth) ||
		got["col2"] != string(common.ColumnRoleNone) {
		t.Errorf("roles = %v, want col1 %q and an out-of-vocabulary value %q",
			got, common.ColumnRoleBoth, common.ColumnRoleNone)
	}
	if !reflect.DeepEqual(profile.Columns, []string{"col1", "col2"}) {
		t.Errorf("columns = %v, want [col1 col2]", profile.Columns)
	}
}

func TestResolveTableProfile_ComponentTier(t *testing.T) {
	nested := map[string]interface{}{
		"Parser:B": map[string]interface{}{
			"spreadsheet": map[string]interface{}{
				"column_mode":  "manual",
				"column_roles": map[string]string{"age": "Metadata"},
				"column_names": []string{"name", "age"},
			},
		},
		"Parser:A": map[string]interface{}{
			"spreadsheet": map[string]interface{}{
				"column_mode": "auto",
			},
		},
	}
	profile := ResolveTableProfile(nested)
	if profile == nil {
		t.Fatal("want a profile from the parser entries")
	}
	if profile.Mode != common.TableColumnModeAuto {
		t.Errorf("mode = %q, want deterministic lowest Parser id (auto)", profile.Mode)
	}
	if len(profile.Roles) != 0 {
		t.Errorf("roles = %v, want empty from the lowest id", profile.Roles)
	}
	// The column list is its own axis, so the names the other entry states are
	// still the published schema.
	if !reflect.DeepEqual(profile.Columns, []string{"name", "age"}) {
		t.Errorf("columns = %v, want [name age]", profile.Columns)
	}
}

// A canvas can carry a spreadsheet block for a component nobody configured.
// Stopping at it would hide the profile a later component does declare, so the
// resolution continues to the next id that states a mode or a role.
func TestResolveTableProfile_SkipsEmptySpreadsheetEntry(t *testing.T) {
	cfg := map[string]interface{}{
		"Parser:A": map[string]interface{}{
			"spreadsheet": map[string]interface{}{},
		},
		"Parser:B": map[string]interface{}{
			"spreadsheet": map[string]interface{}{
				"column_mode":  "manual",
				"column_roles": map[string]interface{}{"age": "metadata"},
			},
		},
	}
	profile := ResolveTableProfile(cfg)
	if profile == nil {
		t.Fatal("want the profile the second entry declares")
	}
	if profile.Mode != common.TableColumnModeManual {
		t.Errorf("mode = %q, want \"manual\" from the entry that states one", profile.Mode)
	}
	if len(profile.Roles) != 1 || profile.Roles["age"] != common.ColumnRoleMetadata {
		t.Errorf("roles = %v, want age=metadata", profile.Roles)
	}
	if len(profile.Columns) != 0 {
		t.Errorf("columns = %v, want none", profile.Columns)
	}

	// A component that states no mode and no roles leaves no profile at all.
	emptyOnly := map[string]interface{}{
		"Parser:A": map[string]interface{}{
			"spreadsheet": map[string]interface{}{"column_mode": "", "column_roles": map[string]interface{}{}},
		},
	}
	if profile := ResolveTableProfile(emptyOnly); profile != nil {
		t.Errorf("empty component = %#v, want no profile", profile)
	}
}

// A run publishes the columns it discovered onto the root keys. That is system
// output rather than a column configuration, so it must not make the root
// authoritative over the manual profile a document dialog stored on the parser
// entry — otherwise the first successful parse would freeze the setting.
func TestResolveTableProfile_PublishedSchemaIsNotIntent(t *testing.T) {
	cfg := map[string]interface{}{
		"table_column_names": []interface{}{"name", "age"},
		"Parser:Table": map[string]interface{}{
			"spreadsheet": map[string]interface{}{
				"column_mode":  "manual",
				"column_roles": map[string]interface{}{"age": "metadata"},
			},
		},
	}
	profile := ResolveTableProfile(cfg)
	if profile == nil {
		t.Fatal("want the component's manual profile")
	}
	if profile.Mode != common.TableColumnModeManual {
		t.Errorf("mode = %q, want manual", profile.Mode)
	}
	if len(profile.Roles) != 1 || profile.Roles["age"] != common.ColumnRoleMetadata {
		t.Errorf("roles = %#v, want age=metadata", profile.Roles)
	}
	if !reflect.DeepEqual(profile.Columns, []string{"name", "age"}) {
		t.Errorf("columns = %v, want the published schema", profile.Columns)
	}
}

func TestResolveTableColumnNames(t *testing.T) {
	cfg := map[string]interface{}{
		"table_column_names": []interface{}{"root"},
		"Parser:A": map[string]interface{}{
			"spreadsheet": map[string]interface{}{"column_names": []interface{}{"component"}},
		},
	}
	if got := ResolveTableColumnNames(cfg); !reflect.DeepEqual(got, []string{"root"}) {
		t.Errorf("got %v, want the root copy first", got)
	}

	withoutRoot := map[string]interface{}{
		"Parser:B": map[string]interface{}{
			"spreadsheet": map[string]interface{}{"column_names": []interface{}{"second"}},
		},
		"Parser:A": map[string]interface{}{
			"spreadsheet": map[string]interface{}{"column_names": []interface{}{"first"}},
		},
	}
	if got := ResolveTableColumnNames(withoutRoot); !reflect.DeepEqual(got, []string{"first"}) {
		t.Errorf("got %v, want the lowest Parser id", got)
	}

	if got := ResolveTableColumnNames(map[string]interface{}{"Parser:A": "not a component"}); got != nil {
		t.Errorf("got %v, want nothing", got)
	}
	if got := ResolveTableColumnNames(nil); got != nil {
		t.Errorf("got %v, want nothing", got)
	}
}

func TestTableParserStripDocMetadataKeys(t *testing.T) {
	cfg := map[string]interface{}{
		"table_column_names": []interface{}{"col1", "col2", "col1", "  "},
	}
	keys := TableParserStripDocMetadataKeys(cfg)
	if len(keys) != 2 || keys[0] != "col1" || keys[1] != "col2" {
		t.Errorf("keys = %v, want [col1, col2]", keys)
	}

	// Fallback to roles if names absent
	cfgRoles := map[string]interface{}{
		"table_column_roles": map[string]interface{}{"roleA": "both"},
	}
	keysRoles := TableParserStripDocMetadataKeys(cfgRoles)
	if len(keysRoles) != 1 || keysRoles[0] != "roleA" {
		t.Errorf("keysRoles = %v, want [roleA]", keysRoles)
	}
}

func TestAggregateTableDocMetadata_AutoMode(t *testing.T) {
	chunks := []map[string]any{
		{
			"text": "- Name: Alice\n- City: Beijing",
			"chunk_data": map[string]interface{}{
				"Name": "Alice",
				"City": "Beijing",
			},
		},
		{
			"text": "- Name: Bob\n- City: Beijing",
			"chunk_data": map[string]interface{}{
				"Name": "Bob",
				"City": "Beijing",
			},
		},
	}
	cfg := map[string]interface{}{
		"table_column_mode": "auto",
	}
	meta := AggregateTableDocMetadata(chunks, cfg)
	if meta == nil {
		t.Fatal("expected non-nil meta")
	}
	cities, ok := meta["City"].([]string)
	if !ok || len(cities) != 1 || cities[0] != "Beijing" {
		t.Errorf("City = %v, want [Beijing]", meta["City"])
	}
	names, ok := meta["Name"].([]string)
	if !ok || len(names) != 2 {
		t.Errorf("Name = %v, want 2 names", meta["Name"])
	}
}

func TestAggregateTableDocMetadata_ManualMode(t *testing.T) {
	chunks := []map[string]any{
		{
			"text": "- Name: Alice",
			"chunk_data": map[string]interface{}{
				"Age": "30",
			},
		},
		{
			"text": "- Name: Bob",
			"chunk_data": map[string]interface{}{
				"Age": "25",
			},
		},
	}
	cfg := map[string]interface{}{
		"table_column_mode": "manual",
		"table_column_roles": map[string]interface{}{
			"Name": "indexing",
			"Age":  "metadata",
		},
	}
	meta := AggregateTableDocMetadata(chunks, cfg)
	if meta == nil {
		t.Fatal("expected non-nil meta")
	}
	if _, hasName := meta["Name"]; hasName {
		t.Errorf("Name should not be in doc metadata when role is indexing")
	}
	ages, ok := meta["Age"].([]string)
	if !ok || len(ages) != 2 {
		t.Errorf("Age = %v, want 2 entries", meta["Age"])
	}
}

// Python compares the stored role as is (rag/utils/table_es_metadata.py:187,
// like the chunk-body membership tests at rag/app/table.py:704-706), so a
// differently cased role excludes the column from document metadata. Only
// "metadata" and "both" aggregate, so the indexing alias "vectorize" excludes
// it as well.
func TestAggregateTableDocMetadata_RoleIsCaseSensitive(t *testing.T) {
	chunks := []map[string]any{
		{
			"text":       "- A: 1",
			"chunk_data": map[string]interface{}{"A": "1", "B": "2"},
		},
	}
	cfg := map[string]interface{}{
		"table_column_mode": "manual",
		"table_column_roles": map[string]interface{}{
			"A": "Metadata",
			"B": "VECTORIZE",
		},
		"table_column_names": []interface{}{"A", "B"},
	}
	if meta := AggregateTableDocMetadata(chunks, cfg); len(meta) != 0 {
		t.Fatalf("a differently cased role must exclude both columns, got %v", meta)
	}

	cfg["table_column_roles"] = map[string]interface{}{"A": "metadata", "B": "vectorize"}
	meta := AggregateTableDocMetadata(chunks, cfg)
	if _, ok := meta["A"]; !ok {
		t.Errorf("A with the canonical role metadata must aggregate, got %v", meta)
	}
	if _, hasB := meta["B"]; hasB {
		t.Errorf("B with the legacy vectorize alias must not aggregate, got %v", meta)
	}
}
