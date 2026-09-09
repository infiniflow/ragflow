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
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTimer_BasicSequentialPhases(t *testing.T) {
	tm := NewTimer()
	tm.Start()

	tm.Enter(PhaseCheckLLM)
	time.Sleep(5 * time.Millisecond)
	tm.Exit(PhaseCheckLLM)

	tm.Enter(PhaseBindModels)
	time.Sleep(3 * time.Millisecond)
	tm.Exit(PhaseBindModels)

	got := tm.Phase(PhaseCheckLLM)
	if got < 4*time.Millisecond || got > 50*time.Millisecond {
		t.Errorf("PhaseCheckLLM = %v, want ~5ms", got)
	}
	got = tm.Phase(PhaseBindModels)
	if got < 2*time.Millisecond || got > 50*time.Millisecond {
		t.Errorf("PhaseBindModels = %v, want ~3ms", got)
	}

	// Untouched phase should be 0.
	if d := tm.Phase(PhaseRetrieval); d != 0 {
		t.Errorf("PhaseRetrieval = %v, want 0", d)
	}

	total := tm.Total()
	if total < 7*time.Millisecond {
		t.Errorf("Total = %v, want >= 7ms", total)
	}
}

func TestTimer_NestedPhasesAddUp(t *testing.T) {
	tm := NewTimer()
	tm.Start()

	tm.Enter(PhaseQueryRefinement) // outer
	time.Sleep(2 * time.Millisecond)
	tm.Enter(PhaseGenerateAnswer) // inner (LLM call inside pre-retrieval)
	time.Sleep(3 * time.Millisecond)
	tm.Exit(PhaseGenerateAnswer)
	time.Sleep(1 * time.Millisecond)
	tm.Exit(PhaseQueryRefinement)

	// Generate answer records the inner 3ms.
	got := tm.Phase(PhaseGenerateAnswer)
	if got < 2*time.Millisecond || got > 50*time.Millisecond {
		t.Errorf("PhaseGenerateAnswer = %v, want ~3ms", got)
	}
	// Pre-retrieval processing records the WHOLE outer span (2 + 3 + 1 ≈ 6ms).
	got = tm.Phase(PhaseQueryRefinement)
	if got < 5*time.Millisecond || got > 50*time.Millisecond {
		t.Errorf("PhaseQueryRefinement = %v, want ~6ms (outer span)", got)
	}
}

func TestTimer_ExitWithoutEnterIsNoop(t *testing.T) {
	tm := NewTimer()
	tm.Start()
	// Should not panic, should not record anything.
	tm.Exit(PhaseRetrieval)
	if d := tm.Phase(PhaseRetrieval); d != 0 {
		t.Errorf("PhaseRetrieval = %v, want 0", d)
	}
}

func TestTimer_StartResetsState(t *testing.T) {
	tm := NewTimer()
	tm.Start()
	tm.Enter(PhaseCheckLLM)
	time.Sleep(2 * time.Millisecond)
	tm.Exit(PhaseCheckLLM)
	if tm.Phase(PhaseCheckLLM) == 0 {
		t.Fatal("precondition: phase must be non-zero before reset")
	}
	tm.Start()
	if d := tm.Phase(PhaseCheckLLM); d != 0 {
		t.Errorf("after Start, PhaseCheckLLM = %v, want 0", d)
	}
	if total := tm.Total(); total > 50*time.Millisecond {
		t.Errorf("after Start, Total = %v, want tiny", total)
	}
}

func TestTimer_ConcurrentAccess(t *testing.T) {
	tm := NewTimer()
	tm.Start()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Go(func() {
			tm.Enter(PhaseRetrieval)
			time.Sleep(time.Millisecond)
			tm.Exit(PhaseRetrieval)
		})
	}
	wg.Wait()
	got := tm.Phase(PhaseRetrieval)
	if got < 9*time.Millisecond {
		t.Errorf("PhaseRetrieval = %v, want ~10ms (10 parallel spans)", got)
	}
}

