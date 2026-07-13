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

package chunker

import (
	"testing"

	"ragflow/internal/agent/runtime"
)

// TestNewChunkersRegistered pins that OneChunker, TagChunker and
// TableChunker are registered under CategoryIngestion (the exact
// invariant the handler/service component-list tests assert). It lives
// here so the check runs without the parser package, which has an
// unrelated, pre-existing build break on the diverged branch.
func TestNewChunkersRegistered(t *testing.T) {
	for _, name := range []string{ComponentNameOneChunker, ComponentNameTagChunker, ComponentNameTableChunker, ComponentNamePresentationChunker} {
		_, cat, _, ok := runtime.DefaultRegistry.Lookup(name)
		if !ok {
			t.Fatalf("%s: not registered", name)
		}
		if cat != runtime.CategoryIngestion {
			t.Errorf("%s: category = %q, want %q", name, cat, runtime.CategoryIngestion)
		}
	}
}
