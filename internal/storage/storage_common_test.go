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

package storage

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// --- retryObjectOp ---

func TestRetryObjectOp_SucceedsFirstAttempt(t *testing.T) {
	var calls, retries int
	err := retryObjectOp(context.Background(), 3, func() { retries++ }, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("op calls = %d, want 1", calls)
	}
	if retries != 0 {
		t.Errorf("onRetry calls = %d, want 0", retries)
	}
}

func TestRetryObjectOp_SucceedsOnLastAttempt(t *testing.T) {
	// Off-by-one guard: with attempts=3 the third attempt must still run.
	var calls int
	err := retryObjectOp(context.Background(), 3, nil, func() error {
		calls++
		if calls < 3 {
			return fmt.Errorf("attempt %d failed", calls)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if calls != 3 {
		t.Errorf("op calls = %d, want 3", calls)
	}
}

func TestRetryObjectOp_ReturnsLastErrorAfterAllAttempts(t *testing.T) {
	var calls, retries int
	err := retryObjectOp(context.Background(), 3, func() { retries++ }, func() error {
		calls++
		return fmt.Errorf("boom-%d", calls)
	})
	if err == nil || err.Error() != "boom-3" {
		t.Fatalf("err = %v, want boom-3 (the LAST error, not the first)", err)
	}
	if calls != 3 {
		t.Errorf("op calls = %d, want 3", calls)
	}
	// Legacy loops reconnect after every failed attempt, including the last
	// one before giving up: the refreshed client benefits the NEXT request.
	// The helper preserves that (zero behavior change), so onRetry runs 3x.
	if retries != 3 {
		t.Errorf("onRetry calls = %d, want 3 (after every failed attempt)", retries)
	}
}

func TestRetryObjectOp_CanceledContextNeverRunsOp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls int
	err := retryObjectOp(ctx, 3, nil, func() error {
		calls++
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls != 0 {
		t.Errorf("op calls = %d, want 0 (canceled ctx must short-circuit before any attempt)", calls)
	}
}

func TestRetryObjectOp_CancelDuringBackoffSurfacesPromptly(t *testing.T) {
	// The op itself cancels the context while failing: the next loop's ctx
	// check must surface context.Canceled without sleeping the backoff window.
	ctx, cancel := context.WithCancel(context.Background())
	var calls int
	start := time.Now()
	err := retryObjectOp(ctx, 3, nil, func() error {
		calls++
		cancel()
		return fmt.Errorf("fail after cancel")
	})
	elapsed := time.Since(start)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Errorf("op calls = %d, want 1", calls)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("took %v; want < 500ms (no backoff sleep on a dead ctx)", elapsed)
	}
}

func TestRetryObjectOp_ContextCanceledAfterOpErrorSkipsSleep(t *testing.T) {
	// ctx is canceled between the op failure and the backoff sleep: the
	// post-error ctx check must win over sleepOrAbort.
	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()
	err := retryObjectOp(ctx, 3, nil, func() error {
		cancel() // cancel as a side effect of the failing op
		return fmt.Errorf("transport error")
	})
	elapsed := time.Since(start)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("took %v; want < 500ms", elapsed)
	}
}

func TestRetryObjectOp_SingleAttemptNoRetry(t *testing.T) {
	var calls, retries int
	err := retryObjectOp(context.Background(), 1, func() { retries++ }, func() error {
		calls++
		return fmt.Errorf("once")
	})
	if err == nil || err.Error() != "once" {
		t.Fatalf("err = %v, want once", err)
	}
	if calls != 1 || retries != 1 {
		t.Errorf("calls=%d retries=%d, want 1/1", calls, retries)
	}
}

func TestRetryObjectOp_ZeroAttemptsIsNoOp(t *testing.T) {
	// Boundary guard: matches the old `for i := 0; i < 0` loops, which never
	// ran and returned the nil lastErr.
	var calls int
	err := retryObjectOp(context.Background(), 0, nil, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if calls != 0 {
		t.Errorf("op calls = %d, want 0", calls)
	}
}

// --- moveWithRollback ---

func TestMoveWithRollback_CopyThenRemoveSucceeds(t *testing.T) {
	var copyCalls, removeSrc, removeDest int
	ok := moveWithRollback(context.Background(),
		"srcb", "srck", "dstb", "dstk",
		func() bool { copyCalls++; return true },
		func(context.Context) error { removeSrc++; return nil },
		func(context.Context) error { removeDest++; return nil },
	)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if copyCalls != 1 || removeSrc != 1 || removeDest != 0 {
		t.Errorf("copy=%d removeSrc=%d removeDest=%d, want 1/1/0", copyCalls, removeSrc, removeDest)
	}
}

func TestMoveWithRollback_CopyFailureLeavesNoSideEffects(t *testing.T) {
	var removeSrc, removeDest int
	ok := moveWithRollback(context.Background(),
		"srcb", "srck", "dstb", "dstk",
		func() bool { return false },
		func(context.Context) error { removeSrc++; return nil },
		func(context.Context) error { removeDest++; return nil },
	)
	if ok {
		t.Fatal("ok = true, want false")
	}
	if removeSrc != 0 || removeDest != 0 {
		t.Errorf("removeSrc=%d removeDest=%d, want 0/0 (no removes when copy fails)", removeSrc, removeDest)
	}
}

func TestMoveWithRollback_SourceRemoveFailureRollsBackDestination(t *testing.T) {
	var removeDest int
	var rollbackCtxErr error
	var hasDeadline bool
	ok := moveWithRollback(context.Background(),
		"srcb", "srck", "dstb", "dstk",
		func() bool { return true },
		func(context.Context) error { return fmt.Errorf("source delete failed") },
		func(ctx context.Context) error {
			removeDest++
			rollbackCtxErr = ctx.Err()
			_, hasDeadline = ctx.Deadline()
			return nil
		},
	)
	if ok {
		t.Fatal("ok = true, want false")
	}
	if removeDest != 1 {
		t.Fatalf("removeDest calls = %d, want 1 (rollback must run)", removeDest)
	}
	if rollbackCtxErr != nil {
		t.Errorf("rollback ctx err = %v, want nil", rollbackCtxErr)
	}
	if !hasDeadline {
		t.Error("rollback ctx has no deadline, want a 30s timeout")
	}
}

func TestMoveWithRollback_RollbackRunsEvenWhenParentCanceled(t *testing.T) {
	// The rollback path derives from context.WithoutCancel: a parent canceled
	// during the source removal must not skip the destination cleanup.
	ctx, cancel := context.WithCancel(context.Background())
	var removeDest int
	var rollbackCtxErr error
	ok := moveWithRollback(ctx,
		"srcb", "srck", "dstb", "dstk",
		func() bool { return true },
		func(context.Context) error {
			cancel() // parent dies while removing the source
			return fmt.Errorf("source delete failed")
		},
		func(rctx context.Context) error {
			removeDest++
			rollbackCtxErr = rctx.Err()
			return nil
		},
	)
	if ok {
		t.Fatal("ok = true, want false")
	}
	if removeDest != 1 {
		t.Fatalf("removeDest calls = %d, want 1 (rollback must survive parent cancellation)", removeDest)
	}
	if rollbackCtxErr != nil {
		t.Errorf("rollback ctx err = %v, want nil (WithoutCancel)", rollbackCtxErr)
	}
}

func TestMoveWithRollback_RollbackFailureStillReportsFailure(t *testing.T) {
	ok := moveWithRollback(context.Background(),
		"srcb", "srck", "dstb", "dstk",
		func() bool { return true },
		func(context.Context) error { return fmt.Errorf("src") },
		func(context.Context) error { return fmt.Errorf("dest") },
	)
	if ok {
		t.Fatal("ok = true, want false (a failed rollback must not report success)")
	}
}
