//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package component

import (
	"context"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/rag/prompts"
	"ragflow/internal/tokenizer"
)

func tocTestChunks(docID string, texts ...string) []map[string]any {
	chunks := make([]map[string]any, len(texts))
	for i, text := range texts {
		chunks[i] = map[string]any{"text": text, "id": common.ChunkID(docID, text)}
	}
	return chunks
}

func useWhitespaceTokenizer(t *testing.T) {
	t.Helper()
	tokenizer.SetEngineType("infinity")
	t.Cleanup(func() { tokenizer.SetEngineType("") })
}

func TestExtractorTOC_AppendsTOCChunk(t *testing.T) {
	useWhitespaceTokenizer(t)
	longTitle := strings.TrimSpace(strings.Repeat("word ", 23))
	stub := withStubChatInvoker(t,
		stubResponse{Content: "```json\n[" +
			`{"title":"Chapter 1 Intro","chunk_id":"0"},` +
			`{"title":"-1","chunk_id":"1"},` +
			`{"chunk_id":"1"},` +
			`{"title":"` + longTitle + `","chunk_id":"1"},` +
			`{"title":"Chapter 2 Methods","chunk_id":"2"},` +
			`{"title":"3.1","chunk_id":"3"},` +
			`{"title":"Section 2.1","chunk_id":3}` +
			"]\n```"},
		stubResponse{Content: `[` +
			`{"level":"1","title":"Chapter 1 Intro"},` +
			`{"level":"1","title":"Chapter 2 Methods"},` +
			`{"level":2,"title":"Section 2.1"}]`},
	)
	chunks := tocTestChunks("doc1", "Chapter 1 Intro", "body one", "Chapter 2 Methods", "Section 2.1 body")
	ids := []string{chunks[0]["id"].(string), chunks[1]["id"].(string), chunks[2]["id"].(string), chunks[3]["id"].(string)}

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID: "gpt-4o-mini",
		TOC:   schema.TOCExtractConfig{Enabled: true},
	}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{"chunks": chunks, "doc_id": "doc1"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got := out["chunks"].([]map[string]any)
	if len(got) != 5 {
		t.Fatalf("chunks len = %d, want 4 content chunks + 1 TOC chunk", len(got))
	}
	if stub.Calls() != 2 {
		t.Fatalf("chat calls = %d, want 1 batch + 1 level call", stub.Calls())
	}
	batchPrompt := stub.requests[0].Messages[len(stub.requests[0].Messages)-1].Content
	if !strings.Contains(batchPrompt, `{"0":"Chapter 1 Intro"}`+"\n"+`{"1":"body one"}`) {
		t.Errorf("batch prompt does not list the chunks as index lines:\n%s", batchPrompt)
	}
	levelPrompt := stub.requests[1].Messages[len(stub.requests[1].Messages)-1].Content
	if levelPrompt != `["Chapter 1 Intro","Chapter 2 Methods","Section 2.1"]` {
		t.Errorf("level prompt = %s", levelPrompt)
	}

	toc := got[4]
	want := `[{"level":"1","title":"Chapter 1 Intro","ids":["` + ids[0] + `","` + ids[1] + `","` + ids[2] + `"]},` +
		`{"level":"1","title":"Chapter 2 Methods","ids":["` + ids[2] + `","` + ids[3] + `"]},` +
		`{"level":"2","title":"Section 2.1","ids":["` + ids[3] + `"]}]`
	if toc["text"] != want {
		t.Errorf("TOC text\n got: %v\nwant: %s", toc["text"], want)
	}
	if toc["id"] != common.ChunkID("doc1", tocChunkIDSeed) {
		t.Errorf("TOC id = %v, want the per-document TOC id", toc["id"])
	}
	if toc["toc_kwd"] != "toc" || toc["available_int"] != 0 || !reflect.DeepEqual(toc["page_num_int"], []int{100000000}) {
		t.Errorf("TOC chunk fields = %+v", toc)
	}
}

// tocChatFunc answers each chat call from its user prompt, because batch
// calls run concurrently and arrive in no fixed order.
type tocChatFunc func(prompt string) string

func (f tocChatFunc) Chat(_ context.Context, req extractorChatRequest) (*extractorChatResponse, error) {
	return &extractorChatResponse{Content: f(req.Messages[len(req.Messages)-1].Content)}, nil
}

