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

	"github.com/spf13/viper"
)

// TestResolveLogLevel pins the precedence CLI flag > config file (viper,
// already merged with env) > default ("warn") and the fail-fast contract:
// the FINAL resolved value is validated once, so an invalid level reaching
// startup from the config file or the environment is rejected instead of
// being silently downgraded by the logger.
func TestResolveLogLevel(t *testing.T) {
	cases := []struct {
		name        string
		configLevel string // "" means do not set log.level in the config
		cli         *string
		want        string
		wantErr     bool
	}{
		{"default when nothing configured", "", nil, "warn", false},
		{"config only", "debug", nil, "debug", false},
		{"cli overrides config", "debug", strPtr("error"), "error", false},
		{"cli only", "", strPtr("info"), "info", false},
		{"warn from config", "warn", nil, "warn", false},
		// An invalid level at the config layer must fail startup, not fall
		// back silently (common.InitLogger downgrades unknown levels to info).
		{"config invalid -> error", "verbose", nil, "", true},
		{"cli invalid -> error", "debug", strPtr("verbose"), "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := viper.New()
			if tc.configLevel != "" {
				v.Set("log", map[string]any{"level": tc.configLevel})
			}
			c := &Config{}
			if err := c.ParseLogConfig(v); err != nil {
				t.Fatalf("ParseLogConfig: %v", err)
			}
			got, err := c.ResolveLogLevel(tc.cli)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ResolveLogLevel() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveLogLevel() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Fatalf("ResolveLogLevel() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestResolveLogLevelZeroValueConfigDefaults pins the defensive default: a
// Config that never ran ParseLogConfig (empty Level) resolves to the
// documented default "warn" rather than failing on an empty string.
func TestResolveLogLevelZeroValueConfigDefaults(t *testing.T) {
	c := &Config{}
	got, err := c.ResolveLogLevel(nil)
	if err != nil {
		t.Fatalf("ResolveLogLevel() error = %v, want nil", err)
	}
	if got != "warn" {
		t.Fatalf("ResolveLogLevel() = %q, want %q", got, "warn")
	}
}

// newEnvViper mirrors the production viper wiring in server.Init: env prefix
// RAGFLOW, "." -> "_" key replacer, and AutomaticEnv. Without this, an
// env-only key (e.g. RAGFLOW_LOG_LEVEL with no `log:` YAML section) is not
// visible to the parser and the override is silently dropped.
func newEnvViper() *viper.Viper {
	v := viper.New()
	v.SetEnvPrefix("RAGFLOW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	return v
}

// TestResolveLogLevelEnvOverride pins the ENVIRONMENT layer of the log-level
// precedence (the layer the config-file-only tests above do not exercise).
// RAGFLOW_LOG_LEVEL must reach ResolveLogLevel so an invalid value aborts
// startup instead of being silently downgraded by common.InitLogger, and a
// valid value wins over the built-in default. The CLI flag still overrides
// the env value.
func TestResolveLogLevelEnvOverride(t *testing.T) {
	cases := []struct {
		name    string
		envVal  string // RAGFLOW_LOG_LEVEL; "" means unset
		cli     *string
		want    string
		wantErr bool
	}{
		{"env only", "debug", nil, "debug", false},
		{"env warn", "warn", nil, "warn", false},
		{"cli overrides env", "debug", strPtr("error"), "error", false},
		// An invalid level arriving from the environment must fail startup,
		// not fall back silently (common.InitLogger downgrades unknown
		// levels to info).
		{"env invalid -> error", "verbose", nil, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := newEnvViper()
			if tc.envVal != "" {
				t.Setenv("RAGFLOW_LOG_LEVEL", tc.envVal)
			}
			c := &Config{}
			if err := c.ParseLogConfig(v); err != nil {
				t.Fatalf("ParseLogConfig: %v", err)
			}
			got, err := c.ResolveLogLevel(tc.cli)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ResolveLogLevel() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveLogLevel() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Fatalf("ResolveLogLevel() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestParseLogConfigReadsEnv pins that an env-only override (no `log:` YAML
// section) reaches the parsed config for every log.* key. viper's v.Sub("log")
// returns nil for an env-only key, so the parser must read each key from the
// root viper (which honours AutomaticEnv) rather than from a Sub view.
func TestParseLogConfigReadsEnv(t *testing.T) {
	v := newEnvViper()
	t.Setenv("RAGFLOW_LOG_LEVEL", "error")
	t.Setenv("RAGFLOW_LOG_PATH", "/var/log/ragflow")
	t.Setenv("RAGFLOW_LOG_MAX_SIZE", "200")

	c := &Config{}
	if err := c.ParseLogConfig(v); err != nil {
		t.Fatalf("ParseLogConfig: %v", err)
	}
	if got := c.GetLogConfig().Level; got != "error" {
		t.Fatalf("Level = %q, want %q (env override dropped?)", got, "error")
	}
	if got := c.GetLogConfig().Path; got != "/var/log/ragflow" {
		t.Fatalf("Path = %q, want %q (env override dropped?)", got, "/var/log/ragflow")
	}
	if got := c.GetLogConfig().MaxSize; got != 200 {
		t.Fatalf("MaxSize = %d, want 200 (env override dropped?)", got)
	}
}
