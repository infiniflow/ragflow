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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/common"
)

// failingStub answers with the given status and counts requests. The status can
// be changed while the test runs, so one server can model "down, then back up".
func failingStub(t *testing.T, status *int32) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		code := int(atomic.LoadInt32(status))
		if code != http.StatusOK {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"error":{"message":"down"}}`))
			return
		}
		answers := map[string]any{}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if qs, ok := body["questions"].(map[string]any); ok {
			for key := range qs {
				answers[key] = map[string]any{"type": "noul", "noul": 0.9}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": answers,
			"usage":   map[string]any{"input_tokens": 100},
		})
	}))
	return srv, &calls
}

// Past the threshold the gate must stop calling the endpoint and let every
// passage through — the documented degraded state, "no decision model as a
// gate" — instead of paying one timeout per passage for an answer it cannot get.
func TestBreakerOpensAndStopsCallingTheJudge(t *testing.T) {
	// These tests cover the multi-phrasing mechanics; the shipped default is one.
	t.Setenv("RAGFLOW_DECISION_PHRASINGS", "4")
	status := int32(http.StatusInternalServerError)
	srv, calls := failingStub(t, &status)
	defer srv.Close()
	t.Setenv(common.EnvDecisionBreakerFailures, "3")
	t.Setenv(common.EnvDecisionBreakerCooldownMS, "60000")
	b := backendFor(t, srv.URL, 0.15)

	segments := make([]string, 10)
	for i := range segments {
		segments[i] = fmt.Sprintf("passage %d", i)
	}
	verdicts := b.judge(context.Background(), "q", segments)

	if got := atomic.LoadInt32(calls); got != 3 {
		t.Fatalf("requests = %d, want 3 (three failures should open the breaker)", got)
	}
	for i, v := range verdicts {
		if v.Judged {
			t.Errorf("passage %d was marked judged after the breaker opened", i)
		}
		if !v.Useful {
			t.Errorf("passage %d was dropped; a failed judgement must keep its passage", i)
		}
	}
	if !b.breaker.isOpen() {
		t.Error("breaker should be open")
	}
}

// After the cooldown one probe is let through. A probe that fails re-opens the
// breaker rather than letting every passage try again.
func TestBreakerLetsOneProbeThroughAfterCooldown(t *testing.T) {
	status := int32(http.StatusInternalServerError)
	srv, calls := failingStub(t, &status)
	defer srv.Close()
	t.Setenv(common.EnvDecisionBreakerFailures, "1")
	t.Setenv(common.EnvDecisionBreakerCooldownMS, "50")
	b := backendFor(t, srv.URL, 0.15)

	b.judge(context.Background(), "q", []string{"first"}) // fails, opens
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
	// Inside the cooldown nothing is sent: this is the part that stops the run
	// paying one timeout per passage while the endpoint is down.
	b.judge(context.Background(), "q", []string{"second"})
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("requests = %d, want 1: the cooldown must block the call", got)
	}
	time.Sleep(60 * time.Millisecond)
	b.judge(context.Background(), "q", []string{"third"}) // probe, fails, re-opens
	if got := atomic.LoadInt32(calls); got != 2 {
		t.Fatalf("requests = %d, want 2: exactly one probe should be let through", got)
	}
	b.judge(context.Background(), "q", []string{"fourth"}) // open again
	if got := atomic.LoadInt32(calls); got != 2 {
		t.Fatalf("requests = %d, want 2: a failed probe must re-open the breaker", got)
	}
}

// A success between failures resets the count: the threshold is about a broken
// endpoint, not about a lifetime total.
func TestBreakerSuccessResetsTheCount(t *testing.T) {
	status := int32(http.StatusInternalServerError)
	srv, calls := failingStub(t, &status)
	defer srv.Close()
	t.Setenv(common.EnvDecisionBreakerFailures, "2")
	t.Setenv(common.EnvDecisionBreakerCooldownMS, "60000")
	b := backendFor(t, srv.URL, 0.15)

	b.judge(context.Background(), "q", []string{"a"}) // 1 failure
	atomic.StoreInt32(&status, http.StatusOK)
	b.judge(context.Background(), "q", []string{"b"}) // success, counter resets
	atomic.StoreInt32(&status, http.StatusInternalServerError)
	b.judge(context.Background(), "q", []string{"c"}) // 1 failure again
	if b.breaker.isOpen() {
		t.Fatal("breaker opened on the second lifetime failure: the count should have reset")
	}
	b.judge(context.Background(), "q", []string{"d"}) // 2nd consecutive
	if !b.breaker.isOpen() {
		t.Fatal("breaker should be open after two consecutive failures")
	}
	if got := atomic.LoadInt32(calls); got != 4 {
		t.Fatalf("requests = %d, want 4", got)
	}
}

// The point of the breaker: when the endpoint hangs, the run must not pay the
// timeout per passage. Ten passages at a 200ms timeout would be 2s serial; with
// the breaker at 3 it is about 3 timeouts.
func TestBreakerCapsTheTimeSpentOnADeadEndpoint(t *testing.T) {
	// These tests cover the multi-phrasing mechanics; the shipped default is one.
	t.Setenv("RAGFLOW_DECISION_PHRASINGS", "4")
	var calls int32
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(2 * time.Second)
	}))
	defer slow.Close()
	t.Setenv(common.EnvDecisionTimeoutMS, "200")
	t.Setenv(common.EnvDecisionBreakerFailures, "3")
	t.Setenv(common.EnvDecisionBreakerCooldownMS, "60000")
	b := backendFor(t, slow.URL, 0.15)

	segments := make([]string, 10)
	for i := range segments {
		segments[i] = fmt.Sprintf("passage %d", i)
	}
	start := time.Now()
	verdicts := b.judge(context.Background(), "q", segments)
	elapsed := time.Since(start)

	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("requests = %d, want 3", got)
	}
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("elapsed %v: the breaker did not stop the run paying per passage", elapsed)
	}
	for i, v := range verdicts {
		if !v.Useful {
			t.Fatalf("passage %d dropped on timeout; the degraded path must keep", i)
		}
	}
}
