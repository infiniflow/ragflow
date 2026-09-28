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
	"github.com/spf13/viper"
)

// DeepDocConfig holds DeepDoc (in-process ONNX) inference settings.
type DeepDocConfig struct {
	// InferenceConcurrency bounds how many DeepDoc ONNX Runs this process may
	// have in flight at once. Each Run is single-threaded (intraOpThreads = 1
	// in the native package), so this is also the number of cores inference may
	// occupy. Default 4.
	InferenceConcurrency int `mapstructure:"inference_concurrency"`
}

// ParseDeepDocConfig populates DeepDocConfig from the viper instance. It reads
// only the deepdoc.inference_concurrency YAML key (default 4). The environment
// variable RAGFLOW_DEEPDOC_INFERENCE_CONCURRENCY does NOT override it here:
// v.Sub("deepdoc") does not inherit viper's AutomaticEnv settings (env prefix,
// replacer, AutomaticEnv are all dropped by Sub), so the env override would be
// missed if applied only through viper. The env override is therefore resolved
// later in the server boot path by cmd.resolveDeepDocInferenceConcurrency via
// os.Getenv, which also applies the --deepdoc-inference-concurrency CLI flag
// (highest precedence) over both.
func (c *Config) ParseDeepDocConfig(v *viper.Viper) error {
	c.deepdoc.InferenceConcurrency = 4
	if sub := v.Sub("deepdoc"); sub != nil && sub.IsSet("inference_concurrency") {
		c.deepdoc.InferenceConcurrency = sub.GetInt("inference_concurrency")
	}
	return nil
}

// GetDeepDocConfig returns the parsed DeepDoc configuration.
func (c *Config) GetDeepDocConfig() *DeepDocConfig {
	return &c.deepdoc
}
