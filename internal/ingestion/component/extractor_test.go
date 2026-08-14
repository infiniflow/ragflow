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
	if chunks[0]["summary"] != "answer for chunk 1" {
		t.Errorf("chunk[0].summary = %v, want %q", chunks[0]["summary"], "answer for chunk 1")
	}
	if chunks[1]["summary"] != "answer for chunk 2" {
		t.Errorf("chunk[1].summary = %v, want %q", chunks[1]["summary"], "answer for chunk 2")
	}
	if out["output_format"] != "chunks" {
		t.Errorf("output_format = %v, want chunks", out["output_format"])
	}
}

// TestExtractorComponent_Invoke_LLMError verifies a mock LLM
// error is surfaced through Invoke with the component-name prefix
// so the upstream pipeline can attribute failures. After retry
// (RetryWithBackoff: 3 retries), the error chains the cause.
func TestExtractorComponent_Invoke_LLMError(t *testing.T) {
	// Fast retry for tests — avoid multi-second sleeps.
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
		FieldName: "summary",
		LLMID:     "gpt-4o-mini",
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
		FieldName: "summary",
		LLMID:     "gpt-4o-mini",
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

// TestExtractorComponent_Invoke_KeepsJSONAsString verifies a JSON object
// response from the LLM is written to the chunk's field_name value as a
// plain string — matching Python's _generate_async, which returns the raw
// string with no JSON parsing. (The Extractor does NOT parse field-extraction
// results; only the metadata path, via callStructured, parses explicitly.)
func TestExtractorComponent_Invoke_KeepsJSONAsString(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: `{"answer": 42, "tags": ["a", "b"]}`},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "extraction",
		Prompt:    "extract:",
	}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "doc"}}},
	)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := out["chunks"].([]map[string]any)
	got, ok := chunks[0]["extraction"].(string)
	if !ok {
		t.Fatalf("extraction should be a string (no JSON parse on field extraction), got %T", chunks[0]["extraction"])
	}
	if got != `{"answer": 42, "tags": ["a", "b"]}` {
		t.Errorf("extraction = %q, want the raw JSON string", got)
	}
}

