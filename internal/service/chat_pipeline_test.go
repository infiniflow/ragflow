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
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
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
	_, err := s.AsyncChat(t.Context(), "user-1", dialForTest(""), messages, false, nil)
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
	_, err := s.AsyncChat(t.Context(), "user-1", dialForTest(""), nil, false, nil)
	if err == nil {
		t.Fatal("expected error for empty messages, got nil")
	}
}

// TestBuildChatConfig_GenerationConfigReachesModel pins that the dialog's LLM
// setting and per-request overrides (temperature, top_p, max_tokens, thinking,
// stop) reach the model driver, and that request-level overrides win.
func TestBuildChatConfig_GenerationConfigReachesModel(t *testing.T) {
	chat := dialForTest("llm-1")
	chat.LLMSetting = entity.JSONMap{
		"temperature": 0.7,
		"top_p":       0.9,
		"max_tokens":  512,
		"thinking":    true,
		"stop":        []interface{}{"\n", "END"},
	}
	// Request-level overrides win over dialog values.
	cfg := BuildChatConfig(chat, map[string]interface{}{"temperature": 0.3})

	if cfg.Temperature == nil || *cfg.Temperature != 0.3 {
		t.Fatalf("Temperature: want request override 0.3, got %v", cfg.Temperature)
	}
	if cfg.TopP == nil || *cfg.TopP != 0.9 {
		t.Fatalf("TopP: want dialog 0.9, got %v", cfg.TopP)
	}
	if cfg.MaxTokens == nil || *cfg.MaxTokens != 512 {
		t.Fatalf("MaxTokens: want 512, got %v", cfg.MaxTokens)
	}
	if cfg.Thinking == nil || !*cfg.Thinking {
		t.Fatalf("Thinking: want true, got %v", cfg.Thinking)
	}
	if cfg.Stop == nil || len(*cfg.Stop) != 2 {
		t.Fatalf("Stop: want [\"\\n\", \"END\"], got %v", cfg.Stop)
	}
}

// --- P1 Timer + decorateAnswer tests (P0/P1/P7 surface) ---

