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

package tokenizer

import (
	"context"
	"testing"
	"time"
)

// TestRunUsageCallSnapshot pins the per-LLM-call breakdown: the aggregate and
// the per-call records must agree, and each record must carry its own input /
// output split, the model that served it, and a monotonic start offset.
func TestRunUsageCallSnapshot(t *testing.T) {
	ctx := WithRunUsage(context.Background())
	RecordRunTokenUsageFor(ctx, "MiniMax-M3", 1200, 80, 1280)
	time.Sleep(2 * time.Millisecond)
	RecordRunTokenUsage(ctx, 3000, 150, 3150) // no model known: still recorded

	sink := GetRunUsage(ctx)
	pt, ct, tt, calls := sink.Snapshot()
	if pt != 4200 || ct != 230 || tt != 4430 || calls != 2 {
		t.Fatalf("aggregate = %d/%d/%d calls=%d", pt, ct, tt, calls)
	}
	turns := sink.CallSnapshot()
	if len(turns) != 2 {
		t.Fatalf("expected 2 per-call records, got %d", len(turns))
	}
	if turns[0].Seq != 1 || turns[0].PromptTokens != 1200 || turns[0].CompletionTokens != 80 || turns[0].Model != "MiniMax-M3" {
		t.Fatalf("first call wrong: %+v", turns[0])
	}
	if turns[1].Seq != 2 || turns[1].Model != "" || turns[1].TotalTokens != 3150 {
		t.Fatalf("second call wrong: %+v", turns[1])
	}
	if turns[1].AtSeconds < turns[0].AtSeconds {
		t.Fatalf("call offsets must be non-decreasing: %v then %v", turns[0].AtSeconds, turns[1].AtSeconds)
	}
	// Without a sink installed, recording is a no-op (not a panic).
	RecordRunTokenUsageFor(context.Background(), "m", 1, 1, 2)
}
