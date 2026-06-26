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
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func setupLangfuseServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.TenantLangfuse{}); err != nil {
		t.Fatalf("failed to migrate test schema: %v", err)
	}

	origDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = origDB })
	return db
}

// stubLangfuseVerifier is a controllable langfuseVerifier for tests.
type stubLangfuseVerifier struct {
	authOK   bool
	authErr  error
	projID   string
	projName string
	projErr  error
}

func (s stubLangfuseVerifier) AuthCheck(_ context.Context, _, _, _ string) (bool, error) {
	return s.authOK, s.authErr
}

func (s stubLangfuseVerifier) GetProject(_ context.Context, _, _, _ string) (string, string, error) {
	return s.projID, s.projName, s.projErr
}

func newLangfuseServiceForTest(v langfuseVerifier) *LangfuseService {
	return &LangfuseService{langfuseDAO: dao.NewLangfuse(), verifier: v}
}

func TestLangfuseService_SetAPIKey_MissingFields(t *testing.T) {
	setupLangfuseServiceTestDB(t)
	svc := newLangfuseServiceForTest(stubLangfuseVerifier{authOK: true})

	_, code, err := svc.SetAPIKey("tenant-1", "", "pk", "host")
	if code != common.CodeDataError || err == nil || err.Error() != "Missing required fields" {
		t.Fatalf("expected Missing required fields/CodeDataError, got code=%d err=%v", code, err)
	}
}

func TestLangfuseService_SetAPIKey_InvalidKeys(t *testing.T) {
	setupLangfuseServiceTestDB(t)
	svc := newLangfuseServiceForTest(stubLangfuseVerifier{authOK: false})

	_, code, err := svc.SetAPIKey("tenant-1", "sk", "pk", "host")
	if code != common.CodeDataError || err == nil || err.Error() != "Invalid Langfuse keys" {
		t.Fatalf("expected Invalid Langfuse keys/CodeDataError, got code=%d err=%v", code, err)
	}
}

func TestLangfuseService_SetAPIKey_VerifierError(t *testing.T) {
	setupLangfuseServiceTestDB(t)
	svc := newLangfuseServiceForTest(stubLangfuseVerifier{authErr: errors.New("network down")})

	_, code, err := svc.SetAPIKey("tenant-1", "sk", "pk", "host")
	if code != common.CodeServerError || err == nil || err.Error() != "network down" {
		t.Fatalf("expected verifier error/CodeServerError, got code=%d err=%v", code, err)
	}
}

func TestLangfuseService_SetAPIKey_CreateThenUpdate(t *testing.T) {
	db := setupLangfuseServiceTestDB(t)
	svc := newLangfuseServiceForTest(stubLangfuseVerifier{authOK: true})

	// Create
	row, code, err := svc.SetAPIKey("tenant-1", "sk-1", "pk-1", "https://a.langfuse.com")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("create failed: code=%d err=%v", code, err)
	}
	if row.SecretKey != "sk-1" || row.Host != "https://a.langfuse.com" {
		t.Fatalf("unexpected row: %+v", row)
	}

	var count int64
	db.Model(&entity.TenantLangfuse{}).Where("tenant_id = ?", "tenant-1").Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 row after create, got %d", count)
	}

	// Update (same tenant) should not create a second row
	_, code, err = svc.SetAPIKey("tenant-1", "sk-2", "pk-2", "https://b.langfuse.com")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("update failed: code=%d err=%v", code, err)
	}
	db.Model(&entity.TenantLangfuse{}).Where("tenant_id = ?", "tenant-1").Count(&count)
	if count != 1 {
		t.Fatalf("expected still 1 row after update, got %d", count)
	}
	stored, _ := dao.NewLangfuse().GetByTenantID("tenant-1")
	if stored == nil || stored.SecretKey != "sk-2" || stored.Host != "https://b.langfuse.com" {
		t.Fatalf("update not persisted: %+v", stored)
	}
}

func TestLangfuseService_GetAPIKey_NoRecord(t *testing.T) {
	setupLangfuseServiceTestDB(t)
	svc := newLangfuseServiceForTest(stubLangfuseVerifier{})

	data, code, message, err := svc.GetAPIKey("tenant-1")
	if err != nil || code != common.CodeSuccess || data != nil {
		t.Fatalf("unexpected: code=%d data=%v err=%v", code, data, err)
	}
	if message != "Have not record any Langfuse keys." {
		t.Fatalf("unexpected message: %q", message)
	}
}

