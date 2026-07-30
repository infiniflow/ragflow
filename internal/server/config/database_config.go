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

// DatabaseConfig database configuration
type DatabaseConfig struct {
	DatabaseType string      `mapstructure:"type"`
	MySQL        MySQLConfig `mapstructure:"mysql"`
}

type MySQLConfig struct {
	DatabaseName     string `mapstructure:"name"` // database name
	User             string `mapstructure:"user"`
	Password         string `mapstructure:"password"`
	Host             string `mapstructure:"host"`
	Port             int    `mapstructure:"port"`
	MaxConnections   int    `mapstructure:"max_connections"`
	StaleTimeout     int    `mapstructure:"stale_timeout"`
	MaxAllowedPacket int    `mapstructure:"max_allowed_packet"`
}

func ParseDatabaseConfig(databaseType string, config *Config, v *viper.Viper) error {
	switch databaseType {
	case "mysql":
		parseMySQLConfig(config, v)
	default:
		return fmt.Errorf("database type %s is not supported", databaseType)
	}
	config.Database.DatabaseType = databaseType
	return nil
}

func parseMySQLConfig(config *Config, v *viper.Viper) {

	// Default MySQL config
	config.Database.MySQL.DatabaseName = "rag_flow"
	config.Database.MySQL.User = "root"
	config.Database.MySQL.Password = "infini_rag_flow"
	config.Database.MySQL.Host = "localhost"
	config.Database.MySQL.Port = 3306
	config.Database.MySQL.MaxConnections = 900
	config.Database.MySQL.StaleTimeout = 300
	config.Database.MySQL.MaxAllowedPacket = 1073741824

	if !v.IsSet("mysql") {
		return
	}
	sub := v.Sub("mysql")
	if sub == nil {
		return
	}

	if sub.IsSet("name") {
		config.Database.MySQL.DatabaseName = sub.GetString("name")
	}

	if sub.IsSet("user") {
		config.Database.MySQL.User = sub.GetString("user")
	}

	if sub.IsSet("password") {
		config.Database.MySQL.Password = sub.GetString("password")
	}

	if sub.IsSet("host") {
		config.Database.MySQL.Host = sub.GetString("host")
	}

	if sub.IsSet("port") {
		config.Database.MySQL.Port = sub.GetInt("port")
	}

	if sub.IsSet("max_connections") {
		config.Database.MySQL.MaxConnections = sub.GetInt("max_connections")
	}

	if sub.IsSet("stale_timeout") {
		config.Database.MySQL.StaleTimeout = sub.GetInt("stale_timeout")
	}

	if sub.IsSet("max_allowed_packet") {
		config.Database.MySQL.MaxAllowedPacket = sub.GetInt("max_allowed_packet")
	}
}
