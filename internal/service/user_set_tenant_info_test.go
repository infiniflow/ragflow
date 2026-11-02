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

package service

import (
	"context"
	"testing"

	"ragflow/internal/common"
)

func TestSetTenantInfoRejectsInvalidSettingTypesBeforeDatabaseLookup(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]interface{}
	}{
		{name: "numeric model id", raw: map[string]interface{}{"llm_id": 123}},
		{name: "object model id", raw: map[string]interface{}{"tenant_embd_id": map[string]interface{}{"id": "model"}}},
		{name: "null required model id", raw: map[string]interface{}{"img2txt_id": nil}},
		{name: "non-string tenant id", raw: map[string]interface{}{"tenant_id": false}},
		{name: "unsupported field", raw: map[string]interface{}{"other": "value"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := (&UserService{}).SetTenantInfo(context.Background(), "user-1", &SetTenantInfoRequest{Raw: tt.raw})
			if code != common.CodeArgumentError || err == nil {
				t.Fatalf("SetTenantInfo() = (%v, %v), want argument error", code, err)
			}
		})
	}
}

func TestTenantModelSettingUpdatesAllowsNullOnlyForNullableColumns(t *testing.T) {
	updates, err := tenantModelSettingUpdates(map[string]interface{}{
		"llm_id":             "chat@provider",
		"tenant_llm_id":      nil,
		"tts_id":             nil,
		"tenant_tts_id":      nil,
		"ocr_id":             nil,
		"tenant_ocr_id":      nil,
		"decision_id":        nil,
		"tenant_decision_id": nil,
	})
	if err != nil {
		t.Fatalf("tenantModelSettingUpdates() error = %v", err)
	}
	if len(updates) != 8 {
		t.Fatalf("len(updates) = %d, want 8", len(updates))
	}
	for _, key := range []string{"tenant_llm_id", "tts_id", "tenant_tts_id", "ocr_id", "tenant_ocr_id", "decision_id", "tenant_decision_id"} {
		if value, ok := updates[key]; !ok || value != nil {
			t.Errorf("updates[%q] = %#v, present=%v; want present null", key, value, ok)
		}
	}
}
