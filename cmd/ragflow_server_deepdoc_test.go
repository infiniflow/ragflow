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
	goruntime "runtime"
	"testing"

	"github.com/spf13/viper"
	common "ragflow/internal/common"
	native "ragflow/internal/deepdoc/native"
	server "ragflow/internal/server/config"
)

// TestParseArgsDeepDocInferenceConcurrency pins the contract that the CLI parser
// rejects a non-positive or non-integer --deepdoc-inference-concurrency up front
// (both the "--flag=value" and "--flag value" forms), and accepts a positive
// value. Because the parser guarantees a positive value, ResolveDeepDocInference
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

// TestResolveIngestorMaxConcurrentWorkers pins the precedence
// CLI > environment > config file > default(1) and the inclusive [1, 256]
// range guard. An out-of-range or non-integer value at any layer is a fatal
// startup error.
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
			args := &serverArgs{ingestorMaxConcurrentWorkers: tc.cli}
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
			args := &serverArgs{ingestorPageConcurrency: tc.cli}
			got, err := resolveIngestorPageConcurrency(args, tc.configured)
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

// TestParseArgsIngestorMaxConcurrentWorkers pins the contract that the CLI
// parser rejects a non-positive or non-integer --ingestor-max-concurrent-workers
// up front (both the "--flag=value" and "--flag value" forms), and accepts a
// positive value.
func TestParseArgsIngestorMaxConcurrentWorkers(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr bool
		want    int
	}{
		{"equals zero", []string{"prog", "--ingestor-max-concurrent-workers=0"}, true, 0},
		{"equals negative", []string{"prog", "--ingestor-max-concurrent-workers=-3"}, true, 0},
		{"equals nonint", []string{"prog", "--ingestor-max-concurrent-workers=abc"}, true, 0},
		{"space zero", []string{"prog", "--ingestor-max-concurrent-workers", "0"}, true, 0},
		{"space negative", []string{"prog", "--ingestor-max-concurrent-workers", "-3"}, true, 0},
		{"space nonint", []string{"prog", "--ingestor-max-concurrent-workers", "abc"}, true, 0},
		{"equals positive", []string{"prog", "--ingestor-max-concurrent-workers=12"}, false, 12},
		{"space positive", []string{"prog", "--ingestor-max-concurrent-workers", "12"}, false, 12},
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
			if got.ingestorMaxConcurrentWorkers == nil || *got.ingestorMaxConcurrentWorkers != tc.want {
				t.Fatalf("ingestorMaxConcurrentWorkers = %v, want %d for %v", got.ingestorMaxConcurrentWorkers, tc.want, tc.args)
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

// TestParseArgsIngestorPageConcurrency pins the same CLI parse contract for
// --ingestor-page-concurrency.
func TestParseArgsIngestorPageConcurrency(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr bool
		want    int
	}{
		{"equals zero", []string{"prog", "--ingestor-page-concurrency=0"}, true, 0},
		{"equals negative", []string{"prog", "--ingestor-page-concurrency=-3"}, true, 0},
		{"equals nonint", []string{"prog", "--ingestor-page-concurrency=abc"}, true, 0},
		{"space zero", []string{"prog", "--ingestor-page-concurrency", "0"}, true, 0},
		{"space negative", []string{"prog", "--ingestor-page-concurrency", "-3"}, true, 0},
		{"space nonint", []string{"prog", "--ingestor-page-concurrency", "abc"}, true, 0},
		{"equals positive", []string{"prog", "--ingestor-page-concurrency=12"}, false, 12},
		{"space positive", []string{"prog", "--ingestor-page-concurrency", "12"}, false, 12},
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
			if got.ingestorPageConcurrency == nil || *got.ingestorPageConcurrency != tc.want {
				t.Fatalf("ingestorPageConcurrency = %v, want %d for %v", got.ingestorPageConcurrency, tc.want, tc.args)
			}
		})
	}
}

