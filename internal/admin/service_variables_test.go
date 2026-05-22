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

package admin

import (
	"ragflow/internal/entity"
	"testing"
)

func TestValidateSystemSettingValue(t *testing.T) {
	tests := []struct {
		name      string
		dataType  string
		value     string
		wantError bool
	}{
		{name: "string accepts arbitrary text", dataType: "string", value: "local host"},
		{name: "integer accepts digits", dataType: "integer", value: "15"},
		{name: "integer rejects text", dataType: "integer", value: "localhost", wantError: true},
		{name: "bool accepts true", dataType: "bool", value: "true"},
		{name: "bool accepts false", dataType: "bool", value: "false"},
		{name: "bool rejects non bool", dataType: "bool", value: "yes", wantError: true},
		{name: "json accepts object", dataType: "json", value: `{"endpoint":"http://localhost:9385"}`},
		{name: "json rejects invalid", dataType: "json", value: "{", wantError: true},
		{name: "unknown type rejects", dataType: "float", value: "1.2", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setting := entity.SystemSettings{Name: "test.setting", DataType: tt.dataType}
			err := validateSystemSettingValue(setting, tt.value)
			if (err != nil) != tt.wantError {
				t.Fatalf("validateSystemSettingValue() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestInferSystemSettingDataType(t *testing.T) {
	tests := map[string]string{
		"sandbox.self_managed": "json",
		"mail.enabled":         "bool",
		"mail.server":          "string",
	}

	for name, want := range tests {
		if got := inferSystemSettingDataType(name); got != want {
			t.Fatalf("inferSystemSettingDataType(%q) = %q, want %q", name, got, want)
		}
	}
}
