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

type AnalyticEngineConfig struct {
	Clickhouse ClickhouseConfig `mapstructure:"clickhouse"`
}

type ClickhouseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
}

func (c *Config) ParseAnalyticEngineConfig(v *viper.Viper) error {
	analyticEngineType := c.general.AnalyticEngine
	var err error
	switch analyticEngineType {
	case "clickhouse":
		err = c.parseClickhouseConfig(v)
	default:
		return fmt.Errorf("analytic engine type %s is not supported", analyticEngineType)
	}

	return err
}

func (c *Config) parseClickhouseConfig(v *viper.Viper) error {
	// Default Clickhouse config
	c.analyticEngine.Clickhouse.Host = "localhost"
	c.analyticEngine.Clickhouse.Port = 9900
	c.analyticEngine.Clickhouse.User = "ragflow"
	c.analyticEngine.Clickhouse.Password = "infini_rag_flow"
	c.analyticEngine.Clickhouse.Database = "ragflow"

	if !v.IsSet("clickhouse") {
		return nil
	}
	sub := v.Sub("clickhouse")
	if sub == nil {
		return nil
	}

	if sub.IsSet("host") {
		c.analyticEngine.Clickhouse.Host = sub.GetString("host")
	}

	if sub.IsSet("port") {
		c.analyticEngine.Clickhouse.Port = sub.GetInt("port")
	}

	if sub.IsSet("user") {
		c.analyticEngine.Clickhouse.User = sub.GetString("user")
	}
	if sub.IsSet("password") {
		c.analyticEngine.Clickhouse.Password = sub.GetString("password")
	}
	if sub.IsSet("database") {
		c.analyticEngine.Clickhouse.Database = sub.GetString("database")
	}
	return nil
}

func (c *Config) GetClickhouseConfig() ClickhouseConfig {
	return c.analyticEngine.Clickhouse
}

func (c *ClickhouseConfig) ExportConfigs() map[string]interface{} {
	var clickhouseConfigs map[string]interface{}
	clickhouseConfigs = make(map[string]interface{})
	clickhouseConfigs["host"] = c.Host
	clickhouseConfigs["port"] = c.Port
	clickhouseConfigs["user"] = c.User
	clickhouseConfigs["password"] = c.Password
	clickhouseConfigs["database"] = c.Database
	return clickhouseConfigs
}
