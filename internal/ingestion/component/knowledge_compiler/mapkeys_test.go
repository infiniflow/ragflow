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

package knowledge_compiler

import (
	"fmt"
	"slices"
	"testing"
)

// TestMapKeysReturnsSortedKeys pins the diagnostics contract: mapKeys exists so
// log lines are deterministic, so the output must be sorted and complete no
// matter how the input map iterates.
func TestMapKeysReturnsSortedKeys(t *testing.T) {
	t.Run("unordered keys come back sorted", func(t *testing.T) {
		got := mapKeys(map[string]any{"charlie": 1, "alpha": 2, "bravo": 3})
		want := []string{"alpha", "bravo", "charlie"}
		if !slices.Equal(got, want) {
			t.Fatalf("mapKeys = %v, want %v", got, want)
		}
	})

	t.Run("empty map yields empty slice", func(t *testing.T) {
		got := mapKeys(map[string]any{})
		if len(got) != 0 {
			t.Fatalf("mapKeys(empty) = %v, want empty", got)
		}
	})

	t.Run("nil map yields empty slice", func(t *testing.T) {
		got := mapKeys(nil)
		if len(got) != 0 {
			t.Fatalf("mapKeys(nil) = %v, want empty", got)
		}
	})

	t.Run("single key", func(t *testing.T) {
		got := mapKeys(map[string]any{"only": 1})
		if !slices.Equal(got, []string{"only"}) {
			t.Fatalf("mapKeys = %v, want [only]", got)
		}
	})

	t.Run("unicode keys sort by byte order like sort.Strings", func(t *testing.T) {
		got := mapKeys(map[string]any{"中文": 1, "abc": 2, "é": 3, "ZZ": 4})
		want := []string{"ZZ", "abc", "é", "中文"}
		if !slices.Equal(got, want) {
			t.Fatalf("mapKeys = %v, want %v", got, want)
		}
	})

	t.Run("many keys stay sorted and complete", func(t *testing.T) {
		m := make(map[string]any, 100)
		for i := 99; i >= 0; i-- { // insert in reverse to fight map iteration order
			m[fmt.Sprintf("key-%03d", i)] = i
		}
		got := mapKeys(m)
		if len(got) != 100 {
			t.Fatalf("mapKeys returned %d keys, want 100", len(got))
		}
		if !slices.IsSorted(got) {
			t.Fatal("mapKeys output is not sorted")
		}
	})
}
