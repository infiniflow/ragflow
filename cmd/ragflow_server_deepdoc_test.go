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

package main

import (
	"testing"

	"ragflow/internal/common"
)

// TestResolveDeepDocInferenceConcurrency pins the precedence
// CLI > environment > config file > default(4).
func TestResolveDeepDocInferenceConcurrency(t *testing.T) {
	const envKey = common.EnvDeepDocInferenceConcurrency

	cases := []struct {
		name       string
		configured int
		env        string // "" means leave unset
		cli        *int
		want       int
	}{
		{"default", 0, "", nil, 4},
		{"config only", 6, "", nil, 6},
		{"env overrides config", 6, "8", nil, 8},
		{"cli overrides env and config", 6, "8", intPtr(12), 12},
		{"cli zero does not override", 6, "8", intPtr(0), 8},
		{"cli negative does not override", 6, "", intPtr(-3), 6},
		{"env invalid falls back to config", 6, "notanint", nil, 6},
		{"env only", 0, "9", nil, 9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv restores the previous value after the subtest.
			t.Setenv(envKey, tc.env)
			args := &serverArgs{deepdocInferenceConcurrency: tc.cli}
			if got := resolveDeepDocInferenceConcurrency(args, tc.configured); got != tc.want {
				t.Fatalf("got %d, want %d (configured=%d env=%q cli=%v)",
					got, tc.want, tc.configured, tc.env, tc.cli)
			}
		})
	}
}

func intPtr(n int) *int { return &n }
