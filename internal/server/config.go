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

package server

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// DefaultConnectTimeout default connection timeout for external services
const DefaultConnectTimeout = 5 * time.Second

// Config application configuration
type Config struct {
	Server          ServerConfig           `mapstructure:"server"`
	Database        DatabaseConfig         `mapstructure:"database"`
	Redis           RedisConfig            `mapstructure:"redis"`
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

// RedisConfig Redis configuration
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

var (
	globalConfig *Config
	globalViper  *viper.Viper
	zapLogger    *zap.Logger
	allConfigs   []map[string]interface{}
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

	docEngine := os.Getenv("DOC_ENGINE")
	if docEngine == "" {
		docEngine = "elasticsearch"
	}
	id := 0
	for k, v := range globalViper.AllSettings() {
		configDict, ok := v.(map[string]interface{})
		if !ok {
			continue
		}

		switch k {
		case "ragflow":
			configDict["id"] = id
			configDict["name"] = fmt.Sprintf("ragflow_%d", id)
			configDict["service_type"] = "ragflow_server"
			configDict["extra"] = map[string]interface{}{}
			configDict["port"] = configDict["http_port"]
			delete(configDict, "http_port")
		case "es":
			// Skip if retrieval_type doesn't match doc_engine
			if docEngine != "elasticsearch" {
				continue
			}
			hosts := getString(configDict, "hosts")
			host, port := parseHostPort(hosts)
			username := getString(configDict, "username")
			password := getString(configDict, "password")
			configDict["id"] = id
			configDict["name"] = "elasticsearch"
			configDict["host"] = host
			configDict["port"] = port
			configDict["service_type"] = "retrieval"
			configDict["extra"] = map[string]interface{}{
				"retrieval_type": "elasticsearch",
				"username":       username,
				"password":       password,
			}
			delete(configDict, "hosts")
			delete(configDict, "username")
			delete(configDict, "password")
		case "infinity":
			// Skip if retrieval_type doesn't match doc_engine
			if docEngine != "infinity" {
				continue
			}
			uri := getString(configDict, "uri")
			host, port := parseHostPort(uri)
			dbName := getString(configDict, "db_name")
			if dbName == "" {
				dbName = "default_db"
			}
			configDict["id"] = id
			configDict["name"] = "infinity"
			configDict["host"] = host
			configDict["port"] = port
			configDict["service_type"] = "retrieval"
			configDict["extra"] = map[string]interface{}{
				"retrieval_type": "infinity",
				"db_name":        dbName,
			}
		case "minio":
			hostPort := getString(configDict, "host")
			host, port := parseHostPort(hostPort)
			user := getString(configDict, "user")
			password := getString(configDict, "password")
			configDict["id"] = id
			configDict["name"] = "minio"
			configDict["host"] = host
			configDict["port"] = port
			configDict["service_type"] = "file_store"
			configDict["extra"] = map[string]interface{}{
				"store_type": "minio",
				"user":       user,
				"password":   password,
			}
			delete(configDict, "bucket")
			delete(configDict, "user")
			delete(configDict, "password")
		case "redis":
			hostPort := getString(configDict, "host")
			host, port := parseHostPort(hostPort)
			password := getString(configDict, "password")
			db := getInt(configDict, "db")
			configDict["id"] = id
			configDict["name"] = "redis"
			configDict["host"] = host
			configDict["port"] = port
			configDict["service_type"] = "message_queue"
			configDict["extra"] = map[string]interface{}{
				"mq_type":  "redis",
				"database": db,
				"password": password,
			}
			delete(configDict, "password")
			delete(configDict, "db")
		case "mysql":
			host := getString(configDict, "host")
			port := getInt(configDict, "port")
			user := getString(configDict, "user")
			password := getString(configDict, "password")
			configDict["id"] = id
			configDict["name"] = "mysql"
			configDict["host"] = host
			configDict["port"] = port
			configDict["service_type"] = "meta_data"
			configDict["extra"] = map[string]interface{}{
				"meta_type": "mysql",
				"username":  user,
				"password":  password,
			}
			delete(configDict, "stale_timeout")
			delete(configDict, "max_connections")
			delete(configDict, "max_allowed_packet")
			delete(configDict, "user")
			delete(configDict, "password")
		case "task_executor":
			mqType := getString(configDict, "message_queue_type")
			configDict["id"] = id
			configDict["name"] = "task_executor"
			configDict["service_type"] = "task_executor"
			configDict["extra"] = map[string]interface{}{
				"message_queue_type": mqType,
			}
			delete(configDict, "message_queue_type")
		case "admin":
			// Skip admin section
			continue
		default:
			// Skip unknown sections
			continue
		}

		// Set default values for empty host/port
		if configDict["host"] == "" {
			configDict["host"] = "-"
		}
		if configDict["port"] == 0 {
			configDict["port"] = "-"
		}

		delete(configDict, "prefix_path")
		delete(configDict, "username")
		allConfigs = append(allConfigs, configDict)
		id++
	}

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
				globalConfig.Server.Port = ragflowConfig.GetInt("http_port") + 2 // 9382, by default
				// globalConfig.Server.Port = ragflowConfig.GetInt("http_port") // Correct
				// If mode is not set, default to debug
				if globalConfig.Server.Mode == "" {
					globalConfig.Server.Mode = "release"
				}
			}
		}
	}

	// Map redis section to RedisConfig
	if globalConfig != nil && globalConfig.Redis.Host != "" {
		if v.IsSet("redis") {
			redisConfig := v.Sub("redis")
			if redisConfig != nil {
				hostStr := redisConfig.GetString("host")
				// Handle host:port format (e.g., "localhost:6379")
				if hostStr == "" {
					return fmt.Errorf("Empty host of redis configuration")
				}

				if idx := strings.LastIndex(hostStr, ":"); idx != -1 {
					globalConfig.Redis.Host = hostStr[:idx]
					if portStr := hostStr[idx+1:]; portStr != "" {
						if port, err := strconv.Atoi(portStr); err == nil {
							globalConfig.Redis.Port = port
						}
					}
				} else {
					return fmt.Errorf("Error address format of redis: %s", hostStr)
				}

				globalConfig.Redis.Password = redisConfig.GetString("password")
				globalConfig.Redis.DB = redisConfig.GetInt("db")
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
func GetConfig() *Config {
	return globalConfig
}

// SetLogger sets the logger instance
func SetLogger(l *zap.Logger) {
	zapLogger = l
}

func GetGlobalViperConfig() *viper.Viper {
	return globalViper
}

func GetAllConfigs() []map[string]interface{} {
	return allConfigs
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

// parseHostPort parses host:port string and returns host and port
func parseHostPort(hostPort string) (string, int) {
	if hostPort == "" {
		return "", 0
	}

	// Handle URL format like http://host:port
	if strings.Contains(hostPort, "://") {
		u, err := url.Parse(hostPort)
		if err == nil {
			hostPort = u.Host
		}
	}

	// Split host:port
	parts := strings.Split(hostPort, ":")
	host := parts[0]
	port := 0
	if len(parts) > 1 {
		port, _ = strconv.Atoi(parts[1])
	}
	return host, port
}

// getString gets string value from map
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// getInt gets int value from map
func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key].(int); ok {
		return v
	}
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}
