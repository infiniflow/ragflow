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

package component

import (
	"context"
	"testing"
	"time"
)

// TestMemCacheStoreDeleteMany verifies the batch delete clears both the data
// map and the set map (mirroring Delete) and reports success. A naive
// DeleteMany that only clears data would leave set members orphaned.
func TestMemCacheStoreDeleteMany(t *testing.T) {
	m := newMemCacheStore()
	m.mu.Lock()
	m.data["a"] = "1"
	m.data["b"] = "2"
	m.sets["a"] = map[string]struct{}{"x": {}}
	m.sets["b"] = map[string]struct{}{"y": {}}
	m.mu.Unlock()

	if !m.DeleteMany(context.Background(), "a", "b") {
		t.Fatal("DeleteMany returned false")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range []string{"a", "b"} {
		if _, ok := m.data[k]; ok {
			t.Errorf("data[%s] not deleted", k)
		}
		if _, ok := m.sets[k]; ok {
			t.Errorf("sets[%s] not deleted", k)
		}
	}
}

// TestFakeCacheStoreDeleteMany verifies DeleteMany clears both kv and sets.
func TestFakeCacheStoreDeleteMany(t *testing.T) {
	f := &fakeCacheStore{
		kv:   map[string]string{},
		ttl:  map[string]time.Duration{},
		sets: map[string]map[string]bool{},
	}
	f.kv["a"] = "1"
	f.kv["b"] = "2"
	f.sets["a"] = map[string]bool{"x": true}
	f.sets["b"] = map[string]bool{"y": true}

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
	}
}
