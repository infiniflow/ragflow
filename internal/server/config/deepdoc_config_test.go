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

package config

import (
	"strings"
	"testing"

	"ragflow/internal/common"

	"github.com/spf13/viper"
)

// TestParseIngestorConfigDeepDocDefaultsToOne pins the default inference
// concurrency when the ingestor.inference_concurrency key is absent.
func TestParseIngestorConfigDeepDocDefaultsToOne(t *testing.T) {
	v := viper.New()
	c := &Config{}
	if err := c.ParseIngestorConfig(v); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	if got := c.ingestor.deepDocInferenceConcurrency; got != 1 {
		t.Fatalf("default inference concurrency = %d, want 1", got)
	}
}

// TestParseIngestorConfigReadsDeepDocYAML pins that the
// ingestor.inference_concurrency key is honoured when present.
func TestParseIngestorConfigReadsDeepDocYAML(t *testing.T) {
	v := viper.New()
	v.Set("ingestor", map[string]any{
		"inference_concurrency": 6,
	})
	c := &Config{}
	if err := c.ParseIngestorConfig(v); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	if got := c.ingestor.deepDocInferenceConcurrency; got != 6 {
		t.Fatalf("inference concurrency = %d, want 6", got)
	}
}

// TestParseIngestorConfigDeepDocIgnoresEnvVar pins the provenance of the env
// override: ParseIngestorConfig reads ONLY the ingestor.inference_concurrency
// YAML key and does not apply RAGFLOW_DEEPDOC_INFERENCE_CONCURRENCY. The env
// override is resolved separately by Config.ResolveDeepDocInferenceConcurrency.
func TestParseIngestorConfigDeepDocIgnoresEnvVar(t *testing.T) {
	t.Setenv(common.EnvDeepDocInferenceConcurrency, "9")

	v := viper.New()
	c := &Config{}
	if err := c.ParseIngestorConfig(v); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	if got := c.ingestor.deepDocInferenceConcurrency; got != 1 {
		t.Fatalf("inference concurrency with only env set = %d, want default 1 (env applied later)", got)
	}
}

// TestParseIngestorConfigDeepDocIgnoresAutoEnvVar pins that the auto-derived
// (undocumented) env var RAGFLOW_INGESTOR_INFERENCE_CONCURRENCY does NOT
// override the YAML ingestor.inference_concurrency. viper's Sub() inherits
// AutomaticEnv, so a plain GetInt would consult that variable before the file
// value. The config parse must be file-only; env precedence is applied later by
// ResolveDeepDocInferenceConcurrency via os.Getenv. This test uses the
// production Viper settings (env prefix + replacer + AutomaticEnv) to reproduce
// the real precedence resolution path.
func TestParseIngestorConfigDeepDocIgnoresAutoEnvVar(t *testing.T) {
	t.Setenv("RAGFLOW_INGESTOR_INFERENCE_CONCURRENCY", "bad")

	v := viper.New()
	v.SetEnvPrefix("RAGFLOW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.Set("ingestor", map[string]any{
		"inference_concurrency": 6,
	})

	c := &Config{}
	if err := c.ParseIngestorConfig(v); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	if got := c.ingestor.deepDocInferenceConcurrency; got != 6 {
		t.Fatalf("inference concurrency = %d, want 6 (auto-env must not override YAML)", got)
	}
}

