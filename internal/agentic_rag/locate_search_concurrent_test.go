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
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/agent/runtime"
)

// entryBarrier parks every search until all expected searches have entered,
// so an overlap assertion measures the code under test rather than the
// scheduler. A bounded timeout with a one-shot release keeps a serial
// regression from hanging the test.
type entryBarrier struct {
	expected int
	arrived  chan struct{}
	release  chan struct{}
	once     sync.Once
}

const entryBarrierTimeout = 3 * time.Second

func newEntryBarrier(n int) *entryBarrier {
	return &entryBarrier{
		expected: n,
		arrived:  make(chan struct{}, n),
		release:  make(chan struct{}),
	}
}

func (b *entryBarrier) wait(ctx context.Context) error {
	b.arrived <- struct{}{}
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		b.once.Do(func() { close(b.release) })
		return ctx.Err()
	case <-time.After(entryBarrierTimeout):
		b.once.Do(func() { close(b.release) })
		return errors.New("entry barrier timed out")
	}
}

func (b *entryBarrier) waitForAll() error {
	timer := time.NewTimer(entryBarrierTimeout)
	defer timer.Stop()
	for i := 0; i < b.expected; i++ {
		select {
		case <-b.arrived:
		case <-timer.C:
			b.once.Do(func() { close(b.release) })
			return fmt.Errorf("entry barrier: only %d of %d calls arrived", i, b.expected)
		}
	}
	b.once.Do(func() { close(b.release) })
	return nil
}

// A barrier must keep a call parked until the gate is opened, otherwise an
// overlap assertion measured through it is still timing-dependent.
func TestEntryBarrierBlocksUntilEveryCallArrives(t *testing.T) {
	barrier := newEntryBarrier(1)
	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(entered)
		done <- barrier.wait(context.Background())
	}()
	<-entered
	select {
	case err := <-done:
		t.Fatalf("wait returned before the gate opened: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	if err := barrier.waitForAll(); err != nil {
		t.Fatalf("waitForAll: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("wait after release: %v", err)
	}
}

// concurrentRetrievalService records peak in-flight searches so the test can
// tell a real concurrent fan-out from a serial loop that merely calls Search
// the same number of times.
type concurrentRetrievalService struct {
	mu       sync.Mutex
	inFlight int
	peak     int
	calls    []runtime.RetrievalRequest
	delay    time.Duration
	barrier  *entryBarrier
	respond  func(string) ([]runtime.RetrievalChunk, error)
}

func (s *concurrentRetrievalService) Search(ctx context.Context, _ *gorm.DB, req runtime.RetrievalRequest) ([]runtime.RetrievalChunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.calls = append(s.calls, req)
	s.inFlight++
	if s.inFlight > s.peak {
		s.peak = s.inFlight
	}
	s.mu.Unlock()

	if s.barrier != nil {
		if err := s.barrier.wait(ctx); err != nil {
			s.mu.Lock()
			s.inFlight--
			s.mu.Unlock()
			return nil, err
		}
	} else {
		time.Sleep(s.delay)
	}

	s.mu.Lock()
	s.inFlight--
	s.mu.Unlock()
	return s.respond(req.Query)
}

func installConcurrentRetrieval(t *testing.T, fake *concurrentRetrievalService) {
	t.Helper()
	runtime.SetRetrievalService(fake)
	t.Cleanup(func() { runtime.SetRetrievalService(nil) })
}

func locateSpec(queries []string) locateSearchSpec {
	return locateSearchSpec{
		tool:            "search_chunks",
		queries:         queries,
		boundDatasetIDs: []string{"kb1"},
		tenantID:        "tenant-1",
		topN:            2,
		weight:          searchChunksDefaultKeywordsSimilarityWeight,
	}
}

func locateTestChunk(id, content string) runtime.RetrievalChunk {
	return runtime.RetrievalChunk{
		ID: id, Content: content, DocumentID: "doc-1", DocumentName: "doc.md",
		DatasetID: "kb1", Score: 0.5,
	}
}

func runLocateForTest(t *testing.T, ctx context.Context, spec locateSearchSpec) string {
	t.Helper()
	out, err := runLocateSearch(ctx, spec)
	if err != nil {
		t.Fatalf("runLocateSearch: %v", err)
	}
	return out
}

var chunkIDPattern = regexp.MustCompile(`chunk_id="([^"]*)"`)

func chunkIDsInOrder(t *testing.T, out string) []string {
	t.Helper()
	matches := chunkIDPattern.FindAllStringSubmatch(out, -1)
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		ids = append(ids, m[1])
	}
	return ids
}

