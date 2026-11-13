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

package infinity

import (
	"fmt"
	"reflect"
	"testing"

	"ragflow/internal/dao"
)

func TestCollectMetadataDocIDs(t *testing.T) {
	groups := []dao.TenantKBIDs{
		{TenantID: "tenant-a", KBIDs: []string{"kb-a", "kb-b"}},
		{TenantID: "tenant-b", KBIDs: []string{"kb-c"}},
		{TenantID: "tenant-c", KBIDs: []string{"kb-d"}},
	}
	docIDs := make([]string, metaPushdownMaxSize+1)
	for i := range docIDs {
		docIDs[i] = fmt.Sprintf("doc-%d", i)
	}
	half := metaPushdownMaxSize / 2
	tests := []struct {
		name      string
		results   [][]string
		want      []string
		wantCalls int
	}{
		{
			name:      "merges all tenants and deduplicates",
			results:   [][]string{{"doc-a", "shared"}, {"shared", "doc-b"}, {"doc-c"}},
			want:      []string{"doc-a", "shared", "doc-b", "doc-c"},
			wantCalls: 3,
		},
		{
			name:      "tenant failure discards partial results and stops",
			results:   [][]string{{"doc-a"}, nil, {"doc-c"}},
			wantCalls: 2,
		},
		{
			name:      "combined overflow falls back and stops",
			results:   [][]string{docIDs[:half], docIDs[half:], {"doc-c"}},
			wantCalls: 2,
		},
		{
			name:      "exact cap succeeds",
			results:   [][]string{docIDs[:half], docIDs[half:metaPushdownMaxSize], {}},
			want:      docIDs[:metaPushdownMaxSize],
			wantCalls: 3,
		},
		{
			name:      "all tenants have zero matches",
			results:   [][]string{{}, {}, {}},
			want:      []string{},
			wantCalls: 3,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			got := collectMetadataDocIDs(groups, func(group dao.TenantKBIDs) []string {
				if calls >= len(groups) {
					t.Fatal("queried more tenants than requested")
				}
				if !reflect.DeepEqual(group, groups[calls]) {
					t.Fatalf("tenant scope = %#v, want %#v", group, groups[calls])
				}
				ids := tc.results[calls]
				calls++
				return ids
			})
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("doc IDs: length = %d, nil = %t; want length = %d, nil = %t", len(got), got == nil, len(tc.want), tc.want == nil)
			}
			if calls != tc.wantCalls {
				t.Fatalf("tenant queries = %d, want %d", calls, tc.wantCalls)
			}
		})
	}
}
