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

import "github.com/spf13/viper"

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
