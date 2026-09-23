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
	"fmt"
	"os"
	"strconv"
	"strings"

	"ragflow/internal/common"

	"github.com/spf13/cast"
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
	// deepDocInferenceConcurrency bounds how many DeepDoc ONNX Runs this process
	// may have in flight at once (K). Each Run opens with
	// max(1, deepDocInferenceCPUCores / deepDocInferenceConcurrency) intra-op
	// threads (see internal/deepdoc/native/inference_config.go), so the total
	// cores inference may occupy is at most deepDocInferenceCPUCores. K must be a
	// positive integer and may not exceed the machine's CPU core count. Default
	// 1. Read from the file-only ingestor.inference_concurrency key; env/CLI
	// precedence is applied later by ResolveDeepDocInferenceConcurrency.
	deepDocInferenceConcurrency int
	// deepDocInferenceConcurrencySet is true when inference_concurrency was
	// explicitly present in the config file. It lets the server distinguish
	// "unset" (fall back to the default of 1) from an explicit value such as 0
	// (which is invalid for K and must be rejected).
	deepDocInferenceConcurrencySet bool
	// deepDocInferenceCPUCores is the CPU-core budget N for DeepDoc in-process
	// inference. A value of 0 means "use all available cores" (resolved to
	// runtime.NumCPU() at startup). The per-Run intra-op thread count is
	// max(1, N/K); N may not exceed the machine's CPU core count. Default 0,
	// which (when unset) is derived from K so each Run stays single-threaded by
	// default, preserving the prior single-core-per-run behaviour; an explicit
	// 0 opts into "all cores".
	deepDocInferenceCPUCores int
	// deepDocInferenceCPUCoresSet is true when inference_cpu_cores was present.
	deepDocInferenceCPUCoresSet bool
}

func (c *Config) ParseIngestorConfig(v *viper.Viper) error {
	// Default Ingestor config
	c.ingestor.MaxConcurrentWorkers = 2
	c.ingestor.CompilerPoolSize = 0
	c.ingestor.deepDocInferenceConcurrency = 1
	c.ingestor.deepDocInferenceConcurrencySet = false
	c.ingestor.deepDocInferenceCPUCores = 0
	c.ingestor.deepDocInferenceCPUCoresSet = false

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

	// Read the DeepDoc inference keys from FILE-ONLY data. sub inherits
	// AutomaticEnv (viper's Sub copies the parent's env settings), so a plain
	// sub.GetInt would also consult the auto-derived env vars
	// RAGFLOW_INGESTOR_INFERENCE_CONCURRENCY / RAGFLOW_INGESTOR_INFERENCE_CPU_CORES
	// and could override or (on a non-integer value) clobber the YAML setting.
	// Env precedence belongs exclusively to ResolveDeepDocInferenceConcurrency /
	// ResolveDeepDocInferenceCPUCores (os.Getenv), so the config tier must be
	// the raw file value. Repoint the env prefix at an impossible token to
	// deterministically disable AutomaticEnv for this read. A present-but
	// non-integer value is a hard error (cast.ToIntE) rather than a silent 0.
	sub.SetEnvPrefix("__DISABLED__")
	if sub.IsSet("inference_concurrency") {
		n, err := cast.ToIntE(sub.Get("inference_concurrency"))
		if err != nil {
			return fmt.Errorf("invalid ingestor.inference_concurrency: %w", err)
		}
		c.ingestor.deepDocInferenceConcurrency = n
		c.ingestor.deepDocInferenceConcurrencySet = true
	}
	if sub.IsSet("inference_cpu_cores") {
		n, err := cast.ToIntE(sub.Get("inference_cpu_cores"))
		if err != nil {
			return fmt.Errorf("invalid ingestor.inference_cpu_cores: %w", err)
		}
		c.ingestor.deepDocInferenceCPUCores = n
		c.ingestor.deepDocInferenceCPUCoresSet = true
	}

	return nil
}

// ResolveDeepDocInferenceConcurrency resolves the effective DeepDoc inference
// concurrency K. Precedence (highest wins): CLI flag > environment variable
// RAGFLOW_DEEPDOC_INFERENCE_CONCURRENCY > ingestor.inference_concurrency
// (config file) > default (1). It returns the resolved value, whether it was
// explicitly set by any layer (config, env, or CLI) as opposed to the built-in
// default, and any error. An explicit value that is non-integer, non-positive,
// or otherwise invalid is reported as an error rather than silently ignored.
func (c *Config) ResolveDeepDocInferenceConcurrency(cli *int) (int, bool, error) {
	val := c.ingestor.deepDocInferenceConcurrency
	explicit := c.ingestor.deepDocInferenceConcurrencySet
	if explicit && val <= 0 {
		return 0, false, fmt.Errorf("invalid ingestor.inference_concurrency %d: must be a positive integer", val)
	}
	if val <= 0 {
		val = 1
		explicit = false
	}
	if v := strings.TrimSpace(os.Getenv(common.EnvDeepDocInferenceConcurrency)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, false, fmt.Errorf("invalid %s %q: %w", common.EnvDeepDocInferenceConcurrency, v, err)
		}
		if n < 1 {
			return 0, false, fmt.Errorf("invalid %s %d: must be a positive integer", common.EnvDeepDocInferenceConcurrency, n)
		}
		val = n
		explicit = true
	}
	// The CLI parser rejects a non-positive --deepdoc-inference-concurrency up
	// front, so the parsed value is already positive; no extra >0 guard needed.
	if cli != nil && *cli > 0 {
		val = *cli
		explicit = true
	}
	return val, explicit, nil
}

// ResolveDeepDocInferenceCPUCores resolves the requested CPU-core budget N for
// DeepDoc in-process inference. Precedence (highest wins): CLI flag > environment
// variable RAGFLOW_DEEPDOC_INFERENCE_CPU_CORES > ingestor.inference_cpu_cores
// (config file) > default (0, meaning "derive from K"). It returns the raw value
// (0 still means "all cores" when explicit), whether N was explicitly set by any
// layer, and any error. A non-integer or negative value is reported as an error;
// an explicit 0 is valid and means "use all available cores".
func (c *Config) ResolveDeepDocInferenceCPUCores(cli *int) (int, bool, error) {
	val := c.ingestor.deepDocInferenceCPUCores
	explicit := c.ingestor.deepDocInferenceCPUCoresSet
	if explicit && val < 0 {
		return 0, false, fmt.Errorf("invalid ingestor.inference_cpu_cores %d: must be >= 0 (0 means all cores)", val)
	}
	if v := strings.TrimSpace(os.Getenv(common.EnvDeepDocInferenceCPUCores)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, false, fmt.Errorf("invalid %s %q: %w", common.EnvDeepDocInferenceCPUCores, v, err)
		}
		if n < 0 {
			return 0, false, fmt.Errorf("invalid %s %d: must be >= 0 (0 means all cores)", common.EnvDeepDocInferenceCPUCores, n)
		}
		val = n
		explicit = true
	}
	// The CLI parser accepts a non-negative --deepdoc-inference-cpu-cores (0 =
	// all cores), so the parsed value is already >= 0.
	if cli != nil {
		val = *cli
		explicit = true
	}
	return val, explicit, nil
}

func (c *Config) GetIngestorConfig() *IngestorConfig {
	return &c.ingestor
}
