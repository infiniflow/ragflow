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
	"fmt"

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
	// LogMaxRowsPerRun is the target row count for folding a terminal run.
	LogMaxRowsPerRun int `mapstructure:"log_max_rows_per_run"`
	// LogMaxRowsPerDocument is the cumulative event-row cap across a document's
	// completed runs.
	LogMaxRowsPerDocument int `mapstructure:"log_max_rows_per_document"`
	// LogMaxMessageChars bounds a persisted event message by Unicode code points.
	LogMaxMessageChars int `mapstructure:"log_max_message_chars"`
	// LogMaxMessageBytes is the hard UTF-8 byte cap for a persisted event.
	LogMaxMessageBytes int `mapstructure:"log_max_message_bytes"`
}

func (c *Config) ParseIngestorConfig(v *viper.Viper) error {
	// Default Ingestor config
	c.ingestor.MaxConcurrentWorkers = 2
	c.ingestor.CompilerPoolSize = 0
	c.ingestor.LogMaxRowsPerRun = 5000
	c.ingestor.LogMaxRowsPerDocument = 20000
	c.ingestor.LogMaxMessageChars = 4000
	c.ingestor.LogMaxMessageBytes = 16384

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

	if sub.IsSet("log_max_rows_per_run") {
		c.ingestor.LogMaxRowsPerRun = sub.GetInt("log_max_rows_per_run")
	}

	if sub.IsSet("log_max_rows_per_document") {
		c.ingestor.LogMaxRowsPerDocument = sub.GetInt("log_max_rows_per_document")
	}

	if sub.IsSet("log_max_message_chars") {
		c.ingestor.LogMaxMessageChars = sub.GetInt("log_max_message_chars")
	}

	if sub.IsSet("log_max_message_bytes") {
		c.ingestor.LogMaxMessageBytes = sub.GetInt("log_max_message_bytes")
	}

	if err := c.ingestor.validateLogRetentionSettings(); err != nil {
		return err
	}

	return nil
}

func (c IngestorConfig) validateLogRetentionSettings() error {
	if c.LogMaxRowsPerRun < 2 {
		return fmt.Errorf("ingestor.log_max_rows_per_run must be at least 2")
	}
	if c.LogMaxRowsPerDocument < c.LogMaxRowsPerRun {
		return fmt.Errorf("ingestor.log_max_rows_per_document must be >= log_max_rows_per_run")
	}
	if c.LogMaxMessageChars <= 0 {
		return fmt.Errorf("ingestor.log_max_message_chars must be positive")
	}
	if c.LogMaxMessageBytes <= 0 {
		return fmt.Errorf("ingestor.log_max_message_bytes must be positive")
	}
	return nil
}

func (c *Config) GetIngestorConfig() *IngestorConfig {
	return &c.ingestor
}
