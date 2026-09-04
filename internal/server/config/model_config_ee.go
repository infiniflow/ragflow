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

import "github.com/spf13/viper"

// DefaultModelsConfig default models configuration
type DefaultModelsConfig struct {
	ChatModel      ModelConfig `mapstructure:"chat_model"`
	EmbeddingModel ModelConfig `mapstructure:"embedding_model"`
	RerankModel    ModelConfig `mapstructure:"rerank_model"`
	ASRModel       ModelConfig `mapstructure:"asr_model"`
	VisionModel    ModelConfig `mapstructure:"vision_model"`
	OCRModel       ModelConfig `mapstructure:"ocr_model"`
	TTSModel       ModelConfig `mapstructure:"tts_model"`
}

// ModelConfig model configuration
type ModelConfig struct {
	Name    string `mapstructure:"name"`
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
	Factory string `mapstructure:"factory"`
}

func (c *Config) ParseDefaultModelsConfig(v *viper.Viper) error {
	return nil
}

func (c *Config) GetDefaultChatModel() ModelConfig {
	return c.defaultModels.ChatModel
}

func (c *Config) GetDefaultEmbeddingModel() ModelConfig {
	return c.defaultModels.EmbeddingModel
}

func (c *Config) GetDefaultRerankModel() ModelConfig {
	return c.defaultModels.RerankModel
}

func (c *Config) GetDefaultASRModel() ModelConfig {
	return c.defaultModels.ASRModel
}

func (c *Config) GetDefaultVisionModel() ModelConfig {
	return c.defaultModels.VisionModel
}

func (c *Config) GetDefaultOCRModel() ModelConfig {
	return c.defaultModels.OCRModel
}

func (c *Config) GetDefaultTTSModel() ModelConfig {
	return c.defaultModels.TTSModel
}
