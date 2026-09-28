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
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func setupSetTenantInfoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Tenant{}, &entity.TenantLLM{}, &entity.LLMFactories{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	origDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = origDB })
	return db
}

func createSetTenantInfoTenant(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	status := "1"
	if err := db.Create(&entity.Tenant{
		ID:        id,
		LLMID:     "original-llm",
		EmbdID:    "original-embd",
		ASRID:     "original-asr",
		Img2TxtID: "original-img2txt",
		ParserIDs: "naive:General",
		Credit:    512,
		Status:    &status,
	}).Error; err != nil {
		t.Fatalf("failed to create tenant %s: %v", id, err)
	}
}

func loadSetTenantInfoTenant(t *testing.T, db *gorm.DB, id string) entity.Tenant {
	t.Helper()
	var tenant entity.Tenant
	if err := db.Where("id = ?", id).First(&tenant).Error; err != nil {
		t.Fatalf("failed to load tenant %s: %v", id, err)
	}
	return tenant
}

func newSetTenantInfoRequest(raw map[string]interface{}) *SetTenantInfoRequest {
	req := &SetTenantInfoRequest{Raw: raw}
	if value, ok := raw["tenant_id"].(string); ok {
		req.TenantID = &value
	}
	return req
}

func TestSetTenantInfoRejectsOtherTenant(t *testing.T) {
	db := setupSetTenantInfoTestDB(t)
	createSetTenantInfoTenant(t, db, "tenant-a")
	createSetTenantInfoTenant(t, db, "tenant-b")

	code, err := NewUserService().SetTenantInfo(t.Context(), "tenant-b", newSetTenantInfoRequest(map[string]interface{}{
		"tenant_id":  "tenant-a",
		"llm_id":     "changed-llm",
		"embd_id":    "changed-embd",
		"asr_id":     "changed-asr",
		"img2txt_id": "changed-img2txt",
	}))

	if err == nil {
		t.Fatal("expected an error when updating another tenant")
	}
	if code != common.CodeAuthenticationError {
		t.Fatalf("expected CodeAuthenticationError, got %v", code)
	}
	if tenant := loadSetTenantInfoTenant(t, db, "tenant-a"); tenant.LLMID != "original-llm" {
		t.Fatalf("tenant-a llm_id changed to %q", tenant.LLMID)
	}
}

func TestSetTenantInfoRejectsNonStringTenantID(t *testing.T) {
	setupSetTenantInfoTestDB(t)

	code, err := NewUserService().SetTenantInfo(t.Context(), "tenant-b", newSetTenantInfoRequest(map[string]interface{}{
		"tenant_id": 42,
		"llm_id":    "changed-llm",
	}))

	if err == nil {
		t.Fatal("expected an error when tenant_id is not a string")
	}
	if code != common.CodeAuthenticationError {
		t.Fatalf("expected CodeAuthenticationError, got %v", code)
	}
}

func TestSetTenantInfoUpdatesOnlyModelFields(t *testing.T) {
	db := setupSetTenantInfoTestDB(t)
	createSetTenantInfoTenant(t, db, "tenant-a")
	llmName := "own-llm"
	ownLLM := &entity.TenantLLM{TenantID: "tenant-a", LLMFactory: "OpenAI", LLMName: &llmName, Status: "1"}
	if err := db.Create(ownLLM).Error; err != nil {
		t.Fatalf("failed to create tenant llm: %v", err)
	}

	code, err := NewUserService().SetTenantInfo(t.Context(), "tenant-a", newSetTenantInfoRequest(map[string]interface{}{
		"tenant_id":     "tenant-a",
		"llm_id":        "own-llm",
		"embd_id":       "changed-embd",
		"asr_id":        "changed-asr",
		"img2txt_id":    "changed-img2txt",
		"tenant_llm_id": "foreign-model-id",
		"credit":        999999,
		"status":        "0",
		"parser_ids":    "",
	}))

	if err != nil {
		t.Fatalf("expected success, got code=%v err=%v", code, err)
	}
	tenant := loadSetTenantInfoTenant(t, db, "tenant-a")
	if tenant.LLMID != "own-llm" || tenant.EmbdID != "changed-embd" || tenant.ASRID != "changed-asr" || tenant.Img2TxtID != "changed-img2txt" {
		t.Fatalf("model fields not updated: %+v", tenant)
	}
	if tenant.TenantLLMID == nil || *tenant.TenantLLMID == "foreign-model-id" {
		t.Fatalf("tenant_llm_id must be resolved from the caller's own models, got %v", tenant.TenantLLMID)
	}
	if tenant.Credit != 512 {
		t.Fatalf("credit changed to %d", tenant.Credit)
	}
	if tenant.Status == nil || *tenant.Status != "1" {
		t.Fatalf("status changed to %v", tenant.Status)
	}
	if tenant.ParserIDs != "naive:General" {
		t.Fatalf("parser_ids changed to %q", tenant.ParserIDs)
	}
}
