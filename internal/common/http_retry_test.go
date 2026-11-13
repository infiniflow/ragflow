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

package common

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestRetryClient(retries int) *http.Client {
	return &http.Client{Transport: &retryTransport{
		base:       http.DefaultTransport.(*http.Transport).Clone(),
		maxRetries: func() int { return retries },
		baseDelay:  time.Millisecond,
		maxDelay:   5 * time.Millisecond,
	}}
}

func post(t *testing.T, client *http.Client, ctx context.Context, url, body string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return client.Do(req)
}

func TestRetryTransportStatusRetries(t *testing.T) {
	tests := []struct {
		name         string
		failStatus   int
		failures     int
		retries      int
		wantStatus   int
		wantRequests int32
	}{
		{"success after one 503", 503, 1, 3, 200, 2},
		{"success after N failures", 502, 3, 3, 200, 4},
		{"retries 429", 429, 2, 3, 200, 3},
		{"retries 408", 408, 1, 3, 200, 2},
		{"retries 500", 500, 1, 3, 200, 2},
		{"exhausted returns last response", 503, 10, 2, 503, 3},
		{"retries 0 disables", 503, 1, 0, 503, 1},
		{"401 not retried", 401, 1, 3, 401, 1},
		{"400 not retried", 400, 1, 3, 400, 1},
		{"404 not retried", 404, 1, 3, 404, 1},
		{"501 not retried", 501, 1, 3, 501, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if string(body) != `{"q":1}` {
					t.Errorf("request %d body = %q, want it replayed intact", requests.Load()+1, body)
				}
				if int(requests.Add(1)) <= tt.failures {
					w.WriteHeader(tt.failStatus)
					_, _ = w.Write([]byte("upstream failure"))
					return
				}
				_, _ = w.Write([]byte("ok"))
			}))
			defer srv.Close()

			resp, err := post(t, newTestRetryClient(tt.retries), context.Background(), srv.URL, `{"q":1}`)
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if got := requests.Load(); got != tt.wantRequests {
				t.Errorf("requests = %d, want %d", got, tt.wantRequests)
			}
			if tt.wantStatus != 200 {
				if body, _ := io.ReadAll(resp.Body); string(body) != "upstream failure" {
					t.Errorf("last response body = %q, want the provider's error body", body)
				}
			}
		})
	}
}

func TestRetryTransportNetworkErrors(t *testing.T) {
	newServer := func(failures int32, requests *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if requests.Add(1) <= failures {
				conn, _, _ := w.(http.Hijacker).Hijack()
				_ = conn.Close()
				return
			}
			_, _ = w.Write([]byte("ok"))
		}))
	}

	t.Run("success after dropped connections", func(t *testing.T) {
		var requests atomic.Int32
		srv := newServer(2, &requests)
		defer srv.Close()
		resp, err := post(t, newTestRetryClient(3), context.Background(), srv.URL, "x")
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		resp.Body.Close()
		if requests.Load() != 3 {
			t.Errorf("requests = %d, want 3", requests.Load())
		}
	})

	t.Run("exhausted returns last error", func(t *testing.T) {
		var requests atomic.Int32
		srv := newServer(100, &requests)
		defer srv.Close()
		_, err := post(t, newTestRetryClient(2), context.Background(), srv.URL, "x")
		if err == nil {
			t.Fatal("expected an error after retries are exhausted")
		}
		if requests.Load() != 3 {
			t.Errorf("requests = %d, want 3", requests.Load())
		}
	})

	t.Run("retries 0 disables", func(t *testing.T) {
		var requests atomic.Int32
		srv := newServer(1, &requests)
		defer srv.Close()
		if _, err := post(t, newTestRetryClient(0), context.Background(), srv.URL, "x"); err == nil {
			t.Fatal("expected an error with retries disabled")
		}
		if requests.Load() != 1 {
			t.Errorf("requests = %d, want 1", requests.Load())
		}
	})

	t.Run("connection refused is retried", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := ln.Addr().String()
		ln.Close()
		var attempts atomic.Int32
		client := &http.Client{Transport: &retryTransport{
			base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				attempts.Add(1)
				return http.DefaultTransport.RoundTrip(req)
			}),
			maxRetries: func() int { return 2 },
			baseDelay:  time.Millisecond,
			maxDelay:   time.Millisecond,
		}}
		if _, err := post(t, client, context.Background(), "http://"+addr, "x"); err == nil {
			t.Fatal("expected a connection error")
		}
		if attempts.Load() != 3 {
			t.Errorf("attempts = %d, want 3", attempts.Load())
		}
	})
}

