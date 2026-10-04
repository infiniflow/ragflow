//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package agentic_rag

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/session"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"ragflow/internal/common"
)

// sessionLogPageSize caps one LoadEvents page. Small pages exercise
// pagination and keep every read bounded: the scans below are terminated by
// what they are looking for, never by the log's length.
const sessionLogPageSize = 32

// conversation is ONE agent conversation a Runner keeps for us (eino v0.10
// session mode): the caller hands a turn nothing but its NEW messages and the
// Runner replays everything older from the session's event log.
//
// It replaces the hand-rolled transcript this package used to carry: an adk
// ChatModelAgent keeps NO state between Run calls, so every turn used to be
// assembled by hand — appending the previous turns, trimming the dangling
// tool-call tail an aborted run leaves behind, and reordering parallel tool
// results into call order (providers match results to calls by POSITION, and
// concurrent tool calls complete out of order). All three are now the
// framework's job: the event log is append-only and replayed in canonical
// order, so every replay is provider-valid by construction.
//
// Two invariants are still ours:
//
//   - A FAILED turn must not survive into the next one: an aborted run can
//     leave tool calls whose results never arrived, and replaying those is a
//     provider error. discardFailedTurn therefore rolls the failed turn back
//     out of the log before the next one reads it.
//   - The caller's chat history (a compacted prior conversation from the DB)
//     belongs to the FIRST turn only; restart marks the conversation unseeded
//     so the next turn carries it again.
type conversation struct {
	// kind names the conversation in logs ("explorer").
	kind string
	// store is the run's event log; both conversations of a run share it and
	// are told apart by id.
	store adk.SessionEventStore[adk.Message]
	// id is the session id inside store.
	id string
	// seq counts restarts (see restart).
	seq int
	// seeded records whether the caller's chat history has already been
	// handed to this conversation. The first turn after a restart carries it;
	// every later turn hands over only new messages.
	seeded bool
}

func newConversation(kind string, store adk.SessionEventStore[adk.Message]) *conversation {
	return &conversation{kind: kind, store: store, id: kind}
}

// runSession owns the event log one agentic turn maintains, plus the
// conversation that writes into it: the explorer's (research turns, one per
// Run).
//
// The store is deliberately process-local and request-scoped: the durable
// boundary of a conversation is the caller's REQUEST, where the shipped answer
// is compacted into the assistant message the next request replays from the
// DB. Nothing here outlives one Run.
type runSession struct {
	store    *session.InMemoryStore[adk.Message]
	explorer *conversation
}

func newRunSession() *runSession {
	store := session.NewInMemoryStore[adk.Message](nil)
	return &runSession{
		store:    store,
		explorer: newConversation("explorer", store),
	}
}

// runner binds an agent to this conversation for ONE turn. Callers pick the
// streaming mode per turn (the main loop streams its research live; the gate's
// repair and audit turns stay non-streaming) — the session history is carried
// by (store, id), not by the runner.
func (c *conversation) runner(ctx context.Context, agent adk.Agent, enableStreaming bool) *adk.Runner {
	return adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: enableStreaming,
		SessionID:       c.id,
		SessionStore:    c.store,
	})
}

// head returns the EventID of the newest COMMITTED turn boundary — the idle
// event of a turn that ended normally — or "" when the conversation has none.
// It is the point a later turn can be rolled back to.
//
// The scan reads the raw log, which is safe for how this package uses it: a
// rollback only ever targets the head captured immediately BEFORE the failing
// turn, and everything after it belongs to a turn that never committed.
func (c *conversation) head(ctx context.Context) string {
	var after string
	for {
		res, err := c.store.LoadEvents(ctx, c.id, &adk.LoadSessionEventsRequest{
			Kinds:   []adk.SessionEventKind{adk.SessionEventSessionStatusIdle},
			After:   after,
			Limit:   sessionLogPageSize,
			Reverse: true,
		})
		if err != nil {
			if !errors.Is(err, adk.ErrEventIDOutOfRange) {
				common.WarnCtx(ctx, "agentic_rag: session head scan failed",
					zap.String("conversation", c.kind), zap.Error(err))
			}
			return ""
		}
		if res == nil {
			return ""
		}
		for _, event := range res.Events {
			if committedTurnBoundary(event) {
				return event.EventID
			}
		}
		if res.Next == "" {
			return ""
		}
		after = res.Next
	}
}