func TestLangfuseService_GetAPIKey_Unauthorized(t *testing.T) {
	setupLangfuseServiceTestDB(t)
	if err := dao.NewLangfuse().Create(&entity.TenantLangfuse{TenantID: "tenant-1", SecretKey: "sk", PublicKey: "pk", Host: "host"}); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	svc := newLangfuseServiceForTest(stubLangfuseVerifier{projErr: ErrLangfuseUnauthorized})

	data, code, message, err := svc.GetAPIKey("tenant-1")
	if data != nil || code != common.CodeDataError || err == nil {
		t.Fatalf("unexpected: code=%d data=%v err=%v", code, data, err)
	}
	if message != "Invalid Langfuse keys loaded" {
		t.Fatalf("unexpected message: %q", message)
	}
}

func TestLangfuseService_GetAPIKey_ApiError(t *testing.T) {
	setupLangfuseServiceTestDB(t)
	if err := dao.NewLangfuse().Create(&entity.TenantLangfuse{TenantID: "tenant-1", SecretKey: "sk", PublicKey: "pk", Host: "host"}); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	svc := newLangfuseServiceForTest(stubLangfuseVerifier{projErr: &LangfuseAPIError{StatusCode: 500, Body: "boom"}})

	data, code, message, err := svc.GetAPIKey("tenant-1")
	if data != nil || code != common.CodeSuccess || err != nil {
		t.Fatalf("unexpected: code=%d data=%v err=%v", code, data, err)
	}
	if message != "Error from Langfuse: langfuse: unexpected status 500: boom" {
		t.Fatalf("unexpected message: %q", message)
	}
}

func TestLangfuseService_GetAPIKey_NonAPIError(t *testing.T) {
	setupLangfuseServiceTestDB(t)
	if err := dao.NewLangfuse().Create(&entity.TenantLangfuse{TenantID: "tenant-1", SecretKey: "sk", PublicKey: "pk", Host: "host"}); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	svc := newLangfuseServiceForTest(stubLangfuseVerifier{projErr: errors.New("json parse failed")})

	data, code, message, err := svc.GetAPIKey("tenant-1")
	if data != nil || code != common.CodeServerError || message != "" || err == nil {
		t.Fatalf("unexpected: code=%d message=%q data=%v err=%v", code, message, data, err)
	}
}

func TestLangfuseService_GetAPIKey_Success(t *testing.T) {
	setupLangfuseServiceTestDB(t)
	if err := dao.NewLangfuse().Create(&entity.TenantLangfuse{TenantID: "tenant-1", SecretKey: "sk", PublicKey: "pk", Host: "https://a.langfuse.com"}); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	svc := newLangfuseServiceForTest(stubLangfuseVerifier{projID: "proj-1", projName: "My Project"})

	data, code, message, err := svc.GetAPIKey("tenant-1")
	if err != nil || code != common.CodeSuccess || message != "success" {
		t.Fatalf("unexpected: code=%d message=%q err=%v", code, message, err)
	}
	if data == nil {
		t.Fatalf("expected data, got nil")
	}
	if data.TenantID != "tenant-1" || data.Host != "https://a.langfuse.com" ||
		data.SecretKey != "sk" || data.PublicKey != "pk" ||
		data.ProjectID != "proj-1" || data.ProjectName != "My Project" {
		t.Fatalf("unexpected data: %+v", data)
	}
}

func TestLangfuseService_DeleteAPIKey_NoRecord(t *testing.T) {
	setupLangfuseServiceTestDB(t)
	svc := newLangfuseServiceForTest(stubLangfuseVerifier{})

	ok, code, message, err := svc.DeleteAPIKey("tenant-1")
	if ok || code != common.CodeSuccess || err != nil {
		t.Fatalf("unexpected: ok=%v code=%d err=%v", ok, code, err)
	}
	if message != "Have not record any Langfuse keys." {
		t.Fatalf("unexpected message: %q", message)
	}
}

func TestLangfuseService_DeleteAPIKey_Success(t *testing.T) {
	db := setupLangfuseServiceTestDB(t)
	if err := dao.NewLangfuse().Create(&entity.TenantLangfuse{TenantID: "tenant-1", SecretKey: "sk", PublicKey: "pk", Host: "host"}); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	svc := newLangfuseServiceForTest(stubLangfuseVerifier{})

	ok, code, message, err := svc.DeleteAPIKey("tenant-1")
	if !ok || code != common.CodeSuccess || message != "" || err != nil {
		t.Fatalf("unexpected: ok=%v code=%d message=%q err=%v", ok, code, message, err)
	}

	var count int64
	db.Model(&entity.TenantLangfuse{}).Where("tenant_id = ?", "tenant-1").Count(&count)
	if count != 0 {
		t.Fatalf("expected row deleted, count=%d", count)
	}
}
