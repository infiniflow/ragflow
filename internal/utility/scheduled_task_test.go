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

package utility

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduledTaskConcurrentStart(t *testing.T) {
	task := NewScheduledTask("concurrent start", time.Hour, func() {})
	t.Cleanup(task.Stop)

	const callers = 64
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			<-start
			task.Start()
		}()
	}

	close(start)
	wg.Wait()

	task.mu.Lock()
	running := task.running
	stop := task.stop
	task.mu.Unlock()
	if !running || stop == nil {
		t.Fatal("concurrent Start calls did not leave the task running")
	}
}

func TestScheduledTaskConcurrentStopIsIdempotent(t *testing.T) {
	task := NewScheduledTask("concurrent stop", time.Hour, func() {})
	task.Start()

	const callers = 64
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			<-start
			task.Stop()
		}()
	}

	close(start)
	wg.Wait()
	task.Stop()

	task.mu.Lock()
	running := task.running
	stop := task.stop
	task.mu.Unlock()
	if running || stop != nil {
		t.Fatal("concurrent Stop calls did not leave the task stopped")
	}
}

func TestScheduledTaskCanRestart(t *testing.T) {
	var runs atomic.Int32
	task := NewScheduledTask("restart", time.Millisecond, func() {
		runs.Add(1)
	})
	t.Cleanup(task.Stop)

	task.Start()
	firstStop := scheduledTaskStopChannel(task)
	waitForScheduledTaskRuns(t, &runs, 1)
	task.Stop()

	task.Start()
	secondStop := scheduledTaskStopChannel(task)
	if firstStop == secondStop {
		t.Fatal("Start reused the closed stop channel")
	}
	select {
	case <-secondStop:
		t.Fatal("restart created an already-closed stop channel")
	default:
	}

	previousRuns := runs.Load()
	waitForScheduledTaskRuns(t, &runs, previousRuns+1)
}

func scheduledTaskStopChannel(task *ScheduledTask) chan struct{} {
	task.mu.Lock()
	defer task.mu.Unlock()
	return task.stop
}

func waitForScheduledTaskRuns(t *testing.T, runs *atomic.Int32, want int32) {
	t.Helper()

	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	for {
		if runs.Load() >= want {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("scheduled task ran %d times; want at least %d", runs.Load(), want)
		case <-ticker.C:
		}
	}
}
