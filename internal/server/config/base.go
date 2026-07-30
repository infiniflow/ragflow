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
)

type Config struct {
	General        GeneralConfig
	Authentication AuthenticationConfig
	Database       DatabaseConfig
	DocEngine      DocEngineConfig
	StorageEngine  StorageConfig
	CacheEngine    CacheEngineConfig
	QueueEngine    QueueEngineConfig
	AnalyticEngine AnalyticEngineConfig
	OTel           OpenTelemetryConfig

	Admin     AdminConfig
	APIServer APIServerConfig
	Ingestor  IngestorConfig
	Syncer    SyncerConfig

	Log             LogConfig
	RegisterEnabled int
	OAuth           map[string]OAuthConfig
	SMTP            common.SMTPConfig

	UserDefaultLLM   UserDefaultLLMConfig
	DefaultSuperUser DefaultSuperUser
	Language         string
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

type TaskExecutorConfig struct {
	MessageQueueType string `mapstructure:"message_queue_type"`
}

type OtelConfig struct {
	Host        string  `mapstructure:"host"`
	Port        int     `mapstructure:"port"`
	SampleRatio float64 `mapstructure:"sample_ratio"`
	Secure      bool    `mapstructure:"secure"`
	Stdout      bool    `mapstructure:"stdout"`
	Enable      bool    `mapstructure:"enable"`
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

// EngineType document engine type
type EngineType string

const (
	EngineElasticsearch EngineType = "elasticsearch"
	EngineInfinity      EngineType = "infinity"
)

type StorageType string

type NatsConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}
