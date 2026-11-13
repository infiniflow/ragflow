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

package nats

import (
	"reflect"
	"testing"

	"ragflow/internal/common"
)

// TestIngestorLanesCommunity pins the community ingestor-lane mapping: the
// open-source build owns exactly one lane (default), so every valid ingestor
// tier collapses to [default], and any unknown tier is an error rather than a
// silent fallback. This guards against a future refactor silently widening or
// narrowing the allowed lane set.
func TestIngestorLanesCommunity(t *testing.T) {
	cases := []struct {
		tier common.Tier
		want []common.Tier
		err  bool
	}{
		{common.TierDefault, []common.Tier{common.TierDefault}, false},
		{common.Tier("high"), nil, true},   // enterprise-only lane name
		{common.Tier("medium"), nil, true}, // enterprise-only lane name
		{common.Tier("bogus"), nil, true},
	}
	for _, c := range cases {
		got, err := ingestorLanes(c.tier)
		if c.err {
			if err == nil {
				t.Errorf("ingestorLanes(%q): expected error, got nil", c.tier)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ingestorLanes(%q): unexpected error: %v", c.tier, err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("ingestorLanes(%q) = %v, want %v", c.tier, got, c.want)
		}
	}
}

// TestPullMessagesForTierCommunity exercises the tier-restricted pull against a
// real (embedded) NATS server. Only the default lane exists in the community
// build, so pulling with TierDefault must return the published task, and pulling
// with an enterprise-only tier name must error.
func TestPullMessagesForTierCommunity(t *testing.T) {
	queue := newTaskQueue(t)
	publishTask(t, queue, "default-only")

	ctx, cancel := taskPullContext(t)
	defer cancel()
	handles, err := queue.PullMessagesForTier(ctx, common.TierDefault, 5)
	if err != nil {
		t.Fatalf("PullMessagesForTier(default): %v", err)
	}
	if len(handles) != 1 || handles[0].GetMessage().TaskID != "default-only" {
		t.Fatalf("PullMessagesForTier(default) = %v, want exactly the default-only task", handles)
	}

	ctx2, cancel2 := taskPullContext(t)
	defer cancel2()
	if _, err := queue.PullMessagesForTier(ctx2, common.Tier("high"), 5); err == nil {
		t.Fatalf("PullMessagesForTier(high) expected error in community build, got nil")
	}
}
