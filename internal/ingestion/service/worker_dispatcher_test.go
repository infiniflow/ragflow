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

type blockingTaskHandleStream struct {
	messages chan common.TaskHandle
	done     chan struct{}

	once sync.Once
	mu   sync.RWMutex
	err  error
}

func newBlockingTaskHandleStream() *blockingTaskHandleStream {
	return &blockingTaskHandleStream{
		messages: make(chan common.TaskHandle),
		done:     make(chan struct{}),
	}
}

func (s *blockingTaskHandleStream) Messages() <-chan common.TaskHandle { return s.messages }

func (s *blockingTaskHandleStream) Done() <-chan struct{} { return s.done }

func (s *blockingTaskHandleStream) Err() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

func (s *blockingTaskHandleStream) close(err error) {
	s.once.Do(func() {
		s.mu.Lock()
		s.err = err
		s.mu.Unlock()
		close(s.messages)
		close(s.done)
	})
}

type dispatcherTestQueue struct {
	engine.MessageQueue

	mu      sync.Mutex
	streams []*blockingTaskHandleStream
	next    int
	calls   chan int
}

func (q *dispatcherTestQueue) PullTaskStream(ctx context.Context, max int) (common.TaskHandleStream, error) {
	q.mu.Lock()
	stream := q.streams[q.next]
	q.next++
	q.mu.Unlock()

	q.calls <- max
	go func() {
		<-ctx.Done()
		stream.close(ctx.Err())
	}()
	return stream, nil
}

// TestWorkerDispatcherStartsNewPullWhileEarlierPullWaits prevents a pending
// Pull(1) from delaying a newly-idle worker until its one-second expiry. A
// synchronous Fetch loop would only record the first call before the timeout.
func TestWorkerDispatcherStartsNewPullWhileEarlierPullWaits(t *testing.T) {
	queue := &dispatcherTestQueue{
		streams: []*blockingTaskHandleStream{
			newBlockingTaskHandleStream(),
			newBlockingTaskHandleStream(),
		},
		calls: make(chan int, 2),
	}
	previousQueue := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(queue)
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousQueue) })

	ingestor := newUnitIngestor("test-independent-pulls", 2, nil)
	ingestor.dispatcherWg.Add(1)
	go ingestor.consumeLoop()
	t.Cleanup(func() {
		ingestor.dispatchCancel()
		ingestor.dispatcherWg.Wait()
		ingestor.pullWg.Wait()
	})

	ingestor.workerQueue <- &worker{id: 1, inbox: make(chan common.TaskHandle)}
	select {
	case max := <-queue.calls:
		if max != 1 {
			t.Fatalf("first Pull max = %d, want 1", max)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("first idle worker did not start Pull(1)")
	}

	ingestor.workerQueue <- &worker{id: 2, inbox: make(chan common.TaskHandle)}
	select {
	case max := <-queue.calls:
		if max != 1 {
			t.Fatalf("second Pull max = %d, want 1", max)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("new idle worker waited for the earlier Pull to expire")
	}
}

// TestWorkerDispatcherBatchesAvailableWorkersInSameTurn prevents the dispatcher
// from turning workers that are already available into redundant Pull(1)
// requests. It must drain only the workers visible in this turn and issue one
// Pull(K).
func TestWorkerDispatcherBatchesAvailableWorkersInSameTurn(t *testing.T) {
	queue := &dispatcherTestQueue{
		streams: []*blockingTaskHandleStream{newBlockingTaskHandleStream()},
		calls:   make(chan int, 1),
	}
	previousQueue := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(queue)
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousQueue) })

	ingestor := newUnitIngestor("test-batch-visible-workers", 2, nil)
	ingestor.workerQueue <- &worker{id: 1, inbox: make(chan common.TaskHandle)}
	ingestor.workerQueue <- &worker{id: 2, inbox: make(chan common.TaskHandle)}
	ingestor.dispatcherWg.Add(1)
	go ingestor.consumeLoop()
	t.Cleanup(func() {
		ingestor.dispatchCancel()
		ingestor.dispatcherWg.Wait()
		ingestor.pullWg.Wait()
	})

	select {
	case max := <-queue.calls:
		if max != 2 {
			t.Fatalf("Pull max = %d, want 2 for the two visible workers", max)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("visible workers did not start a Pull")
	}
}

