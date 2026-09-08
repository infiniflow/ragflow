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
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	eschema "github.com/cloudwego/eino/schema"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
	"ragflow/internal/utility"
)

// stubExtractorChatInvoker is the test seam for the package-level
// extractorChatInvoker. It records every call (for assertions) and
// returns canned responses configured per-test. Concurrent-safe so
// it can backstop concurrent test cases without rewriting.
type stubExtractorChatInvoker struct {
	mu sync.Mutex

	// responses is consumed in order; remaining entries are returned
	// as the wrap-error. tests set entries == call count they expect.
	responses []stubResponse

	// requests records every call in order. Callers read via lastRequest().
	requests []extractorChatRequest
	calls    atomic.Int32
}

// stubResponse couples a Content value and an Err. tests populate
// either field — Err takes precedence over Content when non-nil.
type stubResponse struct {
	Content string
	Err     error
}

func (s *stubExtractorChatInvoker) Chat(_ context.Context, req extractorChatRequest) (*extractorChatResponse, error) {
	s.calls.Add(1)
	s.mu.Lock()
	s.requests = append(s.requests, req)
	var resp stubResponse
	if len(s.responses) > 0 {
		resp = s.responses[0]
		s.responses = s.responses[1:]
	}
	s.mu.Unlock()
	if resp.Err != nil {
		return nil, resp.Err
	}
	return &extractorChatResponse{Content: resp.Content}, nil
}

// lastRequest returns the most recent recorded request. Callers must hold s.mu.
func (s *stubExtractorChatInvoker) lastRequest() extractorChatRequest {
	if len(s.requests) == 0 {
		return extractorChatRequest{}
	}
	return s.requests[len(s.requests)-1]
}

func (s *stubExtractorChatInvoker) Calls() int32 { return s.calls.Load() }

// withStubChatInvoker installs a stub invoker for the duration of
// the test and restores the production invoker on cleanup.
func withStubChatInvoker(t *testing.T, responses ...stubResponse) *stubExtractorChatInvoker {
	t.Helper()
	prev := defaultExtractorChatInvoker
	stub := &stubExtractorChatInvoker{responses: responses}
	SetExtractorChatInvoker(stub)
	t.Cleanup(func() {
		SetExtractorChatInvoker(prev)
	})
	return stub
}

// TestExtractorComponent_Registered verifies the init() registration
// is visible to the runtime registry.
func TestExtractorComponent_Registered(t *testing.T) {
	factory, cat, md, ok := runtime.DefaultRegistry.Lookup("Extractor")
	if !ok {
		t.Fatal("Extractor not registered in runtime.DefaultRegistry")
	}
	if cat != runtime.CategoryIngestion {
		t.Errorf("category = %q, want %q", cat, runtime.CategoryIngestion)
	}
	if factory == nil {
		t.Error("factory is nil")
	}
	if md.Inputs == nil || len(md.Inputs) == 0 {
		t.Errorf("metadata.Inputs empty: %v", md.Inputs)
	}
	if md.Outputs == nil || len(md.Outputs) == 0 {
		t.Errorf("metadata.Outputs empty: %v", md.Outputs)
	}
	if _, has := md.Outputs["chunks"]; !has {
		t.Errorf("metadata.Outputs missing %q", "chunks")
	}
	if _, has := md.Outputs["output_format"]; !has {
		t.Errorf("metadata.Outputs missing %q", "output_format")
	}
}

// TestExtractorComponent_Invoke_HappyPath covers the per-chunk
// auto-extraction (e.g. summary).
func TestExtractorComponent_Invoke_HappyPath(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: "answer for chunk 1"},
		stubResponse{Content: "answer for chunk 2"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID:   "gpt-4o-mini",
		Summary: schema.SummaryExtractConfig{Enabled: true},
	}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{
			{"text": "first text"},
			{"text": "second text"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	chunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks key missing or wrong shape: %T", out["chunks"])
	}
	if len(chunks) != 2 {
		t.Fatalf("chunks len = %d, want 2", len(chunks))
	}
	s0, _ := chunks[0]["summary"].(string)
	s1, _ := chunks[1]["summary"].(string)
	if !((s0 == "answer for chunk 1" && s1 == "answer for chunk 2") || (s0 == "answer for chunk 2" && s1 == "answer for chunk 1")) {
		t.Errorf("unexpected summaries: chunk0=%q, chunk1=%q", s0, s1)
	}
	if out["output_format"] != "chunks" {
		t.Errorf("output_format = %v, want chunks", out["output_format"])
	}
}

// TestExtractorComponent_Invoke_LLMError verifies a mock LLM
// error is surfaced through Invoke with the component-name prefix.
func TestExtractorComponent_Invoke_LLMError(t *testing.T) {
	prevMax, prevDelay := extractorRetryMax, extractorRetryDelay
	extractorRetryMax, extractorRetryDelay = 3, time.Millisecond
	t.Cleanup(func() {
		extractorRetryMax, extractorRetryDelay = prevMax, prevDelay
	})

	errSentinel := errors.New("upstream llm unavailable")
	withStubChatInvoker(t,
		stubResponse{Err: errSentinel},
		stubResponse{Err: errSentinel},
		stubResponse{Err: errSentinel},
		stubResponse{Err: errSentinel},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID:   "gpt-4o-mini",
		Summary: schema.SummaryExtractConfig{Enabled: true},
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "x"}},
	})
	if err == nil {
		t.Fatal("Invoke returned nil error")
	}
	if !strings.HasPrefix(err.Error(), "extractor:") {
		t.Errorf("error should be wrapped with 'extractor:' prefix, got %v", err)
	}
	if !strings.Contains(err.Error(), "upstream llm unavailable") {
		t.Errorf("error should chain underlying error, got %v", err)
	}
}

// TestExtractorComponent_Invoke_RetrySucceeds verifies that a transient
// LLM error is retried (RetryWithBackoff), and the invocation succeeds
// once the LLM recovers.
func TestExtractorComponent_Invoke_RetrySucceeds(t *testing.T) {
	prevMax, prevDelay := extractorRetryMax, extractorRetryDelay
	extractorRetryMax, extractorRetryDelay = 3, time.Millisecond
	t.Cleanup(func() {
		extractorRetryMax, extractorRetryDelay = prevMax, prevDelay
	})

	stub := withStubChatInvoker(t,
		stubResponse{Err: errors.New("transient")},
		stubResponse{Err: errors.New("transient")},
		stubResponse{Content: "recovered"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID:   "gpt-4o-mini",
		Summary: schema.SummaryExtractConfig{Enabled: true},
	}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "x"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if s, _ := chunks[0]["summary"].(string); s != "recovered" {
		t.Errorf("summary = %q, want recovered", s)
	}
	if calls := stub.Calls(); calls != 3 {
		t.Errorf("calls = %d, want 3 (2 transient + 1 success)", calls)
	}
}

// TestExtractorComponent_Invoke_UnknownProvider asserts the
// production (eino) chat invoker handles an unregistered driver
// without panicking.
func TestExtractorComponent_Invoke_UnknownProvider(t *testing.T) {
	inv := &einoExtractorChatInvoker{}
	resp, err := inv.Chat(context.Background(), extractorChatRequest{
		Driver:    "definitely-not-a-real-provider-xyz",
		ModelName: "anything",
	})
	if err == nil && resp == nil {
		t.Fatal("production invoker returned nil error AND nil response for unknown driver — silent no-op")
	}
	if err != nil {
		if !strings.Contains(err.Error(), "definitely-not-a-real-provider-xyz") &&
			!strings.Contains(err.Error(), "no driver") &&
			!strings.Contains(err.Error(), "unknown") &&
			!strings.Contains(err.Error(), "not implemented") {
			t.Errorf("unknown-driver error should mention the driver name or a typed/typed-sentinel substring; got: %v", err)
		}
	}
}

// TestExtractorComponent_Invoke_EmptyChunksReturnsEmpty verifies that
// when len(in.chunks) == 0, Invoke immediately returns empty chunks.
func TestExtractorComponent_Invoke_EmptyChunksReturnsEmpty(t *testing.T) {
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		Summary: schema.SummaryExtractConfig{Enabled: true},
	}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks missing or wrong shape")
	}
	if len(chunks) != 0 {
		t.Fatalf("chunks len = %d, want 0", len(chunks))
	}
}

