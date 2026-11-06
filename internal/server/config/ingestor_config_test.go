//
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
//

package config

import (
	"testing"

	"github.com/spf13/viper"

	"ragflow/internal/common"
)

func TestParseIngestorConfigReadsWorkerAndCompilerSettings(t *testing.T) {
	v := viper.New()
	v.Set("ingestor", map[string]any{
		"concurrent_workers": 4,
		"compiler_pool_size": 2,
	})

	config := &Config{}
	if err := config.ParseIngestorConfig(v); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	if got := config.GetIngestorConfig().ConcurrentWorkers; got != 4 {
		t.Fatalf("max concurrent workers = %d, want 4", got)
	}
	if got := config.GetIngestorConfig().CompilerPoolSize; got != 2 {
		t.Fatalf("compiler pool size = %d, want 2", got)
	}
}

// TestParseIngestorConfigDefaultsToK1AndN2 pins the conservative defaults:
// the ingestor worker count (K) defaults to 1 and the process-wide page
// concurrency (N) defaults to 2 when no ingestor section is configured.
func TestParseIngestorConfigDefaultsToK1AndN2(t *testing.T) {
	config := &Config{}
	if err := config.ParseIngestorConfig(viper.New()); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	if got := config.GetIngestorConfig().ConcurrentWorkers; got != 1 {
		t.Fatalf("max concurrent workers = %d, want 1", got)
	}
	if got := config.GetIngestorConfig().PageConcurrency; got != 2 {
		t.Fatalf("page concurrency = %d, want 2", got)
	}
}

// TestParseIngestorConfigReadsPageConcurrency pins that the flattened
// ingestor.page_concurrency key is read from YAML.
func TestParseIngestorConfigReadsPageConcurrency(t *testing.T) {
	v := viper.New()
	v.Set("ingestor", map[string]any{
		"concurrent_workers": 3,
		"page_concurrency":   10,
		"compiler_pool_size": 2,
	})

	config := &Config{}
	if err := config.ParseIngestorConfig(v); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	if got := config.GetIngestorConfig().ConcurrentWorkers; got != 3 {
		t.Fatalf("max concurrent workers = %d, want 3", got)
	}
	if got := config.GetIngestorConfig().PageConcurrency; got != 10 {
		t.Fatalf("page concurrency = %d, want 10", got)
	}
}

// TestParseIngestorConfigToleratesOutOfRangeWorkers pins the precedence
// contract: ParseIngestorConfig must NOT fail-fast on an out-of-range
// ingestor.concurrent_workers. Range validation belongs to the
// CLI/env/config resolver (config.MinIngestorWorkers / MaxIngestorWorkers),
// which runs later during startup, so a config value that a higher-priority
// source (CLI flag or env var) overrides must survive config load. The resolver
// still rejects the final resolved value, so fail-fast is preserved where it
// matters.
func TestParseIngestorConfigToleratesOutOfRangeWorkers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value int
	}{
		{"zero", 0},
		{"negative", -3},
		{"above max", 257},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := viper.New()
			v.Set("ingestor", map[string]any{"concurrent_workers": tc.value})
			config := &Config{}
			if err := config.ParseIngestorConfig(v); err != nil {
				t.Fatalf("ParseIngestorConfig rejected concurrent_workers=%d: %v (validation moved to resolver)", tc.value, err)
			}
		})
	}
}

// TestParseIngestorConfigToleratesOutOfRangePageConcurrency pins the same
// precedence contract for ingestor.page_concurrency: config load tolerates
// out-of-range values; the resolver rejects the final resolved value.
func TestParseIngestorConfigToleratesOutOfRangePageConcurrency(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value int
	}{
		{"zero", 0},
		{"negative", -1},
		{"above max", 17},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := viper.New()
			v.Set("ingestor", map[string]any{"page_concurrency": tc.value})
			config := &Config{}
			if err := config.ParseIngestorConfig(v); err != nil {
				t.Fatalf("ParseIngestorConfig rejected page_concurrency=%d: %v (validation moved to resolver)", tc.value, err)
			}
		})
	}
}

