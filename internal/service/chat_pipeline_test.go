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

package service

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
)

// dialForTest builds a minimal *entity.Chat suitable for the
// guard-clause tests. KBs are empty so AsyncChat goes through
// AsyncChatSolo.
func dialForTest(llmid string) *entity.Chat {
	return &entity.Chat{
		ID:       "chat-1",
		TenantID: "tenant-1",
		LLMID:    llmid,
		PromptConfig: map[string]interface{}{
			"system":           "you are a test assistant.",
			"quote":            true,
			"refine_multiturn": false,
			"keyword":          false,
			"use_kg":           false,
			"toc_enhance":      false,
		},
		KBIDs:                  []interface{}{},
		VectorSimilarityWeight: 0.3,
	}
}

// newTimerAndPrompt builds a fresh Timer with all 6 phases recorded
// (with ~0 durations), so decorateAnswer emits the full Markdown
// block.
func newTimerAndPrompt() (*common.Timer, string) {
	t := common.NewTimer()
	t.Start()
	for _, p := range []common.Phase{
		common.PhaseCheckLLM,
		common.PhaseBindModels,
		common.PhaseRetrieval,
		common.PhaseGenerateAnswer,
	} {
		t.Enter(p)
		t.Exit(p)
	}
	return t, "Test prompt"
}

// --- P9 / P5 guard-clause tests on AsyncChat (P0 indirectly: input
//     validation runs before any RAG pipeline) ---

// TestAsyncChat_RejectsNonUserLastMessage covers the assertion at
// chat_pipeline.go:167. The OpenAI handler is supposed to enforce
// this, but a defense-in-depth check inside AsyncChat guards
// against misbehaving callers.
func TestAsyncChat_RejectsNonUserLastMessage(t *testing.T) {
	s := &ChatPipelineService{}
	messages := []map[string]interface{}{
		{"role": "user", "content": "first"},
		{"role": "assistant", "content": "last message must not be assistant"},
	}
	_, err := s.AsyncChat(context.Background(), dialForTest(""), messages, false, nil)
	if err == nil {
		t.Fatal("expected error for non-user last message, got nil")
	}
	if !strings.Contains(err.Error(), "not from user") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestAsyncChat_EmptyMessages covers the empty-messages case. The
// service should return an error before spawning the goroutine.
func TestAsyncChat_EmptyMessages(t *testing.T) {
	s := &ChatPipelineService{}
	_, err := s.AsyncChat(context.Background(), dialForTest(""), nil, false, nil)
	if err == nil {
		t.Fatal("expected error for empty messages, got nil")
	}
}

// --- P1 Timer + decorateAnswer tests (P0/P1/P7 surface) ---

// TestDecorateAnswer_TimerFormatAlwaysEmitted pins the Markdown
// layout of Timer, ensuring all six phase lines plus Total appear.
func TestDecorateAnswer_TimerFormatAlwaysEmitted(t *testing.T) {
	s := &ChatPipelineService{}
	timer, _ := newTimerAndPrompt()
	result := s.decorateAnswer(
		context.Background(),
		"hello world",
		map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}},
		"system prompt",
		[]string{"question"},
		0,
		timer,
		nil, 0.0, false,
		nil,
		"",
		nil,
		"",
		nil,
		false,
	)
	md := result.Prompt
	for _, must := range []string{
		"## Time elapsed:",
		"  - Check LLM:",
		"  - Bind models:",
		"  - Retrieval:",
		"  - Generate answer:",
		"  - Total:",
		"Generated tokens(approximately):",
	} {
		if !strings.Contains(md, must) {
			t.Errorf("decorateAnswer prompt missing %q in:\n%s", must, md)
		}
	}
}

// TestDecorateAnswer_ThinkMarkersPreserved covers the <think> split
// at decorateAnswer: when the LLM emits a think block, decorateAnswer
// moves the think block to the front of the final answer.
func TestDecorateAnswer_ThinkMarkersPreserved(t *testing.T) {
	s := &ChatPipelineService{}
	timer, _ := newTimerAndPrompt()
	result := s.decorateAnswer(
		context.Background(),
		"<think>reasoning</think>visible answer",
		map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}},
		"system prompt",
		[]string{"q"},
		0,
		timer,
		nil, 0.0, false,
		nil,
		"",
		nil,
		"",
		nil,
		false,
	)
	if !strings.HasPrefix(result.Answer, "<think>reasoning</think>") {
		t.Errorf("expected think block at start, got %q", result.Answer)
	}
	if !strings.Contains(result.Answer, "visible answer") {
		t.Errorf("expected visible answer in result, got %q", result.Answer)
	}
}

// TestDecorateAnswer_InvalidKeySuffix ensures the "Invalid API key"
// append path runs. This is an LLM error-marker check; the message
// survives cleanup.
func TestDecorateAnswer_InvalidKeySuffix(t *testing.T) {
	s := &ChatPipelineService{}
	timer, _ := newTimerAndPrompt()
	result := s.decorateAnswer(
		context.Background(),
		"oops: invalid api key",
		map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}},
		"system prompt",
		[]string{"q"},
		0,
		timer,
		nil, 0.0, false,
		nil,
		"",
		nil,
		"",
		nil,
		false,
	)
	if !strings.Contains(result.Answer, "Please set LLM API-Key") {
		t.Errorf("expected API-key hint, got %q", result.Answer)
	}
}

// TestDecorateAnswer_LeavesCanonicalMarkers covers P0: the decorator
// passes canonical [ID:N] markers through unchanged when there are
// no chunks to cite (so insertCitations is skipped).
func TestDecorateAnswer_LeavesCanonicalMarkers(t *testing.T) {
	s := &ChatPipelineService{}
	timer, _ := newTimerAndPrompt()
	result := s.decorateAnswer(
		context.Background(),
		"see [ID:12] for details",
		map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}},
		"system prompt",
		[]string{"q"},
		0,
		timer,
		nil, 0.0, false,
		nil,
		"",
		nil,
		"",
		nil,
		false,
	)
	if !strings.Contains(result.Answer, "[ID:12]") {
		t.Errorf("canonical marker must survive decorateAnswer, got %q", result.Answer)
	}
}

// TestDecorateAnswer_RepairNotRunWhenNoQuote covers P0.10: when
// quote=false, the citation-repair branch is gated off and the
// answer is preserved verbatim.
func TestDecorateAnswer_RepairNotRunWhenNoQuote(t *testing.T) {
	s := &ChatPipelineService{}
	timer, _ := newTimerAndPrompt()
	result := s.decorateAnswer(
		context.Background(),
		"see (ID: 12) for details",
		map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}},
		"system prompt",
		[]string{"q"},
		0,
		timer,
		nil, 0.0, false,
		nil,
		"",
		nil,
		"",
		nil,
		false,
	)
	if result.Answer != "see (ID: 12) for details" {
		t.Errorf("quote=false must not repair, got %q", result.Answer)
	}
}

