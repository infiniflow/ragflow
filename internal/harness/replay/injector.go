package replay

import (
	"encoding/json"
	"fmt"

	"ragflow/internal/harness/events"
)

// ---- common overrides ----

// ReplayExactTools returns a ToolOverrideFunc that uses the recorded result
// unchanged. This is the default behaviour for deterministic replay.
func ReplayExactTools() ToolOverrideFunc {
	return func(toolName string, args map[string]any, recordedResult any) (any, error) {
		return recordedResult, nil
	}
}

// ReplayLiveTools returns a ToolOverrideFunc that always returns nil,
// signalling the replay to execute the tool with the real implementation.
func ReplayLiveTools() ToolOverrideFunc {
	return func(toolName string, args map[string]any, recordedResult any) (any, error) {
		// Return nil to indicate "execute live".
		return nil, nil
	}
}

// ReplaySubstituteModel returns a ModelOverrideFunc that replaces the
// recorded LLM response with a fixed string. Use this to compare how
// a different model would change behaviour while keeping tool results frozen.
//
// The callback receives the original recorded response and should return
// the substitute response. Return ("", nil) to suppress the response.
type ReplayModelCallback func(recordedResponse string) string

// ReplaySubstituteModel creates a ModelOverrideFunc from a callback.
func ReplaySubstituteModel(fn ReplayModelCallback) ModelOverrideFunc {
	return func(_ []any, recordedResponse string) (*string, error) {
		substituted := fn(recordedResponse)
		return &substituted, nil
	}
}

// ---- error types ----

type replayError struct {
	msg string
}

func (e *replayError) Error() string { return e.msg }

func errorf(format string, args ...any) error {
	return &replayError{msg: fmt.Sprintf(format, args...)}
}

// ---- helpers ----

func jsonUnmarshal(data []byte, target any) error {
	return json.Unmarshal(data, target)
}

func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// copyEvent creates a shallow copy of an Event with a deep copy of Payload and Metadata.
func copyEvent(ev *events.Event) *events.Event {
	cp := *ev
	if ev.Payload != nil {
		cp.Payload = make([]byte, len(ev.Payload))
		copy(cp.Payload, ev.Payload)
	}
	if ev.Metadata != nil {
		cp.Metadata = make(map[string]any, len(ev.Metadata))
		for k, v := range ev.Metadata {
			cp.Metadata[k] = v
		}
	}
	if ev.CausedBy != nil {
		cp.CausedBy = make([]events.EventID, len(ev.CausedBy))
		copy(cp.CausedBy, ev.CausedBy)
	}
	return &cp
}

// ---- event helpers for test assertions ----

// FindEventsOfType filters events by type.
func FindEventsOfType(evts []*events.Event, typ events.EventType) []*events.Event {
	var result []*events.Event
	for _, ev := range evts {
		if ev.Type == typ {
			result = append(result, ev)
		}
	}
	return result
}

// EventsContains checks if any event has the given type.
func EventsContains(evts []*events.Event, typ events.EventType) bool {
	for _, ev := range evts {
		if ev.Type == typ {
			return true
		}
	}
	return false
}

// EventCount counts events of a given type.
func EventCount(evts []*events.Event, typ events.EventType) int {
	count := 0
	for _, ev := range evts {
		if ev.Type == typ {
			count++
		}
	}
	return count
}
