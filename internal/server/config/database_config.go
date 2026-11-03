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
	"ragflow/internal/common"

	"github.com/spf13/viper"
)

// DatabaseConfig database configuration
type DatabaseConfig struct {
	MySQL    MySQLConfig        `mapstructure:"mysql"`
	Vastbase VastbaseMetaConfig `mapstructure:"vastbase"`
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
	Charset          string `mapstructure:"charset"`
}

// VastbaseMetaConfig is the Vastbase G100 metadata database configuration. It
// reads the same `vastbase:` section the document engine reads: one instance
// serves both the vector store (db_name) and the business metadata (name), so
// host, port, user, password and ssl_mode are shared keys. Only `name` and the
// pool knobs below are metadata-specific.
type VastbaseMetaConfig struct {
	DatabaseName   string `mapstructure:"name"` // metadata database name
	User           string `mapstructure:"user"`
	Password       string `mapstructure:"password"`
	Host           string `mapstructure:"host"`
	Port           int    `mapstructure:"port"`
	MaxConnections int    `mapstructure:"max_connections"`
	StaleTimeout   int    `mapstructure:"stale_timeout"`
	SSLMode        string `mapstructure:"ssl_mode"`
	// DisableReturning suppresses the INSERT/UPDATE/DELETE RETURNING clause
	// the postgres dialect adds, mirroring peewee's returning_clause=False on
	// the Python side. Vastbase B mode rejects RETURNING combined with
	// ON CONFLICT; the default keeps plain RETURNING (needed to read back
	// auto-increment ids) and only flips this when a deployment finds its
	// Vastbase build rejects plain RETURNING too.
	DisableReturning bool `mapstructure:"disable_returning"`
}

func (c *Config) ParseDatabaseConfig(v *viper.Viper) error {
	databaseType := c.general.Database
	if envType := common.GetEnvSmall(common.EnvDBType); envType != "" {
		databaseType = envType
	}
	switch databaseType {
	case "mysql":
		c.parseMySQLConfig(v)
	case "oceanbase":
		c.parseOceanBaseDatabaseConfig(v)
	case "vastbase":
		c.parseVastbaseDatabaseConfig(v)
	default:
		return fmt.Errorf("database type %s is not supported", databaseType)
	}
	return nil
}

func (c *Config) parseOceanBaseDatabaseConfig(v *viper.Viper) {
	// OceanBase uses the MySQL wire protocol, so the existing DAO can keep
	// consuming MySQLConfig. Support both the flat DB_TYPE=oceanbase section and
	// the document-engine nested config.
	c.parseMySQLConfig(v)
	sub := v.Sub("oceanbase")
	if sub == nil {
		return
	}
	if nested := sub.Sub("config"); nested != nil {
		if nested.IsSet("db_name") {
			c.database.MySQL.DatabaseName = nested.GetString("db_name")
		}
		sub = nested
	}
	if sub.IsSet("name") {
		c.database.MySQL.DatabaseName = sub.GetString("name")
	}
	if sub.IsSet("db_name") {
		c.database.MySQL.DatabaseName = sub.GetString("db_name")
	}
	if sub.IsSet("user") {
		c.database.MySQL.User = sub.GetString("user")
	}
	if sub.IsSet("password") {
		c.database.MySQL.Password = sub.GetString("password")
	}
	if sub.IsSet("host") {
		c.database.MySQL.Host = sub.GetString("host")
	}
	if sub.IsSet("port") {
		c.database.MySQL.Port = sub.GetInt("port")
	}
	if sub.IsSet("max_connections") {
		c.database.MySQL.MaxConnections = sub.GetInt("max_connections")
	}
}