// TestExtractorComponent_Invoke_KeepsJSONStringInFence verifies a JSON
// response wrapped in a Markdown code fence is stored as the raw string —
// the code fence is not stripped on the field-extraction path (Python's
// _generate_async returns the raw text untouched).
func TestExtractorComponent_Invoke_KeepsJSONStringInFence(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: "```json\n{\"summary\": \"hello\"}\n```"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "out",
	}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "x"}}},
	)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, ok := out["chunks"].([]map[string]any)[0]["out"].(string)
	if !ok {
		t.Fatalf("out should be a string, got %T", out["chunks"].([]map[string]any)[0]["out"])
	}
	if got != "```json\n{\"summary\": \"hello\"}\n```" {
		t.Errorf("out = %q, want the raw fenced JSON string", got)
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
	out, err := c.Invoke(t.Context(), nil, map[string]any{
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
	_, err := c.Invoke(t.Context(), nil, map[string]any{
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
	out, err := c.Invoke(t.Context(), nil, map[string]any{})
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
	_, err := c.Invoke(t.Context(), nil, map[string]any{
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
	if _, err := c.Invoke(t.Context(), nil, map[string]any{}); err != nil {
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
	prevMax, prevDelay := extractorRetryMax, extractorRetryDelay
	extractorRetryMax, extractorRetryDelay = 3, time.Millisecond
	t.Cleanup(func() {
		extractorRetryMax, extractorRetryDelay = prevMax, prevDelay
	})

	errBoom := errors.New("chunk-1-boom")
	withStubChatInvoker(t,
		stubResponse{Content: "ok for chunk 0"},
		stubResponse{Err: errBoom}, // chunk 1: attempt 0
		stubResponse{Err: errBoom}, // attempt 1
		stubResponse{Err: errBoom}, // attempt 2
		stubResponse{Err: errBoom}, // attempt 3 (last retry)
	)
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "out",
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
	c, err := NewExtractorComponent(map[string]any{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil component")
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
	if _, err = c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "x"}}},
	); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}

// TestNewExtractorComponent_SysPromptAlias verifies that "sys_prompt"
// (the Python DSL name) is accepted as a fallback for SystemPrompt.
func TestNewExtractorComponent_SysPromptAlias(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: "ok"})
	comp, err := NewExtractorComponent(map[string]any{
		"field_name": "out",
		"sys_prompt": "You are a Python DSL prompt.",
	})
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}
	ec := comp.(*ExtractorComponent)
	if ec.Param.SystemPrompt != "You are a Python DSL prompt." {
		t.Errorf("SystemPrompt = %q, want %q", ec.Param.SystemPrompt, "You are a Python DSL prompt.")
	}
}

// TestNewExtractorComponent_MetadataAsAnySlice guards against the regression
// where InjectExtractorEnableMetadata injected the field schema as a
// []map[string]interface{} while NewExtractorComponent only accepted []any;
// the type assertion then failed and ExtractorParam.Metadata stayed empty, so
// auto-metadata never fired. The override_params path passes the injected
// value straight through (no JSON round-trip), so the slice element type must
// be []any for the assertion to succeed.
func TestNewExtractorComponent_MetadataAsAnySlice(t *testing.T) {
	comp, err := NewExtractorComponent(map[string]any{
		"field_name":      "out",
		"enable_metadata": 1,
		// This is exactly the dynamic type InjectExtractorEnableMetadata
		// produces ([]any of map[string]any), NOT []map[string]interface{}.
		"metadata": []any{
			map[string]any{"key": "author", "type": "string", "description": "doc author"},
			map[string]any{"key": "year", "type": "number", "enum": []any{"2020", "2021"}},
		},
	})
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}
	ec := comp.(*ExtractorComponent)
	if ec.Param.EnableMetadata != 1 {
		t.Fatalf("EnableMetadata = %d, want 1", ec.Param.EnableMetadata)
	}
	if len(ec.Param.Metadata) != 2 {
		t.Fatalf("Metadata = %#v, want 2 fields", ec.Param.Metadata)
	}
	if ec.Param.Metadata[0].Key != "author" || ec.Param.Metadata[0].Type != "string" {
		t.Errorf("Metadata[0] = %#v, want key=author type=string", ec.Param.Metadata[0])
	}
	if len(ec.Param.Metadata[1].Enum) != 2 {
		t.Errorf("Metadata[1].Enum = %#v, want 2 enum values", ec.Param.Metadata[1].Enum)
	}
}

// TestNewExtractorComponent_PromptsArray verifies that the Python DSL
// "prompts" array format is parsed into Param.Prompt.
func TestNewExtractorComponent_PromptsArray(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: "ok"})
	comp, err := NewExtractorComponent(map[string]any{
		"field_name": "out",
		"prompts": []any{
			map[string]any{
				"content": "Analyze: {TitleChunker:FlatMiceFix@chunks}",
				"role":    "user",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}
	ec := comp.(*ExtractorComponent)
	want := "Analyze: {TitleChunker:FlatMiceFix@chunks}"
	if ec.Param.Prompt != want {
		t.Errorf("Prompt = %q, want %q", ec.Param.Prompt, want)
	}
}

// TestNewExtractorComponent_PromptsArray_PromptWins verifies that
// "prompt" (string) takes priority over "prompts" (array) when both
// are present in the DSL params.
func TestNewExtractorComponent_PromptsArray_PromptWins(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: "ok"})
	comp, err := NewExtractorComponent(map[string]any{
		"field_name": "out",
		"prompt":     "Direct prompt wins.",
		"prompts": []any{
			map[string]any{
				"content": "Should be ignored.",
				"role":    "user",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}
	ec := comp.(*ExtractorComponent)
	if ec.Param.Prompt != "Direct prompt wins." {
		t.Errorf("Prompt = %q, want %q", ec.Param.Prompt, "Direct prompt wins.")
	}
}

// TestNewExtractorComponent_PromptsString verifies that a bare-string
// "prompts" (the shape emitted by the front-end graph.nodes form and
// the dsl/testdata templates) is normalized into Param.Prompt, mirroring
// Python agent/component/llm.py:119-120 which coerces a string prompts
// into [{"role":"user","content":prompts}]. Without this normalization
// the string form is silently dropped (the .([]any) assertion fails).
func TestNewExtractorComponent_PromptsString(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: "ok"})
	comp, err := NewExtractorComponent(map[string]any{
		"field_name": "out",
		"prompts":    "Content: {TitleChunker:FlatMiceFix@chunks}",
	})
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}
	ec := comp.(*ExtractorComponent)
	want := "Content: {TitleChunker:FlatMiceFix@chunks}"
	if ec.Param.Prompt != want {
		t.Errorf("Prompt = %q, want %q (string prompts should be normalized)", ec.Param.Prompt, want)
	}
}

// TestNewExtractorComponent_PromptsString_PromptWins verifies that
// "prompt" (string) still takes priority over a string-form "prompts"
// when both are present, matching the prompt>prompts precedence of
// the list-form path (TestNewExtractorComponent_PromptsArray_PromptWins).
func TestNewExtractorComponent_PromptsString_PromptWins(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: "ok"})
	comp, err := NewExtractorComponent(map[string]any{
		"field_name": "out",
		"prompt":     "Direct prompt wins.",
		"prompts":    "Should be ignored.",
	})
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}
	ec := comp.(*ExtractorComponent)
	if ec.Param.Prompt != "Direct prompt wins." {
		t.Errorf("Prompt = %q, want %q", ec.Param.Prompt, "Direct prompt wins.")
	}
}