func TestExtractorTOC_KeepsChunkIndexesAcrossBatches(t *testing.T) {
	useWhitespaceTokenizer(t)
	chunks := tocTestChunks("doc1", "Part one", "text a", "Part two", "text b")
	ids := []string{chunks[0]["id"].(string), chunks[1]["id"].(string), chunks[2]["id"].(string), chunks[3]["id"].(string)}

	// Size the context so one batch holds exactly two chunks.
	loader := prompts.EmbeddedPromptLoader{}
	system, err := loader.Load("toc_from_text_system")
	if err != nil {
		t.Fatal(err)
	}
	user, err := loader.Load("toc_from_text_user")
	if err != nil {
		t.Fatal(err)
	}
	budget := tokenizer.NumTokensFromString("Part one") + tokenizer.NumTokensFromString("text a")
	ctxLen := 2 * (tokenizer.NumTokensFromString(user+system) + budget)
	SetExtractorContextLengthOverride(func(context.Context, string) int { return ctxLen })
	t.Cleanup(func() { SetExtractorContextLengthOverride(nil) })

	var batches atomic.Int32
	prev := defaultExtractorChatInvoker
	SetExtractorChatInvoker(tocChatFunc(func(prompt string) string {
		switch {
		case strings.Contains(prompt, `{"0":"Part one"}`):
			batches.Add(1)
			return `[{"title":"Part one","chunk_id":"0"},{"title":"-1","chunk_id":"1"}]`
		case strings.Contains(prompt, `{"2":"Part two"}`):
			batches.Add(1)
			return `[{"title":"Part two","chunk_id":"2"},{"title":"-1","chunk_id":"3"}]`
		case prompt == `["Part one","Part two"]`:
			return `[{"level":"1","title":"Part one"},{"level":"1","title":"Part two"}]`
		}
		t.Errorf("unexpected prompt:\n%s", prompt)
		return "[]"
	}))
	t.Cleanup(func() { SetExtractorChatInvoker(prev) })

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID: "gpt-4o-mini",
		TOC:   schema.TOCExtractConfig{Enabled: true},
	}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{"chunks": chunks, "doc_id": "doc1"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if batches.Load() != 2 {
		t.Fatalf("batch calls = %d, want 2", batches.Load())
	}
	got := out["chunks"].([]map[string]any)
	want := `[{"level":"1","title":"Part one","ids":["` + ids[0] + `","` + ids[1] + `","` + ids[2] + `"]},` +
		`{"level":"1","title":"Part two","ids":["` + ids[2] + `","` + ids[3] + `"]}]`
	if got[len(got)-1]["text"] != want {
		t.Errorf("TOC text\n got: %v\nwant: %s", got[len(got)-1]["text"], want)
	}
}

func TestExtractorTOC_RetriesInvalidJSON(t *testing.T) {
	useWhitespaceTokenizer(t)
	stub := withStubChatInvoker(t,
		stubResponse{Content: "no headings here"},
		stubResponse{Content: `[{"title":"Overview","chunk_id":"0"}]`},
		stubResponse{Content: `[{"level":"1","title":"Overview"}]`},
	)
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID: "gpt-4o-mini",
		TOC:   schema.TOCExtractConfig{Enabled: true},
	}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{"chunks": tocTestChunks("doc1", "Overview"), "doc_id": "doc1"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.Calls() != 3 {
		t.Fatalf("chat calls = %d, want 2 batch attempts + 1 level call", stub.Calls())
	}
	retry := stub.requests[1].Messages[len(stub.requests[1].Messages)-1].Content
	if !strings.Contains(retry, "Generated JSON is as following:\nno headings here\nBut exception while loading:") {
		t.Errorf("retry prompt does not carry the invalid reply:\n%s", retry)
	}
	got := out["chunks"].([]map[string]any)
	if len(got) != 2 {
		t.Fatalf("chunks len = %d, want 2", len(got))
	}
}

func TestExtractorTOC_NoHeadingsAppendsNothing(t *testing.T) {
	useWhitespaceTokenizer(t)
	stub := withStubChatInvoker(t,
		stubResponse{Content: `[{"title":"-1","chunk_id":"0"},{"title":"-1","chunk_id":"1"}]`},
	)
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID: "gpt-4o-mini",
		TOC:   schema.TOCExtractConfig{Enabled: true},
	}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{"chunks": tocTestChunks("doc1", "a", "b"), "doc_id": "doc1"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["chunks"].([]map[string]any); len(got) != 2 {
		t.Fatalf("chunks len = %d, want 2 (no TOC chunk)", len(got))
	}
	if stub.Calls() != 1 {
		t.Fatalf("chat calls = %d, want 1 (no level call without titles)", stub.Calls())
	}
}

