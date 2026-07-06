// Package replay provides deterministic replay, fork, and diff for
// agent execution traces recorded by the events package.
//
// A ReplayEngine replays events from an EventLog, optionally substituting
// model responses or tool results. Fork creates a branched execution from
// any point in the trace. Diff compares two execution traces to detect
// regression or behavioral changes.
package replay

import (
	"context"
	"time"

	"ragflow/internal/harness/events"
)

// ReplayConfig configures a deterministic replay.
type ReplayConfig struct {
	// Store is the event source to replay from.
	Store events.EventLog

	// TraceID identifies the trace to replay.
	TraceID string

	// Start is the starting logical clock (0 = from beginning).
	Start uint64

	// End is the ending logical clock (0 = to end).
	End uint64

	// Substitution strategies.
	ModelOverride ModelOverrideFunc
	ToolOverride  ToolOverrideFunc
	StateOverride StateOverrideFunc

	// OutputStore receives events generated during replay (nil = discard).
	OutputStore events.EventLog

	// DiffEnabled compares replayed events with original trace.
	DiffEnabled bool
}

// ModelOverrideFunc replaces LLM model responses during replay.
// Return a non-nil *string to use the substituted response.
// Return nil, nil to use the recorded response.
type ModelOverrideFunc func(messages []any, recordedResponse string) (*string, error)

// ToolOverrideFunc replaces tool execution results during replay.
// Return a non-nil value to use the substituted result.
// Return nil to use the recorded result.
type ToolOverrideFunc func(toolName string, args map[string]any, recordedResult any) (any, error)

// StateOverrideFunc replaces initial state during replay.
// Return the modified state, or nil to keep the recorded state.
type StateOverrideFunc func(recordedState map[string]any) (map[string]any, error)

// ReplayResult contains the result of a deterministic replay.
type ReplayResult struct {
	// Events generated during replay (when OutputStore is set).
	Events []*events.Event

	// OriginalLen is the number of events in the original trace.
	OriginalLen int

	// ReplayLen is the number of events generated during replay.
	ReplayLen int

	// Divergences between replayed and original events (when DiffEnabled).
	Divergences []EventDivergence

	// ReplayMetrics contains metrics about the replay operation.
	ReplayMetrics ReplayMetrics

	// Duration of the replay operation.
	Duration time.Duration
}

// EventDivergence describes a difference between original and replayed events.
type EventDivergence struct {
	// Clock position in the event log.
	Clock uint64

	// Original event (nil when the event is new in replay).
	OriginalEvent *events.Event

	// Replay event (nil when the original event was skipped).
	ReplayEvent *events.Event

	// Type of divergence.
	Type DivergenceType

	// Description explains the difference.
	Description string
}

// DivergenceType categorises event divergences.
type DivergenceType string

const (
	// DivergenceMissing means the original event is absent in replay.
	DivergenceMissing DivergenceType = "missing"
	// DivergenceExtra means the replay produced an event not in the original.
	DivergenceExtra DivergenceType = "extra"
	// DivergenceMismatch means the event exists in both but differs.
	DivergenceMismatch DivergenceType = "mismatch"
)

// ReplayMetrics contains metrics about a replay operation.
type ReplayMetrics struct {
	TotalEvents     int
	DivergenceCount int
	MatchCount      int
}

// ReplayEngine replays execution traces from an EventLog.
type ReplayEngine struct {
	store events.EventLog
}

// NewReplayEngine creates a ReplayEngine backed by the given event store.
func NewReplayEngine(store events.EventLog) *ReplayEngine {
	return &ReplayEngine{store: store}
}

