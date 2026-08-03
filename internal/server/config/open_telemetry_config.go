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

func (c *Config) ParseOpenTelemetryConfig(v *viper.Viper) error {
	// Default OpenTelemetry config
	c.oTel.Host = "localhost"
	c.oTel.Port = 4318
	c.oTel.Secure = false
	c.oTel.SampleRatio = 1.0
	c.oTel.Stdout = false
	c.oTel.Enable = false

	if !v.IsSet("otel") {
		return nil
	}
	sub := v.Sub("otel")
	if sub == nil {
		return nil
	}

	if sub.IsSet("host") {
		c.oTel.Host = sub.GetString("host")
	}

	if sub.IsSet("port") {
		c.oTel.Port = sub.GetInt("port")
	}

	if sub.IsSet("secure") {
		c.oTel.Secure = sub.GetBool("secure")
	}

	if sub.IsSet("sample_ratio") {
		c.oTel.SampleRatio = sub.GetFloat64("sample_ratio")
	}

	if sub.IsSet("stdout") {
		c.oTel.Stdout = sub.GetBool("stdout")
	}

	if sub.IsSet("enable") {
		c.oTel.Enable = sub.GetBool("enable")
	}

	return nil
}

func (c *Config) GetOpenTelemetryConfig() OpenTelemetryConfig {
	return c.oTel
}

func (o OpenTelemetryConfig) ExportConfigs() map[string]interface{} {
	var oTelConfigs map[string]interface{}
	oTelConfigs = make(map[string]interface{})
	oTelConfigs["host"] = o.Host
	oTelConfigs["port"] = o.Port
	oTelConfigs["secure"] = o.Secure
	oTelConfigs["sample_ratio"] = o.SampleRatio
	oTelConfigs["stdout"] = o.Stdout
	oTelConfigs["enable"] = o.Enable
	return oTelConfigs
}