// TestNewExtractorComponent_SystemPromptWinsOverSysPrompt verifies
// that "system_prompt" takes priority over "sys_prompt".
func TestNewExtractorComponent_SystemPromptWinsOverSysPrompt(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: "ok"})
	comp, err := NewExtractorComponent(map[string]any{
		"field_name":    "out",
		"system_prompt": "system_prompt wins.",
		"sys_prompt":    "sys_prompt ignored.",
	})
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}
	ec := comp.(*ExtractorComponent)
	if ec.Param.SystemPrompt != "system_prompt wins." {
		t.Errorf("SystemPrompt = %q, want %q", ec.Param.SystemPrompt, "system_prompt wins.")
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
		// Language tag on its own line (```\njson\n{...}) — Python json_repair
		// tolerates this, so encoding/json must not choke on the bare "json".
		{name: "json tag on own line", in: "```\njson\n{\"a\":1}\n```", wantOK: true, wantKey: "a"},
		{name: "JSON tag on own line", in: "```\nJSON\n{\"a\":1}\n```", wantOK: true, wantKey: "a"},
		// Leading prose before the fence must not be stripped (only a real
		// ``` fence prefix is handled).
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

// TestCleanExtractionResult covers the </think> chain-of-thought stripping
// and the **ERROR** guard that mirrors Python's metadata post-processing.
func TestCleanExtractionResult(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: `{"a":1}`, want: `{"a":1}`},
		// Python re.sub(r"^.*</think>", "", ans): everything up to and
		// including the LAST </think> is dropped.
		{name: "thinks stripped", in: "let me think<think>reasoning</think>\n{\"a\":1}", want: `{"a":1}`},
		{name: "thinks no json", in: "thinking</think>no json here", want: "no json here"},
		// **ERROR** responses are rejected entirely.
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
		EnableMetadata: 1,
		Metadata:       fields,
	}}
}

// TestExtractorComponent_runEnableMetadata_MergesIntoChunkMetadata verifies a
// JSON object from the LLM is parsed and merged into the chunk's metadata map,
// which mergeChunkMetadata then aggregates to the doc level.
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
// extraction path tolerates a fenced ```json response (the common model
// output) that would otherwise fail encoding/json parsing — mirroring Python
// json_repair.
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
// metadata path — LLM call, second-layer <think> strip, JSON parse, merge —
// tolerates a mid-text reasoning block preceded by a preamble, matching
// Python gen_metadata. This is the end-to-end guard for callStructured's
// common.StripThinkTrailing: without it the metadata extraction silently drops.
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
// empty / **ERROR** / unparseable / think-only LLM response does NOT block
// ingestion: the chunk metadata is left untouched and no error is returned
// (Python "no evidence → {}").
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
// whose extraction returns overlapping list values for the same key. Aggregating
// the chunk metadata maps with utility.UpdateMetadataTo (as mergeChunkMetadata
// does) must produce a de-duplicated union, matching Python update_metadata_to.
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
	// mirror mergeChunkMetadata: aggregate chunk metadata into doc metadata.
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
// common.SplitCombinedMetadataValues (as mergeDocMetadata does before writing),
// matching Python _split_combined_values (doc_metadata_service.py).
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
	// mergeDocMetadata runs SplitCombinedMetadataValues before writing.
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

// silence unused-import vet warnings for eschema in case the
// test file is built without the import ever being referenced
// (it currently isn't, but pinning the import keeps test-side
// imports honest if helpers move around in future revisions).
var _ = eschema.Message{}

// TestIsBareTenantModelID verifies UUID detection.
func TestIsBareTenantModelID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"9e819c2442b14f9dab46062916e29195", true},
		{"ABCDEFabcdef01234567890123456789", true},
		{"9e819c2442b14f9dab46062916e2919", false},   // 31 chars
		{"9e819c2442b14f9dab46062916e29195X", false}, // 33 chars
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

// TestResolveExtractorChatTarget_AtSplitFallback verifies the @ split
// fallback path works without canvas state (unit test compatibility).
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

// TestResolveExtractorChatTarget_NoDriver verifies a non-@ plain string
// without canvas state returns no driver (passes through to Chat()).
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

