package agentic_rag

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/session"
	"github.com/cloudwego/eino/schema"
)

// newTestConversation returns a conversation over a fresh in-memory store,
// detached from any run's shared log.
func newTestConversation() *conversation {
	return newConversation("test", session.NewInMemoryStore[adk.Message](nil))
}

// scriptedSessionAgent is an adk.Agent driven through a real Runner: it emits
// ONE assistant message per turn and fails on demand, so a test can script the
// two things a managed conversation has to survive — history replay across
// turns and the discard of a turn that failed.
type scriptedSessionAgent struct {
	turns int
	// fail reports whether turn n (1-based) must abort with an error.
	fail func(turn int) bool
	// out is the assistant content emitted by turn n.
	out func(turn int) string
	// seen counts the messages the Runner handed to each turn: with a managed
	// session that is the replayed history PLUS this turn's own messages.
	seen []int
}

func (s *scriptedSessionAgent) Name(context.Context) string        { return "scripted" }
func (s *scriptedSessionAgent) Description(context.Context) string { return "scripted test agent" }

func (s *scriptedSessionAgent) Run(_ context.Context, input *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	s.turns++
	turn := s.turns
	s.seen = append(s.seen, len(input.Messages))
	content := ""
	if s.out != nil {
		content = s.out(turn)
	}
	failing := s.fail != nil && s.fail(turn)
	go func() {
		if failing {
			// The real hazard: a tool call issued, its result never arriving.
			gen.Send(assistantMsgEvent("", []schema.ToolCall{
				{ID: "dangling", Function: schema.FunctionCall{Name: "list_chunks", Arguments: "{}"}},
			}))
			gen.Send(&adk.AgentEvent{Err: errors.New("upstream tool failed")})
			gen.Close()
			return
		}
		gen.Send(assistantMsgEvent(content, nil))
		gen.Close()
	}()
	return iter
}

// runTurn plays ONE turn the way production does: only the turn's own messages
// go in, and a turn that failed is rolled out of the conversation.
func runTurn(t *testing.T, conv *conversation, agent adk.Agent, messages ...adk.Message) (string, error) {
	t.Helper()
	head := conv.head(context.Background())
	iter := conv.runner(context.Background(), agent, false).Run(context.Background(), messages)
	final, _, err := consumeAgentEvents(context.Background(), iter, noopDelta, nil, nil, nil)
	if err != nil {
		conv.discardFailedTurn(context.Background(), head)
	}
	return final, err
}

// A managed conversation is continuous: turn 2 receives turn 1 rejoined by the
// Runner — this is what the gate used to reassemble by hand.
func TestConversationReplaysHistoryAcrossTurns(t *testing.T) {
	conv := newTestConversation()
	agent := &scriptedSessionAgent{out: func(turn int) string {
		return "answer " + string(rune('a'+turn-1))
	}}

	if _, err := runTurn(t, conv, agent, schema.UserMessage("first")); err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if _, err := runTurn(t, conv, agent, schema.UserMessage("second")); err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if len(agent.seen) != 2 {
		t.Fatalf("turns = %d, want 2", len(agent.seen))
	}
	if agent.seen[0] != 1 {
		t.Errorf("turn 1 saw %d messages, want just its own user message", agent.seen[0])
	}
	if agent.seen[1] != 3 { // user, assistant, user
		t.Errorf("turn 2 saw %d messages, want turn 1 replayed plus its own user message", agent.seen[1])
	}
}

// A FAILED turn must not reach the next one: an aborted run can leave tool
// calls whose results never arrived, which providers reject on replay. With a
// committed predecessor, the failure rolls back to it.
func TestConversationDiscardFailedTurnRollsBack(t *testing.T) {
	conv := newTestConversation()
	agent := &scriptedSessionAgent{fail: func(turn int) bool { return turn == 2 }}

	if _, err := runTurn(t, conv, agent, schema.UserMessage("first")); err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if _, err := runTurn(t, conv, agent, schema.UserMessage("second")); err == nil {
		t.Fatal("turn 2 was scripted to fail")
	}
	if _, err := runTurn(t, conv, agent, schema.UserMessage("third")); err != nil {
		t.Fatalf("turn 3: %v", err)
	}
	if len(agent.seen) != 3 {
		t.Fatalf("turns = %d, want 3", len(agent.seen))
	}
	if agent.seen[2] != 3 { // user, assistant, user — the failed turn left nothing
		t.Errorf("turn 3 saw %d messages, want the pre-failure state (3)", agent.seen[2])
	}
}