func TestTimer_Report(t *testing.T) {
	tm := NewTimer()
	tm.Start()
	tm.Enter(PhaseCheckLLM)
	time.Sleep(2 * time.Millisecond)
	tm.Exit(PhaseCheckLLM)
	tm.Enter(PhaseBindModels)
	time.Sleep(1 * time.Millisecond)
	tm.Exit(PhaseBindModels)

	r := tm.Report()
	// Required fields
	if _, ok := r.PhasesMs[string(PhaseCheckLLM)]; !ok {
		t.Errorf("Report missing PhaseCheckLLM: %+v", r.PhasesMs)
	}
	if _, ok := r.PhasesMs[string(PhaseBindModels)]; !ok {
		t.Errorf("Report missing PhaseBindModels: %+v", r.PhasesMs)
	}
	if _, ok := r.PhasesMs[string(PhaseGenerateAnswer)]; !ok {
		t.Errorf("Report missing PhaseGenerateAnswer: %+v", r.PhasesMs)
	}
	if r.PhasesMs[string(PhaseCheckLLM)] < 1.0 {
		t.Errorf("Report PhaseCheckLLM_ms = %v, want >= 1.0", r.PhasesMs[string(PhaseCheckLLM)])
	}
	if r.TotalMs < 2.0 {
		t.Errorf("Report TotalMs = %v, want >= 2.0", r.TotalMs)
	}

	// JSON round-trip
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if !strings.Contains(string(b), `"phases_ms"`) || !strings.Contains(string(b), `"total_ms"`) {
		t.Errorf("JSON missing expected keys: %s", b)
	}

	// Direct Marshal of the Timer
	b2, err := json.Marshal(tm)
	if err != nil {
		t.Fatalf("Marshal(Timer) failed: %v", err)
	}
	if !strings.Contains(string(b2), `"phases_ms"`) {
		t.Errorf("Timer JSON missing phases_ms: %s", b2)
	}
}

func TestTimer_Markdown(t *testing.T) {
	tm := NewTimer()
	tm.Start()
	tm.Enter(PhaseCheckLLM)
	time.Sleep(2 * time.Millisecond)
	tm.Exit(PhaseCheckLLM)
	tm.Enter(PhaseRetrieval)
	time.Sleep(5 * time.Millisecond)
	tm.Exit(PhaseRetrieval)
	tm.Enter(PhaseGenerateAnswer)
	time.Sleep(50 * time.Millisecond)
	tm.Exit(PhaseGenerateAnswer)

	md := tm.Markdown()

	// Should start with newline + "## Time elapsed:" header
	if !strings.HasPrefix(md, "\n## Time elapsed:") {
		t.Errorf("Markdown missing header: %q", md)
	}
	// Should contain all 6 phase labels
	for _, label := range []string{"Check LLM", "Check Langfuse tracer", "Bind models", "Query refinement(LLM)", "Retrieval", "Generate answer", "Total"} {
		if !strings.Contains(md, label+":") {
			t.Errorf("Markdown missing label %q: %q", label, md)
		}
	}
	// Phase durations should be numeric with "ms" suffix.
	mdRE := regexp.MustCompile(`(?m)^\s*-\s+([A-Za-z ()\.]+):\s+([0-9.]+)ms$`)
	matches := mdRE.FindAllStringSubmatch(md, -1)
	if len(matches) < 7 {
		t.Errorf("expected 7 phase lines, found %d in:\n%s", len(matches), md)
	}
	// Total should be the sum-ish of the three measured phases.
	totalRE := regexp.MustCompile(`Total:\s+([0-9.]+)ms`)
	totalMatch := totalRE.FindStringSubmatch(md)
	if len(totalMatch) < 2 {
		t.Fatalf("Markdown missing Total line: %q", md)
	}
}

func TestTimer_TotalBeforeStart(t *testing.T) {
	tm := NewTimer()
	// No Start() called.
	if total := tm.Total(); total != 0 {
		t.Errorf("Total before Start = %v, want 0", total)
	}
}
