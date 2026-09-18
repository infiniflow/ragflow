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
	"net"
	"strconv"

	"github.com/spf13/viper"
)

// CacheEngineConfig holds the Go services' cache/queue backend settings. The Go
// stack always talks to Kvrocks (a RocksDB-backed, Redis-protocol store) so it
// is not subject to the in-memory maxmemory cap that Valkey/Redis enforces.
type CacheEngineConfig struct {
	Kvrocks KvrocksConfig `mapstructure:"kvrocks"`
}

// KvrocksConfig connection settings for the Kvrocks backend.
type KvrocksConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

func (c *Config) ParseCacheEngineConfig(v *viper.Viper) error {
	cacheEngineType := c.general.CacheEngine
	var err error
	switch cacheEngineType {
	// The Go stack connects to Kvrocks. "redis" is accepted for backwards
	// compatibility with the shared service_conf.yaml.template (its `redis:`
	// section still drives the Python/Valkey path); both map to Kvrocks here.
	case "redis", "kvrocks":
		err = c.parseKvrocksConfig(v)
	default:
		return fmt.Errorf("cache engine type %s is not supported", cacheEngineType)
	}

	return err
}

func (c *Config) parseKvrocksConfig(v *viper.Viper) error {
	// Sensible defaults; deployed values come from the `kvrocks` section.
	c.cacheEngine.Kvrocks.Host = "localhost"
	c.cacheEngine.Kvrocks.Port = 6379
	c.cacheEngine.Kvrocks.DB = 1
	c.cacheEngine.Kvrocks.Username = ""
	c.cacheEngine.Kvrocks.Password = "infini_rag_flow"

	if !v.IsSet("kvrocks") {
		return nil
	}
	sub := v.Sub("kvrocks")
	if sub == nil {
		return nil
	}

	if sub.IsSet("host") {
		hostStr := sub.GetString("host")
		// Handle host:port format (e.g., "localhost:6379")
		host, portStr, err := net.SplitHostPort(hostStr)
		if err != nil {
			return fmt.Errorf("error address format of Kvrocks: %s", hostStr)
		}

		if host == "" {
			return fmt.Errorf("empty host of Kvrocks configuration")
		}
		c.cacheEngine.Kvrocks.Host = host

		if portStr != "" {
			var port int
			if port, err = strconv.Atoi(portStr); err == nil {
				c.cacheEngine.Kvrocks.Port = port
			}
		}
	}

	if sub.IsSet("db") {
		c.cacheEngine.Kvrocks.DB = sub.GetInt("db")
	}

	if sub.IsSet("username") {
		c.cacheEngine.Kvrocks.Username = sub.GetString("username")
	}

	if sub.IsSet("password") {
		c.cacheEngine.Kvrocks.Password = sub.GetString("password")
	}

	return nil
}

func (c *Config) GetKvrocksConfig() KvrocksConfig {
	return c.cacheEngine.Kvrocks
}

func (r KvrocksConfig) ExportConfigs() map[string]interface{} {
	kvrocksConfigs := make(map[string]interface{})
	kvrocksConfigs["host"] = r.Host
	kvrocksConfigs["port"] = r.Port
	kvrocksConfigs["username"] = r.Username
	kvrocksConfigs["password"] = r.Password
	kvrocksConfigs["db"] = r.DB
	return kvrocksConfigs
}