// TestWorkerDispatcherHandsOffFirstStreamMessageImmediately prevents a
// slice-collecting Pull implementation from delaying the first task until the
// requested batch fills or expires.
func TestWorkerDispatcherHandsOffFirstStreamMessageImmediately(t *testing.T) {
	stream := newBlockingTaskHandleStream()
	queue := &dispatcherTestQueue{
		streams: []*blockingTaskHandleStream{stream},
		calls:   make(chan int, 1),
	}
	previousQueue := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(queue)
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousQueue) })

	ingestor := newUnitIngestor("test-first-stream-handoff", 2, nil)
	firstWorker := &worker{id: 1, inbox: make(chan common.TaskHandle)}
	secondWorker := &worker{id: 2, inbox: make(chan common.TaskHandle)}
	ingestor.workerQueue <- firstWorker
	ingestor.workerQueue <- secondWorker
	ingestor.dispatcherWg.Add(1)
	go ingestor.consumeLoop()
	t.Cleanup(func() {
		ingestor.dispatchCancel()
		ingestor.dispatcherWg.Wait()
		ingestor.pullWg.Wait()
	})

	select {
	case max := <-queue.calls:
		if max != 2 {
			t.Fatalf("Pull max = %d, want 2", max)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("workers did not start Pull(2)")
	}

	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "first-stream-message"}}
	go func() { stream.messages <- handle }()
	select {
	case received := <-firstWorker.inbox:
		if received != handle {
			t.Fatalf("handed-off handle = %v, want first stream handle", received)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("first stream message waited for the Pull batch to fill")
	}
}

// TestPullBatchReturnsOnlyUnmatchedWorkers prevents a partial Pull(K) from
// losing an unused worker or registering a worker whose handle was already handed
// off to a worker.
func TestPullBatchReturnsOnlyUnmatchedWorkers(t *testing.T) {
	stream := newBlockingTaskHandleStream()
	queue := &dispatcherTestQueue{
		streams: []*blockingTaskHandleStream{stream},
		calls:   make(chan int, 1),
	}
	ingestor := newUnitIngestor("test-partial-pull", 2, nil)
	firstWorker := &worker{id: 1, inbox: make(chan common.TaskHandle)}
	secondWorker := &worker{id: 2, inbox: make(chan common.TaskHandle)}

	ingestor.pullWg.Add(1)
	go ingestor.consumePullBatch(queue, []*worker{firstWorker, secondWorker})
	t.Cleanup(func() {
		ingestor.dispatchCancel()
		ingestor.pullWg.Wait()
	})

	select {
	case max := <-queue.calls:
		if max != 2 {
			t.Fatalf("Pull max = %d, want 2", max)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("Pull batch did not start")
	}

	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "partial-pull"}}
	go func() { stream.messages <- handle }()
	select {
	case received := <-firstWorker.inbox:
		if received != handle {
			t.Fatalf("handed-off handle = %v, want partial-pull handle", received)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("partial Pull did not hand off its first handle")
	}
	stream.close(nil)
	ingestor.pullWg.Wait()

	select {
	case returned := <-ingestor.workerQueue:
		if returned != secondWorker {
			t.Fatalf("returned worker = %d, want unmatched worker %d", returned.id, secondWorker.id)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("partial Pull did not return its unmatched worker")
	}
	select {
	case duplicate := <-ingestor.workerQueue:
		t.Fatalf("unexpected duplicate worker registration: %d", duplicate.id)
	default:
	}
}

// TestPullBatchCancellationLeavesReservedHandleUnsettled prevents shutdown
// from blocking forever when a Pull has received a handle but its worker has
// not yet taken the private inbox. The handle belongs to the broker again; it
// must not be locally Acked, Nacked, or handed off after cancellation.
func TestPullBatchCancellationLeavesReservedHandleUnsettled(t *testing.T) {
	stream := newBlockingTaskHandleStream()
	queue := &dispatcherTestQueue{
		streams: []*blockingTaskHandleStream{stream},
		calls:   make(chan int, 1),
	}
	ingestor := newUnitIngestor("test-cancel-reserved-handle", 1, nil)
	w := &worker{id: 1, inbox: make(chan common.TaskHandle)}

	ingestor.pullWg.Add(1)
	go ingestor.consumePullBatch(queue, []*worker{w})
	t.Cleanup(func() {
		ingestor.dispatchCancel()
		ingestor.pullWg.Wait()
	})

	select {
	case <-queue.calls:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("Pull batch did not start")
	}

	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "reserved-on-stop"}}
	sent := make(chan struct{})
	go func() {
		stream.messages <- handle
		close(sent)
	}()
	select {
	case <-sent:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("Pull batch did not reserve the streamed handle")
	}

	ingestor.dispatchCancel()
	waitDone := make(chan struct{})
	go func() {
		ingestor.pullWg.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("cancelled reserved hand-off blocked Pull shutdown")
	}
	if handle.acks.Load() != 0 || handle.nacks.Load() != 0 {
		t.Fatalf("reserved handle settlement = %d Ack / %d Nack, want none", handle.acks.Load(), handle.nacks.Load())
	}
	select {
	case received := <-w.inbox:
		t.Fatalf("reserved handle was handed off after cancellation: %v", received)
	default:
	}
}
