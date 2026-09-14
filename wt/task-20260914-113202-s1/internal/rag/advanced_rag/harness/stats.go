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

package harness

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/agent/chat"
)

// LLM-call instrumentation for the harness.
//
// Mirrors Python harness/stats.py. Every phase of the pipeline (route /
// planner / orchestrator / direct / sufficiency / finalize, ...) drives the LLM,
// and each call is attributed to the phase that was executing. Phase wall-clock
// is measured here rather than summing LLM latency, which is meaningless when
// calls run in parallel.
//
// Python stores the current phase and the active stats in ContextVars so
// parallel asyncio tasks get independent accounting. Go has no ContextVars; the
// same isolation is achieved by carrying both on the context.Context, which is
// already per-request (and per-goroutine when a phase fans out).

// Canonical pipeline order for the per-phase usage table. Phases are listed in
// execution order so the log reads top-to-bottom like the actual flow,
// regardless of which phase first touched the counters. Phases not in this list
// are appended afterwards, alphabetically.
// This list matches Python _PHASE_ORDER (harness/stats.py) EXACTLY, so a Go run
// and a Python run emit comparable usage tables. Python's _PHASE_ORDER omits
// dynamic/draft/sca/rewrite (they are real @in_phase phases in Python too, but
// not in its canonical list, so Python appends them alphabetically at the end);
// compute is a Go-only phase (arithmetic.Compute). The snapshot/Log logic below
// re-appends any phase not present here alphabetically, mirroring Python, so all
// of them still appear in both runs in the same relative position.
var phaseOrder = []string{
	"formalize",
	"route",
	"planner",
	"decompose",
	"direct",
	"orchestrator",
	"claim_research",
	"sufficiency",
	"grounded",
	"finalize",
}

// Phase names used by the harness (Python @in_phase labels).
const (
	PhaseFormalize     = "formalize"
	PhaseRoute         = "route"
	PhasePlanner       = "planner"
	PhaseDecompose     = "decompose"
	PhaseDynamic       = "dynamic"
	PhaseDirect        = "direct"
	PhaseOrchestrator  = "orchestrator"
	PhaseClaimResearch = "claim_research"
	PhaseDraft         = "draft"
	PhaseSufficiency   = "sufficiency"
	PhaseSCA           = "sca"
	PhaseRewrite       = "rewrite"
	PhaseCompute       = "compute"
	PhaseGrounded      = "grounded"
	PhaseFinalize      = "finalize"
)

// phaseRoundKey identifies a phase within one orchestrator round.
type phaseRoundKey struct {
	phase string
	round int
}

// LLMUsageStats holds per-phase LLM call, wall-clock, and token counters for
// one harness run.
type LLMUsageStats struct {
	mu sync.Mutex

	calls            map[string]int
	failed           map[string]int
	phaseTimeMs      map[string]float64
	promptTokens     map[string]int
	completionTokens map[string]int
	totalTokens      map[string]int
	rounds           map[string]int
	roundTimes       map[string][]float64
	// roundPhaseTimesMs is the phase wall-clock split per orchestrator round
	// (index 0 = round 1). A phase that runs several times inside one round
	// accumulates into that round.
	roundPhaseTimesMs map[string][]float64
	roundClaimCounts  map[string][]int

	roundStarts   map[string]time.Time
	currentRound  int
	phaseActive   map[string]int
	phaseStarts   map[string]time.Time
	roundPhaseAct map[phaseRoundKey]int
	roundPhaseSt  map[phaseRoundKey]time.Time
}

// NewLLMUsageStats builds an empty counter set.
func NewLLMUsageStats() *LLMUsageStats {
	return &LLMUsageStats{
		calls:             map[string]int{},
		failed:            map[string]int{},
		phaseTimeMs:       map[string]float64{},
		promptTokens:      map[string]int{},
		completionTokens:  map[string]int{},
		totalTokens:       map[string]int{},
		rounds:            map[string]int{},
		roundTimes:        map[string][]float64{},
		roundPhaseTimesMs: map[string][]float64{},
		roundClaimCounts:  map[string][]int{},
		roundStarts:       map[string]time.Time{},
		phaseActive:       map[string]int{},
		phaseStarts:       map[string]time.Time{},
		roundPhaseAct:     map[phaseRoundKey]int{},
		roundPhaseSt:      map[phaseRoundKey]time.Time{},
	}
}