// The three searches must overlap: a serial loop gives the same call count
// but a different (and three times longer) latency profile.
func TestRunLocateSearchIssuesQueriesConcurrently(t *testing.T) {
	barrier := newEntryBarrier(3)
	fake := &concurrentRetrievalService{
		barrier: barrier,
		respond: func(q string) ([]runtime.RetrievalChunk, error) {
			return []runtime.RetrievalChunk{locateTestChunk("c-"+q, q+" alpha beta")}, nil
		},
	}
	installConcurrentRetrieval(t, fake)

	gateErr := make(chan error, 1)
	go func() { gateErr <- barrier.waitForAll() }()
	out := runLocateForTest(t, context.Background(), locateSpec([]string{"q1", "q2", "q3"}))
	if err := <-gateErr; err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 3 {
		t.Fatalf("Search calls = %d, want 3", len(fake.calls))
	}
	if fake.peak != 3 {
		t.Errorf("peak concurrent searches = %d, want 3; queries are still serialized", fake.peak)
	}
	if got := chunkIDsInOrder(t, out); !slices.Equal(got, []string{"c-q1", "c-q2", "c-q3"}) {
		t.Errorf("chunk order = %v, want [c-q1 c-q2 c-q3]", got)
	}
}

// Concurrent completion must merge in query order, not completion order: the
// dedup set is order-sensitive and the model reads the XML in that order.
func TestRunLocateSearchPreservesQueryOrderAndDedup(t *testing.T) {
	byQuery := map[string][]runtime.RetrievalChunk{
		"q1": {locateTestChunk("c1", "q1 alpha"), locateTestChunk("c2", "q1 beta")},
		"q2": {locateTestChunk("c2", "q2 beta"), locateTestChunk("c3", "q2 gamma")},
		"q3": {locateTestChunk("c1", "q3 alpha"), locateTestChunk("c4", "q3 delta")},
	}
	fake := &concurrentRetrievalService{
		delay:   5 * time.Millisecond,
		respond: func(q string) ([]runtime.RetrievalChunk, error) { return byQuery[q], nil },
	}
	installConcurrentRetrieval(t, fake)

	out := runLocateForTest(t, context.Background(), locateSpec([]string{"q1", "q2", "q3"}))
	if got := chunkIDsInOrder(t, out); !slices.Equal(got, []string{"c1", "c2", "c3", "c4"}) {
		t.Errorf("merged chunk order = %v, want [c1 c2 c3 c4]", got)
	}
	if !strings.Contains(out, `<search_results count="4"`) {
		t.Errorf("want four deduplicated hits, got: %s", out)
	}
}

var errTestQueryFailed = errors.New("embedding backend failed")

// A single failed query must not cancel its siblings: the fan-out joins with
// a WaitGroup, and no derived context is handed to Search that a failure
// could cancel.
func TestRunLocateSearchPartialFailureKeepsSiblingResults(t *testing.T) {
	fake := &concurrentRetrievalService{
		delay: 5 * time.Millisecond,
		respond: func(q string) ([]runtime.RetrievalChunk, error) {
			if q == "q2" {
				return nil, errTestQueryFailed
			}
			return []runtime.RetrievalChunk{locateTestChunk("c-"+q, q+" alpha")}, nil
		},
	}
	installConcurrentRetrieval(t, fake)

	out := runLocateForTest(t, context.Background(), locateSpec([]string{"q1", "q2", "q3"}))
	if got := chunkIDsInOrder(t, out); !slices.Equal(got, []string{"c-q1", "c-q3"}) {
		t.Errorf("surviving chunk order = %v, want [c-q1 c-q3]", got)
	}
	if !strings.Contains(out, `<search_results count="2"`) {
		t.Errorf("partial failure must keep the two live queries' hits, got: %s", out)
	}
	if !strings.Contains(out, `severity="warn"`) || !strings.Contains(out, `failed_queries="1"`) {
		t.Errorf("partial failure notice missing or wrong scope: %s", out)
	}
}

