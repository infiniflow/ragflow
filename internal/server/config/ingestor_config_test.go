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

func TestParseIngestorConfigReadsLogRetentionSettings(t *testing.T) {
	v := viper.New()
	v.Set("ingestor", map[string]any{
		"log_max_rows_per_run":      12,
		"log_max_rows_per_document": 34,
		"log_max_message_chars":     56,
		"log_max_message_bytes":     78,
	})

	c := &Config{}
	if err := c.ParseIngestorConfig(v); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	got := c.GetIngestorConfig()
	if got.LogMaxRowsPerRun != 12 || got.LogMaxRowsPerDocument != 34 || got.LogMaxMessageChars != 56 || got.LogMaxMessageBytes != 78 {
		t.Fatalf("log settings = %+v, want configured values", got)
	}
}

func TestParseIngestorConfigDefaultsAndRejectsInvalidLogRetentionSettings(t *testing.T) {
	defaults := &Config{}
	if err := defaults.ParseIngestorConfig(viper.New()); err != nil {
		t.Fatalf("ParseIngestorConfig defaults: %v", err)
	}
	got := defaults.GetIngestorConfig()
	if got.LogMaxRowsPerRun != 5000 || got.LogMaxRowsPerDocument != 20000 || got.LogMaxMessageChars != 4000 || got.LogMaxMessageBytes != 16384 {
		t.Fatalf("defaults = %+v, want 5000/20000/4000/16384", got)
	}

	invalid := []map[string]any{
		{"log_max_rows_per_run": 1},
		{"log_max_rows_per_document": 4, "log_max_rows_per_run": 5},
		{"log_max_message_chars": 0},
		{"log_max_message_bytes": -1},
	}
	for i, settings := range invalid {
		v := viper.New()
		v.Set("ingestor", settings)
		if err := (&Config{}).ParseIngestorConfig(v); err == nil {
			t.Fatalf("case %d: expected invalid log settings error", i)
		}
	}
}
