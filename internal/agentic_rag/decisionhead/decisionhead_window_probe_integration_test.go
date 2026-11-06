//go:build integration

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

package decisionhead

// Window-policy probe: judges ONE fixed set of passages under both the split
// policy (512-token segments, max 12) and the whole-passage policy (single
// window, truncated at 5000), so the two can be compared without the confound
// that a live run judges a different passage set every time.
//
// It is an integration test because it needs the real model directory and the
// inference sidecar; there is no fake that would answer the question being asked.
// Fixtures are read from $DECISIONHEAD_PROBE_IN (default /tmp/decisionhead_probe_input.json) and
// the verdicts are written to $DECISIONHEAD_PROBE_OUT (default /tmp/decisionhead_probe_output.json).

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

type windowProbeCase struct {
	ID       string `json:"id"`
	QID      string `json:"qid"`
	Question string `json:"question"`
	Text     string `json:"text"`
}

type windowProbeResult struct {
	ID          string  `json:"id"`
	QID         string  `json:"qid"`
	Style       string  `json:"style"`
	SplitKept   bool    `json:"split_kept"`
	SplitSegs   int     `json:"split_segs"`
	SplitMinP   float64 `json:"split_min_p"`
	WholeKept   bool    `json:"whole_kept"`
	WholeTokens int     `json:"whole_tokens"`
	WholeP      float64 `json:"whole_p"`
}

func probePath(env, def string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return def
}

// minProbabilityOf returns the smallest probability the model ruled on, and
// whether anything was ruled on at all. An unjudged verdict carries no
// probability, so it is skipped rather than counted as zero.
func minProbabilityOf(verdicts []Verdict) (float64, bool) {
	min, seen := 0.0, false
	for _, v := range verdicts {
		if !v.Judged {
			continue
		}
		if !seen || v.Probability < min {
			min, seen = v.Probability, true
		}
	}
	return min, seen
}

func anyUseful(verdicts []Verdict) bool {
	for _, v := range verdicts {
		if v.Useful {
			return true
		}
	}
	return false
}

func TestDecisionheadWindowPolicyProbe(t *testing.T) {
	if !Available() {
		t.Skipf("Laya unavailable: set RAGFLOW_DECISION_DIR and start the inference sidecar")
	}
	raw, err := os.ReadFile(probePath("DECISIONHEAD_PROBE_IN", "/tmp/decisionhead_probe_input.json"))
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var cases []windowProbeCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("parse fixtures: %v", err)
	}

	ctx := context.Background()
	results := make([]windowProbeResult, 0, len(cases))
	for _, c := range cases {
		segments, _ := Split(c.Text)
		splitVerdicts := Judge(ctx, c.Question, segments)
		splitMin, _ := minProbabilityOf(splitVerdicts)

		window, _ := SplitWhole(c.Text, 5000)
		wholeVerdicts := Judge(ctx, c.Question, window)
		wholeMin, _ := minProbabilityOf(wholeVerdicts)

		results = append(results, windowProbeResult{
			ID:          c.ID,
			QID:         c.QID,
			Style:       ConfigFromEnv().QuestionStyle,
			SplitKept:   anyUseful(splitVerdicts),
			SplitSegs:   len(splitVerdicts),
			SplitMinP:   splitMin,
			WholeKept:   anyUseful(wholeVerdicts),
			WholeTokens: len(window),
			WholeP:      wholeMin,
		})
	}

	out, err := json.MarshalIndent(results, "", " ")
	if err != nil {
		t.Fatalf("encode results: %v", err)
	}
	if err := os.WriteFile(probePath("DECISIONHEAD_PROBE_OUT", "/tmp/decisionhead_probe_output.json"), out, 0o644); err != nil {
		t.Fatalf("write results: %v", err)
	}
	t.Logf("probed %d passages", len(results))
}
