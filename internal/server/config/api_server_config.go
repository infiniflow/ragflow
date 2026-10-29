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
	"fmt"

	"github.com/spf13/viper"
)

// MinPort / MaxPort are the inclusive bounds for a server HTTP port. They
// are shared by the API and admin server modes.
const (
	MinPort = 1
	MaxPort = 65535
)

type AuthenticationConfig struct {
	EnableRegister bool `mapstructure:"enable_register"`
}

type APIServerConfig struct {
	MCP      MCPConfig
	Host     string `mapstructure:"host"`
	HTTPPort int    `mapstructure:"http_port"`
	// TrustedProxies lists the IPs / CIDRs whose X-Forwarded-For and
	// X-Real-IP headers are trusted when resolving the client address.
	// nil means "not configured" and resolves to common.DefaultTrustedProxies
	// (loopback, i.e. the nginx bundled in the ragflow image). An explicit
	// list replaces that default rather than extending it, and an empty
	// list trusts no proxy at all.
	TrustedProxies []string `mapstructure:"trusted_proxies"`

	Authentication AuthenticationConfig `mapstructure:"authentication"`
}

func (c *Config) ParseAPIServerConfig(v *viper.Viper) error {
	if err := c.parseMCPConfig(v); err != nil {
		return err
	}

	// Defaults; each is overridden from the ROOT viper using the FULL dotted
	// key. The previous guard `if !v.IsSet("ragflow") { return }` checked the
	// PARENT key, which an env var maps to the child key, so the guard was false
	// for a lone env override and the server kept binding 9380. Reading the full
	// key from the root viper (which carries the RAGFLOW prefix/replacer/
	// AutomaticEnv from server.Init) honours both the YAML file and the env
	// layer — including the env-only case — and stays consistent with the
	// env-merge fix in ParseLogConfig.
	//
	// The section is named `api:`, so viper's AutomaticEnv derives
	// RAGFLOW_API_HTTP_PORT for the port, mirroring RAGFLOW_ADMIN_HTTP_PORT on
	// the admin section. The legacy `ragflow:` section name collided with the
	// RAGFLOW env prefix and produced a doubled RAGFLOW_RAGFLOW_HTTP_PORT, so no
	// BindEnv is needed and no doubled variable is accepted.
	c.apiServer.Host = "localhost"
	c.apiServer.HTTPPort = 9380

	if v.IsSet("api.host") {
		c.apiServer.Host = v.GetString("api.host")
	}

	if v.IsSet("api.http_port") {
		c.apiServer.HTTPPort = v.GetInt("api.http_port")
	}

	if v.IsSet("api.trusted_proxies") {
		proxies := v.GetStringSlice("api.trusted_proxies")
		if proxies == nil {
			proxies = []string{}
		}
		c.apiServer.TrustedProxies = proxies
	}

	c.parseAuthenticationConfig(v)

	return nil
}

func (c *Config) parseAuthenticationConfig(v *viper.Viper) {
	apiServerConfig := &c.apiServer
	apiServerConfig.Authentication.EnableRegister = true

	if !v.IsSet("authentication") {
		return
	}
	sub := v.Sub("authentication")
	if sub == nil {
		return
	}

	if sub.IsSet("enable_register") {
		apiServerConfig.Authentication.EnableRegister = sub.GetBool("enable_register")
	}
}

func (c *Config) EnableRegister() bool {
	if c.environments.EnableRegister != nil {
		return *c.environments.EnableRegister
	}
	return c.apiServer.Authentication.EnableRegister
}

func (c *Config) GetAPIServerConfig() APIServerConfig {
	return c.apiServer
}

// SetAPIServerHTTPPort stores a resolved API server port in the global
// config. GetAPIServerConfig returns a copy, so this setter (not a write
// into the returned struct) is how a CLI/env override reaches the config
// the API server actually binds.
func (c *Config) SetAPIServerHTTPPort(port int) {
	c.apiServer.HTTPPort = port
}

// ResolvePort resolves the effective HTTP port from the precedence
// CLI flag > the configured port (config file / viper, already merged with
// env) > the built-in default, and validates the final value against
// [MinPort, MaxPort]. The configured port is passed by the caller so the
// SAME validation serves both the API server (apiServer.HTTPPort) and the
// admin server (admin.HTTPPort) — reading a single receiver field here would
// make the admin server resolve to the API port and collide with it at
// startup. The CLI parser still rejects an out-of-range --port up front; this
// guard owns the config/env layers, where a non-integer value used to reach
// viper's GetInt as a silent 0 (the server would then listen on a random
// port).
func (c *Config) ResolvePort(configured int, cli *int) (int, error) {
	port := configured
	if cli != nil {
		port = *cli
	}
	if port < MinPort || port > MaxPort {
		return 0, fmt.Errorf("invalid port %d: must be in [%d, %d] (check --port or the http_port config key and its environment override)",
			port, MinPort, MaxPort)
	}
	return port, nil
}
