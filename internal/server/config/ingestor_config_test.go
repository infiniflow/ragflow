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
	"testing"
	"time"

	"github.com/spf13/viper"
)

// TestParseIngestorConfigTaskTimeout pins the watchdog knob's parsing rules:
// the default matches Python task_executor's whole-task cap, explicit values
// (including disabling zeros/negatives) are honored, and values that would
// overflow time.Duration at the ingestor wiring call site are clamped first.
func TestParseIngestorConfigTaskTimeout(t *testing.T) {
	t.Run("default aligns with Python do_handle_task whole-task cap", func(t *testing.T) {
		cfg := &Config{}
		if err := cfg.ParseIngestorConfig(viper.New()); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if got := cfg.GetIngestorConfig().TaskTimeoutSeconds; got != 10800 {
			t.Fatalf("TaskTimeoutSeconds = %d, want 10800", got)
		}
	})

	t.Run("explicit override honored", func(t *testing.T) {
		cfg := &Config{}
		v := viper.New()
		v.Set("ingestor", map[string]interface{}{"task_timeout_seconds": 60})
		if err := cfg.ParseIngestorConfig(v); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if got := cfg.GetIngestorConfig().TaskTimeoutSeconds; got != 60 {
			t.Fatalf("TaskTimeoutSeconds = %d, want 60", got)
		}
	})

	t.Run("zero and negative disable the watchdog", func(t *testing.T) {
		for _, set := range []int{0, -1} {
			cfg := &Config{}
			v := viper.New()
			v.Set("ingestor", map[string]interface{}{"task_timeout_seconds": set})
			if err := cfg.ParseIngestorConfig(v); err != nil {
				t.Fatalf("parse(%d): %v", set, err)
			}
			if got := cfg.GetIngestorConfig().TaskTimeoutSeconds; got != set {
				t.Fatalf("TaskTimeoutSeconds = %d, want %d", got, set)
			}
		}
	})

	t.Run("overflow clamped before Duration conversion", func(t *testing.T) {
		cfg := &Config{}
		v := viper.New()
		v.Set("ingestor", map[string]interface{}{"task_timeout_seconds": math.MaxInt64})
		if err := cfg.ParseIngestorConfig(v); err != nil {
			t.Fatalf("parse: %v", err)
		}
		got := cfg.GetIngestorConfig().TaskTimeoutSeconds
		if got != maxTaskTimeoutSeconds {
			t.Fatalf("TaskTimeoutSeconds = %d, want clamp %d", got, maxTaskTimeoutSeconds)
		}
		// The clamped value must survive the seconds→Duration conversion the
		// ingestor wiring performs (time.Duration(n) * time.Second) as a
		// positive duration instead of wrapping negative.
		if d := time.Duration(got) * time.Second; d <= 0 {
			t.Fatalf("clamped value wraps on conversion: %v", d)
		}
	})
}
