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

	"github.com/spf13/cast"
	"github.com/spf13/viper"
)

// DeepDocConfig holds DeepDoc (in-process ONNX) inference settings.
type DeepDocConfig struct {
	// InferenceConcurrency bounds how many DeepDoc ONNX Runs this process may
	// have in flight at once. The process owner pairs it with InferenceCPUCores:
	// each Run opens with max(1, InferenceCPUCores/InferenceConcurrency)
	// intra-op threads (see internal/deepdoc/native/inference_config.go), so the
	// total cores inference may occupy is at most InferenceCPUCores. K must be a
	// positive integer and may not exceed the machine's CPU core count. Default 4.
	InferenceConcurrency int `mapstructure:"inference_concurrency"`
	// InferenceConcurrencySet is true when inference_concurrency was explicitly
	// present in the config file. It lets the server distinguish "unset" (fall
	// back to the default of 4) from an explicit value such as 0 (which is
	// invalid for K and must be rejected).
	InferenceConcurrencySet bool
	// InferenceCPUCores is the CPU-core budget N for DeepDoc in-process
	// inference. A value of 0 means "use all available cores" and is resolved to
	// runtime.NumCPU() at startup. The per-Run intra-op thread count is
	// max(1, N/K); N may not exceed the machine's CPU core count. Default 4.
	InferenceCPUCores int `mapstructure:"inference_cpu_cores"`
	// InferenceCPUCoresSet is true when inference_cpu_cores was explicitly
	// present in the config file. It distinguishes "unset" (fall back to the
	// default of 4) from an explicit 0 (which means "use all cores").
	InferenceCPUCoresSet bool
}

// ParseDeepDocConfig populates DeepDocConfig from the viper instance. It reads
// the deepdoc.inference_concurrency and deepdoc.inference_cpu_cores YAML keys
// (both default 4). The environment variables
// RAGFLOW_DEEPDOC_INFERENCE_CONCURRENCY / RAGFLOW_DEEPDOC_INFERENCE_CPU_CORES
// do NOT override them here: v.Sub("deepdoc") does not inherit viper's
// AutomaticEnv settings (env prefix, replacer, AutomaticEnv are all dropped by
// Sub), so the env override would be missed if applied only through viper. The
// env overrides are therefore resolved later in the server boot path by
// cmd.resolveDeepDocInferenceConcurrency / cmd.resolveDeepDocInferenceCPUCores
// via os.Getenv, which also apply the --deepdoc-inference-concurrency /
// --deepdoc-inference-cpu-cores CLI flags (highest precedence) over both.
func (c *Config) ParseDeepDocConfig(v *viper.Viper) error {
	c.deepdoc.InferenceConcurrency = 4
	c.deepdoc.InferenceConcurrencySet = false
	c.deepdoc.InferenceCPUCores = 4
	c.deepdoc.InferenceCPUCoresSet = false
	if sub := v.Sub("deepdoc"); sub != nil {
		if sub.IsSet("inference_concurrency") {
			n, err := cast.ToIntE(sub.Get("inference_concurrency"))
			if err != nil {
				return fmt.Errorf("invalid deepdoc.inference_concurrency: %w", err)
			}
			c.deepdoc.InferenceConcurrency = n
			c.deepdoc.InferenceConcurrencySet = true
		}
		if sub.IsSet("inference_cpu_cores") {
			n, err := cast.ToIntE(sub.Get("inference_cpu_cores"))
			if err != nil {
				return fmt.Errorf("invalid deepdoc.inference_cpu_cores: %w", err)
			}
			c.deepdoc.InferenceCPUCores = n
			c.deepdoc.InferenceCPUCoresSet = true
		}
	}
	return nil
}

// GetDeepDocConfig returns the parsed DeepDoc configuration.
func (c *Config) GetDeepDocConfig() *DeepDocConfig {
	return &c.deepdoc
}
