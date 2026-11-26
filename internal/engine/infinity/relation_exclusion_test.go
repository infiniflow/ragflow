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

package infinity

import (
	"fmt"
	"strings"
	"testing"
)

// TestEquivalentConditionToStrExcludesRelationWindowIDs drives the real filter
// renderer with the writer's read-id exclusion: each chunk becomes a
// NOT (id IN (...)) conjunct, every read id is excluded once, and an unread id
// is not excluded, for exactly one result window and past it.
func TestEquivalentConditionToStrExcludesRelationWindowIDs(t *testing.T) {
	const chunk = 512
	for _, tc := range []struct {
		name  string
		total int
	}{
		{"exactly one result window", 10000},
		{"past the result window", 10400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ids := make([]string, tc.total)
			for i := range ids {
				ids[i] = fmt.Sprintf("id-%d", i)
			}
			var and []map[string]interface{}
			for start := 0; start < len(ids); start += chunk {
				end := min(start+chunk, len(ids))
				and = append(and, map[string]interface{}{"must_not": map[string]interface{}{"id": ids[start:end]}})
			}
			got := equivalentConditionToStrRaw(map[string]interface{}{"and": and}, nil)

			if want := (tc.total + chunk - 1) / chunk; strings.Count(got, "NOT (id IN (") != want {
				t.Errorf("NOT clauses = %d, want %d", strings.Count(got, "NOT (id IN ("), want)
			}
			if !strings.Contains(got, " AND ") {
				t.Errorf("exclusions are not conjoined: %q", got[:min(len(got), 200)])
			}
			for _, id := range ids {
				if n := strings.Count(got, "'"+id+"'"); n != 1 {
					t.Fatalf("id %q excluded %d times, want 1", id, n)
				}
			}
			if strings.Contains(got, "'unread-id'") {
				t.Error("an id that was not read is excluded")
			}
		})
	}
}
