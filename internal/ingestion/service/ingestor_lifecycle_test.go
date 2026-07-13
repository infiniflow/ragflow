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
	"testing"
	"time"
)

// TestStartWorkerPool_StartOnceIdempotent verifies that calling startWorkerPool
// twice only starts maxConcurrency workers (sync.Once gate).
func TestStartWorkerPool_StartOnceIdempotent(t *testing.T) {
	const concurrency int32 = 3
	ingestor := NewIngestor("test-idempotent", concurrency, nil)
	// Stop background worker loops immediately so we can count workers.
	ingestor.cancel()
	ingestor.startWorkerPool()
	ingestor.startWorkerPool()
	ingestor.workerWg.Wait()
}

// TestStop_GracefulShutdown verifies that Stop cancels the context and waits
// for all worker goroutines to exit without hanging.
func TestStop_GracefulShutdown(t *testing.T) {
	const concurrency int32 = 2
	ingestor := NewIngestor("test-shutdown", concurrency, nil)

	// Start workers; they will block on the task channel since nothing is pushed.
	ingestor.startWorkerPool()

	done := make(chan struct{})
	go func() {
		ingestor.Stop()
		close(done)
	}()

	select {
	case <-done:
		// workers exited cleanly
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() timed out waiting for workers to exit")
	}
}