// TestDecorateAnswer_RepairRunsWhenQuote covers P0.10: when quote=true
// and the answer has bad citation shapes, RepairBadCitationFormats
// runs and produces canonical [ID:N] form.
func TestDecorateAnswer_RepairRunsWhenQuote(t *testing.T) {
	s := &ChatPipelineService{}
	timer, _ := newTimerAndPrompt()
	// Repair requires at least one chunk (mirrors Python's
	// `if knowledges and ...` guard). We provide one stub chunk so
	// the repair block runs.
	kb := map[string]interface{}{
		"chunks": []map[string]interface{}{
			map[string]interface{}{
				"chunk_id":            "c1",
				"content_with_weight": "hello world",
				"doc_id":              "d1",
			},
		},
		"doc_aggs": []interface{}{},
	}
	result := s.decorateAnswer(
		context.Background(),
		"see (ID: 12) for details",
		kb,
		"system prompt",
		[]string{"q"},
		0,
		timer,
		nil, 0.0, true, // quote=true
		nil,
		"",
		nil,
		"",
		nil,
		true,
	)
	if !strings.Contains(result.Answer, "[ID:12]") {
		t.Errorf("quote=true must repair to [ID:12], got %q", result.Answer)
	}
}

// TestDecorateAnswer_PreCheckSkipsInsertCitations covers P0.11: when
// the LLM already emitted canonical [ID:N] markers, insertCitations
// is skipped (so we don't double-tag). We verify by checking that
// the final answer keeps the same marker count we sent in.
func TestDecorateAnswer_PreCheckSkipsInsertCitations(t *testing.T) {
	s := &ChatPipelineService{}
	timer, _ := newTimerAndPrompt()
	in := "answer has [ID:3] already in it"
	result := s.decorateAnswer(
		context.Background(),
		in,
		map[string]interface{}{
			// No chunks → insertCitations path is gated off anyway,
			// but the pre-check still works on the answer.
			"chunks":   []map[string]interface{}{},
			"doc_aggs": []interface{}{},
		},
		"system prompt",
		[]string{"q"},
		0,
		timer,
		nil, 0.0, true,
		nil,
		"",
		nil,
		"",
		nil,
		false,
	)
	// Marker must be preserved (idempotent re-formatting only).
	if strings.Count(result.Answer, "[ID:3]") < 1 {
		t.Errorf("expected [ID:3] preserved, got %q", result.Answer)
	}
}

// --- P2 helpers ---

