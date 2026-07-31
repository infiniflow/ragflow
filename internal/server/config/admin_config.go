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
	// Default Admin config
	c.admin.Host = "localhost"
	c.admin.HTTPPort = 9383

	if !v.IsSet("admin") {
		return nil
	}
	sub := v.Sub("admin")
	if sub == nil {
		return nil
	}

	if sub.IsSet("host") {
		c.admin.Host = sub.GetString("host")
	}

	if sub.IsSet("http_port") {
		c.admin.HTTPPort = sub.GetInt("http_port")
	}

	if c.admin.HTTPPort == 9381 {
		c.admin.HTTPPort = 9383
	}

	return nil
}

func (c *Config) GetAdminServerConfig() AdminConfig {
	return c.admin
}
