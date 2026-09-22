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
	"testing"

	"github.com/spf13/viper"
)

// TestParseDeepDocConfigDefaultsToFour pins the default inference concurrency.
func TestParseDeepDocConfigDefaultsToFour(t *testing.T) {
	v := viper.New()
	c := &Config{}
	if err := c.ParseDeepDocConfig(v); err != nil {
		t.Fatalf("ParseDeepDocConfig: %v", err)
	}
	if got := c.GetDeepDocConfig().InferenceConcurrency; got != 4 {
		t.Fatalf("default inference concurrency = %d, want 4", got)
	}
}

// TestParseDeepDocConfigReadsYAML pins that the deepdoc.inference_concurrency
// key is honoured when present.
func TestParseDeepDocConfigReadsYAML(t *testing.T) {
	v := viper.New()
	v.Set("deepdoc", map[string]any{"inference_concurrency": 6})
	c := &Config{}
	if err := c.ParseDeepDocConfig(v); err != nil {
		t.Fatalf("ParseDeepDocConfig: %v", err)
	}
	if got := c.GetDeepDocConfig().InferenceConcurrency; got != 6 {
		t.Fatalf("inference concurrency = %d, want 6", got)
	}
}
