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

package nats

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func TestIsConsumerGoneErr(t *testing.T) {
	gone := []error{
		jetstream.ErrConsumerNotFound,
		jetstream.ErrConsumerDeleted,
		nats.ErrNoResponders,
		fmt.Errorf("failed to fetch messages: %w", jetstream.ErrConsumerNotFound),
		fmt.Errorf("failed to fetch messages: %w", nats.ErrNoResponders),
		errors.New("failed to fetch messages: nats: no responders available for request"),
		errors.New("failed to fetch messages: consumer not found"),
		errors.New("failed to fetch messages: consumer deleted"),
	}
	for _, err := range gone {
		if !isConsumerGoneErr(err) {
			t.Errorf("isConsumerGoneErr(%v) = false, want true", err)
		}
	}

	notGone := []error{
		nil,
		context.DeadlineExceeded,
		context.Canceled,
		errors.New("failed to fetch messages: nats: timeout"),
		errors.New("NATS consumer is nil, engine not properly initialized"),
	}
	for _, err := range notGone {
		if isConsumerGoneErr(err) {
			t.Errorf("isConsumerGoneErr(%v) = true, want false", err)
		}
	}
}

// A pull must repair itself when the consumer behind the stored handle is gone.
// Before this, the fetch loop answered "no responders available for request"
// forever: the workers kept logging errors, the queue stopped draining, and only
// restarting the process cleared it.
//
// Note on what recovery does and does not cover: the task stream is a
// WorkQueuePolicy stream, so messages that were pending for a deleted consumer
// are gone with it - recreating the consumer restores the pull plumbing, it does
// not resurrect those messages. Asserting that is the point of this test: after
// the repair, a pull must stop erroring and newly published tasks must flow
// again.
func TestPullMessages_RecreatesConsumerWhenItDisappears(t *testing.T) {
	queue := newTaskQueue(t)

	publishTask(t, queue, "before-delete")
	ctx, cancel := taskPullContext(t)
	handle, err := queue.PullMessage(ctx)
	cancel()
	if err != nil {
		t.Fatalf("PullMessage: %v", err)
	}
	if handle == nil || handle.GetMessage().TaskID != "before-delete" {
		t.Fatalf("handle = %v, want before-delete", handle)
	}
	if ackErr := handle.Ack(); ackErr != nil {
		t.Fatalf("ack: %v", ackErr)
	}

	// Delete the consumer behind the engine's back; the stored handle now
	// addresses something that no longer exists.
	if err := queue.stream.DeleteConsumer(t.Context(), "RAGFLOW_CONSUMER"); err != nil {
		t.Fatalf("delete consumer: %v", err)
	}

	// The first pull after the deletion is the one that must self-heal: it may
	// come back empty, but it must not come back with the no-responders error.
	pullCtx, pullCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer pullCancel()
	if _, err := queue.PullMessage(pullCtx); err != nil {
		t.Fatalf("PullMessage after the consumer was deleted: %v", err)
	}

	// The consumer must exist again under the same durable name, so the next
	// restart reattaches instead of orphaning a replacement.
	if _, err := queue.stream.Consumer(t.Context(), "RAGFLOW_CONSUMER"); err != nil {
		t.Fatalf("consumer was not recreated: %v", err)
	}

	// And the recreated consumer must actually deliver: publish after the repair
	// and pull it back.
	publishTask(t, queue, "after-repair")
	recovered, err := queue.PullMessage(pullCtx)
	if err != nil {
		t.Fatalf("PullMessage after the repair: %v", err)
	}
	if recovered == nil || recovered.GetMessage().TaskID != "after-repair" {
		t.Fatalf("recovered handle = %v, want after-repair", recovered)
	}
}

// A healthy pull leaves the handle alone: the repair path must not run on the
// ordinary "nothing to deliver" outcome.
func TestPullMessages_KeepsHandleWhenNothingToDeliver(t *testing.T) {
	queue := newTaskQueue(t)
	before, err := queue.consumerHandle()
	if err != nil {
		t.Fatalf("consumerHandle: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if _, err := queue.PullMessage(ctx); err != nil {
		t.Fatalf("PullMessage on an empty queue: %v", err)
	}

	after, err := queue.consumerHandle()
	if err != nil {
		t.Fatalf("consumerHandle: %v", err)
	}
	if before != after {
		t.Fatal("the consumer handle was replaced although nothing went wrong")
	}
}
