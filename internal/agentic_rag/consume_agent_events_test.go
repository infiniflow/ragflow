package agentic_rag

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func noopDelta(string, string) {}

func assistantMsgEvent(content string, toolCalls []schema.ToolCall) *adk.AgentEvent {
	return &adk.AgentEvent{Output: &adk.AgentOutput{
		MessageOutput: &adk.TypedMessageVariant[*schema.Message]{
			Role: schema.Assistant,
			Message: &schema.Message{
				Role:      schema.Assistant,
				Content:   content,
				ToolCalls: toolCalls,
			},
		},
	}}
}

func toolResultEvent(callID, content string) *adk.AgentEvent {
	return &adk.AgentEvent{Output: &adk.AgentOutput{
		MessageOutput: &adk.TypedMessageVariant[*schema.Message]{
			Role: schema.Tool,
			Message: &schema.Message{
				Role:       schema.Tool,
				Content:    content,
				ToolCallID: callID,
			},
		},
	}}
}

// streamedMsgEvent returns an event whose message arrives as a chunk stream.
func streamedMsgEvent(chunks []string) *adk.AgentEvent {
	sr, sw := schema.Pipe[*schema.Message](len(chunks) + 1)
	go func() {
		for _, c := range chunks {
			sw.Send(&schema.Message{Role: schema.Assistant, Content: c}, nil)
		}
		sw.Close()
	}()
	return &adk.AgentEvent{Output: &adk.AgentOutput{
		MessageOutput: &adk.TypedMessageVariant[*schema.Message]{
			Role:          schema.Assistant,
			IsStreaming:   true,
			MessageStream: sr,
		},
	}}
}

// The ReAct loop emits narration inside tool-call turns; the FOS deliverable
// is the LAST assistant message, so consumeAgentEvents must return exactly
// that — not the concatenation of every turn's prose. The conversation behind
// a turn is the session's business (see run_session.go), not this function's.
func TestConsumeAgentEventsReturnsLastMessage(t *testing.T) {
	const narration = "Let me check the record first."
	const deliverable = "## Reasoning Chain\n- Clue: x (doc: a.md)\n\nFinal Answer: **1939**\n"

	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		gen.Send(assistantMsgEvent(narration, []schema.ToolCall{
			{ID: "call-1", Function: schema.FunctionCall{Name: "grep_chunks", Arguments: "{}"}},
		}))
		gen.Send(toolResultEvent("call-1", "chunk text: the treaty of 1939"))
		gen.Send(streamedMsgEvent([]string{"## Reasoning Chain\n", "- Clue: x (doc: a.md)\n\n"}))
		gen.Send(assistantMsgEvent(deliverable, nil))
		gen.Close()
	}()

	counts := map[string]int{}
	final, evidence, err := consumeAgentEvents(context.Background(), iter, noopDelta, counts, nil, nil)
	if err != nil {
		t.Fatalf("consumeAgentEvents error: %v", err)
	}
	if final != deliverable {
		t.Errorf("final = %q, want exactly the last assistant message %q", final, deliverable)
	}
	if !strings.Contains(evidence, "treaty of 1939") {
		t.Errorf("evidence = %q, want the tool result accumulated", evidence)
	}
	if counts["grep_chunks"] != 1 {
		t.Errorf("grep_chunks tally = %d, want 1", counts["grep_chunks"])
	}
}

// A run that ENDS on a streamed turn must return that message's chunks joined.
func TestConsumeAgentEventsStreamedLastMessage(t *testing.T) {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		gen.Send(assistantMsgEvent("earlier draft", nil))
		gen.Send(streamedMsgEvent([]string{"## Reasoning Chain\n", "- Clue: x\n", "Final Answer: **1897**"}))
		gen.Close()
	}()

	final, _, err := consumeAgentEvents(context.Background(), iter, noopDelta, nil, nil, nil)
	if err != nil {
		t.Fatalf("consumeAgentEvents error: %v", err)
	}
	const want = "## Reasoning Chain\n- Clue: x\nFinal Answer: **1897**"
	if final != want {
		t.Errorf("final = %q, want the streamed message joined %q", final, want)
	}
}

// Streaming tool-call deltas must be merged back by Index so one turn keeps
// every parallel call it issued.
func TestMergeStreamedAssistantMergesToolCallDeltas(t *testing.T) {
	idx0 := 0
	chunks := []*schema.Message{
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{
			{Index: &idx0, ID: "call-1", Function: schema.FunctionCall{Name: "grep_chunks", Arguments: "{\"pa"}},
		}},
		{Role: schema.Assistant, Content: "checking "},
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{
			{Index: &idx0, Function: schema.FunctionCall{Arguments: "ttern\":\"x\"}"}},
		}},
	}
	got := mergeStreamedAssistant(chunks)
	if got.Content != "checking " {
		t.Errorf("content = %q", got.Content)
	}
	if len(got.ToolCalls) != 1 ||
		got.ToolCalls[0].ID != "call-1" ||
		got.ToolCalls[0].Function.Name != "grep_chunks" ||
		got.ToolCalls[0].Function.Arguments != `{"pattern":"x"}` {
		t.Errorf("tool calls not merged: %+v", got.ToolCalls)
	}
}

