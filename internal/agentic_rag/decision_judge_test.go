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
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
)

// resetDecisionBackend clears the memoised backend so a test can choose the
// env. The backend is resolved once per process on purpose — a judge that could
// change mid-run would make the run's threshold unreadable afterwards — so a
// test that wants a different one has to say so.
func resetDecisionBackend(t *testing.T) {
	t.Helper()
	decisionOnce = sync.Once{}
	decisionCurrent = nil
	t.Cleanup(func() {
		decisionOnce = sync.Once{}
		decisionCurrent = nil
	})
}

// decisionStub starts a server that answers every request with the given noul
// value for each of the n templates, and records what it was sent.
func decisionStub(t *testing.T, nouls []float64, status int, seen *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if seen != nil && body != nil {
			body["_auth"] = r.Header.Get("Authorization")
			*seen = body
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"message":"nope"}}`))
			return
		}
		// Answer whatever keys the request asked for: a batch carries one key per
		// (passage, phrasing), so a stub that answers a fixed t0..t3 would look
		// like "every passage came back unanswered" to the batched caller.
		answers := map[string]any{}
		if qs, ok := body["questions"].(map[string]any); ok {
			for key := range qs {
				var i, j int
				if _, err := fmt.Sscanf(key, "p%d_t%d", &i, &j); err != nil {
					continue
				}
				if j < 0 || j >= len(nouls) {
					continue
				}
				answers[key] = map[string]any{"type": "noul", "noul": nouls[j]}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": answers,
			"usage":   map[string]any{"input_tokens": 618},
		})
	}))
}

// backendFor builds a backend against the stub with the given threshold.
func backendFor(t *testing.T, url string, minProb float64) *decisionBackend {
	t.Helper()
	t.Setenv(common.EnvDecisionBackend, "decision-api")
	t.Setenv(common.EnvDecisionKey, "test-key")
	t.Setenv(common.EnvDecisionURL, url)
	t.Setenv(common.EnvDecisionMinProb, fmt.Sprintf("%g", minProb))
	resetDecisionBackend(t)
	b := decisionBackendFromEnv()
	if b == nil {
		t.Fatal("backend did not resolve from env")
	}
	return b
}

// The request has to match the contract the threshold was fitted under: four
// noul questions about ONE state, and the state holds the passage only. Putting
// the question in the state too was measured to cost separation (evidence
// median 0.215 -> 0.160), so the shape is pinned here rather than left to
// whoever next edits the payload.
func TestDecisionRequestShape(t *testing.T) {
	// These tests cover the multi-phrasing mechanics; the shipped default is one.
	t.Setenv("RAGFLOW_DECISION_PHRASINGS", "4")
	var seen map[string]any
	srv := decisionStub(t, []float64{.9, .8, .7, .6}, http.StatusOK, &seen)
	defer srv.Close()
	b := backendFor(t, srv.URL, 0.15)

	if _, _, err := b.ask(context.Background(), "Who wrote it?", "", "the passage"); err != nil {
		t.Fatalf("ask: %v", err)
	}
	if got := seen["_auth"]; got != "Bearer test-key" {
		t.Errorf("Authorization = %v, want %q", got, "Bearer test-key")
	}
	if got := seen["model"]; got != defaultDecisionModel {
		t.Errorf("model = %v, want %q", got, defaultDecisionModel)
	}
	state, ok := seen["state"].(map[string]any)
	if !ok {
		t.Fatalf("state is %T, want object", seen["state"])
	}
	// The passage travels inside each question, not in the state: one request
	// carries several passages and the state holds only one. The state stays
	// empty rather than being repurposed as the question, which was measured to
	// cost separation.
	if len(state) != 1 || state["passage"] != "" {
		t.Errorf("state = %v, want only {\"passage\": \"\"}", state)
	}
	questions, ok := seen["questions"].(map[string]any)
	if !ok {
		t.Fatalf("questions is %T, want object", seen["questions"])
	}
	if len(questions) != len(decisionRelevanceTemplates) {
		t.Fatalf("questions = %d entries, want %d", len(questions), len(decisionRelevanceTemplates))
	}
	bodies := map[string]bool{}
	for _, raw := range questions {
		q, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("question entry is %T", raw)
		}
		if q["type"] != "noul" {
			t.Errorf("question type = %v, want noul", q["type"])
		}
		ins, _ := q["instructions"].(string)
		if !strings.Contains(ins, "Who wrote it?") {
			t.Errorf("instructions %q do not carry the question", ins)
		}
		if !strings.Contains(ins, "the passage") {
			t.Errorf("instructions %q do not carry the passage", ins)
		}
		bodies[ins] = true
	}
	if len(bodies) != len(decisionRelevanceTemplates) {
		t.Errorf("%d distinct instructions, want %d — the four phrasings must differ",
			len(bodies), len(decisionRelevanceTemplates))
	}
}

// The verdict is the MEAN of the four phrasings, not the max: that is what the
// threshold was fitted to, and a passage the phrasings disagree about is one
// the run should keep.
func TestDecisionJudgeAveragesTemplates(t *testing.T) {
	// These tests cover the multi-phrasing mechanics; the shipped default is one.
	t.Setenv("RAGFLOW_DECISION_PHRASINGS", "4")
	srv := decisionStub(t, []float64{1.0, 0.2, 0.1, 0.1}, http.StatusOK, nil)
	defer srv.Close()

	low := backendFor(t, srv.URL, 0.15)
	v := low.judge(context.Background(), "q", []string{"passage"})
	if len(v) != 1 || !v[0].Judged || !v[0].Useful {
		t.Fatalf("verdicts = %+v, want one judged and useful (mean 0.35 >= 0.15)", v)
	}
	if diff := v[0].Probability - 0.35; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("probability = %v, want mean 0.35", v[0].Probability)
	}
	if v[0].Tokens != 618 {
		t.Errorf("tokens = %d, want the endpoint's input_tokens 618", v[0].Tokens)
	}
	if v[0].Latency <= 0 {
		t.Errorf("latency = %v, want a measured duration", v[0].Latency)
	}

	high := backendFor(t, srv.URL, 0.5)
	v = high.judge(context.Background(), "q", []string{"passage"})
	if !v[0].Judged || v[0].Useful {
		t.Errorf("with min_prob 0.5 the same 0.35 must be judged but NOT useful: %+v", v[0])
	}
}

// A probability of zero is a verdict, not a missing answer: the endpoint says
// "certainly not relevant" with it, and the difference matters because a
// missing answer keeps the passage while a zero drops it.
func TestDecisionJudgeTreatsZeroAsAVerdict(t *testing.T) {
	srv := decisionStub(t, []float64{0, 0, 0, 0}, http.StatusOK, nil)
	defer srv.Close()
	b := backendFor(t, srv.URL, 0.15)

	v := b.judge(context.Background(), "q", []string{"passage"})
	if !v[0].Judged {
		t.Errorf("a zero probability must still count as judged: %+v", v[0])
	}
	if v[0].Useful {
		t.Errorf("0.0 < 0.15 must not be useful: %+v", v[0])
	}
}

// Every failure keeps the passage. A filter that cannot run must not delete
// evidence, and this is the property that makes the outage survivable.
func TestDecisionJudgeKeepsOnFailure(t *testing.T) {
	for name, srv := range map[string]*httptest.Server{
		"http 500": decisionStub(t, nil, http.StatusInternalServerError, nil),
		"http 401": decisionStub(t, nil, http.StatusUnauthorized, nil),
	} {
		b := backendFor(t, srv.URL, 0.15)
		v := b.judge(context.Background(), "q", []string{"passage"})
		if v[0].Judged {
			t.Errorf("%s: judged=true on a failed call", name)
		}
		if !v[0].Useful {
			t.Errorf("%s: a passage that was never ruled on must be kept", name)
		}
		srv.Close()
	}

	// A body with no noul value at all is a body we cannot read, not a zero.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"answers":{"t0":{"type":"noul"}}}`))
	}))
	defer bad.Close()
	b := backendFor(t, bad.URL, 0.15)
	v := b.judge(context.Background(), "q", []string{"passage"})
	if v[0].Judged || !v[0].Useful {
		t.Errorf("a response without any noul value must keep the passage: %+v", v[0])
	}

	// An empty question cannot be asked, so nothing is judged and all is kept.
	b = backendFor(t, "http://127.0.0.1:1/none", 0.15)
	v = b.judge(context.Background(), "", []string{"passage"})
	if v[0].Judged || !v[0].Useful {
		t.Errorf("no question means no judgement and no loss: %+v", v[0])
	}
}

