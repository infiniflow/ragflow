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

package advanced_rag

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// TestThinkLoggerForwardsTaggedLines is the core think_log contract: a
// bracket-tagged stage line reaches the per-request sink, and the backend log
// still receives it unchanged.
func TestThinkLoggerForwardsTaggedLines(t *testing.T) {
	var logged bytes.Buffer
	orig := log.New(&logged, "", 0)

	var got []string
	l := thinkLogger(orig, func(line string) { got = append(got, line) })
	if l == nil {
		t.Fatal("thinkLogger returned nil")
	}

	l.Printf("[Planner] Splitting the research question into tasks")
	l.Printf("[Hybrid search] Searching the knowledge base for %q", "what is X")
	l.Printf("this line has no tag")
	l.Printf("[1] first evidence chunk that is not a stage tag")

	if len(got) != 2 {
		t.Fatalf("forwarded %d line(s), want 2: %#v", len(got), got)
	}
	// The think block is HTML, so each line must end with ThinkLineBreak
	// (Python's "<br>"); a bare newline collapses into a space and the stages
	// run together.
	if got[0] != "[Planner] Splitting the research question into tasks"+ThinkLineBreak {
		t.Errorf("line 0 = %q, want a ThinkLineBreak-terminated stage line", got[0])
	}
	if !strings.Contains(got[1], "[Hybrid search]") || !strings.HasSuffix(got[1], ThinkLineBreak) {
		t.Errorf("line 1 = %q, want a ThinkLineBreak-terminated [Hybrid search] line", got[1])
	}

	// Backend logging must be preserved for every line, tagged or not.
	out := logged.String()
	for _, want := range []string{
		"[Planner] Splitting the research question into tasks",
		"[Hybrid search]",
		"this line has no tag",
		"[1] first evidence chunk",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("backend log missing %q:\n%s", want, out)
		}
	}
}

// TestThinkLoggerNilSinkIsPassThrough guards the no-progress case: with no sink
// the original logger is reused so behaviour is byte-for-byte unchanged.
func TestThinkLoggerNilSinkIsPassThrough(t *testing.T) {
	orig := log.New(&bytes.Buffer{}, "", 0)
	if got := thinkLogger(orig, nil); got != orig {
		t.Fatal("nil sink must return the original logger")
	}
}

// TestThinkLoggerSurvivesPanickingSink mirrors Python's bare try/except around
// the sink call: a broken sink must never break logging or the request.
func TestThinkLoggerSurvivesPanickingSink(t *testing.T) {
	var logged bytes.Buffer
	orig := log.New(&logged, "", 0)
	l := thinkLogger(orig, func(string) { panic("sink exploded") })

	l.Printf("[Orchestrator] round 1") // must not panic

	if !strings.Contains(logged.String(), "[Orchestrator] round 1") {
		t.Errorf("backend log lost the line:\n%s", logged.String())
	}
}

func TestIsEvidenceMarker(t *testing.T) {
	cases := map[string]bool{
		"[1] chunk text":      true,
		"[12] another chunk":  true,
		"[Planner] splitting": false,
		"[Hybrid search] x":   false,
		"[SCA] review":        false,
		"[":                   false,
		"[]":                  false,
	}
	for line, want := range cases {
		if got := isEvidenceMarker(line); got != want {
			t.Errorf("isEvidenceMarker(%q) = %v, want %v", line, got, want)
		}
	}
}
