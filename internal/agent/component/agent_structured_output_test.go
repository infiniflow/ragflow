package component

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/chat"
	"ragflow/internal/agent/runtime"
)

func agentCountOutputSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{"count": map[string]any{"type": "integer"}},
		"required":   []any{"count"}, "additionalProperties": false,
	}
}

func TestAgentStructuredOutputValidatesBeforePublishing(t *testing.T) {
	for _, tools := range [][]string{nil, {"execute_sql"}} {
		name := "without_tools"
		if len(tools) > 0 {
			name = "with_tools"
		}
		t.Run(name, func(t *testing.T) {
			runnerCalls := 0
			withAgentRunner(t, func(ctx context.Context, p AgentParam) (*schema.Message, error) {
				runnerCalls++
				if !strings.Contains(p.SystemPrompt, "count") || len(p.Tools) != len(tools) {
					t.Fatalf("runner parameters = %#v", p)
				}
				runtime.EmitAgentMessage(ctx, `{"count":"bad"}`, "unvalidated reasoning")
				return &schema.Message{Role: schema.Assistant, Content: `{"count":"bad"}`}, nil
			})
			stub := &stubInvoker{resp: &ChatInvokeResponse{Content: `{"count":7}`}}
			withStubInvoker(t, stub)
			var emitted strings.Builder
			ctx := runtime.WithAgentMessageEmitter(t.Context(), func(content, thinking string) {
				emitted.WriteString(content + thinking)
			})
			ctx = runtime.WithComponentExecutionOptions(ctx, runtime.ComponentExecutionOptions{DeferAgentToMessage: true})
			c := NewAgentComponent(AgentParam{
				ModelID: "stub", Tools: tools, OutputStructure: agentCountOutputSchema(), MaxRetries: 1,
			})
			out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "count rows"})
			if err != nil {
				t.Fatal(err)
			}
			if runnerCalls != 1 || stub.calls != 1 {
				t.Fatalf("runner calls=%d, correction calls=%d", runnerCalls, stub.calls)
			}
			if stub.captured.Tools != nil || stub.captured.ToolChoice != chat.ToolChoiceNone {
				t.Fatalf("correction exposes tools: %#v", stub.captured)
			}
			if stub.captured.ModelName != "stub" {
				t.Fatalf("correction changed model: %#v", stub.captured)
			}
			if _, ok := out["structured"].(map[string]any); !ok {
				t.Fatalf("missing structured output: %#v", out)
			}
			if out["content"] != `{"count":7}` || emitted.String() != `{"count":7}` {
				t.Fatalf("out=%#v, emitted=%q", out, emitted.String())
			}
		})
	}
}

func TestAgentStructuredOutputFailureDoesNotPublish(t *testing.T) {
	for _, retries := range []int{0, 2} {
		t.Run(fmt.Sprint(retries), func(t *testing.T) {
			withAgentRunner(t, func(ctx context.Context, _ AgentParam) (*schema.Message, error) {
				runtime.EmitAgentMessage(ctx, "unvalidated partial", "")
				return &schema.Message{Role: schema.Assistant, Content: `{"count":"bad"}`}, nil
			})
			stub := &stubInvoker{resp: &ChatInvokeResponse{Content: `{"count":"still bad"}`}}
			withStubInvoker(t, stub)
			var emitted strings.Builder
			ctx := runtime.WithAgentMessageEmitter(t.Context(), func(content, thinking string) {
				emitted.WriteString(content + thinking)
			})
			c := NewAgentComponent(AgentParam{ModelID: "stub", OutputStructure: agentCountOutputSchema(), MaxRetries: retries})
			out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "count rows"})
			if err == nil || out != nil || emitted.Len() != 0 || stub.calls != retries {
				t.Fatalf("out=%#v err=%v emitted=%q corrections=%d", out, err, emitted.String(), stub.calls)
			}
		})
	}
}

