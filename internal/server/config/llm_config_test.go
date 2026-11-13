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
	"strings"
	"testing"

	"github.com/spf13/viper"

	"ragflow/internal/common"
)

func TestParseLLMConfig(t *testing.T) {
	tests := []struct {
		name    string
		file    map[string]any
		env     string
		want    int
		wantErr string
	}{
		{name: "default", want: common.DefaultLLMMaxRetries},
		{name: "config file", file: map[string]any{"max_retries": 5}, want: 5},
		{name: "zero disables", file: map[string]any{"max_retries": 0}, want: 0},
		{name: "upper bound", file: map[string]any{"max_retries": common.MaxLLMMaxRetries}, want: common.MaxLLMMaxRetries},
		{name: "env overrides file", file: map[string]any{"max_retries": 5}, env: "2", want: 2},
		{name: "env zero overrides file", file: map[string]any{"max_retries": 5}, env: "0", want: 0},
		{name: "env alone", env: "7", want: 7},
		{name: "negative file value", file: map[string]any{"max_retries": -1}, wantErr: "llm.max_retries"},
		{name: "too large file value", file: map[string]any{"max_retries": common.MaxLLMMaxRetries + 1}, wantErr: "out of range"},
		{name: "non-numeric file value", file: map[string]any{"max_retries": "many"}, wantErr: "llm.max_retries"},
		{name: "negative env value", env: "-2", wantErr: common.EnvLLMMaxRetries},
		{name: "non-numeric env value", env: "abc", wantErr: common.EnvLLMMaxRetries},
		{name: "invalid env wins over valid file", file: map[string]any{"max_retries": 2}, env: "99", wantErr: common.EnvLLMMaxRetries},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(common.EnvLLMMaxRetries, tt.env)
			v := viper.New()
			if tt.file != nil {
				v.SetConfigType("yaml")
				if err := v.MergeConfigMap(map[string]any{"llm": tt.file}); err != nil {
					t.Fatal(err)
				}
			}

			cfg := &Config{}
			err := cfg.ParseLLMConfig(v)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to mention %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLLMConfig: %v", err)
			}
			if got := cfg.GetLLMConfig().MaxRetries; got != tt.want {
				t.Errorf("MaxRetries = %d, want %d", got, tt.want)
			}
		})
	}
}