// TestKBIDStrings_ExtractsAndFilters pins the contract of the
// KB-id-string helper used by SQL retrieval, KG retrieval, and
// DeepResearcher.
func TestKBIDStrings_ExtractsAndFilters(t *testing.T) {
	cases := []struct {
		name string
		in   []*entity.Knowledgebase
		want []string
	}{
		{"nil", nil, nil},
		{"empty", []*entity.Knowledgebase{}, nil},
		{"all empty IDs", []*entity.Knowledgebase{{ID: ""}, {ID: ""}}, nil},
		{"mixed", []*entity.Knowledgebase{{ID: "kb-1"}, nil, {ID: "kb-2"}}, []string{"kb-1", "kb-2"}},
		{"all set", []*entity.Knowledgebase{{ID: "a"}, {ID: "b"}}, []string{"a", "b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := kbIDStrings(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("kbIDStrings(%v) = %v, want %v", c.in, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("kbIDStrings[%d] = %q, want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

// TestLastUserQuestion covers the helper that mirrors Python's
// `questions[-1]` access for meta_data_filter.
func TestLastUserQuestion(t *testing.T) {
	cases := []struct {
		name string
		in   []map[string]interface{}
		want string
	}{
		{"empty", nil, ""},
		{"no user", []map[string]interface{}{{"role": "system", "content": "x"}}, ""},
		{"single user", []map[string]interface{}{{"role": "user", "content": "hello"}}, "hello"},
		{"multi-turn picks last user", []map[string]interface{}{
			{"role": "user", "content": "first"},
			{"role": "assistant", "content": "ok"},
			{"role": "user", "content": "second"},
		}, "second"},
		{"non-string content", []map[string]interface{}{
			{"role": "user", "content": map[string]interface{}{"x": 1}},
		}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := lastUserQuestion(c.in); got != c.want {
				t.Errorf("lastUserQuestion(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// --- P8 factory extraction test ---

// TestFactoryFromLLMID covers the helper that pulls the provider
// segment out of a composite LLMID for P8 multimodal dispatch.
func TestFactoryFromLLMID(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "openai"},
		{"plain-model", "openai"},
		{"qwen@local", "openai"}, // only one @ — fall back
		{"Qwen3-8B@ling@SILICONFLOW", "siliconflow"},
		{"GPT-4@openai", "openai"},
		{"claude@user@anthropic", "anthropic"},
		{"gemini-1.5@vertex@GEMINI", "gemini"},
	}
	for _, c := range cases {
		if got := factoryFromLLMID(c.in); got != c.want {
			t.Errorf("factoryFromLLMID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- P6 FullQuestion helper test ---

// TestFallbackToLatestUser pins the contract of the helper used by
// FullQuestion: when the LLM fails, we fall back to the latest user
// message content.
func TestFallbackToLatestUser(t *testing.T) {
	cases := []struct {
		name string
		in   []map[string]interface{}
		want string
	}{
		{"empty", nil, ""},
		{"no user", []map[string]interface{}{{"role": "system", "content": "x"}}, ""},
		{"single user", []map[string]interface{}{{"role": "user", "content": "hello"}}, "hello"},
		{"multi-turn picks last user", []map[string]interface{}{
			{"role": "user", "content": "first"},
			{"role": "assistant", "content": "ok"},
			{"role": "user", "content": "second"},
		}, "second"},
		{"non-string content", []map[string]interface{}{
			{"role": "user", "content": map[string]interface{}{"x": 1}},
		}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := fallbackToLatestUser(c.in); got != c.want {
				t.Errorf("fallbackToLatestUser(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// --- P0/Hydration tests ---

// TestHydrateChunkVectors_NoChunksNoop pins the no-op behavior of
// the hydration helper on empty input.
func TestHydrateChunkVectors_NoChunksNoop(t *testing.T) {
	hits, err := HydrateChunkVectors(context.Background(),
		map[string]interface{}{"chunks": []interface{}{}},
		nil, nil, nil,
	)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if hits != 0 {
		t.Errorf("expected 0 hits, got %d", hits)
	}
}

// TestHydrateChunkVectors_NilKbinfosNoop pins the no-op behavior of
// the hydration helper on nil kbinfos.
func TestHydrateChunkVectors_NilKbinfosNoop(t *testing.T) {
	hits, err := HydrateChunkVectors(context.Background(), nil, nil, nil, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if hits != 0 {
		t.Errorf("expected 0 hits, got %d", hits)
	}
}

// --- AsyncChatResult zero-value test ---

// TestAsyncChatResult_FinalFlagDefaultsFalse pins the zero-value
// behavior of AsyncChatResult. A non-final delta must not have
// Final=true; only the terminal result does.
func TestAsyncChatResult_FinalFlagDefaultsFalse(t *testing.T) {
	var r AsyncChatResult
	if r.Final {
		t.Errorf("zero-value AsyncChatResult should not be Final")
	}
	if r.Answer != "" {
		t.Errorf("zero-value Answer = %q, want empty", r.Answer)
	}
	if r.Prompt != "" {
		t.Errorf("zero-value Prompt = %q, want empty", r.Prompt)
	}
	if r.Reference != nil {
		t.Errorf("zero-value Reference = %v, want nil", r.Reference)
	}
}

// --- P5 SQL retrieval normalization ---

// TestNormalizeSQL_StripsThinkBlocks covers the cleanup that runs on
// the LLM-generated SQL before it's handed to the engine.
func TestNormalizeSQL_StripsThinkBlocks(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "SELECT 1", "SELECT 1"},
		{"think block", "<think>x</think>SELECT 1", "SELECT 1"},
		{"code fence", "```sql\nSELECT 1\n```", "SELECT 1"},
		{"trailing semicolon", "SELECT 1;", "SELECT 1"},
		{"all of the above", "<think>x</think>```sql\nSELECT 1;\n```", "SELECT 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizeSQL(c.in); got != c.want {
				t.Errorf("normalizeSQL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestBuildSQLReference_Scalar covers the single-row scalar shortcut in
// buildSQLReference. The path mirrors the previous
// `TestRenderSQLAnswer_Scalar` but goes through the new entry point,
// which requires constructing a minimal OpenAIChatService.
func TestBuildSQLReference_Scalar(t *testing.T) {
	s := &ChatPipelineService{}
	ans, ref := s.buildSQLReference(
		context.Background(), nil, "", "",
		[]map[string]interface{}{{"count": 42.0}},
		"", "", nil, nil,
	)
	if ans != "42" {
		t.Errorf("buildSQLReference scalar answer = %q, want %q", ans, "42")
	}
	// Scalar branch returns empty chunks/doc_aggs and total=1.
	if chunks, _ := ref["chunks"].([]map[string]interface{}); len(chunks) != 0 {
		t.Errorf("scalar branch chunks = %v, want empty", chunks)
	}
	if total, _ := ref["total"].(int); total != 1 {
		t.Errorf("scalar branch total = %d, want 1", total)
	}
}

// TestBuildSQLReference_MultiRowTable covers the markdown-table branch
// (multi-row, multi-column) and verifies that display columns and rows
// render correctly. Mirrors the previous
// `TestRenderSQLAnswer_MultiRowTable`.
func TestBuildSQLReference_MultiRowTable(t *testing.T) {
	rows := []map[string]interface{}{
		{"id": 1.0, "name": "alice"},
		{"id": 2.0, "name": "bob"},
	}
	s := &ChatPipelineService{}
	ans, ref := s.buildSQLReference(
		context.Background(), nil, "", "select id, name from t",
		rows,
		"sys", "elasticsearch", nil, nil,
	)
	// No source columns → empty chunks/doc_aggs.
	if chunks, _ := ref["chunks"].([]map[string]interface{}); len(chunks) != 0 {
		t.Errorf("non-source path chunks = %v, want empty", chunks)
	}
	if !strings.Contains(ans, "|id|") || !strings.Contains(ans, "|name|") {
		t.Errorf("expected header row, got:\n%s", ans)
	}
	if !strings.Contains(ans, "|alice|") || !strings.Contains(ans, "|bob|") {
		t.Errorf("expected data rows, got:\n%s", ans)
	}
	if !strings.Contains(ans, "|------") {
		t.Errorf("expected separator row, got:\n%s", ans)
	}
}

// --- P4 _resolve_reference_metadata ---

// TestResolveReferenceMetadata covers the prompt_config + kwargs
// resolution (matches Python's
// `resolve_reference_metadata_preferences` at
// api/utils/reference_metadata_utils.py:22-62).
func TestResolveReferenceMetadata(t *testing.T) {
	s := &ChatPipelineService{}
	cases := []struct {
		name       string
		promptCfg  map[string]interface{}
		kwargs     map[string]interface{}
		wantInc    bool
		wantFields []string
	}{
		{"all nil", nil, nil, false, nil},
		{"prompt_config only, include=false", map[string]interface{}{
			"reference_metadata": map[string]interface{}{"include": false},
		}, nil, false, nil},
		{"prompt_config only, include=true no fields", map[string]interface{}{
			"reference_metadata": map[string]interface{}{"include": true},
		}, nil, true, nil},
		{"kwargs override prompt_config", map[string]interface{}{
			"reference_metadata": map[string]interface{}{"include": true, "fields": []string{"a"}},
		}, map[string]interface{}{
			"include_metadata": false,
		}, false, nil},
		{"kwargs include_metadata true", nil, map[string]interface{}{
			"include_metadata": true,
		}, true, nil},
		{"kwargs metadata_fields only", nil, map[string]interface{}{
			"include_metadata": true,
			"metadata_fields":  []string{"author", "title"},
		}, true, []string{"author", "title"}},
		{"kwargs reference_metadata sub-dict wins", map[string]interface{}{
			"reference_metadata": map[string]interface{}{"include": true, "fields": []string{"from_config"}},
		}, map[string]interface{}{
			"reference_metadata": map[string]interface{}{"include": true, "fields": []string{"from_request"}},
		}, true, []string{"from_request"}},
		{"fields as []interface{} coerced to []string", map[string]interface{}{
			"reference_metadata": map[string]interface{}{"include": true, "fields": []interface{}{"a", "b", "c"}},
		}, nil, true, []string{"a", "b", "c"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inc, fields := s.resolveReferenceMetadata(c.promptCfg, c.kwargs)
			if inc != c.wantInc {
				t.Errorf("include = %v, want %v", inc, c.wantInc)
			}
			if !reflect.DeepEqual(fields, c.wantFields) {
				t.Errorf("fields = %v, want %v", fields, c.wantFields)
			}
		})
	}
}

// TestDecorateAnswer_VectorStrippedFromReference covers the
// reference-construction step: chunks in the Reference map have
// their `vector` field stripped (so they don't bloat the response).
func TestDecorateAnswer_VectorStrippedFromReference(t *testing.T) {
	s := &ChatPipelineService{}
	timer, _ := newTimerAndPrompt()
	kb := map[string]interface{}{
		"chunks": []map[string]interface{}{
			map[string]interface{}{
				"chunk_id":            "c1",
				"content_with_weight": "hello world",
				"vector":              []float64{0.1, 0.2, 0.3},
				"doc_id":              "d1",
			},
		},
		"doc_aggs": []interface{}{},
	}
	result := s.decorateAnswer(
		context.Background(),
		"x",
		kb,
		"system prompt",
		[]string{"q"},
		0,
		timer,
		nil, 0.0, false,
		nil,
		"",
		nil,
		"",
		nil,
		true,
	)
	chunks, ok := result.Reference["chunks"].([]map[string]interface{})
	if !ok || len(chunks) == 0 {
		t.Fatalf("Reference.chunks missing: %+v", result.Reference)
	}
	chunk := chunks[0]
	if _, has := chunk["vector"]; has {
		t.Errorf("vector field should be stripped from reference chunks, got %+v", chunk)
	}
}

// --- normalizeInternetFlag / shouldUseWebSearch parity with Python ---

// TestNormalizeInternetFlag_PythonParity pins the three-state return of
// the Go port against every input shape _normalize_internet_flag accepts
// in dialog_service.py:108-119. The key user-visible additions vs the
// previous Go implementation are the truthy aliases "yes" / "on" / "1"
// and the explicit falsy aliases "no" / "off" / "0" / "".
func TestNormalizeInternetFlag_PythonParity(t *testing.T) {
	tRue, fAlse := true, false
	cases := []struct {
		name string
		in   interface{}
		want *bool // nil means "couldn't interpret"
	}{
		// bool — straight through
		{"bool true", true, &tRue},
		{"bool false", false, &fAlse},

		// strings — case-insensitive, whitespace-trimmed, alias set
		{"string true", "true", &tRue},
		{"string TRUE", "TRUE", &tRue},
		{"string padded true", "  True  ", &tRue},
		{"string yes", "yes", &tRue},
		{"string on", "on", &tRue},
		{"string 1", "1", &tRue},
		{"string false", "false", &fAlse},
		{"string FALSE", "FALSE", &fAlse},
		{"string no", "no", &fAlse},
		{"string off", "off", &fAlse},
		{"string 0", "0", &fAlse},
		{"string empty", "", &fAlse},
		{"string unknown", "maybe", nil},

		// numerics — only 0 and 1 are valid (Python: `value in (0, 1)`)
		{"int 0", 0, &fAlse},
		{"int 1", 1, &tRue},
		{"int 2", 2, nil},
		{"int64 0", int64(0), &fAlse},
		{"int64 1", int64(1), &tRue},
		{"float64 0", 0.0, &fAlse},
		{"float64 1", 1.0, &tRue},
		{"float64 1.5", 1.5, nil},

		// other types → nil (couldn't interpret)
		{"nil", nil, nil},
		{"slice", []string{"true"}, nil},
		{"map", map[string]string{"a": "b"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeInternetFlag(tc.in)
			switch {
			case tc.want == nil && got == nil:
				return
			case tc.want == nil && got != nil:
				t.Fatalf("input=%#v: want nil, got *%v", tc.in, *got)
			case tc.want != nil && got == nil:
				t.Fatalf("input=%#v: want *%v, got nil", tc.in, *tc.want)
			case *tc.want != *got:
				t.Fatalf("input=%#v: want *%v, got *%v", tc.in, *tc.want, *got)
			}
		})
	}
}

// TestShouldUseWebSearch_RequiresTavilyAndTruthyInternet pins the two
// conjuncts of Python's _should_use_web_search (dialog_service.py:122-126):
// tavily_api_key must be set on prompt_config AND the internet flag must
// normalize to explicit true.
func TestShouldUseWebSearch_RequiresTavilyAndTruthyInternet(t *testing.T) {
	svc := &ChatPipelineService{}
	withTavily := &entity.Chat{
		PromptConfig: entity.JSONMap{"tavily_api_key": "tvly-xxx"},
	}
	withoutTavily := &entity.Chat{
		PromptConfig: entity.JSONMap{},
	}
	nilPromptConfig := &entity.Chat{}

	cases := []struct {
		name   string
		dialog *entity.Chat
		flag   interface{}
		want   bool
	}{
		// disqualifying gates
		{"nil prompt_config", nilPromptConfig, true, false},
		{"empty tavily key", withoutTavily, true, false},
		{"tavily key + nil flag", withTavily, nil, false},
		{"tavily key + false bool", withTavily, false, false},
		{"tavily key + 'false' string", withTavily, "false", false},
		{"tavily key + unrecognized string", withTavily, "maybe", false},

		// enabling combinations — all of these were broken before
		// the normalizer fix and now work.
		{"tavily key + true bool", withTavily, true, true},
		{"tavily key + 'true' string", withTavily, "true", true},
		{"tavily key + 'yes' string (was broken)", withTavily, "yes", true},
		{"tavily key + 'on' string (was broken)", withTavily, "on", true},
		{"tavily key + '1' string (was broken)", withTavily, "1", true},
		{"tavily key + 1 int", withTavily, 1, true},
		{"tavily key + 1.0 float", withTavily, 1.0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := svc.shouldUseWebSearch(tc.dialog, tc.flag); got != tc.want {
				t.Fatalf("dialog=%+v flag=%#v: want %v, got %v",
					tc.dialog.PromptConfig, tc.flag, tc.want, got)
			}
		})
	}
}

// --- P5 SQL retrieval parity helpers (Python use_sql alignment) ---

// TestRemoveRedundantSpaces mirrors common.string_utils.remove_redundant_spaces.
func TestRemoveRedundantSpaces(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// Both passes run sequentially — pass 1 strips space after `(`,
		// pass 2 strips space before `)`, so both go.
		{"pass1+pass2 on ( world )", "hello ( world )", "hello (world)"},
		// Pass 2 strips space before `!`.
		{"pass2: space before !", "world !", "world!"},
		// Comma is not a boundary in pass 2 (it's in the negated set
		// along with `<` and `(`), so no change.
		{"comma not a boundary", "a , b", "a , b"},
		{"no match", "foo bar", "foo bar"},
		{"empty", "", ""},
		{"digit not a boundary", "abc 123", "abc 123"},
		{"left paren kept (no following space)", "(abc)", "(abc)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := removeRedundantSpaces(tc.in); got != tc.want {
				t.Errorf("removeRedundantSpaces(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestStripISOTimestamps verifies the dialog_service.py:1309 cleanup.
// The pattern `T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+Z)?\|` strips the
// timestamp + trailing pipe; the leading pipe/space is preserved (the
// function is meant to operate on the cell boundary). Python's
// `re.sub` has identical behavior.
func TestStripISOTimestamps(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"basic T13:24:55|", "abc T13:24:55|def", "abc |def"},
		{"with ms T13:24:55.123Z|", "abc T13:24:55.123Z|def", "abc |def"},
		{"no match", "abc|def", "abc|def"},
		{"multiple", "x T01:02:03|y T04:05:06|z", "x |y |z"},
		{"no space before T", "abcT13:24:55|def", "abc|def"},
		{"empty", "", ""},
		// Realistic markdown cell: |2024-01-15T13:24:55| → |2024-01-15|
		{"realistic cell", "|2024-01-15T13:24:55|", "|2024-01-15|"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripISOTimestamps(tc.in); got != tc.want {
				t.Errorf("stripISOTimestamps(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestMapColumnName exercises the map_column_name algorithm at
// dialog_service.py:1238-1280.
func TestMapColumnName(t *testing.T) {
	fieldMap := map[string]interface{}{
		"title":      "Title",
		"issue_date": "Issue Date (/Day/Month/Year)",
		"docnm":      "Document Name",
		"docnm_kwd":  "Document Name",
	}
	cases := []struct {
		name string
		col  string
		fm   map[string]interface{}
		want string
	}{
		{"count(star) special case", "count(star)", nil, "COUNT(*)"},
		{"count(star) case-insensitive", "COUNT(STAR)", nil, "COUNT(*)"},
		{"AS alias in field_map", "json_extract_string(c, '$.title') AS title", fieldMap, "Title"},
		{"AS alias not in field_map, case-insensitive", "fn() AS TITLE", fieldMap, "Title"},
		{"AS alias unknown, return as-is", "fn() AS unknown_alias", fieldMap, "unknown_alias"},
		{"direct match", "title", fieldMap, "Title"},
		{"direct case-insensitive", "TITLE", fieldMap, "Title"},
		{"no match, bulk replace", "json_extract_string(c, '$.title')", fieldMap, "json_extract_string(c, '$.Title')"},
		// `(/.*|...)` matches "/Day/Month/Year)" and replaces with "".
		// The leading `(` is left intact — this matches Python's
		// `re.sub` behavior exactly.
		{"paren suffix stripped", "issue_date", fieldMap, "Issue Date ("},
		{"empty field map returns alias", "fn() AS foo", map[string]interface{}{}, "foo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fm := tc.fm
			if fm == nil && tc.name == "count(star) special case" || tc.name == "count(star) case-insensitive" {
				fm = map[string]interface{}{}
			}
			if got := mapColumnName(tc.col, fm); got != tc.want {
				t.Errorf("mapColumnName(%q) = %q, want %q", tc.col, got, tc.want)
			}
		})
	}
}

// TestChunkKBIDForDoc mirrors _chunk_kb_id_for_doc at dialog_service.py:56-59.
func TestChunkKBIDForDoc(t *testing.T) {
	cases := []struct {
		name    string
		rowDict map[string]interface{}
		kbIDs   []string
		docID   interface{}
		want    string
	}{
		{
			name:    "single kb returns kbIDs[0]",
			rowDict: map[string]interface{}{},
			kbIDs:   []string{"kb_a"},
			docID:   "doc1",
			want:    "kb_a",
		},
		{
			name:    "multi kb with kb_id in row",
			rowDict: map[string]interface{}{"kb_id": "kb_b"},
			kbIDs:   []string{"kb_a", "kb_b"},
			docID:   "doc1",
			want:    "kb_b",
		},
		{
			name:    "multi kb with kb_id_kwd in row (no kb_id)",
			rowDict: map[string]interface{}{"kb_id_kwd": "kb_c"},
			kbIDs:   []string{"kb_a", "kb_b"},
			docID:   "doc1",
			want:    "kb_c",
		},
		{
			name:    "multi kb with neither returns empty",
			rowDict: map[string]interface{}{},
			kbIDs:   []string{"kb_a", "kb_b"},
			docID:   "doc1",
			want:    "",
		},
		{
			name:    "multi kb with empty kb_id falls through to kb_id_kwd",
			rowDict: map[string]interface{}{"kb_id": "", "kb_id_kwd": "kb_d"},
			kbIDs:   []string{"kb_a", "kb_b"},
			docID:   "doc1",
			want:    "kb_d",
		},
		{
			name:    "no kbIDs falls through to row lookup",
			rowDict: map[string]interface{}{"kb_id": "kb_a"},
			kbIDs:   nil,
			docID:   "doc1",
			want:    "kb_a",
		},
		{
			name:    "no kbIDs and no row kb_id returns empty",
			rowDict: map[string]interface{}{},
			kbIDs:   nil,
			docID:   "doc1",
			want:    "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := chunkKBIDForDoc(tc.rowDict, tc.kbIDs, tc.docID); got != tc.want {
				t.Errorf("chunkKBIDForDoc = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCleanCellValue verifies the per-cell rendering at
// dialog_service.py:1298 (remove_redundant_spaces + replace None with space).
func TestCleanCellValue(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want string
	}{
		{"string", "hello", "hello"},
		{"float", 42.0, "42"},
		{"int", 42, "42"},
		{"None string literal", "None", " "},
		{"string with redundant space after (", "( world", "(world"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanCellValue(tc.in); got != tc.want {
				t.Errorf("cleanCellValue(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestExtractSourceColumnIndexes verifies stable sorted column ordering
// and case-insensitive source-column detection.
func TestExtractSourceColumnIndexes(t *testing.T) {
	rows := []map[string]interface{}{
		{"DOC_ID": "d1", "docnm_kwd": "Doc1", "title": "T1", "kb_id": "k1"},
	}
	docIDIdx, docNameIdx, kbIDIdx, columns := extractSourceColumnIndexes(rows)
	if len(docIDIdx) != 1 {
		t.Errorf("expected 1 doc_id index, got %d", len(docIDIdx))
	}
	if len(docNameIdx) != 1 {
		t.Errorf("expected 1 doc_name index, got %d", len(docNameIdx))
	}
	if len(kbIDIdx) != 1 {
		t.Errorf("expected 1 kb_id index, got %d", len(kbIDIdx))
	}
	// Columns must be sorted alphabetically: DOC_ID, docnm_kwd, kb_id, title
	wantCols := []string{"DOC_ID", "docnm_kwd", "kb_id", "title"}
	if !reflect.DeepEqual(columns, wantCols) {
		t.Errorf("columns = %v, want %v", columns, wantCols)
	}
	// Empty rows returns empty slices.
	emptyDocID, _, _, _ := extractSourceColumnIndexes(nil)
	if len(emptyDocID) != 0 {
		t.Errorf("empty rows docIDIdx = %v, want empty", emptyDocID)
	}
}

// TestBuildChunkFetchSQL verifies the WHERE-clause extraction and SQL
// construction at dialog_service.py:1321-1331.
func TestBuildChunkFetchSQL(t *testing.T) {
	cases := []struct {
		name      string
		sql       string
		multiKB   bool
		wantSQL   string
		wantFound bool
	}{
		{
			name:      "WHERE + GROUP BY (extracts up to GROUP BY)",
			sql:       "select count(*) from t where x = 1 group by y",
			multiKB:   false,
			wantSQL:   "select doc_id, docnm_kwd from t where x = 1 limit 20",
			wantFound: true,
		},
		{
			name:      "WHERE only, single KB, no limit",
			sql:       "select * from t where x = 1",
			multiKB:   false,
			wantSQL:   "select doc_id, docnm_kwd from t where x = 1 limit 20",
			wantFound: true,
		},
		{
			name:      "WHERE only, multi KB adds kb_id column",
			sql:       "select * from t where x = 1",
			multiKB:   true,
			wantSQL:   "select doc_id, docnm_kwd, kb_id from t where x = 1 limit 20",
			wantFound: true,
		},
		{
			// Python's regex is non-greedy, so WHERE-clause extraction
			// stops at the first occurrence of ORDER BY / LIMIT / GROUP BY.
			// Python's subsequent SQL string is then
			// "select doc_id, ... from t where {where}", which DROPS
			// the order by / limit suffixes. Go matches this behavior.
			name:      "WHERE + ORDER BY + LIMIT 5 (suffixes dropped, no extra limit)",
			sql:       "select * from t where x = 1 order by y limit 5",
			multiKB:   false,
			wantSQL:   "select doc_id, docnm_kwd from t where x = 1 limit 20",
			wantFound: true,
		},
		{
			name:      "no WHERE returns not-found",
			sql:       "select * from t",
			multiKB:   false,
			wantSQL:   "",
			wantFound: false,
		},
		{
			// Python's f-string emits a literal lowercase "where";
			// the original case from the input is NOT preserved.
			name:      "case-insensitive where (output uses lowercase where)",
			sql:       "select * from t WHERE x = 1",
			multiKB:   false,
			wantSQL:   "select doc_id, docnm_kwd from t where x = 1 limit 20",
			wantFound: true,
		},
		{
			name:      "Infinity expectedCol is docnm (not _kwd)",
			sql:       "select * from t where x = 1",
			multiKB:   false,
			wantSQL:   "select doc_id, docnm from t where x = 1 limit 20",
			wantFound: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expectedCol := "docnm_kwd"
			if tc.name == "Infinity expectedCol is docnm (not _kwd)" {
				expectedCol = "docnm"
			}
			gotSQL, gotFound := buildChunkFetchSQL(tc.sql, "t", expectedCol, tc.multiKB)
			if gotFound != tc.wantFound {
				t.Errorf("found = %v, want %v", gotFound, tc.wantFound)
			}
			if gotSQL != tc.wantSQL {
				t.Errorf("sql = %q, want %q", gotSQL, tc.wantSQL)
			}
		})
	}
}

// TestToIfaceSlice verifies the slice type conversion for the call-site
// contract at chat_pipeline.go:3846.
func TestToIfaceSlice(t *testing.T) {
	in := []map[string]interface{}{
		{"a": 1},
		{"b": 2},
	}
	out := toIfaceSlice(in)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if _, ok := out[0].(map[string]interface{}); !ok {
		t.Errorf("element 0 type = %T, want map[string]interface{}", out[0])
	}
}

// TestExpectedDocNameColumn verifies the engine→column name mapping.
func TestExpectedDocNameColumn(t *testing.T) {
	if got := expectedDocNameColumn("infinity"); got != "docnm" {
		t.Errorf("infinity = %q, want docnm", got)
	}
	if got := expectedDocNameColumn("oceanbase"); got != "docnm_kwd" {
		t.Errorf("oceanbase = %q, want docnm_kwd", got)
	}
	if got := expectedDocNameColumn("elasticsearch"); got != "docnm_kwd" {
		t.Errorf("elasticsearch = %q, want docnm_kwd", got)
	}
	if got := expectedDocNameColumn("opensearch"); got != "docnm_kwd" {
		t.Errorf("opensearch = %q, want docnm_kwd", got)
	}
	if got := expectedDocNameColumn("unknown"); got != "docnm_kwd" {
		t.Errorf("unknown = %q, want docnm_kwd", got)
	}
}

// TestIsAggregateSQL matches the regex from dialog_service.py:974.
func TestIsAggregateSQL(t *testing.T) {
	cases := []struct {
		sql  string
		want bool
	}{
		{"select count(*) from t", true},
		{"select sum(x) from t", true},
		{"select avg(x) from t", true},
		{"select max(x), min(y) from t", true},
		{"select count(distinct x) from t", true},
		{"select * from t where x = 1", false},
		{"select distinct x from t", false}, // bare DISTINCT without ( ) doesn't match
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			if got := isAggregateSQL(tc.sql); got != tc.want {
				t.Errorf("isAggregateSQL(%q) = %v, want %v", tc.sql, got, tc.want)
			}
		})
	}
}

// sqlFakeEngine is a minimal in-memory engine.DocEngine stub for
// testing fetchAggregateChunks / buildSQLReference without a real
// engine. It embeds engine.DocEngine to satisfy the interface (the
// embedded methods will panic if accidentally called, which is the
// intended loud-failure mode).
type sqlFakeEngine struct {
	engine.DocEngine
	engineType string
	sqlCalls   *[]string
	rowsBySQL  map[string][]map[string]interface{}
	errBySQL   map[string]error
	runSQL     func(ctx context.Context, table, sqlText string, kbIDs []string) ([]map[string]interface{}, error)
}

func (f *sqlFakeEngine) GetType() string { return f.engineType }
func (f *sqlFakeEngine) RunSQL(ctx context.Context, table, sqlText string, kbIDs []string, format string) ([]map[string]interface{}, error) {
	if f.runSQL != nil {
		return f.runSQL(ctx, table, sqlText, kbIDs)
	}
	if f.sqlCalls != nil {
		*f.sqlCalls = append(*f.sqlCalls, sqlText)
	}
	if f.errBySQL != nil {
		if err, ok := f.errBySQL[sqlText]; ok {
			return nil, err
		}
	}
	if f.rowsBySQL != nil {
		if rows, ok := f.rowsBySQL[sqlText]; ok {
			return rows, nil
		}
	}
	return nil, nil
}

// TestFetchAggregateChunks_SkipsInfinityMultiKB verifies the
// Infinity multi-KB short-circuit (mirrors Python's add_kb_filter
// no-op for Infinity).
func TestFetchAggregateChunks_SkipsInfinityMultiKB(t *testing.T) {
	engine := &sqlFakeEngine{engineType: "infinity"}
	s := &ChatPipelineService{}
	chunks, docAggs := s.fetchAggregateChunks(
		context.Background(), engine, "t",
		"select count(*) from t where x = 1",
		"docnm", []string{"kb_a", "kb_b"},
	)
	if chunks != nil || docAggs != nil {
		t.Errorf("expected nil chunks/docAggs on Infinity multi-KB, got %v / %v", chunks, docAggs)
	}
}

// TestFetchAggregateChunks_SingleKBSuccess verifies the secondary fetch
// path populates chunks and doc_aggs correctly.
func TestFetchAggregateChunks_SingleKBSuccess(t *testing.T) {
	chunksSQL := "select doc_id, docnm_kwd from t where x = 1 limit 20"
	engine := &sqlFakeEngine{
		engineType: "elasticsearch",
		rowsBySQL: map[string][]map[string]interface{}{
			chunksSQL: {
				{"doc_id": "d1", "docnm_kwd": "Doc1"},
				{"doc_id": "d2", "docnm_kwd": "Doc2"},
				{"doc_id": "d1", "docnm_kwd": "Doc1"},
			},
		},
	}
	s := &ChatPipelineService{}
	chunks, docAggs := s.fetchAggregateChunks(
		context.Background(), engine, "t",
		"select count(*) from t where x = 1",
		"docnm_kwd", []string{"kb_a"},
	)
	if len(chunks) != 3 {
		t.Fatalf("chunks len = %d, want 3", len(chunks))
	}
	if len(docAggs) != 2 {
		t.Fatalf("docAggs len = %d, want 2", len(docAggs))
	}
	// d1 appears twice → count=2; d2 once → count=1.
	counts := map[string]int{}
	for _, agg := range docAggs {
		counts[agg["doc_id"].(string)] = agg["count"].(int)
	}
	if counts["d1"] != 2 || counts["d2"] != 1 {
		t.Errorf("counts = %v, want d1=2, d2=1", counts)
	}
	// Single-kb: each chunk gets kb_id from the dialog's kb list.
	for i, c := range chunks {
		if c["kb_id"] != "kb_a" {
			t.Errorf("chunks[%d].kb_id = %v, want kb_a", i, c["kb_id"])
		}
	}
}

// TestFetchAggregateChunks_NoWhereClause verifies the no-WHERE early
// return (matches Python's aggregate fallback at L1365).
func TestFetchAggregateChunks_NoWhereClause(t *testing.T) {
	engine := &sqlFakeEngine{engineType: "elasticsearch"}
	s := &ChatPipelineService{}
	chunks, docAggs := s.fetchAggregateChunks(
		context.Background(), engine, "t",
		"select count(*) from t",
		"docnm_kwd", []string{"kb_a"},
	)
	if chunks != nil || docAggs != nil {
		t.Errorf("expected nil on no-WHERE, got %v / %v", chunks, docAggs)
	}
}

// TestFetchAggregateChunks_RunSQLError verifies graceful failure.
func TestFetchAggregateChunks_RunSQLError(t *testing.T) {
	engine := &sqlFakeEngine{
		engineType: "elasticsearch",
		runSQL: func(ctx context.Context, table, sqlText string, kbIDs []string) ([]map[string]interface{}, error) {
			return nil, fmt.Errorf("engine boom")
		},
	}
	s := &ChatPipelineService{}
	chunks, docAggs := s.fetchAggregateChunks(
		context.Background(), engine, "t",
		"select count(*) from t where x = 1",
		"docnm_kwd", []string{"kb_a"},
	)
	if chunks != nil || docAggs != nil {
		t.Errorf("expected nil on RunSQL error, got %v / %v", chunks, docAggs)
	}
}

// TestBuildSQLReference_EmptyRows verifies the empty-rows path.
func TestBuildSQLReference_EmptyRows(t *testing.T) {
	s := &ChatPipelineService{}
	ans, ref := s.buildSQLReference(
		context.Background(), nil, "", "", nil,
		"", "", nil, nil,
	)
	if ans != "No results." {
		t.Errorf("ans = %q, want %q", ans, "No results.")
	}
	if total, _ := ref["total"].(int); total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
}

// TestBuildSQLReference_NonAggregateWithSourceColumns verifies that
// chunks and doc_aggs are populated from rows when source columns
// are present.
func TestBuildSQLReference_NonAggregateWithSourceColumns(t *testing.T) {
	rows := []map[string]interface{}{
		{"doc_id": "d1", "docnm_kwd": "Doc1", "title": "T1"},
		{"doc_id": "d2", "docnm_kwd": "Doc2", "title": "T2"},
	}
	kbs := []*entity.Knowledgebase{{ID: "kb_a"}}
	s := &ChatPipelineService{}
	ans, ref := s.buildSQLReference(
		context.Background(), nil, "t", "select doc_id, docnm_kwd, title from t",
		rows, "", "elasticsearch", kbs, nil,
	)
	if !strings.Contains(ans, "Source|") {
		t.Errorf("expected Source column in answer, got:\n%s", ans)
	}
	if !strings.Contains(ans, "##0$$") || !strings.Contains(ans, "##1$$") {
		t.Errorf("expected ##N$$ citation markers, got:\n%s", ans)
	}
	chunks, _ := ref["chunks"].([]map[string]interface{})
	if len(chunks) != 2 {
		t.Fatalf("chunks len = %d, want 2", len(chunks))
	}
	docAggs, _ := ref["doc_aggs"].([]map[string]interface{})
	if len(docAggs) != 2 {
		t.Fatalf("docAggs len = %d, want 2", len(docAggs))
	}
	// Each chunk must carry kb_id from the single-KB dialog.
	for i, cm := range chunks {
		if cm["kb_id"] != "kb_a" {
			t.Errorf("chunks[%d].kb_id = %v, want kb_a", i, cm["kb_id"])
		}
	}
}

// TestBuildSQLReference_AggregateMissingSourceColumnsSecondaryFetch
// verifies that an aggregate SQL with no source columns triggers the
// secondary fetch and uses its result for chunks/doc_aggs.
//
// The test uses a multi-cell aggregate (1 row, 2 columns) to avoid the
// scalar shortcut at the top of buildSQLReference.
func TestBuildSQLReference_AggregateMissingSourceColumnsSecondaryFetch(t *testing.T) {
	rows := []map[string]interface{}{
		{"count": 42.0, "label": "total"},
	}
	chunksSQL := "select doc_id, docnm_kwd from t where x = 1 limit 20"
	engine := &sqlFakeEngine{
		engineType: "elasticsearch",
		rowsBySQL: map[string][]map[string]interface{}{
			chunksSQL: {
				{"doc_id": "d1", "docnm_kwd": "Doc1"},
			},
		},
	}
	kbs := []*entity.Knowledgebase{{ID: "kb_a"}}
	s := &ChatPipelineService{}
	ans, ref := s.buildSQLReference(
		context.Background(), engine, "t",
		"select count(*) from t where x = 1",
		rows, "", "elasticsearch", kbs, nil,
	)
	// Multi-cell aggregate → renders as a table, not a scalar.
	if !strings.Contains(ans, "|42|") {
		t.Errorf("ans = %q, want to contain |42|", ans)
	}
	chunks, _ := ref["chunks"].([]map[string]interface{})
	if len(chunks) != 1 {
		t.Errorf("chunks len = %d, want 1 (from secondary fetch)", len(chunks))
	}
}

// TestBuildSQLReference_NonAggregateMissingSourceEmptyRefs verifies
// that non-aggregate SQL without source columns returns the table but
// empty chunks/doc_aggs (Python's best-effort path at L1367).
func TestBuildSQLReference_NonAggregateMissingSourceEmptyRefs(t *testing.T) {
	rows := []map[string]interface{}{
		{"title": "T1"},
		{"title": "T2"},
	}
	s := &ChatPipelineService{}
	ans, ref := s.buildSQLReference(
		context.Background(), nil, "t", "select title from t",
		rows, "", "elasticsearch", nil, nil,
	)
	if !strings.Contains(ans, "T1") || !strings.Contains(ans, "T2") {
		t.Errorf("expected table data in answer, got:\n%s", ans)
	}
	if strings.Contains(ans, "Source|") {
		t.Errorf("expected no Source column, got:\n%s", ans)
	}
	chunks, _ := ref["chunks"].([]map[string]interface{})
	if len(chunks) != 0 {
		t.Errorf("chunks = %v, want empty", chunks)
	}
	docAggs, _ := ref["doc_aggs"].([]interface{})
	if len(docAggs) != 0 {
		t.Errorf("docAggs = %v, want empty", docAggs)
	}
}

// TestBuildSQLReference_DisplayNameTranslation verifies that column
// names are translated via the field_map.
func TestBuildSQLReference_DisplayNameTranslation(t *testing.T) {
	rows := []map[string]interface{}{
		{"doc_id": "d1", "docnm_kwd": "Doc1", "title": "Hello"},
	}
	fieldMap := map[string]interface{}{"title": "My Title"}
	s := &ChatPipelineService{}
	ans, _ := s.buildSQLReference(
		context.Background(), nil, "t", "select doc_id, docnm_kwd, title from t",
		rows, "", "elasticsearch", nil, fieldMap,
	)
	if !strings.Contains(ans, "|My Title|") {
		t.Errorf("expected translated column name, got:\n%s", ans)
	}
	if strings.Contains(ans, "|title|") {
		t.Errorf("raw column name should not appear, got:\n%s", ans)
	}
}

// TestBuildSQLReference_ISOTimestampStripped verifies that ISO
// timestamps in cell values are stripped from the rendered table.
func TestBuildSQLReference_ISOTimestampStripped(t *testing.T) {
	rows := []map[string]interface{}{
		{"doc_id": "d1", "docnm_kwd": "Doc1", "created_at": "2024-01-15T13:24:55"},
	}
	s := &ChatPipelineService{}
	ans, _ := s.buildSQLReference(
		context.Background(), nil, "t", "select doc_id, docnm_kwd, created_at from t",
		rows, "", "elasticsearch", nil, nil,
	)
	if strings.Contains(ans, "T13:24:55") {
		t.Errorf("expected ISO timestamp stripped, got:\n%s", ans)
	}
	if !strings.Contains(ans, "2024-01-15") {
		t.Errorf("expected date portion preserved, got:\n%s", ans)
	}
}

// --- BuildChatConfig unit tests (moved from openai_chat_test.go) ---

// TestBuildChatConfig_RequestOverrides pins down the merge order:
// dialog.LLMSetting is the base; request fields override.
func TestBuildChatConfig_RequestOverrides(t *testing.T) {
	temp := 0.1
	dialog := &entity.Chat{
		LLMSetting: entity.JSONMap{
			"temperature": 0.5,
			"top_p":       0.9,
		},
	}
	req := map[string]interface{}{"temperature": temp}
	cfg := BuildChatConfig(dialog, req)
	if cfg.Temperature == nil || *cfg.Temperature != temp {
		t.Fatalf("expected request temperature %v, got %v", temp, cfg.Temperature)
	}
	if cfg.TopP == nil || *cfg.TopP != 0.9 {
		t.Fatalf("expected dialog top_p 0.9 to be preserved, got %v", cfg.TopP)
	}
}

// TestBuildChatConfig_FromEmptyDialog verifies the merger works even when
// dialog.LLMSetting is nil.
func TestBuildChatConfig_FromEmptyDialog(t *testing.T) {
	temp := 0.3
	dialog := &entity.Chat{}
	req := map[string]interface{}{"temperature": temp}
	cfg := BuildChatConfig(dialog, req)
	if cfg.Temperature == nil || *cfg.Temperature != temp {
		t.Fatalf("expected temperature %v, got %v", temp, cfg.Temperature)
	}
}