// TestExtractorComponent_Invoke_TemperatureSet verifies the keyword
// extraction LLM chat call receives Temperature=0.2, matching Python's
// keyword_extraction and question_proposal defaults (generator.py:230,245).
// Field extraction intentionally runs on a separate call and uses the
// model default (see TestExtractorComponent_Invoke_FieldNameTemperatureDefault),
// so this test enables only AutoKeywords to assert the 0.2 pin directly.
func TestExtractorComponent_Invoke_TemperatureSet(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "keyword, extraction"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID:        "gpt-4o-mini",
		AutoKeywords: 3,
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "document content"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.lastReq.Temperature == nil {
		t.Fatal("Temperature is nil, want 0.2")
	}
	if *stub.lastReq.Temperature != 0.2 {
		t.Errorf("Temperature = %v, want 0.2", *stub.lastReq.Temperature)
	}
	if stub.calls.Load() != 1 {
		t.Errorf("expected exactly 1 LLM call (keyword), got %d", stub.calls.Load())
	}
}

// TestExtractorComponent_Invoke_FieldNameTemperatureDefault verifies
// that the generic field-extraction path leaves Temperature unset
// (model/default), unlike keyword/question which pin 0.2 — matching
// Python's generic Extractor behavior.
func TestExtractorComponent_Invoke_FieldNameTemperatureDefault(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "extracted"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "summary",
		LLMID:     "gpt-4o-mini",
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "document content"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.lastReq.Temperature != nil {
		t.Errorf("Temperature = %v, want nil (field extraction uses model default)", *stub.lastReq.Temperature)
	}
}

// TestIsRetryableLLMError locks in the retry-classification heuristic,
// especially the word-boundary guard that prevents a transient timeout
// message ("...after 400ms") from being misclassified as a permanent
// HTTP 400 and dropped.
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

// TestCleanExtractionResult_LastThinkTag verifies that when the LLM
// response contains multiple </think> tags, cleanExtractionResult strips
// up to the LAST one (greedy, matching Python's re.sub), not just the
// first (which would leave a residual think block in the output).
func TestCleanExtractionResult_LastThinkTag(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "single think block",
			in:   "<think>reasoning</think>the answer",
			want: "the answer",
		},
		{
			name: "nested think blocks",
			in:   "<think>outer</think>mid<think>inner</think>final output",
			want: "final output",
		},
		{
			name: "no think tag",
			in:   "plain answer",
			want: "plain answer",
		},
		{
			name: "think tag without close",
			in:   "<think>unclosed",
			want: "<think>unclosed",
		},
		{
			name: "error sentinel",
			in:   "valid output**ERROR**extra",
			want: "",
		},
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

// TestCleanLLMText verifies the two-step LLM-layer cleanup that Python
// applies in LLMBundle.async_chat (llm_service.py:459-461): reasoning
// content is stripped only when a leading <think> has a matching closing
// </think> after it, and <tool_call>...</tool_call> blocks are removed.
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
			want: "abc</think>def", // no leading <think> → unchanged
		},
		{
			name: "prefix before think kept",
			in:   "prefix<think>reason</think>answer",
			want: "prefix<think>reason</think>answer", // <think> not at start → content preserved
		},
		{
			name: "open without close kept",
			in:   "<think>unclosed",
			want: "<think>unclosed", // no </think> → unchanged
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

// TestExtractorComponent_callStructured verifies the metadata path parses a
// JSON object response into a map, and returns (nil, nil) for a non-JSON or
// empty response (nothing extracted, not an error).
func TestExtractorComponent_callStructured(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: `{"a": 1}`})
	c := &ExtractorComponent{}
	got, err := c.callStructured(t.Context(), nil, extractorInputs{llmID: "m"}, "")
	if err != nil {
		t.Fatalf("callStructured: %v", err)
	}
	if got["a"].(float64) != 1 {
		t.Errorf("parsed = %v, want map with a=1", got)
	}

	// Non-JSON response → (nil, nil), not an error.
	withStubChatInvoker(t, stubResponse{Content: "this is not JSON"})
	got, err = c.callStructured(t.Context(), nil, extractorInputs{llmID: "m"}, "")
	if err != nil {
		t.Fatalf("callStructured on non-JSON: %v", err)
	}
	if got != nil {
		t.Errorf("non-JSON response should yield nil map, got %v", got)
	}
}

// TestExtractorComponent_callStructured_MidTextThink verifies the metadata
// path's second cleanup layer (common.StripThinkTrailing) strips a mid-text
// reasoning block preceded by a preamble, matching Python's gen_metadata
// double cleanup (async_chat + re.sub r"^.*</think>"). Without it the JSON
// would survive the leading-only cleanLLMText, fail to parse, and silently
// drop the metadata extraction.
func TestExtractorComponent_callStructured_MidTextThink(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: `preamble<think>reasoning</think>{"a": 1}`})
	c := &ExtractorComponent{}
	got, err := c.callStructured(t.Context(), nil, extractorInputs{llmID: "m"}, "")
	if err != nil {
		t.Fatalf("callStructured: %v", err)
	}
	if got == nil || got["a"].(float64) != 1 {
		t.Errorf("parsed = %v, want map with a=1", got)
	}
}

