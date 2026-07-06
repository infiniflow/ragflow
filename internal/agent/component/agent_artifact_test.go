package component

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

// artifactTool is an invokable tool that returns a JSON envelope with _ARTIFACTS.
type artifactTool struct {
	result string
}

func (t *artifactTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "artifact_tool",
		Desc: "returns artifacts",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"unused": {Type: schema.String},
		}),
	}, nil
}

func (t *artifactTool) InvokableRun(_ context.Context, _ string, _ ...tool.Option) (string, error) {
	return t.result, nil
}

// artifactModel is a scripted ToolCallingChatModel that emits one tool call
// and then a final answer.
type artifactModel struct {
	turn     int
	callID   string
	toolName string
	toolArgs string
	final    string
}

func (m *artifactModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.turn++
	if m.turn == 1 {
		return &schema.Message{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:   m.callID,
				Type: "function",
				Function: schema.FunctionCall{
					Name:      m.toolName,
					Arguments: m.toolArgs,
				},
			}},
		}, nil
	}
	return &schema.Message{Role: schema.Assistant, Content: m.final}, nil
}

func (m *artifactModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](1)
	sw.Close()
	return sr, nil
}

func (m *artifactModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func TestAgent_ReActAgent_CollectsArtifactsFromCodeExecTool(t *testing.T) {
	toolResult, err := json.Marshal(map[string]any{
		"tool_called": true,
		"message":     "CodeExec executed",
		"_ARTIFACTS": []map[string]any{
			{
				"name": "agent_artifact_bug_demo.png",
				"url":  "/api/v1/documents/artifact/1ae8d553478544628bb8be267d502371.png",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal tool result: %v", err)
	}

	opt, future := react.WithMessageFuture()
	agent, err := react.NewAgent(context.Background(), &react.AgentConfig{
		ToolCallingModel: &artifactModel{
			callID:   "call_1",
			toolName: "artifact_tool",
			toolArgs: `{"unused":"x"}`,
			final:    "The image has been generated.",
		},
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: []tool.BaseTool{&artifactTool{result: string(toolResult)}},
		},
		MaxStep: 3,
	})
	if err != nil {
		t.Fatalf("react.NewAgent: %v", err)
	}

	_, err = agent.Generate(context.Background(), []*schema.Message{
		schema.UserMessage("generate a test image"),
	}, opt)
	if err != nil {
		t.Fatalf("agent.Generate: %v", err)
	}

	// Drain the future so collectArtifactsFromToolCalls can iterate synchronously.
	msgs := drainFutureMessages(t, future)
	if len(msgs) == 0 {
		t.Fatal("MessageFuture produced no messages")
	}

	// Re-create the same sequence in a context and call the collector.
	fakeFuture := newSliceFuture(msgs)
	ctx := setArtifactCollector(context.Background(), fakeFuture)
	got := collectArtifactsFromToolCalls(ctx, nil)

	if len(got) != 1 {
		t.Fatalf("collected %d artifacts, want 1", len(got))
	}
	if got[0].Name != "agent_artifact_bug_demo.png" {
		t.Errorf("name=%q, want agent_artifact_bug_demo.png", got[0].Name)
	}
	if got[0].URL == "" {
		t.Error("artifact URL is empty")
	}

	md := formatArtifactMarkdown(got, "done")
	want := "![agent_artifact_bug_demo.png](/api/v1/documents/artifact/1ae8d553478544628bb8be267d502371.png)"
	if !strings.Contains(md, want) {
		t.Errorf("markdown=%q, want substring %q", md, want)
	}
}

func drainFutureMessages(t *testing.T, future react.MessageFuture) []*schema.Message {
	t.Helper()
	var out []*schema.Message
	iter := future.GetMessages()
	for {
		msg, ok, err := iter.Next()
		if err != nil {
			t.Fatalf("MessageFuture.Next: %v", err)
		}
		if !ok {
			break
		}
		out = append(out, msg)
	}
	return out
}

// sliceFuture is a react.MessageFuture backed by a slice.
type sliceFuture struct {
	iter *react.Iterator[*schema.Message]
}

func newSliceFuture(messages []*schema.Message) *sliceFuture {
	// Re-run a real react agent whose model returns the supplied messages as
	// tool-call / tool-response / final-answer sequence. This gives us a real
	// react.Iterator populated by eino's own callback plumbing.
	opt, future := react.WithMessageFuture()

	// Extract any tool name referenced by the replayed messages so the agent's
	// tool node can dispatch it.
	toolName := "passthrough"
	for _, m := range messages {
		for _, tc := range m.ToolCalls {
			if tc.Function.Name != "" {
				toolName = tc.Function.Name
			}
		}
	}

	model := &replayModel{messages: messages}
	agent, err := react.NewAgent(context.Background(), &react.AgentConfig{
		ToolCallingModel: model,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: []tool.BaseTool{&passthroughTool{name: toolName}},
		},
		MaxStep: len(messages) + 1,
	})
	if err != nil {
		panic(err)
	}
	_, err = agent.Generate(context.Background(), []*schema.Message{
		schema.UserMessage("replay"),
	}, opt)
	if err != nil {
		panic(err)
	}
	return &sliceFuture{iter: future.GetMessages()}
}

func (f *sliceFuture) GetMessages() *react.Iterator[*schema.Message] {
	return f.iter
}

func (f *sliceFuture) GetMessageStreams() *react.Iterator[*schema.StreamReader[*schema.Message]] {
	return nil
}

// replayModel replays a scripted sequence of messages on successive Generate calls.
type replayModel struct {
	messages []*schema.Message
	pos      int
}

func (m *replayModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	if m.pos >= len(m.messages) {
		return &schema.Message{Role: schema.Assistant, Content: "done"}, nil
	}
	msg := m.messages[m.pos]
	m.pos++
	return msg, nil
}

func (m *replayModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](1)
	sw.Close()
	return sr, nil
}

func (m *replayModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

// passthroughTool echoes its input as a tool result.
type passthroughTool struct {
	name string
}

func (t *passthroughTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: t.name,
		Desc: "echoes arguments",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"unused": {Type: schema.String},
		}),
	}, nil
}

func (t *passthroughTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	return argumentsInJSON, nil
}
