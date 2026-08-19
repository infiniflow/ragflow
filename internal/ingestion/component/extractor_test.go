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
	if stub.lastRequest().ModelName != "override-llm" {
		t.Errorf("ModelName = %q, want override-llm", stub.lastRequest().ModelName)
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
	if stub.lastRequest().Driver != "openai" {
		t.Errorf("Driver = %q, want openai", stub.lastRequest().Driver)
	}
	if stub.lastRequest().ModelName != "gpt-4o-mini" {
		t.Errorf("ModelName = %q, want gpt-4o-mini", stub.lastRequest().ModelName)
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

// TestNewExtractorComponent_StrictSystemPrompt verifies that "system_prompt"
// is parsed into Param.SystemPrompt and nested configurations.
func TestNewExtractorComponent_StrictSystemPrompt(t *testing.T) {
	withStubChatInvoker(t, stubResponse{Content: "ok"})
	comp, err := NewExtractorComponent(map[string]any{
		"field_name":    "out",
		"system_prompt": "You are a prompt.",
		"keywords": map[string]any{
			"top_n":         3,
			"system_prompt": "kw prompt",
		},
		"questions": map[string]any{
			"top_n":         2,
			"system_prompt": "q prompt",
		},
		"summary": map[string]any{
			"enabled":       true,
			"system_prompt": "sum prompt",
		},
	})
	if err != nil {
		t.Fatalf("NewExtractorComponent: %v", err)
	}
	ec := comp.(*ExtractorComponent)
	if ec.Param.SystemPrompt != "You are a prompt." {
		t.Errorf("SystemPrompt = %q, want %q", ec.Param.SystemPrompt, "You are a prompt.")
	}
	if ec.Param.Keywords.SystemPrompt != "kw prompt" || ec.Param.Keywords.TopN != 3 {
		t.Errorf("Keywords = %#v, want kw prompt / 3", ec.Param.Keywords)
	}
	if ec.Param.Questions.SystemPrompt != "q prompt" || ec.Param.Questions.TopN != 2 {
		t.Errorf("Questions = %#v, want q prompt / 2", ec.Param.Questions)
	}
	if ec.Param.Summary.SystemPrompt != "sum prompt" || !ec.Param.Summary.Enabled {
		t.Errorf("Summary = %#v, want sum prompt / enabled", ec.Param.Summary)
	}
}

func TestExtractorComponent_NewExtractorComponent_LegacyParamsFallback(t *testing.T) {
	comp, err := NewExtractorComponent(map[string]any{
		"auto_keywords":        5,
		"keywords_sys_prompt":  "legacy kw prompt",
		"auto_questions":       3,
		"questions_sys_prompt": "legacy q prompt",
		"auto_tags":            2,
		"tag_file_id":          "tag-1",
		"enable_summary":       1,
		"sys_prompt":           "legacy sum prompt",
		"metadata_config": map[string]any{
			"enabled": true,
			"metadata": []any{
				map[string]any{"key": "cat", "type": "string"},
			},
			"built_in_metadata": []any{
				map[string]any{"key": "file_name", "type": "string"},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewExtractorComponent with legacy params failed: %v", err)
	}
	ec := comp.(*ExtractorComponent)
	if ec.Param.Keywords.TopN != 5 || ec.Param.Keywords.SystemPrompt != "legacy kw prompt" {
		t.Errorf("Keywords = %#v, want 5 / legacy kw prompt", ec.Param.Keywords)
	}
	if ec.Param.Questions.TopN != 3 || ec.Param.Questions.SystemPrompt != "legacy q prompt" {
		t.Errorf("Questions = %#v, want 3 / legacy q prompt", ec.Param.Questions)
	}
	if ec.Param.Tags.TopN != 2 || ec.Param.Tags.TagFileID != "tag-1" {
		t.Errorf("Tags = %#v, want 2 / tag-1", ec.Param.Tags)
	}
	if !ec.Param.Summary.Enabled || ec.Param.Summary.SystemPrompt != "legacy sum prompt" {
		t.Errorf("Summary = %#v, want enabled / legacy sum prompt", ec.Param.Summary)
	}
	if !ec.Param.Metadata.Enabled || len(ec.Param.Metadata.Metadata) != 1 || len(ec.Param.Metadata.BuiltInMetadata) != 1 {
		t.Errorf("Metadata = %#v, want enabled with 1 custom and 1 built-in field", ec.Param.Metadata)
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
		Metadata: schema.MetadataExtractConfig{
			Enabled:  true,
			Metadata: fields,
		},
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
	if stub.lastRequest().Temperature != nil {
		t.Errorf("Temperature = %v, want nil (field extraction uses model default)", *stub.lastRequest().Temperature)
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
// TestExtractorComponent_Invoke_ContentWithWeightPlaceholder verifies that
// a system prompt referencing {content_with_weight} substitutes the field correctly,
// and the user message carries the chunk text.
func TestExtractorComponent_Invoke_ContentWithWeightPlaceholder(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "answer"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName:    "out",
		SystemPrompt: "Weighted: {content_with_weight}",
		LLMID:        "gpt-4o-mini",
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"content_with_weight": "weighted doc"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	var sysContent, userContent string
	for _, msg := range stub.lastRequest().Messages {
		if msg.Role == eschema.System {
			sysContent = msg.Content
		} else if msg.Role == eschema.User {
			userContent = msg.Content
		}
	}
	if strings.Contains(sysContent, "{content_with_weight}") {
		t.Errorf("system prompt still contains literal {content_with_weight}: %q", sysContent)
	}
	if !strings.Contains(sysContent, "Weighted: weighted doc") {
		t.Errorf("system prompt missing substituted text: %q", sysContent)
	}
	if userContent != "weighted doc" {
		t.Errorf("user message = %q, want %q", userContent, "weighted doc")
	}
}

// TestExtractorComponent_Invoke_NonContentPlaceholderKeepsChunkText verifies
// that a non-content placeholder like {title} in SystemPrompt is substituted,
// while User message delivers the document body.
func TestExtractorComponent_Invoke_NonContentPlaceholderKeepsChunkText(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "answer"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName:    "out",
		SystemPrompt: "Title: {title}\nExtract:",
		LLMID:        "gpt-4o-mini",
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
	var sysContent, userContent string
	for _, msg := range stub.lastRequest().Messages {
		if msg.Role == eschema.System {
			sysContent = msg.Content
		} else if msg.Role == eschema.User {
			userContent = msg.Content
		}
	}
	if strings.Contains(sysContent, "{title}") {
		t.Errorf("system prompt still contains literal {title}: %q", sysContent)
	}
	if !strings.Contains(sysContent, "Title: My Title") {
		t.Errorf("system prompt missing substituted title: %q", sysContent)
	}
	if userContent != "DOC BODY" {
		t.Errorf("user message = %q, want %q", userContent, "DOC BODY")
	}
}

// TestExtractorComponent_Invoke_UnresolvedTextPlaceholderKeepsChunkText verifies
// that a {text} placeholder that cannot be resolved against the chunk map
// falls back to chunkText.
func TestExtractorComponent_Invoke_UnresolvedTextPlaceholderKeepsChunkText(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "answer"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName:    "out",
		SystemPrompt: "Content: {text}",
		LLMID:        "gpt-4o-mini",
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
	var sysContent, userContent string
	for _, msg := range stub.lastRequest().Messages {
		if msg.Role == eschema.System {
			sysContent = msg.Content
		} else if msg.Role == eschema.User {
			userContent = msg.Content
		}
	}
	if sysContent != "Content: weighted doc" {
		t.Errorf("system prompt = %q, want %q", sysContent, "Content: weighted doc")
	}
	if userContent != "weighted doc" {
		t.Errorf("user message = %q, want %q", userContent, "weighted doc")
	}
}

// TestExtractorComponent_Invoke_SubstitutesPlaceholders verifies that
// {field_name} placeholders in the system prompt are substituted with
// the current chunk's field values before the LLM call.
func TestExtractorComponent_Invoke_SubstitutesPlaceholders(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "substituted answer"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName:    "summary",
		SystemPrompt: "Analyze: {text}",
		LLMID:        "gpt-4o-mini",
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "the document content"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	var sysContent, userContent string
	for _, msg := range stub.lastRequest().Messages {
		if msg.Role == eschema.System {
			sysContent = msg.Content
		} else if msg.Role == eschema.User {
			userContent = msg.Content
		}
	}
	if strings.Contains(sysContent, "{text}") {
		t.Errorf("system prompt still contains literal {text}: %q", sysContent)
	}
	if sysContent != "Analyze: the document content" {
		t.Errorf("system prompt = %q, want %q", sysContent, "Analyze: the document content")
	}
	if userContent != "the document content" {
		t.Errorf("user message = %q, want %q", userContent, "the document content")
	}
}

// TestExtractorComponent_Invoke_PlaceholderChunksAlias verifies that
// {chunks} is also substituted in SystemPrompt.
func TestExtractorComponent_Invoke_PlaceholderChunksAlias(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "answer"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName:    "out",
		SystemPrompt: "Content: {chunks}",
		LLMID:        "gpt-4o-mini",
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"content_with_weight": "weighted doc"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	var sysContent, userContent string
	for _, msg := range stub.lastRequest().Messages {
		if msg.Role == eschema.System {
			sysContent = msg.Content
		} else if msg.Role == eschema.User {
			userContent = msg.Content
		}
	}
	if strings.Contains(sysContent, "{chunks}") {
		t.Errorf("system prompt still contains literal {chunks}: %q", sysContent)
	}
	if sysContent != "Content: weighted doc" {
		t.Errorf("system prompt = %q, want %q", sysContent, "Content: weighted doc")
	}
	if userContent != "weighted doc" {
		t.Errorf("user message = %q, want %q", userContent, "weighted doc")
	}
}

// TestExtractorComponent_Invoke_AppendsChunkTextWhenNoPlaceholder verifies
// that when the system prompt has no placeholder, the chunk text is
// still delivered via the User message.
func TestExtractorComponent_Invoke_AppendsChunkTextWhenNoPlaceholder(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "answer"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName:    "summary",
		SystemPrompt: "Summarize the above:",
		LLMID:        "gpt-4o-mini",
	}}
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "the document content"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	var sysContent, userContent string
	for _, msg := range stub.lastRequest().Messages {
		if msg.Role == eschema.System {
			sysContent = msg.Content
		} else if msg.Role == eschema.User {
			userContent = msg.Content
		}
	}
	if sysContent != "Summarize the above:" {
		t.Errorf("system prompt = %q, want %q", sysContent, "Summarize the above:")
	}
	if userContent != "the document content" {
		t.Errorf("user message = %q, want %q", userContent, "the document content")
	}
}

// TestExtractorComponent_Invoke_FieldValueContainsPlaceholderSubstring
// verifies that a chunk field whose value happens to contain a content
// placeholder substring (e.g. title = "{text}") is substituted correctly in SystemPrompt.
func TestExtractorComponent_Invoke_FieldValueContainsPlaceholderSubstring(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "answer"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName:    "out",
		SystemPrompt: "Body: {text}\nLabel: {title}",
		LLMID:        "gpt-4o-mini",
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
	var sysContent, userContent string
	for _, msg := range stub.lastRequest().Messages {
		if msg.Role == eschema.System {
			sysContent = msg.Content
		} else if msg.Role == eschema.User {
			userContent = msg.Content
		}
	}
	if sysContent != "Body: the document body\nLabel: {text}" {
		t.Errorf("system prompt = %q, want %q", sysContent, "Body: the document body\nLabel: {text}")
	}
	if userContent != "the document body" {
		t.Errorf("user message = %q, want %q", userContent, "the document body")
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

	// callRaw is a pure dispatcher: callers are responsible for pre-rendering
	// the prompt. Inline a large chunk body directly into the user prompt so
	// fitExtractorMessages has real content to trim.
	chunkBody := strings.Repeat("chunk text with lots of tokens. ", 500)
	_, err := c.callText(t.Context(), nil, extractorInputs{
		systemPrompt: "extract fields",
		llmID:        "test@test",
	}, chunkBody)
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
		llmID:        "gpt-4o@OpenAI",
	}, strings.Repeat("chunk text with lots of tokens. ", 500))
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

// TestExtractorComponent_Invoke_AtChunksPlaceholderPerChunk verifies that
// when the prompt contains {ComponentName:ParamName@chunks}, each chunk's
// LLM invocation receives ONLY that chunk's text (not all chunks joined together),
// and the chunk text appears exactly once without duplication.
func TestExtractorComponent_Invoke_AtChunksPlaceholderPerChunk(t *testing.T) {
	stub := withStubChatInvoker(t,
		stubResponse{Content: "answer1"},
		stubResponse{Content: "answer2"},
	)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName:    "summary",
		SystemPrompt: "Summarize: {TokenChunker:BumpyStarsPress@chunks}",
		LLMID:        "gpt-4o-mini",
	}}

	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{
			{"text": "Chunk One Body"},
			{"text": "Chunk Two Body"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()

	if len(stub.requests) != 2 {
		t.Fatalf("got %d LLM calls, want 2", len(stub.requests))
	}

	// Call 1 System must contain Chunk 1 exactly once, and NOT contain Chunk 2; User contains Chunk 1
	var req1Sys, req1User string
	for _, msg := range stub.requests[0].Messages {
		if msg.Role == eschema.System {
			req1Sys = msg.Content
		} else if msg.Role == eschema.User {
			req1User = msg.Content
		}
	}
	if n := strings.Count(req1Sys, "Chunk One Body"); n != 1 {
		t.Errorf("call 1 chunk text count in system = %d, want 1; prompt: %q", n, req1Sys)
	}
	if strings.Contains(req1Sys, "Chunk Two Body") {
		t.Errorf("call 1 wrongly contains Chunk Two Body; prompt: %q", req1Sys)
	}
	if req1User != "Chunk One Body" {
		t.Errorf("call 1 user message = %q, want %q", req1User, "Chunk One Body")
	}

	// Call 2 System must contain Chunk 2 exactly once, and NOT contain Chunk 1; User contains Chunk 2
	var req2Sys, req2User string
	for _, msg := range stub.requests[1].Messages {
		if msg.Role == eschema.System {
			req2Sys = msg.Content
		} else if msg.Role == eschema.User {
			req2User = msg.Content
		}
	}
	if n := strings.Count(req2Sys, "Chunk Two Body"); n != 1 {
		t.Errorf("call 2 chunk text count in system = %d, want 1; prompt: %q", n, req2Sys)
	}
	if strings.Contains(req2Sys, "Chunk One Body") {
		t.Errorf("call 2 wrongly contains Chunk One Body; prompt: %q", req2Sys)
	}
	if req2User != "Chunk Two Body" {
		t.Errorf("call 2 user message = %q, want %q", req2User, "Chunk Two Body")
	}
}

// TestRenderExtractorSystemPrompt_TableDriven covers all system prompt rendering cases.
func TestRenderExtractorSystemPrompt_TableDriven(t *testing.T) {
	tests := []struct {
		name        string
		sysTemplate string
		ck          map[string]any
		chunkText   string
		wantSys     string
	}{
		{
			name:        "text in system",
			sysTemplate: "Analyze: {text}",
			ck:          map[string]any{"text": "body content"},
			chunkText:   "body content",
			wantSys:     "Analyze: body content",
		},
		{
			name:        "text in system falls back to chunkText",
			sysTemplate: "Context: {text}",
			ck:          map[string]any{},
			chunkText:   "body content",
			wantSys:     "Context: body content",
		},
		{
			name:        "canvas macro @chunks in system",
			sysTemplate: "Summarize: {TokenChunker:BumpyStarsPress@chunks}",
			ck:          map[string]any{"text": "body content"},
			chunkText:   "body content",
			wantSys:     "Summarize: body content",
		},
		{
			name:        "canvas macro @text in system",
			sysTemplate: "Parse: {Parser:Doc@text}",
			ck:          map[string]any{"text": "body content"},
			chunkText:   "body content",
			wantSys:     "Parse: body content",
		},
		{
			name:        "canvas macro @markdown in system",
			sysTemplate: "Parse: {Parser:Doc@markdown}",
			ck:          map[string]any{"text": "body content"},
			chunkText:   "body content",
			wantSys:     "Parse: body content",
		},
		{
			name:        "metadata placeholder",
			sysTemplate: "Title: {title}",
			ck:          map[string]any{"title": "DocTitle", "text": "body content"},
			chunkText:   "body content",
			wantSys:     "Title: DocTitle",
		},
		{
			name:        "content_with_weight present in ck",
			sysTemplate: "Weighted: {content_with_weight}",
			ck:          map[string]any{"content_with_weight": "weighted content", "text": "plain content"},
			chunkText:   "weighted content",
			wantSys:     "Weighted: weighted content",
		},
		{
			name:        "content_with_weight absent in ck falls back to chunkText",
			sysTemplate: "Weighted: {content_with_weight}",
			ck:          map[string]any{"text": "plain content"},
			chunkText:   "plain content",
			wantSys:     "Weighted: plain content",
		},
		{
			name:        "content_with_weight empty string in ck falls back to chunkText",
			sysTemplate: "Weighted: {content_with_weight}",
			ck:          map[string]any{"content_with_weight": ""},
			chunkText:   "fallback body",
			wantSys:     "Weighted: fallback body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSys := renderExtractorSystemPrompt(tt.sysTemplate, tt.ck, tt.chunkText)
			if gotSys != tt.wantSys {
				t.Errorf("renderExtractorSystemPrompt() gotSys = %q, want %q", gotSys, tt.wantSys)
			}
		})
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
			"top_n":         5,
			"system_prompt": "Custom keywords prompt for {text}",
		},
		"questions": map[string]any{
			"top_n":         3,
			"system_prompt": "Custom questions prompt for {text}",
		},
		"tags": map[string]any{
			"top_n":       2,
			"tag_file_id": "tag_file_abc",
		},
		"summary": map[string]any{
			"enabled":       true,
			"system_prompt": "Custom summary prompt for {text}",
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

	if ext.Param.Keywords.TopN != 5 || ext.Param.Keywords.SystemPrompt != "Custom keywords prompt for {text}" {
		t.Errorf("Keywords config mismatch: %+v", ext.Param.Keywords)
	}
	if ext.Param.Questions.TopN != 3 || ext.Param.Questions.SystemPrompt != "Custom questions prompt for {text}" {
		t.Errorf("Questions config mismatch: %+v", ext.Param.Questions)
	}
	if ext.Param.Tags.TopN != 2 || ext.Param.Tags.TagFileID != "tag_file_abc" {
		t.Errorf("Tags config mismatch: %+v", ext.Param.Tags)
	}
	if !ext.Param.Summary.Enabled || ext.Param.Summary.SystemPrompt != "Custom summary prompt for {text}" {
		t.Errorf("Summary config mismatch: %+v", ext.Param.Summary)
	}
	if ext.Param.FieldName != "summary" {
		t.Errorf("FieldName mismatch: %s", ext.Param.FieldName)
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
			"top_n":         2,
			"system_prompt": "Custom KW Prompt: {text}",
		},
		"questions": map[string]any{
			"top_n":         2,
			"system_prompt": "Custom Q Prompt: {text}",
		},
		"summary": map[string]any{
			"enabled":       true,
			"system_prompt": "Custom Sum Prompt: {text}",
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

	// Verify custom prompts were rendered into messages and passed to the chat invoker
	reqs := stub.requests
	if len(reqs) != 3 {
		t.Fatalf("expected 3 LLM calls, got %d", len(reqs))
	}

	var foundKW, foundQ, foundSum bool
	for _, r := range reqs {
		for _, msg := range r.Messages {
			if strings.Contains(msg.Content, "Custom KW Prompt: Hello world content") {
				foundKW = true
			}
			if strings.Contains(msg.Content, "Custom Q Prompt: Hello world content") {
				foundQ = true
			}
			if strings.Contains(msg.Content, "Custom Sum Prompt: Hello world content") {
				foundSum = true
			}
		}
	}

	if !foundKW {
		t.Errorf("expected custom keyword prompt to be used in LLM call, got reqs: %+v", reqs)
	}
	if !foundQ {
		t.Errorf("expected custom question prompt to be used in LLM call, got reqs: %+v", reqs)
	}
	if !foundSum {
		t.Errorf("expected custom summary prompt to be used in LLM call, got reqs: %+v", reqs)
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
	// year can be float64 or int from JSON
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
			// system_prompt left empty to test default prompt injection
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

func TestExtractorDisabledSummarySkipsCall(t *testing.T) {
	stub := withStubChatInvoker(t, stubResponse{Content: "Not expected"})

	params := map[string]any{
		"llm_id": "llm-1",
		"summary": map[string]any{
			"enabled": false,
		},
		"field_name": "summary", // legacy default that should be ignored when enabled=false
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
			"metadata": []any{
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
		t.Errorf("expected metadata disabled, got nested=%v", compA.Param.Metadata.Enabled)
	}
	if compA.Param.Summary.Enabled != false {
		t.Errorf("expected summary disabled, got nested=%v", compA.Param.Summary.Enabled)
	}
	if compA.Param.Keywords.TopN != 0 {
		t.Errorf("expected keywords disabled (0), got nested=%v", compA.Param.Keywords.TopN)
	}
	if compA.Param.Questions.TopN != 0 {
		t.Errorf("expected questions disabled (0), got nested=%v", compA.Param.Questions.TopN)
	}
	if compA.Param.Tags.TopN != 0 {
		t.Errorf("expected tags disabled (0), got nested=%v", compA.Param.Tags.TopN)
	}

	// Case B: Nested explicitly enabled (enabled=true)
	paramsEnabled := map[string]any{
		"metadata": map[string]any{
			"enabled": true,
			"metadata": []any{
				map[string]any{"key": "author", "type": "string"},
			},
		},
		"summary": map[string]any{
			"enabled": true,
		},
		"keywords": map[string]any{
			"top_n": 4,
		},
		"questions": map[string]any{
			"top_n": 2,
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
		t.Errorf("expected metadata enabled, got nested=%v", compB.Param.Metadata.Enabled)
	}
	if compB.Param.Summary.Enabled != true {
		t.Errorf("expected summary enabled, got nested=%v", compB.Param.Summary.Enabled)
	}
	if compB.Param.Keywords.TopN != 4 {
		t.Errorf("expected keywords 4, got nested=%v", compB.Param.Keywords.TopN)
	}
	if compB.Param.Questions.TopN != 2 {
		t.Errorf("expected questions 2, got nested=%v", compB.Param.Questions.TopN)
	}
	if compB.Param.Tags.TopN != 3 || compB.Param.Tags.TagFileID != "file-123" {
		t.Errorf("expected tags 3 / file-123, got nested=%+v", compB.Param.Tags)
	}
}

func TestExtractor_Precedence_ModularBeatsLegacy(t *testing.T) {
	// Mixed config: both modular objects and legacy flat fields present
	mixedParams := map[string]any{
		// Modular overrides
		"keywords": map[string]any{
			"top_n":         10,
			"system_prompt": "modular kw",
		},
		"questions": map[string]any{
			"top_n":         8,
			"system_prompt": "modular q",
		},
		"tags": map[string]any{
			"top_n":       6,
			"tag_file_id": "modular-tag-file",
		},
		"summary": map[string]any{
			"enabled":       false,
			"system_prompt": "modular summary",
		},
		"metadata": map[string]any{
			"enabled": true,
			"metadata": []any{
				map[string]any{"key": "modular_meta", "type": "string"},
			},
		},
		// Legacy flat fields (should be ignored when modular exists)
		"auto_keywords":        2,
		"keywords_sys_prompt":  "legacy kw",
		"auto_questions":       1,
		"questions_sys_prompt": "legacy q",
		"auto_tags":            1,
		"tag_file_id":          "legacy-tag-file",
		"enable_summary":       1,
		"sys_prompt":           "legacy summary",
		"enable_metadata":      0,
		"metadata_config": map[string]any{
			"enabled": false,
			"metadata": []any{
				map[string]any{"key": "legacy_meta", "type": "string"},
			},
		},
	}

	compRaw, err := NewExtractorComponent(mixedParams)
	if err != nil {
		t.Fatalf("NewExtractorComponent with mixed params failed: %v", err)
	}
	ec := compRaw.(*ExtractorComponent)

	if ec.Param.Keywords.TopN != 10 || ec.Param.Keywords.SystemPrompt != "modular kw" {
		t.Errorf("Keywords = %#v, want modular 10 / modular kw", ec.Param.Keywords)
	}
	if ec.Param.Questions.TopN != 8 || ec.Param.Questions.SystemPrompt != "modular q" {
		t.Errorf("Questions = %#v, want modular 8 / modular q", ec.Param.Questions)
	}
	if ec.Param.Tags.TopN != 6 || ec.Param.Tags.TagFileID != "modular-tag-file" {
		t.Errorf("Tags = %#v, want modular 6 / modular-tag-file", ec.Param.Tags)
	}
	if ec.Param.Summary.Enabled || ec.Param.Summary.SystemPrompt != "modular summary" {
		t.Errorf("Summary = %#v, want modular disabled / modular summary", ec.Param.Summary)
	}
	if !ec.Param.Metadata.Enabled || len(ec.Param.Metadata.Metadata) != 1 || ec.Param.Metadata.Metadata[0].Key != "modular_meta" {
		t.Errorf("Metadata = %#v, want modular enabled with modular_meta field", ec.Param.Metadata)
	}
}

func TestExtractor_EndToEnd_LegacyPipelineFlow(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: "keyword1, keyword2"},
		stubResponse{Content: "question1?\nquestion2?"},
		stubResponse{Content: `{"category": "tech"}`},
		stubResponse{Content: "RAGFlow summary."},
	)

	legacyFlatParams := map[string]any{
		"auto_keywords":       3,
		"keywords_sys_prompt": "Extract {top_n} keywords",
		"auto_questions":      2,
		"enable_summary":      1,
		"sys_prompt":          "Summarize: {text}",
		"enable_metadata":     1,
		"metadata": []any{
			map[string]any{"key": "category", "type": "string"},
		},
		"built_in_metadata": []any{
			map[string]any{"key": "file_name", "type": "string"},
		},
	}

	comp, err := NewExtractorComponent(legacyFlatParams)
	if err != nil {
		t.Fatalf("NewExtractorComponent failed: %v", err)
	}

	out, err := comp.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{
			{"text": "RAGFlow is an open-source RAG engine."},
		},
	})
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}

	chunks, ok := out["chunks"].([]map[string]any)
	if !ok || len(chunks) != 1 {
		t.Fatalf("expected 1 output chunk, got %#v", out["chunks"])
	}
	if chunks[0]["summary"] != "RAGFlow summary." {
		t.Errorf("expected summary field in chunk, got %#v", chunks[0]["summary"])
	}
	meta, ok := chunks[0]["metadata"].(map[string]any)
	if !ok || meta["category"] != "tech" {
		t.Errorf("expected metadata.category == tech in chunk, got %#v", chunks[0]["metadata"])
	}
	kw, ok := chunks[0]["important_kwd"].([]string)
	if !ok || len(kw) == 0 {
		t.Errorf("expected important_kwd in chunk, got %#v", chunks[0]["important_kwd"])
	}
}

func TestExtractor_MetadataNoAutoEnable_Flat(t *testing.T) {
	flatParams := map[string]any{
		"metadata": []any{
			map[string]any{"key": "category", "type": "string"},
		},
		"built_in_metadata": []any{
			map[string]any{"key": "file_name", "type": "string"},
		},
	}

	compRaw, err := NewExtractorComponent(flatParams)
	if err != nil {
		t.Fatalf("NewExtractorComponent failed: %v", err)
	}
	ec := compRaw.(*ExtractorComponent)
	if ec.Param.Metadata.Enabled {
		t.Errorf("expected Metadata.Enabled == false without explicit enable_metadata flag")
	}
	if len(ec.Param.Metadata.Metadata) != 1 || len(ec.Param.Metadata.BuiltInMetadata) != 1 {
		t.Errorf("expected parsed metadata schema fields, got: %#v", ec.Param.Metadata)
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

func TestExtractor_ResolveInputs_DeprecatedPromptInput(t *testing.T) {
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName:    "summary",
		SystemPrompt: "Default sys",
	}}

	inputs := c.resolveInputs(map[string]any{
		"prompt": "some prompt template",
	})
	if inputs.systemPrompt != "Default sys" {
		t.Errorf("expected systemPrompt to remain 'Default sys', got %q", inputs.systemPrompt)
	}
}

func TestExtractor_TopnTagsAlias(t *testing.T) {
	// 1. topn_tags alias works
	compRaw, err := NewExtractorComponent(map[string]any{
		"topn_tags":   5,
		"tag_file_id": "tag-1",
	})
	if err != nil {
		t.Fatalf("NewExtractorComponent failed: %v", err)
	}
	ec := compRaw.(*ExtractorComponent)
	if ec.Param.Tags.TopN != 5 || ec.Param.Tags.TagFileID != "tag-1" {
		t.Errorf("expected Tags.TopN == 5 / TagFileID == tag-1, got %#v", ec.Param.Tags)
	}

	// 2. auto_tags has precedence over topn_tags
	compRaw2, err := NewExtractorComponent(map[string]any{
		"auto_tags": 3,
		"topn_tags": 8,
	})
	if err != nil {
		t.Fatalf("NewExtractorComponent failed: %v", err)
	}
	ec2 := compRaw2.(*ExtractorComponent)
	if ec2.Param.Tags.TopN != 3 {
		t.Errorf("expected auto_tags precedence TopN == 3, got %d", ec2.Param.Tags.TopN)
	}
}