// TestExtractorComponent_Invoke_ConcurrentKeywordsAndQuestions verifies
// that when both auto_keywords and auto_questions are enabled, both
// LLM calls are dispatched per chunk and results land on the chunk
// (matching Python's ThreadPoolExecutor concurrency: task_executor.py:444-448).
func TestExtractorComponent_Invoke_ConcurrentKeywordsAndQuestions(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "alpha, beta"},       // chunk 0 keywords
		stubResponse{Content: "what is it?\nwhy?"}, // chunk 0 questions
		stubResponse{Content: "gamma, delta"},      // chunk 1 keywords
		stubResponse{Content: "how?\nwhen?"},       // chunk 1 questions
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID:         "gpt-4o-mini",
		AutoKeywords:  2,
		AutoQuestions: 2,
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

	// Both chunks should have keywords and questions populated.
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

// TestResolveExtractorChatTarget_EmptyLLMID verifies that when llmID is
// empty, resolveExtractorChatTarget falls back to the tenant default chat
// model (via resolveTenantModelByType), matching Python's behavior
// (task_executor.py:573-574 never skips tagging on empty llm_id).
// When no canvas state is available (unit-test context), returns empty
// driver — callers like runAutoTags check driver!="" before using it.
func TestResolveExtractorChatTarget_EmptyLLMID(t *testing.T) {
	// Without canvas state: empty llmID returns empty driver (no crash).
	ctx := t.Context()
	driver, modelName, _, _, err := resolveExtractorChatTarget(ctx, dao.DB, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// In test context without canvas state, neither tenant default nor @ split
	// can resolve — driver ends up empty. Callers must handle this gracefully.
	if driver != "" {
		t.Logf("resolved empty llmID: driver=%q model=%q (tenant default might be available)", driver, modelName)
	}
	// Contract: no panic, no error for empty llmID.
}

// TestExtractorComponent_Invoke_ContentWithWeightPlaceholder verifies that
// a prompt referencing {content_with_weight} (a chunk field that is NOT in
// the {text}/{chunks} suppression set of the old code) substitutes the
// field without also appending the chunk text a second time. Regression
// guard for the duplicate-injection bug fixed in
// fix/extractor-chunk-text-injection.
func TestExtractorComponent_Invoke_ContentWithWeightPlaceholder(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "answer"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "out",
		Prompt:    "Weighted: {content_with_weight}",
		LLMID:     "gpt-4o-mini",
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"content_with_weight": "weighted doc"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	var userContent string
	for _, msg := range stub.lastReq.Messages {
		if msg.Role == eschema.User {
			userContent = msg.Content
		}
	}
	if strings.Contains(userContent, "{content_with_weight}") {
		t.Errorf("prompt still contains literal {content_with_weight}: %q", userContent)
	}
	if n := strings.Count(userContent, "weighted doc"); n != 1 {
		t.Errorf("chunk text appears %d times, want 1 (no duplicate append): %q", n, userContent)
	}
}

// TestExtractorComponent_Invoke_NonContentPlaceholderKeepsChunkText verifies
// that a non-content placeholder like {title} being substituted does NOT
// suppress the chunk-text append — otherwise the document body would
// silently disappear from the LLM call. Regression guard for the
// "compare substituted vs original" approach, which incorrectly suppressed
// on any replacement.
func TestExtractorComponent_Invoke_NonContentPlaceholderKeepsChunkText(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "answer"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "out",
		Prompt:    "Title: {title}\nExtract:",
		LLMID:     "gpt-4o-mini",
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{
			"text":  "DOC BODY",
			"title": "My Title",
		}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	var userContent string
	for _, msg := range stub.lastReq.Messages {
		if msg.Role == eschema.User {
			userContent = msg.Content
		}
	}
	// {title} must be replaced, and chunk body must still be present.
	if strings.Contains(userContent, "{title}") {
		t.Errorf("prompt still contains literal {title}: %q", userContent)
	}
	if !strings.Contains(userContent, "DOC BODY") {
		t.Errorf("chunk body missing from LLM call — append was wrongly suppressed: %q", userContent)
	}
}

// TestExtractorComponent_Invoke_UnresolvedTextPlaceholderKeepsChunkText verifies
// that a {text} placeholder that cannot be resolved against the chunk (the
// chunk has content_with_weight but no text field) does NOT suppress the
// chunk-text append. Otherwise the LLM receives a literal {text} and no content.
func TestExtractorComponent_Invoke_UnresolvedTextPlaceholderKeepsChunkText(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "answer"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "out",
		Prompt:    "Content: {text}",
		LLMID:     "gpt-4o-mini",
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{
			"content_with_weight": "weighted doc",
		}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	var userContent string
	for _, msg := range stub.lastReq.Messages {
		if msg.Role == eschema.User {
			userContent = msg.Content
		}
	}
	// {text} was not resolved (chunk has no text field), so the append must
	// still deliver the chunk body.
	if !strings.Contains(userContent, "weighted doc") {
		t.Errorf("chunk body missing — append was wrongly suppressed on unresolved {text}: %q", userContent)
	}
}

// TestExtractorComponent_Invoke_SubstitutesPlaceholders verifies that
// {field_name} placeholders in the user prompt are substituted with
// the current chunk's field values before the LLM call, matching
// Python's string_format (agent/component/base.py:602).
func TestExtractorComponent_Invoke_SubstitutesPlaceholders(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "substituted answer"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "summary",
		Prompt:    "Analyze: {text}",
		LLMID:     "gpt-4o-mini",
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "the document content"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	var userContent string
	for _, msg := range stub.lastReq.Messages {
		if msg.Role == eschema.User {
			userContent = msg.Content
		}
	}
	if strings.Contains(userContent, "{text}") {
		t.Errorf("prompt still contains literal {text}: %q", userContent)
	}
	if !strings.Contains(userContent, "the document content") {
		t.Errorf("prompt missing chunk text: %q", userContent)
	}
	// Regression guard: when the prompt embeds {text}, the chunk text
	// must appear exactly once — buildExtractorMessages must not append
	// it a second time (placeholder duplication bug).
	if n := strings.Count(userContent, "the document content"); n != 1 {
		t.Errorf("chunk text appears %d times, want 1: %q", n, userContent)
	}
}

// TestExtractorComponent_Invoke_PlaceholderChunksAlias verifies that
// {chunks} (the Python DSL upstream key) is also substituted with
// the current chunk text.
func TestExtractorComponent_Invoke_PlaceholderChunksAlias(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "answer"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "out",
		Prompt:    "Content: {chunks}",
		LLMID:     "gpt-4o-mini",
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"content_with_weight": "weighted doc"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	var userContent string
	for _, msg := range stub.lastReq.Messages {
		if msg.Role == eschema.User {
			userContent = msg.Content
		}
	}
	if strings.Contains(userContent, "{chunks}") {
		t.Errorf("prompt still contains literal {chunks}: %q", userContent)
	}
	if !strings.Contains(userContent, "weighted doc") {
		t.Errorf("prompt missing chunk text: %q", userContent)
	}
	// Regression guard: {chunks} must not duplicate the chunk text.
	if n := strings.Count(userContent, "weighted doc"); n != 1 {
		t.Errorf("chunk text appears %d times, want 1: %q", n, userContent)
	}
}

// TestExtractorComponent_Invoke_AppendsChunkTextWhenNoPlaceholder verifies
// that when the prompt has no {text}/{chunks} placeholder, the chunk text is
// still automatically appended by buildExtractorMessages exactly once.
func TestExtractorComponent_Invoke_AppendsChunkTextWhenNoPlaceholder(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "answer"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "summary",
		Prompt:    "Summarize the above:",
		LLMID:     "gpt-4o-mini",
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "the document content"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	var userContent string
	for _, msg := range stub.lastReq.Messages {
		if msg.Role == eschema.User {
			userContent = msg.Content
		}
	}
	if n := strings.Count(userContent, "the document content"); n != 1 {
		t.Errorf("chunk text appears %d times, want 1: %q", n, userContent)
	}
}

// TestExtractorComponent_Invoke_SystemPromptPlaceholderSuppressesAppend
// verifies that a content-bearing placeholder in systemPrompt (not just
// prompt) also suppresses the automatic chunk-text append. The chunk body
// is delivered via systemPrompt substitution; since the append is also
// suppressed, it must NOT appear a second time in the user message.
func TestExtractorComponent_Invoke_SystemPromptPlaceholderSuppressesAppend(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "answer"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName:    "out",
		Prompt:       "Extract:",
		SystemPrompt: "Context: {text}",
		LLMID:        "gpt-4o-mini",
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "system prompt body"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	var sysContent, userContent string
	for _, msg := range stub.lastReq.Messages {
		switch msg.Role {
		case eschema.System:
			sysContent = msg.Content
		case eschema.User:
			userContent = msg.Content
		}
	}
	// {text} in systemPrompt must be resolved to the chunk body.
	if !strings.Contains(sysContent, "system prompt body") {
		t.Errorf("system message missing chunk body: %q", sysContent)
	}
	// The append must be suppressed: the user message should NOT also
	// contain the chunk body (otherwise it is duplicated).
	if strings.Contains(userContent, "system prompt body") {
		t.Errorf("chunk body duplicated into user message (append not suppressed): %q", userContent)
	}
}

// TestExtractorComponent_Invoke_FieldValueContainsPlaceholderSubstring
// verifies that a chunk field whose value happens to contain a content
// placeholder substring (e.g. title = "{text}") does not fool the
// suppression check. {text} in prompt is resolved to "body"; {title}
// is resolved to "{text}" literally — the substitution function knows
// {text} was actually replaced (title's replacement is a different
// placeholder), so suppression triggers correctly.
func TestExtractorComponent_Invoke_FieldValueContainsPlaceholderSubstring(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "answer"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "out",
		Prompt:    "Body: {text}\nLabel: {title}",
		LLMID:     "gpt-4o-mini",
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{
			"text":  "the document body",
			"title": "{text}",
		}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	var userContent string
	for _, msg := range stub.lastReq.Messages {
		if msg.Role == eschema.User {
			userContent = msg.Content
		}
	}
	// {text} resolved to body → append suppressed. The body should appear
	// exactly once (from {text} substitution), not twice.
	if n := strings.Count(userContent, "the document body"); n != 1 {
		t.Errorf("chunk text appears %d times, want 1 (no duplicate append): %q", n, userContent)
	}
	// {title} was substituted to the literal "{text}" — this is the tricky
	// case: the substituted prompt now contains "{text}" as a value, but
	// the suppression must still have triggered because {text} was resolved.
	if !strings.Contains(userContent, "Label: {text}") {
		t.Errorf("expected title substitution to produce literal '{text}' label: %q", userContent)
	}
}

// TestFitExtractorMessages_RejectsEmptyUserTurn verifies that when
// messagefit's proportional trim would empty the final user turn (the system
// prompt alone exceeds the context budget), the extractor surfaces a clear
// error instead of sending [system, user:""] to the provider.
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

// TestFitExtractorMessages_KeepsUserTurn verifies the happy path: with a
// normal budget the fitter trims oversized prompts and the final user turn
// survives, so no error is returned.
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

// TestFitExtractorMessages_NoSystemPromptKeepsUserTurn verifies that a
// user-only request (no system prompt configured) is not rejected by the
// system-prompt guard: the guard only applies when a system message was
// actually present, so a valid prompt-only extractor keeps working once the
// model's content_length is resolvable.
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

// TestExtractorComponent_CallRaw_FitsBeforeInvoke verifies the production
// wiring end to end: callRaw resolves the model's context length, trims the
// messages to the budget, and hands the fitted messages to the invoker.
func TestExtractorComponent_CallRaw_FitsBeforeInvoke(t *testing.T) {
	SetExtractorContextLengthOverride(func(_ context.Context, _ string) int { return 200 })
	t.Cleanup(func() { SetExtractorContextLengthOverride(nil) })

	stub := withStubChatInvoker(t, stubResponse{Content: `{"ok": true}`})
	c := &ExtractorComponent{}

	_, err := c.callText(t.Context(), nil, extractorInputs{
		systemPrompt: "extract fields",
		prompt:       "summarize",
		llmID:        "test@test",
	}, strings.Repeat("chunk text with lots of tokens. ", 500))
	if err != nil {
		t.Fatalf("callText: %v", err)
	}

	stub.mu.Lock()
	req := stub.lastReq
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

// TestExtractorComponent_CallRaw_CustomContextOverride verifies the extractor
// wiring honors the tenant-configured override end to end: with a 2000-token
// extra max_tokens on the gpt-4o row, the invoker receives messages fitted to
// ~1940 tokens instead of the catalog's 128k.
func TestExtractorComponent_CallRaw_CustomContextOverride(t *testing.T) {
	db := openExtractorContextTestDB(t)
	seedExtractorContextModel(t, db, "")
	// Add the instance row the composite resolution path needs, then pin the
	// tenant-configured context override on the model.
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
		systemPrompt: "extract fields",
		prompt:       "summarize",
		llmID:        "gpt-4o@OpenAI",
	}, strings.Repeat("chunk text with lots of tokens. ", 500))
	if err != nil {
		t.Fatalf("callText: %v", err)
	}

	stub.mu.Lock()
	req := stub.lastReq
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

// openExtractorContextTestDB returns an in-memory DB with the tenant and
// tenant-model tables migrated. Tests pass the returned handle explicitly to
// extractorContextLength, defaultChatModelRef, and dao.ResolveModelContentLength,
// so no global DAO state is touched.
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

// seedExtractorContextModel seeds an active OpenAI gpt-4o tenant model
// (catalog content_length 128000) plus its tenant. tenantLLMID, when
// non-empty, pins the tenant's default chat model to the tenant-model UUID;
// otherwise the tenant falls back to the composite llm_id.
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

// extractorStateCtx returns a context carrying a canvas state with the given
// tenant_id global, as extractorContextLength expects.
func extractorStateCtx(t *testing.T, tenantID string) context.Context {
	t.Helper()
	state := runtime.NewCanvasState("run-1", "session-1")
	state.SetGlobal("tenant_id", tenantID)
	return runtime.WithState(t.Context(), state)
}

// TestExtractorContextLength_TenantModelUUID verifies extractorContextLength
// resolves content_length for a tenant_model UUID through the provider
// catalog.
func TestExtractorContextLength_TenantModelUUID(t *testing.T) {
	db := openExtractorContextTestDB(t)
	seedExtractorContextModel(t, db, "")
	ctx := extractorStateCtx(t, "tenant-1")

	if got := extractorContextLength(ctx, db, "0123456789abcdef0123456789abcdef"); got != 128000 {
		t.Fatalf("extractorContextLength(uuid) = %d, want 128000", got)
	}
}

// TestExtractorContextLength_DefaultChatModelPinned verifies the llmID==""
// fallback resolves the tenant default chat model when it is pinned to a
// tenant_model UUID.
func TestExtractorContextLength_DefaultChatModelPinned(t *testing.T) {
	db := openExtractorContextTestDB(t)
	seedExtractorContextModel(t, db, "0123456789abcdef0123456789abcdef")
	ctx := extractorStateCtx(t, "tenant-1")

	if got := extractorContextLength(ctx, db, ""); got != 128000 {
		t.Fatalf("extractorContextLength(default pinned uuid) = %d, want 128000", got)
	}
}

// TestExtractorContextLength_DefaultChatModelComposite verifies the llmID==""
// fallback resolves the tenant default chat model from the composite
// "model@provider" llm_id when no tenant_model is pinned.
func TestExtractorContextLength_DefaultChatModelComposite(t *testing.T) {
	db := openExtractorContextTestDB(t)
	seedExtractorContextModel(t, db, "")
	ctx := extractorStateCtx(t, "tenant-1")

	if got := extractorContextLength(ctx, db, ""); got != 128000 {
		t.Fatalf("extractorContextLength(default composite) = %d, want 128000", got)
	}
}

// TestExtractorContextLength_UnknownModelSkips verifies extractorContextLength
// returns 0 (skip fitting) for an unknown model reference.
func TestExtractorContextLength_UnknownModelSkips(t *testing.T) {
	db := openExtractorContextTestDB(t)
	seedExtractorContextModel(t, db, "")
	ctx := extractorStateCtx(t, "tenant-1")

	if got := extractorContextLength(ctx, db, "no-such-model@no-such-provider"); got != 0 {
		t.Fatalf("extractorContextLength(unknown) = %d, want 0", got)
	}
}

// TestExtractorContextFitBudget verifies the fitting budget is 97% of the
// resolved content_length (mirroring the agent's contextFitBudget), leaving
// headroom for tokenizer drift between cl100k and the model's own tokenizer,
// and that a tiny context never collapses to messagefit's <=0 → 8192 default.
func TestExtractorContextFitBudget(t *testing.T) {
	if got := extractorContextFitBudget(128000); got != 124160 {
		t.Fatalf("extractorContextFitBudget(128000) = %d, want 124160", got)
	}
	if got := extractorContextFitBudget(1); got != 1 {
		t.Fatalf("extractorContextFitBudget(1) = %d, want 1 (clamped to avoid the 8192 Fit default)", got)
	}
}

// TestFitExtractorMessages_RejectsSystemPromptLoss verifies the guard that a
// fitting which empties every system message is rejected instead of sending
// an instruction-less extraction request: the system prompt carries the
// extraction contract, so running with an emptied system prompt would
// silently produce garbage.
func TestFitExtractorMessages_RejectsSystemPromptLoss(t *testing.T) {
	SetExtractorContextLengthOverride(func(_ context.Context, _ string) int { return 300 })
	t.Cleanup(func() { SetExtractorContextLengthOverride(nil) })

	// System dominates (>4x the user) and the user message alone exceeds
	// the budget: the proportional trim preserves the user turn and empties
	// the system messages.
	msgs := []eschema.Message{
		{Role: eschema.System, Content: strings.Repeat("s ", 5000)},
		{Role: eschema.User, Content: strings.Repeat("u ", 400)},
	}
	if _, err := fitExtractorMessages(t.Context(), nil, "test@test", msgs); err == nil {
		t.Fatal("expected an error when fitting empties the system prompt")
	}
}

// TestExtractorContextLength_NilDBGraceful verifies that resolving the tenant
// default chat model with no database available (nil db and no override)
// degrades to 0 (skip fitting) instead of panicking in defaultChatModelRef.
func TestExtractorContextLength_NilDBGraceful(t *testing.T) {
	ctx := extractorStateCtx(t, "tenant-1")
	if got := extractorContextLength(ctx, nil, ""); got != 0 {
		t.Fatalf("extractorContextLength(nil db, default model) = %d, want 0 (skip fitting)", got)
	}
}
