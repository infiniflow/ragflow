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

package config

import (
	"github.com/spf13/viper"
)

type IngestorConfig struct {
	// MaxConcurrentWorkers bounds how many ingestion tasks the ingestor runs in
	// parallel — it is the NATS consumer worker count. Valid range [1, 256],
	// enforced at startup by the CLI/env/config resolver (ResolveIngestor*); a
	// value outside that range is a fatal startup error. Default 1.
	MaxConcurrentWorkers int `mapstructure:"max_concurrent_workers"`
	// PageConcurrency bounds the total number of PDF pages parsed concurrently
	// across the whole process (it sizes the single shared page worker pool that
	// all ingestor workers submit to), not a per-document or per-worker limit.
	// Valid range [1, 16], enforced at startup by the CLI/env/config resolver; a
	// value outside that range is a fatal startup error. Default 2.
	PageConcurrency int `mapstructure:"page_concurrency"`
	// CompilerPoolSize bounds the process-wide knowledge-compilation worker
	// pool that drives the cross-doc KNN / LLM-merge / write stages. 0/negative
	// falls back to runtime.NumCPU() (or KC_COMPILE_CONCURRENCY if set).
	CompilerPoolSize int `mapstructure:"compiler_pool_size"`
}

// Ingestor concurrency bounds, shared by the CLI/env/config resolver so the
// configured values can be validated against a single source of truth.
const (
	// MinIngestorWorkers / MaxIngestorWorkers are the inclusive bounds for
	// ingestor.max_concurrent_workers (the NATS consumer count K).
	MinIngestorWorkers = 1
	MaxIngestorWorkers = 256
	// MinPageConcurrency / MaxPageConcurrency are the inclusive bounds for
	// ingestor.page_concurrency (process-wide page parallelism N).
	MinPageConcurrency = 1
	MaxPageConcurrency = 16
)

func (c *Config) ParseIngestorConfig(v *viper.Viper) error {
	// Default Ingestor config.
	c.ingestor.MaxConcurrentWorkers = 1
	c.ingestor.PageConcurrency = 2
	c.ingestor.CompilerPoolSize = 0

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
	if sub.IsSet("page_concurrency") {
		c.ingestor.PageConcurrency = sub.GetInt("page_concurrency")
	}
	if sub.IsSet("compiler_pool_size") {
		c.ingestor.CompilerPoolSize = sub.GetInt("compiler_pool_size")
	}

	return nil
}

func (c *Config) GetIngestorConfig() *IngestorConfig {
	return &c.ingestor
}
