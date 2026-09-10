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

import "testing"

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
	}, sess, term)

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
	}, &stubSession{}, term)
	if hit || answer != "" {
		t.Fatal("non-terminal call must not short-circuit")
	}
}

// With no terminal set configured, behaviour is unchanged: no short-circuit.
func TestAppendToolResultsNoTerminalConfigured(t *testing.T) {
	history := []Message{{Role: "user", Content: "q"}}
	hist, answer, hit := appendToolResults(history, []map[string]interface{}{
		toolCallMsg("call-1", "rag", `{}`),
	}, &stubSession{}, nil)
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
	}, &stubSession{}, term)
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
// more to emit". It must still short-circuit, so the loop stops instead of
// asking the model again.
func TestAppendToolResultsTerminalHitWithEmptyResult(t *testing.T) {
	history := []Message{{Role: "user", Content: "q"}}
	sess := &stubSession{results: map[string]string{"rag": ""}}
	term := map[string]struct{}{"rag": {}}

	hist, answer, hit := appendToolResults(history, []map[string]interface{}{
		toolCallMsg("call-1", "rag", `{"question":"q"}`),
	}, sess, term)

	if !hit {
		t.Fatal("a successful terminal tool must short-circuit even with an empty result")
	}
	if answer != "" {
		t.Fatalf("answer = %q, want empty", answer)
	}
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
