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
	"sync"
	"testing"
	"time"
)

func TestWaitForDeepResearchWorkersReturnsCanceledAfterWorkersComplete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var workers sync.WaitGroup

	if err := waitForDeepResearchWorkers(ctx, &workers); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestWaitForDeepResearchWorkersWhenCanceledWaitsForActiveWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	releaseWorker := make(chan struct{})
	workers.Add(1)
	go func() {
		defer workers.Done()
		<-releaseWorker
	}()

	result := make(chan error, 1)
	cancel()
	go func() {
		result <- waitForDeepResearchWorkers(ctx, &workers)
	}()

	select {
	case err := <-result:
		close(releaseWorker)
		t.Fatalf("returned before worker completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseWorker)

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitForDeepResearchWorkers did not return after worker completed")
	}
}
