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
	"math"
	"time"

	"github.com/spf13/viper"
)

// maxTaskTimeoutSeconds is the largest task_timeout_seconds that survives the
// time.Duration(seconds) * time.Second conversion at the ingestor wiring call
// site without overflowing int64 nanoseconds; larger configured values are
// clamped to it instead of silently wrapping to a negative duration.
const maxTaskTimeoutSeconds = int(math.MaxInt64 / int64(time.Second))

type IngestorConfig struct {
	// MaxConcurrentWorkers bounds how many ingestion tasks the ingestor runs in
	// parallel (the task channel width and dataset-level compile worker count
	// default to this value). 0/negative falls back to runtime.NumCPU().
	MaxConcurrentWorkers int `mapstructure:"max_concurrent_workers"`
	// CompilerPoolSize bounds the process-wide knowledge-compilation worker
	// pool that drives the cross-doc KNN / LLM-merge / write stages. 0/negative
	// falls back to runtime.NumCPU() (or KC_COMPILE_CONCURRENCY if set).
	CompilerPoolSize int `mapstructure:"compiler_pool_size"`
	// TaskTimeoutSeconds bounds how long a single ingestion task may run
	// before the worker watchdog deadline fires; the task is then marked
	// FAILED with a timeout marker on its document instead of lingering in
	// RUNNING at 0% forever (and, with a single worker, blocking every
	// queued document). It is a whole-task budget matching Python
	// task_executor's do_handle_task @timeout(60*60*3, 1) cap (10800s), on
	// top of which Python bounds individual stages (run_raptor_for_kb
	// @timeout(3600), build_chunks @timeout(60*80, 1)). 0/negative disables
	// the watchdog; values above maxTaskTimeoutSeconds are clamped.
	TaskTimeoutSeconds int `mapstructure:"task_timeout_seconds"`
}

func (c *Config) ParseIngestorConfig(v *viper.Viper) error {
	// Default Ingestor config
	c.ingestor.MaxConcurrentWorkers = 2
	c.ingestor.CompilerPoolSize = 0
	c.ingestor.TaskTimeoutSeconds = 10800 // Python do_handle_task whole-task cap: @timeout(60*60*3, 1)

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

	if sub.IsSet("task_timeout_seconds") {
		c.ingestor.TaskTimeoutSeconds = sub.GetInt("task_timeout_seconds")
		if c.ingestor.TaskTimeoutSeconds > maxTaskTimeoutSeconds {
			c.ingestor.TaskTimeoutSeconds = maxTaskTimeoutSeconds
		}
	}

	return nil
}

func (c *Config) GetIngestorConfig() *IngestorConfig {
	return &c.ingestor
}
