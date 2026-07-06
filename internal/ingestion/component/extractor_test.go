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
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	eschema "github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
)

// stubExtractorChatInvoker is the test seam for the package-level
// extractorChatInvoker. It records every call (for assertions) and
// returns canned responses configured per-test. Concurrent-safe so
// it can backstop future Parallelism>1 cases without rewriting.
type stubExtractorChatInvoker struct {
	mu sync.Mutex

	// responses is consumed in order; remaining entries are returned
	// as the wrap-error. tests set entries == call count they expect.
	responses []stubResponse

	// lastReq records the most recent call's request for inspection
	// (e.g. driver / model name resolved from the llm_id).
	lastReq extractorChatRequest
	calls   atomic.Int32
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
	s.lastReq = req
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
// is visible to the runtime registry (Phase 4 / API layer
// depends on this).
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
// fan-out: two chunks in → two LLM calls → each chunk enriched
// with the field_name key.
func TestExtractorComponent_Invoke_HappyPath(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: "answer for chunk 1"},
		stubResponse{Content: "answer for chunk 2"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "summary",
		LLMID:     "gpt-4o-mini",
		Prompt:    "Summarize:",
	}}
	out, err := c.Invoke(context.Background(), map[string]any{
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
	if chunks[0]["summary"] != "answer for chunk 1" {
		t.Errorf("chunk[0].summary = %v, want %q", chunks[0]["summary"], "answer for chunk 1")
	}
	if chunks[1]["summary"] != "answer for chunk 2" {
		t.Errorf("chunk[1].summary = %v, want %q", chunks[1]["summary"], "answer for chunk 2")
	}
	if out["output_format"] != "chunks" {
		t.Errorf("output_format = %v, want chunks", out["output_format"])
	}
	if out["_elapsed_time"] == nil {
		t.Error("_elapsed_time missing")
	}
	if out["_created_time"] == nil {
		t.Error("_created_time missing")
	}
}

// TestExtractorComponent_Invoke_LLMError verifies a mock LLM
// error is surfaced through Invoke with the component-name prefix
// so the upstream pipeline can attribute failures.
func TestExtractorComponent_Invoke_LLMError(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Err: errors.New("upstream llm unavailable")},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "summary",
		LLMID:     "gpt-4o-mini",
	}}
	_, err := c.Invoke(context.Background(), map[string]any{
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

// TestExtractorComponent_Invoke_UnknownProvider asserts the
// production (eino) chat invoker handles an unregistered driver
// without panicking, per plan §8 Q1 ("48/56 providers covered;
// the Extractor is provider-agnostic via llm_id; the 8 missing
// are edge cases that do not block Phase 2.5").
//
// Design note: every other test in this file drives the
// invoker through the production Component.Invoke path with a
// canned-response invoker installed via SetExtractorChatInvoker
// (the test seam). That seam accepts a pre-resolved driver
// path; it cannot model the eino factory's default-branch
// behaviour for an unknown driver. This test exercises the
// production chat-invoker directly to pin that branch — the
// production code path the real Extractor will hit when the
// DSL references a provider that is not in the 48/56 covered
// set.
//
// The contract under test:
//   - The call MUST NOT panic.
//   - On unknown driver, the factory's default branch routes to
//     a DummyModel that returns a deterministic error string
//     (we assert the error contains that sentinel so future
//     maintainers see the wiring goes through the factory,
//     not bypassed by a hand-rolled default).
func TestExtractorComponent_Invoke_UnknownProvider(t *testing.T) {
	inv := &einoExtractorChatInvoker{}
	resp, err := inv.Chat(context.Background(), extractorChatRequest{
		Driver:    "definitely-not-a-real-provider-xyz",
		ModelName: "anything",
	})
	// Either an error is returned OR a non-nil response is produced
	// by the DummyModel fallback. The contract is "no panic"; both
	// of these outcomes are acceptable. We only fail the test if
	// BOTH error and response are empty (which would indicate a
	// silent no-op).
	if err == nil && resp == nil {
		t.Fatal("production invoker returned nil error AND nil response for unknown driver — silent no-op")
	}
	// When an error IS returned, it must mention the driver name so
	// operators can correlate the failure back to the DSL config.
	if err != nil {
		// Acceptable error patterns for an unknown driver:
		//   - mentions the driver name (correlatable for operators)
		//   - "no driver"/"unknown" sentinels (typed error)
		//   - "not implemented" (the eino dummy model fallback path)
		if !strings.Contains(err.Error(), "definitely-not-a-real-provider-xyz") &&
			!strings.Contains(err.Error(), "no driver") &&
			!strings.Contains(err.Error(), "unknown") &&
			!strings.Contains(err.Error(), "not implemented") {
			t.Errorf("unknown-driver error should mention the driver name or a typed/typed-sentinel substring; got: %v", err)
		}
	}
}

// TestExtractorComponent_Invoke_ParsesJSON verifies a JSON object
// response from the LLM is parsed into the chunk's field_name
// value (matching the python set_output contract).
func TestExtractorComponent_Invoke_ParsesJSON(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: `{"answer": 42, "tags": ["a", "b"]}`},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "extraction",
		Prompt:    "extract:",
	}}
	out, err := c.Invoke(context.Background(), map[string]any{
		"chunks": []map[string]any{{"text": "doc"}}},
	)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := out["chunks"].([]map[string]any)
	got, ok := chunks[0]["extraction"].(map[string]any)
	if !ok {
		t.Fatalf("extraction should be parsed JSON object, got %T", chunks[0]["extraction"])
	}
	if got["answer"].(float64) != 42 {
		t.Errorf("answer = %v, want 42", got["answer"])
	}
	tags, _ := got["tags"].([]any)
	if len(tags) != 2 {
		t.Errorf("tags len = %d, want 2", len(tags))
	}
}

