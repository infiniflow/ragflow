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
	EngineType string           `mapstructure:"type"`
	Clickhouse ClickhouseConfig `mapstructure:"clickhouse"`
}

type ClickhouseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
}

func ParseAnalyticEngineConfig(analyticEngineType string, config *Config, v *viper.Viper) error {
	var err error
	switch analyticEngineType {
	case "clickhouse":
		err = parseClickhouseConfig(config, v)
	default:
		return fmt.Errorf("analytic engine type %s is not supported", analyticEngineType)
	}
	if err == nil {
		config.AnalyticEngine.EngineType = analyticEngineType
	}

	return err
}

func parseClickhouseConfig(config *Config, v *viper.Viper) error {
	// Default Clickhouse config
	config.AnalyticEngine.Clickhouse.Host = "localhost"
	config.AnalyticEngine.Clickhouse.Port = 9900
	config.AnalyticEngine.Clickhouse.User = "ragflow"
	config.AnalyticEngine.Clickhouse.Password = "infini_rag_flow"
	config.AnalyticEngine.Clickhouse.Database = "ragflow"

	if !v.IsSet("clickhouse") {
		return nil
	}
	sub := v.Sub("clickhouse")
	if sub == nil {
		return nil
	}

	if sub.IsSet("host") {
		config.AnalyticEngine.Clickhouse.Host = sub.GetString("host")
	}

	if sub.IsSet("port") {
		config.AnalyticEngine.Clickhouse.Port = sub.GetInt("port")
	}

	if sub.IsSet("user") {
		config.AnalyticEngine.Clickhouse.User = sub.GetString("user")
	}
	if sub.IsSet("password") {
		config.AnalyticEngine.Clickhouse.Password = sub.GetString("password")
	}
	if sub.IsSet("database") {
		config.AnalyticEngine.Clickhouse.Database = sub.GetString("database")
	}
	return nil
}
