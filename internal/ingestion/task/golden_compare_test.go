package task

import (
	"testing"
	"time"
)

func TestProcessPipelineOutputForGolden_Markdown(t *testing.T) {
	input := map[string]any{
		"markdown": "# Title\n\nContent",
	}

	result := ProcessPipelineOutputForGolden(input, "doc-1", "kb-1", "sample.md")

	if len(result.NormalizedChunks) != 1 {
		t.Fatalf("normalized len = %d, want 1", len(result.NormalizedChunks))
	}
	if got := result.NormalizedChunks[0]["text"]; got != "# Title\n\nContent" {
		t.Fatalf("normalized text = %v, want markdown string", got)
	}
	if len(result.ProcessedChunks) != 1 {
		t.Fatalf("processed len = %d, want 1", len(result.ProcessedChunks))
	}
	if got := result.ProcessedChunks[0]["content_with_weight"]; got != "# Title\n\nContent" {
		t.Fatalf("content_with_weight = %v, want markdown string", got)
	}
	if _, exists := result.ProcessedChunks[0]["text"]; exists {
		t.Fatal("processed chunk should not keep text key")
	}
}

func TestProcessChunksForDataflow_StableFields(t *testing.T) {
	now := time.Date(2026, 7, 3, 20, 0, 0, 0, time.FixedZone("CST", 8*3600))
	chunks := []map[string]any{
		{
			"text":      "hello",
			"questions": "Q1\nQ2",
			"keywords":  "kw1,kw2",
			"summary":   "sum",
			"metadata":  map[string]any{"author": "Alice"},
		},
	}

	meta := ProcessChunksForDataflow(chunks, "doc-1", "kb-1", "sample.md", now)

	chunk := chunks[0]
	if chunk["doc_id"] != "doc-1" {
		t.Fatalf("doc_id = %v", chunk["doc_id"])
	}
	if chunk["docnm_kwd"] != "sample.md" {
		t.Fatalf("docnm_kwd = %v", chunk["docnm_kwd"])
	}
	if chunk["content_with_weight"] != "hello" {
		t.Fatalf("content_with_weight = %v", chunk["content_with_weight"])
	}
	if _, ok := chunk["id"].(string); !ok {
		t.Fatalf("id should be string, got %T", chunk["id"])
	}
	if _, exists := chunk["questions"]; exists {
		t.Fatal("questions should be removed")
	}
	if _, exists := chunk["keywords"]; exists {
		t.Fatal("keywords should be removed")
	}
	if _, exists := chunk["summary"]; exists {
		t.Fatal("summary should be removed")
	}
	if meta["author"] != "Alice" {
		t.Fatalf("metadata merge failed: %v", meta)
	}
}
