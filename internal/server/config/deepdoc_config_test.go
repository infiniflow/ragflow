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
// concurrency when the ingestor.deepdoc key is absent.
func TestParseIngestorConfigDeepDocDefaultsToOne(t *testing.T) {
	v := viper.New()
	c := &Config{}
	if err := c.ParseIngestorConfig(v); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	if got := c.GetIngestorConfig().DeepDoc.InferenceConcurrency; got != 1 {
		t.Fatalf("default inference concurrency = %d, want 1", got)
	}
}

// TestParseIngestorConfigReadsDeepDocYAML pins that the
// ingestor.deepdoc.inference_concurrency key is honoured when present.
func TestParseIngestorConfigReadsDeepDocYAML(t *testing.T) {
	v := viper.New()
	v.Set("ingestor", map[string]any{
		"deepdoc": map[string]any{"inference_concurrency": 6},
	})
	c := &Config{}
	if err := c.ParseIngestorConfig(v); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	if got := c.GetIngestorConfig().DeepDoc.InferenceConcurrency; got != 6 {
		t.Fatalf("inference concurrency = %d, want 6", got)
	}
}

// TestParseIngestorConfigDeepDocIgnoresEnvVar pins the provenance of the env
// override: ParseIngestorConfig reads ONLY the ingestor.deepdoc YAML key and
// does not apply RAGFLOW_DEEPDOC_INFERENCE_CONCURRENCY. The env override is
// resolved separately by Config.ResolveDeepDocInferenceConcurrency.
func TestParseIngestorConfigDeepDocIgnoresEnvVar(t *testing.T) {
	t.Setenv(common.EnvDeepDocInferenceConcurrency, "9")

	v := viper.New()
	c := &Config{}
	if err := c.ParseIngestorConfig(v); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	if got := c.GetIngestorConfig().DeepDoc.InferenceConcurrency; got != 1 {
		t.Fatalf("inference concurrency with only env set = %d, want default 1 (env applied later)", got)
	}
}

// TestParseIngestorConfigDeepDocIgnoresAutoEnvVar pins that the auto-derived
// (undocumented) env var RAGFLOW_INGESTOR_DEEPDOC_INFERENCE_CONCURRENCY does
// NOT override the YAML ingestor.deepdoc.inference_concurrency. viper's Sub()
// inherits AutomaticEnv, so a plain GetInt would consult that variable before
// the file value. The config parse must be file-only; env precedence is
// applied later by ResolveDeepDocInferenceConcurrency via os.Getenv. This test
// uses the production Viper settings (env prefix + replacer + AutomaticEnv) to
// reproduce the real precedence resolution path.
func TestParseIngestorConfigDeepDocIgnoresAutoEnvVar(t *testing.T) {
	t.Setenv("RAGFLOW_INGESTOR_DEEPDOC_INFERENCE_CONCURRENCY", "bad")

	v := viper.New()
	v.SetEnvPrefix("RAGFLOW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.Set("ingestor", map[string]any{
		"deepdoc": map[string]any{"inference_concurrency": 6},
	})

	c := &Config{}
	if err := c.ParseIngestorConfig(v); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	if got := c.GetIngestorConfig().DeepDoc.InferenceConcurrency; got != 6 {
		t.Fatalf("inference concurrency = %d, want 6 (auto-env must not override YAML)", got)
	}
}

// TestResolveDeepDocInferenceConcurrency pins the precedence
// CLI > environment > config file > default(1).
func TestResolveDeepDocInferenceConcurrency(t *testing.T) {
	const envKey = common.EnvDeepDocInferenceConcurrency

	cases := []struct {
		name       string
		configured int
		env        string // "" means leave unset
		cli        *int
		want       int
	}{
		{"default", 0, "", nil, 1},
		{"config only", 6, "", nil, 6},
		{"env overrides config", 6, "8", nil, 8},
		{"cli overrides env and config", 6, "8", intPtr(12), 12},
		{"env invalid falls back to config", 6, "notanint", nil, 6},
		{"env only", 0, "9", nil, 9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envKey, tc.env)

			c := &Config{}
			if err := c.ParseIngestorConfig(viper.New()); err != nil {
				t.Fatalf("ParseIngestorConfig: %v", err)
			}
			if tc.configured > 0 {
				c.ingestor.DeepDoc.InferenceConcurrency = tc.configured
			}
			if got := c.ResolveDeepDocInferenceConcurrency(tc.cli); got != tc.want {
				t.Fatalf("got %d, want %d (configured=%d env=%q cli=%v)",
					got, tc.want, tc.configured, tc.env, tc.cli)
			}
		})
	}
}

func intPtr(n int) *int { return &n }
