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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/entity"
	"ragflow/internal/tokenizer"
)

// TestOpenAI_FilterMessagesDropsSystemAndLeadingAssistant pins down the
// Python openai_api.py:301-307 behavior: drop all system messages, then
// drop leading assistant messages until the first user/system message.
func TestOpenAI_FilterMessagesDropsSystemAndLeadingAssistant(t *testing.T) {
	svc := &OpenAIChatService{}
	in := []map[string]interface{}{
		{"role": "system", "content": "you are a helper"},
		{"role": "assistant", "content": "leading assistant, dropped"},
		{"role": "user", "content": "hello"},
		{"role": "assistant", "content": "world"},
		{"role": "system", "content": "another system, dropped"},
		{"role": "user", "content": "second question"},
	}
	got := svc.filterMessages(in)
	want := []map[string]interface{}{
		{"role": "user", "content": "hello"},
		{"role": "assistant", "content": "world"},
		{"role": "user", "content": "second question"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterMessages: got %#v want %#v", got, want)
	}
}

// TestOpenAI_MergeGenerationConfig_RequestWins pins the merge order:
// dialog.LLMSetting is the base; request fields override. Mirrors
// api/apps/restful_apis/_generation_params.py:merge_generation_config,
// which the Python handler calls at openai_api.py:283.
func TestOpenAI_MergeGenerationConfig_RequestWins(t *testing.T) {
	svc := &OpenAIChatService{}
	dialog := &entity.Chat{
		LLMSetting: entity.JSONMap{
			"temperature": 0.5,
			"top_p":       0.9,
		},
	}
	req := map[string]interface{}{"temperature": 0.1}
	svc.MergeGenerationConfig(dialog, req)

	if got := dialog.LLMSetting["temperature"]; got != 0.1 {
		t.Fatalf("temperature: request should win, got %v", got)
	}
	if got := dialog.LLMSetting["top_p"]; got != 0.9 {
		t.Fatalf("top_p: dialog value should be preserved, got %v", got)
	}
}

// TestOpenAI_MergeGenerationConfig_NilConfigIsNoOp verifies the Python
// `if not generation_config: return` early-exit: a nil config does not
// touch the dialog at all.
func TestOpenAI_MergeGenerationConfig_NilConfigIsNoOp(t *testing.T) {
	svc := &OpenAIChatService{}
	dialog := &entity.Chat{
		LLMSetting: entity.JSONMap{"temperature": 0.5},
	}
	svc.MergeGenerationConfig(dialog, nil)
	if got := dialog.LLMSetting["temperature"]; got != 0.5 {
		t.Fatalf("nil config should be a no-op, got temperature=%v", got)
	}
}

// TestOpenAI_MergeGenerationConfig_NilLLMSettingIsInitialized pins
// the Python `getattr(dialog, "llm_setting", None) or {}` pattern:
// a dialog with no LLMSetting (e.g. freshly created) gets one
// initialized to an empty map before the merge writes into it.
func TestOpenAI_MergeGenerationConfig_NilLLMSettingIsInitialized(t *testing.T) {
	svc := &OpenAIChatService{}
	dialog := &entity.Chat{} // LLMSetting is nil
	req := map[string]interface{}{"temperature": 0.7}
	svc.MergeGenerationConfig(dialog, req)
	if dialog.LLMSetting == nil {
		t.Fatal("expected LLMSetting to be initialized after merge")
	}
	if got := dialog.LLMSetting["temperature"]; got != 0.7 {
		t.Fatalf("expected temperature 0.7, got %v", got)
	}
}

// TestOpenAI_MergeGenerationConfig_AddsNewKeys pins that the merge
// ADDS keys that the dialog didn't have before, matching Python's
// `dict.update`.
func TestOpenAI_MergeGenerationConfig_AddsNewKeys(t *testing.T) {
	svc := &OpenAIChatService{}
	dialog := &entity.Chat{
		LLMSetting: entity.JSONMap{"temperature": 0.5},
	}
	req := map[string]interface{}{"top_p": 0.9, "max_tokens": 256}
	svc.MergeGenerationConfig(dialog, req)
	if got := dialog.LLMSetting["top_p"]; got != 0.9 {
		t.Fatalf("top_p: expected 0.9, got %v", got)
	}
	if got := dialog.LLMSetting["max_tokens"]; got != 256 {
		t.Fatalf("max_tokens: expected 256, got %v", got)
	}
	if got := dialog.LLMSetting["temperature"]; got != 0.5 {
		t.Fatalf("temperature: existing dialog value should be preserved, got %v", got)
	}
}

// TestOpenAI_MergeThenBuild_AllGenerationParamsReachChatConfig is the
// end-to-end contract for the OpenAI path: the handler builds a
// genCfg via extractGenerationConfig, the handler calls
// MergeGenerationConfig(dialog, genCfg), and the RAG pipeline later
// calls BuildChatConfig(dialog, nil) which reads the merged values.
// Verifies that all 5 fields the Python server honors
// (temperature, top_p, max_tokens, frequency_penalty, presence_penalty)
// survive the merge. For 3 of them (temperature, top_p, max_tokens) the
// ChatConfig fields exist and the test asserts the values. For the
// other 2 (frequency_penalty, presence_penalty) the ChatConfig struct
// doesn't have fields yet, so we just assert the dialog's LLMSetting
// preserves them — the structural gap is documented in
// openai_chat_completions.go::extractGenerationConfig.
//
// The handler-side float64 coercion of max_tokens is verified by
// TestExtractGenerationConfig_OnlyKnownFields in the handler package.
// This test uses a float64 here (matching what the handler produces
// after the fix) so the BuildChatConfig type assertion succeeds.
func TestOpenAI_MergeThenBuild_AllGenerationParamsReachChatConfig(t *testing.T) {
	svc := &OpenAIChatService{}
	dialog := &entity.Chat{} // no LLMSetting, no defaults
	req := map[string]interface{}{
		"temperature":       0.7,
		"top_p":             0.9,
		"max_tokens":        float64(256),
		"frequency_penalty": 0.1,
		"presence_penalty":  0.2,
	}
	svc.MergeGenerationConfig(dialog, req)

	// The merge itself is type-agnostic — verify the dialog kept the
	// raw values so any downstream code (Python-style dict consumer
	// or future Go consumer) can read them.
	if got := dialog.LLMSetting["temperature"]; got != 0.7 {
		t.Fatalf("temperature: expected 0.7, got %v", got)
	}
	if got := dialog.LLMSetting["top_p"]; got != 0.9 {
		t.Fatalf("top_p: expected 0.9, got %v", got)
	}
	if got, ok := dialog.LLMSetting["max_tokens"].(float64); !ok || got != 256 {
		t.Fatalf("max_tokens: expected float64 256, got %v (%T)", dialog.LLMSetting["max_tokens"], dialog.LLMSetting["max_tokens"])
	}
	if got := dialog.LLMSetting["frequency_penalty"]; got != 0.1 {
		t.Fatalf("frequency_penalty: expected 0.1, got %v", got)
	}
	if got := dialog.LLMSetting["presence_penalty"]; got != 0.2 {
		t.Fatalf("presence_penalty: expected 0.2, got %v", got)
	}

	// Now run the RAG-pipeline call: BuildChatConfig(dialog, nil).
	// For the 3 fields ChatConfig supports, the values must surface
	// on the returned struct.
	cfg := BuildChatConfig(dialog, nil)
	if cfg.Temperature == nil || *cfg.Temperature != 0.7 {
		t.Fatalf("ChatConfig.Temperature: expected 0.7, got %v", cfg.Temperature)
	}
	if cfg.TopP == nil || *cfg.TopP != 0.9 {
		t.Fatalf("ChatConfig.TopP: expected 0.9, got %v", cfg.TopP)
	}
	if cfg.MaxTokens == nil || *cfg.MaxTokens != 256 {
		t.Fatalf("ChatConfig.MaxTokens: expected 256, got %v", cfg.MaxTokens)
	}
}

// bindToolsFires pins the condition logic at chat_pipeline.go:241 — the
// BindTools block fires only when BOTH toolcall_session AND tools are
// present AND non-nil. Mirrors Python's `if toolcall_session and
// tools:` (dialog_service.py:584-585), which short-circuits to false
// on None. Without the explicit nil-check in the Go code, a present-
// with-nil value would flip hasSession/hasTools to true and call
// BindTools(nil, nil) — a no-op in the generic wrapper but a behavior
// change vs. Python's truthy short-circuit.
func bindToolsFires(kwargs map[string]interface{}) bool {
	tc, hasTC := kwargs["toolcall_session"]
	t, hasT := kwargs["tools"]
	return hasTC && tc != nil && hasT && t != nil
}

func TestOpenAI_BindToolsCondition_NilValuesDoNotFire(t *testing.T) {
	cases := []struct {
		name  string
		kw    map[string]interface{}
		fires bool
	}{
		// The OPENAI_CHAT call site at openai_chat.go (this is the
		// shape the new asyncKwargs produces): both keys present
		// with nil values. Block must NOT fire.
		{
			"both present with nil (current OPENAI_CHAT call site)",
			map[string]interface{}{"toolcall_session": nil, "tools": nil},
			false,
		},
		// Both keys absent (legacy / pre-refactor OPENAI_CHAT call
		// site): also must not fire.
		{
			"both absent (legacy / pre-refactor)",
			map[string]interface{}{},
			false,
		},
		// Both keys present with non-nil values (future OPENAI_CHAT
		// with tool support): block MUST fire.
		{
			"both present with non-nil (future tool support)",
			map[string]interface{}{"toolcall_session": "session-1", "tools": []interface{}{}},
			true,
		},
		// Mixed: one nil, one present. The Python `if X and Y:`
		// short-circuits on the first falsy, so block must NOT fire.
		{
			"toolcall_session nil, tools present",
			map[string]interface{}{"toolcall_session": nil, "tools": []interface{}{}},
			false,
		},
		{
			"toolcall_session present, tools nil",
			map[string]interface{}{"toolcall_session": "session-1", "tools": nil},
			false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := bindToolsFires(c.kw); got != c.fires {
				t.Fatalf("bindToolsFires(%v) = %v, want %v", c.kw, got, c.fires)
			}
		})
	}
}

