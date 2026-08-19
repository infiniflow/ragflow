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
	"encoding/json"
	"fmt"
	"ragflow/internal/common"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

type StatusMessage struct {
	ID        int       `json:"id"`
	Version   string    `json:"version"`
	Timestamp time.Time `json:"timestamp"`
	NodeName  string    `json:"node_name"`
	ExtInfo   string    `json:"ext_info"`
}

func NewStatusMessage(id int, version string, nodeName string, extInfo string) *StatusMessage {
	return &StatusMessage{
		ID:        id,
		Version:   version,
		NodeName:  nodeName,
		ExtInfo:   extInfo,
	}
}

func StatusMessageSending() {
	// Construct status message
	statusMessage := NewStatusMessage(0, "v1", "ragflow", "")

	// Serialize to JSON
	jsonData, err := json.Marshal(statusMessage)
	if err != nil {
		common.Error("Failed to marshal status message", err)
		return
	}

	// Create HTTP client
	client := NewHTTPClientBuilder().
		WithHost("127.0.0.1").
		WithPort(9381).
		WithSSL(false).
		WithTimeout(10 * time.Second).
		Build()

	// Send POST request
	resp, err := client.PostJSON("/v1/admin/status", jsonData)
	if err != nil {
		common.Error("Error sending status message", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		common.Error("Failed to send status message", fmt.Errorf("status: %d", resp.StatusCode))
	}
}

// ScheduledTask represents a periodic task.
//
// Lifecycle contract (#18468): Start may be called again after Stop (each
// successful Start runs a fresh worker on a fresh stop channel); Stop is
// idempotent and safe under concurrent calls (the close happens exactly
// once); Stop waits for the worker to observe the stop signal, so a job
// already in flight may still finish but no new tick fires after Stop
// returns.
type ScheduledTask struct {
	Name     string
	Interval time.Duration
	Job      func()

	mu        sync.Mutex
	running   bool
	stop      chan struct{}
	done      chan struct{}
	executing int32 // atomic flag: 0 - not executed, 1 running
}

// NewScheduledTask creates a new simple task
func NewScheduledTask(name string, interval time.Duration, job func()) *ScheduledTask {
	return &ScheduledTask{
		Name:     name,
		Interval: interval,
		Job:      job,
	}
}

// Start begins the periodic task. A task that was stopped can be started
// again; each run gets its own stop channel, so a previously-closed channel
// never leaks into the new worker (#18468).
func (t *ScheduledTask) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running {
		return
	}
	t.running = true
	t.stop = make(chan struct{})
	t.done = make(chan struct{})
	stop := t.stop
	done := t.done

	go func() {
		defer close(done)
		ticker := time.NewTicker(t.Interval)
		defer ticker.Stop()

		common.Info("Task started", zap.String("name", t.Name))

		for {
			select {
			case <-ticker.C:
				t.runSafely()
			case <-stop:
				common.Info("Task stopped", zap.String("name", t.Name))
				return
			}
		}
	}()
}

// runSafely executes the job with panic recovery and prevents overlap
func (t *ScheduledTask) runSafely() {
	// Attempt to set the flag
	if !atomic.CompareAndSwapInt32(&t.executing, 0, 1) {
		common.Warn("Task skipped - previous execution still running", zap.String("name", t.Name))
		return
	}

	// Clear atomic flag after execution
	defer atomic.StoreInt32(&t.executing, 0)

	defer func() {
		if r := recover(); r != nil {
			common.Fatal("Task panicked", zap.String("name", t.Name), zap.Any("recover", r))
		}
	}()

	t.Job()
}

// Stop stops the periodic task. Idempotent and concurrency-safe: the close
// runs exactly once no matter how many goroutines race here, and the method
// waits for the worker to observe the signal before returning (#18468). A
// job already executing may still finish; no new tick fires afterwards.
func (t *ScheduledTask) Stop() {
	// Hold the lifecycle lock until the worker exits: flipping running and
	// releasing before waiting let a concurrent Start launch a new worker
	// while the old one was still draining, and let a Stop return early
	// (#18471 review). The worker never takes this lock (Start hands it the
	// channels under the lock), so waiting here cannot deadlock.
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.running {
		return
	}
	close(t.stop)
	<-t.done
	t.running = false
}

// IsRunning reports whether a worker is currently active.
func (t *ScheduledTask) IsRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

// IsExecuting returns whether the task is currently executing
func (t *ScheduledTask) IsExecuting() bool {
	return atomic.LoadInt32(&t.executing) == 1
}
