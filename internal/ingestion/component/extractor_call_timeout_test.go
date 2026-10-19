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

package component

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/schema"
)

// routeStubChatInvoker answers immediately unless a message contains
// "slow-chunk", in which case it hangs until ctx cancellation. It lets tests
// mix healthy and hung chunks deterministically despite pool concurrency.
type routeStubChatInvoker struct {
	calls    atomic.Int32
	mu       sync.Mutex
	requests []extractorChatRequest
}

func (s *routeStubChatInvoker) Chat(ctx context.Context, req extractorChatRequest) (*extractorChatResponse, error) {
	s.calls.Add(1)
	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.mu.Unlock()
	for _, m := range req.Messages {
		if strings.Contains(m.Content, "slow-chunk") {
			<-ctx.Done()
			return nil, ctx.Err()
		}
	}
	return &extractorChatResponse{Content: "kw1, kw2"}, nil
}

func withRouteStubChatInvoker(t *testing.T) *routeStubChatInvoker {
	t.Helper()
	prev := defaultExtractorChatInvoker
	stub := &routeStubChatInvoker{}
	SetExtractorChatInvoker(stub)
	t.Cleanup(func() {
		SetExtractorChatInvoker(prev)
	})
	return stub
}

func keywordsOnlyExtractor() *ExtractorComponent {
	return &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID:    "test-model",
		Keywords: schema.KeywordExtractConfig{Enabled: true, TopN: 3},
	}}
}

// TestExtractorChatTimeoutSkipsSlowChunk: one hung chunk must not fail the
// document. The healthy chunk keeps its extraction, the hung one is skipped.
func TestExtractorChatTimeoutSkipsSlowChunk(t *testing.T) {
	t.Setenv(common.EnvExtractorChatTimeout, "1")
	stub := withRouteStubChatInvoker(t)

	c := keywordsOnlyExtractor()
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{
			{"text": "fast chunk one"},
			{"text": "slow-chunk two hangs"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %d, want 2", len(chunks))
	}
	if _, ok := chunks[0]["important_kwd"]; !ok {
		t.Error("fast chunk lost its keywords")
	}
	if _, ok := chunks[1]["important_kwd"]; ok {
		t.Error("slow chunk should have been skipped without keywords")
	}
	if got := stub.calls.Load(); got != 2 {
		t.Errorf("LLM calls = %d, want 2 (hung call burns one budget, no retry)", got)
	}
}

// TestExtractorChatTimeoutAllHangFails: every chunk hung means the model is
// down — the run must fail loudly, not silently produce empty extractions.
func TestExtractorChatTimeoutAllHangFails(t *testing.T) {
	t.Setenv(common.EnvExtractorChatTimeout, "1")
	withRouteStubChatInvoker(t)

	c := keywordsOnlyExtractor()
	_, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{
			{"text": "slow-chunk one hangs"},
			{"text": "slow-chunk two hangs"},
		},
	})
	if err == nil {
		t.Fatal("Invoke succeeded, want all-chunks-timed-out error")
	}
	if !strings.Contains(err.Error(), "all 2 chunks") {
		t.Errorf("error = %q, want all-chunks-timed-out", err.Error())
	}
}

