//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package connector

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSharePointRetryDelay(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, header string
		want         time.Duration
	}{
		{"seconds", "120", 120 * time.Second},
		{"whitespace", " 60 ", time.Minute},
		{"zero", "0", 0},
		{"date", now.Add(time.Hour).Format(http.TimeFormat), time.Hour},
		{"past date", now.Add(-time.Hour).Format(http.TimeFormat), 0},
		{"missing", "", 4 * time.Second},
		{"invalid", "later", 4 * time.Second},
		{"negative", "-1", 4 * time.Second},
		{"duration overflow", "9223372036854775807", time.Duration(1<<63 - 1)},
		{"integer overflow", "999999999999999999999999", time.Duration(1<<63 - 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sharePointRetryDelay(tc.header, 4*time.Second, now); got != tc.want {
				t.Fatalf("delay = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestSharePointRetriesRespectDelay(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int64
			var first, elapsed atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					first.Store(time.Now().UnixNano())
					w.Header().Set("Retry-After", "1")
					w.WriteHeader(status)
					return
				}
				elapsed.Store(time.Now().UnixNano() - first.Load())
				w.Write([]byte(`{"ok":true}`))
			}))
			defer server.Close()
			c := mustSharePointConnector(t, server.URL, nil)
			c.accessToken, c.tokenExpiry = "token", time.Now().Add(time.Hour)
			if _, err := c.doGraphRequest(t.Context(), server.URL+"/v1.0/test", 1024); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 2 || time.Duration(elapsed.Load()) < time.Second {
				t.Fatalf("calls = %d, retry after %s", calls.Load(), time.Duration(elapsed.Load()))
			}
		})
	}
}

func TestSharePointRetryBackoff(t *testing.T) {
	requests := make(chan time.Time, 3)
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- time.Now()
		if calls.Add(1) <= 2 {
			w.Header().Set("Retry-After", "invalid")
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	defer server.Close()
	c := mustSharePointConnector(t, server.URL, nil)
	c.accessToken, c.tokenExpiry = "token", time.Now().Add(time.Hour)
	if _, err := c.doGraphRequest(t.Context(), server.URL+"/v1.0/test", 1024); err != nil {
		t.Fatal(err)
	}
	first, second, third := <-requests, <-requests, <-requests
	if second.Sub(first) < time.Second || third.Sub(second) < 2*time.Second {
		t.Fatalf("retry intervals = %s, %s", second.Sub(first), third.Sub(second))
	}
}

func TestSharePointRetryWaitCancellation(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	c := mustSharePointConnector(t, server.URL, nil)
	c.accessToken, c.tokenExpiry = "token", time.Now().Add(time.Hour)
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	_, err := c.doGraphRequest(ctx, server.URL+"/v1.0/test", 1024)
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 1 {
		t.Fatalf("error = %v, calls = %d", err, calls.Load())
	}
}

func TestSharePointRetryLimit(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	c := mustSharePointConnector(t, server.URL, nil)
	c.accessToken, c.tokenExpiry = "token", time.Now().Add(time.Hour)
	_, err := c.doGraphRequest(t.Context(), server.URL+"/v1.0/test", 1024)
	var apiErr *sharePointAPIError
	if !errors.As(err, &apiErr) || apiErr.status != http.StatusTooManyRequests || calls.Load() != sharepointRetryCount {
		t.Fatalf("error = %v, calls = %d", err, calls.Load())
	}
}
