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
	"ragflow/internal/common"
	"time"
)

type Config struct {
	General          GeneralConfig
	Authentication   AuthenticationConfig
	Database         DatabaseConfig
	Redis            RedisConfig
	Nats             NatsConfig
	Log              LogConfig
	DocEngine        DocEngineConfig
	StorageEngine    StorageConfig
	RegisterEnabled  int
	OAuth            map[string]OAuthConfig
	SMTP             common.SMTPConfig
	Admin            AdminConfig
	APIServer        APIServerConfig
	UserDefaultLLM   UserDefaultLLMConfig
	DefaultSuperUser DefaultSuperUser
	Language         string
	Ingestor         IngestorConfig
	FileSyncer       FileSyncerConfig
	OTel             OtelConfig
	Clickhouse       ClickhouseConfig
}

// GeneralConfig general configuration
type GeneralConfig struct {
	HeartbeatInterval time.Duration `mapstructure:"heartbeat_interval"`
	Mode              string        `mapstructure:"mode"` // debug, release
	SecretKey         *string       `mapstructure:"secret_key"`
	DocEngine         string        `mapstructure:"doc_engine"`      // Infinity, Elasticsearch
	StorageEngine     string        `mapstructure:"storage_engine"`  // Minio, S3
	CacheEngine       string        `mapstructure:"cache_engine"`    // Redis
	QueueEngine       string        `mapstructure:"queue_engine"`    // NATS
	AnalyticEngine    string        `mapstructure:"analytic_engine"` // Clickhouse
}

// AdminConfig admin server configuration
type AdminConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"http_port"`
}

type APIServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"http_port"`
}

type AuthenticationConfig struct {
	DisablePasswordLogin bool `mapstructure:"disable_password_login"`
	RegisterEnabled      bool `mapstructure:"register_enabled"`
}

type DefaultSuperUser struct {
	Email    string `mapstructure:"email"`
	Password string `mapstructure:"password"`
	Nickname string `mapstructure:"nickname"`
}

type IngestorConfig struct {
	MQType string `mapstructure:"mq_type"`
}

type TaskExecutorConfig struct {
	MessageQueueType string `mapstructure:"message_queue_type"`
}

type FileSyncerConfig struct {
	MaxConcurrentSyncs int `mapstructure:"max_concurrent_syncs"`
	SyncInterval       int `mapstructure:"sync_interval"`
}

type OtelConfig struct {
	Host        string  `mapstructure:"host"`
	Port        int     `mapstructure:"port"`
	SampleRatio float64 `mapstructure:"sample_ratio"`
	Secure      bool    `mapstructure:"secure"`
	Stdout      bool    `mapstructure:"stdout"`
	Enable      bool    `mapstructure:"enable"`
}

type ClickhouseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
}

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
	OCRModel        ModelConfig `mapstructure:"ocr_model"`
	TTSModel        ModelConfig `mapstructure:"tts_model"`
}

// ModelConfig model configuration
type ModelConfig struct {
	Name    string `mapstructure:"name"`
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
	Factory string `mapstructure:"factory"`
}

// OAuthConfig OAuth configuration for a channel.
// Mirrors api/apps/auth/__init__.py's OAUTH_CONFIG entries: a Type that
// selects the auth client flavor (oauth2 / oidc / GitHub), plus the
// transport URLs and client credentials. For OIDC the URLs are derived
// from Issuer via the .well-known/openid-configuration document, so they
// may be left blank.
type OAuthConfig struct {
	DisplayName      string `mapstructure:"display_name"`
	Icon             string `mapstructure:"icon"`
	Type             string `mapstructure:"type"`
	ClientID         string `mapstructure:"client_id"`
	ClientSecret     string `mapstructure:"client_secret"`
	AuthorizationURL string `mapstructure:"authorization_url"`
	TokenURL         string `mapstructure:"token_url"`
	UserinfoURL      string `mapstructure:"userinfo_url"`
	RedirectURI      string `mapstructure:"redirect_uri"`
	Scope            string `mapstructure:"scope"`
	Issuer           string `mapstructure:"issuer"`
}

//name: 'rag_flow'
//user: 'root'
//password: 'infini_rag_flow'
//host: 'localhost'
//port: 3306
//max_connections: 900
//stale_timeout: 300
//max_allowed_packet: 1073741824

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

// LogConfig logging configuration.
//
// Path, MaxSize, MaxBackups, MaxAge, and Compress configure the rotated
// log file. The cmd/* entry points hardcode per-service defaults
// (e.g. "server_main.log" for the API server, "admin_server.log" for
// the admin server, "ingestion_server.log" for the ingestion worker),
// so a typical deployment gets a rotated file without any YAML
// configuration. When Path is empty (the default) the binary's
// hardcoded default filename is used — it does NOT disable file
// output. Set log.path in service_conf.yaml to override the
// per-service default filename.
//
// Compress is a pointer so callers can distinguish "not set" (nil,
// defaults to true) from "explicitly false" (*bool=false). All other
// numeric fields use plain int because their zero values are sensible
// defaults (100 MB / 10 files / 30 days) and there is no operator-meaningful
// reason to distinguish "not set" from "0".
type LogConfig struct {
	Level      string `mapstructure:"level"`       // debug, info, warn, error
	Format     string `mapstructure:"format"`      // json, text (reserved for future use)
	Path       string `mapstructure:"path"`        // per-binary file override; empty = use cmd/* hardcoded default
	MaxSize    int    `mapstructure:"max_size"`    // MB before rotation; default 100
	MaxBackups int    `mapstructure:"max_backups"` // retained rotated files; default 10
	MaxAge     int    `mapstructure:"max_age"`     // days; default 30
	Compress   *bool  `mapstructure:"compress"`    // gzip rotated files; nil = default true
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
	GCS   *GCSConfig   `mapstructure:"gcs"`
}

const (
	StorageOSS   StorageType = "oss"
	StorageS3    StorageType = "s3"
	StorageMinio StorageType = "minio"
	StorageGCS   StorageType = "gcs"
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

type GCSConfig struct {
	Bucket      string `mapstructure:"bucket"`       // Default bucket (optional)
	PrefixPath  string `mapstructure:"prefix_path"`  // Path prefix (optional)
	EndpointURL string `mapstructure:"endpoint_url"` // Custom endpoint (optional)
}

// MinioConfig holds MinIO storage configuration
type MinioConfig struct {
	Host       string `mapstructure:"host"`        // MinIO server host (e.g., "localhost:9000")
	User       string `mapstructure:"user"`        // Access key
	Password   string `mapstructure:"password"`    // Secret key
	Secure     bool   `mapstructure:"secure"`      // Use HTTPS
	Verify     bool   `mapstructure:"verify"`      // Verify SSL certificates
	Region     string `mapstructure:"region"`      // optional
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

type NatsConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}