func TestExtractorTOC_DisabledMakesNoCalls(t *testing.T) {
	stub := withStubChatInvoker(t)
	c := &ExtractorComponent{Param: schema.ExtractorParam{LLMID: "gpt-4o-mini"}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{"chunks": tocTestChunks("doc1", "Chapter 1"), "doc_id": "doc1"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["chunks"].([]map[string]any); len(got) != 1 {
		t.Fatalf("chunks len = %d, want 1", len(got))
	}
	if stub.Calls() != 0 {
		t.Fatalf("chat calls = %d, want 0", stub.Calls())
	}
}

func TestExtractorTOC_MissingDocIDMakesNoCalls(t *testing.T) {
	stub := withStubChatInvoker(t)
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID:    "gpt-4o-mini",
		Keywords: schema.KeywordExtractConfig{Enabled: true, TopN: 3},
		TOC:      schema.TOCExtractConfig{Enabled: true},
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{"chunks": tocTestChunks("doc1", "Chapter 1")})
	if err == nil || !strings.Contains(err.Error(), "doc_id is required") {
		t.Fatalf("err = %v, want doc_id is required", err)
	}
	if stub.Calls() != 0 {
		t.Fatalf("chat calls = %d, want 0", stub.Calls())
	}
}

func TestExtractorTOC_ContextTooSmallFails(t *testing.T) {
	useWhitespaceTokenizer(t)
	stub := withStubChatInvoker(t)
	SetExtractorContextLengthOverride(func(context.Context, string) int { return 10 })
	t.Cleanup(func() { SetExtractorContextLengthOverride(nil) })
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID: "gpt-4o-mini",
		TOC:   schema.TOCExtractConfig{Enabled: true},
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{"chunks": tocTestChunks("doc1", "Chapter 1"), "doc_id": "doc1"})
	if err == nil || !strings.Contains(err.Error(), "model context of 10 tokens leaves no room") {
		t.Fatalf("err = %v, want a context error", err)
	}
	if stub.Calls() != 0 {
		t.Fatalf("chat calls = %d, want 0", stub.Calls())
	}
}

func TestMergeTOCLevels_BadInput(t *testing.T) {
	chunks := tocTestChunks("doc1", "a", "b")
	levels := []any{
		[]any{"1", "From a list"},
		map[string]any{"level": "x", "title": "Bad level"},
		map[string]any{"level": float64(2), "title": "Out of range"},
		map[string]any{"level": "1"},
	}
	titles := []tocTitle{
		{title: "From a list", chunkID: "1"},
		{title: "Bad level", chunkID: "0"},
		{title: "Out of range", chunkID: "9"},
		{title: "No title in reply", chunkID: "0"},
	}
	got := mergeTOCLevels(levels, titles, chunks)
	want := []tocEntry{
		{Level: "1", Title: "From a list", IDs: []string{chunks[1]["id"].(string)}},
		{Level: "2", Title: "Out of range"},
		{Level: "1", Title: "No title in reply", IDs: []string{chunks[0]["id"].(string), chunks[1]["id"].(string)}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeTOCLevels\n got: %+v\nwant: %+v", got, want)
	}
}

// The body chunks after the last heading belong to the last entry.
func TestMergeTOCLevels_LastEntryRunsToLastChunk(t *testing.T) {
	chunks := tocTestChunks("doc1", "Intro", "body one", "body two")
	levels := []any{map[string]any{"level": "1", "title": "Intro"}}
	titles := []tocTitle{{title: "Intro", chunkID: "0"}}
	got := mergeTOCLevels(levels, titles, chunks)
	want := []tocEntry{
		{Level: "1", Title: "Intro", IDs: []string{chunks[0]["id"].(string), chunks[1]["id"].(string), chunks[2]["id"].(string)}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeTOCLevels\n got: %+v\nwant: %+v", got, want)
	}
}

func TestNewExtractorComponent_TOCGroup(t *testing.T) {
	c, err := NewExtractorComponent(map[string]any{"toc": map[string]any{"enabled": true}})
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}
	if !c.(*ExtractorComponent).Param.TOC.Enabled {
		t.Fatal("toc.enabled=true did not enable TOC extraction")
	}
}
