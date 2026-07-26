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

// config.go — type-safe accessors for the JSON config map that the
// admin-panel settings reader passes to each provider's FromConfig.
//
// The config map mirrors the env-var contract (same keys, sans the
// per-provider prefix: e.g. SANDBOX_EXECUTOR_MANAGER_URL on env ==
// "EXECUTOR_MANAGER_URL" in the config map). Values come from
// `internal/dao/system_settings.go::SystemSettingsDAO.GetByName` via
// the manager's LoadFromSettings path.
//
// The accessors are intentionally permissive: they accept any JSON
// shape (string, float64 from JSON numbers, bool, nested maps) and
// coerce to the target Go type. Operators that mis-configure a key
// in the admin panel get a default value (per the env-driven
// fallback) rather than a hard failure — admin-panel settings are
// best-effort overrides of the env defaults.

package sandbox

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// configString returns the string value for key, or "" if missing or
// not coercible. JSON-decoded numbers are converted via strconv.
func configString(cfg map[string]any, key string) string {
	v, ok := cfg[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		// JSON numbers decode to float64; render integers without
		// a decimal so duration / pool-size parse cleanly.
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case json.Number:
		return x.String()
	}
	return fmt.Sprint(v)
}

// configInt returns the int value for key, or fallback if missing
// or not coercible. Accepts JSON numbers (float64) and integer
// strings.
func configInt(cfg map[string]any, key string, fallback int) int {
	v, ok := cfg[key]
	if !ok || v == nil {
		return fallback
	}
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return int(n)
		}
	case string:
		if n, err := strconv.Atoi(x); err == nil {
			return n
		}
	}
	return fallback
}

// configDuration returns the time.Duration value for key, or fallback
// if missing / unparseable. Accepts either a duration string ("30s",
// "1m") or a JSON number interpreted as seconds.
func configDuration(cfg map[string]any, key string, fallback time.Duration) time.Duration {
	v, ok := cfg[key]
	if !ok || v == nil {
		return fallback
	}
	switch x := v.(type) {
	case string:
		if d, err := time.ParseDuration(x); err == nil && d > 0 {
			return d
		}
	case float64:
		if x > 0 {
			return time.Duration(x) * time.Second
		}
	case int:
		if x > 0 {
			return time.Duration(x) * time.Second
		}
	case int64:
		if x > 0 {
			return time.Duration(x) * time.Second
		}
	}
	return fallback
}