// One unreachable segment must not cost the others their verdicts.
func TestDecisionJudgeIsPerSegment(t *testing.T) {
	// One passage per request, so the middle request fails on its own: with
	// batching, a failure costs the whole batch, and this pins the boundary.
	t.Setenv(common.EnvDecisionBatch, "1")
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 2 {
			w.WriteHeader(http.StatusInternalServerError)
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
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers})
	}))
	defer srv.Close()
	b := backendFor(t, srv.URL, 0.15)

	v := b.judge(context.Background(), "q", []string{"a", "b", "c"})
	if !v[0].Judged || !v[0].Useful {
		t.Errorf("segment 0 = %+v, want judged and kept", v[0])
	}
	if v[1].Judged || !v[1].Useful {
		t.Errorf("segment 1 = %+v, want unjudged and kept", v[1])
	}
	if !v[2].Judged || !v[2].Useful {
		t.Errorf("segment 2 = %+v, want judged and kept", v[2])
	}
	if v[1].Segment != "b" || v[2].Segment != "c" {
		t.Errorf("segments were reordered: %q %q", v[1].Segment, v[2].Segment)
	}
}

// The cap cuts cleanly at the character the calibration used. An ellipsis would
// change the text the model reads, and the calibration never saw one.
func TestDecisionSplitWholeCutsCleanly(t *testing.T) {
	t.Setenv(common.EnvDecisionBackend, "decision-api")
	t.Setenv(common.EnvDecisionKey, "test-key")
	t.Setenv(common.EnvDecisionMaxChars, "10")
	resetDecisionBackend(t)
	b := decisionBackendFromEnv()
	if b == nil {
		t.Fatal("backend did not resolve")
	}

	long := "abcdefghijklmnop" // 16 chars
	segs, overflow := b.splitWhole(long, 5000)
	if len(segs) != 1 || segs[0] != "abcdefghij" {
		t.Fatalf("splitWhole = %q, want one clean 10-char segment", segs)
	}
	if overflow == "" {
		t.Error("overflow should carry the text past the window, so a caller can tell truncation happened")
	}
	if strings.Contains(segs[0], "...") {
		t.Error("the judged text must not gain an ellipsis the calibration never saw")
	}

	short := "abc"
	segs, overflow = b.splitWhole(short, 5000)
	if len(segs) != 1 || segs[0] != short || overflow != "" {
		t.Errorf("splitWhole(short) = %q/%q, want the text unchanged and no overflow", segs, overflow)
	}

	// Sub-segmenting is not this backend's job: one passage is one question.
	if got := b.split(long); len(got) != 1 {
		t.Errorf("split returned %d segments, want 1", len(got))
	}
	if got := b.split("   "); got != nil {
		t.Errorf("blank text = %v, want nil", got)
	}
}

