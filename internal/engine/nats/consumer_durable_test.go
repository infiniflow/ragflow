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
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"ragflow/internal/common"
)

func assertDurableTaskConsumer(t *testing.T, queue *NatsEngine) {
	t.Helper()
	consumer, err := queue.stream.Consumer(t.Context(), taskConsumerName)
	if err != nil {
		t.Fatalf("task consumer: %v", err)
	}
	cfg := consumer.CachedInfo().Config
	if cfg.Durable != taskConsumerName {
		t.Errorf("Durable = %q, want %q", cfg.Durable, taskConsumerName)
	}
	if cfg.InactiveThreshold != 0 {
		t.Errorf("InactiveThreshold = %s, want 0 (never reaped)", cfg.InactiveThreshold)
	}
}

// The task consumer must be durable: an ephemeral one is reaped after 5s
// without a pull, so any ingestor downtime recreated it with fresh delivery
// counters and every poison message ran MaxDeliver times again.
func TestInitConsumer_CreatesDurableConsumer(t *testing.T) {
	assertDurableTaskConsumer(t, newTaskQueue(t))
}

// A deployment that still has the ephemeral RAGFLOW_CONSUMER must come up
// with a durable one, and the unacknowledged task must still be delivered.
func TestInitConsumer_ReplacesEphemeralConsumer(t *testing.T) {
	host, port := newEmbeddedNatsServer(t)
	queue := NewNatsEngine(host, port)
	if err := queue.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	ephemeral, err := queue.stream.CreateOrUpdateConsumer(t.Context(), jetstream.ConsumerConfig{
		Name:          taskConsumerName,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    16,
		FilterSubject: "tasks.>",
		AckWait:       60 * time.Second,
	})
	if err != nil {
		t.Fatalf("create ephemeral consumer: %v", err)
	}
	if got := ephemeral.CachedInfo().Config.InactiveThreshold; got == 0 {
		t.Fatal("precondition: a consumer without Durable should get an inactivity threshold")
	}
	publishTask(t, queue, "in-flight")
	batch, err := ephemeral.Fetch(1, jetstream.FetchMaxWait(time.Second))
	if err != nil {
		t.Fatalf("fetch from ephemeral consumer: %v", err)
	}
	for range batch.Messages() {
		// Delivered and left unacknowledged, as when an ingestor dies mid-task.
	}

	if err := queue.InitConsumer(common.TaskSubject); err != nil {
		t.Fatalf("InitConsumer over an ephemeral consumer: %v", err)
	}
	assertDurableTaskConsumer(t, queue)

	ctx, cancel := taskPullContext(t)
	defer cancel()
	handle, err := queue.PullMessage(ctx)
	if err != nil {
		t.Fatalf("PullMessage: %v", err)
	}
	if handle == nil || handle.GetMessage().TaskID != "in-flight" {
		t.Fatalf("handle = %v, want the unacknowledged in-flight task", handle)
	}
}

func TestNatsMessageHandle_NumDelivered(t *testing.T) {
	queue := newTaskQueue(t)
	publishTask(t, queue, "counted")
	ctx, cancel := taskPullContext(t)
	defer cancel()
	handle, err := queue.PullMessage(ctx)
	if err != nil || handle == nil {
		t.Fatalf("PullMessage = %v, %v", handle, err)
	}
	if got := handle.NumDelivered(); got != 1 {
		t.Fatalf("NumDelivered = %d on the first delivery, want 1", got)
	}
}

func TestTaskConsumerMaxDeliverMatchesIngestorLimit(t *testing.T) {
	consumer, err := newTaskQueue(t).stream.Consumer(t.Context(), taskConsumerName)
	if err != nil {
		t.Fatalf("task consumer: %v", err)
	}
	if got := consumer.CachedInfo().Config.MaxDeliver; got != common.TaskMaxDeliver {
		t.Fatalf("MaxDeliver = %d, want common.TaskMaxDeliver = %d", got, common.TaskMaxDeliver)
	}
}
