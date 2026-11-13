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
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/common"
)

const (
	defaultBreakerFailures   = 3
	defaultBreakerCooldownMS = 60000
)

// decisionBreaker switches the gate off when the judge stops answering.
//
// A failed judgement already keeps its passage, so an unreachable endpoint
// never costs evidence. What it does cost is the timeout, per passage, for the
// rest of the run: at the 3s default one result set of sixteen hits spends 48
// seconds waiting on an endpoint that is down, and a run of eighty judgements
// spends minutes — while every verdict comes back "keep", i.e. the run behaves
// exactly like the ungated baseline but takes far longer. Waiting is the only
// thing the gate still does at that point, so it stops.
//
// After `threshold` consecutive failures the breaker opens and the gate is OFF
// for `cooldown`: the documented degraded state, "no decision model as a
// gate", with every passage passing through untouched. One probe is let
// through when the cooldown expires — success closes the breaker, failure
// re-opens it and restarts the cooldown. Transitions are logged once each, not
// per passage, so a run that lost its judge is still identifiable afterwards.
type decisionBreaker struct {
	mu        sync.Mutex
	threshold int
	cooldown  time.Duration
	fails     int
	open      bool
	openedAt  time.Time
	probing   bool
}

func newDecisionBreaker() *decisionBreaker {
	n := envIntDefault(common.EnvDecisionBreakerFailures, defaultBreakerFailures)
	if n < 1 {
		n = 1
	}
	// Parsed here rather than through envIntDefault, which treats 0 as "use the
	// default": a zero cooldown is meaningful (probe on the very next passage)
	// and must not silently become a minute.
	ms := defaultBreakerCooldownMS
	if raw := strings.TrimSpace(common.GetEnv(common.EnvDecisionBreakerCooldownMS)); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 0 {
			ms = v
		}
	}
	return &decisionBreaker{threshold: n, cooldown: time.Duration(ms) * time.Millisecond}
}

// allow reports whether a judgement may be sent now. While the breaker is open
// it also hands out the single probe allowed once the cooldown has elapsed.
func (b *decisionBreaker) allow() bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.open {
		return true
	}
	if time.Since(b.openedAt) < b.cooldown {
		return false
	}
	if b.probing {
		return false
	}
	b.probing = true
	return true
}

// success records an answer, closing the breaker if it was open.
func (b *decisionBreaker) success(ctx context.Context) {
	if b == nil {
		return
	}
	b.mu.Lock()
	wasOpen := b.open
	b.fails = 0
	b.open = false
	b.probing = false
	b.mu.Unlock()
	if wasOpen {
		common.InfoCtx(ctx, "agentic_rag: decision judge recovered, gate back on")
	}
}

// failure records a failed judgement, opening the breaker at the threshold.
func (b *decisionBreaker) failure(ctx context.Context, err error) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.fails++
	b.probing = false
	justOpened := false
	switch {
	case !b.open && b.fails >= b.threshold:
		b.open = true
		b.openedAt = time.Now()
		justOpened = true
	case b.open:
		// The probe failed: start the cooldown again rather than retrying on
		// every subsequent passage.
		b.openedAt = time.Now()
	}
	fails, cooldown := b.fails, b.cooldown
	b.mu.Unlock()
	if justOpened {
		common.WarnCtx(ctx, "agentic_rag: decision judge unavailable, gate off until it answers",
			zap.Int("consecutive_failures", fails),
			zap.Int("threshold", b.threshold),
			zap.Duration("cooldown", cooldown),
			zap.String("error", err.Error()))
	}
}

// open reports the current state, for tests and for callers that want to say
// whether a run was actually gated.
func (b *decisionBreaker) isOpen() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.open
}
