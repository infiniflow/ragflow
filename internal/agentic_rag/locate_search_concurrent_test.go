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
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/agent/runtime"
)

// concurrentRetrievalService records peak in-flight searches so the test can
// tell a real concurrent fan-out from a serial loop that merely calls Search
// the same number of times.
type concurrentRetrievalService struct {
	mu       sync.Mutex
	inFlight int
	peak     int
	calls    []runtime.RetrievalRequest
	delay    time.Duration
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

	time.Sleep(s.delay)

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
	fake := &concurrentRetrievalService{
		delay: 40 * time.Millisecond,
		respond: func(q string) ([]runtime.RetrievalChunk, error) {
			return []runtime.RetrievalChunk{locateTestChunk("c-"+q, q+" alpha beta")}, nil
		},
	}
	installConcurrentRetrieval(t, fake)

	out := runLocateForTest(t, context.Background(), locateSpec([]string{"q1", "q2", "q3"}))
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

// A single failed query must not cancel its siblings: the group is a join,
// not an errgroup.WithContext whose context is handed to Search.
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
	fake := &concurrentRetrievalService{
		delay: 5 * time.Millisecond,
		respond: func(q string) ([]runtime.RetrievalChunk, error) {
			return []runtime.RetrievalChunk{locateTestChunk("c-"+q, q+" alpha")}, nil
		},
	}
	installConcurrentRetrieval(t, fake)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := runLocateForTest(t, ctx, locateSpec([]string{"q1", "q2", "q3"}))
	if !strings.Contains(out, `<search_results count="3"`) {
		t.Errorf("a canceled parent must not kill the engine searches, got: %s", out)
	}
	if fake.peak != 3 {
		t.Errorf("peak concurrent searches = %d, want 3", fake.peak)
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