// TestValidateInferenceConfigKNExceedsBudget pins the CPU-core budget N as a hard
// ceiling via the same exported function the server calls at startup. When K > N,
// max(1, N/K) floors at 1 so total occupancy would be K, oversubscribing the box
// beyond N.
func TestValidateInferenceConfigKNExceedsBudget(t *testing.T) {
	if _, _, err := native.ValidateInferenceConfig(8, 2, 8); err == nil {
		t.Fatal("expected error when concurrency K=8 exceeds cpu-core budget N=2")
	}
	if c, total, err := native.ValidateInferenceConfig(8, 4, 2); err != nil {
		t.Fatalf("unexpected error for K<=N: %v", err)
	} else if c != 2 || total != 4 {
		t.Fatalf("coresPerInference=%d totalCPUCores=%d, want 2/4", c, total)
	}
}

// TestRegisterNativeDeepDocDefaultBootsOnSmallMachine pins the startup contract
// that the shipped default configuration (both deepdoc keys unset, so K defaults
// to 1 and N is derived from K) still boots on a host with fewer than the
// nominal core count: the derived N equals K, ValidateInferenceConfig accepts
// them, and registerNativeDeepDoc must not call common.Fatal. This mirrors the
// resolution order in registerNativeDeepDoc and guards against a regression
// where an explicit default larger than the host core count would fatally reject
// startup.
func TestRegisterNativeDeepDocDefaultBootsOnSmallMachine(t *testing.T) {
	const totalCores = 2
	cfg := &server.Config{}
	if err := cfg.ParseIngestorConfig(viper.New()); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	// No CLI override; both config keys unset.
	K, explicitK, errK := cfg.ResolveDeepDocInferenceConcurrency(nil)
	if errK != nil {
		t.Fatalf("ResolveDeepDocInferenceConcurrency: %v", errK)
	}
	rawN, explicitN, errN := cfg.ResolveDeepDocInferenceCPUCores(nil)
	if errN != nil {
		t.Fatalf("ResolveDeepDocInferenceCPUCores: %v", errN)
	}
	if explicitK || explicitN {
		t.Fatal("default config must report both K and N as not-explicit")
	}
	// Unset N is derived from K, preserving single-threaded-per-run semantics.
	resolvedN := rawN
	if !explicitN {
		resolvedN = K
	}
	if K != 1 {
		t.Fatalf("default K = %d, want 1", K)
	}
	if resolvedN != K {
		t.Fatalf("derived N = %d, want K=%d", resolvedN, K)
	}
	if _, _, err := native.ValidateInferenceConfig(totalCores, resolvedN, K); err != nil {
		t.Fatalf("default config on %d-core host must validate, got: %v", totalCores, err)
	}
}

// TestInferenceTotalCoresIsCgroupAware pins that the DeepDoc inference core
// budget passed to native.ValidateInferenceConfig is derived from
// runtime.GOMAXPROCS(0) (cgroup-quota aware in Go 1.25+) rather than
// runtime.NumCPU() (host affinity mask, which ignores a container's CPU limit).
// This keeps the fail-fast oversubscription guard effective under container CPU
// quotas: with NumCPU(), a 2-CPU-limit pod on a 64-core host would "resolve"
// inference_cpu_cores: 0 to 64 and let inference_concurrency: 16 pass validation,
// oversubscribing the box the check exists to prevent.
func TestInferenceTotalCoresIsCgroupAware(t *testing.T) {
	got := inferenceTotalCores()
	want := goruntime.GOMAXPROCS(0)
	if got != want {
		t.Fatalf("inferenceTotalCores() = %d, want runtime.GOMAXPROCS(0) = %d (cgroup-aware budget)", got, want)
	}
	if got < 1 {
		t.Fatalf("inferenceTotalCores() = %d, want >= 1", got)
	}
}

// intPtr returns a pointer to n, used to build optional CLI override values in
// the resolver precedence tests.
func intPtr(n int) *int { return &n }
