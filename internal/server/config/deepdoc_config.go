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

// ParseDeepDocConfig populates DeepDocConfig from the viper instance. The
// deepdoc.inference_concurrency YAML key is overridden by the environment
// variable RAGFLOW_DEEPDOC_INFERENCE_CONCURRENCY (viper's AutomaticEnv, set in
// server.Init). The CLI flag --deepdoc-inference-concurrency takes precedence
// over both and is applied later in the server boot path.
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