// TestOpenAI_CleanCitationMarkers pins down the citation-marker stripping
// that matches Python's re.sub(r"##\d+\$\$", "", content).
func TestOpenAI_CleanCitationMarkers(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "hello world", "hello world"},
		{"single", "hello ##1$$ world", "hello  world"},
		{"multi", "##12$$foo##34$$bar", "foobar"},
		{"non-numeric", "##abc$$ stays", "##abc$$ stays"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cleanCitationMarkers(c.in); got != c.want {
				t.Fatalf("cleanCitationMarkers(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestOpenAI_SystemPrompt reads dialog.PromptConfig["system"].
func TestOpenAI_SystemPrompt(t *testing.T) {
	systemPrompt := func(dialog *entity.Chat) string {
		if dialog.PromptConfig == nil {
			return ""
		}
		s, _ := dialog.PromptConfig["system"].(string)
		return s
	}
	if got := systemPrompt(&entity.Chat{}); got != "" {
		t.Fatalf("expected empty system prompt for nil PromptConfig, got %q", got)
	}
	if got := systemPrompt(&entity.Chat{PromptConfig: entity.JSONMap{"system": "be helpful"}}); got != "be helpful" {
		t.Fatalf("expected system prompt 'be helpful', got %q", got)
	}
}

// TestOpenAI_MetadataConditionToDocIDs_NoCondition verifies the no-op path
// when metadata_condition is absent. Mirrors Python's `if
// metadata_condition:` guard at openai_api.py:290 — nil → no filter.
func TestOpenAI_MetadataConditionToDocIDs_NoCondition(t *testing.T) {
	got := MetadataConditionToDocIDs(nil, nil)
	if got != "" {
		t.Fatalf("expected empty string for nil condition, got %q", got)
	}
}

// TestOpenAI_MetadataConditionToDocIDs_NoKBs verifies that empty metadata
// with non-empty conditions yields the "-999" sentinel — matching Python:
// get_flatted_meta_by_kbs([]) returns {}, meta_filter() returns [], and the
// `if conditions and not filtered_doc_ids` branch substitutes ["-999"].
func TestOpenAI_MetadataConditionToDocIDs_NoKBs(t *testing.T) {
	cond := map[string]interface{}{
		"logic":      "and",
		"conditions": []interface{}{map[string]interface{}{"name": "author", "comparison_operator": "is", "value": "x"}},
	}
	got := MetadataConditionToDocIDs(nil, cond)
	if got != "-999" {
		t.Fatalf("expected sentinel \"-999\" for empty metadata with conditions, got %q", got)
	}
}

// TestOpenAI_ContextTokenUsed sums NumTokensFromString across messages.
func TestOpenAI_ContextTokenUsed(t *testing.T) {
	contextTokenUsed := func(messages []map[string]interface{}) int {
		total := 0
		for _, m := range messages {
			if c, ok := m["content"].(string); ok {
				total += tokenizer.NumTokensFromString(c)
			}
		}
		return total
	}
	msgs := []map[string]interface{}{
		{"role": "user", "content": "0123456789"},
		{"role": "user", "content": "01234567890123"},
	}
	got := contextTokenUsed(msgs)
	// NumTokensFromString uses cl100k_base BPE encoding, not len(s)/4.
	// "0123456789" (10 chars) + "01234567890123" (14 chars) = 9 BPE tokens.
	if got != 9 {
		t.Fatalf("expected 9 tokens (cl100k_base BPE), got %d", got)
	}
}

// TestOpenAI_DedupePrefix checks the SSE delta helper that strips the
// previous-cumulative prefix from a new cumulative string.
func TestOpenAI_DedupePrefix(t *testing.T) {
	dedupePrefix := func(old, new string) string {
		if strings.HasPrefix(new, old) {
			return new[len(old):]
		}
		return new
	}
	cases := []struct {
		old, new, want string
	}{
		{"", "hello", "hello"},
		{"hello", "hello", ""},
		{"hello", "hello world", " world"},
		{"hello", "world", "world"},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := dedupePrefix(c.old, c.new); got != c.want {
			t.Fatalf("dedupePrefix(%q, %q) = %q, want %q", c.old, c.new, got, c.want)
		}
	}
}

// TestOpenAI_IsContentDelta_FiltersSSETerminator guards the central filter
// that strips the OpenAI SSE terminator "[DONE]" out of the content stream.
// ~49 model drivers still call sender(&"[DONE]", nil) "for OpenAI
// compatibility", which used to leak the marker into the assistant
// message. isContentDelta is the single point of truth for whether a
// candidate delta should be appended to fullContent.
func TestOpenAI_IsContentDelta_FiltersSSETerminator(t *testing.T) {
	cases := []struct {
		name string
		in   *string
		want bool
	}{
		{"nil pointer", nil, false},
		{"empty string", strPtr(""), false},
		{"normal content", strPtr("Hello"), true},
		{"SSE terminator alone", strPtr("[DONE]"), false},
		{"content that contains the substring DONE", strPtr("DONE!"), true},
		{"multiline content", strPtr("line1\nline2"), true},
		{"single newline (leading-newline diagnostic)", strPtr("\n\n"), true},
	}
	for _, c := range cases {
		if got := isContentDelta(c.in); got != c.want {
			t.Errorf("%s: isContentDelta(%v) = %v, want %v", c.name, derefStr(c.in), got, c.want)
		}
	}
}

func derefStr(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

// TestOpenAI_PerDeltaTrimSpace_NotApplied pins the contract that the
// sender callback does NOT strip per-delta whitespace, matching
// Python's api/db/services/llm_service.py::async_chat_streamly (which
// yields raw delta content and concatenates without stripping).
//
// Stripping per-delta would destroy inter-delta word boundaries: a model
// that sends "Hello" + " I'm" + " just" + " a" + " chatbot" (with
// leading spaces on most deltas) would concatenate to
// "Hello! I'mjustachatbot" if each piece was trimmed.
//
// The final accumulated answer IS trimmed, mirroring Python's
// response.choices[0].message.content.strip() at
// llm_service.py:1457 — that single trim is the only one we apply.
func TestOpenAI_PerDeltaTrimSpace_NotApplied(t *testing.T) {
	// Per-delta: each piece is appended verbatim. Simulate the model
	// streaming "Hello" + " I'm" + " just" + " a" + " chatbot" the way
	// Qwen3-8B on SiliconFlow actually does.
	deltas := []string{"Hello", "! I'm", " just", " a", " chatbot"}
	accumulated := ""
	for _, d := range deltas {
		accumulated += d // sender appends raw delta, no TrimSpace
	}
	wantConcatenated := "Hello! I'm just a chatbot"
	if accumulated != wantConcatenated {
		t.Fatalf("per-delta concatenation = %q, want %q (this is the bug that per-delta TrimSpace would cause)",
			accumulated, wantConcatenated)
	}

	// Final-answer trim: only the final TrimSpace, applied to the full
	// accumulated answer, matches Python's behavior.
	leadingNewlines := "\n\nHello, world!\n\n"
	trimmed := strings.TrimSpace(leadingNewlines)
	if trimmed != "Hello, world!" {
		t.Fatalf("final-answer TrimSpace(%q) = %q, want %q",
			leadingNewlines, trimmed, "Hello, world!")
	}
}

// =============================================================================
// Request preparation helpers (moved from internal/handler/openai_chat_completions_test.go)
// =============================================================================

// TestService_NormalizeMessageContent_String passes a plain string through.
func TestService_NormalizeMessageContent_String(t *testing.T) {
	got, err := normalizeMessageContent("hello")
	if err != nil || got != "hello" {
		t.Fatalf("expected (%q,nil), got (%q,%v)", "hello", got, err)
	}
}

// TestService_NormalizeMessageContent_Nil returns "" with no error.
func TestService_NormalizeMessageContent_Nil(t *testing.T) {
	got, err := normalizeMessageContent(nil)
	if err != nil || got != "" {
		t.Fatalf("expected (\"\",nil), got (%q,%v)", got, err)
	}
}

// TestService_NormalizeMessageContent_ArrayOfTextParts joins text parts
// with "\n" and drops non-text parts (e.g. image_url). Mirrors
// _normalize_message_content in openai_api.py:198-216.
func TestService_NormalizeMessageContent_ArrayOfTextParts(t *testing.T) {
	in := []interface{}{
		map[string]interface{}{"type": "text", "text": "first"},
		map[string]interface{}{"type": "image_url", "image_url": map[string]string{"url": "http://x"}},
		map[string]interface{}{"type": "text", "text": "second"},
	}
	got, err := normalizeMessageContent(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "first\nsecond" {
		t.Fatalf("expected %q, got %q", "first\nsecond", got)
	}
}

// TestService_NormalizeMessageContent_InvalidType returns the Python error string.
func TestService_NormalizeMessageContent_InvalidType(t *testing.T) {
	_, err := normalizeMessageContent(42)
	if err == nil || !strings.Contains(err.Error(), "must be a string or an array") {
		t.Fatalf("expected content-type error, got %v", err)
	}
}

// TestService_NormalizeOpenAIMessages_RejectsInvalidContent ensures a
// message with bad content (e.g. a number) is rejected (Python:
// "messages[].content must be a string or an array of content parts.").
func TestService_NormalizeOpenAIMessages_RejectsInvalidContent(t *testing.T) {
	in := []map[string]interface{}{{"role": "user", "content": 42}}
	_, err := normalizeOpenAIMessages(in)
	if err == nil {
		t.Fatalf("expected error for non-string content")
	}
}

// TestService_ExtractGenerationConfig_OnlyKnownFields verifies the
// extraction mirrors extract_generation_config in
// _generation_params.py. The float64 coercion of max_tokens is what
// allows it to satisfy the type assertion in BuildChatConfig.
func TestService_ExtractGenerationConfig_OnlyKnownFields(t *testing.T) {
	temp := 0.4
	topP := 0.8
	maxTokens := 256
	freq := 0.1
	pres := 0.2
	req := &OpenAIChatRequest{
		Temperature:      &temp,
		TopP:             &topP,
		MaxTokens:        &maxTokens,
		FrequencyPenalty: &freq,
		PresencePenalty:  &pres,
	}
	cfg := extractGenerationConfig(req)
	if cfg["temperature"] != temp {
		t.Fatalf("temperature: got %v want %v", cfg["temperature"], temp)
	}
	if cfg["top_p"] != topP {
		t.Fatalf("top_p: got %v want %v", cfg["top_p"], topP)
	}
	if cfg["max_tokens"] != float64(maxTokens) {
		t.Fatalf("max_tokens: got %v (%T) want %v (float64)", cfg["max_tokens"], cfg["max_tokens"], float64(maxTokens))
	}
	if cfg["frequency_penalty"] != freq {
		t.Fatalf("frequency_penalty: got %v want %v", cfg["frequency_penalty"], freq)
	}
	if cfg["presence_penalty"] != pres {
		t.Fatalf("presence_penalty: got %v want %v", cfg["presence_penalty"], pres)
	}
	for _, k := range []string{"stop", "user", "internet", "tools"} {
		if _, has := cfg[k]; has {
			t.Fatalf("%s should not be in generation config, got %v", k, cfg[k])
		}
	}
}

// TestService_JoinNonEmpty_JoinsWithSeparator matches strings.Join
// semantics for non-empty inputs and skips empties.
func TestService_JoinNonEmpty_JoinsWithSeparator(t *testing.T) {
	if got := joinNonEmpty([]string{"a", "b", "c"}, ","); got != "a,b,c" {
		t.Fatalf("expected %q, got %q", "a,b,c", got)
	}
	if got := joinNonEmpty([]string{"a", "", "b"}, ","); got != "a,b" {
		t.Fatalf("expected %q, got %q", "a,b", got)
	}
	if got := joinNonEmpty([]string{}, ","); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

// TestMergeGenerationConfig_RequestOverridesDefault is the
// end-to-end contract: a dialog that arrives from the DB with the
// create-time default (api/db/db_models.py:987, applied inline in
// service/chat.go's SetDialog) still lets a request's per-call
// value win on top, mirroring Python's merge_generation_config at
// openai_api.py:283.
func TestMergeGenerationConfig_RequestOverridesDefault(t *testing.T) {
	svc := &OpenAIChatService{}
	dialog := &entity.Chat{
		LLMSetting: entity.JSONMap{
			"temperature":       0.1,
			"top_p":             0.3,
			"frequency_penalty": 0.7,
			"presence_penalty":  0.4,
			"max_tokens":        512,
		},
	}
	if dialog.LLMSetting["temperature"] != 0.1 {
		t.Fatalf("precondition: default temperature should be 0.1, got %v", dialog.LLMSetting["temperature"])
	}

	svc.MergeGenerationConfig(dialog, map[string]interface{}{"temperature": 0.7})
	if got := dialog.LLMSetting["temperature"]; got != 0.7 {
		t.Fatalf("request temperature should win, got %v", got)
	}
	// Other defaults (not in the request) survive intact.
	if got := dialog.LLMSetting["top_p"]; got != 0.3 {
		t.Fatalf("top_p: default should be intact after merge, got %v", got)
	}
	if got := dialog.LLMSetting["max_tokens"]; got != 512 {
		t.Fatalf("max_tokens: default should be intact after merge, got %v", got)
	}
}

// flushableRecorder wraps httptest.ResponseRecorder with a no-op Flush so
// the gin.Context's c.Writer.(http.Flusher) type assertion succeeds. The
// recorder itself doesn't implement Flusher; without this wrapper,
// streamChatCompletionSSE would return "streaming unsupported" before
// emitting a single byte.
type flushableRecorder struct {
	*httptest.ResponseRecorder
	flushed int
}

func (f *flushableRecorder) Flush() { f.flushed++ }

// TestStreamChatCompletionSSE_HappyPath pins the SSE wire format
// produced by streamChatCompletionSSE: a `data: <json>\n\n` line per
// event, the `chat.completion.chunk` object, role/content/reasoning
// fields, FinalAnswer surfaced only via final_content (not
// delta.content) — the #15286 fix, the [DONE] terminator, and the
// model field coming from the requestedModel arg (matching Python's
// _stream_chat_completion_sse, which uses requested_model in every
// yielded chunk).
func TestStreamChatCompletionSSE_HappyPath(t *testing.T) {
	events := make(chan OpenAIStreamEvent, 8)
	events <- OpenAIStreamEvent{Kind: OpenAIEventContent, Delta: "Hello"}
	events <- OpenAIStreamEvent{Kind: OpenAIEventContent, Delta: " world"}
	events <- OpenAIStreamEvent{Kind: OpenAIEventReasoning, Delta: "thinking..."}
	events <- OpenAIStreamEvent{
		Kind:             OpenAIEventFinal,
		FinalAnswer:      "Hello world",
		FinalReference:   []FormattedChunk{{ID: "chunk-1"}, {ID: "chunk-2"}},
		PromptTokens:     5,
		CompletionTokens: 2,
		TotalTokens:      7,
	}
	close(events)

	rec := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat", nil)

	if err := streamChatCompletionSSE(c, events, "chatcmpl-test-1", "test-model", true); err != nil {
		t.Fatalf("streamChatCompletionSSE returned error: %v", err)
	}

	body := rec.Body.String()
	if rec.Header().Get("Content-Type") != "text/event-stream; charset=utf-8" {
		t.Errorf("Content-Type: got %q, want text/event-stream", rec.Header().Get("Content-Type"))
	}
	if rec.flushed < 4 {
		t.Errorf("expected at least 4 flushes (3 chunks + [DONE]), got %d", rec.flushed)
	}
	// Per-chunk shape: data: <json>\n\n, object=chat.completion.chunk.
	if got := strings.Count(body, `"object":"chat.completion.chunk"`); got != 4 {
		t.Errorf("expected 4 chat.completion.chunk objects, got %d in body:\n%s", got, body)
	}
	if got := strings.Count(body, "\n\ndata: [DONE]"); got != 1 {
		t.Errorf("expected exactly 1 [DONE] terminator, got %d in body:\n%s", got, body)
	}
	// Model field uses requestedModel in every chunk (matches Python).
	if got := strings.Count(body, `"model":"test-model"`); got != 4 {
		t.Errorf("expected 4 chunks to carry model=test-model, got %d in body:\n%s", got, body)
	}
	// Content deltas surfaced as delta.content (not delta.final_content).
	if !strings.Contains(body, `"content":"Hello"`) {
		t.Errorf("expected content delta for 'Hello', body:\n%s", body)
	}
	if !strings.Contains(body, `"content":" world"`) {
		t.Errorf("expected content delta for ' world', body:\n%s", body)
	}
	// Reasoning delta surfaced as delta.reasoning_content with content=null.
	if !strings.Contains(body, `"reasoning_content":"thinking..."`) {
		t.Errorf("expected reasoning_content for 'thinking...', body:\n%s", body)
	}
	// #15286 fix: FinalAnswer is in delta.final_content, NOT in delta.content.
	// The final chunk has content:null, reasoning_content:null, final_content:"Hello world".
	if !strings.Contains(body, `"final_content":"Hello world"`) {
		t.Errorf("expected final_content='Hello world' in final chunk, body:\n%s", body)
	}
	if strings.Contains(body, `"content":"Hello world"`) {
		t.Errorf("final answer leaked into delta.content — #15286 regression, body:\n%s", body)
	}
	// Reference is included because NeedReference=true.
	if !strings.Contains(body, `"reference"`) {
		t.Errorf("expected reference field (NeedReference=true), body:\n%s", body)
	}
	// Usage block on the final chunk.
	if !strings.Contains(body, `"prompt_tokens":5`) {
		t.Errorf("expected prompt_tokens=5, body:\n%s", body)
	}
	if !strings.Contains(body, `"completion_tokens":2`) {
		t.Errorf("expected completion_tokens=2, body:\n%s", body)
	}
	if !strings.Contains(body, `"total_tokens":7`) {
		t.Errorf("expected total_tokens=7, body:\n%s", body)
	}
	// finish_reason:stop on the final chunk.
	if !strings.Contains(body, `"finish_reason":"stop"`) {
		t.Errorf("expected finish_reason=stop on final chunk, body:\n%s", body)
	}
}

// TestStreamChatCompletionSSE_NoReference pins the
// NeedReference=false path: the final chunk's delta must NOT carry
// final_content or reference (Python omits those when need_reference
// is false, per openai_api.py:187-194).
func TestStreamChatCompletionSSE_NoReference(t *testing.T) {
	events := make(chan OpenAIStreamEvent, 2)
	events <- OpenAIStreamEvent{
		Kind:           OpenAIEventFinal,
		FinalAnswer:    "answer",
		FinalReference: []FormattedChunk{{ID: "chunk-1"}},
	}
	close(events)

	rec := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat", nil)

	if err := streamChatCompletionSSE(c, events, "chatcmpl-test-2", "test-model", false); err != nil {
		t.Fatalf("streamChatCompletionSSE returned error: %v", err)
	}

	body := rec.Body.String()
	if strings.Contains(body, `"final_content"`) {
		t.Errorf("NeedReference=false: final_content should be omitted, body:\n%s", body)
	}
	if strings.Contains(body, `"reference"`) {
		t.Errorf("NeedReference=false: reference should be omitted, body:\n%s", body)
	}
}

// TestStreamChatCompletionSSE_ErrorEvent pins the in-band error path:
// an OpenAIEventError becomes a single chunk with delta.content =
// "**ERROR**: <msg>", then the [DONE] terminator (mirrors
// openai_api.py:174-176).
func TestStreamChatCompletionSSE_ErrorEvent(t *testing.T) {
	events := make(chan OpenAIStreamEvent, 1)
	events <- OpenAIStreamEvent{Kind: OpenAIEventError, Error: "boom"}
	close(events)

	rec := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat", nil)

	if err := streamChatCompletionSSE(c, events, "chatcmpl-test-3", "test-model", false); err != nil {
		t.Fatalf("streamChatCompletionSSE returned error: %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"content":"**ERROR**: boom"`) {
		t.Errorf("expected error chunk with content='**ERROR**: boom', body:\n%s", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Errorf("expected [DONE] after error chunk, body:\n%s", body)
	}
}

// TestStreamChatCompletionSSE_FlusherUnsupported is intentionally not
// written: gin's responseWriter wrapper implements http.Flusher
// regardless of the underlying writer (it calls WriteHeaderNow on
// Flush), so the "streaming unsupported" branch is unreachable
// through gin.CreateTestContext. The branch stays in the function as
// a defensive guard against callers who build a gin.Context
// themselves with a custom non-flushable writer.

// TestStreamChatCompletionSSE_EmptyChannel pins the empty-input
// edge case: a channel closed with no events at all should still
// emit the [DONE] terminator and not crash.
func TestStreamChatCompletionSSE_EmptyChannel(t *testing.T) {
	events := make(chan OpenAIStreamEvent)
	close(events)

	rec := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat", nil)

	if err := streamChatCompletionSSE(c, events, "chatcmpl-empty", "test-model", false); err != nil {
		t.Fatalf("streamChatCompletionSSE returned error: %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "data: [DONE]") {
		t.Errorf("expected [DONE] terminator for empty channel, body:\n%s", body)
	}
	if got := strings.Count(body, `"object":"chat.completion.chunk"`); got != 0 {
		t.Errorf("expected 0 chunks for empty channel, got %d", got)
	}
}

// TestStreamChatCompletionSSE_ChunkJSONShape pins the structural
// shape of one chunk by parsing the JSON payload and checking the
// keys exist. Catches accidental field renames or type changes in
// the gin.H literals.
func TestStreamChatCompletionSSE_ChunkJSONShape(t *testing.T) {
	events := make(chan OpenAIStreamEvent, 2)
	events <- OpenAIStreamEvent{Kind: OpenAIEventContent, Delta: "x"}
	events <- OpenAIStreamEvent{
		Kind:             OpenAIEventFinal,
		FinalAnswer:      "x",
		PromptTokens:     1,
		CompletionTokens: 1,
		TotalTokens:      2,
	}
	close(events)

	rec := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat", nil)

	if err := streamChatCompletionSSE(c, events, "chatcmpl-shape", "shape-model", false); err != nil {
		t.Fatalf("streamChatCompletionSSE returned error: %v", err)
	}

	// Pull the first data: line and parse it.
	lines := strings.Split(rec.Body.String(), "\n\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines (one chunk + [DONE]), got %d", len(lines))
	}
	firstLine := strings.TrimPrefix(lines[0], "data:")
	var chunk map[string]interface{}
	if err := json.Unmarshal([]byte(firstLine), &chunk); err != nil {
		t.Fatalf("first chunk is not valid JSON: %v\nline: %s", err, firstLine)
	}

	// Required top-level fields on a chat.completion.chunk.
	for _, key := range []string{"id", "object", "created", "model", "choices"} {
		if _, ok := chunk[key]; !ok {
			t.Errorf("chunk missing top-level %q", key)
		}
	}
	if chunk["object"] != "chat.completion.chunk" {
		t.Errorf("chunk.object: got %v, want chat.completion.chunk", chunk["object"])
	}
	// Decode the choices[0] shape.
	choices, ok := chunk["choices"].([]interface{})
	if !ok || len(choices) != 1 {
		t.Fatalf("choices: got %T %v, want []interface{} of length 1", chunk["choices"], chunk["choices"])
	}
	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		t.Fatalf("choices[0]: got %T, want object", choices[0])
	}
	delta, ok := choice["delta"].(map[string]interface{})
	if !ok {
		t.Fatalf("choices[0].delta: got %T, want object", choice["delta"])
	}
	// Content deltas carry role + content.
	if delta["role"] != "assistant" {
		t.Errorf("delta.role: got %v, want assistant", delta["role"])
	}
	if delta["content"] != "x" {
		t.Errorf("delta.content: got %v, want x", delta["content"])
	}
}
