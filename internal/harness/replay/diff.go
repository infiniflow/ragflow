package replay

import (
	"context"

	"ragflow/internal/harness/events"
)

// DiffResult contains the comparison of two execution traces.
type DiffResult struct {
	// LeftTraceID identifies the left (reference) trace.
	LeftTraceID string

	// RightTraceID identifies the right (candidate) trace.
	RightTraceID string

	// MissingInRight are events present in the left trace but absent in the right.
	MissingInRight []*events.Event

	// MissingInLeft are events present in the right trace but absent in the left.
	MissingInLeft []*events.Event

	// Mismatched are events that exist in both traces but have different payloads.
	Mismatched []EventMismatch

	// StateDiff captures differences in state transitions.
	StateDiff map[string]StateDiff

	// ToolCallDiff captures differences in tool invocations.
	ToolCallDiff []ToolCallDiff

	// LLMResponseDiff captures differences in LLM responses.
	LLMResponseDiff []LLMResponseDiff

	// FinalOutputDiff is the difference in the final output (empty when identical).
	FinalOutputDiff string
}

// EventMismatch describes a single event-level difference between two traces.
type EventMismatch struct {
	Clock      uint64
	LeftEvent  *events.Event
	RightEvent *events.Event
	Field      string
	LeftValue  string
	RightValue string
}

// StateDiff describes a difference in state at a specific point.
type StateDiff struct {
	Clock      uint64
	Key        string
	LeftValue  any
	RightValue any
}

// ToolCallDiff describes a difference in a tool invocation between two traces.
type ToolCallDiff struct {
	Index       int
	ToolName    string
	LeftResult  any
	RightResult any
	LeftError   string
	RightError  string
}

// LLMResponseDiff describes a difference in an LLM response between two traces.
type LLMResponseDiff struct {
	Index        int
	LeftContent  string
	RightContent string
}

// Diff compares two execution traces from the same event store.
// It identifies events that are present in one trace but not the other,
// and events that exist in both but differ in content.
func Diff(ctx context.Context, left, right events.EventLog, leftTraceID, rightTraceID string) (*DiffResult, error) {
	result := &DiffResult{
		LeftTraceID:  leftTraceID,
		RightTraceID: rightTraceID,
		StateDiff:    make(map[string]StateDiff),
	}

	// Collect events from both traces.
	leftEvents, err := readAllEvents(ctx, left, leftTraceID)
	if err != nil {
		return nil, err
	}
	rightEvents, err := readAllEvents(ctx, right, rightTraceID)
	if err != nil {
		return nil, err
	}

	// Build lookup maps.
	leftByClock := make(map[uint64]*events.Event)
	for _, ev := range leftEvents {
		leftByClock[ev.Clock] = ev
	}
	rightByClock := make(map[uint64]*events.Event)
	for _, ev := range rightEvents {
		rightByClock[ev.Clock] = ev
	}

	// Collect all clock values.
	allClocks := make(map[uint64]bool)
	for _, ev := range leftEvents {
		allClocks[ev.Clock] = true
	}
	for _, ev := range rightEvents {
		allClocks[ev.Clock] = true
	}

	// Compare event by event.
	for clock := range allClocks {
		leftEv, leftOk := leftByClock[clock]
		rightEv, rightOk := rightByClock[clock]

		switch {
		case leftOk && !rightOk:
			result.MissingInRight = append(result.MissingInRight, leftEv)
		case !leftOk && rightOk:
			result.MissingInLeft = append(result.MissingInLeft, rightEv)
		case leftOk && rightOk:
			// Both exist — compare.
			if leftEv.Type != rightEv.Type {
				result.Mismatched = append(result.Mismatched, EventMismatch{
					Clock:      clock,
					LeftEvent:  leftEv,
					RightEvent: rightEv,
					Field:      "type",
					LeftValue:  string(leftEv.Type),
					RightValue: string(rightEv.Type),
				})
			}
			if leftEv.Hash != rightEv.Hash {
				result.Mismatched = append(result.Mismatched, EventMismatch{
					Clock:      clock,
					LeftEvent:  leftEv,
					RightEvent: rightEv,
					Field:      "payload",
					LeftValue:  leftEv.Hash[:16],
					RightValue: rightEv.Hash[:16],
				})
			}

			// Categorise by event type.
			switch leftEv.Type {
			case events.EventLLMCallEnd:
				result.LLMResponseDiff = append(result.LLMResponseDiff, LLMResponseDiff{
					Index:        len(result.LLMResponseDiff),
					LeftContent:  extractContent(leftEv),
					RightContent: extractContent(rightEv),
				})
			case events.EventToolCallResult:
				result.ToolCallDiff = append(result.ToolCallDiff, ToolCallDiff{
					Index:    len(result.ToolCallDiff),
					ToolName: extractToolName(leftEv),
				})
			case events.EventStateWrite:
				if leftEv.Node != "" {
					result.StateDiff[leftEv.Node] = StateDiff{
						Clock: clock,
						Key:   leftEv.Node,
					}
				}
			}
		}
	}

	return result, nil
}

// readAllEvents reads all events for a trace from the store.
func readAllEvents(ctx context.Context, store events.EventLog, traceID string) ([]*events.Event, error) {
	iter := store.Stream(ctx, events.EventFilter{TraceID: traceID})
	defer iter.Close()

	var result []*events.Event
	for {
		ev, ok := iter.Next(ctx)
		if !ok {
			break
		}
		result = append(result, ev)
	}
	return result, nil
}

// extractContent extracts the Content field from an LLMCallPayload event.
func extractContent(ev *events.Event) string {
	if ev.Payload == nil {
		return ""
	}
	var payload events.LLMCallPayload
	if err := jsonUnmarshal(ev.Payload, &payload); err != nil {
		return ""
	}
	return payload.Content
}

// extractToolName extracts the ToolName field from a ToolCallPayload event.
func extractToolName(ev *events.Event) string {
	if ev.Payload == nil {
		return ""
	}
	var payload events.ToolCallPayload
	if err := jsonUnmarshal(ev.Payload, &payload); err != nil {
		return ""
	}
	return payload.ToolName
}
