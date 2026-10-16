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
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestParseAPIServerConfigTrustedProxies(t *testing.T) {
	for _, test := range []struct {
		name   string
		values map[string]interface{}
		want   []string
	}{
		// nil means "not configured": the engine falls back to the loopback default.
		{name: "unset", values: map[string]interface{}{"http_port": 9380}, want: nil},
		{name: "explicit list", values: map[string]interface{}{"trusted_proxies": []string{"10.0.0.0/8", "192.168.1.5"}}, want: []string{"10.0.0.0/8", "192.168.1.5"}},
		// An explicit empty list must survive as empty (trust nobody), not collapse to nil.
		{name: "explicit empty", values: map[string]interface{}{"trusted_proxies": []string{}}, want: []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			v := viper.New()
			v.Set("api", test.values)
			cfg := &Config{}
			if err := cfg.ParseAPIServerConfig(v); err != nil {
				t.Fatalf("ParseAPIServerConfig: %v", err)
			}
			if got := cfg.GetAPIServerConfig().TrustedProxies; !reflect.DeepEqual(got, test.want) {
				t.Fatalf("TrustedProxies = %#v, want %#v", got, test.want)
			}
		})
	}
}

// TestResolvePort pins the precedence CLI flag > config file (viper,
// already merged with env) > built-in default and the fail-fast range
// guard [1, 65535] on the FINAL value. The config/env layers used to go
// unvalidated: a non-integer RAGFLOW_*_HTTP_PORT silently became 0 via
// viper's GetInt (listening on a random port), and an out-of-range YAML
// value only failed at ListenAndServe. The CLI parser still rejects an
// out-of-range --port up front, so a bad CLI value fails with a message
// naming the flag; this guard owns the config/env layers.
func TestResolvePort(t *testing.T) {
	cases := []struct {
		name       string
		configured int
		cli        *int
		want       int
		wantErr    bool
	}{
		{"config default", 9380, nil, 9380, false},
		{"config custom", 9443, nil, 9443, false},
		{"cli overrides config", 9380, intPtr(8080), 8080, false},
		{"cli lower boundary", 9380, intPtr(1), 1, false},
		{"cli upper boundary", 9380, intPtr(65535), 65535, false},
		// An explicit zero (e.g. a non-integer env value read as 0 by
		// viper) or an out-of-range value must fail startup.
		{"configured zero -> error", 0, nil, 0, true},
		{"configured above max -> error", 99999, nil, 0, true},
		{"configured negative -> error", -1, nil, 0, true},
		{"cli zero -> error", 9380, intPtr(0), 0, true},
		{"cli above max -> error", 9380, intPtr(65536), 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{}
			c.SetAPIServerHTTPPort(tc.configured)
			got, err := c.ResolvePort(tc.configured, tc.cli)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ResolvePort(%d, %v) = %d, want error", tc.configured, tc.cli, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolvePort(%d, %v) error = %v, want nil", tc.configured, tc.cli, err)
			}
			if got != tc.want {
				t.Fatalf("ResolvePort(%d, %v) = %d, want %d", tc.configured, tc.cli, got, tc.want)
			}
		})
	}
}

// TestResolvePortHonorsServerSpecificConfig pins that ResolvePort validates
// the port passed to it rather than always reading the API server's port from
// the receiver. A regression made ResolvePort read c.apiServer.HTTPPort for
// every caller, so the admin server (whose configured port is 9381) resolved
// to 9380 and collided with the API server at startup (CI: "address already in
// use :9380"). With api=9380 and admin=9381 set independently, resolving the
// admin's configured port must yield 9381, not 9380.
func TestResolvePortHonorsServerSpecificConfig(t *testing.T) {
	c := &Config{}
	c.SetAPIServerHTTPPort(9380)
	c.SetAdminHTTPPort(9381)

	// Admin path: caller passes the admin-configured port.
	got, err := c.ResolvePort(c.GetAdminServerConfig().HTTPPort, nil)
	if err != nil {
		t.Fatalf("ResolvePort(admin) error = %v, want nil", err)
	}
	if got != 9381 {
		t.Fatalf("ResolvePort(admin configured) = %d, want 9381 (must not read the API port)", got)
	}

	// API path: caller passes the api-configured port.
	got, err = c.ResolvePort(c.GetAPIServerConfig().HTTPPort, nil)
	if err != nil {
		t.Fatalf("ResolvePort(api) error = %v, want nil", err)
	}
	if got != 9380 {
		t.Fatalf("ResolvePort(api configured) = %d, want 9380", got)
	}

	// CLI override must still win regardless of which server calls it.
	got, err = c.ResolvePort(c.GetAdminServerConfig().HTTPPort, intPtr(9400))
	if err != nil {
		t.Fatalf("ResolvePort(admin, --port) error = %v, want nil", err)
	}
	if got != 9400 {
		t.Fatalf("ResolvePort(admin, --port) = %d, want 9400 (CLI must override)", got)
	}
}

