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

package sandbox

import (
	"testing"
	"time"
)

func TestConfigString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  map[string]any
		key  string
		want string
	}{
		{"missing", map[string]any{}, "X", ""},
		{"string", map[string]any{"X": "hello"}, "X", "hello"},
		{"empty", map[string]any{"X": ""}, "X", ""},
		{"int", map[string]any{"X": 42}, "X", "42"},
		{"int64", map[string]any{"X": int64(99)}, "X", "99"},
		{"float-int", map[string]any{"X": float64(7)}, "X", "7"},
		{"float-fraction", map[string]any{"X": 3.14}, "X", "3.14"},
		{"bool", map[string]any{"X": true}, "X", "true"},
		{"bool-false", map[string]any{"X": false}, "X", "false"},
		{"nil", map[string]any{"X": nil}, "X", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := configString(c.cfg, c.key); got != c.want {
				t.Errorf("configString(%v, %q) = %q, want %q", c.cfg, c.key, got, c.want)
			}
		})
	}
}

func TestConfigInt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		cfg      map[string]any
		key      string
		fallback int
		want     int
	}{
		{"missing", map[string]any{}, "X", 7, 7},
		{"float-int", map[string]any{"X": float64(42)}, "X", 7, 42},
		{"float-frac", map[string]any{"X": 3.7}, "X", 7, 3},
		{"int", map[string]any{"X": 99}, "X", 7, 99},
		{"string-int", map[string]any{"X": "30"}, "X", 7, 30},
		{"string-bad", map[string]any{"X": "abc"}, "X", 7, 7},
		{"bool", map[string]any{"X": true}, "X", 7, 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := configInt(c.cfg, c.key, c.fallback); got != c.want {
				t.Errorf("configInt = %d, want %d", got, c.want)
			}
		})
	}
}

func TestConfigDuration(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		cfg      map[string]any
		key      string
		fallback time.Duration
		want     time.Duration
	}{
		{"missing", map[string]any{}, "X", 30 * time.Second, 30 * time.Second},
		{"string-30s", map[string]any{"X": "30s"}, "X", time.Second, 30 * time.Second},
		{"string-1m", map[string]any{"X": "1m"}, "X", time.Second, time.Minute},
		{"string-bad", map[string]any{"X": "abc"}, "X", 5 * time.Second, 5 * time.Second},
		{"float-seconds", map[string]any{"X": float64(45)}, "X", time.Second, 45 * time.Second},
		{"int-seconds", map[string]any{"X": 90}, "X", time.Second, 90 * time.Second},
		{"float-zero", map[string]any{"X": float64(0)}, "X", 5 * time.Second, 5 * time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := configDuration(c.cfg, c.key, c.fallback); got != c.want {
				t.Errorf("configDuration = %v, want %v", got, c.want)
			}
		})
	}
}