// TestExtractorParentCancelPropagates: task stop must still fail fast, never
// degrade into per-chunk skips.
func TestExtractorParentCancelPropagates(t *testing.T) {
	t.Setenv(common.EnvExtractorChatTimeout, "1")
	withRouteStubChatInvoker(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	c := keywordsOnlyExtractor()
	_, err := c.Invoke(ctx, nil, map[string]any{
		"chunks": []map[string]any{
			{"text": "fast chunk one"},
			{"text": "fast chunk two"},
		},
	})
	if err == nil {
		t.Fatal("Invoke succeeded on canceled ctx, want error")
	}
}

// TestExtractorChatTimeoutEnv: budget resolution and fallback.
func TestExtractorChatTimeoutEnv(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value string
		want  time.Duration
	}{
		{name: "unset falls back to default", value: "", want: defaultExtractorChatTimeout},
		{name: "override wins", value: "1", want: time.Second},
		{name: "whitespace falls back", value: " \t", want: defaultExtractorChatTimeout},
		{name: "trimmed override wins", value: " 240 ", want: 240 * time.Second},
		{name: "duration suffix falls back", value: "180s", want: defaultExtractorChatTimeout},
		{name: "fractional seconds fall back", value: "1.5", want: defaultExtractorChatTimeout},
		{name: "maximum duration seconds", value: "9223372036", want: 9223372036 * time.Second},
		{name: "duration overflow falls back", value: "9223372037", want: defaultExtractorChatTimeout},
		{name: "integer overflow falls back", value: "9223372036854775808", want: defaultExtractorChatTimeout},
		{name: "zero falls back", value: "0", want: defaultExtractorChatTimeout},
		{name: "negative falls back", value: "-5", want: defaultExtractorChatTimeout},
		{name: "non-numeric falls back", value: "abc", want: defaultExtractorChatTimeout},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(common.EnvExtractorChatTimeout, tt.value)
			if got := extractorChatTimeout(); got != tt.want {
				t.Errorf("extractorChatTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
	if defaultExtractorChatTimeout != 180*time.Second {
		t.Errorf("defaultExtractorChatTimeout = %v, want 180s", defaultExtractorChatTimeout)
	}
}

// summaryHangStub answers every call except summary ones, which hang until
// ctx cancellation. It models a model that serves some extraction types
// while another hangs.
type summaryHangStub struct {
	calls atomic.Int32
}

func (s *summaryHangStub) Chat(ctx context.Context, req extractorChatRequest) (*extractorChatResponse, error) {
	s.calls.Add(1)
	for _, m := range req.Messages {
		if strings.Contains(m.Content, "faithful text summarizer") {
			<-ctx.Done()
			return nil, ctx.Err()
		}
	}
	return &extractorChatResponse{Content: "q1?\nq2?"}, nil
}

func withSummaryHangStub(t *testing.T) {
	t.Helper()
	prev := defaultExtractorChatInvoker
	SetExtractorChatInvoker(&summaryHangStub{})
	t.Cleanup(func() {
		SetExtractorChatInvoker(prev)
	})
}

// TestExtractorPartialTimeoutKeepsCompleted: questions succeed while
// summaries hang for every chunk. The phase must succeed and keep the
// questions instead of failing as all-timed-out.
func TestExtractorPartialTimeoutKeepsCompleted(t *testing.T) {
	t.Setenv(common.EnvExtractorChatTimeout, "1")
	withSummaryHangStub(t)

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID:     "test-model",
		Questions: schema.QuestionExtractConfig{Enabled: true, TopN: 2},
		Summary:   schema.SummaryExtractConfig{Enabled: true},
	}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{
			{"text": "first piece"},
			{"text": "second piece"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %d, want 2", len(chunks))
	}
	for i, ck := range chunks {
		if _, ok := ck["question_kwd"]; !ok {
			t.Errorf("chunk %d lost its completed questions", i)
		}
		if _, ok := ck["summary"]; ok {
			t.Errorf("chunk %d should have no summary (hung)", i)
		}
	}
}

// TestExtractorTagsOnlyCancelFails: the tag phase swallows per-chunk
// failures, so a stop during a tags-only run must still surface as an
// error rather than a false success.
func TestExtractorTagsOnlyCancelFails(t *testing.T) {
	withRouteStubChatInvoker(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	c := &ExtractorComponent{Param: schema.ExtractorParam{
		LLMID: "test-model",
		Tags:  schema.TagExtractConfig{Enabled: true, TopN: 3},
	}}
	_, err := c.Invoke(ctx, nil, map[string]any{
		"chunks": []map[string]any{
			{"text": "some chunk"},
		},
	})
	if err == nil {
		t.Fatal("Invoke succeeded on canceled ctx, want error")
	}
}
