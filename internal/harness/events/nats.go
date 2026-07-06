package events

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	defaultNATSPrefix    = "harness_events"
	natsEventSubject     = "events.event"
	natsSnapshotSubject  = "events.snapshot"
	defaultMaxCacheAge   = 10 * time.Minute
	defaultMaxCacheItems = 10000
)

// cachedEvent wraps an Event with an expiry timestamp for TTL-based cache eviction.
type cachedEvent struct {
	ev        *Event
	expiresAt time.Time
}

// NATSEventStore persists events to NATS JetStream.
// Suitable for production distributed deployments.
type NATSEventStore struct {
	conn        *nats.Conn
	js          jetstream.JetStream
	stream      string // JetStream stream name
	prefix      string // subject prefix
	mu          sync.RWMutex
	cache       map[string]*cachedEvent // ID → timestamped Event for fast Get (bounded)
	maxCacheAge time.Duration
	clock       atomic.Uint64
	subs        map[int64]*nats.Subscription
	subID       atomic.Int64
}

// NewNATSEventStore creates a new NATSEventStore.
func NewNATSEventStore(conn *nats.Conn, stream string) (*NATSEventStore, error) {
	js, err := jetstream.New(conn)
	if err != nil {
		return nil, fmt.Errorf("jetstream init: %w", err)
	}

	// Ensure the stream exists.
	_, err = js.Stream(ctxForInit(), stream)
	if err != nil {
		// Create the stream if it doesn't exist.
		_, err = js.CreateStream(ctxForInit(), jetstream.StreamConfig{
			Name:      stream,
			Subjects:  []string{fmt.Sprintf("%s.>", defaultNATSPrefix)},
			MaxAge:    7 * 24 * time.Hour, // 7 days retention
			Storage:   jetstream.FileStorage,
			Retention: jetstream.LimitsPolicy,
		})
		if err != nil {
			return nil, fmt.Errorf("create jetstream stream: %w", err)
		}
	}

	return &NATSEventStore{
		conn:   conn,
		js:     js,
		stream: stream,
		prefix: defaultNATSPrefix,
		cache:  make(map[string]*cachedEvent),
		subs:   make(map[int64]*nats.Subscription),
	}, nil
}

// ctxForInit returns a background context for NATS stream setup.
func ctxForInit() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	_ = cancel // prevent vet warning; cancel runs when ctx expires
	return ctx
}

// Append implements EventLog.
func (s *NATSEventStore) Append(ctx context.Context, events ...*Event) error {
	for _, ev := range events {
		if ev.Clock == 0 {
			ev.Clock = s.clock.Add(1)
		}
		ev.Seal()

		data, err := json.Marshal(ev)
		if err != nil {
			return fmt.Errorf("marshal event: %w", err)
		}

		subject := fmt.Sprintf("%s.%s", s.prefix, natsEventSubject)
		if _, err := s.js.Publish(ctx, subject, data); err != nil {
			return fmt.Errorf("publish event: %w", err)
		}

		// Update local cache with TTL.
		s.mu.Lock()
		maxAge := s.maxCacheAge
		if maxAge == 0 {
			maxAge = defaultMaxCacheAge
		}
		s.cache[string(ev.ID)] = &cachedEvent{ev: ev, expiresAt: time.Now().Add(maxAge)}
		// Evict expired entries when cache exceeds limit.
		if len(s.cache) > defaultMaxCacheItems {
			s.evictExpiredLocked()
		}
		s.mu.Unlock()
	}
	return nil
}

// Stream implements EventLog by replaying from JetStream.
func (s *NATSEventStore) Stream(ctx context.Context, filter EventFilter) EventIterator {
	subject := fmt.Sprintf("%s.%s", s.prefix, natsEventSubject)
	consumer, err := s.js.OrderedConsumer(ctx, s.stream, jetstream.OrderedConsumerConfig{
		FilterSubjects: []string{subject},
	})
	if err != nil {
		return &natsErrorIterator{err: fmt.Errorf("create consumer: %w", err)}
	}

	return &natsEventIterator{
		consumer: consumer,
		ctx:      ctx,
		filter:   filter,
		buffer:   make([]*Event, 0),
	}
}

// Get implements EventLog.
func (s *NATSEventStore) Get(ctx context.Context, id EventID) (*Event, error) {
	s.mu.RLock()
	ce, ok := s.cache[string(id)]
	s.mu.RUnlock()
	if ok && ce.expiresAt.After(time.Now()) {
		return ce.ev, nil
	}
	// Expired cache entry — remove it.
	if ok {
		s.mu.Lock()
		delete(s.cache, string(id))
		s.mu.Unlock()
	}

	// Fall back to stream scan (slow path).
	iter := s.Stream(ctx, EventFilter{Limit: 1})
	defer iter.Close()
	for {
		e, ok := iter.Next(ctx)
		if !ok {
			break
		}
		if e.ID == id {
			return e, nil
		}
	}
	return nil, nil
}

// Range implements EventLog.
func (s *NATSEventStore) Range(ctx context.Context, from, to uint64, filter EventFilter) ([]*Event, error) {
	iter := s.Stream(ctx, filter)
	defer iter.Close()

	result := make([]*Event, 0)
	for {
		ev, ok := iter.Next(ctx)
		if !ok {
			break
		}
		if ev.Clock < from {
			continue
		}
		if to > 0 && ev.Clock > to {
			continue
		}
		result = append(result, ev)
	}
	return result, nil
}

