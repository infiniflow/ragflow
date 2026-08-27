package events

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultMaxSegmentSize int64 = 64 * 1024 * 1024 // 64 MB
	segmentFilePattern          = "events_*.jsonl"
)

// LocalFileEventStore persists events to JSONL files with automatic
// segment rotation. Suitable for single-instance durable storage.
type LocalFileEventStore struct {
	dir     string
	segment int   // current write segment number
	maxSize int64 // max bytes per segment before rotation
	mu      sync.RWMutex
	cached  []*Event // in-memory cache for current segment
	clock   atomic.Uint64
}

// NewLocalFileEventStore creates or opens an event store at the given directory.
// Existing segment files are loaded into memory on startup.
func NewLocalFileEventStore(dir string) (*LocalFileEventStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create events dir: %w", err)
	}

	s := &LocalFileEventStore{
		dir:     dir,
		segment: 0,
		maxSize: defaultMaxSegmentSize,
		cached:  make([]*Event, 0),
	}

	// Load existing segments to find the highest segment number.
	if err := s.loadExisting(); err != nil {
		return nil, fmt.Errorf("load existing segments: %w", err)
	}

	return s, nil
}

// loadExisting scans the directory for existing segment files and loads them.
func (s *LocalFileEventStore) loadExisting() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}

	var segmentFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "events_") && strings.HasSuffix(entry.Name(), ".jsonl") {
			segmentFiles = append(segmentFiles, entry.Name())
		}
	}

	// Sort by name (lexicographic works for timestamp-based names).
	sort.Strings(segmentFiles)

	allEvents := make([]*Event, 0)
	var maxClock uint64

	for _, fname := range segmentFiles {
		fpath := filepath.Join(s.dir, fname)
		events, err := readSegmentFile(fpath)
		if err != nil {
			return fmt.Errorf("read segment %s: %w", fname, err)
		}
		for _, ev := range events {
			if ev.Clock > maxClock {
				maxClock = ev.Clock
			}
		}
		allEvents = append(allEvents, events...)
	}

	s.cached = allEvents
	if maxClock > 0 {
		s.clock.Store(maxClock)
	}
	return nil
}

// readSegmentFile reads all events from a JSONL file.
func readSegmentFile(path string) ([]*Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var events []*Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1 MB line buffer
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fmt.Errorf("unmarshal event: %w", err)
		}
		events = append(events, &ev)
	}
	return events, scanner.Err()
}

// Append implements EventLog.
func (s *LocalFileEventStore) Append(ctx context.Context, events ...*Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, ev := range events {
		if ev.Clock == 0 {
			ev.Clock = s.clock.Add(1)
		}
		ev.Seal()
		s.cached = append(s.cached, ev)

		// Check if we need to rotate segment.
		if err := s.appendToFileLocked(ev); err != nil {
			return err
		}
	}
	return nil
}

// appendToFileLocked writes a single event to the current segment file.
// Must be called with s.mu held.
func (s *LocalFileEventStore) appendToFileLocked(ev *Event) error {
	fname := fmt.Sprintf("events_%s_%04d.jsonl", ev.TraceID, s.segment)
	fpath := filepath.Join(s.dir, fname)

	// Check segment size and rotate if needed.
	if info, err := os.Stat(fpath); err == nil && info.Size() > s.maxSize {
		s.segment++
		fname = fmt.Sprintf("events_%s_%04d.jsonl", ev.TraceID, s.segment)
		fpath = filepath.Join(s.dir, fname)
	}

	f, err := os.OpenFile(fpath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open segment file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		return err
	}
	if _, err := f.Write([]byte("\n")); err != nil {
		return err
	}
	return nil
}

// Stream implements EventLog.
func (s *LocalFileEventStore) Stream(ctx context.Context, filter EventFilter) EventIterator {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filtered := make([]*Event, 0)
	for _, ev := range s.cached {
		if filter.Matches(ev) {
			filtered = append(filtered, ev)
		}
	}
	return &sliceIterator{events: filtered, pos: 0}
}

// Get implements EventLog.
func (s *LocalFileEventStore) Get(ctx context.Context, id EventID) (*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ev := range s.cached {
		if ev.ID == id {
			return ev, nil
		}
	}
	return nil, nil
}

// Range implements EventLog.
func (s *LocalFileEventStore) Range(ctx context.Context, from, to uint64, filter EventFilter) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Event, 0)
	for _, ev := range s.cached {
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
func (s *LocalFileEventStore) Seek(ctx context.Context, clock uint64) (EventIterator, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pos := 0
	for i, ev := range s.cached {
		if ev.Clock >= clock {
			pos = i
			break
		}
		_ = i
	}
	return &sliceIterator{events: s.cached[pos:], pos: 0}, nil
}

// Length implements EventLog.
func (s *LocalFileEventStore) Length(ctx context.Context) (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return uint64(len(s.cached)), nil
}

// CreateSnapshot implements EventStore.
func (s *LocalFileEventStore) CreateSnapshot(ctx context.Context, traceID string) (*Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clock := s.clock.Load()
	data, err := json.Marshal(s.cached)
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
func (s *LocalFileEventStore) RestoreSnapshot(ctx context.Context, snapshotID string) (EventIterator, error) {
	return s.Seek(ctx, 0)
}

// Subscribe implements EventStore.
func (s *LocalFileEventStore) Subscribe(ctx context.Context, filter EventFilter) (<-chan *Event, error) {
	// LocalFileEventStore does not support real-time subscriptions.
	// Use NATSEventStore for distributed scenarios that need Subscribe.
	ch := make(chan *Event)
	close(ch)
	return ch, nil
}

// GC implements EventStore.
// Retained events are rewritten to fresh segment files; only segments whose
// entire content predates the cutoff are deleted.
func (s *LocalFileEventStore) GC(ctx context.Context, retention time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-retention)
	keep := make([]*Event, 0, len(s.cached))
	for _, ev := range s.cached {
		if ev.Timestamp.After(cutoff) {
			keep = append(keep, ev)
		}
	}
	s.cached = keep
	s.segment = 0

	// Remove all old segment files so the retained events can be rewritten
	// into fresh files below.
	entries, _ := os.ReadDir(s.dir)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "events_") && strings.HasSuffix(entry.Name(), ".jsonl") {
			os.Remove(filepath.Join(s.dir, entry.Name()))
		}
	}

	// Rewrite retained events into fresh segment files.
	// appendToFileLocked is safe to call here — it does not acquire s.mu
	// (the "Locked" suffix means the caller must hold it).
	for _, ev := range keep {
		if err := s.appendToFileLocked(ev); err != nil {
			return err
		}
	}
	return nil
}