// Selecting the backend is what turns it on; without the env, or without a key,
// the filter must stay exactly as it was.
func TestDecisionBackendGating(t *testing.T) {
	t.Setenv(common.EnvDecisionBackend, "")
	t.Setenv(common.EnvDecisionKey, "")
	resetDecisionBackend(t)
	if b := decisionBackendFromEnv(); b != nil {
		t.Error("no backend env means no backend")
	}

	t.Setenv(common.EnvDecisionBackend, "decision-api")
	resetDecisionBackend(t)
	if b := decisionBackendFromEnv(); b != nil {
		t.Error("a backend without a key would 401 on every passage, so it must not exist")
	}

	t.Setenv(common.EnvDecisionKey, "k")
	resetDecisionBackend(t)
	if b := decisionBackendFromEnv(); b == nil {
		t.Error("backend + key must resolve")
	}
	if got := decisionBackendFromEnv(); got != decisionCurrent {
		t.Error("resolution must be memoised, not rebuilt per call")
	}
}

// The dispatch itself: the filter's seams must reach the backend, and the
// timeout must be a real one rather than an unbounded wait inside a tool call.
func TestDecisionJudgeDispatchesToDecisionBackend(t *testing.T) {
	srv := decisionStub(t, []float64{0.9, 0.9, 0.9, 0.9}, http.StatusOK, nil)
	defer srv.Close()
	backendFor(t, srv.URL, 0.15)

	ctx := context.Background()
	if !decisionAvailable(ctx) {
		t.Fatal("decisionAvailable must be true when the backend is selected")
	}
	segs, _ := decisionSplit(ctx, "a passage")
	if len(segs) != 1 {
		t.Fatalf("decisionSplit = %v, want one segment", segs)
	}
	window, _ := decisionSplitWhole(ctx, "a passage", 5000)
	if len(window) != 1 || window[0] != "a passage" {
		t.Fatalf("decisionSplitWhole = %v, want the passage", window)
	}
	v := decisionJudge(context.Background(), "q", segs)
	if len(v) != 1 || !v[0].Judged || !v[0].Useful {
		t.Fatalf("decisionJudge = %+v, want a judged, useful verdict from the stub", v)
	}

	// A stalled endpoint must give up inside the tool call and keep the text.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{"t0": map[string]any{"type": "noul", "noul": 0.0}},
		})
	}))
	defer slow.Close()
	t.Setenv(common.EnvDecisionURL, slow.URL)
	t.Setenv(common.EnvDecisionTimeoutMS, "50")
	resetDecisionBackend(t)
	if b := decisionBackendFromEnv(); b == nil {
		t.Fatal("backend did not resolve")
	}
	v = decisionJudge(context.Background(), "q", []string{"passage"})
	if v[0].Judged || !v[0].Useful {
		t.Errorf("a timed-out judgement must keep the passage: %+v", v[0])
	}
}