// discardFailedTurn removes the turn that just failed from this conversation:
// without it the NEXT turn replays whatever the failure left behind — tool
// calls whose results never arrived, which providers reject outright.
//
// With a committed predecessor the turn is rolled back to it. Without one
// (nothing committed yet, typically a failure during the very first turn) the
// log holds nothing but debris, so the conversation restarts under a fresh id
// — the durable equivalent of dropping it — and the next turn re-seeds.
func (c *conversation) discardFailedTurn(ctx context.Context, head string) {
	if head != "" {
		if err := adk.RollbackSession(ctx, c.store, c.id, head); err != nil {
			common.WarnCtx(ctx, "agentic_rag: session rollback failed",
				zap.String("conversation", c.kind), zap.Error(err))
		}
		return
	}
	c.restart()
}

func (c *conversation) restart() {
	c.seq++
	c.id = fmt.Sprintf("%s#%d", c.kind, c.seq)
	c.seeded = false
}

// needsSeed reports whether the caller's chat history still has to go into the
// next turn (see turnMessages).
func (c *conversation) needsSeed() bool {
	return !c.seeded
}

func (c *conversation) markSeeded() {
	c.seeded = true
}

// turnMessages renders what this turn hands the Runner: only its NEW messages.
// An unseeded conversation carries the caller's chat history and the standing
// deliverable first — a repair turn that lost its predecessor's events to a
// restart must still see the question it is answering — every later turn sends
// the directive alone and lets the Runner replay the rest.
func (c *conversation) turnMessages(seed []*schema.Message, final, directive string) []adk.Message {
	if c.seeded {
		return []adk.Message{schema.UserMessage(directive)}
	}
	c.markSeeded()
	msgs := make([]adk.Message, 0, len(seed)+2)
	msgs = append(msgs, seed...)
	if strings.TrimSpace(final) != "" {
		// An empty assistant message trips some provider APIs; with no
		// standing deliverable the directive alone carries the ask.
		msgs = append(msgs, schema.AssistantMessage(final, nil))
	}
	return append(msgs, schema.UserMessage(directive))
}

// lastAssistant walks the conversation's message events NEWEST FIRST and
// returns the content of the first assistant message pick accepts — the
// recovery ladder's view of the run (see Run): everything the explorer ever
// said, including turns whose checkpoints are already gone from `final`.
func (c *conversation) lastAssistant(ctx context.Context, pick func(*schema.Message) bool) string {
	var after string
	for {
		res, err := c.store.LoadEvents(ctx, c.id, &adk.LoadSessionEventsRequest{
			Kinds:   []adk.SessionEventKind{adk.SessionEventMessage},
			After:   after,
			Limit:   sessionLogPageSize,
			Reverse: true,
		})
		if err != nil {
			if !errors.Is(err, adk.ErrEventIDOutOfRange) {
				common.WarnCtx(ctx, "agentic_rag: session message scan failed",
					zap.String("conversation", c.kind), zap.Error(err))
			}
			return ""
		}
		if res == nil {
			return ""
		}
		for _, event := range res.Events {
			msg := event.Message
			if msg == nil || msg.Role != schema.Assistant || len(msg.ToolCalls) > 0 {
				continue
			}
			if pick(msg) {
				return msg.Content
			}
		}
		if res.Next == "" {
			return ""
		}
		after = res.Next
	}
}

// committedTurnBoundary reports whether a session event is the idle marker of
// a turn that ended normally — the only point RollbackSession accepts.
func committedTurnBoundary(event *adk.SessionEvent[adk.Message]) bool {
	return event != nil &&
		event.Kind == adk.SessionEventSessionStatusIdle &&
		event.Lifecycle != nil &&
		event.Lifecycle.State == adk.SessionRunStateIdle &&
		event.Lifecycle.StopReason != nil &&
		event.Lifecycle.StopReason.Type == adk.StopReasonEndTurn
}

// toolResultText concatenates the tool results this conversation read, oldest
// first - the haystack for the delivery gate's grounding check: everything the
// corpus actually returned to the agent. Tool results are stored as messages
// with the Tool role; assistant prose and user turns are skipped.
func (c *conversation) toolResultText(ctx context.Context) string {
	var b strings.Builder
	var after string
	for {
		res, err := c.store.LoadEvents(ctx, c.id, &adk.LoadSessionEventsRequest{
			Kinds: []adk.SessionEventKind{adk.SessionEventMessage},
			After: after,
			Limit: sessionLogPageSize,
		})
		if err != nil || res == nil {
			return b.String()
		}
		for _, event := range res.Events {
			msg := event.Message
			if msg == nil || msg.Role != schema.Tool || msg.Content == "" {
				continue
			}
			b.WriteString(msg.Content)
			b.WriteString("\n")
		}
		if res.Next == "" {
			return b.String()
		}
		after = res.Next
	}
}
