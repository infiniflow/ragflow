package core

import (
	"context"
	"testing"

	"ragflow/internal/harness/core/schema"
)

func BenchmarkReActAgent_ReActLoop(b *testing.B) {
	tool := &mockTool{name: "bench_tool", desc: "benchmark tool"}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		model := &mockModel{}
		model.addResp("tool")
		model.addResp("done")
		agent := NewReActAgent(&ReActConfig[*schema.Message]{
			Model: model,
			Tools: []Tool{tool},
			ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
		})
		agent.name = "bench"
		iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("bench")}})
		for {
			_, ok := iter.Next()
			if !ok {
				break
			}
		}
	}
}

func BenchmarkReActAgent_NoTools(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		model := &mockModel{}
		model.addResp("done")
		agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model})
		agent.name = "bench_nt"
		iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("hi")}})
		for {
			_, ok := iter.Next()
			if !ok {
				break
			}
		}
	}
}

func BenchmarkCancelContext_NewCancel(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cc := newCancelContext()
		cc.triggerCancel(CancelImmediate)
	}
}
