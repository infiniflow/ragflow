package events

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// MemoryEventStore is an in-memory EventStore implementation.
// Suitable for testing and single-instance development.
// All events are lost on process restart.
type MemoryEventStore struct {
	mu     sync.RWMutex
	events []*Event
	byID   map[EventID]*Event
	clock  atomic.Uint64
	subs   map[int64]chan *Event
	subID  atomic.Int64
}

// NewMemoryEventStore creates a new empty MemoryEventStore.
func NewMemoryEventStore() *MemoryEventStore {
	return &MemoryEventStore{
		events: make([]*Event, 0, 1024),
		byID:   make(map[EventID]*Event),
		subs:   make(map[int64]chan *Event),
	}
}

// Append implements EventLog.
func (s *MemoryEventStore) Append(ctx context.Context, events ...*Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, ev := range events {
		if ev.Clock == 0 {
			ev.Clock = s.clock.Add(1)
		}
		ev.Seal()
		s.events = append(s.events, ev)
		s.byID[ev.ID] = ev

		// Dispatch to subscribers.
		for id, ch := range s.subs {
			select {
			case ch <- ev:
			default:
				// Drop slow subscriber.
			}
			_ = id
		}
	}
	return nil
}

// Stream implements EventLog.
func (s *MemoryEventStore) Stream(ctx context.Context, filter EventFilter) EventIterator {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filtered := make([]*Event, 0)
	for _, ev := range s.events {
		if filter.Matches(ev) {
			filtered = append(filtered, ev)
		}
	}
	return &sliceIterator{events: filtered, pos: 0}
}

// Get implements EventLog.
func (s *MemoryEventStore) Get(ctx context.Context, id EventID) (*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ev, ok := s.byID[id]
	if !ok {
		return nil, nil
	}
	return ev, nil
}

// Range implements EventLog.
func (s *MemoryEventStore) Range(ctx context.Context, from, to uint64, filter EventFilter) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Event, 0)
	for _, ev := range s.events {
		if ev.Clock < from {
			continue
		}
		if to > 0 && ev.Clock > to {
			continue
		}
		if filter.Matches(ev) {
			result = append(result, ev)
		}
	}
	return result, nil
}

// Seek implements EventLog.
func (s *MemoryEventStore) Seek(ctx context.Context, clock uint64) (EventIterator, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pos := 0
	for i, ev := range s.events {
		if ev.Clock >= clock {
			pos = i
			break
		}
		_ = i
	}
	return &sliceIterator{events: s.events[pos:], pos: 0}, nil
}

// Length implements EventLog.
func (s *MemoryEventStore) Length(ctx context.Context) (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return uint64(len(s.events)), nil
}

// CreateSnapshot implements EventStore.
func (s *MemoryEventStore) CreateSnapshot(ctx context.Context, traceID string) (*Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clock := s.clock.Load()
	data, err := json.Marshal(s.events)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}
	return &Snapshot{
		ID:        fmt.Sprintf("snap-%s-%d", traceID, clock),
		TraceID:   traceID,
		Clock:     clock,
		CreatedAt: time.Now(),
		Data:      data,
	}, nil
}

// RestoreSnapshot implements EventStore.
func (s *MemoryEventStore) RestoreSnapshot(ctx context.Context, snapshotID string) (EventIterator, error) {
	// For MemoryEventStore, we simply seek past the snapshot's clock.
	// The snapshot data itself is not needed since events are still in memory.
	return s.Seek(ctx, 0)
}

// Subscribe implements EventStore.
func (s *MemoryEventStore) Subscribe(ctx context.Context, filter EventFilter) (<-chan *Event, error) {
	ch := make(chan *Event, 256)
	id := s.subID.Add(1)

	s.mu.Lock()
	s.subs[id] = ch
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		delete(s.subs, id)
		s.mu.Unlock()
		close(ch)
	}()

	return ch, nil
}

// GC implements EventStore.
func (s *MemoryEventStore) GC(ctx context.Context, retention time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-retention)
	keep := make([]*Event, 0, len(s.events))
	for _, ev := range s.events {
		if ev.Timestamp.After(cutoff) {
			keep = append(keep, ev)
		} else {
			delete(s.byID, ev.ID)
		}
	}
	s.events = keep
	return nil
}

// ---- sliceIterator ----

type sliceIterator struct {
	events []*Event
	pos    int
}

func (it *sliceIterator) Next(_ context.Context) (*Event, bool) {
	if it.pos >= len(it.events) {
		return nil, false
	}
	ev := it.events[it.pos]
	it.pos++
	return ev, true
}

func (it *sliceIterator) Close() error {
	it.events = nil
	return nil
}
