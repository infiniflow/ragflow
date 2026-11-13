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
		{name: "json accepts object", dataType: "json", value: `{"endpoint":"http://sandbox-executor-manager:9385"}`},
		{name: "json rejects invalid", dataType: "json", value: "{", wantError: true},
		{name: "unknown type rejects", dataType: "float", value: "1.2", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setting := entity.SystemSettings{Name: "test.setting", DataType: tt.dataType}
			err := entity.ValidateSystemSettingValue(setting, tt.value)
			if (err != nil) != tt.wantError {
				t.Fatalf("validateSystemSettingValue() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestInferSystemSettingDataType(t *testing.T) {
	tests := map[string]string{
		"sandbox.provider_type": "string",
		"sandbox.self_managed":  "json",
		"mail.enabled":          "bool",
		"mail.server":           "string",
	}

	for name, want := range tests {
		if got := entity.InferSystemSettingDataType(name); got != want {
			t.Fatalf("inferSystemSettingDataType(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestValidateLLMMaxRetriesSetting(t *testing.T) {
	tests := []struct {
		value     string
		wantError bool
	}{
		{value: "", wantError: false},
		{value: "0"}, {value: "3"}, {value: "10"},
		{value: "-1", wantError: true}, {value: "11", wantError: true},
		{value: "1.5", wantError: true}, {value: "three", wantError: true},
	}
	for _, tt := range tests {
		setting := entity.SystemSettings{Name: "llm.max_retries", DataType: "integer"}
		if err := entity.ValidateSystemSettingValue(setting, tt.value); (err != nil) != tt.wantError {
			t.Errorf("value %q: error = %v, wantError %v", tt.value, err, tt.wantError)
		}
	}
	if got := entity.InferSystemSettingDataType("llm.max_retries"); got != "integer" {
		t.Errorf("data type = %q, want integer", got)
	}
}
