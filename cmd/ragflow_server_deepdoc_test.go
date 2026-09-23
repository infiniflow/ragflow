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

func intPtr(n int) *int { return &n }

// TestResolveDeepDocInferenceConcurrency pins the precedence
// CLI > environment > config file > default(4) and the fail-fast contract:
// any non-integer, non-positive, or otherwise invalid value is an error, never a
// silent fallback.
func TestResolveDeepDocInferenceConcurrency(t *testing.T) {
	const envKey = common.EnvDeepDocInferenceConcurrency

	cases := []struct {
		name          string
		configured    int
		configuredSet bool
		env           string // "" means leave unset
		cli           *int
		want          int
		wantErr       bool
	}{
		{"default", 0, false, "", nil, 4, false},
		{"config only", 6, true, "", nil, 6, false},
		{"env overrides config", 6, true, "8", nil, 8, false},
		{"cli overrides env and config", 6, true, "8", intPtr(12), 12, false},
		{"env only", 0, false, "9", nil, 9, false},
		// Invalid values are rejected, not silently ignored.
		{"env invalid -> error", 6, true, "notanint", nil, 0, true},
		{"config zero -> error", 0, true, "", nil, 0, true},
		{"config negative -> error", -3, true, "", nil, 0, true},
		{"env negative -> error", 6, true, "-2", nil, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv restores the previous value after the subtest.
			t.Setenv(envKey, tc.env)
			args := &serverArgs{deepdocInferenceConcurrency: tc.cli}
			got, err := resolveDeepDocInferenceConcurrency(args, tc.configured, tc.configuredSet)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d (configured=%d configuredSet=%v env=%q cli=%v)",
					got, tc.want, tc.configured, tc.configuredSet, tc.env, tc.cli)
			}
		})
	}
}

// TestResolveDeepDocInferenceCPUCores pins the precedence
// CLI > environment > config file > default(4) for the CPU-core budget N, and that
// the second return reports whether N was explicitly set by any layer. An explicit
// 0 means "use all cores" and is returned as 0 (resolved against runtime.NumCPU()
// by the caller); only non-integer or negative values error.
func TestResolveDeepDocInferenceCPUCores(t *testing.T) {
	const envKey = common.EnvDeepDocInferenceCPUCores

	cases := []struct {
		name          string
		configured    int
		configuredSet bool
		env           string
		cli           *int
		want          int
		wantExplicit  bool
		wantErr       bool
	}{
		{"default", 0, false, "", nil, 4, false, false},
		{"config zero -> all cores sentinel", 0, true, "", nil, 0, true, false},
		{"config only", 8, true, "", nil, 8, true, false},
		{"env overrides config", 8, true, "16", nil, 16, true, false},
		{"cli overrides env and config", 8, true, "16", intPtr(32), 32, true, false},
		{"env only", 0, false, "12", nil, 12, true, false},
		{"cli zero valid -> all cores sentinel", 0, true, "", intPtr(0), 0, true, false},
		// Invalid values are rejected.
		{"env invalid -> error", 8, true, "notanint", nil, 0, false, true},
		{"env negative -> error", 8, true, "-1", nil, 0, false, true},
		{"config negative -> error", -1, true, "", nil, 0, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envKey, tc.env)
			args := &serverArgs{deepdocInferenceCPUCores: tc.cli}
			got, explicit, err := resolveDeepDocInferenceCPUCores(args, tc.configured, tc.configuredSet)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %d (explicit=%v)", got, explicit)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d (configured=%d configuredSet=%v env=%q cli=%v)",
					got, tc.want, tc.configured, tc.configuredSet, tc.env, tc.cli)
			}
			if explicit != tc.wantExplicit {
				t.Fatalf("explicit = %v, want %v", explicit, tc.wantExplicit)
			}
		})
	}
}

// TestClampDefaultCPUCores pins the default-budget rule: the default 4 is clamped
// down to the available core count when 4 exceeds it, but only when the budget was
// not explicitly set. An explicit budget is never clamped (a genuine
// misconfiguration must surface as an error elsewhere).
func TestClampDefaultCPUCores(t *testing.T) {
	cases := []struct {
		totalCores int
		rawN       int
		explicit   bool
		want       int
	}{
		{8, 4, false, 4}, // default 4 fits
		{2, 4, false, 2}, // default 4 clamped to M=2
		{1, 4, false, 1}, // default 4 clamped to M=1
		{2, 8, true, 8},  // explicit 8 NOT clamped (error handled upstream)
		{4, 4, true, 4},  // explicit 4 unchanged
		{8, 0, true, 0},  // explicit 0 (all cores) unchanged
		{8, 0, false, 0}, // default 0 (all cores) unchanged
	}
	for _, c := range cases {
		if got := clampDefaultCPUCores(c.totalCores, c.rawN, c.explicit); got != c.want {
			t.Errorf("clampDefaultCPUCores(%d, %d, %v) = %d, want %d",
				c.totalCores, c.rawN, c.explicit, got, c.want)
		}
	}
}

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

// TestParseArgsDeepDocInferenceCPUCores pins the contract that the CLI parser
// accepts a non-negative --deepdoc-inference-cpu-cores (0 means "all cores"),
// and rejects a negative or non-integer value (both forms).
func TestParseArgsDeepDocInferenceCPUCores(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr bool
		want    int
	}{
		{"equals zero valid", []string{"prog", "--deepdoc-inference-cpu-cores=0"}, false, 0},
		{"equals positive", []string{"prog", "--deepdoc-inference-cpu-cores=4"}, false, 4},
		{"space positive", []string{"prog", "--deepdoc-inference-cpu-cores", "4"}, false, 4},
		{"equals negative", []string{"prog", "--deepdoc-inference-cpu-cores=-1"}, true, 0},
		{"equals nonint", []string{"prog", "--deepdoc-inference-cpu-cores=abc"}, true, 0},
		{"space nonint", []string{"prog", "--deepdoc-inference-cpu-cores", "abc"}, true, 0},
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
			if got.deepdocInferenceCPUCores == nil || *got.deepdocInferenceCPUCores != tc.want {
				t.Fatalf("deepdocInferenceCPUCores = %v, want %d for %v", got.deepdocInferenceCPUCores, tc.want, tc.args)
			}
		})
	}
}
