//go:build cgo

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

package component

import (
	"context"
	"strings"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/ingestion/component/progress"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"
)

// TestVisionSourceFetchDoesNotBlockImageAdmission pins the invariant that
// acquiring the source document for one document's PDF crops must not be done
// while holding the process-wide image admission slot.
//
// The two budgets are what make the ordering matter rather than merely being
// untidy: the fetch is bounded by visionPDFSourceBudget (90s) and runs on the
// invocation context, while a waiter only holds visionMediaItemBudget (10s) for
// the image slot. Holding the slot across the fetch therefore makes an
// unrelated document's image materialization fail on a timeout that no amount
// of capacity can fix — the waiter is outlasted, not outnumbered.
//
// Two concurrent calls model two documents parsed in the same process, which
// requires ingestor.max_concurrent_workers >= 2.
func TestVisionSourceFetchDoesNotBlockImageAdmission(t *testing.T) {
	if testing.Short() {
		t.Skip("needs the stub fetch to outlast visionMediaItemBudget")
	}

	invoker := &visionEnhanceCaptureInvoker{}
	swapVisionGlobals(t, fakeResolver, invoker.invoke, fakePrompt)

	originalFetcher := visionSourceFetcher
	fetchStarted := make(chan struct{}, 1)
	visionSourceFetcher = func(ctx context.Context, _, _ string) ([]byte, error) {
		select {
		case fetchStarted <- struct{}{}:
		default:
		}
		select {
		case <-time.After(visionMediaItemBudget + 2*time.Second):
		case <-ctx.Done():
		}
		// Non-PDF bytes: ensureEngine stops before opening an engine, so the
		// test exercises source acquisition without the native backend.
		return []byte("not a pdf"), nil
	}
	t.Cleanup(func() { visionSourceFetcher = originalFetcher })

	newItem := func() map[string]any {
		return map[string]any{
			"doc_type_kwd": "image",
			"text":         "existing caption",
			"positions":    [][]any{{[]any{1}, 50.0, 300.0, 200.0, 400.0}},
		}
	}
	run := func(inputs map[string]any) []string {
		dispatched := parser.ParseResult{OutputFormat: "json", JSON: parser.MapsToItems([]map[string]any{newItem()})}
		res, _, err := maybeDispatchVisionEnhancement(
			context.Background(), dao.DB, utility.FileTypePDF, dispatched,
			inputs, visionSettings{}, progress.Window{},
		)
		if err != nil {
			t.Errorf("maybeDispatchVisionEnhancement: %v", err)
		}
		return res.Warnings
	}

	// A's fetch is in flight — and must not be occupying the image slot.
	sourceHolder := make(chan []string, 1)
	go func() { sourceHolder <- run(map[string]any{"tenant_id": "t1", "bucket": "b", "path": "A.pdf"}) }()
	select {
	case <-fetchStarted:
	case <-time.After(visionMediaItemBudget):
		t.Fatal("source fetch never started")
	}

	// B materializes images while A's fetch is still running.
	for _, warning := range run(map[string]any{"tenant_id": "t1", "bucket": "b", "path": "B.pdf"}) {
		if strings.Contains(warning, "media admission failed") {
			t.Fatalf("image admission was blocked by another document's source fetch: %s", warning)
		}
	}
	<-sourceHolder
}