// Seek implements EventLog.
func (s *NATSEventStore) Seek(ctx context.Context, clock uint64) (EventIterator, error) {
	return s.Stream(ctx, EventFilter{FromClock: clock}), nil
}

// Length implements EventLog by counting messages in the stream.
func (s *NATSEventStore) Length(ctx context.Context) (uint64, error) {
	streamInfo, err := s.js.Stream(ctx, s.stream)
	if err != nil {
		return 0, fmt.Errorf("get stream info: %w", err)
	}
	return streamInfo.CachedInfo().State.Msgs, nil
}

// CreateSnapshot implements EventStore.
func (s *NATSEventStore) CreateSnapshot(ctx context.Context, traceID string) (*Snapshot, error) {
	clock := s.clock.Load()

	// Collect all events for the trace.
	iter := s.Stream(ctx, EventFilter{TraceID: traceID})
	defer iter.Close()

	var traceEvents []*Event
	for {
		ev, ok := iter.Next(ctx)
		if !ok {
			break
		}
		traceEvents = append(traceEvents, ev)
	}

	data, err := json.Marshal(traceEvents)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}

	snap := &Snapshot{
		ID:        fmt.Sprintf("snap-%s-%d", traceID, clock),
		TraceID:   traceID,
		Clock:     clock,
		CreatedAt: time.Now(),
		Data:      data,
	}

	snapData, _ := json.Marshal(snap)
	subject := fmt.Sprintf("%s.%s.%s", s.prefix, natsSnapshotSubject, traceID)
	s.js.Publish(ctx, subject, snapData)

	return snap, nil
}

// RestoreSnapshot implements EventStore.
func (s *NATSEventStore) RestoreSnapshot(ctx context.Context, snapshotID string) (EventIterator, error) {
	return s.Seek(ctx, 0)
}

// Subscribe implements EventStore using NATS JetStream push consumer.
func (s *NATSEventStore) Subscribe(ctx context.Context, filter EventFilter) (<-chan *Event, error) {
	subject := fmt.Sprintf("%s.%s", s.prefix, natsEventSubject)
	ch := make(chan *Event, 256)

	consumer, err := s.js.OrderedConsumer(ctx, s.stream, jetstream.OrderedConsumerConfig{
		FilterSubjects: []string{subject},
		DeliverPolicy:  jetstream.DeliverNewPolicy,
	})
	if err != nil {
		close(ch)
		return ch, fmt.Errorf("create consumer: %w", err)
	}

	// Start goroutine to forward messages.
	go func() {
		defer close(ch)
		for {
			msg, err := consumer.Next()
			if err != nil {
				return
			}
			var ev Event
			if err := json.Unmarshal(msg.Data(), &ev); err != nil {
				continue
			}
			if filter.Matches(&ev) {
				select {
				case ch <- &ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

// GC implements EventStore (purge stream is handled by JetStream TTL).
func (s *NATSEventStore) GC(ctx context.Context, retention time.Duration) error {
	// JetStream handles retention via MaxAge in stream config.
	// For explicit GC, update the stream config.
	info, err := s.js.Stream(ctx, s.stream)
	if err != nil {
		return err
	}
	cfg := info.CachedInfo().Config
	cfg.MaxAge = retention
	_, err = s.js.UpdateStream(ctx, cfg)
	return err
}

// evictExpiredLocked removes cache entries whose TTL has expired.
// Must be called with s.mu held (Lock, not RLock).
func (s *NATSEventStore) evictExpiredLocked() {
	now := time.Now()
	for k, ce := range s.cache {
		if now.After(ce.expiresAt) {
			delete(s.cache, k)
		}
	}
}

// ---- natsEventIterator ----

type natsEventIterator struct {
	consumer jetstream.Consumer
	ctx      context.Context
	filter   EventFilter
	buffer   []*Event
	bufPos   int
}

func (it *natsEventIterator) Next(_ context.Context) (*Event, bool) {
	// Return from buffer first.
	if it.bufPos < len(it.buffer) {
		ev := it.buffer[it.bufPos]
		it.bufPos++
		return ev, true
	}
	it.buffer = it.buffer[:0]
	it.bufPos = 0

	// Fetch next batch.
	for i := 0; i < 100; i++ {
		msg, err := it.consumer.Next()
		if err != nil {
			return nil, false
		}
		var ev Event
		if err := json.Unmarshal(msg.Data(), &ev); err != nil {
			continue
		}
		if it.filter.Matches(&ev) {
			it.buffer = append(it.buffer, &ev)
		}
	}
	if len(it.buffer) == 0 {
		return nil, false
	}
	ev := it.buffer[0]
	it.bufPos = 1
	return ev, true
}

func (it *natsEventIterator) Close() error {
	return nil
}

// ---- natsErrorIterator ----

type natsErrorIterator struct {
	err     error
	emitted bool
}

func (it *natsErrorIterator) Next(_ context.Context) (*Event, bool) {
	return nil, false
}

func (it *natsErrorIterator) Close() error {
	return nil
}
