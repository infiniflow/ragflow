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

package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"
)

type sharedTaskClaimStore struct {
	mu         sync.Mutex
	owners     map[string]string
	renewErr   error
	releaseErr error
	renewed    chan struct{}
	renewOnce  sync.Once
}

func newSharedTaskClaimStore() *sharedTaskClaimStore {
	return &sharedTaskClaimStore{owners: make(map[string]string), renewed: make(chan struct{})}
}

func (*sharedTaskClaimStore) Available() bool { return true }

func (s *sharedTaskClaimStore) Acquire(_ context.Context, taskID, owner string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.owners[taskID]; exists {
		return false, nil
	}
	s.owners[taskID] = owner
	return true, nil
}

func (s *sharedTaskClaimStore) Renew(_ context.Context, taskID, owner string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.renewOnce.Do(func() { close(s.renewed) })
	if s.renewErr != nil {
		return false, s.renewErr
	}
	return s.owners[taskID] == owner, nil
}

func (s *sharedTaskClaimStore) Release(_ context.Context, taskID, owner string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.releaseErr != nil {
		return false, s.releaseErr
	}
	if s.owners[taskID] != owner {
		return false, nil
	}
	delete(s.owners, taskID)
	return true, nil
}

func TestTaskClaimCoordinatesIndependentIngestors(t *testing.T) {
	store := newSharedTaskClaimStore()
	first := NewIngestor("first", 1, []string{"pdf"})
	second := NewIngestor("second", 1, []string{"pdf"})
	first.taskClaims = store
	second.taskClaims = store
	first.taskClaimRefreshInterval = time.Hour
	second.taskClaimRefreshInterval = time.Hour

	if !claimTaskForTest(t, first, "task-1") {
		t.Fatal("first ingestor should acquire the distributed claim")
	}
	if claimTaskForTest(t, second, "task-1") {
		t.Fatal("second ingestor must not acquire a claim held by the first")
	}

	first.releaseTask("task-1")
	if !claimTaskForTest(t, second, "task-1") {
		t.Fatal("second ingestor should acquire after the first releases")
	}
	// A delayed duplicate release from the former owner must not delete the
	// successor's claim.
	first.releaseTask("task-1")
	if claimTaskForTest(t, first, "task-1") {
		t.Fatal("former owner must not clear the successor's claim")
	}
	second.releaseTask("task-1")
}

func TestTaskClaimHeartbeatStartsBeforeExecution(t *testing.T) {
	store := newSharedTaskClaimStore()
	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.taskClaims = store
	ingestor.taskClaimRefreshInterval = time.Millisecond
	claimCtx, claimed, err := ingestor.claimTask(t.Context(), "task-1")
	if err != nil || !claimed {
		t.Fatalf("claimTask = %v, %v; want true, nil", claimed, err)
	}
	defer ingestor.releaseTask("task-1")
	select {
	case <-store.renewed:
	case <-time.After(time.Second):
		t.Fatal("claim was not renewed while waiting for execution")
	}
	if err := claimCtx.Err(); err != nil {
		t.Fatalf("claim context cancelled after successful renewal: %v", err)
	}
}

func TestTaskClaimDerivesRefreshIntervalFromTTL(t *testing.T) {
	store := newSharedTaskClaimStore()
	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.taskClaims = store
	ingestor.taskClaimTTL = 30 * time.Millisecond
	ingestor.taskClaimRefreshInterval = 0
	claimCtx, claimed, err := ingestor.claimTask(t.Context(), "task-1")
	if err != nil || !claimed {
		t.Fatalf("claimTask = %v, %v; want true, nil", claimed, err)
	}
	defer ingestor.releaseTask("task-1")
	select {
	case <-store.renewed:
	case <-time.After(time.Second):
		t.Fatal("claim did not derive a renewal interval from its TTL")
	}
	if err := claimCtx.Err(); err != nil {
		t.Fatalf("claim context cancelled after derived-interval renewal: %v", err)
	}
}

