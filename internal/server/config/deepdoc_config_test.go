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

// TestParseDeepDocConfigDefaultsToFour pins the default inference concurrency.
func TestParseDeepDocConfigDefaultsToFour(t *testing.T) {
	v := viper.New()
	c := &Config{}
	if err := c.ParseDeepDocConfig(v); err != nil {
		t.Fatalf("ParseDeepDocConfig: %v", err)
	}
	if got := c.GetDeepDocConfig().InferenceConcurrency; got != 4 {
		t.Fatalf("default inference concurrency = %d, want 4", got)
	}
}

// TestParseDeepDocConfigReadsYAML pins that the deepdoc.inference_concurrency
// key is honoured when present.
func TestParseDeepDocConfigReadsYAML(t *testing.T) {
	v := viper.New()
	v.Set("deepdoc", map[string]any{"inference_concurrency": 6})
	c := &Config{}
	if err := c.ParseDeepDocConfig(v); err != nil {
		t.Fatalf("ParseDeepDocConfig: %v", err)
	}
	if got := c.GetDeepDocConfig().InferenceConcurrency; got != 6 {
		t.Fatalf("inference concurrency = %d, want 6", got)
	}
}

// TestParseDeepDocConfigIgnoresEnvVar pins the provenance of the env override:
// ParseDeepDocConfig reads ONLY the deepdoc.inference_concurrency YAML key and
// does not apply the RAGFLOW_DEEPDOC_INFERENCE_CONCURRENCY environment variable.
// This holds even with viper's AutomaticEnv configured exactly as server.Init
// does, because v.Sub("deepdoc") does not inherit the parent's env (prefix /
// replacer / AutomaticEnv). The env override is resolved later in the server
// boot path by cmd.resolveDeepDocInferenceConcurrency (via os.Getenv), so the
// configured value returned here must stay at the YAML/default value.
func TestParseDeepDocConfigIgnoresEnvVar(t *testing.T) {
	t.Setenv(common.EnvDeepDocInferenceConcurrency, "9")

	v := viper.New()
	v.SetEnvPrefix("RAGFLOW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	c := &Config{}
	if err := c.ParseDeepDocConfig(v); err != nil {
		t.Fatalf("ParseDeepDocConfig: %v", err)
	}
	if got := c.GetDeepDocConfig().InferenceConcurrency; got != 4 {
		t.Fatalf("inference concurrency with only env set = %d, want default 4 (env must not be applied here)", got)
	}
}

// TestParseDeepDocConfigRejectsNonInteger pins the fail-fast contract from the
// resolver docs: a present-but-non-integer deepdoc value must surface as an
// error rather than being silently coerced to 0 by viper/cast.
func TestParseDeepDocConfigRejectsNonInteger(t *testing.T) {
	v := viper.New()
	v.Set("deepdoc", map[string]any{"inference_concurrency": "four"})
	c := &Config{}
	if err := c.ParseDeepDocConfig(v); err == nil {
		t.Fatal("expected error for non-integer inference_concurrency, got nil (silently coerced to 0)")
	}

	v2 := viper.New()
	v2.Set("deepdoc", map[string]any{"inference_cpu_cores": "alsobad"})
	c2 := &Config{}
	if err := c2.ParseDeepDocConfig(v2); err == nil {
		t.Fatal("expected error for non-integer inference_cpu_cores, got nil (silently coerced to 0)")
	}
}
