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

package chunkcache

import (
	"testing"

	"ragflow/internal/engine/redis"
)

// Compile-time proof that the real client still satisfies Store. Without it a
// signature change in the redis package would only surface at the call sites.
var _ Store = (*redis.Client)(nil)

// TestClient_NilInterfaceWhenRedisAbsent is the reason Client() exists. Handing
// redis.Get() straight to a Store parameter would wrap a typed nil pointer in a
// non-nil interface, defeating every `s == nil` guard and panicking on the
// first call in a Redis-less deployment. Client() must return an interface that
// compares equal to nil.
func TestClient_NilInterfaceWhenRedisAbsent(t *testing.T) {
	if redis.Get() != nil {
		t.Skip("a global Redis client is configured in this process")
	}
	if got := Client(); got != nil {
		t.Fatalf("Client() = %#v, want a nil interface", got)
	}
	// The guards must hold for the value Client() actually returns.
	Set(taskCtx("task-1"), Client(), "k", "v")
	if got, ok := Get(taskCtx("task-1"), Client(), "k"); ok || got != "" {
		t.Errorf("Get = (%q, %v), want (\"\", false)", got, ok)
	}
	PurgeTask(taskCtx("task-1"), Client(), "task-1")
}
