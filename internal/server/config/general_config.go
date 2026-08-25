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
	Database          string        `mapstructure:"database"`
	DocEngine         string        `mapstructure:"doc_engine"`      // Infinity, Elasticsearch
	StorageEngine     string        `mapstructure:"storage_engine"`  // Minio, S3
	CacheEngine       string        `mapstructure:"cache_engine"`    // Redis
	QueueEngine       string        `mapstructure:"queue_engine"`    // NATS
	AnalyticEngine    string        `mapstructure:"analytic_engine"` // Clickhouse
	Language          string        `mapstructure:"language"`
}

func (c *Config) ParseGeneralConfig(v *viper.Viper) error {

	// Default General config
	c.general.HeartbeatInterval = 3 * time.Second
	c.general.Mode = "release"
	c.general.Database = "mysql"
	c.general.DocEngine = "elasticsearch"
	c.general.StorageEngine = "minio"
	c.general.CacheEngine = "redis"
	c.general.QueueEngine = "nats"
	c.general.AnalyticEngine = "clickhouse"
	c.general.Language = "en"

	if !v.IsSet("general") {
		return nil
	}
	sub := v.Sub("general")
	if sub == nil {
		return nil
	}

	if sub.IsSet("heartbeat_interval") {
		c.general.HeartbeatInterval = sub.GetDuration("heartbeat_interval")
	}
	if sub.IsSet("mode") {
		c.general.Mode = sub.GetString("mode")
	}
	if sub.IsSet("database") {
		c.general.Database = sub.GetString("database")
	}
	if sub.IsSet("doc_engine") {
		c.general.DocEngine = sub.GetString("doc_engine")
	}
	if sub.IsSet("storage_engine") {
		c.general.StorageEngine = sub.GetString("storage_engine")
	}
	if sub.IsSet("cache_engine") {
		c.general.CacheEngine = sub.GetString("cache_engine")
	}
	if sub.IsSet("queue_engine") {
		c.general.QueueEngine = sub.GetString("queue_engine")
	}
	if sub.IsSet("analytic_engine") {
		c.general.AnalyticEngine = sub.GetString("analytic_engine")
	}
	if sub.IsSet("language") {
		c.general.Language = sub.GetString("language")
	}
	return nil
}

func (c *Config) GetHeartbeatInterval() time.Duration {
	return c.general.HeartbeatInterval
}

func (c *Config) GetMode() string {
	return c.general.Mode
}

func (c *Config) DatabaseType() string {
	if c.environments.DatabaseType != "" {
		return c.environments.DatabaseType
	}
	return c.general.Database
}

func (c *Config) DocEngineType() string {
	if c.environments.DocumentEngineType != "" {
		return c.environments.DocumentEngineType
	}
	return c.general.DocEngine
}

func (c *Config) StorageEngineType() string {
	return c.general.StorageEngine
}

func (c *Config) CacheEngineType() string {
	return c.general.CacheEngine
}

func (c *Config) QueueEngineType() string {
	return c.general.QueueEngine
}

func (c *Config) AnalyticEngineType() string {
	return c.general.AnalyticEngine
}

func (c *Config) Language() string {
	if c.environments.Language != "" {
		return c.environments.Language
	}
	return c.general.Language
}