func TestAgentStructuredOutputCorrectionDoesNotRepeatRealReActTool(t *testing.T) {
	model := &textThenToolCallModel{}
	tool := &passthroughTool{name: "execute_code"}
	withAgentRunner(t, func(ctx context.Context, _ AgentParam) (*schema.Message, error) {
		agent, err := react.NewAgent(ctx, &react.AgentConfig{
			ToolCallingModel: model,
			ToolsConfig:      compose.ToolsNodeConfig{Tools: []einotool.BaseTool{tool}},
			MaxStep:          4,
		})
		if err != nil {
			return nil, err
		}
		return agent.Generate(ctx, []*schema.Message{schema.UserMessage("count")})
	})
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: `{"count":2}`}}
	withStubInvoker(t, stub)
	c := NewAgentComponent(AgentParam{ModelID: "stub", OutputStructure: agentCountOutputSchema(), MaxRetries: 1})
	out, err := c.Invoke(t.Context(), nil, map[string]any{"user_prompt": "count"})
	if err != nil {
		t.Fatal(err)
	}
	if tool.calls.Load() != 1 || stub.calls != 1 || out["content"] != `{"count":2}` {
		t.Fatalf("tool calls=%d corrections=%d out=%#v", tool.calls.Load(), stub.calls, out)
	}
}

type structuredTranscriptModel struct {
	turn  int
	final string
}

func correctionToolObservations(t *testing.T, req ChatInvokeRequest) []schema.Message {
	t.Helper()
	if req.Tools != nil || req.ToolChoice != chat.ToolChoiceNone {
		t.Fatalf("correction exposes executable tools: %#v", req)
	}
	for _, message := range req.Messages {
		if message.Role == schema.Tool || len(message.ToolCalls) > 0 || message.ToolCallID != "" {
			t.Fatalf("correction contains protocol tool messages: %#v", message)
		}
		if message.Role == schema.User && strings.HasPrefix(message.Content, "ReAct tool observations ") {
			_, text, _ := strings.Cut(message.Content, "\n")
			var observations []schema.Message
			if err := json.Unmarshal([]byte(text), &observations); err != nil {
				t.Fatal(err)
			}
			return observations
		}
	}
	t.Fatal("correction lost tool observations")
	return nil
}

func (m *structuredTranscriptModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func (m *structuredTranscriptModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("unexpected non-streaming model call")
}

func (m *structuredTranscriptModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.turn++
	if m.turn == 1 {
		index := 0
		return schema.StreamReaderFromArray([]*schema.Message{
			{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{Index: &index, ID: "count-call", Type: "function",
				Function: schema.FunctionCall{Name: "count_rows", Arguments: `{"count":`}}}},
			{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{Index: &index,
				Function: schema.FunctionCall{Arguments: `7,"source":"inventory"}`}}}},
		}), nil
	}
	return schema.StreamReaderFromArray([]*schema.Message{{Role: schema.Assistant, Content: m.final}}), nil
}

func TestAgentStructuredOutputCorrectionPreservesStreamedToolContext(t *testing.T) {
	for _, answer := range []string{`{}`, `{"count":"unknown"}`} {
		t.Run(answer, func(t *testing.T) {
			mdl := &structuredTranscriptModel{final: answer}
			tool := &passthroughTool{name: "count_rows"}
			withAgentRunner(t, func(ctx context.Context, p AgentParam) (*schema.Message, error) {
				agent, err := react.NewAgent(ctx, &react.AgentConfig{
					ToolCallingModel:      mdl,
					ToolsConfig:           compose.ToolsNodeConfig{Tools: []einotool.BaseTool{tool}},
					MessageModifier:       agentMessageModifier(ctx, p),
					StreamToolCallChecker: scanAllStreamForToolCall, MaxStep: 4,
				})
				if err != nil {
					return nil, err
				}
				opt, future := react.WithMessageFuture()
				ctx = setArtifactCollector(ctx, future)
				done := emitAgentModelStreams(ctx, future, true)
				stream, err := agent.Stream(ctx, buildAgentInputMessages(ctx, p), opt)
				if err != nil {
					return nil, err
				}
				message, err := schema.ConcatMessageStream(stream)
				if emitErr := <-done; err == nil {
					err = emitErr
				}
				return message, err
			})
			corrections := 0
			withStubInvoker(t, structuredRepairInvoker(func(_ context.Context, req ChatInvokeRequest) (*ChatInvokeResponse, error) {
				corrections++
				observations := correctionToolObservations(t, req)
				if len(observations) != 2 || len(observations[0].ToolCalls) != 1 {
					t.Fatalf("unexpected tool observations: %#v", observations)
				}
				call, result := observations[0].ToolCalls[0], observations[1]
				if call.ID != "count-call" || call.Function.Name != "count_rows" || call.Function.Arguments != `{"count":7,"source":"inventory"}` ||
					result.Role != schema.Tool || result.ToolCallID != call.ID || result.Content != call.Function.Arguments {
					t.Fatalf("correction changed tool evidence: %#v", observations)
				}
				if corrections == 1 {
					return &ChatInvokeResponse{Content: `{}`}, nil
				}
				return &ChatInvokeResponse{Content: `{"count":7}`}, nil
			}))
			c := NewAgentComponent(AgentParam{ModelID: "stub", OutputStructure: agentCountOutputSchema(), MaxRetries: 2})
			out, err := c.Invoke(t.Context(), nil, map[string]any{"user_prompt": "count inventory rows"})
			if err != nil || out["content"] != `{"count":7}` || corrections != 2 || mdl.turn != 2 || tool.calls.Load() != 1 {
				t.Fatalf("out=%#v err=%v corrections=%d model turns=%d tool calls=%d", out, err, corrections, mdl.turn, tool.calls.Load())
			}
		})
	}
}