// TestExtractorComponent_Invoke_JSONListInput verifies that the chunks list
// can be provided under the "json" key.
func TestExtractorComponent_Invoke_JSONListInput(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: "json chunk summary"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		Summary: schema.SummaryExtractConfig{Enabled: true},
	}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"json": []map[string]any{{"text": "json payload chunk"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok || len(chunks) != 1 {
		t.Fatalf("chunks malformed: %v", out["chunks"])
	}
	if chunks[0]["summary"] != "json chunk summary" {
		t.Errorf("summary = %v, want %q", chunks[0]["summary"], "json chunk summary")
	}
}

// TestExtractorComponent_Invoke_PerCallLLMIDOverride verifies an
// inputs["llm_id"] override wins over Param.LLMID.
func TestExtractorComponent_Invoke_PerCallLLMIDOverride(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "summary result"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID:   "static-llm",
		Summary: schema.SummaryExtractConfig{Enabled: true},
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "sample text"}},
		"llm_id": "override-llm",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.lastRequest().ModelName != "override-llm" {
		t.Errorf("ModelName = %q, want override-llm", stub.lastRequest().ModelName)
	}
}

// TestExtractorComponent_Invoke_CompositeLLMID verifies the
// composite "gpt-4o-mini@openai" form is split into driver and model.
func TestExtractorComponent_Invoke_CompositeLLMID(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "summary result"},
	)
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID:   "gpt-4o-mini@openai",
		Summary: schema.SummaryExtractConfig{Enabled: true},
	}}
	if _, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "sample text"}},
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.lastRequest().Driver != "openai" {
		t.Errorf("Driver = %q, want openai", stub.lastRequest().Driver)
	}
	if stub.lastRequest().ModelName != "gpt-4o-mini" {
		t.Errorf("ModelName = %q, want gpt-4o-mini", stub.lastRequest().ModelName)
	}
}

// TestExtractorComponent_Invoke_ChunkIndexInError verifies the
// error message includes the failing chunk index.
func TestExtractorComponent_Invoke_ChunkIndexInError(t *testing.T) {
	prevMax, prevDelay := extractorRetryMax, extractorRetryDelay
	extractorRetryMax, extractorRetryDelay = 3, time.Millisecond
	t.Cleanup(func() {
		extractorRetryMax, extractorRetryDelay = prevMax, prevDelay
	})

	errBoom := errors.New("chunk-1-boom")
	withStubChatInvoker(t,
		stubResponse{Content: "ok for chunk 0"},
		stubResponse{Err: errBoom},
		stubResponse{Err: errBoom},
		stubResponse{Err: errBoom},
		stubResponse{Err: errBoom},
	)
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		Summary: schema.SummaryExtractConfig{Enabled: true},
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{
			{"text": "first"},
			{"text": "second"},
		},
	})
	if err == nil {
		t.Fatal("Invoke returned nil error")
	}
	if !strings.Contains(err.Error(), "chunk 0") && !strings.Contains(err.Error(), "chunk 1") {
		t.Errorf("error should mention failing chunk index: %v", err)
	}
	if !strings.Contains(err.Error(), "chunk-1-boom") {
		t.Errorf("error should chain underlying error: %v", err)
	}
}

func TestExtractorComponent_NewExtractorComponent_ParamCheck(t *testing.T) {
	c, err := NewExtractorComponent(map[string]any{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil component")
	}
}

func TestExtractorComponent_NewExtractorComponent_Happy(t *testing.T) {
	c, err := NewExtractorComponent(map[string]any{
		"llm_id": "openai/gpt-4o-mini",
		"summary": map[string]any{
			"enabled": true,
		},
	})
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}
	ext := c.(*ExtractorComponent)
	if !ext.Param.Summary.Enabled || ext.Param.LLMID != "openai/gpt-4o-mini" {
		t.Errorf("unexpected params: %+v", ext.Param)
	}
}

// TestExtractorComponent_InputsOutputs_NonEmpty verifies Inputs and Outputs shapes.
func TestExtractorComponent_InputsOutputs_NonEmpty(t *testing.T) {
	c := &ExtractorComponent{}
	ins := c.Inputs()
	outs := c.Outputs()
	if len(ins) == 0 {
		t.Error("Inputs() returned empty map")
	}
	if _, ok := ins["chunks"]; !ok {
		t.Errorf("Inputs() missing %q", "chunks")
	}
	if _, ok := ins["llm_id"]; !ok {
		t.Errorf("Inputs() missing %q", "llm_id")
	}
	if _, ok := ins["prompt"]; ok {
		t.Errorf("Inputs() should not contain deprecated %q", "prompt")
	}
	if _, ok := ins["system_prompt"]; ok {
		t.Errorf("Inputs() should not contain deprecated %q", "system_prompt")
	}
	if len(outs) == 0 {
		t.Error("Outputs() returned empty map")
	}
	if _, ok := outs["chunks"]; !ok {
		t.Errorf("Outputs() missing %q", "chunks")
	}
	if _, ok := outs["output_format"]; !ok {
		t.Errorf("Outputs() missing %q", "output_format")
	}
}

// TestSplitExtractorLLID covers the composite-id parser in isolation.
func TestSplitExtractorLLID(t *testing.T) {
	cases := []struct {
		in           string
		wantModel    string
		wantProvider string
		wantOK       bool
	}{
		{"gpt-4o-mini@openai", "gpt-4o-mini", "openai", true},
		{"bare-model", "bare-model", "", false},
		{"trailing@", "trailing", "", true},
		{"@leading", "", "leading", true},
		{"", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			model, provider, ok := splitExtractorLLIDPair(tc.in)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if model != tc.wantModel {
				t.Errorf("model = %q, want %q", model, tc.wantModel)
			}
			if provider != tc.wantProvider {
				t.Errorf("provider = %q, want %q", provider, tc.wantProvider)
			}
		})
	}
}

// TestTryParseJSONObject covers the JSON parser.
func TestTryParseJSONObject(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantOK  bool
		wantKey string
	}{
		{name: "object", in: `{"a":1}`, wantOK: true, wantKey: "a"},
		{name: "object with fence", in: "```json\n{\"a\":1}\n```", wantOK: true, wantKey: "a"},
		{name: "fence without json tag", in: "```\n{\"a\":1}\n```", wantOK: true, wantKey: "a"},
		{name: "json tag on own line", in: "```\njson\n{\"a\":1}\n```", wantOK: true, wantKey: "a"},
		{name: "JSON tag on own line", in: "```\nJSON\n{\"a\":1}\n```", wantOK: true, wantKey: "a"},
		{name: "leading prose no fence", in: "Here is the result: {\"a\":1}", wantOK: false},
		{name: "plain string", in: "hello", wantOK: false},
		{name: "array", in: `[1,2]`, wantOK: false},
		{name: "empty object", in: `{}`, wantOK: false},
		{name: "empty", in: ``, wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, ok := tryParseJSONObject(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (got %v)", ok, tc.wantOK, parsed)
			}
			if ok && tc.wantKey != "" {
				if _, has := parsed[tc.wantKey]; !has {
					t.Errorf("parsed map missing %q: %v", tc.wantKey, parsed)
				}
			}
		})
	}
}

// TestCleanExtractionResult covers think tag and error marker stripping.
func TestCleanExtractionResult(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: `{"a":1}`, want: `{"a":1}`},
		{name: "thinks stripped", in: "let me think<think>reasoning</think>\n{\"a\":1}", want: `{"a":1}`},
		{name: "thinks no json", in: "thinking</think>no json here", want: "no json here"},
		{name: "error marker rejected", in: "**ERROR** could not extract", want: ""},
		{name: "error after think", in: "x</think>**ERROR** boom", want: ""},
		{name: "whitespace trimmed", in: "  {\"a\":1}  ", want: `{"a":1}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanExtractionResult(tc.in); got != tc.want {
				t.Errorf("cleanExtractionResult(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// newMetadataExtractor returns an ExtractorComponent wired for doc-level
// metadata extraction with the given field definitions.
func newMetadataExtractor(fields ...common.MetadataFieldDef) *ExtractorComponent {
	return &ExtractorComponent{Param: schema.ExtractorParam{
		Metadata: schema.MetadataExtractConfig{
			Enabled:  true,
			Metadata: fields,
		},
	}}
}

// TestExtractorComponent_runEnableMetadata_MergesIntoChunkMetadata verifies a
// JSON object from the LLM is parsed and merged into the chunk's metadata map.
func TestExtractorComponent_runEnableMetadata_MergesIntoChunkMetadata(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: `{"category":"finance","region":"east"}`})
	c := newMetadataExtractor(
		common.MetadataFieldDef{Key: "category", Type: "string"},
		common.MetadataFieldDef{Key: "region", Type: "string"},
	)
	ck := map[string]any{}
	if err := c.runEnableMetadata(t.Context(), nil, extractorInputs{llmID: "m"}, ck, "chunk text"); err != nil {
		t.Fatalf("runEnableMetadata: %v", err)
	}
	meta, ok := ck["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("ck[metadata] missing or wrong type: %T", ck["metadata"])
	}
	if meta["category"] != "finance" || meta["region"] != "east" {
		t.Errorf("metadata = %v, want category=finance region=east", meta)
	}
}

// TestExtractorComponent_runEnableMetadata_StripsJSONFence verifies the
// extraction path tolerates a fenced ```json response.
func TestExtractorComponent_runEnableMetadata_StripsJSONFence(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: "```json\n{\"category\":\"law\"}\n```"})
	c := newMetadataExtractor(common.MetadataFieldDef{Key: "category", Type: "string"})
	ck := map[string]any{}
	if err := c.runEnableMetadata(t.Context(), nil, extractorInputs{llmID: "m"}, ck, "chunk text"); err != nil {
		t.Fatalf("runEnableMetadata: %v", err)
	}
	meta, ok := ck["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("ck[metadata] missing: %T", ck["metadata"])
	}
	if meta["category"] != "law" {
		t.Errorf("metadata = %v, want category=law", meta)
	}
}

