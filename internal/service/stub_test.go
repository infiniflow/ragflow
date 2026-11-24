//go:build !ee

package service

import (
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