func TestAgentStructuredOutputCorrectionPreservesAllToolTextParts(t *testing.T) {
	call := schema.AssistantMessage("checking inventory", []schema.ToolCall{
		{ID: "count", Function: schema.FunctionCall{Name: "count_rows", Arguments: `{}`}},
		{ID: "source", Function: schema.FunctionCall{Name: "get_source", Arguments: `{}`}},
	})
	result := &schema.Message{Role: schema.Tool, ToolCallID: "count", UserInputMultiContent: []schema.MessageInputPart{
		{Type: schema.ChatMessagePartTypeText, Text: `{"count":7}`},
		{Type: schema.ChatMessagePartTypeText, Text: "inventory snapshot 42"},
	}, Extra: map[string]any{"runtime_only": make(chan int)}}
	withAgentRunner(t, func(ctx context.Context, p AgentParam) (*schema.Message, error) {
		modify := agentMessageModifier(ctx, p)
		modify(ctx, []*schema.Message{schema.UserMessage(p.UserPrompt), call, result})
		modify(ctx, []*schema.Message{schema.UserMessage(p.UserPrompt), call, result, schema.ToolMessage("warehouse A", "source")})
		return schema.AssistantMessage(`{}`, nil), nil
	})
	withStubInvoker(t, structuredRepairInvoker(func(_ context.Context, req ChatInvokeRequest) (*ChatInvokeResponse, error) {
		observations := correctionToolObservations(t, req)
		if len(observations) != 3 || len(observations[0].ToolCalls) != 2 || len(observations[1].UserInputMultiContent) != 2 ||
			observations[1].UserInputMultiContent[0].Text != `{"count":7}` || observations[1].UserInputMultiContent[1].Text != "inventory snapshot 42" ||
			observations[2].ToolCallID != "source" || observations[2].Content != "warehouse A" {
			t.Fatalf("incomplete or duplicated observations: %#v", observations)
		}
		return &ChatInvokeResponse{Content: `{"count":7}`}, nil
	}))
	c := NewAgentComponent(AgentParam{ModelID: "stub", OutputStructure: agentCountOutputSchema(), MaxRetries: 1})
	if _, err := c.Invoke(t.Context(), nil, map[string]any{"user_prompt": "count"}); err != nil {
		t.Fatal(err)
	}
	if result.Extra["runtime_only"] == nil {
		t.Fatal("transcript capture mutated the original tool result")
	}
}

