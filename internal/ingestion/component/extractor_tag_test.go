package component

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/schema"
)

type stubExtractorTagChat struct {
	responses map[string]string
}

func (s *stubExtractorTagChat) Chat(_ context.Context, req extractorChatRequest) (*extractorChatResponse, error) {
	if s.responses != nil {
		if msg, ok := s.responses["__static__"]; ok {
			return &extractorChatResponse{Content: msg}, nil
		}
	}
	return &extractorChatResponse{Content: `{"RAG": 8, "vector database": 6}`}, nil
}

func pushExtractorTagChatStub(t *testing.T, responses map[string]string) {
	t.Helper()
	stub := &stubExtractorTagChat{responses: responses}
	SetExtractorChatInvoker(stub)
	t.Cleanup(func() { SetExtractorChatInvoker(nil) })
}

func pushExtractorTagTargetResolverStub(t *testing.T) {
	t.Helper()
	SetExtractorChatTargetResolverOverride(func(llmID string) (string, string, string, string, bool) {
		return "test_driver", "test_model", "test_key", "", true
	})
	t.Cleanup(func() { SetExtractorChatTargetResolverOverride(nil) })
}

func TestExtractorTags_NoTagFileID(t *testing.T) {
	pushExtractorTagChatStub(t, nil)
	comp, _ := NewExtractorComponent(map[string]any{"auto_tags": 3})
	out, err := comp.Invoke(context.Background(), map[string]any{
		"chunks": []map[string]any{
			{"content_with_weight": "test"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if _, ok := chunks[0][common.TAG_FLD]; ok {
		t.Fatal("tag_feas should not be set when tag_file_id is absent")
	}
}

func TestExtractorTags_NoLLMID(t *testing.T) {
	pushExtractorTagChatStub(t, nil)
	comp, _ := NewExtractorComponent(map[string]any{"auto_tags": 3})
	out, err := comp.Invoke(context.Background(), map[string]any{
		"chunks": []map[string]any{
			{"content_with_weight": "some unrelated text"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if _, ok := chunks[0][common.TAG_FLD]; ok {
		t.Fatal("tag_feas should not be set without tag_file_id")
	}
}

func TestExtractorTags_WithKeywords(t *testing.T) {
	pushExtractorTagChatStub(t, nil)
	pushExtractorTagTargetResolverStub(t)

	comp, _ := NewExtractorComponent(map[string]any{
		"llm_id":        "test@test",
		"auto_tags":     3,
		"auto_keywords": 3,
	})
	out, err := comp.Invoke(context.Background(), map[string]any{
		"chunks": []map[string]any{
			{"content_with_weight": "some unrelated textxyz"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if chunks[0][common.TAG_FLD] != nil {
		t.Fatal("tag_feas should not be set without tag_file_id")
	}
}

func TestExtractorTags_ComponentRegistration(t *testing.T) {
	factory, cat, md, ok := runtime.DefaultRegistry.Lookup(componentNameExtractor)
	if !ok {
		t.Fatal("Extractor not registered in runtime.DefaultRegistry")
	}
	if cat != runtime.CategoryIngestion {
		t.Fatalf("expected ingestion category, got %v", cat)
	}
	if factory == nil {
		t.Fatal("factory is nil")
	}
	data, _ := json.Marshal(md)
	t.Logf("Extractor metadata: %s", data)
}

func TestParseTaggerResponse(t *testing.T) {
	raw := `</think>{"RAG": 8, "LLM": 3, "open": 1}`
	result := parseTaggerResponse(raw, 2)
	if len(result) != 2 {
		t.Fatalf("expected 2 (top-2), got %d: %v", len(result), result)
	}
	if result["RAG"] != 8 {
		t.Fatalf("expected RAG=8, got %d", result["RAG"])
	}
	if _, ok := result["open"]; ok {
		t.Fatal("open should be trimmed (not top-2)")
	}
}

func TestParseTaggerResponse_JSONRepair(t *testing.T) {
	raw := `some prefix garbage {"RAG": 8, "LLM": 5} trailing stuff`
	result := parseTaggerResponse(raw, 2)
	if result["RAG"] != 8 || result["LLM"] != 5 {
		t.Fatalf("json-repair fallback failed, got %v", result)
	}
}

func TestParseCSVTagSource_Comma(t *testing.T) {
	// A comma-delimited file maps to exactly two columns, so each line can
	// carry only a single tag (tags are comma-separated within the 2nd col).
	text := "RAGFlow tutorial,RAG\nsome text,LLM"
	result := parseCSVTagSource(text)
	if len(result) != 2 {
		t.Fatalf("expected 2 examples, got %d", len(result))
	}
	if result[0].Content != "RAGFlow tutorial" {
		t.Errorf("content[0] = %q", result[0].Content)
	}
	if len(result[0].Tags) != 1 || result[0].Tags[0] != "RAG" {
		t.Errorf("tags[0] = %v", result[0].Tags)
	}
}

func TestParseCSVTagSourceBytes_Comma(t *testing.T) {
	data := []byte("RAGFlow tutorial,RAG\nsome text,LLM")
	result := parseCSVTagSourceBytes(data)
	if len(result) != 2 {
		t.Fatalf("expected 2 examples, got %d", len(result))
	}
	if result[1].Tags[0] != "LLM" {
		t.Fatalf("unexpected tags: %v", result[1].Tags)
	}
}

func TestParseCSVTagSourceReader_Comma(t *testing.T) {
	r := bytes.NewBufferString("RAGFlow tutorial,RAG\nsome text,LLM")
	result := parseCSVTagSourceReader(r, ",")
	if len(result) != 2 {
		t.Fatalf("expected 2 examples, got %d", len(result))
	}
	if result[0].Content != "RAGFlow tutorial" {
		t.Fatalf("unexpected content: %q", result[0].Content)
	}
	if len(result[1].Tags) != 1 || result[1].Tags[0] != "LLM" {
		t.Fatalf("unexpected tags: %v", result[1].Tags)
	}
}

func TestParseCSVTagSource_Tab(t *testing.T) {
	text := "RAGFlow tutorial\tRAG,tutorial\nvector database guide\tvector database,config"
	result := parseCSVTagSource(text)
	if len(result) != 2 {
		t.Fatalf("expected 2 examples, got %d", len(result))
	}
	if result[0].Content != "RAGFlow tutorial" {
		t.Errorf("content[0] = %q", result[0].Content)
	}
	if len(result[1].Tags) != 2 || result[1].Tags[1] != "config" {
		t.Errorf("tags[1] = %v", result[1].Tags)
	}
}

func TestParseCSVTagSource_Accumulation(t *testing.T) {
	// Mirrors rag/app/tag.py txt semantics: lines that do not split into two
	// columns are accumulated as body text and prepended to the next tagged
	// line's content.
	text := "intro paragraph\nmore body\nRAGFlow tutorial\tRAG,tutorial"
	result := parseCSVTagSource(text)
	if len(result) != 1 {
		t.Fatalf("expected 1 example, got %d", len(result))
	}
	want := "intro paragraph\nmore body\nRAGFlow tutorial"
	if result[0].Content != want {
		t.Errorf("content = %q, want %q", result[0].Content, want)
	}
	if len(result[0].Tags) != 2 || result[0].Tags[0] != "RAG" || result[0].Tags[1] != "tutorial" {
		t.Errorf("tags = %v", result[0].Tags)
	}
}

func stubXLSXBytes(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(f.SetCellValue("Sheet1", "A1", "RAGFlow guide"))
	must(f.SetCellValue("Sheet1", "B1", "RAG, tutorial"))
	must(f.SetCellValue("Sheet1", "A2", "vector db"))
	must(f.SetCellValue("Sheet1", "B2", "vector database, config"))
	_, err := f.NewSheet("Sheet2")
	must(err)
	must(f.SetCellValue("Sheet2", "A1", "LLM intro"))
	must(f.SetCellValue("Sheet2", "B1", "LLM"))
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParseCSVQuoteAwareReader(t *testing.T) {
	// A quoted content field containing a comma must not be split into extra
	// columns: "RAGFlow, the guide",RAG is two fields, not three.
	text := "\"RAGFlow, the guide\",RAG\nplain line,LLM"
	result := parseCSVQuoteAwareReader(strings.NewReader(text))
	if len(result) != 2 {
		t.Fatalf("expected 2 examples, got %d", len(result))
	}
	// First line: quoted content keeps the comma -> content "RAGFlow, the guide", tags [RAG]
	if result[0].Content != "RAGFlow, the guide" {
		t.Errorf("content[0] = %q", result[0].Content)
	}
	if len(result[0].Tags) != 1 || result[0].Tags[0] != "RAG" {
		t.Errorf("tags[0] = %v", result[0].Tags)
	}
	// Second line: plain comma split -> content "plain line", tags [LLM]
	if result[1].Content != "plain line" {
		t.Errorf("content[1] = %q", result[1].Content)
	}
	if len(result[1].Tags) != 1 || result[1].Tags[0] != "LLM" {
		t.Errorf("tags[1] = %v", result[1].Tags)
	}
}

func TestParseXLSXTagSource(t *testing.T) {
	data := stubXLSXBytes(t)
	result := parseXLSXTagSource(data)
	if len(result) != 3 {
		t.Fatalf("expected 3 examples (multi-sheet), got %d", len(result))
	}
	// Each row's first cell is content, second is comma-separated tags.
	if result[0].Content != "RAGFlow guide" {
		t.Errorf("content[0] = %q", result[0].Content)
	}
	if len(result[0].Tags) != 2 || result[0].Tags[1] != "tutorial" {
		t.Errorf("tags[0] = %v", result[0].Tags)
	}
	if result[2].Content != "LLM intro" {
		t.Errorf("content[2] = %q", result[2].Content)
	}
	if len(result[2].Tags) != 1 || result[2].Tags[0] != "LLM" {
		t.Errorf("tags[2] = %v", result[2].Tags)
	}
}

func TestParseCSVTagSource_EmptyTags(t *testing.T) {
	// Mirrors rag/app/tag.py: a row with exactly two columns is appended even
	// when the tag column is empty (beAdoc always appends for a 2-column row).
	text := "content one\tRAG,tutorial\nsolo line\t"
	result := parseCSVTagSource(text)
	if len(result) != 2 {
		t.Fatalf("expected 2 examples, got %d", len(result))
	}
	if len(result[0].Tags) != 2 {
		t.Errorf("tags[0] = %v", result[0].Tags)
	}
	// Second row has an empty tag column -> still an example, with no tags.
	if len(result[1].Tags) != 0 {
		t.Errorf("tags[1] = %v, want empty (matches Python)", result[1].Tags)
	}
	if result[1].Content != "solo line" {
		t.Errorf("content[1] = %q", result[1].Content)
	}
}

func TestParseTagSourceByFilename(t *testing.T) {
	// .xlsx -> multi-sheet, 2 columns per row.
	if got, err := parseTagSourceByFilename(stubXLSXBytes(t), "tags.xlsx"); err != nil || len(got) != 3 {
		t.Errorf("xlsx: expected 3 examples, got %d (err=%v)", len(got), err)
	}
	// .csv -> quote-aware comma parsing.
	csvData := []byte("\"a, b\",RAG\nsimple,LLM")
	if got, err := parseTagSourceByFilename(csvData, "tags.csv"); err != nil || len(got) != 2 {
		t.Errorf("csv: expected 2 examples, got %d (err=%v)", len(got), err)
	}
	if got, err := parseTagSourceByFilename(csvData, "tags.CSV"); err != nil || len(got) != 2 {
		t.Errorf("csv (uppercase ext): expected 2 examples, got %d (err=%v)", len(got), err)
	}
	// .txt -> delimiter-detecting txt reader.
	txtData := []byte("content one\tRAG,tutorial")
	if got, err := parseTagSourceByFilename(txtData, "tags.txt"); err != nil || len(got) != 1 {
		t.Errorf("txt: expected 1 example, got %d (err=%v)", len(got), err)
	}
	// Unsupported / no extension mirrors Python's NotImplementedError: the tag
	// source is not parsed and an error is returned.
	if _, err := parseTagSourceByFilename(txtData, "noextension"); err == nil {
		t.Error("no extension: expected an error (unsupported extension)")
	}
	if _, err := parseTagSourceByFilename(txtData, "tags.json"); err == nil {
		t.Error("unsupported extension: expected an error")
	}
}

func TestJaccardOverlap(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []string
		expected float64
	}{
		{"identical", []string{"a", "b"}, []string{"a", "b"}, 1.0},
		{"half overlap", []string{"a", "b"}, []string{"b", "c"}, 1.0 / 3.0},
		{"no overlap", []string{"a", "b"}, []string{"c", "d"}, 0.0},
		{"empty a", []string{}, []string{"a"}, 0.0},
		{"empty b", []string{"a"}, []string{}, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jaccardOverlap(tt.a, tt.b)
			if got != tt.expected {
				t.Fatalf("jaccard(%v, %v) = %.4f, want %.4f", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}

func TestBuildAllTagsProportions(t *testing.T) {
	ts := []schema.TagLabel{
		{Content: "a", Tags: []string{"RAG", "LLM"}},
		{Content: "b", Tags: []string{"RAG", "open"}},
	}
	result := buildAllTagsProportions(ts)
	if len(result) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(result))
	}
	if result["RAG"] <= result["LLM"] {
		t.Fatalf("expected RAG proportion > LLM, got RAG=%.6f LLM=%.6f", result["RAG"], result["LLM"])
	}
	if result["LLM"] <= 0 {
		t.Fatal("expected LLM proportion > 0")
	}
}

func TestJsonRepairExtract(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		hasField string
		want     int
	}{
		{"valid json", `{"RAG": 8}`, "RAG", 8},
		{"prefix garbage", `xxx {"RAG": 8, "LLM": 5} yyy`, "LLM", 5},
		{"nested braces", `{"outer": "x", "inner": {"k": 1}}`, "outer", 0},
		{"no json", `no json here`, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := jsonRepairExtract(tt.input)
			if tt.hasField == "" {
				if result != nil {
					t.Fatalf("expected nil, got %v", result)
				}
				return
			}
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			switch v := result[tt.hasField].(type) {
			case float64:
				if int(v) != tt.want && tt.want != 0 {
					t.Fatalf("expected %s=%d, got %d", tt.hasField, tt.want, int(v))
				}
			}
		})
	}
}
