package core

import (
	"testing"

	"ragflow/internal/harness/core/schema"
)

func TestEventSenderModelWrapper_Creation(t *testing.T) {
	wrapper := NewEventSenderModelWrapper[*schema.Message]()
	if wrapper == nil {
		t.Fatal("nil wrapper")
	}
}

func TestHasUserEventSenderModelWrapper_Empty(t *testing.T) {
	handlers := []TypedReActMiddleware[*schema.Message]{}
	if HasUserEventSenderModelWrapper(handlers) {
		t.Error("should be false for empty handlers")
	}
}

func TestHasUserEventSenderModelWrapper_Present(t *testing.T) {
	wrapper := NewEventSenderModelWrapper[*schema.Message]()
	handlers := []TypedReActMiddleware[*schema.Message]{wrapper}
	if !HasUserEventSenderModelWrapper(handlers) {
		t.Error("should detect user's EventSenderModelWrapper")
	}
}

func TestEventSenderModelWrapper_AllNoOp(t *testing.T) {
	wrapper := NewEventSenderModelWrapper[*schema.Message]()
	_, _, _ = wrapper.BeforeAgent(nil, nil)
	_, _ = wrapper.AfterAgent(nil, nil)
	_, _, _ = wrapper.BeforeModelRewrite(nil, nil, nil)
	_, _, _ = wrapper.AfterModelRewrite(nil, nil, nil)
}

func TestResumeWithData(t *testing.T) {
	info := ResumeWithData(&ReActAgentResumeData{})
	if info.ResumeData == nil {
		t.Error("ResumeData should be set")
	}
	if info.WasInterrupted {
		t.Error("WasInterrupted should default to false")
	}
}

func TestExactRunPathMatch(t *testing.T) {
	a := []RunStep{{agentName: "a"}, {agentName: "b"}}
	b := []RunStep{{agentName: "a"}, {agentName: "b"}}
	if !exactRunPathMatch(a, b) {
		t.Error("equal paths should match")
	}
	if exactRunPathMatch(a, []RunStep{{agentName: "a"}}) {
		t.Error("different length paths should not match")
	}
}