func TestRetryTransportDoesNotRetryRejectedRequests(t *testing.T) {
	var attempts atomic.Int32
	rejected := errors.New("destination is not allowed")
	client := &http.Client{Transport: &retryTransport{
		base: roundTripFunc(func(*http.Request) (*http.Response, error) {
			attempts.Add(1)
			return nil, rejected
		}),
		maxRetries: func() int { return 3 },
		baseDelay:  time.Millisecond,
		maxDelay:   time.Millisecond,
	}}
	if _, err := post(t, client, context.Background(), "http://example.invalid", "x"); !errors.Is(err, rejected) {
		t.Fatalf("err = %v, want the rejection", err)
	}
	if attempts.Load() != 1 {
		t.Errorf("attempts = %d, want 1", attempts.Load())
	}
}

func TestRetryTransportContextCancel(t *testing.T) {
	t.Run("cancelled while backing off", func(t *testing.T) {
		var requests atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			w.WriteHeader(503)
		}))
		defer srv.Close()

		ctx, cancel := context.WithCancel(context.Background())
		client := &http.Client{Transport: &retryTransport{
			base:       http.DefaultTransport.(*http.Transport).Clone(),
			maxRetries: func() int { return 5 },
			baseDelay:  time.Hour,
			maxDelay:   time.Hour,
		}}
		go func() {
			for requests.Load() == 0 {
				time.Sleep(time.Millisecond)
			}
			cancel()
		}()
		start := time.Now()
		_, err := post(t, client, ctx, srv.URL, "x")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		if time.Since(start) > 10*time.Second {
			t.Error("cancel did not interrupt the backoff")
		}
		if requests.Load() != 1 {
			t.Errorf("requests = %d, want 1", requests.Load())
		}
	})

	t.Run("already cancelled sends nothing", func(t *testing.T) {
		var requests atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
		}))
		defer srv.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := post(t, newTestRetryClient(3), ctx, srv.URL, "x"); !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		if requests.Load() != 0 {
			t.Errorf("requests = %d, want 0", requests.Load())
		}
	})

	t.Run("deadline during attempt is not retried", func(t *testing.T) {
		var requests atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			_, _ = io.Copy(io.Discard, r.Body) // lets the server notice the client hanging up
			<-r.Context().Done()
		}))
		defer srv.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if _, err := post(t, newTestRetryClient(3), ctx, srv.URL, "x"); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want context.DeadlineExceeded", err)
		}
		if requests.Load() != 1 {
			t.Errorf("requests = %d, want 1", requests.Load())
		}
	})
}

func TestRetryTransportResponseHeaderTimeoutIsRetried(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			_, _ = io.Copy(io.Discard, r.Body) // lets the server notice the client hanging up
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	base := http.DefaultTransport.(*http.Transport).Clone()
	base.ResponseHeaderTimeout = 50 * time.Millisecond
	client := &http.Client{Transport: &retryTransport{
		base:       base,
		maxRetries: func() int { return 2 },
		baseDelay:  time.Millisecond,
		maxDelay:   time.Millisecond,
	}}
	resp, err := post(t, client, context.Background(), srv.URL, "x")
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
	if requests.Load() != 2 {
		t.Errorf("requests = %d, want 2", requests.Load())
	}
}

func TestRetryTransportStreaming(t *testing.T) {
	t.Run("retried before the first chunk", func(t *testing.T) {
		var requests atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if requests.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: hello\n\ndata: [DONE]\n\n")
		}))
		defer srv.Close()

		resp, err := post(t, newTestRetryClient(3), context.Background(), srv.URL, "x")
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil || !strings.Contains(string(body), "hello") {
			t.Fatalf("body = %q, err = %v", body, err)
		}
		if requests.Load() != 2 {
			t.Errorf("requests = %d, want 2", requests.Load())
		}
	})

	t.Run("not retried after the first chunk", func(t *testing.T) {
		var requests atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			conn, rw, _ := w.(http.Hijacker).Hijack()
			defer conn.Close()
			writeTruncatedStream(rw)
		}))
		defer srv.Close()

		resp, err := post(t, newTestRetryClient(3), context.Background(), srv.URL, "x")
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err == nil {
			t.Fatalf("expected a mid-stream error, got body %q", body)
		}
		if !strings.Contains(string(body), "hello") {
			t.Errorf("body = %q, want the chunk delivered before the failure", body)
		}
		if requests.Load() != 1 {
			t.Errorf("requests = %d, want 1: a stream that has produced output must not be replayed", requests.Load())
		}
	})
}

