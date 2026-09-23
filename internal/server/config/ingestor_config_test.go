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
)

func TestParseIngestorConfigReadsWorkerAndCompilerSettings(t *testing.T) {
	v := viper.New()
	v.Set("ingestor", map[string]any{
		"max_concurrent_workers": 4,
		"compiler_pool_size":     2,
	})

	config := &Config{}
	if err := config.ParseIngestorConfig(v); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	if got := config.GetIngestorConfig().MaxConcurrentWorkers; got != 4 {
		t.Fatalf("max concurrent workers = %d, want 4", got)
	}
	if got := config.GetIngestorConfig().CompilerPoolSize; got != 2 {
		t.Fatalf("compiler pool size = %d, want 2", got)
	}
}

// TestParseIngestorConfigDefaultsToK1AndN2 pins the conservative defaults:
// the ingestor worker count (K) defaults to 1 and the per-document page
// concurrency (N) defaults to 2 when no ingestor section is configured.
func TestParseIngestorConfigDefaultsToK1AndN2(t *testing.T) {
	config := &Config{}
	if err := config.ParseIngestorConfig(viper.New()); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	if got := config.GetIngestorConfig().MaxConcurrentWorkers; got != 1 {
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
		"max_concurrent_workers": 3,
		"page_concurrency":       10,
		"compiler_pool_size":     2,
	})

	config := &Config{}
	if err := config.ParseIngestorConfig(v); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	if got := config.GetIngestorConfig().MaxConcurrentWorkers; got != 3 {
		t.Fatalf("max concurrent workers = %d, want 3", got)
	}
	if got := config.GetIngestorConfig().PageConcurrency; got != 10 {
		t.Fatalf("page concurrency = %d, want 10", got)
	}
}

// TestParseIngestorConfigRejectsOutOfRangeWorkers pins the fail-fast contract:
// an explicit ingestor.max_concurrent_workers outside [1, 256] is rejected at
// config load, not silently coerced.
func TestParseIngestorConfigRejectsOutOfRangeWorkers(t *testing.T) {
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
			v.Set("ingestor", map[string]any{"max_concurrent_workers": tc.value})
			config := &Config{}
			if err := config.ParseIngestorConfig(v); err == nil {
				t.Fatalf("ParseIngestorConfig accepted max_concurrent_workers=%d, want error", tc.value)
			}
		})
	}
}

// TestParseIngestorConfigRejectsOutOfRangePageConcurrency pins the same
// fail-fast contract for ingestor.page_concurrency outside [1, 16].
func TestParseIngestorConfigRejectsOutOfRangePageConcurrency(t *testing.T) {
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
			if err := config.ParseIngestorConfig(v); err == nil {
				t.Fatalf("ParseIngestorConfig accepted page_concurrency=%d, want error", tc.value)
			}
		})
	}
}
