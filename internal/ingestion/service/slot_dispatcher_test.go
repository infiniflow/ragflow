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

type slotTestQueue struct {
	engine.MessageQueue

	mu      sync.Mutex
	streams []*blockingTaskHandleStream
	next    int
	calls   chan int
}

func (q *slotTestQueue) PullTaskStream(ctx context.Context, max int) (common.TaskHandleStream, error) {
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

// TestSlotDispatcherStartsNewPullWhileEarlierPullWaits prevents a pending
// Pull(1) from delaying a newly-idle worker until its one-second expiry. A
// synchronous Fetch loop would only record the first call before the timeout.
func TestSlotDispatcherStartsNewPullWhileEarlierPullWaits(t *testing.T) {
	queue := &slotTestQueue{
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

	ingestor.idleSlots <- &workerSlot{id: 1, inbox: make(chan common.TaskHandle)}
	select {
	case max := <-queue.calls:
		if max != 1 {
			t.Fatalf("first Pull max = %d, want 1", max)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("first idle slot did not start Pull(1)")
	}

	ingestor.idleSlots <- &workerSlot{id: 2, inbox: make(chan common.TaskHandle)}
	select {
	case max := <-queue.calls:
		if max != 1 {
			t.Fatalf("second Pull max = %d, want 1", max)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("new idle slot waited for the earlier Pull to expire")
	}
}

// TestSlotDispatcherBatchesSlotsAlreadyIdleInSameTurn prevents the dispatcher
// from turning slots that are already available into redundant Pull(1)
// requests. It must drain only the slots visible in this turn and issue one
// Pull(K).
func TestSlotDispatcherBatchesSlotsAlreadyIdleInSameTurn(t *testing.T) {
	queue := &slotTestQueue{
		streams: []*blockingTaskHandleStream{newBlockingTaskHandleStream()},
		calls:   make(chan int, 1),
	}
	previousQueue := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(queue)
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousQueue) })

	ingestor := newUnitIngestor("test-batch-visible-slots", 2, nil)
	ingestor.idleSlots <- &workerSlot{id: 1, inbox: make(chan common.TaskHandle)}
	ingestor.idleSlots <- &workerSlot{id: 2, inbox: make(chan common.TaskHandle)}
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
			t.Fatalf("Pull max = %d, want 2 for the two visible slots", max)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("visible idle slots did not start a Pull")
	}
}

// TestSlotDispatcherHandsOffFirstStreamMessageImmediately prevents a
// slice-collecting Pull implementation from delaying the first task until the
// requested batch fills or expires.
func TestSlotDispatcherHandsOffFirstStreamMessageImmediately(t *testing.T) {
	stream := newBlockingTaskHandleStream()
	queue := &slotTestQueue{
		streams: []*blockingTaskHandleStream{stream},
		calls:   make(chan int, 1),
	}
	previousQueue := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(queue)
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousQueue) })

	ingestor := newUnitIngestor("test-first-stream-handoff", 2, nil)
	firstSlot := &workerSlot{id: 1, inbox: make(chan common.TaskHandle)}
	secondSlot := &workerSlot{id: 2, inbox: make(chan common.TaskHandle)}
	ingestor.idleSlots <- firstSlot
	ingestor.idleSlots <- secondSlot
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
		t.Fatal("slots did not start Pull(2)")
	}

	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "first-stream-message"}}
	go func() { stream.messages <- handle }()
	select {
	case received := <-firstSlot.inbox:
		if received != handle {
			t.Fatalf("handed-off handle = %v, want first stream handle", received)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("first stream message waited for the Pull batch to fill")
	}
}

// TestPullBatchReturnsOnlyUnmatchedSlots prevents a partial Pull(K) from
// losing an unused slot or registering a slot whose handle was already handed
// off to a worker.
func TestPullBatchReturnsOnlyUnmatchedSlots(t *testing.T) {
	stream := newBlockingTaskHandleStream()
	queue := &slotTestQueue{
		streams: []*blockingTaskHandleStream{stream},
		calls:   make(chan int, 1),
	}
	ingestor := newUnitIngestor("test-partial-pull", 2, nil)
	firstSlot := &workerSlot{id: 1, inbox: make(chan common.TaskHandle)}
	secondSlot := &workerSlot{id: 2, inbox: make(chan common.TaskHandle)}
	ingestor.markSlotReserved(firstSlot)
	ingestor.markSlotReserved(secondSlot)

	ingestor.pullWg.Add(1)
	go ingestor.consumePullBatch(queue, []*workerSlot{firstSlot, secondSlot})
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
	case received := <-firstSlot.inbox:
		if received != handle {
			t.Fatalf("handed-off handle = %v, want partial-pull handle", received)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("partial Pull did not hand off its first handle")
	}
	stream.close(nil)
	ingestor.pullWg.Wait()

	select {
	case returned := <-ingestor.idleSlots:
		if returned != secondSlot {
			t.Fatalf("returned slot = %d, want unmatched slot %d", returned.id, secondSlot.id)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("partial Pull did not return its unmatched slot")
	}
	select {
	case duplicate := <-ingestor.idleSlots:
		t.Fatalf("unexpected duplicate slot registration: %d", duplicate.id)
	default:
	}
}

// TestPullBatchCancellationLeavesReservedHandleUnsettled prevents shutdown
// from blocking forever when a Pull has received a handle but its worker has
// not yet taken the private inbox. The handle belongs to the broker again; it
// must not be locally Acked, Nacked, or handed off after cancellation.
func TestPullBatchCancellationLeavesReservedHandleUnsettled(t *testing.T) {
	stream := newBlockingTaskHandleStream()
	queue := &slotTestQueue{
		streams: []*blockingTaskHandleStream{stream},
		calls:   make(chan int, 1),
	}
	ingestor := newUnitIngestor("test-cancel-reserved-handle", 1, nil)
	slot := &workerSlot{id: 1, inbox: make(chan common.TaskHandle)}
	ingestor.markSlotReserved(slot)

	ingestor.pullWg.Add(1)
	go ingestor.consumePullBatch(queue, []*workerSlot{slot})
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
	case received := <-slot.inbox:
		t.Fatalf("reserved handle was handed off after cancellation: %v", received)
	default:
	}
	if SlotState(slot.state.Load()) != SlotStateIdle {
		t.Fatalf("cancelled slot state = %v, want Idle", SlotState(slot.state.Load()))
	}
}
