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
	"time"

	"github.com/spf13/viper"
)

// GeneralConfig general configuration
type GeneralConfig struct {
	HeartbeatInterval time.Duration `mapstructure:"heartbeat_interval"`
	Mode              string        `mapstructure:"mode"` // debug, release
	SecretKey         string        `mapstructure:"secret_key"`
	Database          string        `mapstructure:"database"`
	DocEngine         string        `mapstructure:"doc_engine"`      // Infinity, Elasticsearch
	StorageEngine     string        `mapstructure:"storage_engine"`  // Minio, S3
	CacheEngine       string        `mapstructure:"cache_engine"`    // Redis
	QueueEngine       string        `mapstructure:"queue_engine"`    // NATS
	AnalyticEngine    string        `mapstructure:"analytic_engine"` // Clickhouse
	Language          string        `mapstructure:"language"`
}

func ParseGeneralConfig(config *Config, v *viper.Viper) error {

	// Default General config
	config.General.HeartbeatInterval = 3
	config.General.Mode = "release"
	config.General.SecretKey = ""
	config.General.Database = "mysql"
	config.General.DocEngine = "elasticsearch"
	config.General.StorageEngine = "minio"
	config.General.CacheEngine = "redis"
	config.General.QueueEngine = "nats"
	config.General.AnalyticEngine = "clickhouse"
	config.General.Language = "english"

	if !v.IsSet("general") {
		return nil
	}
	sub := v.Sub("general")
	if sub == nil {
		return nil
	}

	if sub.IsSet("heartbeat_interval") {
		config.General.HeartbeatInterval = sub.GetDuration("heartbeat_interval")
	}
	if sub.IsSet("mode") {
		config.General.Mode = sub.GetString("mode")
	}
	if sub.IsSet("secret_key") {
		config.General.SecretKey = sub.GetString("secret_key")
	}
	if sub.IsSet("database") {
		config.General.Database = sub.GetString("database")
	}
	if sub.IsSet("doc_engine") {
		config.General.DocEngine = sub.GetString("doc_engine")
	}
	if sub.IsSet("storage_engine") {
		config.General.StorageEngine = sub.GetString("storage_engine")
	}
	if sub.IsSet("cache_engine") {
		config.General.CacheEngine = sub.GetString("cache_engine")
	}
	if sub.IsSet("queue_engine") {
		config.General.QueueEngine = sub.GetString("queue_engine")
	}
	if sub.IsSet("analytic_engine") {
		config.General.AnalyticEngine = sub.GetString("analytic_engine")
	}
	if sub.IsSet("language") {
		config.General.Language = sub.GetString("language")
	}
	return nil
}