// Replay executes a deterministic replay of the given trace.
// It replays events from the EventLog sequentially, optionally calling
// ModelOverride and ToolOverride to substitute non-deterministic operations.
func (e *ReplayEngine) Replay(ctx context.Context, cfg *ReplayConfig) (*ReplayResult, error) {
	start := time.Now()

	// Default to exact replay when no overrides are set.
	modelOverride := cfg.ModelOverride
	if modelOverride == nil {
		modelOverride = func(_ []any, recorded string) (*string, error) {
			return &recorded, nil
		}
	}
	toolOverride := cfg.ToolOverride
	if toolOverride == nil {
		toolOverride = func(_ string, _ map[string]any, recorded any) (any, error) {
			return recorded, nil
		}
	}

	// Use config store, falling back to engine store.
	store := cfg.Store
	if store == nil {
		store = e.store
	}

	filter := events.EventFilter{
		TraceID:   cfg.TraceID,
		FromClock: cfg.Start,
		ToClock:   cfg.End,
	}

	iter := store.Stream(ctx, filter)
	defer iter.Close()

	result := &ReplayResult{}
	var originalEvents []*events.Event
	var replayEvents []*events.Event

	// Phase 1: read original events.
	for {
		ev, ok := iter.Next(ctx)
		if !ok {
			break
		}
		originalEvents = append(originalEvents, ev)
	}
	result.OriginalLen = len(originalEvents)

	// Apply StateOverride to the first EventStateWrite event (initial state).
	// Work on a copy to preserve the original for diff.
	if cfg.StateOverride != nil {
		for i, ev := range originalEvents {
			if ev.Type == events.EventStateWrite {
				var st events.StateTransitionPayload
				if ev.Payload != nil {
					_ = jsonUnmarshal(ev.Payload, &st)
				}
				recorded := map[string]any{st.Channel: st.NewValue}
				modified, err := cfg.StateOverride(recorded)
				if err != nil {
					return nil, err
				}
				if modified != nil {
					if val, ok := modified[st.Channel]; ok {
						st.NewValue = val
						repl := copyEvent(ev)
						repl.Payload, _ = jsonMarshal(st)
						repl.Seal()
						originalEvents[i] = repl
					}
				}
				break
			}
		}
	}

	// Phase 2: replay with overrides.
	// Copy each event before modifying so the original list is preserved
	// for accurate diff comparison.
	for _, original := range originalEvents {
		replayEv := copyEvent(original)

		switch original.Type {
		case events.EventLLMCallStart, events.EventLLMCallEnd:
			// Apply model override.
			if original.Type == events.EventLLMCallEnd {
				var payload events.LLMCallPayload
				_ = parsePayload(original, &payload)
				substituted, err := modelOverride(payload.Messages, payload.Content)
				if err != nil {
					return nil, err
				}
				if substituted != nil {
					payload.Content = *substituted
					replayEv.Payload, _ = jsonMarshal(payload)
					replayEv.Seal()
				}
			}
			replayEvents = append(replayEvents, replayEv)

		case events.EventToolCallStart, events.EventToolCallResult:
			// Apply tool override.
			if original.Type == events.EventToolCallResult {
				var payload events.ToolCallPayload
				_ = parsePayload(original, &payload)
				substituted, err := toolOverride(payload.ToolName, payload.Arguments, payload.Result)
				if err != nil {
					return nil, err
				}
				if substituted != nil {
					payload.Result = substituted
					replayEv.Payload, _ = jsonMarshal(payload)
					replayEv.Seal()
				}
			}
			replayEvents = append(replayEvents, replayEv)

		default:
			replayEvents = append(replayEvents, replayEv)
		}
	}

	result.ReplayLen = len(replayEvents)

	// Phase 3: diff (optional).
	var divergences []EventDivergence
	if cfg.DiffEnabled {
		divergences = diffEventLists(originalEvents, replayEvents)
		result.Divergences = divergences
	}

	// Populate ReplayMetrics.
	divergenceCount := len(divergences)
	replayMetrics := ReplayMetrics{
		TotalEvents:     result.ReplayLen,
		DivergenceCount: divergenceCount,
		MatchCount:      result.ReplayLen - divergenceCount,
	}
	result.ReplayMetrics = replayMetrics

	// Phase 4: write to output store (optional).
	if cfg.OutputStore != nil {
		if err := cfg.OutputStore.Append(ctx, replayEvents...); err != nil {
			return nil, err
		}
		result.Events = replayEvents
	}

	result.Duration = time.Since(start)
	return result, nil
}

// parsePayload unmarshals a typed payload from an event.
func parsePayload(ev *events.Event, target any) error {
	if ev.Payload == nil {
		return nil
	}
	return jsonUnmarshal(ev.Payload, target)
}

// diffEventLists compares original and replayed event lists.
func diffEventLists(original, replayed []*events.Event) []EventDivergence {
	var divergences []EventDivergence
	maxLen := len(original)
	if len(replayed) > maxLen {
		maxLen = len(replayed)
	}

	for i := 0; i < maxLen; i++ {
		var orig *events.Event
		var replay *events.Event

		if i < len(original) {
			orig = original[i]
		}
		if i < len(replayed) {
			replay = replayed[i]
		}

		if orig == nil && replay != nil {
			divergences = append(divergences, EventDivergence{
				Clock:       replay.Clock,
				ReplayEvent: replay,
				Type:        DivergenceExtra,
				Description: "replay produced extra event",
			})
			continue
		}
		if orig != nil && replay == nil {
			divergences = append(divergences, EventDivergence{
				Clock:         orig.Clock,
				OriginalEvent: orig,
				Type:          DivergenceMissing,
				Description:   "original event missing in replay",
			})
			continue
		}

		// Both exist — compare.
		if orig.Type != replay.Type {
			divergences = append(divergences, EventDivergence{
				Clock:         orig.Clock,
				OriginalEvent: orig,
				ReplayEvent:   replay,
				Type:          DivergenceMismatch,
				Description:   "event type mismatch",
			})
		}
		if orig.Hash != replay.Hash {
			divergences = append(divergences, EventDivergence{
				Clock:         orig.Clock,
				OriginalEvent: orig,
				ReplayEvent:   replay,
				Type:          DivergenceMismatch,
				Description:   "payload mismatch (hash differs)",
			})
		}
	}

	return divergences
}
