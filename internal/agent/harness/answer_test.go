package harness

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"

	"ragflow/internal/agent/component"
)

// TestFormalizeAnswer_Abstain asserts abstain short-circuits to the abstain
// message without calling the model.
func TestFormalizeAnswer_Abstain(t *testing.T) {
	res := FormalizeAnswer(t.Context(), nil, "Q", &Kbinfos{}, false, true, false, "", false, "")
	if !res.Abstained || res.FinalAnswer != abstainMessage {
		t.Errorf("abstain result = %+v, want abstain message", res)
	}
}

// TestFormalizeAnswer_Empty asserts empty chunks short-circuit to the empty
// message.
func TestFormalizeAnswer_Empty(t *testing.T) {
	res := FormalizeAnswer(t.Context(), nil, "Q", &Kbinfos{}, false, false, false, "", false, "")
	if !res.Empty || res.FinalAnswer != emptyResultMessage {
		t.Errorf("empty result = %+v, want empty message", res)
	}
}

// TestFormalizeAnswer_Generates asserts a chat invoker is used to compose the
// final answer from the evidence.
func TestFormalizeAnswer_Generates(t *testing.T) {
	installChat(t, "here is the final answer")
	kb := &Kbinfos{Chunks: []map[string]interface{}{{"content_with_weight": "evidence alpha"}}}
	res := FormalizeAnswer(t.Context(), nil, "What is X?", kb, false, false, false, "", false, "")
	if res.FinalAnswer != "here is the final answer" {
		t.Errorf("final answer = %q, want chat output", res.FinalAnswer)
	}
}

// TestFormalizeAnswer_PartialPreamble asserts the partial preamble is included
// when partial=true.
func TestFormalizeAnswer_PartialPreamble(t *testing.T) {
	installChat(t, "partial ans")
	kb := &Kbinfos{Chunks: []map[string]interface{}{{"content_with_weight": "evidence"}}}
	res := FormalizeAnswer(t.Context(), nil, "Q", kb, true, false, false, "", false, "")
	if !strings.Contains(res.FinalAnswer, "partial ans") {
		t.Errorf("final answer = %q", res.FinalAnswer)
	}
}

// TestFormalizeAnswer_ForceLLM asserts forceLLM skips the empty short-circuit and
// still calls the model, so the FALLBACK_LLM verdict reaches the direct LLM.
func TestFormalizeAnswer_ForceLLM(t *testing.T) {
	installChat(t, "direct llm fallback answer")
	res := FormalizeAnswer(t.Context(), nil, "Q", &Kbinfos{}, false, false, true, "", true, "")
	if res.Empty {
		t.Error("forceLLM must not return the empty short-circuit")
	}
	if res.FinalAnswer != "direct llm fallback answer" {
		t.Errorf("final answer = %q, want chat output (direct LLM invoked)", res.FinalAnswer)
	}
}

// TestRunAgenticRAG_LowMode asserts low mode does a single direct search and
// produces an answer.
func TestRunAgenticRAG_LowMode(t *testing.T) {
	installChat(t, "final composed answer")
	res := RunAgenticRAG(t.Context(), nil, "What is a rocket?", "", "low",
		func(_ context.Context, _, _ string) ([]map[string]interface{}, []map[string]interface{}) {
			return []map[string]interface{}{{"chunk_id": "a", "content_with_weight": "rocket evidence"}}, nil
		}, "")
	if res.FinalAnswer != "final composed answer" {
		t.Errorf("final answer = %q", res.FinalAnswer)
	}
}

// TestRunAgenticRAG_EmptySearch asserts an empty search yields the empty message.
func TestRunAgenticRAG_EmptySearch(t *testing.T) {
	res := RunAgenticRAG(t.Context(), nil, "Q", "", "low",
		func(_ context.Context, _, _ string) ([]map[string]interface{}, []map[string]interface{}) {
			return nil, nil
		}, "")
	if !res.Empty || res.FinalAnswer != emptyResultMessage {
		t.Errorf("empty result = %+v", res)
	}
}

// recordingChatInvoker records the system prompt it receives and returns a fixed
// answer, so tests can assert the final-answer system message is composed
// correctly.
type recordingChatInvoker struct {
	content string
	system  string
}

func (r *recordingChatInvoker) Invoke(_ context.Context, _ *gorm.DB, req component.ChatInvokeRequest) (*component.ChatInvokeResponse, error) {
	for _, m := range req.Messages {
		if m.Role == schema.System {
			r.system = m.Content
			break
		}
	}
	return &component.ChatInvokeResponse{Content: r.content}, nil
}

// TestFormalizeAnswer_SystemPromptPrepended asserts that a non-empty dialog
// system prompt is prepended to the final-answer system prompt.
func TestFormalizeAnswer_SystemPromptPrepended(t *testing.T) {
	inv := &recordingChatInvoker{content: "final composed answer"}
	component.SetDefaultChatInvoker(inv)
	t.Cleanup(func() { component.SetDefaultChatInvoker(nil) })

	kb := &Kbinfos{Chunks: []map[string]interface{}{{"content_with_weight": "evidence"}}}
	res := FormalizeAnswer(t.Context(), nil, "Q", kb, false, false, false, "", false, "Be concise and factual.")
	if res.FinalAnswer != "final composed answer" {
		t.Errorf("final answer = %q, want chat output", res.FinalAnswer)
	}
	if inv.system == "" {
		t.Fatal("system prompt was not sent to the chat invoker")
	}
	if !strings.Contains(inv.system, "Be concise and factual.") {
		t.Errorf("system prompt does not contain the dialog prompt: %q", inv.system)
	}
	if !strings.Contains(inv.system, "You are a smart agent") {
		t.Errorf("system prompt does not contain the default final-answer prompt: %q", inv.system)
	}
}
