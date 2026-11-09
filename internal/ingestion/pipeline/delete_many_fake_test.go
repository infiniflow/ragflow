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

package pipeline

import (
	"context"
	"testing"
)

// TestCleanupFakeStoreDeleteMany verifies DeleteMany clears the kv, sets and
// bytes maps together (mirroring Delete) and reports success.
func TestCleanupFakeStoreDeleteMany(t *testing.T) {
	f := newCleanupFakeStore()
	f.kv["a"] = "1"
	f.kv["b"] = "2"
	f.sets["a"] = map[string]bool{"x": true}
	f.sets["b"] = map[string]bool{"y": true}
	f.bytes["a"] = 10
	f.bytes["b"] = 20

	if !f.DeleteMany(context.Background(), "a", "b") {
		t.Fatal("DeleteMany returned false")
	}

	for _, k := range []string{"a", "b"} {
		if _, ok := f.kv[k]; ok {
			t.Errorf("kv[%s] not deleted", k)
		}
		if _, ok := f.sets[k]; ok {
			t.Errorf("sets[%s] not deleted", k)
		}
		if _, ok := f.bytes[k]; ok {
			t.Errorf("bytes[%s] not deleted", k)
		}
	}
}
