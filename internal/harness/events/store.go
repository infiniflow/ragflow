package events

import (
	"context"
	"slices"
	"time"
)

// EventLog is the append-only event log interface.
// All implementations must be safe for concurrent use.
type EventLog interface {
	// Append appends one or more events to the log. Events are immutable
	// once appended.
	Append(ctx context.Context, events ...*Event) error

	// Stream returns an iterator over events matching the filter,
	// ordered by logical clock.
	Stream(ctx context.Context, filter EventFilter) EventIterator

	// Get retrieves a single event by ID. Returns nil, nil if not found.
	Get(ctx context.Context, id EventID) (*Event, error)

	// Range returns events with logical clock in [from, to] matching the filter.
	Range(ctx context.Context, from, to uint64, filter EventFilter) ([]*Event, error)

	// Seek returns an iterator starting from the given logical clock.
	Seek(ctx context.Context, clock uint64) (EventIterator, error)

	// Length returns the total number of events in the log.
	Length(ctx context.Context) (uint64, error)
}

// EventFilter specifies criteria for filtering events.
type EventFilter struct {
	// TraceID filters by trace.
	TraceID string
	// ThreadID filters by thread.
	ThreadID string
	// Types restricts to specific event types. Empty means all types.
	Types []EventType
	// Node restricts to a specific graph node.
	Node string
	// FromClock is the minimum logical clock (inclusive).
	FromClock uint64
	// ToClock is the maximum logical clock (inclusive). 0 means no upper bound.
	ToClock uint64
	// FromTime is the minimum wall-clock time.
	FromTime time.Time
	// ToTime is the maximum wall-clock time.
	ToTime time.Time
	// Limit caps the number of events returned. 0 means no limit.
	Limit int
}

// Matches checks whether an event matches this filter.
func (f EventFilter) Matches(e *Event) bool {
	if f.TraceID != "" && e.TraceID != f.TraceID {
		return false
	}
	if f.ThreadID != "" && e.ThreadID != f.ThreadID {
		return false
	}
	if len(f.Types) > 0 {
		if !slices.Contains(f.Types, e.Type) {
			return false
		}
	}
	if f.Node != "" && e.Node != f.Node {
		return false
	}
	if f.FromClock > 0 && e.Clock < f.FromClock {
		return false
	}
	if f.ToClock > 0 && e.Clock > f.ToClock {
		return false
	}
	if !f.FromTime.IsZero() && e.Timestamp.Before(f.FromTime) {
		return false
	}
	if !f.ToTime.IsZero() && e.Timestamp.After(f.ToTime) {
		return false
	}
	return true
}

// EventIterator allows iterating over events in order.
type EventIterator interface {
	// Next returns the next event. Returns nil, false when exhausted.
	Next(ctx context.Context) (*Event, bool)
	// Close releases resources held by the iterator.
	Close() error
}

// Snapshot represents a point-in-time snapshot of event state,
// used to accelerate replay (avoids replaying from event 0).
type Snapshot struct {
	ID        string    `json:"id"`
	TraceID   string    `json:"trace_id"`
	Clock     uint64    `json:"clock"`
	CreatedAt time.Time `json:"created_at"`
	Data      []byte    `json:"data,omitempty"`
}

// EventStore extends EventLog with lifecycle management.
type EventStore interface {
	EventLog

	// CreateSnapshot creates a snapshot for the given trace.
	CreateSnapshot(ctx context.Context, traceID string) (*Snapshot, error)

	// RestoreSnapshot loads a snapshot and returns an iterator positioned
	// after the snapshot's clock position.
	RestoreSnapshot(ctx context.Context, snapshotID string) (EventIterator, error)

	// Subscribe returns a channel that receives new events matching the filter
	// as they are appended. The channel is closed when the context is cancelled.
	Subscribe(ctx context.Context, filter EventFilter) (<-chan *Event, error)

	// GC removes events older than the given retention period.
	GC(ctx context.Context, retention time.Duration) error
}
