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

package models

import (
	"errors"
	"strings"
	"testing"
)

// Same contract as TestPlatformDriverErrorCopiesNoProviderDigits: no number
// of the provider text other than a real status may reach the sanitized text.
func TestPlatformErrorKeepsNoProviderDigits(t *testing.T) {
	for _, tc := range []struct{ raw, secret string }{
		{"xunfei error: appid abc123 apisecret mismatch", "123"},
		{"error: invalid key sk-ab123cd", "123"},
		{"upstream error at llm.internal:443", "443"},
		{"chat failed: tenant 557 error", "557"},
		{"key sk-x4291y rejected", "429"},
	} {
		safe := PlatformDriverError(errors.New(tc.raw)).Error()
		if strings.Contains(safe, tc.secret) {
			t.Errorf("raw %q -> safe %q", tc.raw, safe)
		}
	}
}