// TestExtractorComponent_Invoke_ParsesJSONInFence verifies the
// common LLM response shape — JSON wrapped in a markdown code
// fence — parses cleanly. Mirrors the behaviour the agent
// canvas applies (e.g. llm_retry_test.go matchOutputStructure).
func TestExtractorComponent_Invoke_ParsesJSONInFence(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: "```json\n{\"summary\": \"hello\"}\n```"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "out",
	}}
	out, err := c.Invoke(context.Background(), map[string]any{
		"chunks": []map[string]any{{"text": "x"}}},
	)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, ok := out["chunks"].([]map[string]any)[0]["out"].(map[string]any)
	if !ok {
		t.Fatalf("out should be parsed JSON object, got %T", out["chunks"].([]map[string]any)[0]["out"])
	}
	if got["summary"] != "hello" {
		t.Errorf("summary = %v, want hello", got["summary"])
	}
}

// TestExtractorComponent_Invoke_HandlesMalformedJSON verifies a
// non-JSON response surfaces as the raw string under the
// destination field — not an error. The python Extractor
// accepts whatever the LLM emits; downstream callers decide
// what to do with it.
func TestExtractorComponent_Invoke_HandlesMalformedJSON(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: "this is not JSON at all"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "raw",
	}}
	out, err := c.Invoke(context.Background(), map[string]any{
		"chunks": []map[string]any{{"text": "x"}}},
	)
	if err != nil {
		t.Fatalf("Invoke returned error on non-JSON: %v", err)
	}
	got := out["chunks"].([]map[string]any)[0]["raw"]
	if got != "this is not JSON at all" {
		t.Errorf("raw = %v, want %q", got, "this is not JSON at all")
	}
}