// elapsedMs returns a monotonic-aware wall duration in milliseconds. Python's
// stats use time.perf_counter() (a monotonic clock); Go's time.Time carries a
// monotonic reading too as long as we keep the struct (time.Now().UnixNano()
// would strip it and expose wall-clock step-backs), so we subtract the stored
// start time directly. This matches perf_counter's guarantee that phase/round
// durations never go negative when the system clock is stepped backwards.
func elapsedMs(now, start time.Time) float64 {
	return now.Sub(start).Seconds() * 1000.0
}

// CurrentRound is the 1-based index of the orchestrator round executing, 0 outside.
func (s *LLMUsageStats) CurrentRound() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentRound
}

// RecordCall counts one LLM call in phaseName.
func (s *LLMUsageStats) RecordCall(phaseName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls[phaseName]++
}

// RecordFailed counts one failed LLM call in phaseName.
func (s *LLMUsageStats) RecordFailed(phaseName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed[phaseName]++
}

// RecordUsage adds provider-reported token usage to phaseName.
func (s *LLMUsageStats) RecordUsage(phaseName string, prompt, completion, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.promptTokens[phaseName] += prompt
	s.completionTokens[phaseName] += completion
	s.totalTokens[phaseName] += total
}

// RecordRound counts one iteration of a looping phase (e.g. an orchestrator
// cycle) and closes the previous iteration's wall-clock.
func (s *LLMUsageStats) RecordRound(phaseName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rounds[phaseName]++
	s.currentRound = s.rounds[phaseName]
	now := time.Now()
	if prev, ok := s.roundStarts[phaseName]; ok {
		s.roundTimes[phaseName] = append(s.roundTimes[phaseName], elapsedMs(now, prev))
	}
	s.roundStarts[phaseName] = now
}

// RecordRoundClaims records how many claim-level tasks ran in the current round.
func (s *LLMUsageStats) RecordRoundClaims(phaseName string, count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentRound <= 0 {
		return
	}
	counts := s.roundClaimCounts[phaseName]
	for len(counts) < s.currentRound {
		counts = append(counts, 0)
	}
	counts[s.currentRound-1] += count
	s.roundClaimCounts[phaseName] = counts
}

// notePhaseEnter / notePhaseExit implement the re-entrancy rule from Python:
// the same phase may be wrapped several times along one call path, and only the
// outermost interval is timed.
func (s *LLMUsageStats) notePhaseEnter(phaseName string, entryRound int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.phaseActive[phaseName]++
	if s.phaseActive[phaseName] == 1 {
		s.phaseStarts[phaseName] = now
	}
	if entryRound > 0 {
		k := phaseRoundKey{phaseName, entryRound}
		s.roundPhaseAct[k]++
		if s.roundPhaseAct[k] == 1 {
			s.roundPhaseSt[k] = now
		}
	}
}

func (s *LLMUsageStats) notePhaseExit(phaseName string, entryRound int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if active := s.phaseActive[phaseName]; active > 0 {
		active--
		if active == 0 {
			start, ok := s.phaseStarts[phaseName]
			if !ok {
				start = now
			}
			delete(s.phaseStarts, phaseName)
			delete(s.phaseActive, phaseName)
			s.phaseTimeMs[phaseName] += elapsedMs(now, start)
			pending := s.rounds[phaseName] - len(s.roundTimes[phaseName])
			if pending > 0 {
				settled := 0.0
				for _, t := range s.roundTimes[phaseName] {
					settled += t
				}
				s.roundTimes[phaseName] = append(s.roundTimes[phaseName], max(0.0, s.phaseTimeMs[phaseName]-settled))
				delete(s.roundStarts, phaseName)
			}
			if phaseName == PhaseOrchestrator {
				s.currentRound = 0
			}
		} else {
			s.phaseActive[phaseName] = active
		}
	}
	if entryRound > 0 {
		k := phaseRoundKey{phaseName, entryRound}
		if active := s.roundPhaseAct[k]; active > 0 {
			active--
			if active == 0 {
				start, ok := s.roundPhaseSt[k]
				if !ok {
					start = now
				}
				delete(s.roundPhaseSt, k)
				delete(s.roundPhaseAct, k)
				times := s.roundPhaseTimesMs[phaseName]
				for len(times) < entryRound {
					times = append(times, 0.0)
				}
				times[entryRound-1] += elapsedMs(now, start)
				s.roundPhaseTimesMs[phaseName] = times
			} else {
				s.roundPhaseAct[k] = active
			}
		}
	}
}

