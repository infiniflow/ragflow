package common

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
)

// failChatInvoker returns err on every Chat call and counts invocations.
type failChatInvoker struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *failChatInvoker) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return nil, f.err
}

// TestGenJSON_RetriesTransientError verifies the retry loop still retries on a
// transient error and that WithRetryDelay controls the (now ExponentialBackoff)
// base delay. With retryMax=3 it must issue 1 initial + 3 retries = 4 calls.
func TestGenJSON_RetriesTransientError(t *testing.T) {
	inv := &failChatInvoker{err: errors.New("connection timeout")}
	ctx := WithRetryDelay(t.Context(), time.Millisecond)
	if _, err := GenJSON(ctx, inv, ChatRequest{}, 3); err == nil {
		t.Fatal("expected error")
	}
	if inv.calls != 4 {
		t.Errorf("calls = %d, want 4 (1 initial + 3 retries)", inv.calls)
	}
}

// TestGenJSON_ReportsTheActualBackoffDelay pins the retry progress messages to
// the wait the loop is about to take. The reporter emits the first failure and
// then every third repeated one, so retryMax=4 produces exactly two messages:
// attempt 1 (base) and attempt 4 (base*2^3). A constant delay here means the
// message under-reports the wait the caller is actually about to take.
func TestGenJSON_ReportsTheActualBackoffDelay(t *testing.T) {
	var messages []string
	ctx := WithRetryDelay(t.Context(), time.Millisecond)
	ctx = runtime.WithProgressMessageCallback(ctx, func(_, message string) {
		messages = append(messages, message)
	})
	inv := &failChatInvoker{err: errors.New("connection timeout")}
	if _, err := GenJSON(ctx, inv, ChatRequest{}, 4); err == nil {
		t.Fatal("expected error")
	}
	var reported []string
	for _, message := range messages {
		if _, after, ok := strings.Cut(message, "retrying in "); ok {
			reported = append(reported, after)
		}
	}
	if len(reported) != 2 {
		t.Fatalf("retry delays = %v, want 2 (attempt 1 and attempt 4); messages = %v", reported, messages)
	}
	if reported[0] != "1ms" || reported[1] != "8ms" {
		t.Fatalf("reported delays = %v, want [1ms 8ms] (base, then base*2^3)", reported)
	}
}

// TestGenJSON_NoRetryOnPermanentError verifies a permanent (non-transient)
// error escapes the loop immediately with a single call.
func TestGenJSON_NoRetryOnPermanentError(t *testing.T) {
	inv := &failChatInvoker{err: errors.New("API request failed with status 400: bad request")}
	if _, err := GenJSON(t.Context(), inv, ChatRequest{}); err == nil {
		t.Fatal("expected error")
	}
	if inv.calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on permanent error)", inv.calls)
	}
}

// TestTryUnmarshalJSONErr_RejectsArray verifies that tryUnmarshalJSONErr keeps
// object-only semantics so callers expecting keyed objects (wiki, merge, chain)
// still trigger retries instead of silently receiving {"items": ...}.
func TestTryUnmarshalJSONErr_RejectsArray(t *testing.T) {
	s := `[{"from_entity": "WO-001", "to_entity": "Pump-001", "relation": "repairs"}]`
	if _, err := tryUnmarshalJSONErr(s); err == nil {
		t.Fatal("expected tryUnmarshalJSONErr to reject top-level array")
	}
}

// TestTryUnmarshalJSONOrArrayErr_Array verifies that top-level JSON arrays are
// safely unmarshaled and wrapped in {"items": [...]} when array wrapping is enabled.
func TestTryUnmarshalJSONOrArrayErr_Array(t *testing.T) {
	s := `[{"from_entity": "WO-001", "to_entity": "Pump-001", "relation": "repairs"}]`
	m, err := tryUnmarshalJSONOrArrayErr(s)
	if err != nil {
		t.Fatalf("tryUnmarshalJSONOrArrayErr array: %v", err)
	}
	items, ok := m["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected items array, got %#v", m)
	}
}

type scriptedChatInvoker struct {
	mu      sync.Mutex
	replies []string
	calls   int
}

func (s *scriptedChatInvoker) Chat(_ context.Context, _ ChatRequest) (*ChatResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.calls
	if idx >= len(s.replies) {
		idx = len(s.replies) - 1
	}
	s.calls++
	return &ChatResponse{Content: s.replies[idx]}, nil
}

// TestGenJSON_RetriesOnTopLevelArray verifies that GenJSON rejects a bare array
// and retries instead of returning an empty keyed-object map.
func TestGenJSON_RetriesOnTopLevelArray(t *testing.T) {
	ctx := WithRetryDelay(t.Context(), time.Millisecond)
	inv := &scriptedChatInvoker{replies: []string{`[{"a": 1}]`, `{"entities": [{"name": "Alpha"}]}`}}
	got, err := GenJSON(ctx, inv, ChatRequest{}, 1)
	if err != nil {
		t.Fatalf("GenJSON: %v", err)
	}
	if inv.calls != 2 {
		t.Fatalf("calls = %d, want 2", inv.calls)
	}
	if _, ok := got["entities"]; !ok {
		t.Fatalf("expected entities key, got %#v", got)
	}
}

// TestGenJSONOrArray_AcceptsTopLevelArray verifies that GenJSONOrArray accepts
// a bare array on the first attempt and wraps it into {"items": [...]}.
func TestGenJSONOrArray_AcceptsTopLevelArray(t *testing.T) {
	inv := &scriptedChatInvoker{replies: []string{`[{"name": "Alpha"}]`}}
	got, err := GenJSONOrArray(t.Context(), inv, ChatRequest{}, 0)
	if err != nil {
		t.Fatalf("GenJSONOrArray: %v", err)
	}
	if inv.calls != 1 {
		t.Fatalf("calls = %d, want 1", inv.calls)
	}
	items, ok := got["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected items array, got %#v", got)
	}
}