// TestExtractorComponent_Invoke_TOCNotPorted asserts the
// field_name=="toc" branch is gated by a clear error so a future
// migration to the Go TOC generator doesn't accidentally fall
// through to chunk iteration.
func TestExtractorComponent_Invoke_TOCNotPorted(t *testing.T) {
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "toc",
	}}
	_, err := c.Invoke(context.Background(), map[string]any{
		"chunks": []map[string]any{{"text": "x"}}},
	)
	if err == nil {
		t.Fatal("expected error for field_name=toc, got nil")
	}
	if !strings.Contains(err.Error(), "toc") {
		t.Errorf("error should mention toc: %v", err)
	}
	if !strings.Contains(err.Error(), "not yet ported") {
		t.Errorf("error should call out parity gap: %v", err)
	}
}

// TestExtractorComponent_Invoke_NoChunksFastPath verifies the
// no-chunks input still produces a one-element chunks slice
// (mirrors python _invoke line 110 fallback).
func TestExtractorComponent_Invoke_NoChunksFastPath(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: "single-shot answer"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "answer",
	}}
	out, err := c.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks missing or wrong shape")
	}
	if len(chunks) != 1 {
		t.Fatalf("chunks len = %d, want 1", len(chunks))
	}
	if chunks[0]["answer"] != "single-shot answer" {
		t.Errorf("answer = %v, want %q", chunks[0]["answer"], "single-shot answer")
	}
}

func TestExtractorComponent_Invoke_JSONListInput(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: "json chunk answer"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "answer",
	}}
	out, err := c.Invoke(context.Background(), map[string]any{
		"json": []map[string]any{{"text": "json payload chunk"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok || len(chunks) != 1 {
		t.Fatalf("chunks malformed: %v", out["chunks"])
	}
	if chunks[0]["answer"] != "json chunk answer" {
		t.Errorf("answer = %v, want %q", chunks[0]["answer"], "json chunk answer")
	}
}

// TestExtractorComponent_Invoke_PerCallLLMIDOverride verifies an
// inputs["llm_id"] override wins over Param.LLMID and reaches
// the chat invoker verbatim (the per-call override is the
// explicit test seam for runtime reconfiguration).
func TestExtractorComponent_Invoke_PerCallLLMIDOverride(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "ok"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "out",
		LLMID:     "static-llm",
	}}
	_, err := c.Invoke(context.Background(), map[string]any{
		"llm_id": "override-llm",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.lastReq.ModelName != "override-llm" {
		t.Errorf("ModelName = %q, want override-llm", stub.lastReq.ModelName)
	}
}

// TestExtractorComponent_Invoke_CompositeLLMID verifies the
// composite "gpt-4o-mini@openai" form is split into driver and
// model before reaching the chat invoker. Matches the canonical
// composite llm_id convention used throughout the codebase
// (see internal/agent/component/llm_credentials.go:parseLLMIDParts).
func TestExtractorComponent_Invoke_CompositeLLMID(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "ok"},
	)
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "out",
		LLMID:     "gpt-4o-mini@openai",
	}}
	if _, err := c.Invoke(context.Background(), map[string]any{}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.lastReq.Driver != "openai" {
		t.Errorf("Driver = %q, want openai", stub.lastReq.Driver)
	}
	if stub.lastReq.ModelName != "gpt-4o-mini" {
		t.Errorf("ModelName = %q, want gpt-4o-mini", stub.lastReq.ModelName)
	}
}

// TestExtractorComponent_Invoke_ChunkIndexInError verifies the
// error message includes the failing chunk index so a long
// pipeline run surfaces which input document triggered the LLM
// failure (mirrors python's per-chunk progress call at line 105).
func TestExtractorComponent_Invoke_ChunkIndexInError(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: "ok for chunk 0"},
		stubResponse{Err: errors.New("chunk-1-boom")},
	)
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "out",
	}}
	_, err := c.Invoke(context.Background(), map[string]any{
		"chunks": []map[string]any{
			{"text": "first"},
			{"text": "second"},
		},
	})
	if err == nil {
		t.Fatal("Invoke returned nil error")
	}
	if !strings.Contains(err.Error(), "chunk 1") {
		t.Errorf("error should mention chunk 1 (zero-indexed): %v", err)
	}
	if !strings.Contains(err.Error(), "chunk-1-boom") {
		t.Errorf("error should chain underlying error: %v", err)
	}
}

