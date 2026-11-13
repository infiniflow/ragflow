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

package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine"
)

type dispatcherPullResult struct {
	handle   common.TaskHandle
	err      error
	ready    <-chan struct{}
	returned chan<- struct{}
}

type dispatcherTestQueue struct {
	engine.MessageQueue

	mu    sync.Mutex
	pulls []dispatcherPullResult
	next  int
	calls chan struct{}
}

func (q *dispatcherTestQueue) PullMessage(ctx context.Context) (common.TaskHandle, error) {
	q.mu.Lock()
	if q.next == len(q.pulls) {
		q.mu.Unlock()
		q.calls <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	pull := q.pulls[q.next]
	q.next++
	q.mu.Unlock()

	q.calls <- struct{}{}
	select {
	case <-pull.ready:
		if pull.returned != nil {
			close(pull.returned)
		}
		return pull.handle, pull.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func closedChannel() <-chan struct{} {
	ready := make(chan struct{})
	close(ready)
	return ready
}

// TestWorkerDispatcherLimitsPendingPullsToOne ensures an empty pull does not
// cause the dispatcher to create another outstanding broker request.
func TestWorkerDispatcherLimitsPendingPullsToOne(t *testing.T) {
	firstReady := make(chan struct{})
	queue := &dispatcherTestQueue{
		pulls: []dispatcherPullResult{{ready: firstReady}, {}},
		calls: make(chan struct{}, 2),
	}
	previousQueue := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(queue)
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousQueue) })

	ingestor := newUnitIngestor("test-single-pull", 2, nil)
	ingestor.dispatcherWg.Add(1)
	go ingestor.consumeLoop()
	t.Cleanup(func() {
		ingestor.dispatchCancel()
		ingestor.dispatcherWg.Wait()
	})

	ingestor.workerQueue <- &worker{id: 1, inbox: make(chan common.TaskHandle)}
	select {
	case <-queue.calls:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("first idle worker did not start PullMessage")
	}

	ingestor.workerQueue <- &worker{id: 2, inbox: make(chan common.TaskHandle)}
	select {
	case <-queue.calls:
		t.Fatal("dispatcher started a second PullMessage before the first completed")
	case <-time.After(100 * time.Millisecond):
	}

	close(firstReady)
	select {
	case <-queue.calls:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("dispatcher did not start the next pull after the empty result")
	}
}

func TestWorkerDispatcherHandsOffPulledMessageImmediately(t *testing.T) {
	ready := make(chan struct{})
	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "single-pull-message"}}
	queue := &dispatcherTestQueue{
		pulls: []dispatcherPullResult{{handle: handle, ready: ready}},
		calls: make(chan struct{}, 1),
	}
	previousQueue := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(queue)
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousQueue) })

	ingestor := newUnitIngestor("test-single-message-handoff", 1, nil)
	worker := &worker{id: 1, inbox: make(chan common.TaskHandle)}
	ingestor.workerQueue <- worker
	ingestor.dispatcherWg.Add(1)
	go ingestor.consumeLoop()
	t.Cleanup(func() {
		ingestor.dispatchCancel()
		ingestor.dispatcherWg.Wait()
	})

	select {
	case <-queue.calls:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("worker did not start PullMessages(1)")
	}

	close(ready)
	select {
	case received := <-worker.inbox:
		if received != handle {
			t.Fatalf("handed-off handle = %v, want pulled handle", received)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("pulled message was not handed off immediately")
	}
}

func TestWorkerDispatcherNacksReservedHandleOnShutdown(t *testing.T) {
	returned := make(chan struct{})
	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "reserved-on-stop"}}
	queue := &dispatcherTestQueue{
		pulls: []dispatcherPullResult{{handle: handle, ready: closedChannel(), returned: returned}},
		calls: make(chan struct{}, 1),
	}
	previousQueue := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(queue)
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousQueue) })
	ingestor := newUnitIngestor("test-nack-reserved-handle", 1, nil)
	worker := &worker{id: 1, inbox: make(chan common.TaskHandle)}
	t.Cleanup(func() {
		ingestor.dispatchCancel()
		ingestor.dispatcherWg.Wait()
	})
	ingestor.workerQueue <- worker
	ingestor.dispatcherWg.Add(1)
	go ingestor.consumeLoop()

	select {
	case <-queue.calls:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("pull did not start")
	}
	select {
	case <-returned:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("pull did not reserve the handle")
	}

	ingestor.dispatchCancel()
	waitDone := make(chan struct{})
	go func() {
		ingestor.dispatcherWg.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("cancelled hand-off blocked dispatcher shutdown")
	}
	if handle.acks.Load() != 0 || handle.nacks.Load() != 1 {
		t.Fatalf("reserved handle settlement = %d Ack / %d Nack, want 0 Ack / 1 Nack", handle.acks.Load(), handle.nacks.Load())
	}
}
