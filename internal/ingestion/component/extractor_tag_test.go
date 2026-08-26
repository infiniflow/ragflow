package component

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
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
	comp, _ := NewExtractorComponent(map[string]any{"tags": map[string]any{"top_n": 3}})
	out, err := comp.Invoke(t.Context(), nil, map[string]any{
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
	if _, ok := chunks[0]["tag_kwd"]; ok {
		t.Fatal("tag_kwd should not be set when tag_file_id is absent")
	}
}

func TestExtractorTags_NoLLMID(t *testing.T) {
	pushExtractorTagChatStub(t, nil)
	comp, _ := NewExtractorComponent(map[string]any{"tags": map[string]any{"top_n": 3}})
	out, err := comp.Invoke(t.Context(), nil, map[string]any{
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
	if _, ok := chunks[0]["tag_kwd"]; ok {
		t.Fatal("tag_kwd should not be set without tag_file_id")
	}
}

func TestExtractorTags_WithKeywords(t *testing.T) {
	pushExtractorTagChatStub(t, nil)
	pushExtractorTagTargetResolverStub(t)

	comp, _ := NewExtractorComponent(map[string]any{
		"llm_id": "test@test",
		"tags": map[string]any{
			"top_n": 3,
		},
		"keywords": map[string]any{
			"top_n": 3,
		},
	})
	out, err := comp.Invoke(t.Context(), nil, map[string]any{
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
	if chunks[0]["tag_kwd"] != nil {
		t.Fatal("tag_kwd should not be set without tag_file_id")
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

type capturingExtractorTagChat struct {
	req extractorChatRequest
}

func (c *capturingExtractorTagChat) Chat(_ context.Context, req extractorChatRequest) (*extractorChatResponse, error) {
	c.req = req
	return &extractorChatResponse{Content: `{"RAG": 8, "vector database": 6}`}, nil
}

func pushCapturingTagChat(t *testing.T) *capturingExtractorTagChat {
	t.Helper()
	capt := &capturingExtractorTagChat{}
	SetExtractorChatInvoker(capt)
	t.Cleanup(func() { SetExtractorChatInvoker(nil) })
	return capt
}

func longChunkText() string {
	return strings.TrimSpace(strings.Repeat("RAGFlow is an open source retrieval augmented generation engine. ", 10))
}

func TestLlmtagChunk_MessageFit(t *testing.T) {
	const budget = 20
	SetExtractorContextLengthOverride(func(_ context.Context, _ string) int { return budget })
	t.Cleanup(func() { SetExtractorContextLengthOverride(nil) })

	capt := pushCapturingTagChat(t)

	longText := longChunkText()
	chunk := map[string]any{"content_with_weight": longText}
	allTags := map[string]float64{"RAG": 1, "database": 1, "AI": 1}
	examples := []schema.TaggedChunk{{Content: "example one", TagWeights: map[string]int{"AI": 5}}}

	llmTagChunk(t.Context(), nil, capt, chunk, allTags, examples, "test@test", "test_driver", "test_model", "test_key", "", 3, nil)

	if len(capt.req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(capt.req.Messages))
	}
	if strings.Contains(capt.req.Messages[0].Content, longText) {
		t.Fatal("system prompt was not trimmed to the context budget")
	}
	total := tokenizer.NumTokensFromString(capt.req.Messages[0].Content) +
		tokenizer.NumTokensFromString(capt.req.Messages[1].Content)
	if total > extractorContextFitBudget(budget) {
		t.Fatalf("fitted messages total %d tokens exceeds the margin-adjusted budget %d", total, extractorContextFitBudget(budget))
	}
}

func TestLlmtagChunk_NoContextLength_SkipsFit(t *testing.T) {
	capt := pushCapturingTagChat(t)

	longText := longChunkText()
	chunk := map[string]any{"content_with_weight": longText}
	allTags := map[string]float64{"RAG": 1}
	examples := []schema.TaggedChunk{{Content: "example", TagWeights: map[string]int{"AI": 5}}}

	llmTagChunk(t.Context(), nil, capt, chunk, allTags, examples, "test@test", "test_driver", "test_model", "test_key", "", 3, nil)

	if len(capt.req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(capt.req.Messages))
	}
	if !strings.Contains(capt.req.Messages[0].Content, longText) {
		t.Fatal("system prompt should pass through untrimmed when context length is unknown")
	}
}

func TestLlmtagChunk_ColdStartFallback(t *testing.T) {
	capt := pushCapturingTagChat(t)

	idx := &MemoryTagIndex{
		examples: []schema.TagLabel{
			{Content: "real sample text one", Tags: []string{"NLP", "AI"}},
			{Content: "real sample text two", Tags: []string{"Search"}},
		},
		allTags: map[string]float64{"NLP": 0.33, "AI": 0.33, "Search": 0.33},
	}

	chunk := map[string]any{"content_with_weight": "some content"}
	llmTagChunk(t.Context(), nil, capt, chunk, idx.allTags, nil, "test@test", "test_driver", "test_model", "test_key", "", 3, idx)

	if len(capt.req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(capt.req.Messages))
	}
	// Verify that real sample content is present in prompt
	promptContent := capt.req.Messages[0].Content
	if !strings.Contains(promptContent, "real sample text one") && !strings.Contains(promptContent, "real sample text two") {
		t.Fatalf("cold start fallback did not include real sample text: %s", promptContent)
	}
	// Verify fake mock example is NOT present
	if strings.Contains(promptContent, "This is an example") {
		t.Fatal("prompt contains fake example 'This is an example'")
	}
}

func TestParseCSVQuoteAwareReader(t *testing.T) {
	text := "\"RAGFlow, the guide\",RAG\nplain line,LLM"
	result := parseCSVQuoteAwareReader(strings.NewReader(text))
	if len(result) != 2 {
		t.Fatalf("expected 2 examples, got %d", len(result))
	}
	if result[0].Content != "RAGFlow, the guide" {
		t.Errorf("content[0] = %q", result[0].Content)
	}
	if len(result[0].Tags) != 1 || result[0].Tags[0] != "RAG" {
		t.Errorf("tags[0] = %v", result[0].Tags)
	}
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
	text := "content one\tRAG,tutorial\nsolo line\t"
	result := parseCSVTagSource(text)
	if len(result) != 2 {
		t.Fatalf("expected 2 examples, got %d", len(result))
	}
	if len(result[0].Tags) != 2 {
		t.Errorf("tags[0] = %v", result[0].Tags)
	}
	if len(result[1].Tags) != 0 {
		t.Errorf("tags[1] = %v, want empty", result[1].Tags)
	}
	if result[1].Content != "solo line" {
		t.Errorf("content[1] = %q", result[1].Content)
	}
}

func TestParseTagSourceByFilename(t *testing.T) {
	if got, err := parseTagSourceByFilename(stubXLSXBytes(t), "tags.xlsx"); err != nil || len(got) != 3 {
		t.Errorf("xlsx: expected 3 examples, got %d (err=%v)", len(got), err)
	}
	csvData := []byte("\"a, b\",RAG\nsimple,LLM")
	if got, err := parseTagSourceByFilename(csvData, "tags.csv"); err != nil || len(got) != 2 {
		t.Errorf("csv: expected 2 examples, got %d (err=%v)", len(got), err)
	}
	if got, err := parseTagSourceByFilename(csvData, "tags.CSV"); err != nil || len(got) != 2 {
		t.Errorf("csv (uppercase ext): expected 2 examples, got %d (err=%v)", len(got), err)
	}
	txtData := []byte("content one\tRAG,tutorial")
	if got, err := parseTagSourceByFilename(txtData, "tags.txt"); err != nil || len(got) != 1 {
		t.Errorf("txt: expected 1 example, got %d (err=%v)", len(got), err)
	}
	if _, err := parseTagSourceByFilename(txtData, "noextension"); err == nil {
		t.Error("no extension: expected an error (unsupported extension)")
	}
	if _, err := parseTagSourceByFilename(txtData, "tags.json"); err == nil {
		t.Error("unsupported extension: expected an error")
	}
}

func TestParseTaggerResponse_DotSanitization(t *testing.T) {
	raw := `{"model.v1.0": 8, "rag_tech": 6, "rag.db.v2": 4}`
	result := parseTaggerResponse(raw, 5)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result["model_v1_0"] != 8 {
		t.Fatalf("expected model_v1_0=8, got %d", result["model_v1_0"])
	}
	if result["rag_tech"] != 6 {
		t.Fatalf("expected rag_tech=6, got %d", result["rag_tech"])
	}
	if result["rag_db_v2"] != 4 {
		t.Fatalf("expected rag_db_v2=4, got %d", result["rag_db_v2"])
	}
	for k := range result {
		if strings.Contains(k, ".") {
			t.Fatalf("found unescaped dot in tag key: %s", k)
		}
	}

	// Test collision preservation of higher score
	collisionRaw := `{"model.v1.0": 3, "model_v1_0": 9, "tag.a": 8, "tag_a": 4}`
	collisionResult := parseTaggerResponse(collisionRaw, 5)
	if collisionResult["model_v1_0"] != 9 {
		t.Fatalf("expected highest score 9 for model_v1_0 collision, got %d", collisionResult["model_v1_0"])
	}
	if collisionResult["tag_a"] != 8 {
		t.Fatalf("expected highest score 8 for tag_a collision, got %d", collisionResult["tag_a"])
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

func TestParseTaggerResponse_LastThinkTag(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantKeys []string
	}{
		{
			name:     "single think block",
			raw:      `<think>reasoning</think>{"tag": 5}`,
			wantKeys: []string{"tag"},
		},
		{
			name:     "nested think blocks",
			raw:      `<think>outer</think>mid<think>inner</think>{"real": 3}`,
			wantKeys: []string{"real"},
		},
		{
			name:     "no think tag",
			raw:      `{"simple": 7}`,
			wantKeys: []string{"simple"},
		},
		{
			name: "error sentinel",
			raw:  "**ERROR**",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTaggerResponse(tt.raw, 5)
			if len(tt.wantKeys) == 0 {
				if len(got) != 0 {
					t.Errorf("expected empty result, got %v", got)
				}
				return
			}
			for _, k := range tt.wantKeys {
				if _, ok := got[k]; !ok {
					t.Errorf("expected key %q in result, got %v", k, got)
				}
			}
		})
	}
}

func TestGetChunkText_Enrichment(t *testing.T) {
	tests := []struct {
		name  string
		chunk map[string]any
		want  string
	}{
		{
			name:  "nil chunk",
			chunk: nil,
			want:  "",
		},
		{
			name:  "content only",
			chunk: map[string]any{"content_with_weight": "simple body text"},
			want:  "simple body text",
		},
		{
			name:  "text fallback",
			chunk: map[string]any{"text": "text fallback body"},
			want:  "text fallback body",
		},
		{
			name: "content present ignores docnm_kwd",
			chunk: map[string]any{
				"docnm_kwd":           "2026_Engineering_Bidding_Doc.pdf",
				"content_with_weight": "contract guidelines",
			},
			want: "contract guidelines",
		},
		{
			name: "title fallback chain docnm_kwd wins when no content",
			chunk: map[string]any{
				"docnm_kwd": "doc_name.docx",
				"title_tks": "ignored_title",
			},
			want: "doc_name",
		},
		{
			name: "title fallback to title_tks when no content and docnm_kwd absent",
			chunk: map[string]any{
				"title_tks": "parsed_title.txt",
			},
			want: "parsed_title",
		},
		{
			name: "important_kwd string slice",
			chunk: map[string]any{
				"important_kwd":       []string{"Bidding", "Tender"},
				"content_with_weight": "body",
			},
			want: "body Bidding Tender",
		},
		{
			name: "important_kwd any slice",
			chunk: map[string]any{
				"important_kwd":       []any{"Alpha", "Beta"},
				"content_with_weight": "body",
			},
			want: "body Alpha Beta",
		},
		{
			name: "important_kwd string",
			chunk: map[string]any{
				"important_kwd":       "SingleKey",
				"content_with_weight": "body",
			},
			want: "body SingleKey",
		},
		{
			name: "all sources combined ignores title when content present",
			chunk: map[string]any{
				"docnm_kwd":           "Project_Tender_Specification.pdf",
				"important_kwd":       []string{"Procurement", "Compliance"},
				"content_with_weight": "All bidders must follow instructions.",
			},
			want: "All bidders must follow instructions. Procurement Compliance",
		},
		{
			name: "only docnm_kwd present",
			chunk: map[string]any{
				"docnm_kwd": "2026_Engineering_Bidding_Doc.pdf",
			},
			want: "2026_Engineering_Bidding_Doc",
		},
		{
			name: "all fields empty or whitespace",
			chunk: map[string]any{
				"docnm_kwd":           "   ",
				"important_kwd":       []string{"  ", ""},
				"content_with_weight": "   ",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getChunkText(tt.chunk)
			if got != tt.want {
				t.Errorf("getChunkText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOrderedTagWeights_MarshalJSON(t *testing.T) {
	weights := OrderedTagWeights{
		"价格咨询": 1,
		"活动咨询": 3,
		"物流查询": 2,
	}
	data, err := json.Marshal(weights)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	got := string(data)
	want := `{"活动咨询":3,"物流查询":2,"价格咨询":1}`
	if got != want {
		t.Fatalf("OrderedTagWeights JSON order mismatch: got %s, want %s", got, want)
	}

	// Test embedding inside a chunk map (mimicking Elasticsearch doc serialization)
	doc := map[string]any{
		"doc_id":   "doc-1",
		"tag_feas": weights,
	}
	docData, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("json.Marshal doc failed: %v", err)
	}
	docStr := string(docData)
	if !strings.Contains(docStr, `"tag_feas":{"活动咨询":3,"物流查询":2,"价格咨询":1}`) {
		t.Fatalf("doc JSON tag_feas order mismatch: got %s", docStr)
	}
}

func TestContainsCJK(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"Hello world", false},
		{"Hello 世界", true},
		{"这是一段中文", true},
		{"12345!@#$", false},
		{"\u3400\u3401", true}, // CJK Extension A
	}
	for _, tt := range tests {
		if got := containsCJK(tt.input); got != tt.want {
			t.Errorf("containsCJK(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestContainsCJK_LanguageAutoDetection(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"pure english text", "This is pure English text without any Asian characters.", false},
		{"pure ascii numbers and symbols", "1234567890 !@#$%^&*()_+=-~`[]{}\\|;':\",.<>/?", false},
		{"simplified chinese characters", "这是简体中文字符串", true},
		{"traditional chinese characters", "這是繁體中文詞彙", true},
		{"mixed english and chinese", "RAGFlow 智能知识库问答系统 version 2.0", true},
		{"single CJK character boundary low", "\u4e00", true},  // U+4E00: '一'
		{"single CJK character boundary high", "\u9fa5", true}, // U+9FA5
		{"CJK Extension A boundary low", "\u3400", true},       // U+3400
		{"CJK Extension A boundary high", "\u4dbf", true},      // U+4DBF
		{"non-CJK unicode characters (latin accents)", "Café, résumé, naïve, señor", false},
		{"greek unicode characters", "αβγδε", false},
		{"cyrillic unicode characters", "Привет мир", false},
		{"empty string", "", false},
		{"whitespace only", "   \t\r\n   ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsCJK(tt.input)
			if got != tt.want {
				t.Errorf("containsCJK(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestPopulateTagKwd_LLMTagChunk(t *testing.T) {
	capt := pushCapturingTagChat(t)
	chunk := map[string]any{"content_with_weight": "some content"}
	allTags := map[string]float64{"RAG": 0.5, "vector database": 0.5}

	llmTagChunk(t.Context(), nil, capt, chunk, allTags, nil, "test@test", "test_driver", "test_model", "test_key", "", 3, nil)

	tagKwd, ok := chunk["tag_kwd"].([]string)
	if !ok {
		t.Fatalf("expected chunk['tag_kwd'] to be []string, got %T (%v)", chunk["tag_kwd"], chunk["tag_kwd"])
	}
	if len(tagKwd) != 2 {
		t.Fatalf("expected 2 tags in tag_kwd, got %d: %v", len(tagKwd), tagKwd)
	}
	// Verify strict score-descending order: RAG (8) > vector database (6)
	if tagKwd[0] != "RAG" || tagKwd[1] != "vector database" {
		t.Fatalf("expected tag_kwd to be ordered descending by score ['RAG', 'vector database'], got: %v", tagKwd)
	}
	if chunk[common.TAG_FLD] == nil {
		t.Fatal("expected tag_feas to be populated")
	}
}

func TestSortedTagWeightsKeys(t *testing.T) {
	weights := map[string]int{
		"Low":     3,
		"Highest": 9,
		"Medium":  6,
		"Another": 6,
	}
	got := sortedTagWeightsKeys(weights)
	expected := []string{"Highest", "Another", "Medium", "Low"}
	if len(got) != len(expected) {
		t.Fatalf("length mismatch: got %d, want %d", len(got), len(expected))
	}
	if got[0] != "Highest" || got[3] != "Low" {
		t.Fatalf("sortedTagWeightsKeys order mismatch: %v", got)
	}
}

func TestCleanTitle_PreservesVersionNumbers(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"document.pdf", "document"},
		{"report_v1.0.docx", "report_v1.0"},
		{"version 2.0", "version 2.0"},
		{"manual_v2.5.txt", "manual_v2.5"},
		{"data.csv", "data"},
	}
	for _, tt := range tests {
		got := cleanTitle(tt.input)
		if got != tt.want {
			t.Errorf("cleanTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMatchAndTagChunk_TagKwdPopulated(t *testing.T) {
	tokenizer.SetEngineType("infinity")
	t.Cleanup(func() { tokenizer.SetEngineType("") })
	tok := tokenizer.New("English")
	rawExamples := []schema.TagLabel{
		{Content: "retrieval augmented generation vector search database", Tags: []string{"RAG", "Search"}},
		{Content: "deep learning neural network models", Tags: []string{"AI", "DeepLearning"}},
	}
	idx := buildMemoryTagIndex(rawExamples, tok)
	if idx == nil {
		t.Fatal("buildMemoryTagIndex returned nil")
	}

	chunk := map[string]any{
		"content_with_weight": "retrieval augmented generation system with vector search database",
	}

	res := matchAndTagChunk(chunk, idx, tok, 5)
	if res == nil {
		t.Fatal("expected matchAndTagChunk to succeed and return tagged chunk")
	}

	// Verify chunk["tag_feas"] (common.TAG_FLD) is populated as OrderedTagWeights
	feas, ok := chunk[common.TAG_FLD].(OrderedTagWeights)
	if !ok || len(feas) == 0 {
		t.Fatalf("expected chunk[%q] to be non-empty OrderedTagWeights, got %T (%v)", common.TAG_FLD, chunk[common.TAG_FLD], chunk[common.TAG_FLD])
	}

	// Verify chunk["tag_kwd"] is populated as []string
	kwds, ok := chunk["tag_kwd"].([]string)
	if !ok || len(kwds) == 0 {
		t.Fatalf("expected chunk['tag_kwd'] to be non-empty []string, got %T (%v)", chunk["tag_kwd"], chunk["tag_kwd"])
	}

	// Verify lengths match
	if len(kwds) != len(feas) {
		t.Fatalf("tag_kwd length (%d) != tag_feas length (%d)", len(kwds), len(feas))
	}

	// Verify every tag in tag_kwd is present in tag_feas
	for _, k := range kwds {
		if _, exists := feas[k]; !exists {
			t.Errorf("tag %q in tag_kwd not found in tag_feas %v", k, feas)
		}
	}

	// Verify res.Tags matches chunk["tag_kwd"]
	if len(res.Tags) != len(kwds) {
		t.Fatalf("res.Tags length (%d) != chunk['tag_kwd'] length (%d)", len(res.Tags), len(kwds))
	}
	for i, tag := range res.Tags {
		if tag != kwds[i] {
			t.Errorf("res.Tags[%d] = %q, want %q", i, tag, kwds[i])
		}
	}
}

func TestMatchAndTagChunk_ChunkTFSaliencyGradient(t *testing.T) {
	tokenizer.SetEngineType("infinity")
	t.Cleanup(func() { tokenizer.SetEngineType("") })
	tok := tokenizer.New("English")

	// Construct a corpus where Topic A and Topic B have equal background prior frequency
	var rawExamples []schema.TagLabel
	// Topic A examples (Kubernetes / Container Orchestration)
	rawExamples = append(rawExamples,
		schema.TagLabel{Content: "kubernetes cluster container orchestration pod management", Tags: []string{"TopicA"}},
		schema.TagLabel{Content: "kubernetes deployment scaling service mesh ingress traffic", Tags: []string{"TopicA"}},
		schema.TagLabel{Content: "kubernetes persistent volume storage node scheduling workload", Tags: []string{"TopicA"}},
	)
	// Topic B examples (Database / SQL Optimization)
	rawExamples = append(rawExamples,
		schema.TagLabel{Content: "database relational sql table query indexing optimization", Tags: []string{"TopicB"}},
		schema.TagLabel{Content: "database transaction isolation levels acid compliance locking", Tags: []string{"TopicB"}},
		schema.TagLabel{Content: "database replication sharding clustering high availability", Tags: []string{"TopicB"}},
	)
	// Background corpus to establish balanced priors
	for i := 0; i < 20; i++ {
		rawExamples = append(rawExamples, schema.TagLabel{
			Content: fmt.Sprintf("background general cloud computing infrastructure component %d", i),
			Tags:    []string{"CloudInfra"},
		})
	}

	idx := buildMemoryTagIndex(rawExamples, tok)
	if idx == nil {
		t.Fatal("buildMemoryTagIndex returned nil")
	}

	// Chunk heavily discusses Topic A (matching multiple Topic A examples / repeated keyword facets)
	// and only briefly mentions Topic B (matching 1 Topic B example).
	chunkText := strings.Join([]string{
		"Comprehensive guide to kubernetes cluster container orchestration pod management in production.",
		"Configuring kubernetes deployment scaling service mesh ingress traffic for microservices.",
		"Managing kubernetes persistent volume storage node scheduling workload distribution across nodes.",
		"Kubernetes cluster container orchestration deployment scaling persistent volume storage.",
		"Note: basic database relational sql table query indexing optimization is used for logging.",
	}, " ")

	chunk := map[string]any{
		"content_with_weight": chunkText,
	}

	res := matchAndTagChunk(chunk, idx, tok, 5)
	if res == nil {
		t.Fatal("expected matchAndTagChunk to succeed")
	}

	feas, ok := chunk[common.TAG_FLD].(OrderedTagWeights)
	if !ok {
		t.Fatalf("expected OrderedTagWeights, got %T", chunk[common.TAG_FLD])
	}

	scoreA, hasA := feas["TopicA"]
	scoreB, hasB := feas["TopicB"]
	if !hasA || !hasB {
		t.Fatalf("expected both TopicA and TopicB in tag results, got: %v", feas)
	}

	t.Logf("Chunk TF Saliency Gradient Results: TopicA=%d, TopicB=%d (all tags: %v)", scoreA, scoreB, feas)

	// Topic A should receive a substantially higher score (7~9) than Topic B (4~5),
	// confirming that chunk-side TF salience / multi-hit accumulation breaks score ties.
	if scoreA < 7 || scoreA > 9 {
		t.Errorf("expected TopicA score in range [7, 9], got %d", scoreA)
	}
	if scoreB < 4 || scoreB > 5 {
		t.Errorf("expected TopicB score in range [4, 5], got %d", scoreB)
	}
	if scoreA <= scoreB {
		t.Errorf("expected TopicA score (%d) to be substantially higher than TopicB score (%d)", scoreA, scoreB)
	}
}

func TestMatchAndTagChunk_RankDecayScoreDistribution(t *testing.T) {
	tokenizer.SetEngineType("infinity")
	t.Cleanup(func() { tokenizer.SetEngineType("") })
	tok := tokenizer.New("English")
	// Construct a corpus with high-frequency common tags, medium-frequency tags, and rare tags
	var rawExamples []schema.TagLabel
	for i := 0; i < 50; i++ {
		rawExamples = append(rawExamples, schema.TagLabel{
			Content: fmt.Sprintf("common general engineering document text number %d", i),
			Tags:    []string{"GeneralEngineering"},
		})
	}
	// Distinct medium-frequency topic examples
	rawExamples = append(rawExamples,
		schema.TagLabel{
			Content: "database relational sql query indexing optimization",
			Tags:    []string{"Database"},
		},
		schema.TagLabel{
			Content: "database transaction acid isolation levels concurrency control",
			Tags:    []string{"Database"},
		},
		schema.TagLabel{
			Content: "database replication clustering high availability failover",
			Tags:    []string{"Database"},
		},
	)
	// Rare/specific topic examples
	rawExamples = append(rawExamples,
		schema.TagLabel{
			Content: "retrieval augmented generation vector search algorithm indexing",
			Tags:    []string{"RareRAG"},
		},
		schema.TagLabel{
			Content: "retrieval augmented generation embedding chunk retrieval ranker",
			Tags:    []string{"RareRAG"},
		},
	)

	idx := buildMemoryTagIndex(rawExamples, tok)
	if idx == nil {
		t.Fatal("buildMemoryTagIndex returned nil")
	}

	// Chunk containing repeated terms that match specific/rare tag (dominant topic) and 1 mention of medium tag
	chunk := map[string]any{
		"content_with_weight": strings.Join([]string{
			"retrieval augmented generation vector search algorithm indexing",
			"retrieval augmented generation embedding chunk retrieval ranker",
			"retrieval augmented generation vector search algorithm indexing",
			"database relational sql query indexing optimization",
		}, " "),
	}

	res := matchAndTagChunk(chunk, idx, tok, 10)
	if res == nil {
		t.Fatal("expected matchAndTagChunk to succeed")
	}

	feas, ok := chunk[common.TAG_FLD].(OrderedTagWeights)
	if !ok || len(feas) < 1 {
		t.Fatalf("expected at least 1 scored tag, got %d (%v)", len(feas), chunk[common.TAG_FLD])
	}

	// Verify all scores are within [1, 10]
	scores := make(map[string]int)
	for tag, score := range feas {
		if score < 1 || score > 10 {
			t.Errorf("tag %q score %d out of bounds [1, 10]", tag, score)
		}
		scores[tag] = score
	}

	t.Logf("RankDecayScoreDistribution Scored Tags: %v", feas)

	// Rare tag with high lift and high coverage should reach top band (>= 8)
	rareScore, hasRare := scores["RareRAG"]
	if !hasRare {
		t.Fatal("expected RareRAG in scored tags")
	}
	if rareScore < 8 || rareScore > 10 {
		t.Errorf("expected RareRAG score in top band [8, 10], got %d", rareScore)
	}

	// Verify active score gradients across [1, 10]:
	// Medium-frequency tag should receive a lower score than RareRAG
	if dbScore, hasDB := scores["Database"]; hasDB {
		if dbScore >= rareScore {
			t.Errorf("expected Database score (%d) to be lower than RareRAG (%d)", dbScore, rareScore)
		}
	}
}

func TestGetChunkText_NoTitlePollution(t *testing.T) {
	// Case 1: When content_with_weight is present, docnm_kwd should NOT be prepended
	chunkWithContent := map[string]any{
		"docnm_kwd":           "Financial_Report_2026.pdf",
		"content_with_weight": "Quarterly revenue increased by 15 percent.",
	}
	got := getChunkText(chunkWithContent)
	want := "Quarterly revenue increased by 15 percent."
	if got != want {
		t.Errorf("getChunkText with content_with_weight = %q, want %q", got, want)
	}

	// Case 2: When text is present (fallback), docnm_kwd should NOT be prepended
	chunkWithText := map[string]any{
		"docnm_kwd": "Technical_Specification.docx",
		"text":      "System architecture overview and requirements.",
	}
	gotText := getChunkText(chunkWithText)
	wantText := "System architecture overview and requirements."
	if gotText != wantText {
		t.Errorf("getChunkText with text = %q, want %q", gotText, wantText)
	}

	// Case 3: When important_kwd is present alongside content, keywords are appended but title is ignored
	chunkWithKwds := map[string]any{
		"docnm_kwd":           "Internal_Guidelines.pdf",
		"important_kwd":       []string{"Security", "Compliance"},
		"content_with_weight": "All employees must follow access protocols.",
	}
	gotKwds := getChunkText(chunkWithKwds)
	wantKwds := "All employees must follow access protocols. Security Compliance"
	if gotKwds != wantKwds {
		t.Errorf("getChunkText with keywords = %q, want %q", gotKwds, wantKwds)
	}

	// Case 4: Title is used ONLY when both content and text are absent
	chunkTitleOnly := map[string]any{
		"docnm_kwd": "User_Manual_v2.pdf",
	}
	gotTitle := getChunkText(chunkTitleOnly)
	wantTitle := "User_Manual_v2"
	if gotTitle != wantTitle {
		t.Errorf("getChunkText title only = %q, want %q", gotTitle, wantTitle)
	}
}

func TestProbabilityMassConservation_MultiTag(t *testing.T) {
	tokenizer.SetEngineType("infinity")
	t.Cleanup(func() { tokenizer.SetEngineType("") })
	tok := tokenizer.New("English")
	// Example 1 has 3 tags: TagA, TagB, TagC
	// Example 2 has 1 tag: TagD
	rawExamples := []schema.TagLabel{
		{
			Content: "quantum computing quantum algorithms superposition",
			Tags:    []string{"TagA", "TagB", "TagC"},
		},
		{
			Content: "quantum computing quantum circuits entanglement",
			Tags:    []string{"TagD"},
		},
	}
	idx := buildMemoryTagIndex(rawExamples, tok)
	if idx == nil {
		t.Fatal("buildMemoryTagIndex returned nil")
	}

	// Check that background prior totalTagCount counted all tags properly
	if len(idx.allTags) != 4 {
		t.Fatalf("expected 4 tags in allTags, got %d", len(idx.allTags))
	}

	// Match a chunk that matches Example 1 with 100% coverage
	chunk := map[string]any{
		"content_with_weight": "quantum computing quantum algorithms superposition",
	}

	res := matchAndTagChunk(chunk, idx, tok, 10)
	if res == nil {
		t.Fatal("expected matchAndTagChunk to succeed")
	}

	feas := chunk[common.TAG_FLD].(OrderedTagWeights)
	// Because Example 1 has TagA, TagB, TagC, each receives 1/3 of the coverage weight.
	// Since all 3 tags have identical background frequency in this corpus, they should receive identical scores.
	scoreA, okA := feas["TagA"]
	scoreB, okB := feas["TagB"]
	scoreC, okC := feas["TagC"]
	if !okA || !okB || !okC {
		t.Fatalf("expected TagA, TagB, TagC to be present in %v", feas)
	}
	if scoreA != scoreB || scoreB != scoreC {
		t.Errorf("expected equal scores across multi-tag example: TagA=%d, TagB=%d, TagC=%d", scoreA, scoreB, scoreC)
	}
}

func TestSampleWithoutReplacement(t *testing.T) {
	// Empty slice
	if got := sampleWithoutReplacement(nil, 2, 42); got != nil {
		t.Fatalf("expected nil for empty slice, got %v", got)
	}
	// k <= 0
	items := []schema.TaggedChunk{{Content: "c1"}, {Content: "c2"}, {Content: "c3"}}
	if got := sampleWithoutReplacement(items, 0, 42); got != nil {
		t.Fatalf("expected nil for k=0, got %v", got)
	}
	// k >= len(slice)
	if got := sampleWithoutReplacement(items, 5, 42); len(got) != 3 {
		t.Fatalf("expected 3 items when k >= len, got %d", len(got))
	}
	// Verify slice length > 1 never returns duplicate elements
	slice2 := []schema.TaggedChunk{
		{Content: "item A"},
		{Content: "item B"},
	}
	for run := 0; run < 50; run++ {
		picked := sampleWithoutReplacement(slice2, 2, int64(run*17+3))
		if len(picked) != 2 {
			t.Fatalf("expected 2 items, got %d", len(picked))
		}
		if picked[0].Content == picked[1].Content {
			t.Fatalf("duplicate element returned from 2-element slice: %s", picked[0].Content)
		}
	}
	// Verify deterministic reproducibility: same seed ALWAYS produces identical pick
	seedA := int64(123456789)
	pick1 := sampleWithoutReplacement(slice2, 2, seedA)
	pick2 := sampleWithoutReplacement(slice2, 2, seedA)
	if pick1[0].Content != pick2[0].Content || pick1[1].Content != pick2[1].Content {
		t.Fatalf("expected deterministic sampling for identical seed: %v vs %v", pick1, pick2)
	}

	// k < len(slice): test repeated runs never have duplicates
	bigSlice := make([]schema.TaggedChunk, 20)
	for i := range bigSlice {
		bigSlice[i] = schema.TaggedChunk{Content: fmt.Sprintf("chunk-%d", i)}
	}
	for run := 0; run < 100; run++ {
		picked := sampleWithoutReplacement(bigSlice, 5, int64(run*31+7))
		if len(picked) != 5 {
			t.Fatalf("expected 5 items, got %d", len(picked))
		}
		seen := make(map[string]bool)
		for _, item := range picked {
			if seen[item.Content] {
				t.Fatalf("duplicate item found in sampleWithoutReplacement: %s", item.Content)
			}
			seen[item.Content] = true
		}
	}
}

func TestTaggerCacheKey_IncludesFewShot(t *testing.T) {
	allTags := map[string]float64{"TagA": 0.5, "TagB": 0.5}
	ex1 := []schema.TaggedChunk{
		{Content: "sample one", TagWeights: map[string]int{"TagA": 8}},
	}
	ex2 := []schema.TaggedChunk{
		{Content: "sample two", TagWeights: map[string]int{"TagB": 7}},
	}
	ex3 := []schema.TaggedChunk{
		{Content: "sample one", TagWeights: map[string]int{"TagA": 5}},
	}

	k1 := taggerCacheKey("llm-1", "test text", allTags, ex1, 3)
	k2 := taggerCacheKey("llm-1", "test text", allTags, ex2, 3)
	k3 := taggerCacheKey("llm-1", "test text", allTags, ex3, 3)
	kEmpty := taggerCacheKey("llm-1", "test text", allTags, nil, 3)

	if k1 == k2 {
		t.Fatalf("expected different cache keys for different few-shot examples: %s vs %s", k1, k2)
	}
	if k1 == k3 {
		t.Fatalf("expected different cache keys for different tag weights: %s vs %s", k1, k3)
	}
	if k1 == kEmpty {
		t.Fatalf("expected different cache keys for few-shot vs empty: %s vs %s", k1, kEmpty)
	}

	// Identical few-shot examples produce identical key
	k1Dup := taggerCacheKey("llm-1", "test text", allTags, ex1, 3)
	if k1 != k1Dup {
		t.Fatalf("expected identical cache keys for same few-shot examples: %s vs %s", k1, k1Dup)
	}
}

func TestMatchAndTagChunk_ShortExampleMultiTokenRequirement(t *testing.T) {
	tokenizer.SetEngineType("infinity")
	t.Cleanup(func() { tokenizer.SetEngineType("") })
	tok := tokenizer.New("English")

	// Create multi-token examples with balanced background frequency
	rawExamples := []schema.TagLabel{
		{Content: "machine learning", Tags: []string{"ML"}},
		{Content: "machine vision", Tags: []string{"MV"}},
		{Content: "deep learning", Tags: []string{"DL"}},
		{Content: "quantum machine", Tags: []string{"QM"}},
		{Content: "active learning", Tags: []string{"AL"}},
	}
	idx := buildMemoryTagIndex(rawExamples, tok)
	if idx == nil {
		t.Fatal("buildMemoryTagIndex returned nil")
	}

	// Chunk 1: Matches only 1 token ("machine") of the 2-token example "machine learning"
	// Should NOT match on only 1 token
	chunkSingleMatch := map[string]any{
		"content_with_weight": "the coffee machine is broken today",
	}
	res1 := matchAndTagChunk(chunkSingleMatch, idx, tok, 5)
	if res1 != nil {
		if _, ok := res1.TagWeights["ML"]; ok {
			t.Fatalf("expected 1-token match on 2-token example 'machine learning' not to match, got %v", res1.Tags)
		}
	}

	// Chunk 2: Matches both tokens ("machine learning")
	// Should match
	chunkFullMatch := map[string]any{
		"content_with_weight": "practical machine learning models and training",
	}
	res2 := matchAndTagChunk(chunkFullMatch, idx, tok, 5)
	if res2 == nil {
		t.Fatal("expected matchAndTagChunk to succeed for 2-token match")
	}
	if _, ok := res2.TagWeights["ML"]; !ok {
		t.Fatalf("expected ML tag for full 2-token match, got: %v", res2.TagWeights)
	}
}

func TestParseXLSXTagSource_SingleCellAccumulation(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(f.SetCellValue("Sheet1", "A1", "Introductory title row"))
	must(f.SetCellValue("Sheet1", "A2", "Second paragraph of text"))
	must(f.SetCellValue("Sheet1", "A3", "Third paragraph body"))
	must(f.SetCellValue("Sheet1", "B3", "TagX, TagY"))
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}

	result := parseXLSXTagSource(buf.Bytes())
	if len(result) != 1 {
		t.Fatalf("expected 1 accumulated example, got %d", len(result))
	}
	wantContent := "Introductory title row\nSecond paragraph of text\nThird paragraph body"
	if result[0].Content != wantContent {
		t.Errorf("content = %q, want %q", result[0].Content, wantContent)
	}
	if len(result[0].Tags) != 2 || result[0].Tags[0] != "TagX" || result[0].Tags[1] != "TagY" {
		t.Errorf("tags = %v", result[0].Tags)
	}
}

func TestDetectCSVDelimiterBytes_QuoteAware(t *testing.T) {
	// Comma-separated with embedded quotes containing commas
	quotedComma := []byte("\"City, Country\",Geography\n\"Name, Title\",Personnel\n")
	delim := detectCSVDelimiterBytes(quotedComma)
	if delim != "," {
		t.Fatalf("expected delimiter ',' for quoted comma CSV, got %q", delim)
	}

	// Tab-separated with embedded comma in tag column
	tabData := []byte("Document Title\tTag1,Tag2\nAnother Document\tTag3,Tag4\n")
	delimTab := detectCSVDelimiterBytes(tabData)
	if delimTab != "\t" {
		t.Fatalf("expected delimiter '\\t' for TSV, got %q", delimTab)
	}
}

func TestIsHighConfidenceMatch_Phase1Isolation(t *testing.T) {
	lowConf := &schema.TaggedChunk{
		Content:    "low confidence sample",
		TagWeights: OrderedTagWeights{"tag1": 3, "tag2": 5},
	}
	if isHighConfidenceMatch(lowConf) {
		t.Fatal("expected isHighConfidenceMatch to be false for scores < 6")
	}

	highConf := &schema.TaggedChunk{
		Content:    "high confidence sample",
		TagWeights: OrderedTagWeights{"tag1": 4, "tag2": 6},
	}
	if !isHighConfidenceMatch(highConf) {
		t.Fatal("expected isHighConfidenceMatch to be true for score >= 6")
	}

	if isHighConfidenceMatch(nil) {
		t.Fatal("expected false for nil chunk")
	}
}

func TestDetectTextLanguage_CJK_Kana_Hangul(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "Japanese with Hiragana",
			text: "これは日本語のテキストです。",
			want: "Japanese",
		},
		{
			name: "Japanese with Katakana and Kanji",
			text: "ベクトルデータベースの検索システム",
			want: "Japanese",
		},
		{
			name: "Korean with Hangul",
			text: "안녕하세요. 이것은 한국어 텍스트입니다.",
			want: "Korean",
		},
		{
			name: "Simplified Chinese",
			text: "这是一个纯中文的测试文本。",
			want: "Chinese",
		},
		{
			name: "Traditional Chinese",
			text: "這是一個繁體中文測試文檔。",
			want: "Chinese",
		},
		{
			name: "English",
			text: "This is an English text without any Asian characters.",
			want: "English",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := []map[string]any{
				{"content_with_weight": tt.text},
			}
			got := detectTextLanguage(chunks)
			if got != tt.want {
				t.Errorf("detectTextLanguage(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestMatchAndTagChunk_RankDecaySalienceBoost(t *testing.T) {
	tokenizer.SetEngineType("infinity")
	t.Cleanup(func() { tokenizer.SetEngineType("") })
	tok := tokenizer.New("English")

	rawExamples := []schema.TagLabel{
		{Content: "vector database storage", Tags: []string{"Storage"}},
		{Content: "vector database indexing", Tags: []string{"Indexing"}},
	}
	idx := buildMemoryTagIndex(rawExamples, tok)
	if idx == nil {
		t.Fatal("buildMemoryTagIndex returned nil")
	}

	// Chunk has "indexing" repeated 5 times
	chunk := map[string]any{
		"content_with_weight": "vector database indexing indexing indexing indexing indexing",
	}

	res := matchAndTagChunk(chunk, idx, tok, 5)
	if res == nil {
		t.Fatal("expected matchAndTagChunk to succeed")
	}

	weights := chunk[common.TAG_FLD].(OrderedTagWeights)
	if weights["Indexing"] <= weights["Storage"] {
		t.Fatalf("expected Indexing score (%d) > Storage score (%d) due to TF boost", weights["Indexing"], weights["Storage"])
	}
}

func TestMatchAndTagChunk_RankDecayGradientScores(t *testing.T) {
	tokenizer.SetEngineType("infinity")
	t.Cleanup(func() { tokenizer.SetEngineType("") })
	tok := tokenizer.New("English")

	var rawExamples []schema.TagLabel
	// Background corpus
	for i := 0; i < 50; i++ {
		rawExamples = append(rawExamples, schema.TagLabel{
			Content: fmt.Sprintf("general common document text number %d", i),
			Tags:    []string{"GeneralDocs"},
		})
	}
	// Common peripheral topic in background
	for i := 0; i < 8; i++ {
		rawExamples = append(rawExamples, schema.TagLabel{
			Content: fmt.Sprintf("unrelated cloud telemetry logging metrics cluster node %d", i),
			Tags:    []string{"CloudInfra"},
		})
	}
	rawExamples = append(rawExamples,
		schema.TagLabel{
			Content: "cloud deployment devops kubernetes",
			Tags:    []string{"CloudInfra"},
		},
	)
	// Secondary topic (moderate background)
	for i := 0; i < 2; i++ {
		rawExamples = append(rawExamples, schema.TagLabel{
			Content: fmt.Sprintf("unrelated database transaction internals locking protocol %d", i),
			Tags:    []string{"Database"},
		})
	}
	rawExamples = append(rawExamples,
		schema.TagLabel{
			Content: "database storage query index optimization postgresql",
			Tags:    []string{"Database"},
		},
	)
	// Primary topic (specific domain with high prominence)
	rawExamples = append(rawExamples,
		schema.TagLabel{
			Content: "neural network deep learning transformer model training",
			Tags:    []string{"DeepLearning"},
		},
		schema.TagLabel{
			Content: "deep learning transformer attention mechanism architecture",
			Tags:    []string{"DeepLearning"},
		},
	)

	idx := buildMemoryTagIndex(rawExamples, tok)
	if idx == nil {
		t.Fatal("buildMemoryTagIndex returned nil")
	}

	// Chunk has high prominence with repeated terms for DeepLearning, secondary support for Database, minor mention of CloudInfra
	chunk := map[string]any{
		"content_with_weight": "neural network deep learning transformer model training deep learning transformer attention mechanism architecture deep learning transformer neural network deep learning transformer deep learning transformer database storage query index optimization postgresql cloud deployment devops kubernetes",
	}

	res := matchAndTagChunk(chunk, idx, tok, 10)
	if res == nil {
		t.Fatal("expected matchAndTagChunk to succeed")
	}

	weights := chunk[common.TAG_FLD].(OrderedTagWeights)
	dlScore := weights["DeepLearning"]
	dbScore := weights["Database"]
	cloudScore := weights["CloudInfra"]

	t.Logf("Scores: DeepLearning=%d, Database=%d, CloudInfra=%d, All=%v", dlScore, dbScore, cloudScore, weights)

	if dlScore < 8 {
		t.Errorf("expected high-prominence DeepLearning score >= 8, got %d", dlScore)
	}
	if dbScore < 5 || dbScore > 7 {
		t.Errorf("expected secondary Database score in 5~7 range, got %d", dbScore)
	}
	if cloudScore < 3 || cloudScore > 5 {
		t.Errorf("expected peripheral CloudInfra score in 3~5 range, got %d", cloudScore)
	}
	if dlScore <= dbScore {
		t.Errorf("expected primary DeepLearning (%d) > secondary Database (%d)", dlScore, dbScore)
	}
	if dbScore <= cloudScore {
		t.Errorf("expected secondary Database (%d) > peripheral CloudInfra (%d)", dbScore, cloudScore)
	}
}