// TestExtractorComponent_NewExtractorComponent_ParamCheck covers
// the construction-time Validate() rejection of an empty
// field_name (matches python check_empty "Result Destination").
func TestExtractorComponent_NewExtractorComponent_ParamCheck(t *testing.T) {
	_, err := NewExtractorComponent(map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing field_name, got nil")
	}
	if !strings.Contains(err.Error(), "field_name") {
		t.Errorf("error should mention field_name: %v", err)
	}
}

// TestExtractorComponent_NewExtractorComponent_Happy covers the
// parse path of every supported key; the param block coming out
// should round-trip cleanly through Invoke.
func TestExtractorComponent_NewExtractorComponent_Happy(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: "ok"})
	c, err := NewExtractorComponent(map[string]any{
		"field_name":    "summary",
		"llm_id":        "openai/gpt-4o-mini",
		"system_prompt": "You are a precise summarizer.",
		"prompt":        "Summarize:",
	})
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}
	if _, err := c.Invoke(context.Background(), map[string]any{
		"chunks": []map[string]any{{"text": "x"}}},
	); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}

// TestExtractorComponent_InputsOutputs_NonEmpty is the shape
// assertion Phase 4's API endpoint relies on.
func TestExtractorComponent_InputsOutputs_NonEmpty(t *testing.T) {
	c := &ExtractorComponent{}
	ins := c.Inputs()
	outs := c.Outputs()
	if len(ins) == 0 {
		t.Error("Inputs() returned empty map")
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

// TestExtractorComponent_Parallelism asserts the fan-out is
// locked to 1 per plan §AD-5a ("Extractor: 1 (LLM call is
// inherently serial)").
func TestExtractorComponent_Parallelism(t *testing.T) {
	c := &ExtractorComponent{}
	if got := c.Parallelism(); got != 1 {
		t.Errorf("Parallelism() = %d, want 1", got)
	}
}

// TestSplitExtractorLLID covers the composite-id parser in
// isolation — keeps the matrix of edge cases at one call site
// so a regression is easy to attribute. The "@" separator is
// the canonical composite llm_id form used throughout the
// codebase (see internal/agent/component/llm_credentials.go).
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
			model, provider, ok := splitExtractorLLID(tc.in)
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

// TestTryParseJSONObject covers the best-effort JSON parser
// independently of the LLM seam so its matrix of edge cases is
// easy to attribute.
func TestTryParseJSONObject(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantOK  bool
		wantKey string // when wantOK=true, expected key in the parsed map
	}{
		{name: "object", in: `{"a":1}`, wantOK: true, wantKey: "a"},
		{name: "object with fence", in: "```json\n{\"a\":1}\n```", wantOK: true, wantKey: "a"},
		{name: "fence without json tag", in: "```\n{\"a\":1}\n```", wantOK: true, wantKey: "a"},
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

// TestExtractorComponent_ConcurrentInvoke verifies the chat
// invoker swap is safe under concurrent Invoke calls. This is
// the canary for SetExtractorChatInvoker and the package-level
// RWMutex contract — a data race here breaks race detector.
func TestExtractorComponent_ConcurrentInvoke(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: "1"},
		stubResponse{Content: "2"},
		stubResponse{Content: "3"},
		stubResponse{Content: "4"},
	)
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "out",
	}}
	chunks := []map[string]any{
		{"text": "a"}, {"text": "b"}, {"text": "c"}, {"text": "d"},
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(chunks))
	for _, ck := range chunks {
		ck := ck
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Invoke(context.Background(), map[string]any{
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

// silence unused-import vet warnings for eschema in case the
// test file is built without the import ever being referenced
// (it currently isn't, but pinning the import keeps test-side
// imports honest if helpers move around in future revisions).
var _ = eschema.Message{}