// TestResolveDeepDocInferenceConcurrency pins the precedence
// CLI > environment > config file > default(1) and the fail-fast contract: any
// non-integer, non-positive, or otherwise invalid value is an error, never a
// silent fallback. The second return reports whether K was explicitly set by any
// layer (as opposed to the built-in default of 1).
func TestResolveDeepDocInferenceConcurrency(t *testing.T) {
	const envKey = common.EnvDeepDocInferenceConcurrency

	cases := []struct {
		name          string
		configured    int
		configuredSet bool
		env           string // "" means leave unset
		cli           *int
		want          int
		wantExplicit  bool
		wantErr       bool
	}{
		{"default", 0, false, "", nil, 1, false, false},
		{"config only", 6, true, "", nil, 6, true, false},
		{"env overrides config", 6, true, "8", nil, 8, true, false},
		{"cli overrides env and config", 6, true, "8", intPtr(12), 12, true, false},
		{"env only", 0, false, "9", nil, 9, true, false},
		// Invalid values are rejected, not silently ignored.
		{"env invalid -> error", 6, true, "notanint", nil, 0, false, true},
		{"config zero -> error", 0, true, "", nil, 0, false, true},
		{"config negative -> error", -3, true, "", nil, 0, false, true},
		{"env negative -> error", 6, true, "-2", nil, 0, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envKey, tc.env)

			c := &Config{}
			if err := c.ParseIngestorConfig(viper.New()); err != nil {
				t.Fatalf("ParseIngestorConfig: %v", err)
			}
			c.ingestor.deepDocInferenceConcurrency = tc.configured
			c.ingestor.deepDocInferenceConcurrencySet = tc.configuredSet
			got, explicit, err := c.ResolveDeepDocInferenceConcurrency(tc.cli)
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

// TestResolveDeepDocInferenceCPUCores pins the precedence
// CLI > environment > config file > default(0, "derive from K") for the CPU-core
// budget N, and that the second return reports whether N was explicitly set by
// any layer. An explicit 0 means "use all cores" and is returned as 0 (resolved
// against runtime.NumCPU() by the caller); only non-integer or negative values
// error.
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
		{"default unset", 0, false, "", nil, 0, false, false},
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

			c := &Config{}
			if err := c.ParseIngestorConfig(viper.New()); err != nil {
				t.Fatalf("ParseIngestorConfig: %v", err)
			}
			c.ingestor.deepDocInferenceCPUCores = tc.configured
			c.ingestor.deepDocInferenceCPUCoresSet = tc.configuredSet
			got, explicit, err := c.ResolveDeepDocInferenceCPUCores(tc.cli)
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

// TestParseIngestorConfigDeepDocRejectsNonInteger pins the fail-fast contract:
// a present-but-non-integer ingestor.inference_concurrency / inference_cpu_cores
// value must surface as an error rather than being silently coerced to 0 by
// viper/cast.
func TestParseIngestorConfigDeepDocRejectsNonInteger(t *testing.T) {
	v := viper.New()
	v.Set("ingestor", map[string]any{
		"inference_concurrency": "four",
	})
	c := &Config{}
	if err := c.ParseIngestorConfig(v); err == nil {
		t.Fatal("expected error for non-integer inference_concurrency, got nil (silently coerced to 0)")
	}

	v2 := viper.New()
	v2.Set("ingestor", map[string]any{
		"inference_cpu_cores": "alsobad",
	})
	c2 := &Config{}
	if err := c2.ParseIngestorConfig(v2); err == nil {
		t.Fatal("expected error for non-integer inference_cpu_cores, got nil (silently coerced to 0)")
	}
}

// TestParseIngestorConfigReadsDeepDocCPUCoresYAML pins that the
// ingestor.inference_cpu_cores key is honoured when present.
func TestParseIngestorConfigReadsDeepDocCPUCoresYAML(t *testing.T) {
	v := viper.New()
	v.Set("ingestor", map[string]any{
		"inference_cpu_cores": 8,
	})
	c := &Config{}
	if err := c.ParseIngestorConfig(v); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	if got := c.ingestor.deepDocInferenceCPUCores; got != 8 {
		t.Fatalf("inference cpu cores = %d, want 8", got)
	}
	if !c.ingestor.deepDocInferenceCPUCoresSet {
		t.Fatal("deepDocInferenceCPUCoresSet should be true when key present")
	}
}

func intPtr(n int) *int { return &n }
