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

package connector

import (
	"testing"
	"time"
)

func TestHoldWaterlineKeepsWindowEndWithoutFailures(t *testing.T) {
	windowEnd := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)
	if got := HoldWaterline(windowEnd, nil); !got.Equal(windowEnd) {
		t.Fatalf("waterline = %v, want window end %v", got, windowEnd)
	}
}

func TestHoldWaterlineHoldsBehindEarliestFailedDocument(t *testing.T) {
	windowEnd := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)
	earliest := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	later := time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)
	failures := []SyncFailure{
		{SourceID: "b", UpdatedAt: later},
		{SourceID: "a", UpdatedAt: earliest},
		// A failure without a modification time must not hold the waterline:
		// the source re-selects such documents on every run.
		{SourceID: "nodate"},
	}
	// The selection window is strict half-open (start, end], so the held
	// waterline must be strictly before the failed document's mtime.
	if got, want := HoldWaterline(windowEnd, failures), earliest.Add(-time.Nanosecond); !got.Equal(want) {
		t.Fatalf("waterline = %v, want %v", got, want)
	}
}

func TestHoldWaterlineKeepsWindowEndWhenAllFailuresPostdateIt(t *testing.T) {
	windowEnd := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)
	failures := []SyncFailure{{SourceID: "future", UpdatedAt: windowEnd.Add(time.Hour)}}
	if got := HoldWaterline(windowEnd, failures); !got.Equal(windowEnd) {
		t.Fatalf("waterline = %v, want window end %v", got, windowEnd)
	}
}