func TestAgentStructuredOutputCorrectionKeepsChildEvidenceSeparate(t *testing.T) {
	for _, childStructured := range []bool{false, true} {
		t.Run(fmt.Sprint(childStructured), func(t *testing.T) {
			withAgentRunner(t, func(ctx context.Context, p AgentParam) (*schema.Message, error) {
				result := schema.ToolMessage(`{"count":7}`, "parent-call")
				if p.UserPrompt == "child" {
					result = schema.ToolMessage(`{"count":99}`, "child-call")
				}
				agentMessageModifier(ctx, p)(ctx, []*schema.Message{result})
				if p.UserPrompt == "child" {
					return schema.AssistantMessage(`{"count":99}`, nil), nil
				}
				child := AgentParam{ModelID: "stub", UserPrompt: "child"}
				if childStructured {
					child.OutputStructure = agentCountOutputSchema()
				}
				if _, err := NewAgentComponent(child).Invoke(ctx, nil, nil); err != nil {
					return nil, err
				}
				return schema.AssistantMessage(`{}`, nil), nil
			})
			withStubInvoker(t, structuredRepairInvoker(func(_ context.Context, req ChatInvokeRequest) (*ChatInvokeResponse, error) {
				observations := correctionToolObservations(t, req)
				if len(observations) != 1 || observations[0].ToolCallID != "parent-call" || observations[0].Content != `{"count":7}` {
					t.Fatalf("child replaced the parent's evidence: %#v", observations)
				}
				return &ChatInvokeResponse{Content: `{"count":7}`}, nil
			}))
			c := NewAgentComponent(AgentParam{ModelID: "stub", OutputStructure: agentCountOutputSchema(), MaxRetries: 1})
			if _, err := c.Invoke(t.Context(), nil, map[string]any{"user_prompt": "parent"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAgentStructuredOutputTranscriptEncodingFailureDoesNotPublish(t *testing.T) {
	withAgentRunner(t, func(ctx context.Context, p AgentParam) (*schema.Message, error) {
		agentMessageModifier(ctx, p)(ctx, []*schema.Message{schema.AssistantMessage("", []schema.ToolCall{
			{ID: "call", Function: schema.FunctionCall{Name: "count"}, Extra: map[string]any{"invalid": make(chan int)}},
		})})
		return schema.AssistantMessage(`{}`, nil), nil
	})
	stub := &stubInvoker{}
	withStubInvoker(t, stub)
	state := runtime.NewCanvasState("run", "task")
	var published strings.Builder
	ctx := runtime.WithState(t.Context(), state)
	ctx = runtime.WithAgentMessageEmitter(ctx, func(content, _ string) { published.WriteString(content) })
	c := NewAgentComponent(AgentParam{ModelID: "stub", OutputStructure: agentCountOutputSchema(), MaxRetries: 1})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "count"})
	var encodingErr *json.UnsupportedTypeError
	if !errors.As(err, &encodingErr) || out != nil || stub.calls != 0 || published.Len() != 0 || len(state.SnapshotMemory()) != 0 {
		t.Fatalf("out=%#v err=%v calls=%d published=%q memory=%#v", out, err, stub.calls, published.String(), state.SnapshotMemory())
	}
}

func TestAgentStructuredOutputRejectsInvalidSchemaBeforeRunner(t *testing.T) {
	withAgentRunner(t, func(context.Context, AgentParam) (*schema.Message, error) {
		t.Fatal("invalid schema must fail before runner")
		return nil, nil
	})
	stub := &stubInvoker{}
	withStubInvoker(t, stub)
	c := NewAgentComponent(AgentParam{ModelID: "stub", OutputStructure: map[string]any{"type": "not-a-type"}})
	if out, err := c.Invoke(t.Context(), nil, map[string]any{"user_prompt": "count"}); err == nil || out != nil {
		t.Fatalf("out=%#v err=%v", out, err)
	}
	if stub.calls != 0 {
		t.Fatalf("model calls=%d", stub.calls)
	}
}

func TestAgentStructuredOutputCancellationDoesNotPublish(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	withAgentRunner(t, func(context.Context, AgentParam) (*schema.Message, error) {
		cancel()
		return &schema.Message{Role: schema.Assistant, Content: `{"count":7}`}, nil
	})
	var emitted strings.Builder
	ctx = runtime.WithAgentMessageEmitter(ctx, func(content, _ string) { emitted.WriteString(content) })
	c := NewAgentComponent(AgentParam{ModelID: "stub", OutputStructure: agentCountOutputSchema(), MaxRetries: 3})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "count"})
	if !errors.Is(err, context.Canceled) || out != nil || emitted.Len() != 0 {
		t.Fatalf("out=%#v err=%v emitted=%q", out, err, emitted.String())
	}
}

func TestAgentStructuredOutputDoesNotAppendCitationOrArtifactMarkdown(t *testing.T) {
	withAgentRunner(t, func(ctx context.Context, _ AgentParam) (*schema.Message, error) {
		collector := ctx.Value(artifactCollectorKey{}).(*artifactCollector)
		collector.artifacts = []artifactEntry{{Name: "report.png", URL: "/report.png", MIMEType: "image/png"}}
		return &schema.Message{Role: schema.Assistant, Content: `{"count":7}`}, nil
	})
	stub := &stubInvoker{err: errors.New("unexpected extra model call")}
	withStubInvoker(t, stub)
	c := NewAgentComponent(AgentParam{ModelID: "stub", OutputStructure: agentCountOutputSchema(), Cite: true})
	state := runtime.NewCanvasState("run", "task")
	state.SetRetrievalChunks([]map[string]any{{"id": "0", "content": "source"}})
	out, err := c.Invoke(runtime.WithState(t.Context(), state), nil, map[string]any{"user_prompt": "count"})
	if err != nil {
		t.Fatal(err)
	}
	if out["content"] != `{"count":7}` || len(out["artifacts"].([]artifactEntry)) != 1 || stub.calls != 0 {
		t.Fatalf("out=%#v extra calls=%d", out, stub.calls)
	}
}

func TestAgentStructuredOutputConfig(t *testing.T) {
	p := agentParamFromMap(map[string]any{"outputs": map[string]any{"structured": agentCountOutputSchema()}})
	if p.MaxRetries != 3 || p.OutputStructure == nil {
		t.Fatalf("factory parameters=%#v", p)
	}
	p = mergeAgentParam(p, map[string]any{"max_retries": 0, "output_structure": map[string]any{}})
	if p.MaxRetries != 0 || len(p.OutputStructure.(map[string]any)) != 0 {
		t.Fatalf("runtime parameters=%#v", p)
	}
}

func TestAgentStructuredOutputEmptySchemaKeepsDeferredExecution(t *testing.T) {
	withAgentRunner(t, func(context.Context, AgentParam) (*schema.Message, error) {
		t.Fatal("disabled schema should retain deferred execution")
		return nil, nil
	})
	ctx := runtime.WithComponentExecutionOptions(t.Context(), runtime.ComponentExecutionOptions{DeferAgentToMessage: true})
	c := NewAgentComponent(AgentParam{ModelID: "stub", OutputStructure: map[string]any{}})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "count"})
	if err != nil || !runtime.IsDeferredStream(out["content"]) {
		t.Fatalf("out=%#v err=%v", out, err)
	}
}

