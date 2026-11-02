package events

import (
	"slices"
	"testing"
	"time"
)

// TestEventStoreSeekBoundaries checks both local EventLog implementations.
// Seeking past the final logical clock must yield no events, not a replay
// of the entire log.
func TestEventStoreSeekBoundaries(t *testing.T) {
	stores := []struct {
		name string
		new  func(*testing.T) EventLog
	}{
		{"memory", func(t *testing.T) EventLog { return NewMemoryEventStore() }},
		{"local-file", func(t *testing.T) EventLog {
			s, err := NewLocalFileEventStore(t.TempDir())
			if err != nil {
				t.Fatalf("new local store: %v", err)
			}
			return s
		}},
	}

	for _, tc := range stores {
		t.Run(tc.name, func(t *testing.T) {
			store := tc.new(t)
			ctx := t.Context()

			// Empty logs always return an exhausted iterator.
			empty, err := store.Seek(ctx, 1)
			if err != nil {
				t.Fatalf("seek empty log: %v", err)
			}
			if event, ok := empty.Next(ctx); ok {
				t.Fatalf("empty log returned event %+v", event)
			}
			_ = empty.Close()

			if err := store.Append(ctx,
				&Event{ID: "seek-1", Type: EventStepStart, Clock: 1, TraceID: "seek", Timestamp: time.Now()},
				&Event{ID: "seek-3", Type: EventStepEnd, Clock: 3, TraceID: "seek", Timestamp: time.Now()},
			); err != nil {
				t.Fatalf("append events: %v", err)
			}

			cases := []struct {
				clock uint64
				want  []uint64
			}{
				{0, []uint64{1, 3}},
				{1, []uint64{1, 3}},
				{2, []uint64{3}},
				{3, []uint64{3}},
				{4, nil},
			}
			for _, c := range cases {
				iter, err := store.Seek(ctx, c.clock)
				if err != nil {
					t.Fatalf("seek clock %d: %v", c.clock, err)
				}
				var got []uint64
				for {
					event, ok := iter.Next(ctx)
					if !ok {
						break
					}
					got = append(got, event.Clock)
				}
				if err := iter.Close(); err != nil {
					t.Fatalf("close iterator at %d: %v", c.clock, err)
				}
				if !slices.Equal(got, c.want) {
					t.Fatalf("seek clock %d: got %v, want %v", c.clock, got, c.want)
				}
			}
		})
	}
}
