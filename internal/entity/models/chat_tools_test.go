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

package models

import (
	"context"
	"testing"

	"ragflow/internal/tokenizer"
)

// stubSession returns a canned result per tool name.
type stubSession struct {
	results map[string]string
}

func (s *stubSession) ToolCall(name string, _ map[string]interface{}) (string, error) {
	if r, ok := s.results[name]; ok {
		return r, nil
	}
	return "ok:" + name, nil
}

func toolCallMsg(id, name, args string) map[string]interface{} {
	return map[string]interface{}{
		"id": id,
		"function": map[string]interface{}{
			"name":      name,
			"arguments": args,
		},
	}
}

// A successful terminal tool short-circuits and surfaces its result.
func TestAppendToolResultsTerminalHit(t *testing.T) {
	history := []Message{{Role: "user", Content: "summarise onboarding"}}
	sess := &stubSession{results: map[string]string{"summarize_document": "[SUMMARY]"}}
	term := map[string]struct{}{"rag": {}, "summarize_document": {}}

	hist, answer, hit := appendToolResults(history, []map[string]interface{}{
		toolCallMsg("call-1", "summarize_document", `{"doc_id":"abc"}`),
	}, sess, term, false)

	if !hit {
		t.Fatal("terminal tool success must short-circuit")
	}
	if answer != "[SUMMARY]" {
		t.Fatalf("answer = %q, want the terminal result", answer)
	}
	// History must still carry the assistant tool_calls + tool result so it is
	// well-formed even though the caller ignores it on short-circuit.
	if len(hist) != 3 {
		t.Fatalf("history length = %d, want 3 (user + assistant + tool)", len(hist))
	}
}

// A failing terminal tool must NOT short-circuit.
func TestAppendToolResultsTerminalErrorDoesNotShortCircuit(t *testing.T) {
	history := []Message{{Role: "user", Content: "q"}}
	term := map[string]struct{}{"rag": {}}

	_, answer, hit := appendToolResults(history, []map[string]interface{}{
		toolCallMsg("call-1", "nonterminal", `{}`),
	}, &stubSession{}, term, false)
	if hit || answer != "" {
		t.Fatal("non-terminal call must not short-circuit")
	}
}

// With no terminal set configured, behaviour is unchanged: no short-circuit.
func TestAppendToolResultsNoTerminalConfigured(t *testing.T) {
	history := []Message{{Role: "user", Content: "q"}}
	hist, answer, hit := appendToolResults(history, []map[string]interface{}{
		toolCallMsg("call-1", "rag", `{}`),
	}, &stubSession{}, nil, false)
	if hit || answer != "" {
		t.Fatal("no terminal set must never short-circuit")
	}
	if len(hist) != 3 {
		t.Fatalf("history length = %d, want 3 (user + assistant + tool)", len(hist))
	}
}

// A non-terminal tool among terminal-configured set must not short-circuit.
func TestAppendToolResultsNonTerminalIgnoresSet(t *testing.T) {
	history := []Message{{Role: "user", Content: "q"}}
	term := map[string]struct{}{"rag": {}}
	_, answer, hit := appendToolResults(history, []map[string]interface{}{
		toolCallMsg("call-1", "retrieve", `{}`),
	}, &stubSession{}, term, false)
	if hit || answer != "" {
		t.Fatal("non-terminal tool must not short-circuit even with a terminal set present")
	}
}

// SetTerminalTools registers the names on the bound ToolConfig.
func TestSetTerminalToolsRegistersNames(t *testing.T) {
	cm := NewChatModel(&DummyModel{}, strPtr("m"), nil)
	cm.BindTools(&stubSession{}, []map[string]interface{}{})
	cm.SetTerminalTools("rag", "summarize_document")

	term := terminalSet(cm)
	if _, ok := term["rag"]; !ok {
		t.Fatal("rag must be terminal")
	}
	if _, ok := term["summarize_document"]; !ok {
		t.Fatal("summarize_document must be terminal")
	}
	if _, ok := term["retrieve"]; ok {
		t.Fatal("retrieve must not be terminal")
	}
}

// SetTerminalTools on a model with no bound tools is a no-op.
func TestSetTerminalToolsWithoutToolsIsNoop(t *testing.T) {
	cm := NewChatModel(&DummyModel{}, strPtr("m"), nil)
	cm.SetTerminalTools("rag") // must not panic
	if terminalSet(cm) != nil {
		t.Fatal("no ToolConfig means no terminal set")
	}
}

// A terminal tool that already streamed its answer returns "" to say "nothing
// more to emit". On the STREAMING path (emptyTerminalIsHit=true) it must still
// short-circuit, so the loop stops instead of asking the model again — the Go
// streaming tool's "" means "already delivered", unlike Python's rag tool,
// which returns the full text and whose all-empty fold falls through
// (chat_model.py:692-704).
func TestAppendToolResultsTerminalHitWithEmptyResult(t *testing.T) {
	history := []Message{{Role: "user", Content: "q"}}
	sess := &stubSession{results: map[string]string{"rag": ""}}
	term := map[string]struct{}{"rag": {}}

	hist, answer, hit := appendToolResults(history, []map[string]interface{}{
		toolCallMsg("call-1", "rag", `{"question":"q"}`),
	}, sess, term, true)

	if !hit {
		t.Fatal("a successful terminal tool must short-circuit even with an empty result (streaming contract)")
	}
	if answer != "" {
		t.Fatalf("answer = %q, want empty", answer)
	}
	if len(hist) != 3 {
		t.Fatalf("history length = %d, want 3 (user + assistant + tool)", len(hist))
	}
}