// A ReAct turn often renders a complete deliverable, keeps retrieving, and then
// renders a better one. EVERY message goes out live — but on the THINKING
// channel, so the user watches the work without being handed several competing
// "Final Answer" blocks. The answer channel stays empty here: it is emitted
// once, by Run, when the shipped deliverable is settled.
func TestConsumeAgentEventsStreamsEverythingAsThinking(t *testing.T) {
	first := "## Candidate Matrix\n\n### Sub-question 1: x\n- Retained: a\n\nFinal Answer: **11 人**\n"
	second := "## Candidate Matrix\n\n### Sub-question 1: x\n- Retained: a\n- Retained: b\n\nFinal Answer: **14 人**\n"

	var answers, thinkings strings.Builder
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		gen.Send(streamedMsgEvent([]string{first}))
		gen.Send(streamedMsgEvent([]string{second}))
		gen.Close()
	}()

	final, _, err := consumeAgentEvents(context.Background(), iter,
		func(contentDelta, thinkingDelta string) {
			answers.WriteString(contentDelta)
			thinkings.WriteString(thinkingDelta)
		}, nil, nil, nil)
	if err != nil {
		t.Fatalf("consumeAgentEvents: %v", err)
	}
	if final != second {
		t.Errorf("final = %q, want the last message", final)
	}
	if answers.String() != "" {
		t.Errorf("answer channel = %q, want nothing (Run emits the answer once)", answers.String())
	}
	if thinkings.String() != first+second {
		t.Errorf("thinking channel = %q, want both deliverables streamed live", thinkings.String())
	}
}

// Nothing may be held back: a single-message run still streams live (as
// thinking) rather than waiting for a second message that never comes.
func TestConsumeAgentEventsStreamsSingleMessageLive(t *testing.T) {
	only := "## Candidate Matrix\n\n- Retained: a\n\nFinal Answer: **7 人**\n"
	var answers, thinkings strings.Builder

	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		gen.Send(streamedMsgEvent([]string{only}))
		gen.Close()
	}()
	if _, _, err := consumeAgentEvents(context.Background(), iter,
		func(contentDelta, thinkingDelta string) {
			answers.WriteString(contentDelta)
			thinkings.WriteString(thinkingDelta)
		}, nil, nil, nil); err != nil {
		t.Fatalf("consumeAgentEvents: %v", err)
	}
	if thinkings.String() != only {
		t.Errorf("thinking channel = %q, want the message streamed live", thinkings.String())
	}
	if answers.String() != "" {
		t.Errorf("answer channel = %q, want nothing", answers.String())
	}
}

// A streamed tool-calls turn arrives as content deltas plus ONE complete
// tool-calls chunk carrying EVERY parallel call, each with its own Index
// (set by the models layer). The merge must keep them as separate calls —
// collapsing them (nil Index reads as 0) orphans all but the last call's
// results and makes the replayed conversation provider-invalid
// (MiniMax: `tool result's tool id(X) not found`).
func TestMergeStreamedAssistantKeepsParallelCompleteCalls(t *testing.T) {
	i0, i1 := 0, 1
	chunks := []*schema.Message{
		{Role: schema.Assistant, Content: "searching "},
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{
			{Index: &i0, ID: "call_x_1", Function: schema.FunctionCall{Name: "list_chunks", Arguments: `{"doc_id":"d"}`}},
			{Index: &i1, ID: "call_x_2", Function: schema.FunctionCall{Name: "list_chunks", Arguments: `{"doc_id":"e"}`}},
		}},
	}
	got := mergeStreamedAssistant(chunks)
	if got.Content != "searching " {
		t.Errorf("content = %q", got.Content)
	}
	if len(got.ToolCalls) != 2 {
		t.Fatalf("tool calls = %d, want 2: %+v", len(got.ToolCalls), got.ToolCalls)
	}
	if got.ToolCalls[0].ID != "call_x_1" || got.ToolCalls[0].Function.Arguments != `{"doc_id":"d"}` {
		t.Errorf("call 0 not preserved: %+v", got.ToolCalls[0])
	}
	if got.ToolCalls[1].ID != "call_x_2" || got.ToolCalls[1].Function.Arguments != `{"doc_id":"e"}` {
		t.Errorf("call 1 not preserved: %+v", got.ToolCalls[1])
	}
}
