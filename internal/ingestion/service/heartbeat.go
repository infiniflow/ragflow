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
	"fmt"
	"sync"
	"time"

	"ragflow/internal/common"
)

// Heartbeat renews one broker message's AckWait deadline by calling
// Handle.InProgress periodically until Stop. It is task-kind agnostic: the log
// label comes from the explicit id passed at construction, never from
// TaskContext.IngestionTask (which is nil for memory tasks), so the same type
// serves both the ingestion and memory execution paths.
//
// Contract: Stop blocks until the renewal goroutine has exited, guaranteeing no
// InProgress call is in flight when it returns. Callers must Stop before
// Acking/Nacking the message so settlement never races an InProgress on the
// same message.
type Heartbeat struct {
	id       string
	handle   common.TaskHandle
	interval time.Duration
	ctx      context.Context

	mu      sync.Mutex
	started bool
	stopped bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewHeartbeat builds a Heartbeat for the given message handle. A nil handle or
// non-positive interval makes Start a no-op (standalone/test path).
func NewHeartbeat(id string, handle common.TaskHandle, interval time.Duration) *Heartbeat {
	return &Heartbeat{
		id:       id,
		handle:   handle,
		interval: interval,
		ctx:      context.Background(),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// WithContext binds an external cancellation signal (e.g. ingestor shutdown) so
// the renewal goroutine also exits when ctx is done. It must be called before
// Start.
func (h *Heartbeat) WithContext(ctx context.Context) *Heartbeat {
	if h != nil && ctx != nil {
		h.ctx = ctx
	}
	return h
}

// Start launches the renewal goroutine. It is idempotent: repeated calls and a
// call after Stop are no-ops. With a nil handle or non-positive interval it
// starts nothing, so the corresponding Stop returns immediately.
func (h *Heartbeat) Start() {
	if h == nil || h.handle == nil || h.interval <= 0 {
		return
	}
	h.mu.Lock()
	if h.started || h.stopped {
		h.mu.Unlock()
		return
	}
	h.started = true
	h.mu.Unlock()
	go h.loop()
}

// Stop signals the renewal goroutine to exit and BLOCKS until it has, so no
// InProgress is in flight when it returns. It is idempotent; calling Stop when
// no goroutine was started (or before Start) returns immediately and prevents a
// later Start from launching a renewal goroutine.
func (h *Heartbeat) Stop() {
	if h == nil {
		return
	}
	h.mu.Lock()
	started := h.started
	if !h.stopped {
		h.stopped = true
		if started {
			close(h.stopCh)
		}
	}
	h.mu.Unlock()
	if started {
		<-h.doneCh
	}
}

func (h *Heartbeat) loop() {
	defer close(h.doneCh)
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := h.handle.InProgress(); err != nil {
				common.Error(fmt.Sprintf("heartbeat task %s", h.id), err)
			}
		case <-h.stopCh:
			return
		case <-h.ctx.Done():
			return
		}
	}
}
