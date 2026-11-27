//go:build !ee

package service

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/entity"
)

func TestCompletionLogHooksAreNoops(t *testing.T) {
	chatResults := make(chan AsyncChatResult)
	if got := logChatCompletion(t.Context(), &entity.Chat{}, nil, true, chatResults); got != chatResults {
		t.Fatal("community chat logging changed the result channel")
	}
	agentResults := make(chan canvas.RunEvent)
	if got := logAgentCompletion(t.Context(), "agent", "session", "private message", agentResults); got != agentResults {
		t.Fatal("community agent logging changed the result channel")
	}
}

func TestOutputFilterHooksAreNoops(t *testing.T) {
	ctx := t.Context()
	if got, err := prepareAgentOutput(ctx); got != ctx || err != nil {
		t.Fatalf("prepareAgentOutput changed the context: %v", err)
	}
	chatResults := make(chan AsyncChatResult)
	if got := (&ChatPipelineService{}).filterChatOutput(ctx, &entity.Chat{}, true, chatResults); got != chatResults {
		t.Fatal("community output filtering changed the chat channel")
	}
	askResults := make(chan AskDelta)
	if got := filterAskOutput(ctx, askResults); got != askResults {
		t.Fatal("community output filtering changed the search channel")
	}
	reference := map[string]any{"content": "secret"}
	answer, thinking, refs := filterAgentOutput(ctx, "secret", "secret", reference)
	if answer != "secret" || thinking != "secret" || refs["content"] != "secret" {
		t.Fatal("community output filtering changed the agent reply")
	}
	_, _, refs = filterAgentOutput(ctx, "", "", nil)
	if refs != nil {
		t.Fatal("community output filtering changed a nil reference")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if got, err := prepareAgentOutput(canceled); got != canceled || !errors.Is(err, context.Canceled) {
		t.Fatalf("prepareAgentOutput lost cancellation: %v", err)
	}
}
