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

// #18468: concurrent Stop calls must close the channel exactly once — the
// old shape panicked with "close of closed channel" on the second caller.
func TestScheduledTaskConcurrentStop(t *testing.T) {
	for i := 0; i < 50; i++ {
		task := NewScheduledTask("concurrent-stop", 10*time.Millisecond, func() {})
		task.Start()

		var wg sync.WaitGroup
		for j := 0; j < 8; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				task.Stop() // must never panic
			}()
		}
		wg.Wait()

		if task.IsRunning() {
			t.Fatalf("iteration %d: task still running after concurrent stops", i)
		}
	}
}

// #18468: concurrent Start calls must launch exactly one worker. Counted via
// job executions racing a tiny interval — an extra worker shows up as more
// than one in-flight execution or duplicated ticks.
func TestScheduledTaskConcurrentStartSingleWorker(t *testing.T) {
	for i := 0; i < 50; i++ {
		task := NewScheduledTask("concurrent-start", 2*time.Millisecond, func() {
			time.Sleep(5 * time.Millisecond) // hold the executing flag so a second worker would skip-and-warn, not run
		})

		var wg sync.WaitGroup
		for j := 0; j < 8; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				task.Start()
			}()
		}
		wg.Wait()
		time.Sleep(20 * time.Millisecond)

		if !task.IsRunning() {
			t.Fatalf("iteration %d: task should be running before Stop", i)
		}
		task.Stop()
		// Stop waited for the worker: after Stop returns, a subsequent tick
		// must not fire.
		fired := int32(0)
		task.Job = func() { atomic.AddInt32(&fired, 1) }
		time.Sleep(20 * time.Millisecond)
		if got := atomic.LoadInt32(&fired); got != 0 {
			t.Fatalf("iteration %d: %d ticks fired after Stop returned", i, got)
		}
	}
}

// #18468: Start after Stop must run a fresh worker — the old shape reused
// the permanently-closed stop channel, so the "restarted" worker exited
// immediately while running stayed true.
func TestScheduledTaskRestartRunsFreshWorker(t *testing.T) {
	ran := make(chan struct{}, 4)
	task := NewScheduledTask("restart", 5*time.Millisecond, func() {
		ran <- struct{}{}
	})

	task.Start()
	<-ran
	task.Stop()

	if task.IsRunning() {
		t.Fatal("task should not be running after Stop")
	}

	task.Start()
	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("restarted task never ran — the old worker exited on the stale closed stop channel")
	}
	task.Stop()
}

// #18468: Stop waits for the worker. A slow job already in flight may
// finish, but Stop returning must mean no worker loop remains.
func TestScheduledTaskStopWaitsForWorker(t *testing.T) {
	inJob := make(chan struct{})
	release := make(chan struct{})
	task := NewScheduledTask("stop-waits", 1*time.Millisecond, func() {
		select {
		case inJob <- struct{}{}:
		default:
		}
		<-release
	})

	task.Start()
	<-inJob

	stopped := make(chan struct{})
	go func() {
		task.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		close(release)
		t.Fatal("Stop returned while the job was still executing and the worker had not observed the signal")
	case <-time.After(100 * time.Millisecond):
		// Stop is (correctly) waiting for the worker.
		close(release)
	}

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop never returned after the job released")
	}
}
