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

package service

import (
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// insertTTSTestTenant inserts a tenant row; ttsID may be nil (no default
// TTS model configured).
func insertTTSTestTenant(t *testing.T, id string, ttsID *string) {
	t.Helper()
	name := "tts-test-tenant"
	row := &entity.Tenant{
		ID:        id,
		Name:      &name,
		LLMID:     "glm-4-flash@ZHIPU-AI",
		EmbdID:    "BAAI/bge-m3@ZHIPU-AI",
		TTSID:     ttsID,
		ParserIDs: "naive",
	}
	if err := dao.DB.Create(row).Error; err != nil {
		t.Fatalf("insert test tenant: %v", err)
	}
}

// TestAudioSpeech_NoSelectionNoDefaultTTS covers the agent Message
// auto_play path: no model id and no provider/instance/model names. When
// the tenant has no default TTS model the call must return the typed
// "no default tts model is set" error instead of panicking on a nil
// provider-name dereference.
func TestAudioSpeech_NoSelectionNoDefaultTTS(t *testing.T) {
	testDB := setupServiceTestDB(t)
	pushServiceDB(t, testDB)
	insertTTSTestTenant(t, "tenant-1", nil)

	svc := NewModelProviderService()
	text := "hello"
	resp, code, err := svc.AudioSpeech(t.Context(), nil, nil, nil, nil, "tenant-1", &text, nil, nil)
	if err == nil {
		t.Fatalf("expected error for missing default TTS model, got resp=%v", resp)
	}
	if !strings.Contains(err.Error(), "no default tts model is set") {
		t.Fatalf("err = %v, want it to mention 'no default tts model is set'", err)
	}
	if code != common.CodeNotFound {
		t.Errorf("code = %d, want %d", code, common.CodeNotFound)
	}
	if resp != nil {
		t.Errorf("resp = %v, want nil", resp)
	}
}

// TestAudioSpeech_PartialNamesRejected verifies the by-name lookup guard:
// a modelName without provider/instance names (the shape the auto_play
// dispatch used to send, e.g. "gtts") returns a clear error instead of
// panicking.
func TestAudioSpeech_PartialNamesRejected(t *testing.T) {
	testDB := setupServiceTestDB(t)
	pushServiceDB(t, testDB)

	svc := NewModelProviderService()
	text := "hello"
	badModel := "gtts"
	resp, code, err := svc.AudioSpeech(t.Context(), nil, nil, &badModel, nil, "tenant-1", &text, nil, nil)
	if err == nil {
		t.Fatalf("expected error for partial model names, got resp=%v", resp)
	}
	if !strings.Contains(err.Error(), "provider name, instance name and model name are required") {
		t.Fatalf("err = %v, want it to mention the required-name guard", err)
	}
	if code != common.CodeNotFound {
		t.Errorf("code = %d, want %d", code, common.CodeNotFound)
	}
}

// TestGetTenantDefaultModelByType_NilTTSPointer verifies the tenant
// default-model resolution is nil-safe when the tts_id column is NULL
// (the reporter's configuration: every default set except TTS).
func TestGetTenantDefaultModelByType_NilTTSPointer(t *testing.T) {
	testDB := setupServiceTestDB(t)
	pushServiceDB(t, testDB)
	insertTTSTestTenant(t, "tenant-2", nil)

	svc := NewModelProviderService()
	_, _, _, _, err := svc.GetTenantDefaultModelByType(t.Context(), "tenant-2", entity.ModelTypeTTS)
	if err == nil {
		t.Fatal("expected error for nil tts_id, got nil")
	}
	if !strings.Contains(err.Error(), "no default tts model is set") {
		t.Fatalf("err = %v, want 'no default tts model is set'", err)
	}
}
