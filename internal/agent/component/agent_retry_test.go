package component

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type retryTestChatModel struct {
	generateCalls             int
	streamCalls               int
	failStreamAfterFirstChunk bool
}

func (m *retryTestChatModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func (m *retryTestChatModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.generateCalls++
	if m.generateCalls == 1 {
		return nil, errors.New("temporary model failure")
	}
	return &schema.Message{Role: schema.Assistant, Content: "recovered"}, nil
}

func (m *retryTestChatModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.streamCalls++
	reader, writer := schema.Pipe[*schema.Message](1)
	go func(attempt int) {
		defer writer.Close()
		if attempt == 1 {
			if m.failStreamAfterFirstChunk {
				writer.Send(&schema.Message{Role: schema.Assistant, Content: "partial"}, nil)
				writer.Send(nil, errors.New("stream failed after output"))
				return
			}
			writer.Send(nil, errors.New("temporary stream failure"))
			return
		}
		writer.Send(&schema.Message{Role: schema.Assistant, Content: "recovered"}, nil)
	}(m.streamCalls)
	return reader, nil
}

func TestRoundLimitedChatModelDoesNotRetryStreamAfterOutput(t *testing.T) {
	inner := &retryTestChatModel{failStreamAfterFirstChunk: true}
	chatModel := &roundLimitedChatModel{
		ToolCallingChatModel: inner,
		remaining:            1,
		maxRetries:           2,
	}

	stream, err := chatModel.Stream(t.Context(), []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer stream.Close()
	message, err := stream.Recv()
	if err != nil || message.Content != "partial" {
		t.Fatalf("first Recv = (%v, %v), want partial message", message, err)
	}
	if _, err = stream.Recv(); err == nil || err.Error() != "stream failed after output" {
		t.Fatalf("second Recv error = %v, want original stream error", err)
	}
	if inner.streamCalls != 1 {
		t.Fatalf("stream called %d times, want no retry after output", inner.streamCalls)
	}
}

func TestRoundLimitedChatModelWithToolsPreservesRetrySettings(t *testing.T) {
	chatModel := &roundLimitedChatModel{
		ToolCallingChatModel: &retryTestChatModel{},
		remaining:            2,
		maxRetries:           3,
		initialDelay:         250 * time.Millisecond,
	}

	bound, err := chatModel.WithTools([]*schema.ToolInfo{{Name: "search"}})
	if err != nil {
		t.Fatalf("WithTools: %v", err)
	}
	got, ok := bound.(*roundLimitedChatModel)
	if !ok {
		t.Fatalf("bound model type = %T, want *roundLimitedChatModel", bound)
	}
	if got.maxRetries != 3 || got.initialDelay != 250*time.Millisecond || got.remaining != 2 {
		t.Fatalf("bound retry settings = (%d, %s, %d), want (3, 250ms, 2)", got.maxRetries, got.initialDelay, got.remaining)
	}
}

func TestRoundLimitedChatModelRetriesGenerate(t *testing.T) {
	inner := &retryTestChatModel{}
	chatModel := &roundLimitedChatModel{
		ToolCallingChatModel: inner,
		remaining:            1,
		maxRetries:           1,
	}

	got, err := chatModel.Generate(t.Context(), []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got.Content != "recovered" || inner.generateCalls != 2 {
		t.Fatalf("Generate = (%q, %d calls), want (recovered, 2 calls)", got.Content, inner.generateCalls)
	}
}

func TestRoundLimitedChatModelRetriesStreamBeforeFirstMessage(t *testing.T) {
	inner := &retryTestChatModel{}
	chatModel := &roundLimitedChatModel{
		ToolCallingChatModel: inner,
		remaining:            1,
		maxRetries:           1,
	}

	stream, err := chatModel.Stream(t.Context(), []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer stream.Close()
	got, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if got.Content != "recovered" || inner.streamCalls != 2 {
		t.Fatalf("Stream = (%q, %d calls), want (recovered, 2 calls)", got.Content, inner.streamCalls)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("second Recv error = %v, want EOF", err)
	}
}

func TestMergeAgentParamReadsAdvancedSettings(t *testing.T) {
	p := mergeAgentParam(AgentParam{}, map[string]any{
		"max_retries":       float64(4),
		"delay_after_error": float64(0.25),
		"visual_files_var":  "Begin:attachments",
	})

	if p.MaxRetries != 4 {
		t.Errorf("MaxRetries = %d, want 4", p.MaxRetries)
	}
	if p.DelayAfterError != 250*time.Millisecond {
		t.Errorf("DelayAfterError = %s, want 250ms", p.DelayAfterError)
	}
	if p.VisualFilesVar != "Begin:attachments" {
		t.Errorf("VisualFilesVar = %q, want Begin:attachments", p.VisualFilesVar)
	}
}
