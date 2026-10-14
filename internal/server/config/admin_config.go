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
	"github.com/spf13/viper"
)

type AdminConfig struct {
	Host     string `mapstructure:"host"`
	HTTPPort int    `mapstructure:"http_port"`
}

func (c *Config) ParseAdminConfig(v *viper.Viper) error {
	// Defaults; overridden from the ROOT viper using the FULL dotted key. The
	// previous guard `if !v.IsSet("admin") { return }` checked the PARENT
	// key, which an env var maps to the child key `admin.http_port`, so the
	// guard was false for a lone env override and the server kept binding
	// 9381. Reading the full key from the root viper (which carries the
	// RAGFLOW prefix/replacer/AutomaticEnv from server.Init) honours both the
	// YAML file and the env layer — including the env-only case. The admin
	// port env var is RAGFLOW_ADMIN_HTTP_PORT (no prefix doubling, unlike the
	// ragflow section), so the root read already consults it.
	c.admin.Host = "localhost"
	c.admin.HTTPPort = 9381

	if v.IsSet("admin.host") {
		c.admin.Host = v.GetString("admin.host")
	}

	if v.IsSet("admin.http_port") {
		c.admin.HTTPPort = v.GetInt("admin.http_port")
	}

	return nil
}

func (c *Config) GetAdminServerConfig() AdminConfig {
	return c.admin
}

// SetAdminHTTPPort stores a resolved admin server port in the global
// config. GetAdminServerConfig returns a copy, so this setter (not a write
// into the returned struct) is how a CLI/env override reaches the config
// the admin server actually binds.
func (c *Config) SetAdminHTTPPort(port int) {
	c.admin.HTTPPort = port
}