// parseVastbaseDatabaseConfig reads the metadata-facing keys of the shared
// `vastbase:` section. Fully qualified keys rather than v.Sub("vastbase") for
// the same reason as parseVastbaseConfig: Sub drops the root AutomaticEnv, so
// an env-only deployment would silently keep every default.
func (c *Config) parseVastbaseDatabaseConfig(v *viper.Viper) {
	c.database.Vastbase.DatabaseName = "rag_flow"
	c.database.Vastbase.Host = "vastbase"
	c.database.Vastbase.Port = 5432
	c.database.Vastbase.User = "ragflow"
	c.database.Vastbase.Password = "Infini_Rag@123"
	c.database.Vastbase.MaxConnections = 50
	c.database.Vastbase.StaleTimeout = 300

	if v.IsSet("vastbase.name") {
		c.database.Vastbase.DatabaseName = v.GetString("vastbase.name")
	}
	if v.IsSet("vastbase.host") {
		c.database.Vastbase.Host = v.GetString("vastbase.host")
	}
	if v.IsSet("vastbase.port") {
		c.database.Vastbase.Port = v.GetInt("vastbase.port")
	}
	if v.IsSet("vastbase.user") {
		c.database.Vastbase.User = v.GetString("vastbase.user")
	}
	if v.IsSet("vastbase.password") {
		c.database.Vastbase.Password = v.GetString("vastbase.password")
	}
	if v.IsSet("vastbase.max_connections") {
		c.database.Vastbase.MaxConnections = v.GetInt("vastbase.max_connections")
	}
	if v.IsSet("vastbase.stale_timeout") {
		c.database.Vastbase.StaleTimeout = v.GetInt("vastbase.stale_timeout")
	}
	if v.IsSet("vastbase.ssl_mode") {
		c.database.Vastbase.SSLMode = v.GetString("vastbase.ssl_mode")
	}
	if v.IsSet("vastbase.disable_returning") {
		c.database.Vastbase.DisableReturning = v.GetBool("vastbase.disable_returning")
	}
}

func (c *Config) parseMySQLConfig(v *viper.Viper) {
	// Default MySQL config
	c.database.MySQL.DatabaseName = "rag_flow"
	c.database.MySQL.User = "root"
	c.database.MySQL.Password = "infini_rag_flow"
	c.database.MySQL.Host = "localhost"
	c.database.MySQL.Port = 3306
	c.database.MySQL.MaxConnections = 900
	c.database.MySQL.StaleTimeout = 300
	c.database.MySQL.MaxAllowedPacket = 1073741824
	c.database.MySQL.Charset = "utf8mb4"

	if !v.IsSet("mysql") {
		return
	}
	sub := v.Sub("mysql")
	if sub == nil {
		return
	}

	if sub.IsSet("name") {
		c.database.MySQL.DatabaseName = sub.GetString("name")
	}

	if sub.IsSet("user") {
		c.database.MySQL.User = sub.GetString("user")
	}

	if sub.IsSet("password") {
		c.database.MySQL.Password = sub.GetString("password")
	}

	if sub.IsSet("host") {
		c.database.MySQL.Host = sub.GetString("host")
	}

	if sub.IsSet("port") {
		c.database.MySQL.Port = sub.GetInt("port")
	}

	if sub.IsSet("max_connections") {
		c.database.MySQL.MaxConnections = sub.GetInt("max_connections")
	}

	if sub.IsSet("stale_timeout") {
		c.database.MySQL.StaleTimeout = sub.GetInt("stale_timeout")
	}

	if sub.IsSet("max_allowed_packet") {
		c.database.MySQL.MaxAllowedPacket = sub.GetInt("max_allowed_packet")
	}

	if sub.IsSet("charset") {
		c.database.MySQL.Charset = sub.GetString("charset")
	}
}

func (c *Config) GetMySQLConfig() MySQLConfig {
	return c.database.MySQL
}

// GetVastbaseMetaConfig returns the Vastbase metadata database configuration.
func (c *Config) GetVastbaseMetaConfig() VastbaseMetaConfig {
	return c.database.Vastbase
}

func (m *VastbaseMetaConfig) ExportConfigs() map[string]interface{} {
	vastbaseConfigs := make(map[string]interface{})
	vastbaseConfigs["database_name"] = m.DatabaseName
	vastbaseConfigs["username"] = m.User
	vastbaseConfigs["password"] = m.Password
	vastbaseConfigs["host"] = m.Host
	vastbaseConfigs["port"] = m.Port
	vastbaseConfigs["ssl_mode"] = m.SSLMode
	vastbaseConfigs["max_connections"] = m.MaxConnections
	vastbaseConfigs["stale_timeout"] = m.StaleTimeout
	return vastbaseConfigs
}

func (m *MySQLConfig) ExportConfigs() map[string]interface{} {
	var mysqlConfigs map[string]interface{}
	mysqlConfigs = make(map[string]interface{})
	mysqlConfigs["host"] = m.Host
	mysqlConfigs["port"] = m.Port
	mysqlConfigs["database_name"] = m.DatabaseName
	mysqlConfigs["username"] = m.User
	mysqlConfigs["password"] = m.Password
	mysqlConfigs["charset"] = m.Charset
	mysqlConfigs["max_connections"] = m.MaxConnections
	mysqlConfigs["stale_timeout"] = m.StaleTimeout
	mysqlConfigs["max_allowed_packet"] = m.MaxAllowedPacket
	return mysqlConfigs
}
