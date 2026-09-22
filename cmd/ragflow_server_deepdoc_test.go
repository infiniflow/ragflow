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
	"os"
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

// TestParseArgsDeepDocInferenceConcurrency pins the contract that the CLI parser
// rejects a non-positive or non-integer --deepdoc-inference-concurrency up front
// (both the "--flag=value" and "--flag value" forms), and accepts a positive
// value. Because the parser guarantees a positive value, resolveDeepDocInference
// Concurrency can trust the parsed pointer and needs no extra >0 guard.
func TestParseArgsDeepDocInferenceConcurrency(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr bool
		want    int
	}{
		{"equals zero", []string{"prog", "--deepdoc-inference-concurrency=0"}, true, 0},
		{"equals negative", []string{"prog", "--deepdoc-inference-concurrency=-3"}, true, 0},
		{"equals nonint", []string{"prog", "--deepdoc-inference-concurrency=abc"}, true, 0},
		{"space zero", []string{"prog", "--deepdoc-inference-concurrency", "0"}, true, 0},
		{"space negative", []string{"prog", "--deepdoc-inference-concurrency", "-3"}, true, 0},
		{"space nonint", []string{"prog", "--deepdoc-inference-concurrency", "abc"}, true, 0},
		{"equals positive", []string{"prog", "--deepdoc-inference-concurrency=12"}, false, 12},
		{"space positive", []string{"prog", "--deepdoc-inference-concurrency", "12"}, false, 12},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old := os.Args
			defer func() { os.Args = old }()
			os.Args = tc.args

			got, err := parseArgs()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseArgs() = nil error, want rejection for %v", tc.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseArgs() error = %v, want nil for %v", err, tc.args)
			}
			if got.deepdocInferenceConcurrency == nil || *got.deepdocInferenceConcurrency != tc.want {
				t.Fatalf("deepdocInferenceConcurrency = %v, want %d for %v", got.deepdocInferenceConcurrency, tc.want, tc.args)
			}
		})
	}
}
