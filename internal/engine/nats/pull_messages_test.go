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
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"

	"github.com/nats-io/nats.go/jetstream"
)

func newTaskQueue(t *testing.T) *NatsEngine {
	t.Helper()
	host, port := newEmbeddedNatsServer(t)
	queue := NewNatsEngine(host, port)
	if err := queue.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := queue.InitConsumer(common.DefaultTierSubject()); err != nil {
		t.Fatalf("InitConsumer: %v", err)
	}
	return queue
}

func taskPullContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), time.Second)
}

func publishTask(t *testing.T, queue *NatsEngine, taskID string) {
	t.Helper()
	payload, err := json.Marshal(common.TaskMessage{
		TaskID:   taskID,
		TaskType: common.TaskTypeIngestionTask,
	})
	if err != nil {
		t.Fatalf("marshal task: %v", err)
	}
	if err := queue.PublishTask(common.DefaultTierSubject(), payload); err != nil {
		t.Fatalf("publish task: %v", err)
	}
}

func TestPullMessageFetchesOneMessage(t *testing.T) {
	queue := newTaskQueue(t)
	publishTask(t, queue, "stream-one")
	publishTask(t, queue, "stream-two")

	ctx, cancel := taskPullContext(t)
	defer cancel()
	handle, err := queue.PullMessage(ctx)
	if err != nil {
		t.Fatalf("PullMessage: %v", err)
	}
	if handle == nil || handle.GetMessage().TaskID != "stream-one" {
		t.Fatalf("stream handle = %v, want stream-one", handle)
	}

	handles, err := queue.PullMessages(ctx, 1)
	if err != nil {
		t.Fatalf("PullMessages: %v", err)
	}
	if len(handles) != 1 || handles[0].GetMessage().TaskID != "stream-two" {
		t.Fatalf("remaining messages = %+v, want stream-two", handles)
	}
}

func TestPullMessageRequiresDeadline(t *testing.T) {
	queue := newTaskQueue(t)
	_, err := queue.PullMessage(t.Context())
	if err == nil {
		t.Fatal("PullMessage without a deadline succeeded")
	}
	if !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("PullMessage error = %v, want deadline error", err)
	}
}

func TestPullMessageReturnsNilForEmptyQueue(t *testing.T) {
	queue := newTaskQueue(t)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	handle, err := queue.PullMessage(ctx)
	if err != nil {
		t.Fatalf("PullMessage: %v", err)
	}
	if handle != nil {
		t.Fatalf("empty pull handle = %v, want nil", handle)
	}
}

func TestPullMessageReturnsCancellation(t *testing.T) {
	queue := newTaskQueue(t)
	ctx, cancel := taskPullContext(t)
	cancel()

	_, err := queue.PullMessage(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PullMessage error = %v, want cancellation", err)
	}
}

func TestPullMessageReportsMaxWaiting(t *testing.T) {
	queue := newTaskQueue(t)
	ctx, cancel := taskPullContext(t)
	defer cancel()
	// PullMessage drains lanes in priority order (tierOrder), trying the
	// highest-priority lane first. The background pull occupies the sole
	// MaxWaiting slot on that first lane, so a concurrent pull is rejected
	// there. Install the capacity-limited consumer on tierOrder[0], the lane
	// PullMessage reaches first, so the rejection is observable regardless of
	// how many lanes are configured.
	pt := tierOrder[0]
	primary := tierStreams[pt]
	primaryStream := queue.streams[pt]
	if err := primaryStream.DeleteConsumer(ctx, primary.consumerName); err != nil {
		t.Fatalf("delete default consumer: %v", err)
	}
	consumer, err := primaryStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          primary.consumerName,
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: primary.subjects[0],
		MaxWaiting:    1,
	})
	if err != nil {
		t.Fatalf("create limited consumer: %v", err)
	}
	queue.consumerMu.Lock()
	queue.consumers[pt] = consumer
	queue.consumerMu.Unlock()

	firstCtx, cancelFirst := taskPullContext(t)
	defer cancelFirst()
	firstResult := make(chan error, 1)
	go func() {
		_, err := queue.PullMessage(firstCtx)
		firstResult <- err
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		info, err := consumer.Info(t.Context())
		if err != nil {
			t.Fatalf("consumer info: %v", err)
		}
		if info.NumWaiting == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	info, err := consumer.Info(t.Context())
	if err != nil {
		t.Fatalf("consumer info: %v", err)
	}
	if info.NumWaiting != 1 {
		t.Fatalf("waiting pulls = %d, want 1", info.NumWaiting)
	}

	secondCtx, cancelSecond := taskPullContext(t)
	defer cancelSecond()
	_, err = queue.PullMessage(secondCtx)
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("MaxWaiting error = %v, want capacity rejection", err)
	}

	cancelFirst()
	select {
	case <-firstResult:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("first pending pull did not exit after cancellation")
	}
}

