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

type OpenTelemetryConfig struct {
	Host        string  `mapstructure:"host"`
	Port        int     `mapstructure:"port"`
	Secure      bool    `mapstructure:"secure"`
	SampleRatio float64 `mapstructure:"sample_ratio"`
	Stdout      bool    `mapstructure:"stdout"`
	Enable      bool    `mapstructure:"enable"`
}

func ParseOpenTelemetryConfig(config *Config, v *viper.Viper) error {
	// Default OpenTelemetry config
	config.OTel.Host = "localhost"
	config.OTel.Port = 4318
	config.OTel.Secure = false
	config.OTel.SampleRatio = 1.0
	config.OTel.Stdout = false
	config.OTel.Enable = false

	if !v.IsSet("otel") {
		return nil
	}
	sub := v.Sub("otel")
	if sub == nil {
		return nil
	}

	if sub.IsSet("host") {
		config.OTel.Host = sub.GetString("host")
	}

	if sub.IsSet("port") {
		config.OTel.Port = sub.GetInt("port")
	}

	if sub.IsSet("secure") {
		config.OTel.Secure = sub.GetBool("secure")
	}

	if sub.IsSet("sample_ratio") {
		config.OTel.SampleRatio = sub.GetFloat64("sample_ratio")
	}

	if sub.IsSet("stdout") {
		config.OTel.Stdout = sub.GetBool("stdout")
	}

	if sub.IsSet("enable") {
		config.OTel.Enable = sub.GetBool("enable")
	}

	return nil
}