// TestDecorateAnswer_TimerFormatAlwaysEmitted pins the Markdown
// layout of Timer, ensuring all six phase lines plus Total appear.
func TestDecorateAnswer_TimerFormatAlwaysEmitted(t *testing.T) {
	s := &ChatPipelineService{}
	timer, _ := newTimerAndPrompt()
	result := s.decorateAnswer(
		t.Context(),
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
		t.Context(),
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
		t.Context(),
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
		t.Context(),
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
		t.Context(),
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
		t.Context(),
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
		t.Context(),
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
	hits, err := HydrateChunkVectors(t.Context(),
		map[string]interface{}{"chunks": []interface{}{}},
		nil, nil, 0, nil,
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
	hits, err := HydrateChunkVectors(t.Context(), nil, nil, nil, 0, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if hits != 0 {
		t.Errorf("expected 0 hits, got %d", hits)
	}
}

// TestVectorSignalHelpers pins the Python parity that a zero vector is NOT
// hydrated content: dialog_service.py:93-95 skips a chunk only when
// `any(x for x in v)`, so a placeholder of zeros must still be queued for the
// engine fetch. Without that, the agentic path — whose evidence chunks carry no
// `vector` key at all — left every chunk at zero similarity, and
// insertCitations could never cite anything, so agentic answers came back with
// no references while naive answers (real vectors inline) had them.
func TestVectorSignalHelpers(t *testing.T) {
	// No vector key at all: the agentic evidence shape.
	agentic := map[string]interface{}{"chunk_id": "c1"}
	if vectorHasSignal(chunkVector(agentic)) {
		t.Fatal("chunk without a vector must not count as hydrated")
	}
	// Zero placeholder: the naive ES shape for an unselected embedding.
	zero := map[string]interface{}{"vector": make([]float64, 4)}
	if vectorHasSignal(chunkVector(zero)) {
		t.Fatal("zero placeholder must not count as hydrated")
	}
	if got := firstChunkVectorDim([]map[string]interface{}{agentic, zero}); got != 4 {
		t.Fatalf("firstChunkVectorDim = %d, want 4 (the placeholder length names the field)", got)
	}
	// Real vector in the []interface{} shape a JSON round-trip produces.
	real := map[string]interface{}{"vector": []interface{}{0.0, 1.0, 0.0, 0.0}}
	if !vectorHasSignal(chunkVector(real)) {
		t.Fatal("non-zero vector must count as hydrated")
	}
	if got := firstChunkVectorDim([]map[string]interface{}{real}); got != 4 {
		t.Fatalf("firstChunkVectorDim = %d, want 4", got)
	}
}

// TestGetChunkValueFallsBackOnAbsenceOnly pins Python's get_value semantics
// (`d.get(k1, d.get(k2))`, rag/prompts/generator.py:37-38): the fallback fires
// on key ABSENCE only, so a key present with a nil value wins over k2.
func TestGetChunkValueFallsBackOnAbsenceOnly(t *testing.T) {
	if got := getChunkValue(map[string]interface{}{"content": nil, "content_with_weight": "body"}, "content", "content_with_weight"); got != nil {
		t.Fatalf("got %v, want nil (a present key wins even when nil)", got)
	}
	if got := getChunkValue(map[string]interface{}{"content_with_weight": "body"}, "content", "content_with_weight"); got != "body" {
		t.Fatalf("got %v, want body", got)
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
		t.Context(), nil, "", "",
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
		t.Context(), nil, "", "select id, name from t",
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
		t.Context(),
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
		// Realistic Markdown cell: |2024-01-15T13:24:55| → |2024-01-15|
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
	if got := expectedDocNameColumn("seekdb"); got != "docnm_kwd" {
		t.Errorf("seekdb = %q, want docnm_kwd", got)
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
	sqlEngine := &sqlFakeEngine{engineType: "infinity"}
	s := &ChatPipelineService{}
	chunks, docAggs := s.fetchAggregateChunks(
		t.Context(), sqlEngine, "t",
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
	sqlEngine := &sqlFakeEngine{
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
		t.Context(), sqlEngine, "t",
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
	sqlEngine := &sqlFakeEngine{engineType: "elasticsearch"}
	s := &ChatPipelineService{}
	chunks, docAggs := s.fetchAggregateChunks(
		t.Context(), sqlEngine, "t",
		"select count(*) from t",
		"docnm_kwd", []string{"kb_a"},
	)
	if chunks != nil || docAggs != nil {
		t.Errorf("expected nil on no-WHERE, got %v / %v", chunks, docAggs)
	}
}

// TestFetchAggregateChunks_RunSQLError verifies graceful failure.
func TestFetchAggregateChunks_RunSQLError(t *testing.T) {
	sqlEngine := &sqlFakeEngine{
		engineType: "elasticsearch",
		runSQL: func(ctx context.Context, table, sqlText string, kbIDs []string) ([]map[string]interface{}, error) {
			return nil, fmt.Errorf("engine boom")
		},
	}
	s := &ChatPipelineService{}
	chunks, docAggs := s.fetchAggregateChunks(
		t.Context(), sqlEngine, "t",
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
		t.Context(), nil, "", "", nil,
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
		t.Context(), nil, "t", "select doc_id, docnm_kwd, title from t",
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
	// Each chunk must carry dataset_id (remapped from kb_id by chunksFormat) from the single-KB dialog.
	for i, cm := range chunks {
		if cm["dataset_id"] != "kb_a" {
			t.Errorf("chunks[%d].dataset_id = %v, want kb_a", i, cm["dataset_id"])
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
	sqlEngine := &sqlFakeEngine{
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
		t.Context(), sqlEngine, "t",
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
		t.Context(), nil, "t", "select title from t",
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
		t.Context(), nil, "t", "select doc_id, docnm_kwd, title from t",
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
		t.Context(), nil, "t", "select doc_id, docnm_kwd, created_at from t",
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
			"max_tokens":  float64(512),
		},
	}
	req := map[string]interface{}{
		"temperature": temp,
		"max_tokens":  128,
	}
	cfg := BuildChatConfig(dialog, req)
	if cfg.Temperature == nil || *cfg.Temperature != temp {
		t.Fatalf("expected request temperature %v, got %v", temp, cfg.Temperature)
	}
	if cfg.TopP == nil || *cfg.TopP != 0.9 {
		t.Fatalf("expected dialog top_p 0.9 to be preserved, got %v", cfg.TopP)
	}
	if cfg.MaxTokens == nil || *cfg.MaxTokens != 128 {
		t.Fatalf("expected request max_tokens 128, got %v", cfg.MaxTokens)
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

func TestClampChatConfigMaxTokensUsesRemainingBudget(t *testing.T) {
	dialog := &entity.Chat{
		LLMSetting: entity.JSONMap{"max_tokens": float64(1024)},
	}
	cfg := BuildChatConfig(dialog, map[string]interface{}{"max_tokens": 700})

	adjusted, ok, err := clampChatConfigMaxTokens(cfg, 1000, 450)
	if err != nil {
		t.Fatalf("clampChatConfigMaxTokens returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected max_tokens to be clamped")
	}
	if adjusted != 550 {
		t.Fatalf("adjusted max_tokens = %d, want 550", adjusted)
	}
	if cfg.MaxTokens == nil || *cfg.MaxTokens != 550 {
		t.Fatalf("config max_tokens = %v, want 550", cfg.MaxTokens)
	}
	if dialog.LLMSetting["max_tokens"] != float64(1024) {
		t.Fatalf("dialog max_tokens was mutated: %v", dialog.LLMSetting["max_tokens"])
	}
}

func TestBuildChatConfigIgnoresNonPositiveMaxTokens(t *testing.T) {
	dialog := &entity.Chat{
		LLMSetting: entity.JSONMap{"max_tokens": float64(0)},
	}
	cfg := BuildChatConfig(dialog, nil)
	if cfg.MaxTokens != nil {
		t.Fatalf("persisted max_tokens=0 must not populate ChatConfig, got %v", *cfg.MaxTokens)
	}

	cfg = BuildChatConfig(&entity.Chat{}, map[string]interface{}{"max_tokens": -1})
	if cfg.MaxTokens != nil {
		t.Fatalf("request max_tokens=-1 must not populate ChatConfig, got %v", *cfg.MaxTokens)
	}
}

func TestClampChatConfigMaxTokensRejectsNonPositive(t *testing.T) {
	for _, value := range []int{0, -1} {
		cfg := &modelModule.ChatConfig{MaxTokens: &value}
		if adjusted, ok, err := clampChatConfigMaxTokens(cfg, 1000, 100); err != nil {
			t.Fatalf("max_tokens=%d returned unexpected error: %v", value, err)
		} else if ok {
			t.Fatalf("max_tokens=%d unexpectedly clamped to %d", value, adjusted)
		}
		if cfg.MaxTokens != nil {
			t.Fatalf("max_tokens=%d should be cleared before provider request, got %v", value, *cfg.MaxTokens)
		}
	}
}

func TestClampChatConfigMaxTokensRejectsExhaustedCapacity(t *testing.T) {
	for _, usedTokenCount := range []int{1000, 1001} {
		maxTokens := 700
		cfg := &modelModule.ChatConfig{MaxTokens: &maxTokens}

		if adjusted, ok, err := clampChatConfigMaxTokens(cfg, 1000, usedTokenCount); err == nil {
			t.Fatalf("usedTokenCount=%d expected capacity error, got adjusted=%d ok=%t", usedTokenCount, adjusted, ok)
		}
		if cfg.MaxTokens == nil || *cfg.MaxTokens != maxTokens {
			t.Fatalf("capacity error should not set max_tokens to zero, got %v", cfg.MaxTokens)
		}
	}
}

// stubHarness installs a fake harnessRetriever returning the given answer and
// restores the previous one when the test ends.
func stubHarness(t *testing.T, answer string) {
	t.Helper()
	prev := harnessRetriever
	t.Cleanup(func() { harnessRetriever = prev })
	harnessRetriever = func(ctx context.Context, req HarnessRequest) (HarnessResult, error) {
		return HarnessResult{Answer: answer}, nil
	}
}

// collectSink records every delta with its isThink flag.
func collectSink(got *[]string, thinks *[]bool) func(string, bool) {
	return func(delta string, isThink bool) {
		*got = append(*got, delta)
		*thinks = append(*thinks, isThink)
	}
}

// collectThinkSink is collectSink's structured twin: the harness' step events in
// order, so a test can assert what reached the request's ThinkSink.
func collectThinkSink(events *[]ThinkEvent) func(ThinkEvent) {
	return func(ev ThinkEvent) { *events = append(*events, ev) }
}

// TestRetrieveViaHarnessDoesNotSynthesizeLoopLines pins the fix for a trace that
// described a loop which had not run: the pipeline used to emit
// "[Tool loop] Deciding what to do next ..." / "Step 1: running rag..." around
// EVERY non-naive harness call, whether or not the outer react loop was the
// thing driving research. Those steps are now reported by the loop itself
// (advanced_rag.runOuterReact*), so a harness stub — which by definition runs no
// outer loop — must produce no narration at all.
func TestRetrieveViaHarnessDoesNotSynthesizeLoopLines(t *testing.T) {
	stubHarness(t, "the final cited answer")
	var receivedHistory []map[string]interface{}
	retriever := harnessRetriever
	harnessRetriever = func(ctx context.Context, req HarnessRequest) (HarnessResult, error) {
		receivedHistory = req.Messages
		return retriever(ctx, req)
	}
	history := []map[string]interface{}{{"role": "user", "content": "earlier question"}, {"role": "assistant", "content": "earlier answer"}, {"role": "user", "content": "q"}}

	var got []string
	var thinks []bool
	var events []ThinkEvent
	s := &ChatPipelineService{}
	_, _, _, answer, err := s.retrieveViaHarness(t.Context(), "q", nil, nil, nil, "", "high", "t", "m", "sess", nil, collectSink(&got, &thinks), collectThinkSink(&events), "", history)
	if err != nil {
		t.Fatalf("retrieveViaHarness: %v", err)
	}
	if !reflect.DeepEqual(receivedHistory, history) {
		t.Fatalf("harness history = %#v, want %#v", receivedHistory, history)
	}
	if answer != "the final cited answer" {
		t.Fatalf("answer = %q", answer)
	}
	if len(got) != 0 {
		t.Errorf("the pipeline synthesized %d think delta(s), want none: %#v", len(got), got)
	}
	if len(events) != 0 {
		t.Errorf("the pipeline synthesized %d step event(s), want none: %#v", len(events), events)
	}
}

// TestRetrieveViaHarnessForwardsThinkSink pins the structured-step channel: the
// sink the pipeline is given reaches the harness request, and the events the
// harness emits arrive with their fields intact.
func TestRetrieveViaHarnessForwardsThinkSink(t *testing.T) {
	prev := harnessRetriever
	t.Cleanup(func() { harnessRetriever = prev })
	harnessRetriever = func(ctx context.Context, req HarnessRequest) (HarnessResult, error) {
		if req.ThinkSink == nil {
			return HarnessResult{}, fmt.Errorf("harness request carries no ThinkSink")
		}
		req.ThinkSink(ThinkEvent{
			Kind: "tool_result", Stage: "Function tool", Tool: "retrieve",
			Status: "ok", Reason: "no_doc", Cause: "index not found", Results: 3, Documents: 2,
			Sources: []string{"c1", "c2"}, DurationMS: 12,
			Summary: "[Function tool] The retrieve tool returned 3 results.",
		})
		return HarnessResult{Answer: "a"}, nil
	}

	// The text sink is beside the point here (this test is about the structured
	// channel) and the local `got` below is the EVENT, so leave answerSink nil.
	var events []ThinkEvent
	s := &ChatPipelineService{}
	if _, _, _, _, err := s.retrieveViaHarness(t.Context(), "q", nil, nil, nil, "", "high", "t", "m", "sess", nil, nil, collectThinkSink(&events), "", nil); err != nil {
		t.Fatalf("retrieveViaHarness: %v", err)
	}
	// Only what the harness reported: the pipeline adds nothing of its own.
	var got *ThinkEvent
	for i := range events {
		if events[i].Kind == "tool_result" {
			got = &events[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("no tool_result among %#v", events)
	}
	if got.Tool != "retrieve" || got.Status != "ok" || got.Cause != "index not found" ||
		got.Results != 3 || got.Documents != 2 || got.DurationMS != 12 ||
		len(got.Sources) != 2 || got.Summary == "" {
		t.Errorf("event lost fields on the way through: %#v", *got)
	}
	if len(events) != 1 {
		t.Errorf("events = %#v, want only what the harness reported", events)
	}
}

// TestHarnessThinkSinkForwardsEvent covers the pipeline-side half: one event in,
// one chunk out, carrying the event and no answer delta.
func TestHarnessThinkSinkForwardsEvent(t *testing.T) {
	out := make(chan AsyncChatResult, 1)
	harnessThinkSink(context.Background(), out)(ThinkEvent{Kind: "stage", Summary: "[Planner] x"})

	select {
	case chunk := <-out:
		if chunk.ThinkEvent == nil || chunk.ThinkEvent.Kind != "stage" || chunk.ThinkEvent.Summary != "[Planner] x" {
			t.Fatalf("chunk = %#v, want the event", chunk)
		}
		if chunk.Answer != "" || chunk.Final {
			t.Errorf("an event chunk must carry no answer and not be final: %#v", chunk)
		}
	default:
		t.Fatal("event was not forwarded")
	}

	// A cancelled request must not block the producer.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	harnessThinkSink(ctx, make(chan AsyncChatResult))(ThinkEvent{Summary: "y"})
}

// TestRetrieveViaHarnessNaiveEmitsNothing guards the naive path: no agentic loop
// runs, so nothing is narrated — and with the pipeline no longer synthesizing
// its own lines, nothing is emitted at all.
func TestRetrieveViaHarnessNaiveEmitsNothing(t *testing.T) {
	stubHarness(t, "x")

	var got []string
	var thinks []bool
	s := &ChatPipelineService{}
	if _, _, _, _, err := s.retrieveViaHarness(t.Context(), "q", nil, nil, nil, "", "naive", "t", "m", "sess", nil, collectSink(&got, &thinks), nil, "", nil); err != nil {
		t.Fatalf("retrieveViaHarness: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("naive mode emitted %d deltas, want 0: %#v", len(got), got)
	}
}

// TestDecorateHarnessAnswerReferenceUsesClientChunkShape pins that the reasoning
// path hands consumers the same reference shape as the non-reasoning one
// (content / document_name / dataset_id — not the engine keys), and that the
// source chunks are left untouched (no in-place vector stripping).
func TestDecorateHarnessAnswerReferenceUsesClientChunkShape(t *testing.T) {
	src := map[string]interface{}{
		"chunk_id":            "c1",
		"content_with_weight": "hello",
		"docnm_kwd":           "Doc One",
		"doc_id":              "d1",
		"kb_id":               "kb1",
		"vector":              []float64{0.1, 0.2},
	}
	kbinfos := map[string]interface{}{
		"chunks":   []map[string]interface{}{src},
		"doc_aggs": []interface{}{map[string]interface{}{"doc_id": "d1", "doc_name": "Doc One"}},
	}

	s := &ChatPipelineService{}
	res := s.decorateHarnessAnswer("The answer [ID:0]", kbinfos, nil, nil)
	if res.Reference == nil {
		t.Fatal("a cited answer must carry a reference")
	}
	chunks, _ := res.Reference["chunks"].([]map[string]interface{})
	if len(chunks) != 1 {
		t.Fatalf("reference chunks = %#v, want 1", res.Reference["chunks"])
	}
	if chunks[0]["content"] != "hello" || chunks[0]["document_name"] != "Doc One" || chunks[0]["dataset_id"] != "kb1" {
		t.Errorf("reference chunk = %#v, want the client-facing keys populated", chunks[0])
	}
	for _, engineKey := range []string{"content_with_weight", "docnm_kwd", "kb_id", "vector"} {
		if _, has := chunks[0][engineKey]; has {
			t.Errorf("reference chunk leaked engine key %q", engineKey)
		}
	}
	if _, has := src["vector"]; !has {
		t.Error("the shared source chunk lost its vector: the reference must not mutate it")
	}
}

// TestReferenceChunksKeepsRenderedSlots pins the index contract of the reference
// payload: the client resolves a marker by indexing reference.chunks with the
// number the model wrote, so entry i has to be the chunk of rendered block i. A
// block whose chunk cannot be located keeps its slot — dropping it would slide
// every later marker onto a passage the model never cited.
func TestReferenceChunksKeepsRenderedSlots(t *testing.T) {
	pool := []map[string]interface{}{
		{"chunk_id": "p0", "content": "zero"},
		{"chunk_id": "p1", "content": "one"},
		{"chunk_id": "p2", "content": "two"},
	}
	// Rendered blocks: pool 2, one that cannot be located, pool 0. The remaining
	// pool chunk follows so nothing retrieved is lost.
	got := referenceChunks([]int{2, -1, 0}, pool)
	if len(got) != 4 {
		t.Fatalf("reference = %d entries (%v), want 3 rendered slots plus the pool remainder", len(got), got)
	}
	if got[0]["chunk_id"] != "p2" || got[1] != nil || got[2]["chunk_id"] != "p0" {
		t.Fatalf("reference = %v, want p2 at block 0, an empty slot at block 1, p0 at block 2", got)
	}
	if got[3]["chunk_id"] != "p1" {
		t.Errorf("reference tail = %v, want the remaining pool chunk p1", got[3])
	}
}

// TestReferenceChunksFallsBackToPoolOrder keeps the no-CiteChunkIDs behavior: the
// pool order is then the numbering, so the reference is the pool untouched.
func TestReferenceChunksFallsBackToPoolOrder(t *testing.T) {
	pool := []map[string]interface{}{{"chunk_id": "p0"}, {"chunk_id": "p1"}}
	got := referenceChunks(nil, pool)
	if len(got) != 2 || got[0]["chunk_id"] != "p0" || got[1]["chunk_id"] != "p1" {
		t.Fatalf("reference = %v, want the pool in order", got)
	}
}

// TestCitationAuditReportsFigureChunkAndImage pins the observability line that
// answers "why does Fig. n show no image": it names the chunk behind each cited
// figure (Fig. n = rendered block n-1) and whether that chunk has an image, so a
// text-only chunk (nothing to draw) is distinguishable from a marker that resolved
// to the wrong chunk.
func TestCitationAuditReportsFigureChunkAndImage(t *testing.T) {
	chunks := []map[string]interface{}{
		{"chunk_id": "c0", "image_id": "img-0"},
		{"chunk_id": "c1"},
		{"chunk_id": "c2", "img_id": "img-2"},
	}
	// Rendered blocks: pool 2, pool 0, pool 1 (Fig. 1..3). The model cited blocks 0
	// and 2, so pool 2 (image) and pool 1 (no image) come back resolved.
	got := citationAudit([]int{2, 0, 1}, []int{2, 1}, chunks)
	want := "Fig.1=c2 image=true Fig.3=c1 image=false"
	if got != want {
		t.Errorf("citationAudit = %q, want %q", got, want)
	}
}

// TestDecorateHarnessAnswerRewritesSlotCitations is the end-to-end lock for
// the leaked "[ID:Slot 0]" bug: the harness answer arrives citing the internal
// slot table, and the final answer must cite the chunk that filled the slot
// instead — with that chunk present in the reference payload.
func TestDecorateHarnessAnswerRewritesSlotCitations(t *testing.T) {
	kbinfos := map[string]interface{}{
		"chunks": []map[string]interface{}{
			{"chunk_id": "c1", "content_with_weight": "unrelated", "doc_id": "d0", "docnm_kwd": "Other"},
			{"chunk_id": "c9", "content_with_weight": "eiffel", "doc_id": "d1", "docnm_kwd": "Doc One"},
		},
		"doc_aggs": []interface{}{
			map[string]interface{}{"doc_id": "d1", "doc_name": "Doc One"},
		},
	}
	s := &ChatPipelineService{}
	res := s.decorateHarnessAnswer(
		"The tower opened in 1889 [ID:Slot 0].",
		kbinfos,
		map[string][]string{"0": {"c9"}},
		nil,
	)
	if strings.Contains(res.Answer, "[ID:Slot") {
		t.Fatalf("final answer still carries the internal slot citation: %q", res.Answer)
	}
	if !strings.Contains(res.Answer, "[ID:1]") {
		t.Fatalf("final answer must cite the evidence chunk pool index, got %q", res.Answer)
	}
	chunks, _ := res.Reference["chunks"].([]map[string]interface{})
	// The reference keeps the whole citation pool (Python parity); it must at
	// least carry the slot's evidence chunk so [ID:1] is openable.
	found := false
	for _, c := range chunks {
		if c["content"] == "eiffel" {
			found = true
		}
	}
	if !found {
		t.Fatalf("reference must carry the slot's evidence chunk, got %#v", res.Reference["chunks"])
	}
}

// TestThinkEventWireKeys pins the JSON keys of a fully populated step: they ARE
// the client contract (the SSE `think_event` field), so renaming or dropping a tag
// breaks every client while neither the compiler nor any Go caller notices.
//
// It is also the only assertion on this struct's shape, which is why the harness
// ALIASES it (harness.ThinkEvent) instead of carrying a second copy bridged field
// by field in cmd — that copy compiled for as long as it stayed a subset, and
// silently dropped any field added on the other side.
func TestThinkEventWireKeys(t *testing.T) {
	// Every field populated: with `omitempty` on all but Kind and Summary, that is
	// what makes the whole key set appear.
	full := ThinkEvent{
		Kind:       "tool_result",
		Stage:      "Function tool",
		Tool:       "retrieve",
		Args:       `{"query":"q"}`,
		Status:     "ok",
		Reason:     "no_doc",
		Cause:      "the knowledge base returned no searchable hits",
		Results:    3,
		Documents:  2,
		Sources:    []string{"c1", "c2"},
		DurationMS: 12,
		Summary:    "The retrieve tool returned 3 results from 2 documents.",
	}
	want := []string{
		"args", "cause", "documents", "duration_ms", "kind", "reason",
		"results", "sources", "stage", "status", "summary", "tool",
	}
	if got := thinkEventWireKeys(t, full); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("wire keys = %v, want %v", got, want)
	}

	// A stage step carries only Kind, Stage and Summary; everything else is
	// omitted rather than sent as a zero — a client must treat each optional field
	// as absent-when-zero, not as present-and-empty.
	got := thinkEventWireKeys(t, ThinkEvent{Kind: "stage", Stage: "RAGAgent", Summary: "x"})
	if strings.Join(got, ",") != "kind,stage,summary" {
		t.Errorf("stage step keys = %v, want kind,stage,summary", got)
	}
}

// thinkEventWireKeys marshals one event and returns its sorted JSON key set.
func thinkEventWireKeys(t *testing.T, ev ThinkEvent) []string {
	t.Helper()
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	keys := make([]string, 0, len(decoded))
	for k := range decoded {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestDecorateHarnessAnswerUncitedStillCarriesReference: an agentic answer the
// model composed WITHOUT citation markers must still ship the passages it was
// composed from. Python's `refs = ... if doc_ids else []` returned an empty
// reference for exactly that case (a one-passage pool is the common one), which
// left the user with an answer and nothing to open — while the naive path hands
// the passages back unconditionally.
func TestDecorateHarnessAnswerUncitedStillCarriesReference(t *testing.T) {
	kbinfos := map[string]interface{}{
		"chunks": []map[string]interface{}{
			{
				"chunk_id":            "c1",
				"content_with_weight": "the wolf is grey",
				"doc_id":              "d1",
				"docnm_kwd":           "wolf.jpg",
				"kb_id":               "kb1",
			},
		},
		"doc_aggs": []interface{}{
			map[string]interface{}{"doc_id": "d1", "doc_name": "wolf.jpg"},
		},
	}

	s := &ChatPipelineService{}
	res := s.decorateHarnessAnswer("小狼的颜色是灰色的。", kbinfos, nil, nil)
	if res.Reference == nil {
		t.Fatal("an agentic answer with a citation pool must carry a reference")
	}
	chunks, _ := res.Reference["chunks"].([]map[string]interface{})
	if len(chunks) != 1 || chunks[0]["document_name"] != "wolf.jpg" {
		t.Fatalf("reference chunks = %#v, want the pool in the client-facing shape", res.Reference["chunks"])
	}
	// Nothing was cited, so no document can be filtered out.
	if aggs, _ := res.Reference["doc_aggs"].([]interface{}); len(aggs) != 1 {
		t.Fatalf("reference doc_aggs = %#v, want the whole pool kept", res.Reference["doc_aggs"])
	}
}

// TestDecorateHarnessAnswerResolvesRenderedPosition is the regression lock for
// the one-passage agentic answer that came back with no reference. The compose
// numbers its evidence 0-based and the model writes that number, so the marker
// and the reference line up: the passage it names is openable, and a marker that
// names nothing is removed instead of being handed to the client.
func TestDecorateHarnessAnswerResolvesRenderedPosition(t *testing.T) {
	kbinfos := map[string]interface{}{
		"chunks": []map[string]interface{}{
			{
				"chunk_id":            "c1",
				"content_with_weight": "the wolf is grey",
				"doc_id":              "d1",
				"docnm_kwd":           "wolf.jpg",
			},
		},
		"doc_aggs": []interface{}{
			map[string]interface{}{"doc_id": "d1", "doc_name": "wolf.jpg"},
		},
	}

	s := &ChatPipelineService{}
	res := s.decorateHarnessAnswer("小狼的颜色是灰色的 [ID:0]。", kbinfos, nil, []string{"c1"})
	if !strings.Contains(res.Answer, "[ID:0]") {
		t.Fatalf("the marker is the client's index and must survive, got %q", res.Answer)
	}
	if res.Reference == nil {
		t.Fatal("a one-passage agentic answer must carry a reference")
	}
	chunks, _ := res.Reference["chunks"].([]map[string]interface{})
	if len(chunks) != 1 || chunks[0]["document_name"] != "wolf.jpg" {
		t.Fatalf("reference chunks = %#v, want the cited passage", res.Reference["chunks"])
	}

	// The old numbering (a 1-based position) names no entry in a one-passage
	// reference, so it must be dropped rather than shipped as a dead marker.
	res = s.decorateHarnessAnswer("小狼的颜色是灰色的 [ID:1]。", kbinfos, nil, []string{"c1"})
	if strings.Contains(res.Answer, "[ID:1]") {
		t.Fatalf("an unresolvable marker must be dropped, got %q", res.Answer)
	}
	if res.Reference == nil {
		t.Fatal("the reference must survive a dropped marker")
	}
}

// TestDecorateHarnessAnswerOrdersReferenceByRenderedOrder pins the ordering half
// of the contract: the compose renders the evidence similarity-ranked (and
// capped), so rendered position 0 is not pool[0]. The reference must put the
// rendered list first — a marker's index is its position there — and append the
// rest of the pool.
func TestDecorateHarnessAnswerOrdersReferenceByRenderedOrder(t *testing.T) {
	kbinfos := map[string]interface{}{
		"chunks": []map[string]interface{}{
			{"chunk_id": "c1", "content_with_weight": "unrelated", "doc_id": "d1", "docnm_kwd": "Doc One"},
			{"chunk_id": "c2", "content_with_weight": "unrelated too", "doc_id": "d1", "docnm_kwd": "Doc One"},
			{"chunk_id": "c3", "content_with_weight": "eiffel", "doc_id": "d2", "docnm_kwd": "Doc Two"},
		},
		"doc_aggs": []interface{}{
			map[string]interface{}{"doc_id": "d1", "doc_name": "Doc One"},
			map[string]interface{}{"doc_id": "d2", "doc_name": "Doc Two"},
		},
	}

	s := &ChatPipelineService{}
	// The compose ranked c3 first, so the model's [ID:0] is c3.
	res := s.decorateHarnessAnswer(
		"The tower opened in 1889 [ID:0].",
		kbinfos,
		nil,
		[]string{"c3", "c1"},
	)
	chunks, _ := res.Reference["chunks"].([]map[string]interface{})
	if len(chunks) != 3 {
		t.Fatalf("reference chunks = %#v, want the rendered list plus the remaining pool", res.Reference["chunks"])
	}
	if chunks[0]["content"] != "eiffel" || chunks[0]["id"] != "c3" {
		t.Fatalf("reference[0] = %#v, want the rendered first passage c3", chunks[0])
	}
	if chunks[1]["id"] != "c1" || chunks[2]["id"] != "c2" {
		t.Fatalf("reference order = %#v, want [c3 c1 c2]", chunks)
	}
	// Only Doc Two (c3) was cited, so the other document is filtered out.
	aggs, _ := res.Reference["doc_aggs"].([]interface{})
	if len(aggs) != 1 {
		t.Fatalf("reference doc_aggs = %#v, want only the cited document", res.Reference["doc_aggs"])
	}
	if dam, _ := aggs[0].(map[string]interface{}); dam["doc_id"] != "d2" {
		t.Fatalf("reference doc_aggs = %#v, want d2", aggs[0])
	}
}

// TestDecorateHarnessAnswerDropsOnlyCanonicalMarkers pins the text-safety rule: a
// canonical citation naming no rendered passage is removed (the user could never
// open it), while a bare bracketed number in prose is left alone — "[2024]" is far
// more likely to be ordinary text than a citation, and deleting user-visible text
// is worse than a chip that cannot open.
func TestDecorateHarnessAnswerDropsOnlyCanonicalMarkers(t *testing.T) {
	kbinfos := map[string]interface{}{
		"chunks": []map[string]interface{}{
			{"chunk_id": "c1", "content_with_weight": "one", "doc_id": "d1", "docnm_kwd": "Doc One"},
		},
		"doc_aggs": []interface{}{map[string]interface{}{"doc_id": "d1", "doc_name": "Doc One"}},
	}

	s := &ChatPipelineService{}
	res := s.decorateHarnessAnswer(
		"发布于 [2024] 年 [ID:0]，并在 [ID:7] 修订。",
		kbinfos,
		nil,
		[]string{"c1"},
	)
	if !strings.Contains(res.Answer, "[2024]") {
		t.Errorf("prose brackets must survive, got %q", res.Answer)
	}
	if !strings.Contains(res.Answer, "[ID:0]") {
		t.Errorf("a resolvable citation must survive, got %q", res.Answer)
	}
	if strings.Contains(res.Answer, "[ID:7]") {
		t.Errorf("an unresolvable canonical marker must be dropped, got %q", res.Answer)
	}
}

// TestDecorateHarnessAnswerKeepsPositionsAfterUnknownBlock: when the harness
// publishes an id the pool does not carry, that block keeps its slot so the blocks
// after it keep the numbers the model saw. A marker naming a later block must
// still resolve to the right chunk — and to the right document for doc_aggs.
func TestDecorateHarnessAnswerKeepsPositionsAfterUnknownBlock(t *testing.T) {
	kbinfos := map[string]interface{}{
		"chunks": []map[string]interface{}{
			{"chunk_id": "c1", "content_with_weight": "one", "doc_id": "d1", "docnm_kwd": "Doc One"},
			{"chunk_id": "c2", "content_with_weight": "two", "doc_id": "d1", "docnm_kwd": "Doc One"},
			{"chunk_id": "c3", "content_with_weight": "eiffel", "doc_id": "d2", "docnm_kwd": "Doc Two"},
		},
		"doc_aggs": []interface{}{
			map[string]interface{}{"doc_id": "d1", "doc_name": "Doc One"},
			map[string]interface{}{"doc_id": "d2", "doc_name": "Doc Two"},
		},
	}

	s := &ChatPipelineService{}
	// The harness rendered c1, an unknown chunk, then c3 — so [ID:2] is c3.
	res := s.decorateHarnessAnswer(
		"The tower opened in 1889 [ID:2].",
		kbinfos,
		nil,
		[]string{"c1", "gone", "c3"},
	)
	if !strings.Contains(res.Answer, "[ID:2]") {
		t.Fatalf("a marker past the unknown block must survive, got %q", res.Answer)
	}
	aggs, _ := res.Reference["doc_aggs"].([]interface{})
	if len(aggs) != 1 {
		t.Fatalf("reference doc_aggs = %#v, want only the cited document", res.Reference["doc_aggs"])
	}
	if dam, _ := aggs[0].(map[string]interface{}); dam["doc_id"] != "d2" {
		t.Fatalf("reference doc_aggs = %#v, want d2 (c3)", aggs[0])
	}
}

// setupChatPipelineToolSupportTestDB seeds an in-memory scope whose models differ
// only in their persisted tool-calling flag, so the probe runs without the provider
// catalog:
//
//	model-tools-on        — chat,    extra {"is_tools": true}
//	model-tools-off       — chat,    extra {"is_tools": false}
//	model-tools-disabled  — chat,    extra {"is_tools": true}, status inactive
//	model-embedding-tools — embedding, extra {"is_tools": true}
func setupChatPipelineToolSupportTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
	); err != nil {
		t.Fatalf("failed to migrate model tables: %v", err)
	}

	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	activeStatus := "1"
	rows := []interface{}{
		&entity.UserTenant{ID: "user-tenant-1", UserID: "user-1", TenantID: "tenant-1", Role: "owner", InvitedBy: "user-1", Status: &activeStatus},
		&entity.TenantModelProvider{ID: "provider-zhipu", TenantID: "tenant-1", ProviderName: "ZHIPU-AI"},
		&entity.TenantModelInstance{ID: "instance-zhipu", ProviderID: "provider-zhipu", InstanceName: "default", APIKey: "sk-test", Status: "active", Extra: "{}"},
		&entity.TenantModel{ID: "model-tools-on", ProviderID: "provider-zhipu", InstanceID: "instance-zhipu", ModelName: "glm-4-plus", ModelType: int(entity.ModelTypeChat), Status: "active", Extra: `{"is_tools":true}`},
		&entity.TenantModel{ID: "model-tools-off", ProviderID: "provider-zhipu", InstanceID: "instance-zhipu", ModelName: "glm-4-flash", ModelType: int(entity.ModelTypeChat), Status: "active", Extra: `{"is_tools":false}`},
		&entity.TenantModel{ID: "model-tools-disabled", ProviderID: "provider-zhipu", InstanceID: "instance-zhipu", ModelName: "glm-4-air", ModelType: int(entity.ModelTypeChat), Status: "inactive", Extra: `{"is_tools":true}`},
		&entity.TenantModel{ID: "model-embedding-tools", ProviderID: "provider-zhipu", InstanceID: "instance-zhipu", ModelName: "embedding-3", ModelType: int(entity.ModelTypeEmbedding), Status: "active", Extra: `{"is_tools":true}`},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("failed to seed %T: %v", row, err)
		}
	}
	return db
}

// seedChatPipelineToolSupportTenant sets the tenant's default chat model, which
// the no-explicit-LLM branch of the probe resolves.
func seedChatPipelineToolSupportTenant(t *testing.T, db *gorm.DB, defaultLLMID string) {
	t.Helper()
	status := string(entity.StatusValid)
	if err := db.Create(&entity.Tenant{
		ID:     "tenant-1",
		LLMID:  defaultLLMID,
		Status: &status,
	}).Error; err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}
}

// TestChatModelSupportsToolsReadsTenantModelFlag pins that the probe reads the
// flag persisted on the tenant model's extra JSON, in both directions — the
// tenant's own value beats the catalog.
func TestChatModelSupportsToolsReadsTenantModelFlag(t *testing.T) {
	setupChatPipelineToolSupportTestDB(t)
	svc := NewChatPipelineService()

	for _, tc := range []struct {
		name string
		llm  string
		want bool
	}{
		{"tool-capable model", "model-tools-on", true},
		{"model without tool support", "model-tools-off", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.chatModelSupportsTools(t.Context(), &entity.Chat{TenantID: "tenant-1", LLMID: tc.llm})
			if got != tc.want {
				t.Errorf("chatModelSupportsTools(%s) = %v, want %v", tc.llm, got, tc.want)
			}
		})
	}
}

// TestChatModelSupportsToolsRejectsInvalidReferences pins the fail-closed rule:
// a disabled model, a model not enrolled as a chat model, and an unenrolled
// reference all report no tool support rather than a definitive yes — the same
// outcome as Python's `getattr(chat_mdl, "is_tools", False)` default.
func TestChatModelSupportsToolsRejectsInvalidReferences(t *testing.T) {
	setupChatPipelineToolSupportTestDB(t)
	svc := NewChatPipelineService()

	for _, tc := range []struct {
		name string
		llm  string
	}{
		{"disabled model", "model-tools-disabled"},
		{"not enrolled as chat", "model-embedding-tools"},
		{"unknown model", "no-such-model"},
		{"unenrolled composite reference", "ghost@default@ZHIPU-AI"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := svc.chatModelSupportsTools(t.Context(), &entity.Chat{TenantID: "tenant-1", LLMID: tc.llm}); got {
				t.Errorf("chatModelSupportsTools(%s) = true, want false", tc.llm)
			}
		})
	}
}

// TestChatModelSupportsToolsFallsBackToTenantDefault covers the no-explicit-LLM
// branch: the probe must resolve the tenant default chat model (composite
// "model@provider" reference) and read its flag.
func TestChatModelSupportsToolsFallsBackToTenantDefault(t *testing.T) {
	db := setupChatPipelineToolSupportTestDB(t)
	seedChatPipelineToolSupportTenant(t, db, "glm-4-plus@ZHIPU-AI")
	svc := NewChatPipelineService()

	if got := svc.chatModelSupportsTools(t.Context(), &entity.Chat{TenantID: "tenant-1"}); !got {
		t.Error("tenant default with is_tools=true reported no tool support, want true")
	}
}

// TestChatModelSupportsToolsTenantDefaultWithoutToolSupport is the mirror case:
// a tenant default that cannot call tools must not be reported as capable.
func TestChatModelSupportsToolsTenantDefaultWithoutToolSupport(t *testing.T) {
	db := setupChatPipelineToolSupportTestDB(t)
	seedChatPipelineToolSupportTenant(t, db, "glm-4-flash@ZHIPU-AI")
	svc := NewChatPipelineService()

	if got := svc.chatModelSupportsTools(t.Context(), &entity.Chat{TenantID: "tenant-1"}); got {
		t.Error("tenant default with is_tools=false reported tool support, want false")
	}
}

// TestChatModelSupportsToolsMissingDependencies is the guard case: an
// uninitialised service or a nil dialog must not panic and must report no tool
// support.
func TestChatModelSupportsToolsMissingDependencies(t *testing.T) {
	var nilSvc *ChatPipelineService
	if got := nilSvc.chatModelSupportsTools(t.Context(), &entity.Chat{TenantID: "tenant-1", LLMID: "model-tools-on"}); got {
		t.Error("nil service reported tool support, want false")
	}

	emptySvc := &ChatPipelineService{}
	if got := emptySvc.chatModelSupportsTools(t.Context(), &entity.Chat{TenantID: "tenant-1", LLMID: "model-tools-on"}); got {
		t.Error("service without a model provider reported tool support, want false")
	}

	svc := NewChatPipelineService()
	if got := svc.chatModelSupportsTools(t.Context(), nil); got {
		t.Error("nil dialog reported tool support, want false")
	}
}

// TestReasoningNeedsAgenticGraphGatesOnToolSupport pins the gate Python
// rag_agent applies before building its agentic loop: reasoning level 0 turns it
// off, and so does a chat model that cannot call tools. Only reasoning enabled
// together with tool support drives the agentic graph.
func TestReasoningNeedsAgenticGraphGatesOnToolSupport(t *testing.T) {
	setupChatPipelineToolSupportTestDB(t)
	svc := NewChatPipelineService()

	capable := &entity.Chat{TenantID: "tenant-1", LLMID: "model-tools-on"}
	incapable := &entity.Chat{TenantID: "tenant-1", LLMID: "model-tools-off"}

	for _, tc := range []struct {
		name           string
		chat           *entity.Chat
		reasoningLevel int
		want           bool
	}{
		{"reasoning off", capable, 0, false},
		{"reasoning on and tool-capable", capable, 2, true},
		{"reasoning on but no tool support", incapable, 2, false},
		{"reasoning on, unresolvable model", &entity.Chat{TenantID: "tenant-1", LLMID: "no-such-model"}, 4, false},
		{"reasoning on, nil dialog", nil, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := svc.reasoningNeedsAgenticGraph(t.Context(), tc.chat, tc.reasoningLevel); got != tc.want {
				t.Errorf("reasoningNeedsAgenticGraph(level=%d) = %v, want %v", tc.reasoningLevel, got, tc.want)
			}
		})
	}
}

// TestChatModelSupportsToolsProbeIsCached pins the memoization: a verdict may be
// reused, but only for chatModelToolSupportTTL.
func TestChatModelSupportsToolsProbeIsCached(t *testing.T) {
	db := setupChatPipelineToolSupportTestDB(t)
	svc := NewChatPipelineService()
	chat := &entity.Chat{TenantID: "tenant-1", LLMID: "model-tools-on"}

	if !svc.chatModelSupportsTools(t.Context(), chat) {
		t.Fatal("tool-capable model reported no support")
	}

	// Flip the persisted flag: the verdict cached by the first probe must hold
	// until its TTL lapses, which is what makes repeated reasoning turns cheap.
	if err := db.Model(&entity.TenantModel{}).Where("id = ?", "model-tools-on").
		Update("extra", `{"is_tools":false}`).Error; err != nil {
		t.Fatalf("flip is_tools: %v", err)
	}
	if !svc.chatModelSupportsTools(t.Context(), chat) {
		t.Fatal("cached verdict was dropped before its TTL lapsed")
	}

	// Expire the entry through the same accessors the probe uses: the next call
	// must read the row again and see the flipped flag.
	provider := svc.ModelProviderSvc
	provider.toolSupportMu.Lock()
	provider.toolSupportCache[chatModelToolSupportKey("tenant-1", "model-tools-on")] = chatModelToolSupportEntry{
		supported: true,
		expiresAt: time.Now().Add(-time.Second),
	}
	provider.toolSupportMu.Unlock()

	if svc.chatModelSupportsTools(t.Context(), chat) {
		t.Fatal("expired verdict was reused instead of re-probing")
	}
}
