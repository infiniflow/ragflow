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
			name: "title extension stripped with content",
			chunk: map[string]any{
				"docnm_kwd":           "2026_Engineering_Bidding_Doc.pdf",
				"content_with_weight": "contract guidelines",
			},
			want: "2026_Engineering_Bidding_Doc contract guidelines",
		},
		{
			name: "title fallback chain docnm_kwd wins",
			chunk: map[string]any{
				"docnm_kwd":           "doc_name.docx",
				"title_tks":           "ignored_title",
				"content_with_weight": "body",
			},
			want: "doc_name body",
		},
		{
			name: "title fallback to title_tks when docnm_kwd absent",
			chunk: map[string]any{
				"title_tks":           "parsed_title.txt",
				"content_with_weight": "body",
			},
			want: "parsed_title body",
		},
		{
			name: "important_kwd string slice",
			chunk: map[string]any{
				"important_kwd":       []string{"Bidding", "Tender"},
				"content_with_weight": "body",
			},
			want: "Bidding Tender body",
		},
		{
			name: "important_kwd any slice",
			chunk: map[string]any{
				"important_kwd":       []any{"Alpha", "Beta"},
				"content_with_weight": "body",
			},
			want: "Alpha Beta body",
		},
		{
			name: "important_kwd string",
			chunk: map[string]any{
				"important_kwd":       "SingleKey",
				"content_with_weight": "body",
			},
			want: "SingleKey body",
		},
		{
			name: "all sources combined",
			chunk: map[string]any{
				"docnm_kwd":           "Project_Tender_Specification.pdf",
				"important_kwd":       []string{"Procurement", "Compliance"},
				"content_with_weight": "All bidders must follow instructions.",
			},
			want: "Project_Tender_Specification Procurement Compliance All bidders must follow instructions.",
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
