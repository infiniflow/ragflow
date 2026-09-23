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
	"os"
	"strconv"
	"strings"

	"ragflow/internal/common"

	"github.com/spf13/viper"
)

type IngestorConfig struct {
	// MaxConcurrentWorkers bounds how many ingestion tasks the ingestor runs in
	// parallel (and dataset-level compile worker count defaults to this value).
	// 0/negative falls back to runtime.NumCPU().
	MaxConcurrentWorkers int `mapstructure:"max_concurrent_workers"`
	// CompilerPoolSize bounds the process-wide knowledge-compilation worker
	// pool that drives the cross-doc KNN / LLM-merge / write stages. 0/negative
	// falls back to runtime.NumCPU() (or KC_COMPILE_CONCURRENCY if set).
	CompilerPoolSize int `mapstructure:"compiler_pool_size"`
	// DeepDoc holds in-process (Go) DeepDoc ONNX inference settings.
	DeepDoc DeepDocConfig `mapstructure:"deepdoc"`
}

// DeepDocConfig holds DeepDoc (in-process ONNX) inference settings.
type DeepDocConfig struct {
	// InferenceConcurrency bounds how many DeepDoc ONNX Runs this process may
	// have in flight at once. Each Run is single-threaded (intraOpThreads = 1
	// in the native package), so this is also the number of cores inference may
	// occupy. Default 1.
	InferenceConcurrency int `mapstructure:"inference_concurrency"`
}

func (c *Config) ParseIngestorConfig(v *viper.Viper) error {
	// Default Ingestor config
	c.ingestor.MaxConcurrentWorkers = 2
	c.ingestor.CompilerPoolSize = 0
	c.ingestor.DeepDoc.InferenceConcurrency = 1

	if !v.IsSet("ingestor") {
		return nil
	}
	sub := v.Sub("ingestor")
	if sub == nil {
		return nil
	}

	if sub.IsSet("max_concurrent_workers") {
		c.ingestor.MaxConcurrentWorkers = sub.GetInt("max_concurrent_workers")
	}

	if sub.IsSet("compiler_pool_size") {
		c.ingestor.CompilerPoolSize = sub.GetInt("compiler_pool_size")
	}

	if ds := sub.Sub("deepdoc"); ds != nil && ds.IsSet("inference_concurrency") {
		c.ingestor.DeepDoc.InferenceConcurrency = ds.GetInt("inference_concurrency")
	}

	return nil
}

// ResolveDeepDocInferenceConcurrency resolves the effective DeepDoc inference
// concurrency. Precedence (highest wins): CLI flag > environment variable
// RAGFLOW_DEEPDOC_INFERENCE_CONCURRENCY > ingestor.deepdoc.inference_concurrency
// (config file) > default (1). The cli pointer is nil when the flag was not
// supplied; a non-positive stored value falls back to the default.
func (c *Config) ResolveDeepDocInferenceConcurrency(cli *int) int {
	val := c.ingestor.DeepDoc.InferenceConcurrency
	if val <= 0 {
		val = 1
	}
	if v := strings.TrimSpace(os.Getenv(common.EnvDeepDocInferenceConcurrency)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			val = n
		}
	}
	if cli != nil && *cli > 0 {
		val = *cli
	}
	return val
}

func (c *Config) GetIngestorConfig() *IngestorConfig {
	return &c.ingestor
}
