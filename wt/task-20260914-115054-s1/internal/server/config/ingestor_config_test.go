//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestParseIngestorConfigReadsWorkerAndCompilerSettings(t *testing.T) {
	v := viper.New()
	v.Set("ingestor", map[string]any{
		"max_concurrent_workers": 4,
		"compiler_pool_size":     2,
	})

	config := &Config{}
	if err := config.ParseIngestorConfig(v); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	if got := config.GetIngestorConfig().MaxConcurrentWorkers; got != 4 {
		t.Fatalf("max concurrent workers = %d, want 4", got)
	}
	if got := config.GetIngestorConfig().CompilerPoolSize; got != 2 {
		t.Fatalf("compiler pool size = %d, want 2", got)
	}
}
