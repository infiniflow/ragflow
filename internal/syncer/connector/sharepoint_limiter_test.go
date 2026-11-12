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
	"sort"
	"sync"
	"testing"
	"time"
)

func TestSharePointLimiterPacesConcurrentRequests(t *testing.T) {
	l := newSharePointLimiter(30 * time.Millisecond)
	starts := make(chan time.Time, 4)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if err := l.acquire(t.Context()); err != nil {
				t.Error(err)
				return
			}
			starts <- time.Now()
			l.release()
		})
	}
	wg.Wait()
	close(starts)
	var times []time.Time
	for start := range starts {
		times = append(times, start)
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	if len(times) != 4 || times[3].Sub(times[0]) < 90*time.Millisecond {
		t.Fatalf("requests did not obey shared pacing: %v", times)
	}
}

func TestSharePointLimiterCancellationReleasesSlot(t *testing.T) {
	l := newSharePointLimiter(0)
	for range 2 {
		if err := l.acquire(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := l.acquire(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
	l.release()
	l.release()
	l.postpone(time.Hour)
	if err := l.acquire(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
	if len(l.slots) != 0 {
		t.Fatal("canceled waiter leaked a slot")
	}
}

func TestSharePointLimiterSharedByCredentials(t *testing.T) {
	a := sharedSharePointLimiter(t.Name(), "client")
	b := sharedSharePointLimiter(t.Name(), "client")
	c := sharedSharePointLimiter(t.Name(), "other-client")
	if a != b || a == c {
		t.Fatal("unexpected limiter scope")
	}
	t.Cleanup(func() {
		sharePointLimiters.Delete([2]string{t.Name(), "client"})
		sharePointLimiters.Delete([2]string{t.Name(), "other-client"})
	})
}

func TestSharePointThrottlePausesOtherConnector(t *testing.T) {
	requests := make(chan time.Time, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- time.Now()
		if r.URL.Path == "/v1.0/throttled" {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	defer server.Close()
	a := mustSharePointConnector(t, server.URL, nil)
	b := mustSharePointConnector(t, server.URL, nil)
	a.limiter = newSharePointLimiter(0)
	b.limiter = a.limiter
	b.accessToken, b.tokenExpiry = "token", time.Now().Add(time.Hour)
	// One real HTTP attempt publishes cooldown even if its task stops afterwards.
	_, _, _, err := a.graphRequestAttempt(t.Context(), server.URL+"/v1.0/throttled", "token", 1024, time.Second)
	if err == nil {
		t.Fatal("expected throttle")
	}
	if _, err = b.doGraphRequest(t.Context(), server.URL+"/v1.0/other", 1024); err != nil {
		t.Fatal(err)
	}
	first, second := <-requests, <-requests
	if second.Sub(first) < time.Second {
		t.Fatalf("other connector ignored cooldown: %s", second.Sub(first))
	}
}