// TestExtractorComponent_runEnableMetadata_MidTextThink verifies the full
// metadata path tolerates a mid-text reasoning block preceded by a preamble.
func TestExtractorComponent_runEnableMetadata_MidTextThink(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: `preamble<think>reasoning</think>{"category":"finance"}`})
	c := newMetadataExtractor(common.MetadataFieldDef{Key: "category", Type: "string"})
	ck := map[string]any{}
	if err := c.runEnableMetadata(t.Context(), nil, extractorInputs{llmID: "m"}, ck, "chunk text"); err != nil {
		t.Fatalf("runEnableMetadata: %v", err)
	}
	meta, ok := ck["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("ck[metadata] missing: %T", ck["metadata"])
	}
	if meta["category"] != "finance" {
		t.Errorf("metadata = %v, want category=finance", meta)
	}
}

// TestExtractorComponent_runEnableMetadata_DegradesGracefully verifies that an
// empty / **ERROR** / unparseable / think-only LLM response does NOT block ingestion.
func TestExtractorComponent_runEnableMetadata_DegradesGracefully(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"empty", ""},
		{"error_marker", "**ERROR** something went wrong"},
		{"garbage", "I could not find any metadata in this text."},
		{"not_json", "{\"category\": } partial"},
		{"think_only", "<think>let me think</think>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withStubChatInvoker(t, stubResponse{Content: tc.content})
			c := newMetadataExtractor(common.MetadataFieldDef{Key: "category", Type: "string"})
			ck := map[string]any{"metadata": map[string]any{"preexisting": "keep"}}
			if err := c.runEnableMetadata(t.Context(), nil, extractorInputs{llmID: "m"}, ck, tc.name); err != nil {
				t.Fatalf("runEnableMetadata returned error: %v", err)
			}
			meta, ok := ck["metadata"].(map[string]any)
			if !ok {
				t.Fatalf("ck[metadata] should remain a map, got %T", ck["metadata"])
			}
			if meta["preexisting"] != "keep" {
				t.Errorf("preexisting metadata must be preserved: %v", meta)
			}
			if _, has := meta["category"]; has {
				t.Errorf("category should not be set on degraded response: %v", meta)
			}
		})
	}
}

// TestExtractorComponent_runEnableMetadata_CrossChunkUnion simulates two chunks
// whose extraction returns overlapping list values for the same key.
func TestExtractorComponent_runEnableMetadata_CrossChunkUnion(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: `{"people":["关羽","张辽"]}`},
		stubResponse{Content: `{"people":["张辽","刘备"]}`},
	)
	c := newMetadataExtractor(common.MetadataFieldDef{Key: "people", Type: "string"})
	ck1 := map[string]any{}
	ck2 := map[string]any{}
	if err := c.runEnableMetadata(t.Context(), nil, extractorInputs{llmID: "m"}, ck1, "chunk one"); err != nil {
		t.Fatalf("ck1: %v", err)
	}
	if err := c.runEnableMetadata(t.Context(), nil, extractorInputs{llmID: "m"}, ck2, "chunk two"); err != nil {
		t.Fatalf("ck2: %v", err)
	}
	m1, ok := ck1["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("ck1[metadata] missing: %T", ck1["metadata"])
	}
	m2, ok := ck2["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("ck2[metadata] missing: %T", ck2["metadata"])
	}
	docMeta := map[string]any{}
	docMeta = utility.UpdateMetadataTo(docMeta, m1)
	docMeta = utility.UpdateMetadataTo(docMeta, m2)
	people, ok := docMeta["people"].([]string)
	if !ok {
		t.Fatalf("people = %T, want []string", docMeta["people"])
	}
	want := map[string]bool{"关羽": true, "张辽": true, "刘备": true}
	if len(people) != len(want) {
		t.Fatalf("people = %v, want union of %v", people, want)
	}
	for _, p := range people {
		if !want[p] {
			t.Errorf("unexpected person %q", p)
		}
	}
}

// TestExtractorComponent_runEnableMetadata_CombinedValueSplit verifies a value
// the LLM combines with Chinese/comma delimiters is split when passed through
// common.SplitCombinedMetadataValues.
func TestExtractorComponent_runEnableMetadata_CombinedValueSplit(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: `{"people":["关羽、张辽、刘备"]}`})
	c := newMetadataExtractor(common.MetadataFieldDef{Key: "people", Type: "string"})
	ck := map[string]any{}
	if err := c.runEnableMetadata(t.Context(), nil, extractorInputs{llmID: "m"}, ck, "chunk text"); err != nil {
		t.Fatalf("runEnableMetadata: %v", err)
	}
	rawMeta, ok := ck["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("ck[metadata] missing: %T", ck["metadata"])
	}
	raw, ok := rawMeta["people"].([]any)
	if !ok || len(raw) != 1 {
		t.Fatalf("raw people = %v, want 1 combined element", rawMeta["people"])
	}
	split := common.SplitCombinedMetadataValues(ck["metadata"].(map[string]any))
	people, ok := split["people"].([]string)
	if !ok {
		t.Fatalf("people = %T, want []string", split["people"])
	}
	want := map[string]bool{"关羽": true, "张辽": true, "刘备": true}
	if len(people) != len(want) {
		t.Fatalf("people = %v, want 3 split elements", people)
	}
	for _, p := range people {
		if !want[p] {
			t.Errorf("unexpected %q", p)
		}
	}
}

// TestExtractorComponent_ConcurrentInvoke verifies the chat
// invoker swap is safe under concurrent Invoke calls.
func TestExtractorComponent_ConcurrentInvoke(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: "1"},
		stubResponse{Content: "2"},
		stubResponse{Content: "3"},
		stubResponse{Content: "4"},
	)
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		Summary: schema.SummaryExtractConfig{Enabled: true},
	}}
	chunks := []map[string]any{
		{"text": "a"}, {"text": "b"}, {"text": "c"}, {"text": "d"},
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(chunks))
	for _, ck := range chunks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Invoke(t.Context(), nil, map[string]any{
				"chunks": []map[string]any{ck},
			})
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("Invoke error under concurrency: %v", err)
	}
}

var _ = eschema.Message{}

