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

package vastbase

import "testing"

// TestCopyMapNilAndIsolation pins the two properties callers rely on: a nil
// filter still yields a writable map, and discriminator writes never alias the
// caller's map. maps.Clone(nil) returns nil, so the guard is load-bearing.
func TestCopyMapNilAndIsolation(t *testing.T) {
	got := copyMap(nil)
	if got == nil {
		t.Fatal("copyMap(nil) returned nil, want writable empty map")
	}
	got["tenant_id"] = "t1"

	src := map[string]interface{}{"a": 1}
	cp := copyMap(src)
	cp["kb_id"] = "k1"
	if _, ok := src["kb_id"]; ok {
		t.Fatal("copyMap aliases its source")
	}
	if cp["a"] != 1 {
		t.Fatalf("copied value = %v, want 1", cp["a"])
	}
}