func TestMessageHeartbeatStartsWhileClaimIsQueued(t *testing.T) {
	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.taskClaims = nil
	ingestor.heartbeatInterval = time.Millisecond
	claimCtx, claimed, err := ingestor.claimTask(t.Context(), "task-1")
	if err != nil || !claimed {
		t.Fatalf("claimTask = %v, %v; want true, nil", claimed, err)
	}
	handle := &fakeTaskHandle{}
	taskCtx := taskpkg.NewTaskContextForScheduling(claimCtx, &entity.IngestionTask{ID: "task-1"})
	taskCtx.Handle = handle
	ingestor.ensureTaskMessageHeartbeat(taskCtx)
	defer ingestor.releaseTask("task-1")

	deadline := time.After(time.Second)
	for handle.inProgress.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("message heartbeat did not start before worker execution")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestMessageHeartbeatDoesNotReadReplacedTaskContext(t *testing.T) {
	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.heartbeatInterval = time.Microsecond
	heartbeatCtx, cancelHeartbeat := context.WithCancel(t.Context())
	handle := &fakeTaskHandle{}
	taskCtx := taskpkg.NewTaskContextForScheduling(heartbeatCtx, &entity.IngestionTask{ID: "task-1"})
	taskCtx.Handle = handle
	stop := ingestor.startHeartbeat(taskCtx)
	deadline := time.After(time.Second)
	for handle.inProgress.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("heartbeat did not start")
		default:
			time.Sleep(time.Microsecond)
		}
	}

	replacementCtx := context.Background()
	for range 10_000 {
		taskCtx.Ctx = replacementCtx
	}
	cancelHeartbeat()
	stop()
}

func TestConcurrentLocalTaskClaimsHaveSingleOwner(t *testing.T) {
	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.taskClaims = nil
	const contenders = 32
	start := make(chan struct{})
	results := make(chan bool, contenders)
	var wg sync.WaitGroup
	for range contenders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, claimed, err := ingestor.claimTask(t.Context(), "task-1")
			if err != nil {
				t.Errorf("claimTask: %v", err)
			}
			results <- claimed
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	winners := 0
	for claimed := range results {
		if claimed {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("successful claims = %d, want 1", winners)
	}
	ingestor.releaseTask("task-1")
}

func TestConcurrentDistributedTaskClaimsHaveSingleOwner(t *testing.T) {
	store := newSharedTaskClaimStore()
	const contenders = 32
	ingestors := make([]*Ingestor, contenders)
	start := make(chan struct{})
	results := make(chan int, contenders)
	var wg sync.WaitGroup
	for i := range contenders {
		ingestor := NewIngestor(fmt.Sprintf("worker-%d", i), 1, []string{"pdf"})
		ingestor.taskClaims = store
		ingestor.taskClaimRefreshInterval = time.Hour
		ingestors[i] = ingestor
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			_, claimed, err := ingestors[index].claimTask(t.Context(), "task-1")
			if err != nil {
				t.Errorf("claimTask(%d): %v", index, err)
				return
			}
			if claimed {
				results <- index
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	winners := make([]int, 0, 1)
	for index := range results {
		winners = append(winners, index)
	}
	if len(winners) != 1 {
		t.Fatalf("successful distributed claims = %d, want 1", len(winners))
	}
	ingestors[winners[0]].releaseTask("task-1")
}

func TestTaskClaimHeartbeatCancelsWithLeaseLostCause(t *testing.T) {
	store := newSharedTaskClaimStore()
	store.renewErr = errors.New("redis unavailable")
	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.taskClaims = store
	ingestor.taskClaimRefreshInterval = time.Millisecond
	claimCtx, claimed, err := ingestor.claimTask(t.Context(), "task-1")
	if err != nil || !claimed {
		t.Fatalf("claimTask = %v, %v; want true, nil", claimed, err)
	}
	defer ingestor.releaseTask("task-1")
	select {
	case <-claimCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("lease renewal failure did not cancel the task")
	}
	if !errors.Is(context.Cause(claimCtx), errTaskClaimLost) {
		t.Fatalf("cancel cause = %v, want errTaskClaimLost", context.Cause(claimCtx))
	}
}

func TestRunTaskLeaseLossLeavesTaskRunningForRedelivery(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(errTaskClaimLost)
	terminal := ingestor.runTask(ctx, &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING,
	})
	if terminal {
		t.Fatal("lease loss must remain non-terminal so the broker redelivers")
	}

	task, err := ingestor.ingestionTaskSvc.GetTask(t.Context(), taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Status != common.RUNNING {
		t.Fatalf("task status = %s, want RUNNING", task.Status)
	}
}
