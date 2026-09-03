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

type CacheEngineConfig struct {
	Redis RedisConfig `mapstructure:"redis"`
}

// RedisConfig Redis configuration
type RedisConfig struct {
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
	case "redis":
		err = c.parseRedisConfig(v)
	default:
		return fmt.Errorf("cache engine type %s is not supported", cacheEngineType)
	}

	return err
}

func (c *Config) parseRedisConfig(v *viper.Viper) error {
	// Default Redis config
	c.cacheEngine.Redis.Host = "localhost"
	c.cacheEngine.Redis.Port = 6379
	c.cacheEngine.Redis.DB = 1
	c.cacheEngine.Redis.Username = ""
	c.cacheEngine.Redis.Password = "infini_rag_flow"

	if !v.IsSet("redis") {
		return nil
	}
	sub := v.Sub("redis")
	if sub == nil {
		return nil
	}

	if sub.IsSet("host") {
		hostStr := sub.GetString("host")
		// Handle host:port format (e.g., "localhost:6379")
		host, portStr, err := net.SplitHostPort(hostStr)
		if err != nil {
			return fmt.Errorf("error address format of Redis: %s", hostStr)
		}

		if host == "" {
			return fmt.Errorf("empty host of Redis configuration")
		}
		c.cacheEngine.Redis.Host = host

		if portStr != "" {
			var port int
			if port, err = strconv.Atoi(portStr); err == nil {
				c.cacheEngine.Redis.Port = port
			}
		}
	}

	if sub.IsSet("db") {
		c.cacheEngine.Redis.DB = sub.GetInt("db")
	}

	if sub.IsSet("username") {
		c.cacheEngine.Redis.Username = sub.GetString("username")
	}

	if sub.IsSet("password") {
		c.cacheEngine.Redis.Password = sub.GetString("password")
	}

	return nil
}

func (c *Config) GetRedisConfig() RedisConfig {
	return c.cacheEngine.Redis
}

func (r RedisConfig) ExportConfigs() map[string]interface{} {
	var redisConfigs map[string]interface{}
	redisConfigs = make(map[string]interface{})
	redisConfigs["host"] = r.Host
	redisConfigs["port"] = r.Port
	redisConfigs["username"] = r.Username
	redisConfigs["password"] = r.Password
	redisConfigs["db"] = r.DB
	return redisConfigs
}