// A failure with nothing committed yet has no rollback target: the conversation
// restarts, and the next turn carries the caller's history again.
func TestConversationRestartWhenNothingCommitted(t *testing.T) {
	conv := newTestConversation()
	agent := &scriptedSessionAgent{fail: func(turn int) bool { return turn == 1 }}

	if _, err := runTurn(t, conv, agent, schema.UserMessage("first")); err == nil {
		t.Fatal("turn 1 was scripted to fail")
	}
	if conv.id == "test" {
		t.Errorf("conversation id = %q, want a fresh one after an unrecoverable failure", conv.id)
	}
	if !conv.needsSeed() {
		t.Error("a restarted conversation must re-seed the caller's history")
	}
	base := []*schema.Message{schema.UserMessage("who signed?")}
	got := conv.turnMessages(base, "Final Answer: **1897**", "FIX IT")
	if len(got) != 3 || got[0].Content != "who signed?" || got[2].Content != "FIX IT" {
		t.Fatalf("reseeded turn = %v, want history + final + directive", got)
	}
	if _, err := runTurn(t, conv, agent, got...); err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if agent.seen[1] != 3 {
		t.Errorf("restarted turn saw %d messages, want exactly what it handed over", agent.seen[1])
	}
}

// The caller's chat history belongs to ONE turn: later turns hand over nothing
// but their own message.
func TestConversationTurnMessagesSeedsOnce(t *testing.T) {
	conv := newTestConversation()
	base := []*schema.Message{schema.UserMessage("who signed?")}

	first := conv.turnMessages(base, "", "FIX IT")
	if len(first) != 2 || first[0].Content != "who signed?" || first[1].Content != "FIX IT" {
		t.Fatalf("unseeded turn = %v, want history + directive", first)
	}
	second := conv.turnMessages(base, "Final Answer: **1897**", "AGAIN")
	if len(second) != 1 || second[0].Content != "AGAIN" {
		t.Fatalf("seeded turn = %v, want the directive alone", second)
	}
}

// lastAssistant reads the recovery path straight out of the session: the
// newest assistant message that carries a cited answer, even when later turns
// were pure narration.
func TestConversationLastAssistant(t *testing.T) {
	conv := newTestConversation()
	const deliverable = "The pass holds 14 people.\n\nchunk_id: abc123"
	agent := &scriptedSessionAgent{out: func(turn int) string {
		if turn == 1 {
			return deliverable
		}
		return "Let me also check the Huarong Pass scene."
	}}
	if _, err := runTurn(t, conv, agent, schema.UserMessage("q")); err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if _, err := runTurn(t, conv, agent, schema.UserMessage("keep going")); err != nil {
		t.Fatalf("turn 2: %v", err)
	}

	ctx := context.Background()
	answer := func(m *schema.Message) bool {
		return strings.TrimSpace(m.Content) != "" && strings.Contains(m.Content, "chunk_id:")
	}
	if got := conv.lastAssistant(ctx, answer); got != deliverable {
		t.Errorf("lastAssistant(answer) = %q, want the earlier deliverable", got)
	}
	substantive := func(m *schema.Message) bool { return strings.TrimSpace(m.Content) != "" }
	if got := conv.lastAssistant(ctx, substantive); strings.Contains(got, "chunk_id:") || got == "" {
		t.Errorf("lastAssistant(substantive) = %q, want the narration tail", got)
	}
}

func TestConversationHeadOnEmptySession(t *testing.T) {
	conv := newTestConversation()
	if got := conv.head(context.Background()); got != "" {
		t.Errorf("head of an empty conversation = %q, want empty", got)
	}
	if got := conv.lastAssistant(context.Background(), func(*schema.Message) bool { return true }); got != "" {
		t.Errorf("empty conversation has no assistant message, got %q", got)
	}
}
