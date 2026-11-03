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

//go:build !ee

package config

import (
	"ragflow/internal/common"

	"github.com/spf13/viper"
)

// ModelConfig is one model chosen as a deployment default.
type ModelConfig struct {
	Name    string `mapstructure:"name"`
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
	Factory string `mapstructure:"factory"`
}

// DefaultModelsConfig carries the per-deployment default models. The OSS build
// leaves every slot empty.
type DefaultModelsConfig struct {
	ChatModel      ModelConfig `mapstructure:"chat_model"`
	EmbeddingModel ModelConfig `mapstructure:"embedding_model"`
	RerankModel    ModelConfig `mapstructure:"rerank_model"`
	ASRModel       ModelConfig `mapstructure:"asr_model"`
	VisionModel    ModelConfig `mapstructure:"vision_model"`
	OCRModel       ModelConfig `mapstructure:"ocr_model"`
	TTSModel       ModelConfig `mapstructure:"tts_model"`
}

// BillingConfig carries the billing deployment settings. The OSS build has no
// billing.
type BillingConfig struct{}

// GetBillingConfig returns the billing settings, which the OSS build leaves
// empty.
func (c *Config) GetBillingConfig() BillingConfig {
	return c.billing
}

// ParseBillingConfig ignores billing configuration: the OSS build has no
// billing.
func (c *Config) ParseBillingConfig(v *viper.Viper) error { return nil }

// ParseDefaultModelsConfig ignores deployment model defaults: the OSS build
// has none.
func (c *Config) ParseDefaultModelsConfig(v *viper.Viper) error { return nil }

// GetDefaultChatModel returns the deployment chat model, empty in the OSS
// build.
func (c *Config) GetDefaultChatModel() ModelConfig { return c.defaultModels.ChatModel }

// GetDefaultEmbeddingModel returns the deployment embedding model, empty in the
// OSS build.
func (c *Config) GetDefaultEmbeddingModel() ModelConfig { return c.defaultModels.EmbeddingModel }

// GetDefaultRerankModel returns the deployment rerank model, empty in the OSS
// build.
func (c *Config) GetDefaultRerankModel() ModelConfig { return c.defaultModels.RerankModel }

// GetDefaultASRModel returns the deployment ASR model, empty in the OSS build.
func (c *Config) GetDefaultASRModel() ModelConfig { return c.defaultModels.ASRModel }

// GetDefaultVisionModel returns the deployment vision model, empty in the OSS
// build.
func (c *Config) GetDefaultVisionModel() ModelConfig { return c.defaultModels.VisionModel }

// GetDefaultOCRModel returns the deployment OCR model, empty in the OSS build.
func (c *Config) GetDefaultOCRModel() ModelConfig { return c.defaultModels.OCRModel }

// GetDefaultTTSModel returns the deployment TTS model, empty in the OSS build.
func (c *Config) GetDefaultTTSModel() ModelConfig { return c.defaultModels.TTSModel }

// OAuthConfig carries the OAuth/OIDC login provider settings. The OSS build
// has no OAuth login.
type OAuthConfig struct{}

type OAuthChannel struct{}

// ParseOAuthConfig ignores OAuth configuration: the OSS build has no OAuth
// login.
func (c *Config) ParseOAuthConfig(v *viper.Viper) error { return nil }

func (c *Config) ParseAuditConfig(v *viper.Viper) error { return nil }

func (c *Config) GetAuditConfig() AuditConfig {
	return c.audit
}

func (c *AuditConfig) GetFileOutput() common.FileOutput {
	return common.FileOutput{}
}

// Default ingest-log rotation values mirror the documented IngestLogConfig
// defaults. File output is opt-in: leave ingestor.log.path empty and events emit
// to stdout only.
const (
	defaultIngestLogMaxSizeMB  = 1000
	defaultIngestLogMaxBackups = 20
	defaultIngestLogMaxAgeDays = 60
)

// ParseIngestLogConfig reads the ingestor.log.* keys from the sub-viper passed by
// the ingestor config parser (already merged with the environment by
// server.Init), so env overrides such as RAGFLOW_INGESTOR_LOG_PATH win over the
// YAML file. A nil viper is a no-op so the struct keeps its zero (off) state.
func (c *IngestLogConfig) ParseIngestLogConfig(v *viper.Viper) error {
	if v == nil {
		return nil
	}
	c.Type = "file"
	c.Format = "json"
	c.MaxSize = defaultIngestLogMaxSizeMB
	c.MaxBackups = defaultIngestLogMaxBackups
	c.MaxAge = defaultIngestLogMaxAgeDays
	if v.IsSet("type") {
		c.Type = v.GetString("type")
	}
	if v.IsSet("format") {
		c.Format = v.GetString("format")
	}
	if v.IsSet("path") {
		c.Path = v.GetString("path")
	}
	if v.IsSet("max_size") {
		c.MaxSize = v.GetInt("max_size")
	}
	if v.IsSet("max_backups") {
		c.MaxBackups = v.GetInt("max_backups")
	}
	if v.IsSet("max_age") {
		c.MaxAge = v.GetInt("max_age")
	}
	return nil
}

// GetFileOutput projects the ingest log config onto the common.FileOutput sink
// descriptor. An empty Path means no file sink (stdout only), which keeps the
// feature off by default.
func (c *IngestLogConfig) GetFileOutput() common.FileOutput {
	return common.FileOutput{
		Path:       c.Path,
		MaxSize:    c.MaxSize,
		MaxBackups: c.MaxBackups,
		MaxAge:     c.MaxAge,
	}
}
