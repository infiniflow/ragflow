package component

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/runtime"
)

func TestAgentReActModelObserverGenerateAttempts(t *testing.T) {
	var records []*agentObserverRecord
	ctx := withTestAgentObserver(t.Context(), &records)
	inner := &retryTestChatModel{}
	wrapped := &roundLimitedChatModel{ToolCallingChatModel: inner, remaining: 1, maxRetries: 1}
	result, err := wrapped.Generate(ctx, []*schema.Message{schema.UserMessage("hello")})
	if err != nil || result.Content != "recovered" || len(records) != 2 {
		t.Fatalf("result=%+v err=%v observations=%d", result, err, len(records))
	}
	if records[0].kind != "ModelCall" || records[0].attempt != 1 || records[0].err == nil || records[1].attempt != 2 || records[1].err != nil {
		t.Fatalf("records=%+v %+v", records[0], records[1])
	}
	output, _ := records[1].output.(map[string]any)
	if output["content"] != "recovered" {
		t.Fatalf("output=%#v", records[1].output)
	}
}

func TestAgentReActModelObserverStreamAttempts(t *testing.T) {
	var records []*agentObserverRecord
	ctx := withTestAgentObserver(t.Context(), &records)
	wrapped := &roundLimitedChatModel{ToolCallingChatModel: &retryTestChatModel{}, remaining: 1, maxRetries: 1}
	stream, err := wrapped.Stream(ctx, []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	message, err := stream.Recv()
	if err != nil || message.Content != "recovered" {
		t.Fatalf("message=%+v err=%v", message, err)
	}
	if _, err = stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("end=%v", err)
	}
	if len(records) != 2 || records[0].err == nil || records[1].err != nil || records[1].attempt != 2 {
		t.Fatalf("observations=%+v", records)
	}
}

type observedStreamChatModel struct {
	retryTestChatModel
	stream func(context.Context) (*schema.StreamReader[*schema.Message], error)
}

func (m *observedStreamChatModel) Stream(ctx context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return m.stream(ctx)
}

func TestAgentReActModelObserverStreamSetupFailure(t *testing.T) {
	var records []*agentObserverRecord
	ctx := withTestAgentObserver(t.Context(), &records)
	failure := errors.New("provider unavailable")
	inner := &observedStreamChatModel{stream: func(context.Context) (*schema.StreamReader[*schema.Message], error) { return nil, failure }}
	wrapped := &roundLimitedChatModel{ToolCallingChatModel: inner, remaining: 1}
	stream, err := wrapped.Stream(ctx, nil)
	if stream != nil || !errors.Is(err, failure) || len(records) != 1 || !errors.Is(records[0].err, failure) {
		t.Fatalf("stream=%v err=%v observations=%+v", stream, err, records)
	}
}

func TestAgentReActModelObserverStreamConsumerClose(t *testing.T) {
	finished := make(chan error, 1)
	upstreamClosed := make(chan bool, 1)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx = runtime.WithAgentNodeObserver(ctx, func(ctx context.Context, _ string, _ string, _ string, _ uint32, _ any) (context.Context, func(any, error)) {
		return ctx, func(_ any, err error) { finished <- err }
	})
	providerReader, providerWriter := schema.Pipe[*schema.Message](0)
	defer providerWriter.Close()
	inner := &observedStreamChatModel{stream: func(context.Context) (*schema.StreamReader[*schema.Message], error) { return providerReader, nil }}
	wrapped := &roundLimitedChatModel{ToolCallingChatModel: inner, remaining: 1}
	stream, err := wrapped.Stream(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	stream.Close()
	go func() { upstreamClosed <- providerWriter.Send(schema.AssistantMessage("late", nil), nil) }()
	select {
	case closed := <-upstreamClosed:
		if !closed {
			t.Fatal("consumer Close did not close upstream")
		}
	case <-time.After(time.Second):
		t.Fatal("consumer Close left upstream blocked")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("completion=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled model observation did not finish")
	}
}

func TestAgentReActModelWithoutObserverPreservesStream(t *testing.T) {
	original := schema.StreamReaderFromArray([]*schema.Message{schema.AssistantMessage("reply", nil)})
	inner := &observedStreamChatModel{stream: func(context.Context) (*schema.StreamReader[*schema.Message], error) { return original, nil }}
	wrapped := &roundLimitedChatModel{ToolCallingChatModel: inner, remaining: 1}
	got, err := wrapped.Stream(t.Context(), nil)
	if err != nil || got != original {
		t.Fatalf("no-observer stream changed: got=%p original=%p err=%v", got, original, err)
	}
	got.Close()
}
