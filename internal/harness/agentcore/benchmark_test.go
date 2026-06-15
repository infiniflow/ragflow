package agentcore

import (
	"context"
	"testing"

	"ragflow/internal/harness/agentcore/schema"
)

func BenchmarkReActAgent_ReActLoop(b *testing.B) {
	model := &mockModel{}
	tool := &mockTool{name: "bench_tool", desc: "benchmark tool"}
	model.addResp("tool")
	model.addResp("done")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model,
		Tools: []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
	})
	agent.name = "bench"
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
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
	model := &mockModel{}
	model.addResp("done")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model})
	agent.name = "bench_nt"
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
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