func TestAgentStructuredOutputStreamSurfacesValidationError(t *testing.T) {
	withAgentRunner(t, func(context.Context, AgentParam) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: `{"count":"bad"}`}, nil
	})
	c := NewAgentComponent(AgentParam{ModelID: "stub", OutputStructure: agentCountOutputSchema()})
	stream, err := c.Stream(t.Context(), nil, map[string]any{"user_prompt": "count"})
	if err != nil {
		t.Fatal(err)
	}
	out := <-stream
	if out["error"] == nil || out["content"] != nil || out["structured"] != nil {
		t.Fatalf("out=%#v", out)
	}
	if _, more := <-stream; more {
		t.Fatal("stream contains extra values")
	}
}

func TestAgentStructuredOutputFinalizesOnlyValidatedAnswer(t *testing.T) {
	for _, valid := range []bool{false, true} {
		t.Run(fmt.Sprint(valid), func(t *testing.T) {
			withAgentRunner(t, func(ctx context.Context, _ AgentParam) (*schema.Message, error) {
				// Child lifecycle hooks and deltas must not change the parent's publication state.
				runtime.ResetAgentMessageEmission(ctx)
				runtime.EmitAgentMessage(ctx, "unvalidated partial", "")
				runtime.FinalizeAgentMessage(ctx)
				answer := `{"count":"bad"}`
				if valid {
					answer = `{"count":7}`
				}
				return &schema.Message{Role: schema.Assistant, Content: answer}, nil
			})
			var published strings.Builder
			finalizations, resets := 0, 0
			ctx := runtime.WithAgentMessageEmitterControl(t.Context(),
				func(content, _ string) { published.WriteString(content) },
				func() bool { finalizations++; return true },
				func() { resets++ },
			)
			c := NewAgentComponent(AgentParam{ModelID: "stub", OutputStructure: agentCountOutputSchema()})
			out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "count"})
			if resets != 1 {
				t.Fatalf("parent resets=%d, want 1", resets)
			}
			if valid {
				if err != nil || finalizations != 1 || published.String() != `{"count":7}` {
					t.Fatalf("out=%#v err=%v finalizations=%d published=%q", out, err, finalizations, published.String())
				}
			} else if err == nil || out != nil || finalizations != 0 || published.Len() != 0 {
				t.Fatalf("out=%#v err=%v finalizations=%d published=%q", out, err, finalizations, published.String())
			}
		})
	}
}