// TestIsBareTenantModelID verifies UUID detection.
func TestIsBareTenantModelID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"9e819c2442b14f9dab46062916e29195", true},
		{"ABCDEFabcdef01234567890123456789", true},
		{"9e819c2442b14f9dab46062916e2919", false},
		{"9e819c2442b14f9dab46062916e29195X", false},
		{"gpt-4o-mini@openai", false},
		{"", false},
		{"not-a-uuid", false},
	}
	for _, tc := range tests {
		got := isBareTenantModelID(tc.input)
		if got != tc.want {
			t.Errorf("isBareTenantModelID(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// TestResolveExtractorChatTarget_AtSplitFallback verifies the @ split fallback.
func TestResolveExtractorChatTarget_AtSplitFallback(t *testing.T) {
	ctx := t.Context()
	driver, modelName, apiKey, baseURL, err := resolveExtractorChatTarget(
		ctx, dao.DB, "gpt-4o-mini@openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if driver != "openai" {
		t.Errorf("driver = %q, want openai", driver)
	}
	if modelName != "gpt-4o-mini" {
		t.Errorf("modelName = %q, want gpt-4o-mini", modelName)
	}
	if apiKey != "" || baseURL != "" {
		t.Errorf("apiKey/baseURL should be empty in fallback path")
	}
}

// TestResolveExtractorChatTarget_NoDriver verifies a non-@ plain string returns no driver.
func TestResolveExtractorChatTarget_NoDriver(t *testing.T) {
	ctx := t.Context()
	driver, modelName, _, _, err := resolveExtractorChatTarget(
		ctx, dao.DB, "plain-name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if driver != "" {
		t.Errorf("driver should be empty for plain name, got %q", driver)
	}
	if modelName != "plain-name" {
		t.Errorf("modelName = %q, want plain-name", modelName)
	}
}

// TestExtractorComponent_Invoke_TemperatureSet verifies keyword extraction receives Temperature=0.2.
func TestExtractorComponent_Invoke_TemperatureSet(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "keyword, extraction"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID:    "gpt-4o-mini",
		Keywords: schema.KeywordExtractConfig{TopN: 3},
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "document content"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.lastRequest().Temperature == nil {
		t.Fatal("Temperature is nil, want 0.2")
	}
	if *stub.lastRequest().Temperature != 0.2 {
		t.Errorf("Temperature = %v, want 0.2", *stub.lastRequest().Temperature)
	}
	if stub.calls.Load() != 1 {
		t.Errorf("expected exactly 1 LLM call (keyword), got %d", stub.calls.Load())
	}
}

// TestIsRetryableLLMError tests the retry classification heuristic.
func TestIsRetryableLLMError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil is retryable", err: nil, want: true},
		{name: "context canceled is terminal", err: context.Canceled, want: false},
		{name: "deadline exceeded is terminal", err: context.DeadlineExceeded, want: false},
		{
			name: "wrapped deadline with 400ms must stay retryable",
			err:  errors.New("context deadline exceeded after 400ms"),
			want: true,
		},
		{name: "429 stays retryable", err: errors.New("429 Too Many Requests"), want: true},
		{name: "503 stays retryable", err: errors.New("503 Service Unavailable"), want: true},
		{name: "401 unauthorized is terminal", err: errors.New("HTTP 401 Unauthorized"), want: false},
		{name: "403 forbidden is terminal", err: errors.New("403 forbidden"), want: false},
		{name: "404 not found is terminal", err: errors.New("HTTP 404 Not Found"), want: false},
		{name: "405 method not allowed is terminal", err: errors.New("405 Method Not Allowed"), want: false},
		{name: "422 unprocessable is terminal", err: errors.New("422 Unprocessable Entity"), want: false},
		{name: "bad request is terminal", err: errors.New("400 Bad Request: malformed"), want: false},
		{name: "api key phrase is terminal", err: errors.New("invalid api key"), want: false},
		{name: "no driver phrase is terminal", err: errors.New("no driver resolved for llm_id"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableLLMError(tt.err); got != tt.want {
				t.Errorf("isRetryableLLMError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestCleanExtractionResult_LastThinkTag verifies think tag removal.
func TestCleanExtractionResult_LastThinkTag(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "single think block", in: "<think>reasoning</think>the answer", want: "the answer"},
		{name: "nested think blocks", in: "<think>outer</think>mid<think>inner</think>final output", want: "final output"},
		{name: "no think tag", in: "plain answer", want: "plain answer"},
		{name: "think tag without close", in: "<think>unclosed", want: "<think>unclosed"},
		{name: "error sentinel", in: "valid output**ERROR**extra", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanExtractionResult(tt.in)
			if got != tt.want {
				t.Errorf("cleanExtractionResult(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestCleanLLMText verifies cleanLLMText reasoning tag and tool call removal.
func TestCleanLLMText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "think block stripped",
			in:   "<think>reasoning</think>the answer",
			want: "the answer",
		},
		{
			name: "close without open kept",
			in:   "abc</think>def",
			want: "abc</think>def",
		},
		{
			name: "prefix before think kept",
			in:   "prefix<think>reason</think>answer",
			want: "prefix<think>reason</think>answer",
		},
		{
			name: "open without close kept",
			in:   "<think>unclosed",
			want: "<think>unclosed",
		},
		{
			name: "tool_call block removed",
			in:   "before<tool_call>{\"name\":\"x\"}</tool_call>after",
			want: "beforeafter",
		},
		{
			name: "consecutive tool_call blocks",
			in:   "a<tool_call>1</tool_call>b<tool_call>2</tool_call>c",
			want: "abc",
		},
		{
			name: "think then tool_call",
			in:   "<think>r</think>out<tool_call>t</tool_call>end",
			want: "outend",
		},
		{
			name: "plain text",
			in:   "  plain answer  ",
			want: "plain answer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanLLMText(tt.in); got != tt.want {
				t.Errorf("cleanLLMText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestExtractorComponent_callStructured verifies structured parsing.
func TestExtractorComponent_callStructured(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: `{"a": 1}`})
	c := &ExtractorComponent{}
	got, err := c.callStructured(t.Context(), nil, extractorInputs{llmID: "m"}, "system", "")
	if err != nil {
		t.Fatalf("callStructured: %v", err)
	}
	if got["a"].(float64) != 1 {
		t.Errorf("parsed = %v, want map with a=1", got)
	}

	// Non-JSON response → (nil, nil), not an error.
	withStubChatInvoker(t, stubResponse{Content: "this is not JSON"})
	got, err = c.callStructured(t.Context(), nil, extractorInputs{llmID: "m"}, "system", "")
	if err != nil {
		t.Fatalf("callStructured on non-JSON: %v", err)
	}
	if got != nil {
		t.Errorf("non-JSON response should yield nil map, got %v", got)
	}
}

// TestExtractorComponent_callStructured_MidTextThink verifies think trailing stripping.
func TestExtractorComponent_callStructured_MidTextThink(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: `preamble<think>reasoning</think>{"a": 1}`})
	c := &ExtractorComponent{}
	got, err := c.callStructured(t.Context(), nil, extractorInputs{llmID: "m"}, "system", "")
	if err != nil {
		t.Fatalf("callStructured: %v", err)
	}
	if got == nil || got["a"].(float64) != 1 {
		t.Errorf("parsed = %v, want map with a=1", got)
	}
}

// TestExtractorComponent_Invoke_ConcurrentKeywordsAndQuestions verifies
// keyword and question extraction on chunks.
func TestExtractorComponent_Invoke_ConcurrentKeywordsAndQuestions(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "alpha, beta"},       // chunk 0 keywords
		stubResponse{Content: "what is it?\nwhy?"}, // chunk 0 questions
		stubResponse{Content: "gamma, delta"},      // chunk 1 keywords
		stubResponse{Content: "how?\nwhen?"},       // chunk 1 questions
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID:     "gpt-4o-mini",
		Keywords:  schema.KeywordExtractConfig{TopN: 2},
		Questions: schema.QuestionExtractConfig{TopN: 2},
	}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{
			{"text": "first doc"},
			{"text": "second doc"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	chunks, ok := out["chunks"].([]map[string]any)
	if !ok || len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %v", out["chunks"])
	}

	for i, ck := range chunks {
		kwds, hasKW := ck["important_kwd"].([]string)
		if !hasKW || len(kwds) == 0 {
			t.Errorf("chunk %d: missing important_kwd", i)
		}
		qs, hasQ := ck["question_kwd"].([]string)
		if !hasQ || len(qs) == 0 {
			t.Errorf("chunk %d: missing question_kwd", i)
		}
	}

	if calls := stub.Calls(); calls != 4 {
		t.Errorf("expected 4 LLM calls (2 chunks × 2 types), got %d", calls)
	}
}

// TestResolveExtractorChatTarget_EmptyLLMID verifies default fallback when llmID is empty.
func TestResolveExtractorChatTarget_EmptyLLMID(t *testing.T) {
	ctx := t.Context()
	driver, modelName, _, _, err := resolveExtractorChatTarget(ctx, dao.DB, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if driver != "" {
		t.Logf("resolved empty llmID: driver=%q model=%q", driver, modelName)
	}
}

// TestFitExtractorMessages_RejectsEmptyUserTurn verifies rejection of emptied user turn.
func TestFitExtractorMessages_RejectsEmptyUserTurn(t *testing.T) {
	SetExtractorContextLengthOverride(func(_ context.Context, _ string) int { return 500 })
	t.Cleanup(func() { SetExtractorContextLengthOverride(nil) })

	msgs := []eschema.Message{
		{Role: eschema.System, Content: strings.Repeat("s ", 1000)},
		{Role: eschema.User, Content: strings.Repeat("u ", 400)},
	}
	if _, err := fitExtractorMessages(t.Context(), nil, "test@test", msgs); err == nil {
		t.Fatal("expected an error when fitting empties the user turn")
	}
}

// TestFitExtractorMessages_KeepsUserTurn verifies prompt trimming happy path.
func TestFitExtractorMessages_KeepsUserTurn(t *testing.T) {
	SetExtractorContextLengthOverride(func(_ context.Context, _ string) int { return 2000 })
	t.Cleanup(func() { SetExtractorContextLengthOverride(nil) })

	msgs := []eschema.Message{
		{Role: eschema.System, Content: "you are a helpful assistant"},
		{Role: eschema.User, Content: strings.Repeat("u ", 3000)},
	}
	fitted, err := fitExtractorMessages(t.Context(), nil, "test@test", msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fitted) != 2 {
		t.Fatalf("got %d messages, want 2", len(fitted))
	}
	if strings.TrimSpace(fitted[1].Content) == "" {
		t.Fatal("user turn was emptied")
	}
}

// TestFitExtractorMessages_NoSystemPromptKeepsUserTurn verifies user-only message fitting.
func TestFitExtractorMessages_NoSystemPromptKeepsUserTurn(t *testing.T) {
	SetExtractorContextLengthOverride(func(_ context.Context, _ string) int { return 2000 })
	t.Cleanup(func() { SetExtractorContextLengthOverride(nil) })

	msgs := []eschema.Message{
		{Role: eschema.User, Content: strings.Repeat("u ", 3000)},
	}
	fitted, err := fitExtractorMessages(t.Context(), nil, "test@test", msgs)
	if err != nil {
		t.Fatalf("unexpected error for user-only prompt: %v", err)
	}
	if len(fitted) != 1 || fitted[0].Role != eschema.User {
		t.Fatalf("got %d messages, want the single user turn: %+v", len(fitted), fitted)
	}
	if strings.TrimSpace(fitted[0].Content) == "" {
		t.Fatal("user turn was emptied")
	}
}

// TestExtractorComponent_CallRaw_FitsBeforeInvoke verifies message fitting end to end.
func TestExtractorComponent_CallRaw_FitsBeforeInvoke(t *testing.T) {
	SetExtractorContextLengthOverride(func(_ context.Context, _ string) int { return 200 })
	t.Cleanup(func() { SetExtractorContextLengthOverride(nil) })

	stub := withStubChatInvoker(t, stubResponse{Content: `{"ok": true}`})
	c := &ExtractorComponent{}

	chunkBody := strings.Repeat("chunk text with lots of tokens. ", 500)
	_, err := c.callText(t.Context(), nil, extractorInputs{
		llmID: "test@test",
	}, "extract fields", chunkBody)
	if err != nil {
		t.Fatalf("callText: %v", err)
	}

	stub.mu.Lock()
	req := stub.lastRequest()
	stub.mu.Unlock()
	if len(req.Messages) == 0 {
		t.Fatal("invoker was not called")
	}
	if req.Messages[0].Role != eschema.System || strings.TrimSpace(req.Messages[0].Content) == "" {
		t.Fatalf("system prompt lost or emptied before invoke: %+v", req.Messages[0])
	}
	total := 0
	for _, m := range req.Messages {
		total += tokenizer.NumTokensFromString(m.Content)
	}
	if total > extractorContextFitBudget(200) {
		t.Fatalf("sent messages total %d exceed the fitting budget %d", total, extractorContextFitBudget(200))
	}
	if !strings.Contains(req.Messages[len(req.Messages)-1].Content, "chunk text") {
		t.Fatal("chunk text lost from the user turn")
	}
}

// TestExtractorComponent_CallRaw_CustomContextOverride verifies tenant-configured context override.
func TestExtractorComponent_CallRaw_CustomContextOverride(t *testing.T) {
	db := openExtractorContextTestDB(t)
	seedExtractorContextModel(t, db, "")
	if err := db.Create(&entity.TenantModelInstance{
		ID:           "instance-1",
		ProviderID:   "provider-openai",
		InstanceName: "default",
		Status:       "active",
	}).Error; err != nil {
		t.Fatalf("create instance: %v", err)
	}
	if err := db.Model(&entity.TenantModel{}).
		Where("id = ?", "0123456789abcdef0123456789abcdef").
		Update("extra", `{"max_tokens": 2000}`).Error; err != nil {
		t.Fatalf("set model extra: %v", err)
	}
	ctx := extractorStateCtx(t, "tenant-1")

	stub := withStubChatInvoker(t, stubResponse{Content: `{"ok": true}`})
	c := &ExtractorComponent{}
	_, err := c.callText(ctx, db, extractorInputs{
		llmID: "gpt-4o@OpenAI",
	}, "extract fields", strings.Repeat("chunk text with lots of tokens. ", 500))
	if err != nil {
		t.Fatalf("callText: %v", err)
	}

	stub.mu.Lock()
	req := stub.lastRequest()
	stub.mu.Unlock()
	if len(req.Messages) == 0 {
		t.Fatal("invoker was not called")
	}
	if req.Messages[0].Role != eschema.System || strings.TrimSpace(req.Messages[0].Content) == "" {
		t.Fatalf("system prompt lost or emptied: %+v", req.Messages[0])
	}
	total := 0
	for _, m := range req.Messages {
		total += tokenizer.NumTokensFromString(m.Content)
	}
	if total > 2000 {
		t.Fatalf("sent messages total %d exceed the custom 2000-token context window", total)
	}
}

func openExtractorContextTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Tenant{}, &entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedExtractorContextModel(t *testing.T, db *gorm.DB, tenantLLMID string) {
	t.Helper()
	status := "1"
	tenant := entity.Tenant{
		ID:     "tenant-1",
		LLMID:  "gpt-4o@openai",
		Status: &status,
	}
	if tenantLLMID != "" {
		tenant.TenantLLMID = &tenantLLMID
	}
	if err := db.Create(&tenant).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := db.Create(&entity.TenantModelProvider{
		ID:           "provider-openai",
		ProviderName: "OpenAI",
		TenantID:     "tenant-1",
	}).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if err := db.Create(&entity.TenantModel{
		ID:         "0123456789abcdef0123456789abcdef",
		ProviderID: "provider-openai",
		InstanceID: "instance-1",
		ModelName:  "gpt-4o",
		ModelType:  int(entity.ModelTypeChat),
		Status:     "active",
	}).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}
}

func extractorStateCtx(t *testing.T, tenantID string) context.Context {
	t.Helper()
	state := runtime.NewCanvasState("run-1", "session-1")
	state.SetGlobal("tenant_id", tenantID)
	return runtime.WithState(t.Context(), state)
}

func TestExtractorContextLength_TenantModelUUID(t *testing.T) {
	db := openExtractorContextTestDB(t)
	seedExtractorContextModel(t, db, "")
	ctx := extractorStateCtx(t, "tenant-1")

	if got := extractorContextLength(ctx, db, "0123456789abcdef0123456789abcdef"); got != 128000 {
		t.Fatalf("extractorContextLength(uuid) = %d, want 128000", got)
	}
}

func TestExtractorContextLength_DefaultChatModelPinned(t *testing.T) {
	db := openExtractorContextTestDB(t)
	seedExtractorContextModel(t, db, "0123456789abcdef0123456789abcdef")
	ctx := extractorStateCtx(t, "tenant-1")

	if got := extractorContextLength(ctx, db, ""); got != 128000 {
		t.Fatalf("extractorContextLength(default pinned uuid) = %d, want 128000", got)
	}
}

func TestExtractorContextLength_DefaultChatModelComposite(t *testing.T) {
	db := openExtractorContextTestDB(t)
	seedExtractorContextModel(t, db, "")
	ctx := extractorStateCtx(t, "tenant-1")

	if got := extractorContextLength(ctx, db, ""); got != 128000 {
		t.Fatalf("extractorContextLength(default composite) = %d, want 128000", got)
	}
}

func TestExtractorContextLength_UnknownModelSkips(t *testing.T) {
	db := openExtractorContextTestDB(t)
	seedExtractorContextModel(t, db, "")
	ctx := extractorStateCtx(t, "tenant-1")

	if got := extractorContextLength(ctx, db, "no-such-model@no-such-provider"); got != 0 {
		t.Fatalf("extractorContextLength(unknown) = %d, want 0", got)
	}
}

func TestExtractorContextFitBudget(t *testing.T) {
	if got := extractorContextFitBudget(128000); got != 124160 {
		t.Fatalf("extractorContextFitBudget(128000) = %d, want 124160", got)
	}
	if got := extractorContextFitBudget(1); got != 1 {
		t.Fatalf("extractorContextFitBudget(1) = %d, want 1", got)
	}
}

func TestFitExtractorMessages_RejectsSystemPromptLoss(t *testing.T) {
	SetExtractorContextLengthOverride(func(_ context.Context, _ string) int { return 300 })
	t.Cleanup(func() { SetExtractorContextLengthOverride(nil) })

	msgs := []eschema.Message{
		{Role: eschema.System, Content: strings.Repeat("s ", 5000)},
		{Role: eschema.User, Content: strings.Repeat("u ", 400)},
	}
	if _, err := fitExtractorMessages(t.Context(), nil, "test@test", msgs); err == nil {
		t.Fatal("expected an error when fitting empties the system prompt")
	}
}

func TestExtractorContextLength_NilDBGraceful(t *testing.T) {
	ctx := extractorStateCtx(t, "tenant-1")
	if got := extractorContextLength(ctx, nil, ""); got != 0 {
		t.Fatalf("extractorContextLength(nil db, default model) = %d, want 0", got)
	}
}

func TestBuildExtractorMessages(t *testing.T) {
	msgs := buildExtractorMessages("System rule", "Chunk content")
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != eschema.System || msgs[0].Content != "System rule" {
		t.Errorf("msgs[0] = %#v, want system message", msgs[0])
	}
	if msgs[1].Role != eschema.User || msgs[1].Content != "Chunk content" {
		t.Errorf("msgs[1] = %#v, want user message", msgs[1])
	}

	// Empty system prompt omitted
	msgsNoSys := buildExtractorMessages("", "Chunk content")
	if len(msgsNoSys) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgsNoSys))
	}
	if msgsNoSys[0].Role != eschema.User || msgsNoSys[0].Content != "Chunk content" {
		t.Errorf("msgsNoSys[0] = %#v, want user message", msgsNoSys[0])
	}

	// Empty user chunk text normalized to single space
	msgsEmptyUser := buildExtractorMessages("System rule", "")
	if len(msgsEmptyUser) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgsEmptyUser))
	}
	if msgsEmptyUser[1].Role != eschema.User || msgsEmptyUser[1].Content != " " {
		t.Errorf("msgsEmptyUser[1] = %#v, want single space", msgsEmptyUser[1])
	}
}

