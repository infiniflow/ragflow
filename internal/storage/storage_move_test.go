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
	"fmt"
	"testing"
)

// --- MoveWithRollback ---

func TestMoveWithRollback_CopyThenRemoveSucceeds(t *testing.T) {
	var copyCalls, removeSrc, removeDest int
	ok := MoveWithRollback(context.Background(),
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
	ok := MoveWithRollback(context.Background(),
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
	ok := MoveWithRollback(context.Background(),
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
	ok := MoveWithRollback(ctx,
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
	ok := MoveWithRollback(context.Background(),
		"srcb", "srck", "dstb", "dstk",
		func() bool { return true },
		func(context.Context) error { return fmt.Errorf("src") },
		func(context.Context) error { return fmt.Errorf("dest") },
	)
	if ok {
		t.Fatal("ok = true, want false (a failed rollback must not report success)")
	}
}
