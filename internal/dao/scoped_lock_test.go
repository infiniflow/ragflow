// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dao

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestScopedLockRetainedUntilTransactionEnds(t *testing.T) {
	ctx, finish := WithScopedLocks(context.Background())
	var mu sync.Mutex
	HoldScopedLock(ctx, "page", &mu)()
	HoldScopedLock(ctx, "page", &mu)()
	if mu.TryLock() {
		mu.Unlock()
		t.Fatal("lock released before transaction finished")
	}
	acquired := make(chan struct{})
	go func() { mu.Lock(); close(acquired); mu.Unlock() }()
	select {
	case <-acquired:
		t.Fatal("lock released before transaction finished")
	default:
	}
	finish()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("lock not released after transaction")
	}
}
