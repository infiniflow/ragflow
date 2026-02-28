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
	"os"
	"strconv"
	"strings"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// Config application configuration
type Config struct {
	Server          ServerConfig           `mapstructure:"server"`
	Database        DatabaseConfig         `mapstructure:"database"`
	Log             LogConfig              `mapstructure:"log"`
	DocEngine       DocEngineConfig        `mapstructure:"doc_engine"`
	RegisterEnabled int                    `mapstructure:"register_enabled"`
	OAuth           map[string]OAuthConfig `mapstructure:"oauth"`
}

// OAuthConfig OAuth configuration for a channel
type OAuthConfig struct {
	DisplayName string `mapstructure:"display_name"`
	Icon        string `mapstructure:"icon"`
}

// ServerConfig server configuration
type ServerConfig struct {
	Mode string `mapstructure:"mode"` // debug, release
	Port int    `mapstructure:"port"`
}

// DatabaseConfig database configuration
type DatabaseConfig struct {
	Driver   string `mapstructure:"driver"` // mysql
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Database string `mapstructure:"database"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Charset  string `mapstructure:"charset"`
}

// LogConfig logging configuration
type LogConfig struct {
	Level  string `mapstructure:"level"`  // debug, info, warn, error
	Format string `mapstructure:"format"` // json, text
}

// DocEngineConfig document engine configuration
type DocEngineConfig struct {
	Type     EngineType           `mapstructure:"type"`
	ES       *ElasticsearchConfig `mapstructure:"es"`
	Infinity *InfinityConfig      `mapstructure:"infinity"`
}

// EngineType document engine type
type EngineType string

const (
	EngineElasticsearch EngineType = "elasticsearch"
	EngineInfinity      EngineType = "infinity"
)

// ElasticsearchConfig Elasticsearch configuration
type ElasticsearchConfig struct {
	Hosts    string `mapstructure:"hosts"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// InfinityConfig Infinity configuration
type InfinityConfig struct {
	URI          string `mapstructure:"uri"`
	PostgresPort int    `mapstructure:"postgres_port"`
	DBName       string `mapstructure:"db_name"`
}

var (
	globalConfig *Config
	globalViper  *viper.Viper
	zapLogger    *zap.Logger
)

// Init initialize configuration
func Init(configPath string) error {
	v := viper.New()

	// Set configuration file path
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// Try to load service_conf.yaml from conf directory first
		v.SetConfigName("service_conf")
		v.SetConfigType("yaml")
		v.AddConfigPath("./conf")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
		v.AddConfigPath("./internal/config")
		v.AddConfigPath("/etc/ragflow/")
	}

	// Read environment variables
	v.SetEnvPrefix("RAGFLOW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read configuration file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("read config file error: %w", err)
		}
		zapLogger.Info("Config file not found, using environment variables only")
	}

	// Save viper instance
	globalViper = v

	// Unmarshal configuration to globalConfig
	// Note: This will only unmarshal fields that match the Config struct
	if err := v.Unmarshal(&globalConfig); err != nil {
		return fmt.Errorf("unmarshal config error: %w", err)
	}

	// Load REGISTER_ENABLED from environment variable (default: 1)
	registerEnabled := 1
	if envVal := os.Getenv("REGISTER_ENABLED"); envVal != "" {
		if parsed, err := strconv.Atoi(envVal); err == nil {
			registerEnabled = parsed
		}
	}
	globalConfig.RegisterEnabled = registerEnabled

	// If we loaded service_conf.yaml, map mysql fields to DatabaseConfig
	if globalConfig != nil && globalConfig.Database.Host == "" {
		// Try to map from mysql section
		if v.IsSet("mysql") {
			mysqlConfig := v.Sub("mysql")
			if mysqlConfig != nil {
				globalConfig.Database.Driver = "mysql"
				globalConfig.Database.Host = mysqlConfig.GetString("host")
				globalConfig.Database.Port = mysqlConfig.GetInt("port")
				globalConfig.Database.Database = mysqlConfig.GetString("name")
				globalConfig.Database.Username = mysqlConfig.GetString("user")
				globalConfig.Database.Password = mysqlConfig.GetString("password")
				globalConfig.Database.Charset = "utf8mb4"
			}
		}
	}

	// Map ragflow section to ServerConfig
	if globalConfig != nil && globalConfig.Server.Port == 0 {
		// Try to map from ragflow section
		if v.IsSet("ragflow") {
			ragflowConfig := v.Sub("ragflow")
			if ragflowConfig != nil {
				globalConfig.Server.Port = ragflowConfig.GetInt("http_port")
				// If mode is not set, default to debug
				if globalConfig.Server.Mode == "" {
					globalConfig.Server.Mode = "release"
				}
			}
		}
	}

	// Map doc_engine section to DocEngineConfig
	if globalConfig != nil && globalConfig.DocEngine.Type == "" {
		// Try to map from doc_engine section
		if v.IsSet("doc_engine") {
			docEngineConfig := v.Sub("doc_engine")
			if docEngineConfig != nil {
				globalConfig.DocEngine.Type = EngineType(docEngineConfig.GetString("type"))
			}
		}
		// Also check legacy es section for backward compatibility
		if v.IsSet("es") {
			esConfig := v.Sub("es")
			if esConfig != nil {
				if globalConfig.DocEngine.Type == "" {
					globalConfig.DocEngine.Type = EngineElasticsearch
				}
				if globalConfig.DocEngine.ES == nil {
					globalConfig.DocEngine.ES = &ElasticsearchConfig{
						Hosts:    esConfig.GetString("hosts"),
						Username: esConfig.GetString("username"),
						Password: esConfig.GetString("password"),
					}
				}
			}
		}
		if v.IsSet("infinity") {
			infConfig := v.Sub("infinity")
			if infConfig != nil {
				if globalConfig.DocEngine.Type == "" {
					globalConfig.DocEngine.Type = EngineInfinity
				}
				if globalConfig.DocEngine.Infinity == nil {
					globalConfig.DocEngine.Infinity = &InfinityConfig{
						URI:          infConfig.GetString("uri"),
						PostgresPort: infConfig.GetInt("postgres_port"),
						DBName:       infConfig.GetString("db_name"),
					}
				}
			}
		}
	}

	return nil
}

// Get get global configuration
func Get() *Config {
	return globalConfig
}

// SetLogger sets the logger instance
func SetLogger(l *zap.Logger) {
	zapLogger = l
}

// PrintAll prints all configuration settings
func PrintAll() {
	if globalViper == nil {
		zapLogger.Info("Configuration not initialized")
		return
	}

	allSettings := globalViper.AllSettings()
	zapLogger.Info("=== All Configuration Settings ===")
	for key, value := range allSettings {
		zapLogger.Info("config", zap.String("key", key), zap.Any("value", value))
	}
	zapLogger.Info("=== End Configuration ===")
}