// writeTruncatedStream sends headers and one chunk of an unfinished chunked
// response, then returns so the caller drops the connection mid-stream.
func writeTruncatedStream(rw *bufio.ReadWriter) {
	_, _ = rw.WriteString("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n")
	chunk := "data: hello\n\n"
	_, _ = rw.WriteString("d\r\n" + chunk + "\r\n")
	_ = rw.Flush()
}

func TestRetryTransportRequestWithoutReplayableBodyIsSentOnce(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(503)
	}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL, io.NopCloser(strings.NewReader("x")))
	resp, err := newTestRetryClient(3).Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
	if requests.Load() != 1 {
		t.Errorf("requests = %d, want 1", requests.Load())
	}
}

func TestRetryTransportRetriesBodylessRequests(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	resp, err := newTestRetryClient(2).Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || requests.Load() != 2 {
		t.Errorf("status = %d, requests = %d, want 200 and 2", resp.StatusCode, requests.Load())
	}
}

func TestRetryAfter(t *testing.T) {
	tests := []struct {
		name   string
		header string
		check  func(time.Duration) bool
	}{
		{"absent", "", func(d time.Duration) bool { return d == 0 }},
		{"seconds", "7", func(d time.Duration) bool { return d == 7*time.Second }},
		{"negative", "-3", func(d time.Duration) bool { return d == 0 }},
		{"garbage", "soon", func(d time.Duration) bool { return d == 0 }},
		{"http date in the future", time.Now().Add(time.Minute).UTC().Format(http.TimeFormat),
			func(d time.Duration) bool { return d > 30*time.Second && d <= time.Minute }},
		{"http date in the past", time.Now().Add(-time.Minute).UTC().Format(http.TimeFormat),
			func(d time.Duration) bool { return d == 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			if tt.header != "" {
				resp.Header.Set("Retry-After", tt.header)
			}
			if got := retryAfter(resp); !tt.check(got) {
				t.Errorf("retryAfter(%q) = %v", tt.header, got)
			}
		})
	}
}

func TestRetryTransportHonorsRetryAfter(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := &http.Client{Transport: &retryTransport{
		base:       http.DefaultTransport.(*http.Transport).Clone(),
		maxRetries: func() int { return 1 },
		baseDelay:  time.Millisecond,
		maxDelay:   2 * time.Second,
	}}
	start := time.Now()
	resp, err := post(t, client, context.Background(), srv.URL, "x")
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Errorf("retried after %v, want at least the 1s Retry-After", elapsed)
	}
}

func TestRetryTransportBackoff(t *testing.T) {
	rt := &retryTransport{baseDelay: 100 * time.Millisecond, maxDelay: time.Second}
	for retry, ceiling := range map[int]time.Duration{1: 100 * time.Millisecond, 2: 200 * time.Millisecond, 3: 400 * time.Millisecond, 20: time.Second} {
		for range 50 {
			if d := rt.backoff(retry); d < ceiling/2 || d > ceiling {
				t.Fatalf("backoff(%d) = %v, want within [%v, %v]", retry, d, ceiling/2, ceiling)
			}
		}
	}
}

func TestLLMMaxRetries(t *testing.T) {
	t.Cleanup(func() { _ = SetLLMMaxRetries(DefaultLLMMaxRetries) })
	if got := LLMMaxRetries(); got != DefaultLLMMaxRetries {
		t.Fatalf("default = %d, want %d", got, DefaultLLMMaxRetries)
	}

	tests := []struct {
		n       int
		wantErr bool
	}{
		{0, false}, {1, false}, {MaxLLMMaxRetries, false},
		{-1, true}, {MaxLLMMaxRetries + 1, true},
	}
	for _, tt := range tests {
		before := LLMMaxRetries()
		err := SetLLMMaxRetries(tt.n)
		if (err != nil) != tt.wantErr {
			t.Errorf("SetLLMMaxRetries(%d) error = %v, wantErr %v", tt.n, err, tt.wantErr)
		}
		want := tt.n
		if tt.wantErr {
			want = before
		}
		if got := LLMMaxRetries(); got != want {
			t.Errorf("after SetLLMMaxRetries(%d): LLMMaxRetries() = %d, want %d", tt.n, got, want)
		}
	}
}
