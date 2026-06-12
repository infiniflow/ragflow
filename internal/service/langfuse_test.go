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
	"net"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// setupLangfuseTestDB opens an in-memory SQLite database and migrates
// TenantLangfuse.
func setupLangfuseTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.TenantLangfuse{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// swapLangfuseDB replaces dao.DB with db and restores the original in Cleanup.
func swapLangfuseDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })
}

// ── validateLangfuseHost ──────────────────────────────────────────────────────

func TestValidateLangfuseHost(t *testing.T) {
	cases := []struct {
		host    string
		wantErr bool
	}{
		{"https://langfuse.example.com", false},
		{"http://localhost:3000", false},
		{"http://langfuse.internal/", false},
		{"", true},
		{"ftp://langfuse.example.com", true},
		{"//langfuse.example.com", true},
		{"not-a-url", true},
		{"https://", true},
	}

	for _, tc := range cases {
		err := validateLangfuseHost(tc.host)
		if (err != nil) != tc.wantErr {
			t.Errorf("validateLangfuseHost(%q) error=%v, wantErr=%v", tc.host, err, tc.wantErr)
		}
	}
}

// ── isGloballyRoutableIP ─────────────────────────────────────────────────────

func TestIsGloballyRoutableIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		// public addresses
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"203.0.113.1", true},
		// loopback
		{"127.0.0.1", false},
		{"::1", false},
		// private RFC1918
		{"10.0.0.1", false},
		{"172.16.0.1", false},
		{"192.168.1.1", false},
		// link-local
		{"169.254.0.1", false},
		// CG-NAT (RFC 6598)
		{"100.64.0.1", false},
		{"100.127.255.255", false},
		// multicast
		{"224.0.0.1", false},
		// unspecified
		{"0.0.0.0", false},
	}

	for _, tc := range cases {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("invalid test IP: %s", tc.ip)
		}
		got := isGloballyRoutableIP(ip)
		if got != tc.want {
			t.Errorf("isGloballyRoutableIP(%s) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

// ── SetAPIKey (no HTTP) ───────────────────────────────────────────────────────

func TestLangfuseService_SetAPIKey_EmptyFields(t *testing.T) {
	svc := NewLangfuseService()

	cases := []struct {
		name string
		req  SetLangfuseAPIKeyRequest
	}{
		{"empty secret_key", SetLangfuseAPIKeyRequest{SecretKey: "", PublicKey: "pk", Host: "https://h.example.com"}},
		{"empty public_key", SetLangfuseAPIKeyRequest{SecretKey: "sk", PublicKey: "", Host: "https://h.example.com"}},
		{"empty host", SetLangfuseAPIKeyRequest{SecretKey: "sk", PublicKey: "pk", Host: ""}},
		{"whitespace secret_key", SetLangfuseAPIKeyRequest{SecretKey: "   ", PublicKey: "pk", Host: "https://h.example.com"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := svc.SetAPIKey("t-1", &tc.req)
			if err == nil {
				t.Errorf("expected error for %q, got data=%v", tc.name, data)
			}
			if data != nil {
				t.Errorf("expected nil data, got %v", data)
			}
		})
	}
}

// ── GetAPIKey — not found ─────────────────────────────────────────────────────

func TestLangfuseService_GetAPIKey_NotFound(t *testing.T) {
	db := setupLangfuseTestDB(t)
	swapLangfuseDB(t, db)

	svc := NewLangfuseService()
	data, err := svc.GetAPIKey("nonexistent-tenant")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data != nil {
		t.Errorf("expected nil data for missing record, got %v", data)
	}
}

// ── DeleteAPIKey — not found ──────────────────────────────────────────────────

func TestLangfuseService_DeleteAPIKey_NotFound(t *testing.T) {
	db := setupLangfuseTestDB(t)
	swapLangfuseDB(t, db)

	svc := NewLangfuseService()
	deleted, err := svc.DeleteAPIKey("nonexistent-tenant")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted {
		t.Error("expected deleted=false for missing record, got true")
	}
}

// ── DeleteAPIKey — success ────────────────────────────────────────────────────

func TestLangfuseService_DeleteAPIKey_Success(t *testing.T) {
	db := setupLangfuseTestDB(t)
	swapLangfuseDB(t, db)

	// Seed a record directly.
	if err := db.Create(&entity.TenantLangfuse{
		TenantID:  "tenant-del-1",
		SecretKey: "sk-test",
		PublicKey: "pk-test",
		Host:      "https://langfuse.example.com",
	}).Error; err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	svc := NewLangfuseService()
	deleted, err := svc.DeleteAPIKey("tenant-del-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("expected deleted=true, got false")
	}

	// Verify the row is gone.
	var count int64
	db.Model(&entity.TenantLangfuse{}).Where("tenant_id = ?", "tenant-del-1").Count(&count)
	if count != 0 {
		t.Errorf("expected 0 rows after delete, got %d", count)
	}
}
