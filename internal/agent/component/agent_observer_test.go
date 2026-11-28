package component

import (
	"context"
	"errors"
	"testing"
	"time"

	einotool "github.com/cloudwego/eino/components/tool"
	"gorm.io/gorm"

	"ragflow/internal/agent/chat"
	"ragflow/internal/agent/runtime"
)

type agentObservedTool struct {
	referenceRecordingFakeTool
	run func(context.Context) (string, error)
}

func (tool *agentObservedTool) InvokableRun(ctx context.Context, _ string, _ ...einotool.Option) (string, error) {
	return tool.run(ctx)
}

type agentObserverRecord struct {
	kind          string
	attempt       uint32
	input, output any
	err           error
}

func withTestAgentObserver(ctx context.Context, records *[]*agentObserverRecord) context.Context {
	return runtime.WithAgentNodeObserver(ctx, func(ctx context.Context, _ string, _ string, kind string, attempt uint32, input any) (context.Context, func(any, error)) {
		record := &agentObserverRecord{kind: kind, attempt: attempt, input: input}
		*records = append(*records, record)
		return ctx, func(output any, err error) { record.output, record.err = output, err }
	})
}

func TestAgentObserverToolOutcomes(t *testing.T) {
	failure := errors.New("tool failed")
	for _, tc := range []struct {
		name, result string
		err          error
		wantFailure  bool
	}{
		{name: "success", result: `{"result":"ok"}`},
		{name: "returned error", err: failure, wantFailure: true},
		{name: "handled error", result: `{"_ERROR":"failed inside tool"}`, wantFailure: true},
		{name: "empty error", result: `{"_ERROR":"","result":"ok"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var records []*agentObserverRecord
			ctx := withTestAgentObserver(t.Context(), &records)
			inner := &agentObservedTool{run: func(context.Context) (string, error) { return tc.result, tc.err }}
			tool := &toolInvocationRecordingTool{InvokableTool: inner, name: "search", recorder: &toolInvocationRecorder{}}
			result, err := tool.InvokableRun(ctx, `{"query":"hello"}`)
			if result != tc.result || !errors.Is(err, tc.err) {
				t.Fatalf("tool result changed: %q %v", result, err)
			}
			if len(records) != 1 || records[0].kind != "Tool" || records[0].attempt != 1 || (records[0].err != nil) != tc.wantFailure || records[0].output != tc.result {
				t.Fatalf("records=%+v", records)
			}
			if tc.err != nil && !errors.Is(records[0].err, tc.err) {
				t.Fatalf("observation lost error: %v", records[0].err)
			}
		})
	}
}

func TestAgentObserverToolPropagatesContextAndPanic(t *testing.T) {
	type parentKey struct{}
	var observedErr error
	ctx := runtime.WithAgentNodeObserver(t.Context(), func(ctx context.Context, _ string, _ string, _ string, _ uint32, _ any) (context.Context, func(any, error)) {
		return context.WithValue(ctx, parentKey{}, "tool execution"), func(_ any, err error) { observedErr = err }
	})
	sentinel := new(struct{})
	inner := &agentObservedTool{run: func(ctx context.Context) (string, error) {
		if ctx.Value(parentKey{}) != "tool execution" {
			t.Error("child context did not reach tool")
		}
		panic(sentinel)
	}}
	tool := &toolInvocationRecordingTool{InvokableTool: inner, name: "nested_agent", recorder: &toolInvocationRecorder{}}
	func() {
		defer func() {
			if recovered := recover(); recovered != sentinel {
				t.Errorf("panic changed: %v", recovered)
			}
		}()
		_, _ = tool.InvokableRun(ctx, `{}`)
	}()
	if observedErr == nil {
		t.Fatal("panic was not recorded as a failed tool")
	}
}

func TestAgentObserverModelRetries(t *testing.T) {
	failure := errors.New("temporary")
	var records []*agentObserverRecord
	ctx := withTestAgentObserver(t.Context(), &records)
	want := &ChatInvokeResponse{Content: "done", Model: "model", Usage: &chat.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}, ToolCalls: []chat.ToolCall{{ID: "call", Name: "search", Arguments: map[string]any{"query": "hello"}}}}
	inv := newRetryInvoker(&scriptedInvoker{failTimes: 1, err: failure, resp: want}, 2, 0)
	result, err := inv.Invoke(ctx, nil, ChatInvokeRequest{ModelName: "model"})
	if err != nil || result != want || len(records) != 2 {
		t.Fatalf("result=%+v err=%v attempts=%d", result, err, len(records))
	}
	output, ok := records[1].output.(map[string]any)
	if !ok || output["content"] != "done" || output["model"] != "model" {
		t.Fatalf("model result is not a snapshot-compatible map: %#v", records[1].output)
	}
	usage, _ := output["usage"].(map[string]any)
	calls, _ := output["tool_calls"].([]any)
	if usage["total_tokens"] != 5 || len(calls) != 1 || calls[0].(map[string]any)["name"] != "search" {
		t.Fatalf("model usage or tool calls missing: %#v", output)
	}
	if records[0].attempt != 1 || !errors.Is(records[0].err, failure) || records[1].attempt != 2 || records[1].err != nil || records[1].kind != "ModelCall" {
		t.Fatalf("attempts=%+v %+v", records[0], records[1])
	}
}

type agentCancelInvoker struct{ cancel context.CancelFunc }

func (inv agentCancelInvoker) Invoke(ctx context.Context, _ *gorm.DB, _ ChatInvokeRequest) (*ChatInvokeResponse, error) {
	inv.cancel()
	return nil, ctx.Err()
}
func TestAgentObserverModelCancellationStopsRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var records []*agentObserverRecord
	ctx = withTestAgentObserver(ctx, &records)
	inv := newRetryInvoker(agentCancelInvoker{cancel: cancel}, 3, time.Hour)
	_, err := inv.Invoke(ctx, nil, ChatInvokeRequest{ModelName: "model"})
	if !errors.Is(err, context.Canceled) || len(records) != 1 || !errors.Is(records[0].err, context.Canceled) {
		t.Fatalf("err=%v attempts=%+v", err, records)
	}
}

func TestAgentObserverStreamAttemptsAndCallbackErrors(t *testing.T) {
	failure := errors.New("temporary")
	var records []*agentObserverRecord
	ctx := withTestAgentObserver(t.Context(), &records)
	inv := newRetryInvoker(&mockStreamingInvoker{failTimes: 1, err: failure, deltas: []string{"a", "b"}}, 2, 0)
	result, err := inv.Stream(ctx, nil, ChatInvokeRequest{ModelName: "model"}, nil)
	if err != nil || result.Content != "ab" || len(records) != 2 || !errors.Is(records[0].err, failure) || records[1].err != nil {
		t.Fatalf("result=%+v err=%v attempts=%+v", result, err, records)
	}
	records = nil
	callbackErr := errors.New("client stopped")
	inv = newRetryInvoker(&mockStreamingInvoker{deltas: []string{"a", "b"}}, 3, 0)
	_, err = inv.Stream(ctx, nil, ChatInvokeRequest{}, func(string, bool) error { return callbackErr })
	if !errors.Is(err, callbackErr) || len(records) != 1 || !errors.Is(records[0].err, callbackErr) {
		t.Fatalf("err=%v attempts=%+v", err, records)
	}
}
