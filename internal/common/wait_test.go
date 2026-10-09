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

package common

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitForReadyImmediateSuccess(t *testing.T) {
	calls := 0
	err := WaitForReady(t.Context(), "dep", time.Second, func(ctx context.Context) error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("WaitForReady() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("WaitForReady() made %d calls, want 1", calls)
	}
}

func TestWaitForReadySucceedsAfterFailures(t *testing.T) {
	calls := 0
	err := WaitForReady(t.Context(), "dep", 5*time.Second, func(ctx context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("not yet")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WaitForReady() error = %v", err)
	}
	if calls != 3 {
		t.Fatalf("WaitForReady() made %d calls, want 3", calls)
	}
}

func TestWaitForReadyTimeout(t *testing.T) {
	start := time.Now()
	err := WaitForReady(t.Context(), "dep", 300*time.Millisecond, func(ctx context.Context) error {
		return errors.New("never")
	})
	if err == nil {
		t.Fatal("WaitForReady() error = nil, want timeout error")
	}
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond || elapsed > 2*time.Second {
		t.Fatalf("WaitForReady() returned after %s, want ~300ms", elapsed)
	}
}

func TestWaitForReadyContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	err := WaitForReady(ctx, "dep", 30*time.Second, func(ctx context.Context) error {
		return errors.New("never")
	})
	if err == nil {
		t.Fatal("WaitForReady() error = nil, want cancellation error")
	}
}
