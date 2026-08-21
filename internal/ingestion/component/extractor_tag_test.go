package component

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

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
	return strings.Repeat("RAGFlow is an open source retrieval augmented generation engine. ", 10)
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
	requireTokenizerPool(t)
	capt := pushCapturingTagChat(t)

	tok := tokenizer.New("english")
	rawEx := []schema.TagLabel{
		{Content: "real sample text one", Tags: []string{"NLP", "AI"}},
		{Content: "real sample text two", Tags: []string{"Search"}},
	}
	idx := buildMemoryTagIndex(rawEx, tok)
	if idx == nil {
		t.Fatal("expected non-nil index")
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

func requireTokenizerPool(t testing.TB) {
	t.Helper()
	if err := tokenizer.Init(&tokenizer.PoolConfig{
		DictPath:       "/usr/share/infinity/resource",
		MinSize:        1,
		MaxSize:        2,
		IdleTimeout:    30 * time.Second,
		AcquireTimeout: 5 * time.Second,
	}); err != nil {
		t.Skipf("tokenizer pool unavailable: %v", err)
	}
}

func TestBuildMemoryTagIndex(t *testing.T) {
	requireTokenizerPool(t)
	tok := tokenizer.New("english")
	raw := []schema.TagLabel{
		{Content: "machine learning models", Tags: []string{"ML.v1", "AI"}},
		{Content: "deep learning neural nets", Tags: []string{"AI", "DL"}},
		{Content: "   ", Tags: []string{"Ignored"}},
	}
	idx := buildMemoryTagIndex(raw, tok)
	if idx == nil {
		t.Fatal("expected non-nil MemoryTagIndex")
	}
	if len(idx.examples) != 2 {
		t.Fatalf("expected 2 clean examples, got %d", len(idx.examples))
	}
	if _, ok := idx.allTags["ML_v1"]; !ok {
		t.Fatalf("expected tag ML_v1 after dot sanitization, got %v", idx.allTags)
	}
	if _, ok := idx.allTags["ML.v1"]; ok {
		t.Fatal("expected ML.v1 with dot to be replaced")
	}
}

func TestMatchAndTagChunk_AsymmetricLength(t *testing.T) {
	requireTokenizerPool(t)
	tok := tokenizer.New("english")
	// Reference set: 10-word rare example
	rawEx := []schema.TagLabel{
		{Content: "RAGFlow vector database retrieval architecture engine", Tags: []string{"RAG", "VectorDB"}},
		{Content: "general culinary cooking recipe baking", Tags: []string{"Cooking"}},
	}
	idx := buildMemoryTagIndex(rawEx, tok)
	if idx == nil {
		t.Fatal("expected non-nil index")
	}

	// 800-word long technical chunk containing the reference example words
	longBody := strings.Repeat("RAGFlow is an advanced system that integrates vector database and retrieval architecture engine into scalable workflows. ", 40)
	chunk := map[string]any{"content_with_weight": longBody}

	matched := matchAndTagChunk(chunk, idx, tok, 5)
	if matched == nil {
		t.Fatal("expected non-nil matched chunk for asymmetric length matching")
	}
	if len(matched.Tags) == 0 {
		t.Fatal("expected non-empty matched tags")
	}
	if matched.TagWeights["RAG"] <= 0 || matched.TagWeights["VectorDB"] <= 0 {
		t.Fatalf("expected positive scores for RAG and VectorDB, got: %v", matched.TagWeights)
	}
	for tag, score := range matched.TagWeights {
		if score < 1 || score > 10 {
			t.Fatalf("tag %s score %d is outside [1, 10]", tag, score)
		}
	}
}

func TestMatchAndTagChunk_TopKWeighted(t *testing.T) {
	requireTokenizerPool(t)
	tok := tokenizer.New("english")
	rawEx := []schema.TagLabel{
		{Content: "machine learning artificial intelligence deep neural networks", Tags: []string{"AI"}},
		{Content: "financial market stocks bonds banking economy", Tags: []string{"Finance"}},
	}
	idx := buildMemoryTagIndex(rawEx, tok)
	if idx == nil {
		t.Fatal("expected non-nil index")
	}

	// Chunk has high overlap with AI example, slight overlap with Finance
	chunk := map[string]any{
		"content_with_weight": "machine learning artificial intelligence deep neural networks banking financial",
	}

	matched := matchAndTagChunk(chunk, idx, tok, 5)
	if matched == nil {
		t.Fatal("expected non-nil matched chunk")
	}
	aiScore := matched.TagWeights["AI"]
	finScore := matched.TagWeights["Finance"]
	if aiScore <= finScore {
		t.Fatalf("expected AI score (%d) > Finance score (%d)", aiScore, finScore)
	}
}

func TestMatchAndTagChunk_DuplicateTagsDedup(t *testing.T) {
	requireTokenizerPool(t)
	tok := tokenizer.New("english")
	rawEx := []schema.TagLabel{
		{Content: "natural language processing text analysis", Tags: []string{"NLP", "NLP", "AI"}},
	}
	idx := buildMemoryTagIndex(rawEx, tok)
	if idx == nil {
		t.Fatal("expected non-nil index")
	}
	if len(idx.examples[0].Tags) != 2 {
		t.Fatalf("expected 2 unique tags in example, got %v", idx.examples[0].Tags)
	}

	chunk := map[string]any{"content_with_weight": "natural language processing text analysis"}
	matched := matchAndTagChunk(chunk, idx, tok, 5)
	if matched == nil {
		t.Fatal("expected non-nil match")
	}
	if matched.TagWeights["NLP"] < 1 || matched.TagWeights["NLP"] > 10 {
		t.Fatalf("invalid score for NLP: %d", matched.TagWeights["NLP"])
	}
}

func TestMatchAndTagChunk_AllEmptyTags(t *testing.T) {
	requireTokenizerPool(t)
	tok := tokenizer.New("english")
	rawEx := []schema.TagLabel{
		{Content: "some text without tags", Tags: []string{}},
		{Content: "another text with empty tag", Tags: []string{"", "  "}},
	}
	idx := buildMemoryTagIndex(rawEx, tok)
	if idx != nil {
		t.Fatalf("expected nil index when all tags are empty, got %v", idx)
	}

	chunk := map[string]any{"content_with_weight": "some text without tags"}
	matched := matchAndTagChunk(chunk, idx, tok, 5)
	if matched != nil {
		t.Fatalf("expected nil match when index is nil, got %v", matched)
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

func TestMatchAndTagChunk_DeterministicTieBreak(t *testing.T) {
	requireTokenizerPool(t)
	tok := tokenizer.New("english")
	rawEx := []schema.TagLabel{
		{Content: "shared keyword match document", Tags: []string{"tag_b", "tag_a", "tag_c"}},
	}
	idx := buildMemoryTagIndex(rawEx, tok)
	if idx == nil {
		t.Fatal("expected non-nil index")
	}

	chunk := map[string]any{"content_with_weight": "shared keyword match document"}
	// Ask for top 2 out of 3 equal-scoring tags
	matched := matchAndTagChunk(chunk, idx, tok, 2)
	if matched == nil {
		t.Fatal("expected non-nil matched chunk")
	}
	if len(matched.Tags) != 2 {
		t.Fatalf("expected exactly 2 tags, got %d", len(matched.Tags))
	}
	// Alphabetical tie-break: tag_a, tag_b
	if matched.Tags[0] != "tag_a" || matched.Tags[1] != "tag_b" {
		t.Fatalf("expected deterministic alphabetical tie-break ['tag_a', 'tag_b'], got %v", matched.Tags)
	}
}

func BenchmarkMatchAndTagChunk_5000Examples(b *testing.B) {
	requireTokenizerPool(b)
	tok := tokenizer.New("english")
	rawEx := make([]schema.TagLabel, 5000)
	for i := 0; i < 5000; i++ {
		rawEx[i] = schema.TagLabel{
			Content: fmt.Sprintf("sample content document %d with keywords topic%d and subtopic%d for domain categorization", i, i%50, i%100),
			Tags:    []string{fmt.Sprintf("topic_%d", i%50), fmt.Sprintf("subtopic_%d", i%100)},
		}
	}
	idx := buildMemoryTagIndex(rawEx, tok)
	if idx == nil {
		b.Fatal("failed to build index")
	}

	chunk := map[string]any{
		"content_with_weight": "This is a detailed chunk talking about sample content document 42 with keywords topic42 and subtopic42 for domain categorization in practice.",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matchAndTagChunk(chunk, idx, tok, 3)
	}
}