func TestExtractorModularParams(t *testing.T) {
	params := map[string]any{
		"llm_id": "test-llm-1",
		"keywords": map[string]any{
			"top_n": 5,
		},
		"questions": map[string]any{
			"top_n": 3,
		},
		"tags": map[string]any{
			"top_n":       2,
			"tag_file_id": "tag_file_abc",
		},
		"summary": map[string]any{
			"enabled": true,
		},
		"metadata": map[string]any{
			"enabled": true,
			"metadata": []any{
				map[string]any{"key": "category", "type": "string"},
			},
		},
	}

	comp, err := NewExtractorComponent(params)
	if err != nil {
		t.Fatalf("NewExtractorComponent failed: %v", err)
	}

	ext, ok := comp.(*ExtractorComponent)
	if !ok {
		t.Fatalf("expected *ExtractorComponent, got %T", comp)
	}

	if ext.Param.Keywords.TopN != 5 {
		t.Errorf("Keywords config mismatch: %+v", ext.Param.Keywords)
	}
	if ext.Param.Questions.TopN != 3 {
		t.Errorf("Questions config mismatch: %+v", ext.Param.Questions)
	}
	if ext.Param.Tags.TopN != 2 || ext.Param.Tags.TagFileID != "tag_file_abc" {
		t.Errorf("Tags config mismatch: %+v", ext.Param.Tags)
	}
	if !ext.Param.Summary.Enabled {
		t.Errorf("Summary config mismatch: %+v", ext.Param.Summary)
	}
	if !ext.Param.Metadata.Enabled || len(ext.Param.Metadata.Metadata) != 1 {
		t.Errorf("Metadata config mismatch: %+v", ext.Param.Metadata)
	}
}

