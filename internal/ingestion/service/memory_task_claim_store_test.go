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

//go:build !ee

package service

import (
	"context"
	"testing"
)

func TestMemoryTaskClaimStore(t *testing.T) {
	store := newTaskClaimStore("test-ingestor")
	ctx := context.Background()

	claimed, err := store.Claim(ctx, "task-1")
	if err != nil || !claimed {
		t.Fatalf("first claim = (%v, %v), want (true, nil)", claimed, err)
	}
	claimed, err = store.Claim(ctx, "task-1")
	if err != nil || claimed {
		t.Fatalf("duplicate claim = (%v, %v), want (false, nil)", claimed, err)
	}
	if got := store.InFlight(); len(got) != 1 || got[0] != "task-1" {
		t.Fatalf("InFlight() = %v, want [task-1]", got)
	}

	store.Release(ctx, "task-1")
	if got := store.InFlight(); len(got) != 0 {
		t.Fatalf("InFlight() after release = %v, want empty", got)
	}
	claimed, err = store.Claim(ctx, "task-1")
	if err != nil || !claimed {
		t.Fatalf("claim after release = (%v, %v), want (true, nil)", claimed, err)
	}
}
