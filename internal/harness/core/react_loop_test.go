package core

import (
	"context"
	"testing"

	"ragflow/internal/harness/core/schema"
)

func TestReActAgentCompletesOnLastIteration(t *testing.T) {
	tests := []struct {
		name           string
		maxIterations  int
		callsTool      bool
		returnDirectly bool
		wantOutput     string
	}{
		{
			name:          "final answer on first iteration",
			maxIterations: 1,
			wantOutput:    "Final answer",
		},
		{
			name:          "final answer after tool call",
			maxIterations: 2,
			callsTool:     true,
			wantOutput:    "Final answer",
		},
		{
			name:           "return directly on last iteration",
			maxIterations:  1,
			callsTool:      true,
			returnDirectly: true,
			wantOutput:     "mock result for search",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &forcedToolModel{
				toolCalls: []schema.ToolCall{{ID: "call-1", Function: schema.ToolCallFunction{Name: "search", Arguments: "{}"}}},
				finalResp: "Final answer",
				firstCall: tt.callsTool,
			}
			tool := &mockTool{name: "search", desc: "Search tool"}
			afterAgentCalls := 0
			middleware := &testMiddleware{
				afterAgent: func(ctx context.Context, _ *ReActAgentState) (context.Context, error) {
					afterAgentCalls++
					return ctx, nil
				},
			}
			agent := NewReActAgent(&ReActConfig[*schema.Message]{
				Model:          model,
				Tools:          []Tool{tool},
				MaxIterations:  tt.maxIterations,
				ReturnDirectly: map[string]bool{"search": tt.returnDirectly},
				OutputKey:      "answer",
				Middlewares:    []TypedReActMiddleware[*schema.Message]{middleware},
			})
			input := &AgentInput{Messages: []Message{schema.UserMessage("Search")}}
			ctx, _ := initRunCtx(t.Context(), "react", input)
			events := drainAgentEvents(t, agent.Run(ctx, input))
			for _, event := range events {
				if event.Err != nil {
					t.Errorf("successful completion emitted an error: %v", event.Err)
				}
			}
			if got := getSession(ctx).Values["answer"]; got != tt.wantOutput {
				t.Errorf("saved output = %v, want %q", got, tt.wantOutput)
			}
			if afterAgentCalls != 1 {
				t.Errorf("AfterAgent calls = %d, want 1", afterAgentCalls)
			}
		})
	}
}
