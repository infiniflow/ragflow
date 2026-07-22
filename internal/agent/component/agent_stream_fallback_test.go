package component

import (
	"context"
	"errors"
	"io"
	"testing"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type agentChatModelStub struct {
	streamMessages []*schema.Message
	streamErr      error
	generateResult *schema.Message
	generateCalls  int
}

func (m *agentChatModelStub) Generate(context.Context, []*schema.Message, ...einomodel.Option) (*schema.Message, error) {
	m.generateCalls++
	return m.generateResult, nil
}

func (m *agentChatModelStub) Stream(context.Context, []*schema.Message, ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	stream, writer := schema.Pipe[*schema.Message](1)
	go func() {
		defer writer.Close()
		for _, msg := range m.streamMessages {
			if writer.Send(msg, nil) {
				return
			}
		}
		if m.streamErr != nil {
			_ = writer.Send(nil, m.streamErr)
		}
	}()
	return stream, nil
}

func (m *agentChatModelStub) WithTools([]*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	return m, nil
}

func TestAgentChatModelFallsBackWhenStreamingIsUnsupported(t *testing.T) {
	stub := &agentChatModelStub{
		streamErr:      errors.New("provider does not implement ChatStreamlyWithSender"),
		generateResult: schema.AssistantMessage("complete answer", nil),
	}
	model := &agentChatModel{ToolCallingChatModel: stub}

	stream, err := model.Stream(context.Background(), []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	msg, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if msg == nil || msg.Content != "complete answer" {
		t.Fatalf("fallback message = %#v, want complete answer", msg)
	}
	if _, err = stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("second Recv error = %v, want EOF", err)
	}
	if stub.generateCalls != 1 {
		t.Fatalf("Generate calls = %d, want 1", stub.generateCalls)
	}
}

func TestAgentChatModelDoesNotFallbackForOtherStreamErrors(t *testing.T) {
	streamErr := errors.New("connection reset")
	stub := &agentChatModelStub{
		streamErr:      streamErr,
		generateResult: schema.AssistantMessage("unexpected fallback", nil),
	}
	model := &agentChatModel{ToolCallingChatModel: stub}

	stream, err := model.Stream(context.Background(), []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if _, err = stream.Recv(); !errors.Is(err, streamErr) {
		t.Fatalf("Recv error = %v, want %v", err, streamErr)
	}
	if stub.generateCalls != 0 {
		t.Fatalf("Generate calls = %d, want 0", stub.generateCalls)
	}
}

func TestAgentChatModelDoesNotFallbackAfterStreamOutput(t *testing.T) {
	streamErr := errors.New("provider does not implement ChatStreamlyWithSender")
	stub := &agentChatModelStub{
		streamMessages: []*schema.Message{schema.AssistantMessage("partial", nil)},
		streamErr:      streamErr,
		generateResult: schema.AssistantMessage("unexpected fallback", nil),
	}
	model := &agentChatModel{ToolCallingChatModel: stub}

	stream, err := model.Stream(context.Background(), []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	msg, err := stream.Recv()
	if err != nil || msg == nil || msg.Content != "partial" {
		t.Fatalf("first Recv = (%#v, %v), want partial", msg, err)
	}
	if _, err = stream.Recv(); !errors.Is(err, streamErr) {
		t.Fatalf("second Recv error = %v, want %v", err, streamErr)
	}
	if stub.generateCalls != 0 {
		t.Fatalf("Generate calls = %d, want 0", stub.generateCalls)
	}
}