func TestExtractorModularPromptsExecution(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "kw1, kw2"},
		stubResponse{Content: "question 1?\nquestion 2?"},
		stubResponse{Content: "Summary of chunk"},
	)

	params := map[string]any{
		"llm_id": "llm-1",
		"keywords": map[string]any{
			"top_n": 2,
		},
		"questions": map[string]any{
			"top_n": 2,
		},
		"summary": map[string]any{
			"enabled": true,
		},
	}

	comp, err := NewExtractorComponent(params)
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}

	in := map[string]any{
		"chunks": []map[string]any{
			{"content_with_weight": "Hello world content"},
		},
	}

	out, err := comp.Invoke(t.Context(), nil, in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	chunks, ok := out["chunks"].([]map[string]any)
	if !ok || len(chunks) != 1 {
		t.Fatalf("expected 1 chunk output, got %v", out)
	}

	ck := chunks[0]
	if kwds, ok := ck["important_kwd"].([]string); !ok || len(kwds) != 2 {
		t.Errorf("expected important_kwd = [kw1, kw2], got %v", ck["important_kwd"])
	}
	if qs, ok := ck["question_kwd"].([]string); !ok || len(qs) != 2 {
		t.Errorf("expected question_kwd = [question 1?, question 2?], got %v", ck["question_kwd"])
	}
	if sum, ok := ck["summary"].(string); !ok || sum != "Summary of chunk" {
		t.Errorf("expected summary = 'Summary of chunk', got %v", ck["summary"])
	}

	reqs := stub.requests
	if len(reqs) != 3 {
		t.Fatalf("expected 3 LLM calls, got %d", len(reqs))
	}
}

func TestExtractorModularMetadataConfig(t *testing.T) {
	params := map[string]any{
		"llm_id": "llm-1",
		"metadata": map[string]any{
			"enabled": true,
			"metadata": []any{
				map[string]any{
					"key":         "author",
					"type":        "string",
					"description": "The author name",
					"enum":        []any{"Alice", "Bob"},
				},
				map[string]any{
					"key":  "year",
					"type": "integer",
				},
			},
			"built_in_metadata": []any{
				map[string]any{
					"key":  "file_name",
					"type": "string",
				},
			},
		},
	}

	comp, err := NewExtractorComponent(params)
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}

	ext, ok := comp.(*ExtractorComponent)
	if !ok {
		t.Fatalf("expected *ExtractorComponent, got %T", comp)
	}

	if !ext.Param.Metadata.Enabled {
		t.Errorf("Metadata enabled mismatch: %+v", ext.Param.Metadata.Enabled)
	}
	if len(ext.Param.Metadata.Metadata) != 2 {
		t.Fatalf("Metadata fields mismatch: %d", len(ext.Param.Metadata.Metadata))
	}
	if ext.Param.Metadata.Metadata[0].Key != "author" || ext.Param.Metadata.Metadata[0].Description != "The author name" || len(ext.Param.Metadata.Metadata[0].Enum) != 2 {
		t.Errorf("Metadata field 0 mismatch: %+v", ext.Param.Metadata.Metadata[0])
	}
	if ext.Param.Metadata.Metadata[1].Key != "year" {
		t.Errorf("Metadata field 1 mismatch: %+v", ext.Param.Metadata.Metadata[1])
	}
	if len(ext.Param.Metadata.BuiltInMetadata) != 1 || ext.Param.Metadata.BuiltInMetadata[0].Key != "file_name" {
		t.Errorf("BuiltInMetadata mismatch: %+v", ext.Param.Metadata.BuiltInMetadata)
	}
}

func TestExtractorModularMetadataExecution(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: `{"author": "Alice", "year": 2026}`},
	)

	params := map[string]any{
		"llm_id": "llm-1",
		"metadata": map[string]any{
			"enabled": true,
			"metadata": []any{
				map[string]any{
					"key":  "author",
					"type": "string",
				},
				map[string]any{
					"key":  "year",
					"type": "integer",
				},
			},
		},
	}

	comp, err := NewExtractorComponent(params)
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}

	in := map[string]any{
		"chunks": []map[string]any{
			{"content_with_weight": "Written by Alice in 2026."},
		},
	}

	out, err := comp.Invoke(t.Context(), nil, in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	chunks, ok := out["chunks"].([]map[string]any)
	if !ok || len(chunks) != 1 {
		t.Fatalf("expected 1 chunk output, got %v", out)
	}

	ck := chunks[0]
	meta, ok := ck["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected chunk metadata map, got %T: %v", ck["metadata"], ck["metadata"])
	}
	if meta["author"] != "Alice" {
		t.Errorf("expected metadata.author = Alice, got %v", meta["author"])
	}
	if fmt.Sprintf("%v", meta["year"]) != "2026" {
		t.Errorf("expected metadata.year = 2026, got %v", meta["year"])
	}
}

func TestExtractorDefaultSummaryPromptInjection(t *testing.T) {
	stub := withStubChatInvoker(t, stubResponse{Content: "A concise summary."})

	params := map[string]any{
		"llm_id": "llm-1",
		"summary": map[string]any{
			"enabled": true,
		},
	}

	comp, err := NewExtractorComponent(params)
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}

	in := map[string]any{
		"chunks": []map[string]any{
			{"content_with_weight": "This is a detailed paragraph about artificial intelligence."},
		},
	}

	out, err := comp.Invoke(t.Context(), nil, in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	chunks, ok := out["chunks"].([]map[string]any)
	if !ok || len(chunks) != 1 {
		t.Fatalf("expected 1 chunk output, got %v", out)
	}

	if chunks[0]["summary"] != "A concise summary." {
		t.Errorf("expected summary = 'A concise summary.', got %v", chunks[0]["summary"])
	}

	if stub.Calls() != 1 {
		t.Fatalf("expected 1 LLM call, got %d", stub.Calls())
	}

	lastReq := stub.lastRequest()
	msgs := lastReq.Messages
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d: %+v", len(msgs), msgs)
	}

	if msgs[0].Role != "system" || !strings.Contains(msgs[0].Content, "You are a precise and faithful text summarizer") {
		t.Errorf("expected autoSummaryPrompt in system message, got: %+v", msgs[0])
	}

	if msgs[1].Role != "user" || !strings.Contains(msgs[1].Content, "This is a detailed paragraph about artificial intelligence.") {
		t.Errorf("expected chunk text in user message, got: %+v", msgs[1])
	}
}

