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

package service

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	ingestion "ragflow/internal/ingestion"
)

var errTierPull = errors.New("tier pull reached")

// tierPullTestQueue advertises the tier-restricted pull capability and records
// the tier it was asked to drain. The embedded interface is left nil: only the
// overridden method is exercised.
type tierPullTestQueue struct {
	engine.MessageQueue
	gotTier common.Tier
}

func (q *tierPullTestQueue) PullMessageForTier(_ context.Context, tier common.Tier) (common.TaskHandle, error) {
	q.gotTier = tier
	return nil, errTierPull
}

// legacyPullTestQueue implements only the base queue surface, so an engine
// without the tier capability must fall back to PullMessage.
type legacyPullTestQueue struct {
	engine.MessageQueue
	called bool
}

func (q *legacyPullTestQueue) PullMessage(_ context.Context) (common.TaskHandle, error) {
	q.called = true
	return nil, errTierPull
}

// TestPullOneTaskRoutesThroughTierCapability verifies the consume-loop helper
// uses the tier-restricted pull when the engine advertises it, and forwards
// the ingestor's configured tier verbatim. A sentinel tier (not the default)
// makes a hard-coded TierDefault implementation fail.
func TestPullOneTaskRoutesThroughTierCapability(t *testing.T) {
	queue := &tierPullTestQueue{}
	ing := NewIngestor("tier-capability-test", 1, common.Tier("sentinel-tier"), &ingestion.Deps{MQ: queue})

	_, err := ing.pullOneTask(t.Context())
	if !errors.Is(err, errTierPull) {
		t.Fatalf("pullOneTask error = %v, want the tier puller's error", err)
	}
	if queue.gotTier != common.Tier("sentinel-tier") {
		t.Fatalf("tier puller got tier %q, want the ingestor's configured sentinel tier", queue.gotTier)
	}
}

// TestPullOneTaskFallsBackWithoutTierCapability verifies an engine that lacks
// the tier-restricted pull keeps the legacy whole-queue path.
func TestPullOneTaskFallsBackWithoutTierCapability(t *testing.T) {
	queue := &legacyPullTestQueue{}
	ing := NewIngestor("legacy-capability-test", 1, common.TierDefault, &ingestion.Deps{MQ: queue})

	_, err := ing.pullOneTask(t.Context())
	if !errors.Is(err, errTierPull) {
		t.Fatalf("pullOneTask error = %v, want the legacy puller's error", err)
	}
	if !queue.called {
		t.Fatal("legacy PullMessage was not called for an engine without tier capability")
	}
}