func TestPullMessagesFetchesMessages(t *testing.T) {
	queue := newTaskQueue(t)
	publishTask(t, queue, "admin-direct-pull-1")
	publishTask(t, queue, "admin-direct-pull-2")

	ctx, cancel := taskPullContext(t)
	defer cancel()
	handles, err := queue.PullMessages(ctx, 2)
	if err != nil {
		t.Fatalf("PullMessages: %v", err)
	}
	if len(handles) != 2 {
		t.Fatalf("len(handles) = %d, want 2", len(handles))
	}
	for index, taskID := range []string{"admin-direct-pull-1", "admin-direct-pull-2"} {
		if got := handles[index].GetMessage().TaskID; got != taskID {
			t.Fatalf("task id = %s, want %s", got, taskID)
		}
	}
}

func TestPullMessagesReturnsPartialBatchOnDeadline(t *testing.T) {
	queue := newTaskQueue(t)
	publishTask(t, queue, "partial-batch")

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	handles, err := queue.PullMessages(ctx, 2)
	if err != nil {
		t.Fatalf("PullMessages: %v", err)
	}
	if len(handles) != 1 || handles[0].GetMessage().TaskID != "partial-batch" {
		t.Fatalf("partial handles = %+v, want partial-batch", handles)
	}
}

func TestPullMessagesRejectsOutOfRangeMessageCount(t *testing.T) {
	queue := newTaskQueue(t)
	ctx, cancel := taskPullContext(t)
	defer cancel()

	for _, testCase := range []struct {
		name         string
		messageCount int
	}{
		{name: "zero", messageCount: 0},
		{name: "negative", messageCount: -1},
		{name: "above limit", messageCount: 101},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := queue.PullMessages(ctx, testCase.messageCount)
			if err == nil {
				t.Fatal("PullMessages succeeded for an out-of-range message count")
			}
		})
	}
}

func TestPullMessagesReportsBatchError(t *testing.T) {
	queue := newTaskQueue(t)
	ctx, cancel := taskPullContext(t)
	defer cancel()
	pt := tierOrder[0]
	primary := tierStreams[pt]
	primaryStream := queue.streams[pt]
	if err := primaryStream.DeleteConsumer(ctx, primary.consumerName); err != nil {
		t.Fatalf("delete default consumer: %v", err)
	}
	consumer, err := primaryStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          primary.consumerName,
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: primary.subjects[0],
		MaxWaiting:    1,
	})
	if err != nil {
		t.Fatalf("create limited consumer: %v", err)
	}
	queue.consumerMu.Lock()
	queue.consumers[pt] = consumer
	queue.consumerMu.Unlock()

	occupiedCtx, cancelOccupied := taskPullContext(t)
	defer cancelOccupied()
	occupied := make(chan error, 1)
	go func() {
		_, err := queue.PullMessage(occupiedCtx)
		occupied <- err
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		info, err := consumer.Info(t.Context())
		if err != nil {
			t.Fatalf("consumer info: %v", err)
		}
		if info.NumWaiting == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if _, err := queue.PullMessages(ctx, 1); err == nil {
		t.Fatal("PullMessages succeeded after the consumer rejected its pull")
	}
	cancelOccupied()
	select {
	case <-occupied:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("occupied pull did not exit after cancellation")
	}
}