func TestExtractorCustomSummarySystemPrompt(t *testing.T) {
	stub := withStubChatInvoker(t, stubResponse{Content: "Custom summary."})

	params := map[string]any{
		"llm_id": "llm-1",
		"summary": map[string]any{
			"enabled":       true,
			"system_prompt": "Custom system prompt for summarization.",
		},
	}

	comp, err := NewExtractorComponent(params)
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}

	in := map[string]any{
		"chunks": []map[string]any{
			{"content_with_weight": "Text to summarize."},
		},
	}

	out, err := comp.Invoke(t.Context(), nil, in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	chunks, ok := out["chunks"].([]map[string]any)
	if !ok || len(chunks) != 1 {
		t.Fatalf("expected 1 chunk output, got %v", out)
	}

	if chunks[0]["summary"] != "Custom summary." {
		t.Errorf("expected summary = 'Custom summary.', got %v", chunks[0]["summary"])
	}

	if stub.Calls() != 1 {
		t.Fatalf("expected 1 LLM call, got %d", stub.Calls())
	}

	lastReq := stub.lastRequest()
	msgs := lastReq.Messages
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d: %+v", len(msgs), msgs)
	}

	if msgs[0].Role != "system" || msgs[0].Content != "Custom system prompt for summarization." {
		t.Errorf("expected custom system prompt in system message, got: %+v", msgs[0])
	}
}

func TestExtractorCustomKeywordsAndQuestionsSystemPrompt(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "custom, keywords"},
		stubResponse{Content: "Custom question 1?\nCustom question 2?"},
	)

	params := map[string]any{
		"llm_id": "llm-1",
		"keywords": map[string]any{
			"top_n":         2,
			"system_prompt": "Custom keywords system prompt.",
		},
		"questions": map[string]any{
			"top_n":         2,
			"system_prompt": "Custom questions system prompt.",
		},
	}

	comp, err := NewExtractorComponent(params)
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}

	in := map[string]any{
		"chunks": []map[string]any{
			{"content_with_weight": "Content text."},
		},
	}

	out, err := comp.Invoke(t.Context(), nil, in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	chunks, ok := out["chunks"].([]map[string]any)
	if !ok || len(chunks) != 1 {
		t.Fatalf("expected 1 chunk output, got %v", out)
	}

	reqs := stub.requests
	if len(reqs) != 2 {
		t.Fatalf("expected 2 LLM calls, got %d", len(reqs))
	}

	if reqs[0].Messages[0].Content != "Custom keywords system prompt." {
		t.Errorf("expected custom keywords system prompt, got: %q", reqs[0].Messages[0].Content)
	}
	if reqs[1].Messages[0].Content != "Custom questions system prompt." {
		t.Errorf("expected custom questions system prompt, got: %q", reqs[1].Messages[0].Content)
	}
}

// TestExtractorTopNPlaceholderSubstitution verifies the {{ topn }} placeholder
// in custom keyword/question system prompts is replaced with the configured
// top_n, so the count slider stays authoritative when the frontend pre-fills
// a prompt.
func TestExtractorTopNPlaceholderSubstitution(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "alpha, beta"},
		stubResponse{Content: "q1?\nq2?"},
	)

	params := map[string]any{
		"llm_id": "llm-1",
		"keywords": map[string]any{
			"top_n":         9,
			"system_prompt": "Give the top {{ topn }} keywords.",
		},
		"questions": map[string]any{
			"top_n":         7,
			"system_prompt": "Propose {{topn}} questions.",
		},
	}

	comp, err := NewExtractorComponent(params)
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}

	in := map[string]any{
		"chunks": []map[string]any{
			{"content_with_weight": "Content text."},
		},
	}

	if _, err := comp.Invoke(t.Context(), nil, in); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	reqs := stub.requests
	if len(reqs) != 2 {
		t.Fatalf("expected 2 LLM calls, got %d", len(reqs))
	}

	if got := reqs[0].Messages[0].Content; got != "Give the top 9 keywords." {
		t.Errorf("expected keywords prompt with top_n=9 substituted, got: %q", got)
	}
	if got := reqs[1].Messages[0].Content; got != "Propose 7 questions." {
		t.Errorf("expected questions prompt with top_n=7 substituted, got: %q", got)
	}
}

// TestExtractorDefaultPromptsRenderTopN verifies the built-in keyword/question
// prompts interpolate the configured top_n when no custom prompt is set.
func TestExtractorDefaultPromptsRenderTopN(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "k1, k2"},
		stubResponse{Content: "q1?"},
	)

	params := map[string]any{
		"llm_id":    "llm-1",
		"keywords":  map[string]any{"top_n": 4},
		"questions": map[string]any{"top_n": 6},
	}

	comp, err := NewExtractorComponent(params)
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}

	in := map[string]any{
		"chunks": []map[string]any{
			{"content_with_weight": "Content text."},
		},
	}

	if _, err := comp.Invoke(t.Context(), nil, in); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	reqs := stub.requests
	if len(reqs) != 2 {
		t.Fatalf("expected 2 LLM calls, got %d", len(reqs))
	}

	kwPrompt := reqs[0].Messages[0].Content
	if !strings.Contains(kwPrompt, "top 4 important keywords/phrases") {
		t.Errorf("expected built-in keywords prompt with top_n=4, got: %q", kwPrompt)
	}
	qPrompt := reqs[1].Messages[0].Content
	if !strings.Contains(qPrompt, "top 6 important questions") {
		t.Errorf("expected built-in questions prompt with top_n=6, got: %q", qPrompt)
	}
	if strings.Contains(kwPrompt, "{{") || strings.Contains(qPrompt, "{{") {
		t.Errorf("expected no leftover placeholders, got keywords=%q questions=%q", kwPrompt, qPrompt)
	}
}

func TestExtractorDisabledSummarySkipsCall(t *testing.T) {
	stub := withStubChatInvoker(t, stubResponse{Content: "Not expected"})

	params := map[string]any{
		"llm_id": "llm-1",
		"summary": map[string]any{
			"enabled": false,
		},
	}

	comp, err := NewExtractorComponent(params)
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}

	in := map[string]any{
		"chunks": []map[string]any{
			{"content_with_weight": "Some text."},
		},
	}

	out, err := comp.Invoke(t.Context(), nil, in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	if stub.Calls() != 0 {
		t.Errorf("expected 0 LLM calls when summary is disabled, got %d", stub.Calls())
	}

	chunks, ok := out["chunks"].([]map[string]any)
	if !ok || len(chunks) != 1 {
		t.Fatalf("expected 1 chunk output, got %v", out)
	}
	if _, has := chunks[0]["summary"]; has {
		t.Errorf("expected no summary key in chunk, got %v", chunks[0]["summary"])
	}
}

