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
	"strconv"
	"strings"

	"github.com/spf13/viper"

	"ragflow/internal/common"
)

// LLMConfig holds settings for calls to model providers.
type LLMConfig struct {
	// MaxRetries is how many times a failed provider call is retried.
	MaxRetries int
}

// ParseLLMConfig resolves llm.max_retries with the precedence
// environment variable (LLM_MAX_RETRIES) > config file > default. A value that
// is not an integer in [0, common.MaxLLMMaxRetries] is a startup error rather
// than a silent fallback, so a typo cannot quietly disable retries.
func (c *Config) ParseLLMConfig(v *viper.Viper) error {
	c.llm.MaxRetries = common.DefaultLLMMaxRetries

	source, raw := "llm.max_retries", ""
	if v.InConfig("llm.max_retries") {
		raw = fmt.Sprint(v.Get("llm.max_retries"))
	}
	if env := strings.TrimSpace(common.GetEnv(common.EnvLLMMaxRetries)); env != "" {
		source, raw = common.EnvLLMMaxRetries, env
	}
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid %s %q: %w", source, raw, err)
	}
	if err := common.ValidateLLMMaxRetries(n); err != nil {
		return fmt.Errorf("invalid %s: %w", source, err)
	}
	c.llm.MaxRetries = n
	return nil
}

func (c *Config) GetLLMConfig() *LLMConfig {
	return &c.llm
}
