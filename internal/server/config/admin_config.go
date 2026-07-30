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

func ParseAdminConfig(config *Config, v *viper.Viper) error {
	// Default Admin config
	config.Admin.Host = "localhost"
	config.Admin.HTTPPort = 9383

	if !v.IsSet("admin") {
		return nil
	}
	sub := v.Sub("admin")
	if sub == nil {
		return nil
	}

	if sub.IsSet("host") {
		config.Admin.Host = sub.GetString("host")
	}

	if sub.IsSet("http_port") {
		config.Admin.HTTPPort = sub.GetInt("http_port")
	}

	if config.Admin.HTTPPort == 9381 {
		config.Admin.HTTPPort = 9383
	}

	return nil
}
