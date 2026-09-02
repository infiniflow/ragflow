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
	"fmt"
	"strings"
	"sync"
	"time"
)

// Phase is a named timing bucket in the RAG pipeline
type Phase string

const (
	PhaseCheckLLM        Phase = "check_llm"
	PhaseCheckLangfuse   Phase = "check_langfuse"
	PhaseBindModels      Phase = "bind_models"
	PhaseQueryRefinement Phase = "query_refinement"
	PhaseRetrieval       Phase = "retrieval"
	PhaseGenerateAnswer  Phase = "generate_answer"
)

// allPhases ordered for Markdown() display.
var allPhases = []Phase{
	PhaseCheckLLM,
	PhaseCheckLangfuse,
	PhaseBindModels,
	PhaseQueryRefinement,
	PhaseRetrieval,
	PhaseGenerateAnswer,
}

// Timer tracks elapsed wall-clock time per named Phase.
// Supports reentrant Enter/Exit on the same phase (inner span's duration
// adds to the outer span's accumulated total).
type Timer struct {
	mu      sync.Mutex
	start   time.Time
	phases  map[Phase]time.Duration
	entries map[Phase][]time.Time
}

// NewTimer constructs a Timer.
func NewTimer() *Timer {
	return &Timer{
		phases:  make(map[Phase]time.Duration, len(allPhases)),
		entries: make(map[Phase][]time.Time, len(allPhases)),
	}
}

// Start anchors the timer. Calling Start() twice resets all state.
func (t *Timer) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.start = time.Now()
	t.phases = make(map[Phase]time.Duration, len(allPhases))
	t.entries = make(map[Phase][]time.Time, len(allPhases))
}

// Enter marks the start of phase p. Reentrant calls push a new anchor.
func (t *Timer) Enter(p Phase) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries[p] = append(t.entries[p], time.Now())
}

// Exit records the duration since the most recent Enter(p). No-op if no Enter.
func (t *Timer) Exit(p Phase) {
	t.mu.Lock()
	defer t.mu.Unlock()
	stack := t.entries[p]
	if len(stack) == 0 {
		return
	}
	open := stack[len(stack)-1]
	t.entries[p] = stack[:len(stack)-1]
	t.phases[p] += time.Since(open)
}

// Phase returns the accumulated duration for phase p.
func (t *Timer) Phase(p Phase) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.phases[p]
}

// Total returns the elapsed time since Start().
func (t *Timer) Total() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.start.IsZero() {
		return 0
	}
	return time.Since(t.start)
}

// PhaseReport is the JSON-serializable view of a Timer's state.
type PhaseReport struct {
	PhasesMs map[string]float64 `json:"phases_ms"`
	TotalMs  float64            `json:"total_ms"`
}

// Report returns a JSON-marshalable snapshot with microsecond precision.
func (t *Timer) Report() *PhaseReport {
	t.mu.Lock()
	defer t.mu.Unlock()
	phases := make(map[string]float64, len(allPhases))
	for _, p := range allPhases {
		phases[string(p)] = float64(t.phases[p].Microseconds()) / 1000.0
	}
	var totalMs float64
	if !t.start.IsZero() {
		totalMs = float64(time.Since(t.start).Microseconds()) / 1000.0
	}
	return &PhaseReport{PhasesMs: phases, TotalMs: totalMs}
}

func (t *Timer) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.Report())
}

// Markdown renders the Timer as a "## Time elapsed:" block matching
func (t *Timer) Markdown() string {
	r := t.Report()
	var b strings.Builder
	b.WriteString("\n## Time elapsed:\n")
	b.WriteString(fmt.Sprintf("  - Total: %.1fms\n", r.TotalMs))
	for _, p := range allPhases {
		ms := r.PhasesMs[string(p)]
		b.WriteString(fmt.Sprintf("  - %s: %.1fms\n", displayName(p), ms))
	}
	b.WriteString("\n")
	return b.String()
}

func displayName(p Phase) string {
	switch p {
	case PhaseCheckLLM:
		return "Check LLM"
	case PhaseCheckLangfuse:
		return "Check Langfuse tracer"
	case PhaseBindModels:
		return "Bind models"
	case PhaseQueryRefinement:
		return "Query refinement(LLM)"
	case PhaseRetrieval:
		return "Retrieval"
	case PhaseGenerateAnswer:
		return "Generate answer"
	default:
		return string(p)
	}
}
