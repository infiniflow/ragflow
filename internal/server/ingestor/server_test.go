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

package ingestor

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/server/bootstrap"
)

func intPtr(n int) *int { return &n }

func TestResolveIngestorMaxConcurrentWorkers(t *testing.T) {
	const envKey = common.EnvIngestorMaxConcurrentWorkers

	cases := []struct {
		name       string
		configured int
		env        string // "" means leave unset
		cli        *int
		want       int
		wantErr    bool
	}{
		{"default", 0, "", nil, 1, false},
		{"config only", 5, "", nil, 5, false},
		{"env overrides config", 5, "8", nil, 8, false},
		{"cli overrides env and config", 5, "8", intPtr(12), 12, false},
		{"env invalid errors", 5, "notanint", nil, 0, true},
		{"env only", 0, "9", nil, 9, false},
		{"config above range errors", 300, "", nil, 300, true},
		{"cli below range errors", 0, "", intPtr(0), 0, true},
		{"cli above range errors", 0, "", intPtr(1000), 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv restores the previous value after the subtest.
			t.Setenv(envKey, tc.env)
			args := &bootstrap.ServerArgs{IngestorMaxConcurrentWorkers: tc.cli}
			got, err := resolveIngestorMaxConcurrentWorkers(args, tc.configured)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("got nil error, want rejection (configured=%d env=%q cli=%v)",
						tc.configured, tc.env, tc.cli)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d (configured=%d env=%q cli=%v)",
					got, tc.want, tc.configured, tc.env, tc.cli)
			}
		})
	}
}

// TestResolveIngestorPageConcurrency pins the precedence
// CLI > environment > config file > default(2) and the inclusive [1, 16]
// range guard. An out-of-range or non-integer value at any layer is a fatal
// startup error.
func TestResolveIngestorPageConcurrency(t *testing.T) {
	const envKey = common.EnvIngestorPageConcurrency

	cases := []struct {
		name       string
		configured int
		env        string // "" means leave unset
		cli        *int
		want       int
		wantErr    bool
	}{
		{"default", 0, "", nil, 2, false},
		{"config only", 6, "", nil, 6, false},
		{"env overrides config", 6, "8", nil, 8, false},
		{"cli overrides env and config", 6, "8", intPtr(12), 12, false},
		{"env invalid errors", 6, "notanint", nil, 0, true},
		{"env only", 0, "9", nil, 9, false},
		{"config above range errors", 20, "", nil, 20, true},
		{"cli below range errors", 0, "", intPtr(0), 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envKey, tc.env)
			args := &bootstrap.ServerArgs{IngestorPageConcurrency: tc.cli}
			got, err := ResolveIngestorPageConcurrency(args, tc.configured)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("got nil error, want rejection (configured=%d env=%q cli=%v)",
						tc.configured, tc.env, tc.cli)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d (configured=%d env=%q cli=%v)",
					got, tc.want, tc.configured, tc.env, tc.cli)
			}
		})
	}
}