// TestGetIngestorConcurrentWorkers pins the precedence
// CLI > environment > config file > default(1) and the inclusive [1, 256]
// range guard on the FINAL value. An out-of-range or non-integer value at
// any layer is a fatal startup error. An explicit zero or negative value at
// the config layer is rejected (not silently defaulted), so an operator typo
// such as concurrent_workers: 0 or -1 fails fast instead of silently running
// at the default of 1.
func TestGetIngestorConcurrentWorkers(t *testing.T) {
	const envKey = common.EnvIngestorConcurrentWorkers

	cases := []struct {
		name       string
		configured int
		set        bool   // true iff concurrent_workers was explicitly present in the config file
		env        string // "" means leave unset
		argValue   *int
		want       int
		wantErr    bool
	}{
		{"default", 0, false, "", nil, 1, false},
		{"config only", 5, true, "", nil, 5, false},
		{"env overrides config", 5, true, "8", nil, 8, false},
		{"cli overrides env and config", 5, true, "8", intPtr(12), 12, false},
		{"env invalid errors", 5, true, "notanint", nil, 0, true},
		{"env only", 0, false, "9", nil, 9, false},
		{"config above range errors", 300, true, "", nil, 0, true},
		{"cli below range errors", 0, false, "", intPtr(0), 0, true},
		{"cli above range errors", 0, false, "", intPtr(1000), 0, true},
		// Adversarial: an explicitly present zero/negative config value must be
		// rejected, distinguishing "explicit 0" (set=true) from "absent" (set=false
		// -> default 1). The old >0 guard silently defaulted both to 1.
		{"config explicit zero errors", 0, true, "", nil, 0, true},
		{"config negative errors", -5, true, "", nil, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv restores the previous value after the subtest.
			t.Setenv(envKey, tc.env)
			c := &Config{}
			if err := c.ParseIngestorConfig(viper.New()); err != nil {
				t.Fatalf("ParseIngestorConfig: %v", err)
			}
			c.ingestor.ConcurrentWorkers = tc.configured
			c.ingestor.ConcurrentWorkersSet = tc.set
			got, err := c.GetIngestorConcurrentWorkers(tc.argValue)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("got nil error, want rejection (configured=%d env=%q argValue=%v)",
						tc.configured, tc.env, tc.argValue)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d (configured=%d env=%q argValue=%v)",
					got, tc.want, tc.configured, tc.env, tc.argValue)
			}
		})
	}
}

// TestGetIngestorPageConcurrency pins the precedence
// CLI > environment > config file > default(2) and the inclusive [1, 16]
// range guard on the FINAL value. An out-of-range or non-integer value at
// any layer is a fatal startup error. An explicit zero or negative value at
// the config layer is rejected (not silently defaulted to 2), matching the
// env/CLI behavior.
func TestGetIngestorPageConcurrency(t *testing.T) {
	const envKey = common.EnvIngestorPageConcurrency

	cases := []struct {
		name       string
		configured int
		set        bool   // true iff page_concurrency was explicitly present in the config file
		env        string // "" means leave unset
		argValue   *int
		want       int
		wantErr    bool
	}{
		{"default", 0, false, "", nil, 2, false},
		{"config only", 6, true, "", nil, 6, false},
		{"env overrides config", 6, true, "8", nil, 8, false},
		{"cli overrides env and config", 6, true, "8", intPtr(12), 12, false},
		{"env invalid errors", 6, true, "notanint", nil, 0, true},
		{"env only", 0, false, "9", nil, 9, false},
		{"config above range errors", 20, true, "", nil, 0, true},
		{"cli below range errors", 0, false, "", intPtr(0), 0, true},
		// Adversarial: an explicitly present zero/negative config value must be
		// rejected, distinguishing "explicit 0" (set=true) from "absent" (set=false
		// -> default 2).
		{"config explicit zero errors", 0, true, "", nil, 0, true},
		{"config negative errors", -5, true, "", nil, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envKey, tc.env)
			c := &Config{}
			if err := c.ParseIngestorConfig(viper.New()); err != nil {
				t.Fatalf("ParseIngestorConfig: %v", err)
			}
			c.ingestor.PageConcurrency = tc.configured
			c.ingestor.PageConcurrencySet = tc.set
			got, err := c.GetIngestorPageConcurrency(tc.argValue)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("got nil error, want rejection (configured=%d env=%q argValue=%v)",
						tc.configured, tc.env, tc.argValue)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d (configured=%d env=%q argValue=%v)",
					got, tc.want, tc.configured, tc.env, tc.argValue)
			}
		})
	}
}