// TestPortSetterRoundTrip pins the supported write path for a resolved
// HTTP port. GetAPIServerConfig / GetAdminServerConfig return COPIES, so
// writing a --port override into the returned struct (as main used to do)
// silently dropped the override: the server later re-read the port from
// the global config and bound the unmodified value. The setters are the
// only way a resolved port reaches the config the servers actually read.
func TestPortSetterRoundTrip(t *testing.T) {
	c := &Config{}
	if err := c.ParseAPIServerConfig(viper.New()); err != nil {
		t.Fatalf("ParseAPIServerConfig: %v", err)
	}
	if err := c.ParseAdminConfig(viper.New()); err != nil {
		t.Fatalf("ParseAdminConfig: %v", err)
	}
	c.SetAPIServerHTTPPort(8080)
	c.SetAdminHTTPPort(9443)
	if got := c.GetAPIServerConfig().HTTPPort; got != 8080 {
		t.Fatalf("API server HTTPPort = %d, want 8080", got)
	}
	if got := c.GetAdminServerConfig().HTTPPort; got != 9443 {
		t.Fatalf("admin server HTTPPort = %d, want 9443", got)
	}
}

// TestParseAPIServerConfigReadsEnvOnlyPort pins that a lone RAGFLOW_API_HTTP_PORT
// (no `api:` YAML section) still reaches the parsed config. The previous guard
// `if !v.IsSet("ragflow") { return nil }` checked the PARENT key, which an env
// var maps to the child key, so the guard was false and the env override was
// silently dropped (the server kept binding 9380). Reading the full dotted key
// from the root viper (which carries the RAGFLOW prefix/replacer/AutomaticEnv
// from server.Init) honours the env layer in the env-only case.
//
// The section is named `api:`, so viper derives RAGFLOW_API_HTTP_PORT directly.
// The legacy `ragflow:` section name collided with the RAGFLOW env prefix and
// made AutomaticEnv resolve the key to a doubled RAGFLOW_RAGFLOW_HTTP_PORT.
func TestParseAPIServerConfigReadsEnvOnlyPort(t *testing.T) {
	v := viper.New()
	v.SetEnvPrefix("RAGFLOW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	t.Setenv("RAGFLOW_API_HTTP_PORT", "9399")

	c := &Config{}
	if err := c.ParseAPIServerConfig(v); err != nil {
		t.Fatalf("ParseAPIServerConfig: %v", err)
	}
	if got := c.GetAPIServerConfig().HTTPPort; got != 9399 {
		t.Fatalf("env-only RAGFLOW_API_HTTP_PORT = %d, want 9399 (env layer must reach the config)", got)
	}
}

// TestParseAPIServerConfigReadsAPISection pins that the `api:` YAML section
// reaches the parsed config for host / http_port / trusted_proxies. The section
// was renamed from the legacy `ragflow:` section, whose name collided with the
// RAGFLOW env prefix.
func TestParseAPIServerConfigReadsAPISection(t *testing.T) {
	v := viper.New()
	v.Set("api", map[string]interface{}{
		"host":            "0.0.0.0",
		"http_port":       9443,
		"trusted_proxies": []string{"10.0.0.0/8"},
	})

	c := &Config{}
	if err := c.ParseAPIServerConfig(v); err != nil {
		t.Fatalf("ParseAPIServerConfig: %v", err)
	}
	got := c.GetAPIServerConfig()
	if got.Host != "0.0.0.0" {
		t.Fatalf("Host = %q, want %q", got.Host, "0.0.0.0")
	}
	if got.HTTPPort != 9443 {
		t.Fatalf("HTTPPort = %d, want 9443", got.HTTPPort)
	}
	if !reflect.DeepEqual(got.TrustedProxies, []string{"10.0.0.0/8"}) {
		t.Fatalf("TrustedProxies = %#v, want [10.0.0.0/8]", got.TrustedProxies)
	}
}

// TestParseAPIServerConfigIgnoresRagflowSection pins the hard rename: the
// legacy `ragflow:` section (and the doubled RAGFLOW_RAGFLOW_HTTP_PORT env var
// it produced) is no longer read. A config that still writes `ragflow.http_port`
// must fall back to the built-in default (9380) rather than silently binding a
// stale value. The project has not launched yet, so no compatibility shim is
// kept for the old section name.
func TestParseAPIServerConfigIgnoresRagflowSection(t *testing.T) {
	v := viper.New()
	v.Set("ragflow", map[string]interface{}{"http_port": 9999})

	c := &Config{}
	if err := c.ParseAPIServerConfig(v); err != nil {
		t.Fatalf("ParseAPIServerConfig: %v", err)
	}
	if got := c.GetAPIServerConfig().HTTPPort; got != 9380 {
		t.Fatalf("HTTPPort = %d, want 9380 (legacy ragflow: section must be ignored)", got)
	}
}

// TestParseAdminConfigReadsEnvOnlyPort mirrors the above for the admin server:
// a lone RAGFLOW_ADMIN_HTTP_PORT (no `admin:` YAML section) must reach the
// parsed config instead of being silently dropped by the parent IsSet guard.
// The admin port env var is RAGFLOW_ADMIN_HTTP_PORT (no prefix doubling), so
// the root read already consults it.
func TestParseAdminConfigReadsEnvOnlyPort(t *testing.T) {
	v := viper.New()
	v.SetEnvPrefix("RAGFLOW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	t.Setenv("RAGFLOW_ADMIN_HTTP_PORT", "9401")

	c := &Config{}
	if err := c.ParseAdminConfig(v); err != nil {
		t.Fatalf("ParseAdminConfig: %v", err)
	}
	if got := c.GetAdminServerConfig().HTTPPort; got != 9401 {
		t.Fatalf("env-only RAGFLOW_ADMIN_HTTP_PORT = %d, want 9401 (env layer must reach the config)", got)
	}
}
