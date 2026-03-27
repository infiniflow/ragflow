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
	"net"
	"net/mail"
	"net/url"
	"os"
	"ragflow/internal/logger"
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
	Server           ServerConfig           `mapstructure:"server"`
	Database         DatabaseConfig         `mapstructure:"database"`
	Redis            RedisConfig            `mapstructure:"redis"`
	Log              LogConfig              `mapstructure:"log"`
	DocEngine        DocEngineConfig        `mapstructure:"doc_engine"`
	StorageEngine    StorageConfig          `mapstructure:"storage_engine"`
	RegisterEnabled  int                    `mapstructure:"register_enabled"`
	OAuth            map[string]OAuthConfig `mapstructure:"oauth"`
	Admin            AdminConfig            `mapstructure:"admin"`
	UserDefaultLLM   UserDefaultLLMConfig   `mapstructure:"user_default_llm"`
	DefaultSuperUser DefaultSuperUser       `mapstructure:"default_super_user"`
	Language         string                 `mapstructure:"language"`
}

// AdminConfig admin server configuration
type AdminConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"http_port"`
}

type DefaultSuperUser struct {
	Email    string `mapstructure:"email"`
	Password string `mapstructure:"password"`
	Nickname string `mapstructure:"nickname"`
}

// UserDefaultLLMConfig user default LLM configuration
type UserDefaultLLMConfig struct {
	DefaultModels DefaultModelsConfig `mapstructure:"default_models"`
}

// DefaultModelsConfig default models configuration
type DefaultModelsConfig struct {
	ChatModel       ModelConfig `mapstructure:"chat_model"`
	EmbeddingModel  ModelConfig `mapstructure:"embedding_model"`
	RerankModel     ModelConfig `mapstructure:"rerank_model"`
	ASRModel        ModelConfig `mapstructure:"asr_model"`
	Image2TextModel ModelConfig `mapstructure:"image2text_model"`
}

// ModelConfig model configuration
type ModelConfig struct {
	Name    string `mapstructure:"name"`
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
	Factory string `mapstructure:"factory"`
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
	URI                    string `mapstructure:"uri"`
	PostgresPort           int    `mapstructure:"postgres_port"`
	DBName                 string `mapstructure:"db_name"`
	MappingFileName        string `mapstructure:"mapping_file_name"`
	DocMetaMappingFileName string `mapstructure:"doc_meta_mapping_file_name"`
}

type StorageType string

// StorageConfig holds all storage-related configurations
type StorageConfig struct {
	Type  StorageType  `mapstructure:"type"`
	Minio *MinioConfig `mapstructure:"minio"`
	S3    *S3Config    `mapstructure:"s3"`
	OSS   *OSSConfig   `mapstructure:"oss"`
}

const (
	StorageOSS   StorageType = "oss"
	StorageS3    StorageType = "s3"
	StorageMinio StorageType = "minio"
)

// OSSConfig holds Aliyun OSS storage configuration
// OSS is compatible with S3 API
type OSSConfig struct {
	AccessKey        string `mapstructure:"access_key"`        // OSS Access Key ID
	SecretKey        string `mapstructure:"secret_key"`        // OSS Secret Access Key
	EndpointURL      string `mapstructure:"endpoint_url"`      // OSS Endpoint (e.g., "https://oss-cn-hangzhou.aliyuncs.com")
	Region           string `mapstructure:"region"`            // Region (e.g., "cn-hangzhou")
	Bucket           string `mapstructure:"bucket"`            // Default bucket (optional)
	PrefixPath       string `mapstructure:"prefix_path"`       // Path prefix (optional)
	SignatureVersion string `mapstructure:"signature_version"` // Signature version
	AddressingStyle  string `mapstructure:"addressing_style"`  // Addressing style
}

// MinioConfig holds MinIO storage configuration
type MinioConfig struct {
	Host       string `mapstructure:"host"`        // MinIO server host (e.g., "localhost:9000")
	User       string `mapstructure:"user"`        // Access key
	Password   string `mapstructure:"password"`    // Secret key
	Secure     bool   `mapstructure:"secure"`      // Use HTTPS
	Verify     bool   `mapstructure:"verify"`      // Verify SSL certificates
	Bucket     string `mapstructure:"bucket"`      // Default bucket (optional)
	PrefixPath string `mapstructure:"prefix_path"` // Path prefix (optional)
}

// S3Config holds AWS S3 storage configuration
type S3Config struct {
	AccessKey        string `mapstructure:"access_key"`        // AWS Access Key ID
	SecretKey        string `mapstructure:"secret_key"`        // AWS Secret Access Key
	Region           string `mapstructure:"region_name"`       // AWS Region
	SessionToken     string `mapstructure:"session_token"`     // AWS Session Token (optional)
	EndpointURL      string `mapstructure:"endpoint_url"`      // Custom endpoint (optional)
	SignatureVersion string `mapstructure:"signature_version"` // Signature version
	AddressingStyle  string `mapstructure:"addressing_style"`  // Addressing style
	Bucket           string `mapstructure:"bucket"`            // Default bucket (optional)
	PrefixPath       string `mapstructure:"prefix_path"`       // Path prefix (optional)
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

	err := FromConfigFile(configPath)
	if err != nil {
		return err
	}

	err = FromEnvironments()
	if err != nil {
		return err
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
			if globalConfig.DocEngine.Type != "elasticsearch" {
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
			if globalConfig.DocEngine.Type != "infinity" {
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

	return nil
}

func FromEnvironments() error {
	// Doc engine
	docEngine := strings.ToLower(os.Getenv("DOC_ENGINE"))
	switch docEngine {
	case "infinity":
		globalConfig.DocEngine.Type = EngineInfinity
	case "":
		// Default
		if globalConfig.DocEngine.Type == "" {
			globalConfig.DocEngine.Type = EngineElasticsearch
		}
	case "elasticsearch":
		globalConfig.DocEngine.Type = EngineElasticsearch
	case "opensearch":
	case "oceanbase":
		return fmt.Errorf("not implemented: %s", docEngine)
	default:
		return fmt.Errorf("invalid doc engine: %s", docEngine)
	}

	// Default super user email
	globalConfig.DefaultSuperUser.Email = "admin@ragflow.io"
	superUserEmail := os.Getenv("DEFAULT_SUPERUSER_EMAIL")
	if superUserEmail != "" {
		_, err := mail.ParseAddress(superUserEmail)
		if err != nil {
			return fmt.Errorf("invalid super user email: %s", superUserEmail)
		}
		globalConfig.DefaultSuperUser.Email = superUserEmail
	}

	globalConfig.DefaultSuperUser.Password = "admin"
	superUserPassword := os.Getenv("DEFAULT_SUPERUSER_PASSWORD")
	if superUserPassword != "" {
		globalConfig.DefaultSuperUser.Password = superUserPassword
	}

	globalConfig.DefaultSuperUser.Nickname = "admin"
	superUserNickname := os.Getenv("DEFAULT_SUPERUSER_NICKNAME")
	if superUserNickname != "" {
		globalConfig.DefaultSuperUser.Nickname = superUserNickname
	}

	// Meta database
	databaseType := strings.ToLower(os.Getenv("DB_TYPE"))
	switch databaseType {
	case "mysql":
		globalConfig.Database.Driver = "mysql"
	case "":
		// Default
		if globalConfig.Database.Driver == "" {
			globalConfig.Database.Driver = "mysql"
		}
	default:
		return fmt.Errorf("invalid database type: %s", databaseType)
	}

	// Storage
	storageType := strings.ToLower(os.Getenv("STORAGE_IMPL"))
	switch storageType {
	case "minio":
		globalConfig.StorageEngine.Type = StorageMinio
	case "s3":
		globalConfig.StorageEngine.Type = StorageS3
	case "oss":
		globalConfig.StorageEngine.Type = StorageOSS
	case "":
		// Default
		if globalConfig.StorageEngine.Type == "" {
			globalConfig.StorageEngine.Type = StorageMinio
		}
	default:
		return fmt.Errorf("invalid storage type: %s", storageType)
	}

	// Minio
	minioIP := strings.ToLower(os.Getenv("MINIO_IP"))
	if minioIP != "" {
		_, port, err := net.SplitHostPort(globalConfig.StorageEngine.Minio.Host)
		if err != nil {
			return fmt.Errorf("Error parsing host address %s: %v\n", globalConfig.StorageEngine.Minio.Host, err)
		}
		globalConfig.StorageEngine.Minio.Host = fmt.Sprintf("%s:%s", minioIP, port)
	}

	minioPort := strings.ToLower(os.Getenv("MINIO_PORT"))
	logger.Info(fmt.Sprintf("MINIO ip and port from env: %s:%s", minioIP, minioPort))
	if minioPort != "" {
		ip, _, err := net.SplitHostPort(globalConfig.StorageEngine.Minio.Host)
		if err != nil {
			return fmt.Errorf("Error parsing host address %s: %v\n", globalConfig.StorageEngine.Minio.Host, err)
		}
		globalConfig.StorageEngine.Minio.Host = fmt.Sprintf("%s:%s", ip, minioPort)
	}

	// Language
	if globalConfig.Language == "" {
		globalConfig.Language = GetLanguage()
	}

	return nil
}

func FromConfigFile(configPath string) error {
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

	// Set default values for admin configuration if not configured
	if globalConfig.Admin.Host == "" {
		globalConfig.Admin.Host = "127.0.0.1"
	}
	if globalConfig.Admin.Port == 0 {
		globalConfig.Admin.Port = 9383
	} else {
		globalConfig.Admin.Port += 2
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
				globalConfig.Server.Port = ragflowConfig.GetInt("http_port") + 4 // 9384, by default
				//globalConfig.Server.Port = ragflowConfig.GetInt("http_port") // Correct
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

	if globalConfig != nil && globalConfig.StorageEngine.Type == "" {
		// Also check legacy es section for backward compatibility
		if v.IsSet("minio") {
			minioConfig := v.Sub("minio")
			if minioConfig != nil {
				if globalConfig.StorageEngine.Minio == nil {
					globalConfig.StorageEngine.Minio = &MinioConfig{
						Host:       minioConfig.GetString("host"),
						User:       minioConfig.GetString("user"),
						Password:   minioConfig.GetString("password"),
						Secure:     minioConfig.GetBool("secure"),
						PrefixPath: minioConfig.GetString("prefix_path"),
						Verify:     minioConfig.GetBool("verify"),
						Bucket:     minioConfig.GetString("bucket"),
					}
				}
			}
		}

		if v.IsSet("s3") {
			s3Config := v.Sub("s3")
			if s3Config != nil {
				if globalConfig.StorageEngine.S3 == nil {
					globalConfig.StorageEngine.S3 = &S3Config{
						AccessKey: s3Config.GetString("access_key"),
						SecretKey: s3Config.GetString("secret_key"),
						Region:    s3Config.GetString("region"),
					}
				}
			}
		}

		if v.IsSet("oss") {
			ossConfig := v.Sub("oss")
			if ossConfig != nil {
				if globalConfig.StorageEngine.OSS == nil {
					globalConfig.StorageEngine.OSS = &OSSConfig{
						AccessKey:        ossConfig.GetString("access_key"),
						SecretKey:        ossConfig.GetString("secret_key"),
						EndpointURL:      ossConfig.GetString("endpoint_url"),
						Region:           ossConfig.GetString("region"),
						Bucket:           ossConfig.GetString("bucket"),
						SignatureVersion: ossConfig.GetString("signature_version"),
						AddressingStyle:  ossConfig.GetString("addressing_style"),
					}
				}
			}
		}
	}

	// Map user_default_llm section to UserDefaultLLMConfig
	if v.IsSet("user_default_llm") {
		userDefaultLLMConfig := v.Sub("user_default_llm")
		if userDefaultLLMConfig != nil {
			if defaultModels := userDefaultLLMConfig.Sub("default_models"); defaultModels != nil {
				globalConfig.UserDefaultLLM.DefaultModels.ChatModel = ModelConfig{
					Name:    defaultModels.GetString("chat_model.name"),
					APIKey:  defaultModels.GetString("chat_model.api_key"),
					BaseURL: defaultModels.GetString("chat_model.base_url"),
					Factory: defaultModels.GetString("chat_model.factory"),
				}
				globalConfig.UserDefaultLLM.DefaultModels.EmbeddingModel = ModelConfig{
					Name:    defaultModels.GetString("embedding_model.name"),
					APIKey:  defaultModels.GetString("embedding_model.api_key"),
					BaseURL: defaultModels.GetString("embedding_model.base_url"),
					Factory: defaultModels.GetString("embedding_model.factory"),
				}
				globalConfig.UserDefaultLLM.DefaultModels.RerankModel = ModelConfig{
					Name:    defaultModels.GetString("rerank_model.name"),
					APIKey:  defaultModels.GetString("rerank_model.api_key"),
					BaseURL: defaultModels.GetString("rerank_model.base_url"),
					Factory: defaultModels.GetString("rerank_model.factory"),
				}
				globalConfig.UserDefaultLLM.DefaultModels.ASRModel = ModelConfig{
					Name:    defaultModels.GetString("asr_model.name"),
					APIKey:  defaultModels.GetString("asr_model.api_key"),
					BaseURL: defaultModels.GetString("asr_model.base_url"),
					Factory: defaultModels.GetString("asr_model.factory"),
				}
				globalConfig.UserDefaultLLM.DefaultModels.Image2TextModel = ModelConfig{
					Name:    defaultModels.GetString("image2text_model.name"),
					APIKey:  defaultModels.GetString("image2text_model.api_key"),
					BaseURL: defaultModels.GetString("image2text_model.base_url"),
					Factory: defaultModels.GetString("image2text_model.factory"),
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

// GetAdminConfig gets the admin server configuration
func GetAdminConfig() *AdminConfig {
	if globalConfig == nil {
		return nil
	}
	return &globalConfig.Admin
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

func GetLanguage() string {
	lang := os.Getenv("LANG")
	if lang == "" {
		lang = os.Getenv("LANGUAGE")
	}

	lang = strings.ToLower(lang)

	if strings.Contains(lang, "zh_") ||
		strings.Contains(lang, "zh-") ||
		strings.HasPrefix(lang, "zh") {
		return "Chinese"
	}

	return "English"
}
