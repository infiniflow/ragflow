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

package dao

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func TestAgentWebhookCredentialRotationAndRevocation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.AgentWebhookCredential{}); err != nil {
		t.Fatalf("migrate webhook credentials: %v", err)
	}

	identity := struct{ canvasID, userID, tenantID string }{"canvas-1", "user-1", "tenant-1"}
	first := &entity.AgentWebhookCredential{
		ID: "credential-1", CanvasID: identity.canvasID, UserID: identity.userID,
		TenantID: identity.tenantID, TokenHash: "first-hash",
	}
	if err := SaveAgentWebhookCredential(t.Context(), db, first); err != nil {
		t.Fatalf("save initial credential: %v", err)
	}
	if _, err := GetActiveAgentWebhookCredential(t.Context(), db, identity.canvasID, "first-hash"); err != nil {
		t.Fatalf("load initial credential: %v", err)
	}

	rotated := &entity.AgentWebhookCredential{
		ID: "credential-2", CanvasID: identity.canvasID, UserID: identity.userID,
		TenantID: identity.tenantID, TokenHash: "second-hash",
	}
	if err := SaveAgentWebhookCredential(t.Context(), db, rotated); err != nil {
		t.Fatalf("rotate credential: %v", err)
	}
	if _, err := GetActiveAgentWebhookCredential(t.Context(), db, identity.canvasID, "first-hash"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("old credential lookup error = %v, want record not found", err)
	}
	if got, err := GetActiveAgentWebhookCredential(t.Context(), db, identity.canvasID, "second-hash"); err != nil || got.UserID != identity.userID || got.TenantID != identity.tenantID {
		t.Fatalf("load rotated credential = (%+v, %v), want bound identity", got, err)
	}

	if err := RevokeAgentWebhookCredential(t.Context(), db, identity.canvasID, identity.userID); err != nil {
		t.Fatalf("revoke credential: %v", err)
	}
	if _, err := GetActiveAgentWebhookCredential(t.Context(), db, identity.canvasID, "second-hash"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("revoked credential lookup error = %v, want record not found", err)
	}
	if err := RevokeAgentWebhookCredential(t.Context(), db, identity.canvasID, identity.userID); err != nil {
		t.Fatalf("repeat revoke: %v", err)
	}
}