// questionKeyedSession returns a canned result per question argument, so two
// same-name tool calls can be told apart regardless of the goroutine order
// appendToolResults executes them in.
type questionKeyedSession struct {
	results map[string]string
}

func (s *questionKeyedSession) ToolCall(_ string, args map[string]interface{}) (string, error) {
	if q, ok := args["question"].(string); ok {
		if r, ok := s.results[q]; ok {
			return r, nil
		}
	}
	return "", nil
}

// TestAppendToolResultsSkipsEmptyTerminalForNonEmptySibling pins the fold
// alignment with Python chat_model.py:696 (`if out:`): an EMPTY (or
// whitespace-only) terminal result never ships while a later non-empty
// sibling exists — the second call's answer is the final one.
func TestAppendToolResultsSkipsEmptyTerminalForNonEmptySibling(t *testing.T) {
	history := []Message{{Role: "user", Content: "q"}}
	sess := &questionKeyedSession{results: map[string]string{
		"first":  "",
		"second": "  \n\t",
		"third":  "103 years",
	}}
	term := map[string]struct{}{"rag": {}}

	_, answer, hit := appendToolResults(history, []map[string]interface{}{
		toolCallMsg("call-1", "rag", `{"question":"first"}`),
		toolCallMsg("call-2", "rag", `{"question":"second"}`),
		toolCallMsg("call-3", "rag", `{"question":"third"}`),
	}, sess, term, false)

	if !hit {
		t.Fatal("the non-empty terminal sibling must short-circuit")
	}
	if answer != "103 years" {
		t.Fatalf("answer = %q, want the non-empty sibling's result", answer)
	}
}

// TestAppendToolResultsAllEmptyTerminalFallsThrough pins the non-streaming
// all-empty case against Python chat_model.py:692-704 (no else branch: an
// unqualified terminal result is skipped, the tool responses land in history
// and the loop runs another model round): hit=false so the caller re-invokes
// the model instead of shipping a blank answer.
func TestAppendToolResultsAllEmptyTerminalFallsThrough(t *testing.T) {
	history := []Message{{Role: "user", Content: "q"}}
	sess := &stubSession{results: map[string]string{"rag": ""}}
	term := map[string]struct{}{"rag": {}}

	hist, answer, hit := appendToolResults(history, []map[string]interface{}{
		toolCallMsg("call-1", "rag", `{"question":"q"}`),
	}, sess, term, false)

	if hit || answer != "" {
		t.Fatal("all-empty terminal results must fall through to the next model round")
	}
	// History still carries the assistant tool_calls + tool result, well-formed
	// for the next round.
	if len(hist) != 3 {
		t.Fatalf("history length = %d, want 3 (user + assistant + tool)", len(hist))
	}
}

// sendTerminal must stay silent for an empty answer, so a streamed terminal
// result is not emitted a second time by the short-circuit.
func TestSendTerminalEmptyAnswerIsSilent(t *testing.T) {
	var got []string
	sender := func(delta *string, _ *string) error {
		if delta != nil {
			got = append(got, *delta)
		}
		return nil
	}
	empty := ""
	if err := sendTerminal(sender, &empty); err != nil {
		t.Fatalf("sendTerminal: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("streamed %v, want nothing for an empty terminal answer", got)
	}
}

// TestChatWithToolsCountsTerminalRoundUsageOnce pins that a terminal-tool round
// contributes its tokens exactly once, in both the returned total and the
// run-usage sink (whose Add accumulates): runToolLoop folds the round in before
// the short-circuit decides.
func TestChatWithToolsCountsTerminalRoundUsageOnce(t *testing.T) {
	const (
		answer = "[SUMMARY]"
		total  = 15
	)
	driver := &captureToolDriver{resp: &ChatResponse{
		ToolCalls: []map[string]interface{}{toolCallMsg("call-1", "summarize_document", `{"doc_id":"abc"}`)},
		Usage:     &TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: total},
	}}
	modelName, apiKey := "m", "k"
	cm := NewChatModel(driver, &modelName, &APIConfig{ApiKey: &apiKey})
	cm.ToolConfig = &ToolConfig{
		Tools:           `[{"type":"function","function":{"name":"summarize_document"}}]`,
		ToolCallSession: &stubSession{results: map[string]string{"summarize_document": answer}},
		TerminalTools:   map[string]struct{}{"summarize_document": {}},
	}

	ctx := tokenizer.WithRunUsage(context.Background())
	got, tokens, err := cm.ChatWithTools(ctx, "", []Message{{Role: "user", Content: "q"}}, &ChatConfig{})
	if err != nil {
		t.Fatalf("ChatWithTools: %v", err)
	}
	if got != answer {
		t.Fatalf("answer = %q, want the terminal result %q", got, answer)
	}
	if tokens != total {
		t.Errorf("totalTokens = %d, want %d (the terminal round must be counted once)", tokens, total)
	}
	if _, _, runTotal, calls := tokenizer.GetRunUsage(ctx).Snapshot(); runTotal != total || calls != 1 {
		t.Errorf("run usage total=%d calls=%d, want total=%d calls=1", runTotal, calls, total)
	}
}
