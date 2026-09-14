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

import "github.com/spf13/viper"

type AuthenticationConfig struct {
	DisablePasswordLogin bool `mapstructure:"disable_password_login"`
	EnableRegister       bool `mapstructure:"enable_register"`
}

type APIServerConfig struct {
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
	// Default Admin config
	c.apiServer.Host = "localhost"
	c.apiServer.HTTPPort = 9384

	if !v.IsSet("ragflow") {
		return nil
	}
	sub := v.Sub("ragflow")
	if sub == nil {
		return nil
	}

	if sub.IsSet("host") {
		c.apiServer.Host = sub.GetString("host")
	}

	if sub.IsSet("http_port") {
		c.apiServer.HTTPPort = sub.GetInt("http_port")
	}

	if c.apiServer.HTTPPort == 9380 {
		c.apiServer.HTTPPort = 9384
	}

	if sub.IsSet("trusted_proxies") {
		proxies := sub.GetStringSlice("trusted_proxies")
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
	apiServerConfig.Authentication.DisablePasswordLogin = false
	apiServerConfig.Authentication.EnableRegister = true

	if !v.IsSet("authentication") {
		return
	}
	sub := v.Sub("authentication")
	if sub == nil {
		return
	}

	if sub.IsSet("disable_password_login") {
		apiServerConfig.Authentication.DisablePasswordLogin = sub.GetBool("disable_password_login")
	}

	if sub.IsSet("enable_register") {
		apiServerConfig.Authentication.EnableRegister = sub.GetBool("enable_register")
	}
}

func (c *Config) DisablePasswordLogin() bool {
	if c.environments.DisablePasswordLogin != nil {
		return *c.environments.DisablePasswordLogin
	}
	return c.apiServer.Authentication.DisablePasswordLogin
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
