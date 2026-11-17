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
	"context"
	"errors"
	"testing"
	"time"

	"ragflow/internal/common"
)

// TestTierPullMirrorsWholeQueuePull characterizes both public pull entry
// points against the same boundary and error fixtures. PullMessagesForTier
// must stay observationally identical to PullMessages for the only tier this
// build accepts; these guards are what make sharing their drain loop safe.
func TestTierPullMirrorsWholeQueuePull(t *testing.T) {
	t.Run("out of range count", func(t *testing.T) {
		queue := newTaskQueue(t)
		ctx, cancel := taskPullContext(t)
		defer cancel()
		for _, count := range []int{0, -1, common.MaxManualPullMessages + 1} {
			if _, err := queue.PullMessages(ctx, count); err == nil {
				t.Errorf("PullMessages(%d) error = nil, want rejection", count)
			}
			if _, err := queue.PullMessagesForTier(ctx, common.TierDefault, count); err == nil {
				t.Errorf("PullMessagesForTier(%d) error = nil, want rejection", count)
			}
		}
	})

	t.Run("missing deadline", func(t *testing.T) {
		queue := newTaskQueue(t)
		if _, err := queue.PullMessages(t.Context(), 1); err == nil {
			t.Error("PullMessages without deadline error = nil, want rejection")
		}
		if _, err := queue.PullMessagesForTier(t.Context(), common.TierDefault, 1); err == nil {
			t.Error("PullMessagesForTier without deadline error = nil, want rejection")
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		queue := newTaskQueue(t)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		cancel()
		if _, err := queue.PullMessages(ctx, 1); !errors.Is(err, context.Canceled) {
			t.Errorf("PullMessages error = %v, want context.Canceled", err)
		}
		if _, err := queue.PullMessagesForTier(ctx, common.TierDefault, 1); !errors.Is(err, context.Canceled) {
			t.Errorf("PullMessagesForTier error = %v, want context.Canceled", err)
		}
	})

	t.Run("empty queue yields no handle", func(t *testing.T) {
		queue := newTaskQueue(t)
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		handle, err := queue.PullMessageForTier(ctx, common.TierDefault)
		if err != nil {
			t.Fatalf("PullMessageForTier on empty queue: %v", err)
		}
		if handle != nil {
			t.Fatalf("PullMessageForTier on empty queue = %v, want nil", handle)
		}
	})

	t.Run("single published task", func(t *testing.T) {
		queue := newTaskQueue(t)
		publishTask(t, queue, "mirrored-pull")
		ctx, cancel := taskPullContext(t)
		defer cancel()
		handles, err := queue.PullMessages(ctx, 1)
		if err != nil || len(handles) != 1 || handles[0].GetMessage().TaskID != "mirrored-pull" {
			t.Fatalf("PullMessages = %v, %v", handles, err)
		}
		publishTask(t, queue, "mirrored-tier-pull")
		ctx2, cancel2 := taskPullContext(t)
		defer cancel2()
		handles2, err := queue.PullMessagesForTier(ctx2, common.TierDefault, 1)
		if err != nil || len(handles2) != 1 || handles2[0].GetMessage().TaskID != "mirrored-tier-pull" {
			t.Fatalf("PullMessagesForTier = %v, %v", handles2, err)
		}
	})
}

// TestUninitializedEngineTierPullReturnsError extends the uninitialized-engine
// guard to the tier-aware pull added by this change.
func TestUninitializedEngineTierPullReturnsError(t *testing.T) {
	e := NewNatsEngine("127.0.0.1", 1)
	if _, err := e.PullMessageForTier(t.Context(), common.TierDefault); err == nil {
		t.Fatal("PullMessageForTier on uninitialized engine error = nil, want rejection")
	}
}
