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

package common

import (
	"reflect"
	"testing"
)

// TestDeduplicatePreservesContract pins the behaviour relied on by the removed
// dedupeMetaStrings helper: first-seen order, empty strings retained, and nil
// input returning nil. The last point is adversarial — the old helper returned
// a non-nil empty slice for nil input.
func TestDeduplicatePreservesContract(t *testing.T) {
	if got := Deduplicate(nil); got != nil {
		t.Errorf("Deduplicate(nil) = %v, want nil", got)
	}
	if got := Deduplicate([]string{}); got == nil {
		t.Error("Deduplicate([]string{}) returned nil, want non-nil empty slice")
	}
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "no duplicates", in: []string{"a", "b"}, want: []string{"a", "b"}},
		{name: "with duplicates", in: []string{"b", "a", "b", "c", "a"}, want: []string{"b", "a", "c"}},
		{name: "empty strings retained", in: []string{"", "a", "", "b"}, want: []string{"", "a", "b"}},
		{name: "unicode", in: []string{"中", "文", "中"}, want: []string{"中", "文"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Deduplicate(c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("Deduplicate(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
