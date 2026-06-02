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

package models

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// withTestTimeouts temporarily shrinks the shared timeout knobs so deadline
// behaviour can be exercised with millisecond-scale delays instead of the
// production minutes/seconds, and restores them when the test ends.
func withTestTimeouts(t *testing.T, nonStream, stream time.Duration) {
	t.Helper()
	origNon, origStream := nonStreamCallTimeout, streamCallTimeout
	nonStreamCallTimeout = nonStream
	streamCallTimeout = stream
	t.Cleanup(func() {
		nonStreamCallTimeout = origNon
		streamCallTimeout = origStream
	})
}

func newTimeoutTestGroq(baseURL string) *GroqModel {
	return NewGroqModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

// TestStreamNotTruncatedByNonStreamTimeout is the regression guard for the
// original bug: a streaming chat response that keeps emitting data for longer
// than the short non-streaming deadline must not be severed mid-stream. The
// stream is bounded by the generous streamCallTimeout, so a server that
// dribbles SSE events over a span well past nonStreamCallTimeout still
// completes intact. If a provider regressed to wrapping its stream in
// nonStreamCallTimeout (the old 120s-wall footgun, just relocated), the
// context would cancel mid-stream and this test would fail.
func TestStreamNotTruncatedByNonStreamTimeout(t *testing.T) {
	// Stream emits for ~240ms, far past the 60ms non-stream deadline but well
	// inside the 10s stream deadline.
	withTestTimeouts(t, 60*time.Millisecond, 10*time.Second)

	chunks := []string{"Hello", " ", "streamed", " ", "world"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Errorf("ResponseWriter does not support flushing")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, c := range chunks {
			time.Sleep(40 * time.Millisecond)
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n", c)
			flusher.Flush()
		}
		time.Sleep(40 * time.Millisecond)
		_, _ = io.WriteString(w, "data: [DONE]\n")
		flusher.Flush()
	}))
	defer srv.Close()

	apiKey := "test-key"
	var got strings.Builder
	err := newTimeoutTestGroq(srv.URL).ChatStreamlyWithSender(
		"llama-3.3-70b-versatile",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		func(c *string, _ *string) error {
			if c != nil && *c != "[DONE]" {
				got.WriteString(*c)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("streaming returned error (stream was truncated): %v", err)
	}
	if want := "Hello streamed world"; got.String() != want {
		t.Fatalf("streamed content=%q, want %q", got.String(), want)
	}
}

// TestNonStreamHonorsShortDeadline ensures dropping the blanket
// http.Client.Timeout did not leave non-streaming calls unbounded: a slow
// server must still trip the per-call nonStreamCallTimeout promptly.
func TestNonStreamHonorsShortDeadline(t *testing.T) {
	withTestTimeouts(t, 100*time.Millisecond, 10*time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(800 * time.Millisecond) // far beyond the 100ms non-stream deadline
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"late"}}]}`)
	}))
	defer srv.Close()

	apiKey := "test-key"
	start := time.Now()
	_, err := newTimeoutTestGroq(srv.URL).ChatWithMessages(
		"llama-3.3-70b-versatile",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
	)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected a deadline error from the slow server, got nil")
	}
	if elapsed > 400*time.Millisecond {
		t.Fatalf("call took %v; expected it to abort near the 100ms deadline", elapsed)
	}
}