// Snapshot mirrors Python LLMUsageStats.snapshot: rows keyed by phase in
// canonical pipeline order.
func (s *LLMUsageStats) Snapshot() map[string]map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	known := map[string]struct{}{}
	for k := range s.calls {
		known[k] = struct{}{}
	}
	for k := range s.failed {
		known[k] = struct{}{}
	}
	for k := range s.totalTokens {
		known[k] = struct{}{}
	}
	for k := range s.phaseTimeMs {
		known[k] = struct{}{}
	}
	for k := range s.rounds {
		known[k] = struct{}{}
	}
	phases := make([]string, 0, len(known))
	for _, p := range phaseOrder {
		if _, ok := known[p]; ok {
			phases = append(phases, p)
			delete(known, p)
		}
	}
	for p := range known {
		phases = append(phases, p)
	}
	sort.Strings(phases[len(phases)-len(known):])

	rows := make(map[string]map[string]any, len(phases))
	for _, p := range phases {
		perRound := s.roundPhaseTimesMs[p]
		rounds, roundTimes := s.rounds[p], append([]float64(nil), s.roundTimes[p]...)
		if len(perRound) > 0 {
			rounds, roundTimes = len(perRound), append([]float64(nil), perRound...)
		}
		rows[p] = map[string]any{
			"calls":                s.calls[p],
			"failed":               s.failed[p],
			"phase_time_ms":        s.phaseTimeMs[p],
			"prompt_tokens":        s.promptTokens[p],
			"completion_tokens":    s.completionTokens[p],
			"total_tokens":         s.totalTokens[p],
			"rounds":               rounds,
			"round_times":          roundTimes,
			"round_claim_counts":   append([]int(nil), s.roundClaimCounts[p]...),
			"round_phase_times_ms": append([]float64(nil), s.roundPhaseTimesMs[p]...),
		}
	}
	return rows
}

// ---------------------------------------------------------------------------
// Context plumbing
// ---------------------------------------------------------------------------

type statsCtxKey struct{}
type phaseCtxKey struct{}
type progressCtxKey struct{}

// WithStats binds stats to ctx so every LLM call beneath it is attributed there.
func WithStats(ctx context.Context, stats *LLMUsageStats) context.Context {
	return context.WithValue(ctx, statsCtxKey{}, stats)
}

// CurrentStats returns the stats bound to ctx, or nil.
func CurrentStats(ctx context.Context) *LLMUsageStats {
	if s, ok := ctx.Value(statsCtxKey{}).(*LLMUsageStats); ok {
		return s
	}
	return nil
}

// WithProgress binds a per-request progress sink to ctx so every engine stage and
// every search beneath it can forward tagged lines (Python think_log counterpart)
// to the caller's live reasoning block. progress may be nil to disable.
func WithProgress(ctx context.Context, progress func(string)) context.Context {
	return context.WithValue(ctx, progressCtxKey{}, progress)
}

// CurrentProgress returns the progress sink bound to ctx, or nil.
func CurrentProgress(ctx context.Context) func(string) {
	if p, ok := ctx.Value(progressCtxKey{}).(func(string)); ok {
		return p
	}
	return nil
}

// CurrentPhase returns the phase executing in ctx ("unknown" when none set).
func CurrentPhase(ctx context.Context) string {
	if p, ok := ctx.Value(phaseCtxKey{}).(string); ok && p != "" {
		return p
	}
	return "unknown"
}

// Phase mirrors Python's phase() context manager: it marks the enclosed block
// as executing `name` and accrues its wall-clock into the bound stats.
//
// The returned func MUST be called when the block ends (defer it). Nesting the
// same phase name is supported: only the outermost interval is timed, so the
// time is not counted two or three times along a call path.
func Phase(ctx context.Context, name string) (context.Context, func()) {
	stats := CurrentStats(ctx)
	round := 0
	if stats != nil {
		round = stats.CurrentRound()
		stats.notePhaseEnter(name, round)
	}
	child := context.WithValue(ctx, phaseCtxKey{}, name)
	return child, func() {
		if stats != nil {
			stats.notePhaseExit(name, round)
		}
	}
}

// InPhase mirrors Python's in_phase decorator: it runs fn inside Phase(name).
func InPhase(ctx context.Context, name string, fn func(context.Context) error) error {
	ctx, done := Phase(ctx, name)
	defer done()
	return fn(ctx)
}