// The engine context, not the run context, must reach every concurrent
// Search: eino may hand the tool an already-canceled context.
func TestRunLocateSearchConcurrentCallsUseEngineContext(t *testing.T) {
	barrier := newEntryBarrier(3)
	fake := &concurrentRetrievalService{
		barrier: barrier,
		respond: func(q string) ([]runtime.RetrievalChunk, error) {
			return []runtime.RetrievalChunk{locateTestChunk("c-"+q, q+" alpha")}, nil
		},
	}
	installConcurrentRetrieval(t, fake)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	gateErr := make(chan error, 1)
	go func() { gateErr <- barrier.waitForAll() }()
	out := runLocateForTest(t, ctx, locateSpec([]string{"q1", "q2", "q3"}))
	if err := <-gateErr; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `<search_results count="3"`) {
		t.Errorf("a canceled parent must not kill the engine searches, got: %s", out)
	}
	if fake.peak != 3 {
		t.Errorf("peak concurrent searches = %d, want 3", fake.peak)
	}
}

// A panic inside a retrieval goroutine must become that query's failure, not
// take down the process: without a recover in the spawned goroutine the whole
// test binary dies here, which is exactly the regression being guarded.
func TestRunLocateSearchPanicBecomesQueryFailure(t *testing.T) {
	barrier := newEntryBarrier(3)
	fake := &concurrentRetrievalService{
		barrier: barrier,
		respond: func(q string) ([]runtime.RetrievalChunk, error) {
			if q == "q2" {
				panic("simulated retrieval engine panic")
			}
			return []runtime.RetrievalChunk{locateTestChunk("c-"+q, q+" alpha")}, nil
		},
	}
	installConcurrentRetrieval(t, fake)

	gateErr := make(chan error, 1)
	go func() { gateErr <- barrier.waitForAll() }()
	out := runLocateForTest(t, context.Background(), locateSpec([]string{"q1", "q2", "q3"}))
	if err := <-gateErr; err != nil {
		t.Fatal(err)
	}
	if got := chunkIDsInOrder(t, out); !slices.Equal(got, []string{"c-q1", "c-q3"}) {
		t.Errorf("surviving chunk order = %v, want [c-q1 c-q3]", got)
	}
	if !strings.Contains(out, `<search_results count="2"`) {
		t.Errorf("a panicking query must not drop the two live queries' hits, got: %s", out)
	}
	if !strings.Contains(out, `severity="warn"`) || !strings.Contains(out, `failed_queries="1"`) ||
		!strings.Contains(out, "retrieval panicked") {
		t.Errorf("panic must surface as that query's partial failure, got: %s", out)
	}
}

// The all-panic case must still reach the outage notice — neither a crash nor
// a misleading empty-corpus result.
func TestRunLocateSearchAllQueriesPanicIsOutageNotice(t *testing.T) {
	barrier := newEntryBarrier(3)
	fake := &concurrentRetrievalService{
		barrier: barrier,
		respond: func(string) ([]runtime.RetrievalChunk, error) {
			panic("engine down")
		},
	}
	installConcurrentRetrieval(t, fake)

	gateErr := make(chan error, 1)
	go func() { gateErr <- barrier.waitForAll() }()
	out := runLocateForTest(t, context.Background(), locateSpec([]string{"q1", "q2", "q3"}))
	if err := <-gateErr; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `<tool_error tool="search_chunks" severity="error" failed_queries="3">`) {
		t.Errorf("all-panic case must be an error tool_result, got: %s", out)
	}
	if !strings.Contains(out, "SEMANTIC RETRIEVAL UNAVAILABLE") || !strings.Contains(out, "retrieval panicked") {
		t.Errorf("all-panic case must carry the outage reason, got: %s", out)
	}
}

func BenchmarkRunLocateSearchConcurrent(b *testing.B) {
	fake := &concurrentRetrievalService{
		respond: func(q string) ([]runtime.RetrievalChunk, error) {
			return []runtime.RetrievalChunk{locateTestChunk("c-"+q, q+" alpha")}, nil
		},
	}
	runtime.SetRetrievalService(fake)
	defer runtime.SetRetrievalService(nil)
	spec := locateSpec([]string{"q1", "q2", "q3"})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		fake.calls = nil
		if _, err := runLocateSearch(context.Background(), spec); err != nil {
			b.Fatal(err)
		}
	}
}