func TestExtractor_ModularConfiguration(t *testing.T) {
	paramsDisabled := map[string]any{
		"metadata": map[string]any{
			"enabled": false,
			"fields": []any{
				map[string]any{"key": "category", "type": "string"},
			},
		},
		"summary": map[string]any{
			"enabled": false,
		},
		"keywords": map[string]any{
			"top_n": 0,
		},
		"questions": map[string]any{
			"top_n": 0,
		},
		"tags": map[string]any{
			"top_n": 0,
		},
	}

	compRawA, err := NewExtractorComponent(paramsDisabled)
	if err != nil {
		t.Fatalf("NewExtractorComponent A: %v", err)
	}
	compA := compRawA.(*ExtractorComponent)
	if compA.Param.Metadata.Enabled != false {
		t.Errorf("expected metadata disabled, got %v", compA.Param.Metadata.Enabled)
	}
	if compA.Param.Summary.Enabled != false {
		t.Errorf("expected summary disabled, got %v", compA.Param.Summary.Enabled)
	}
	if compA.Param.Keywords.TopN != 0 {
		t.Errorf("expected keywords disabled (0), got %v", compA.Param.Keywords.TopN)
	}
	if compA.Param.Questions.TopN != 0 {
		t.Errorf("expected questions disabled (0), got %v", compA.Param.Questions.TopN)
	}
	if compA.Param.Tags.TopN != 0 {
		t.Errorf("expected tags disabled (0), got %v", compA.Param.Tags.TopN)
	}

	// Explicitly enabled
	paramsEnabled := map[string]any{
		"metadata": map[string]any{
			"enabled": true,
			"metadata": []any{
				map[string]any{"key": "author", "type": "string"},
			},
		},
		"summary": map[string]any{
			"enabled":       true,
			"system_prompt": "Custom summary prompt",
		},
		"keywords": map[string]any{
			"top_n":         4,
			"system_prompt": "Custom keywords prompt",
		},
		"questions": map[string]any{
			"top_n":         2,
			"system_prompt": "Custom questions prompt",
		},
		"tags": map[string]any{
			"top_n":       3,
			"tag_file_id": "file-123",
		},
	}

	compRawB, err := NewExtractorComponent(paramsEnabled)
	if err != nil {
		t.Fatalf("NewExtractorComponent B: %v", err)
	}
	compB := compRawB.(*ExtractorComponent)
	if compB.Param.Metadata.Enabled != true {
		t.Errorf("expected metadata enabled, got %v", compB.Param.Metadata.Enabled)
	}
	if len(compB.Param.Metadata.Metadata) != 1 || compB.Param.Metadata.Metadata[0].Key != "author" {
		t.Errorf("expected metadata fields with author, got %+v", compB.Param.Metadata.Metadata)
	}
	if compB.Param.Summary.Enabled != true || compB.Param.Summary.SystemPrompt != "Custom summary prompt" {
		t.Errorf("expected summary enabled with custom prompt, got %+v", compB.Param.Summary)
	}
	if compB.Param.Keywords.TopN != 4 || compB.Param.Keywords.SystemPrompt != "Custom keywords prompt" {
		t.Errorf("expected keywords 4 with custom prompt, got %+v", compB.Param.Keywords)
	}
	if compB.Param.Questions.TopN != 2 || compB.Param.Questions.SystemPrompt != "Custom questions prompt" {
		t.Errorf("expected questions 2 with custom prompt, got %+v", compB.Param.Questions)
	}
	if compB.Param.Tags.TopN != 3 || compB.Param.Tags.TagFileID != "file-123" {
		t.Errorf("expected tags 3 / file-123, got %+v", compB.Param.Tags)
	}
}

func TestExtractor_ParseMetadataFieldDefs_MapSlice(t *testing.T) {
	inputMapSlice := []map[string]any{
		{"key": "author", "type": "string", "description": "Author name"},
	}
	defs := parseMetadataFieldDefs(inputMapSlice)
	if len(defs) != 1 || defs[0].Key != "author" || defs[0].Type != "string" || defs[0].Description != "Author name" {
		t.Errorf("parseMetadataFieldDefs failed for []map[string]any: %#v", defs)
	}

	inputDefs := []common.MetadataFieldDef{
		{Key: "tag", Type: "string"},
	}
	directDefs := parseMetadataFieldDefs(inputDefs)
	if len(directDefs) != 1 || directDefs[0].Key != "tag" {
		t.Errorf("parseMetadataFieldDefs failed for []common.MetadataFieldDef: %#v", directDefs)
	}
}

func TestExtractorBuiltInDoesNotCallLLM(t *testing.T) {
	// 1b39355c regressed by making runEnableMetadata fire when only built_in_metadata was configured.
	// With the modular shape, BuiltInMetadata must never trigger an LLM call; it is applied by the finalizer.
	stub := withStubChatInvoker(t)
	params := map[string]any{
		"llm_id": "llm-1",
		"metadata": map[string]any{
			"enabled": true,
			"built_in_metadata": []any{
				map[string]any{"key": "file_name", "type": "string"},
				map[string]any{"key": "update_time", "type": "time"},
			},
			"metadata": []any{},
		},
	}
	comp, err := NewExtractorComponent(params)
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}
	in := map[string]any{
		"chunks": []map[string]any{{"text": "hello world"}},
	}
	out, err := comp.Invoke(t.Context(), nil, in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.calls.Load() != 0 {
		t.Fatalf("built_in-only must not call LLM, got %d calls, requests=%v", stub.calls.Load(), stub.requests)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %v", out)
	}
	if _, ok := chunks[0]["metadata"]; ok {
		t.Fatalf("built_in must not produce chunk metadata, got %v", chunks[0]["metadata"])
	}
}

func TestExtractorEnabledFalseDoesNotCallLLM(t *testing.T) {
	stub := withStubChatInvoker(t)
	params := map[string]any{
		"llm_id": "llm-1",
		"metadata": map[string]any{
			"enabled": false,
			"metadata": []any{
				map[string]any{"key": "author", "type": "string"},
			},
			"built_in_metadata": []any{
				map[string]any{"key": "file_name", "type": "string"},
			},
		},
	}
	comp, err := NewExtractorComponent(params)
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}
	in := map[string]any{"chunks": []map[string]any{{"text": "hello"}}}
	out, err := comp.Invoke(t.Context(), nil, in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.calls.Load() != 0 {
		t.Fatalf("enabled=false must not call LLM, got %d", stub.calls.Load())
	}
	if chunks, _ := out["chunks"].([]map[string]any); len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %v", out)
	}
}

func TestExtractor_KeywordsThenTagsSynergy(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: "Bidding, Procurement"})

	params := map[string]any{
		"llm_id": "llm-1",
		"keywords": map[string]any{
			"top_n": 2,
		},
	}
	comp, err := NewExtractorComponent(params)
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}

	in := map[string]any{
		"chunks": []map[string]any{
			{
				"docnm_kwd":           "Tender_Notice.pdf",
				"content_with_weight": "General bidding notice content.",
			},
		},
	}

	out, err := comp.Invoke(t.Context(), nil, in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	chunks, ok := out["chunks"].([]map[string]any)
	if !ok || len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %v", out)
	}

	kwds, ok := chunks[0]["important_kwd"].([]string)
	if !ok || len(kwds) != 2 {
		t.Fatalf("expected important_kwd populated with 2 keywords, got %v", chunks[0]["important_kwd"])
	}

	// Verify getChunkText on the resulting chunk merges extracted keywords and content without title pollution
	chunkText := getChunkText(chunks[0])
	if !strings.Contains(chunkText, "Bidding") || !strings.Contains(chunkText, "Procurement") || !strings.Contains(chunkText, "General bidding notice content.") {
		t.Fatalf("expected chunk text to contain content and extracted keywords, got %q", chunkText)
	}
	if strings.Contains(chunkText, "Tender_Notice") {
		t.Fatalf("expected chunk text to NOT contain title when content is present, got %q", chunkText)
	}
}

func TestExtractor_LLMCacheKey(t *testing.T) {
	k1 := extractorLLMCacheKey("keywords", "modelA", "prompt1", "text1")
	k2 := extractorLLMCacheKey("keywords", "modelA", "prompt1", "text1")
	if k1 != k2 {
		t.Errorf("extractorLLMCacheKey should be deterministic: %s != %s", k1, k2)
	}

	// Task type isolation
	kQuestions := extractorLLMCacheKey("questions", "modelA", "prompt1", "text1")
	if k1 == kQuestions {
		t.Errorf("Different task types must produce different keys: %s == %s", k1, kQuestions)
	}

	// Model isolation
	kModelB := extractorLLMCacheKey("keywords", "modelB", "prompt1", "text1")
	if k1 == kModelB {
		t.Errorf("Different models must produce different keys: %s == %s", k1, kModelB)
	}

	// NUL separator collision test ("ab", "c") vs ("a", "bc")
	kColl1 := extractorLLMCacheKey("k", "m", "ab", "c")
	kColl2 := extractorLLMCacheKey("k", "m", "a", "bc")
	if kColl1 == kColl2 {
		t.Errorf("NUL separator should prevent collisions: %s == %s", kColl1, kColl2)
	}
}

func TestExtractor_CallTextCached_NoRedis_FailOpen(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "Alpha, Beta"},
		stubResponse{Content: "What is Alpha?\nWhat is Beta?"},
		stubResponse{Content: "This is a summary without Redis."},
	)

	params := map[string]any{
		"llm_id": "llm-test-noredis",
		"keywords": map[string]any{
			"top_n": 2,
		},
		"questions": map[string]any{
			"top_n": 2,
		},
		"summary": map[string]any{
			"enabled": true,
		},
	}
	comp, err := NewExtractorComponent(params)
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}

	in := map[string]any{
		"chunks": []map[string]any{
			{"text": "Sample text for fail open test."},
		},
	}

	out, err := comp.Invoke(t.Context(), nil, in)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if calls := stub.Calls(); calls != 3 {
		t.Fatalf("Expected 3 LLM calls, got %d", calls)
	}
	ck := out["chunks"].([]map[string]any)[0]
	if sum, ok := ck["summary"].(string); !ok || sum != "This is a summary without Redis." {
		t.Errorf("got summary %v, want 'This is a summary without Redis.'", ck["summary"])
	}
}