func TestAgentStructuredOutputErrorShortcutsFailClosed(t *testing.T) {
	for _, failure := range []string{"No dataset is selected.", "[GraphRunError] exceeds max steps"} {
		t.Run(failure, func(t *testing.T) {
			runnerCalls := 0
			withAgentRunner(t, func(context.Context, AgentParam) (*schema.Message, error) {
				runnerCalls++
				return nil, errors.New(failure)
			})
			var published strings.Builder
			finalizations := 0
			ctx := runtime.WithState(t.Context(), runtime.NewCanvasState("run", "task"))
			ctx = runtime.WithAgentMessageEmitter(ctx,
				func(content, _ string) { published.WriteString(content) },
				func() bool { finalizations++; return true },
			)
			inputs := map[string]any{"user_prompt": "count"}
			if failure == "No dataset is selected." {
				inputs["_ERROR"] = failure
			}
			c := NewAgentComponent(AgentParam{ModelID: "stub", OutputStructure: agentCountOutputSchema()})
			out, err := c.Invoke(ctx, nil, inputs)
			if err == nil || !strings.Contains(err.Error(), failure) || out != nil || published.Len() != 0 || finalizations != 0 {
				t.Fatalf("out=%#v err=%v published=%q finalizations=%d", out, err, published.String(), finalizations)
			}
			if failure == "No dataset is selected." && runnerCalls != 0 {
				t.Fatalf("runner calls=%d, want 0", runnerCalls)
			}
		})
	}
}

func TestAgentStructuredOutputCancellationDuringMemoryDoesNotPublish(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	withAgentRunner(t, func(context.Context, AgentParam) (*schema.Message, error) {
		return &schema.Message{
			Role: schema.Assistant, Content: `{"count":7}`,
			ToolCalls: []schema.ToolCall{{ID: "call", Type: "function", Function: schema.FunctionCall{Name: "count"}}},
		}, nil
	})
	summaryCalls := 0
	withStubInvoker(t, structuredRepairInvoker(func(context.Context, ChatInvokeRequest) (*ChatInvokeResponse, error) {
		summaryCalls++
		cancel()
		return nil, context.Canceled
	}))
	state := runtime.NewCanvasState("run", "task")
	ctx = runtime.WithState(ctx, state)
	var published strings.Builder
	finalizations := 0
	ctx = runtime.WithAgentMessageEmitter(ctx,
		func(content, _ string) { published.WriteString(content) },
		func() bool { finalizations++; return true },
	)
	c := NewAgentComponent(AgentParam{ModelID: "stub", OutputStructure: agentCountOutputSchema()})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "count"})
	if !errors.Is(err, context.Canceled) || out != nil || published.Len() != 0 || finalizations != 0 || summaryCalls != 1 {
		t.Fatalf("out=%#v err=%v published=%q finalizations=%d summaryCalls=%d", out, err, published.String(), finalizations, summaryCalls)
	}
	if len(state.SnapshotMemory()) != 0 {
		t.Fatal("canceled summary wrote memory")
	}
}