// RecordExternalResponse mirrors Python record_external_response: records a raw
// completion response that bypasses CountingInvoker, including token usage.
// action_session calls this for the two raw model calls it makes directly
// (action_session.py:_acompletion/1086).
func RecordExternalResponse(ctx context.Context, resp *chat.Response) {
	stats := CurrentStats(ctx)
	if stats == nil {
		return
	}
	phase := CurrentPhase(ctx)
	stats.RecordCall(phase)
	if resp != nil {
		recordResponseUsage(stats, phase, resp.Usage, resp.Tokens)
	}
}

// recordResponseUsage attributes one call's token usage, honouring the split
// when it is present and falling back to the single total counter otherwise.
func recordResponseUsage(stats *LLMUsageStats, phase string, usage *chat.Usage, totalOnly int) {
	if usage != nil {
		stats.RecordUsage(phase, usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
		return
	}
	if totalOnly > 0 {
		stats.RecordUsage(phase, 0, 0, totalOnly)
	}
}

// RecordRound mirrors Python record_round.
func RecordRound(ctx context.Context, name string) {
	if s := CurrentStats(ctx); s != nil {
		s.RecordRound(name)
	}
}

// RecordRoundClaims mirrors Python record_round_claims.
func RecordRoundClaims(ctx context.Context, name string, count int) {
	if s := CurrentStats(ctx); s != nil {
		s.RecordRoundClaims(name, count)
	}
}

// Log emits the per-phase usage table. Mirrors Python LLMUsageStats.log.
//
// With orchestrator-round data it expands hierarchically: each round repeats
// its "orchestrator" row with the nested sub-phases (claim_research /
// sufficiency / grounded) indented underneath, matching the Python rendering.
// Phases outside the loop (route / planner / finalize) are listed flat.
func (s *LLMUsageStats) Log(logger *log.Logger) {
	rows := s.Snapshot()
	if len(rows) == 0 {
		// No LLM activity (e.g. a cache hit). Still emit a line so every
		// completed run is accounted for instead of silently disappearing.
		if logger != nil {
			logger.Println("[Agentic RAG] LLM usage by phase: (cached / no LLM calls)")
		}
		return
	}
	totalCalls, totalTokens := 0, 0
	for _, r := range rows {
		totalCalls += r["calls"].(int)
		totalTokens += r["total_tokens"].(int)
	}

	// Canonical order: phases in phaseOrder first, then any leftover sorted.
	seen := map[string]bool{}
	phases := make([]string, 0, len(rows))
	for _, p := range phaseOrder {
		if _, ok := rows[p]; ok {
			phases = append(phases, p)
			seen[p] = true
		}
	}
	rest := make([]string, 0, len(rows)-len(phases))
	for p := range rows {
		if !seen[p] {
			rest = append(rest, p)
		}
	}
	sort.Strings(rest)
	phases = append(phases, rest...)

	// Per-round structure, read from the locked snapshot (Python reads
	// self.round_phase_times_ms / self.round_times directly).
	orchRT := []float64{}
	if orchRow, ok := rows[PhaseOrchestrator]; ok {
		if v, ok := orchRow["round_times"].([]float64); ok {
			orchRT = v
		}
	}
	perRound := map[string][]float64{}
	for p, r := range rows {
		if v, ok := r["round_phase_times_ms"].([]float64); ok && len(v) > 0 {
			perRound[p] = v
		}
	}
	nRounds := len(orchRT)
	for _, v := range perRound {
		if len(v) > nRounds {
			nRounds = len(v)
		}
	}

	lines := []string{
		"[Agentic RAG] LLM usage by phase:",
		fmt.Sprintf("  %-16s %7s %10s %12s %10s %10s", "phase", "llm_calls", "prompt_tok", "output_tok", "total_tok", "time(s)"),
	}

	// phaseLabel mirrors Python's phase_label: claim_research gets a
	// "(N)" suffix with the round's claim count when known.
	phaseLabel := func(p string, r map[string]any, roundIdx int) string {
		label := p
		if p == PhaseClaimResearch {
			if counts, ok := r["round_claim_counts"].([]int); ok && roundIdx < len(counts) && counts[roundIdx] > 0 {
				label = fmt.Sprintf("%s (%d)", p, counts[roundIdx])
			}
		}
		return label
	}
	// row mirrors Python's custom_row / row: the label is printed verbatim
	// (orchestrator round headers pass a custom label), token columns come
	// from rows[p]).
	row := func(indent, label, p string, tMs float64) string {
		r := rows[p]
		return fmt.Sprintf("%s%-16s %7d %10d %12d %10d %10.1f",
			indent, label, r["calls"].(int), r["prompt_tokens"].(int), r["completion_tokens"].(int), r["total_tokens"].(int), tMs/1000.0)
	}

	inRounds := make(map[string]bool, len(perRound))
	for k := range perRound {
		inRounds[k] = true
	}

	for _, p := range phases {
		if p == PhaseOrchestrator && nRounds > 0 {
			for i := 0; i < nRounds; i++ {
				orchT := 0.0
				if i < len(orchRT) {
					orchT = orchRT[i]
				} else {
					orchT = rows[p]["phase_time_ms"].(float64)
				}
				lines = append(lines, row("  ", fmt.Sprintf("orchestrator round %d", i+1), p, orchT))
				for _, sub := range phases {
					if sub == PhaseOrchestrator {
						continue
					}
					v, ok := perRound[sub]
					if !ok || i >= len(v) {
						continue
					}
					lines = append(lines, row("    ", phaseLabel(sub, rows[sub], i), sub, v[i]))
				}
			}
		} else if inRounds[p] && nRounds > 0 {
			// Already printed as a nested sub-phase of each orchestrator round.
			continue
		} else {
			lines = append(lines, row("  ", p, p, rows[p]["phase_time_ms"].(float64)))
		}
	}
	lines = append(lines, fmt.Sprintf("  total: %d LLM calls, %d tokens", totalCalls, totalTokens))
	if logger != nil {
		logger.Println(joinLines(lines))
	}
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}

// CountingInvoker mirrors Python CountingChatModel: it wraps a chat.Invoker and
// records calls / failures / token usage against the phase carried on ctx.
//
// Unlike the Python proxy (which falls back to a bundle-wide stats object), the
// Go version records only when stats are bound to the context — matching how
// Python's _CURRENT_STATS resolution behaves for the innermost active stats.
type CountingInvoker struct {
	Inner chat.Invoker
	Stats *LLMUsageStats
}

// Invoke implements chat.Invoker.
func (c *CountingInvoker) Invoke(ctx context.Context, db *gorm.DB, req chat.Request) (*chat.Response, error) {
	stats := CurrentStats(ctx)
	if stats == nil {
		stats = c.Stats
	}
	phase := CurrentPhase(ctx)
	if stats != nil {
		stats.RecordCall(phase)
	}
	resp, err := c.Inner.Invoke(ctx, db, req)
	if err != nil {
		if stats != nil {
			stats.RecordFailed(phase)
		}
		return nil, err
	}
	if stats != nil && resp != nil {
		recordResponseUsage(stats, phase, resp.Usage, resp.Tokens)
	}
	return resp, nil
}

// Stream implements chat.StreamingInvoker so a wrapped invoker keeps streaming
// AND is counted. Without this, StreamComplete's type assertion
// (m.Invoker.(chat.StreamingInvoker)) fails on the wrapper and the caller falls
// back to a one-shot (non-streaming) Invoke — losing both the stream and the
// accounting. Mirrors Python CountingChatModel.async_chat_streamly /
// async_chat_streamly_delta, which record usage in a finally block so the call
// is counted even when the inner model raised (last_usage is None then, so
// record_usage is a no-op — the same outcome as the success-only branch below).
//
// record_usage is taken from the returned *Response, mirroring Python's
// _last_usage(self._chat_mdl). If the inner invoker is not itself a
// StreamingInvoker, we decline rather than silently downgrade to a blocking
// Invoke, so callers keep their existing "fall back to the one-shot call"
// behaviour.
func (c *CountingInvoker) Stream(ctx context.Context, db *gorm.DB, req chat.Request, onDelta func(delta string, isThink bool) error) (*chat.Response, error) {
	stats := CurrentStats(ctx)
	if stats == nil {
		stats = c.Stats
	}
	phase := CurrentPhase(ctx)
	if stats != nil {
		stats.RecordCall(phase)
	}
	inner, ok := c.Inner.(chat.StreamingInvoker)
	if !ok {
		return nil, fmt.Errorf("harness: chat invoker %T does not support streaming", c.Inner)
	}
	resp, err := inner.Stream(ctx, db, req, onDelta)
	if err != nil {
		if stats != nil {
			stats.RecordFailed(phase)
		}
		return nil, err
	}
	if stats != nil && resp != nil {
		recordResponseUsage(stats, phase, resp.Usage, resp.Tokens)
	}
	return resp, nil
}
