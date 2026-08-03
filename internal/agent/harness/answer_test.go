package harness

import (
	"context"
	"strings"
	"testing"
)

// TestFormalizeAnswer_Abstain asserts abstain short-circuits to the abstain
// message without calling the model.
func TestFormalizeAnswer_Abstain(t *testing.T) {
	res := FormalizeAnswer(context.Background(), nil, "Q", &Kbinfos{}, false, true, false)
	if !res.Abstained || res.FinalAnswer != abstainMessage {
		t.Errorf("abstain result = %+v, want abstain message", res)
	}
}

// TestFormalizeAnswer_Empty asserts empty chunks short-circuit to the empty
// message.
func TestFormalizeAnswer_Empty(t *testing.T) {
	res := FormalizeAnswer(context.Background(), nil, "Q", &Kbinfos{}, false, false, false)
	if !res.Empty || res.FinalAnswer != emptyResultMessage {
		t.Errorf("empty result = %+v, want empty message", res)
	}
}

// TestFormalizeAnswer_Generates asserts a chat invoker is used to compose the
// final answer from the evidence.
func TestFormalizeAnswer_Generates(t *testing.T) {
	installChat(t, "here is the final answer")
	kb := &Kbinfos{Chunks: []map[string]interface{}{{"content_with_weight": "evidence alpha"}}}
	res := FormalizeAnswer(context.Background(), nil, "What is X?", kb, false, false, false)
	if res.FinalAnswer != "here is the final answer" {
		t.Errorf("final answer = %q, want chat output", res.FinalAnswer)
	}
}

// TestFormalizeAnswer_PartialPreamble asserts the partial preamble is included
// when partial=true.
func TestFormalizeAnswer_PartialPreamble(t *testing.T) {
	installChat(t, "partial ans")
	kb := &Kbinfos{Chunks: []map[string]interface{}{{"content_with_weight": "evidence"}}}
	res := FormalizeAnswer(context.Background(), nil, "Q", kb, true, false, false)
	if !strings.Contains(res.FinalAnswer, "partial ans") {
		t.Errorf("final answer = %q", res.FinalAnswer)
	}
}

// TestRunAgenticRAG_LowMode asserts low mode does a single direct search and
// produces an answer.
func TestRunAgenticRAG_LowMode(t *testing.T) {
	installChat(t, "final composed answer")
	res := RunAgenticRAG(context.Background(), nil, "What is a rocket?", "", "low",
		func(_ context.Context, _, _ string) ([]map[string]interface{}, []map[string]interface{}) {
			return []map[string]interface{}{{"chunk_id": "a", "content_with_weight": "rocket evidence"}}, nil
		})
	if res.FinalAnswer != "final composed answer" {
		t.Errorf("final answer = %q", res.FinalAnswer)
	}
}

// TestRunAgenticRAG_EmptySearch asserts an empty search yields the empty message.
func TestRunAgenticRAG_EmptySearch(t *testing.T) {
	res := RunAgenticRAG(context.Background(), nil, "Q", "", "low",
		func(_ context.Context, _, _ string) ([]map[string]interface{}, []map[string]interface{}) {
			return nil, nil
		})
	if !res.Empty || res.FinalAnswer != emptyResultMessage {
		t.Errorf("empty result = %+v", res)
	}
}
