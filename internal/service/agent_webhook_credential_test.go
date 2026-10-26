package service

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func TestAgentWebhookCredentialUsesBoundPrincipalAndCanBeRotated(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserTenant{}, &entity.UserCanvas{}, &entity.AgentWebhookCredential{}); err != nil {
		t.Fatalf("migrate webhook credential tables: %v", err)
	}

	joined := entity.TenantMembershipStatusJoined
	tenantID := "tenant-1"
	if err := db.Create(&entity.UserTenant{
		ID: "membership-1", UserID: "caller-1", TenantID: tenantID, Role: "normal", Status: &joined,
	}).Error; err != nil {
		t.Fatalf("seed tenant membership: %v", err)
	}
	canvas := &entity.UserCanvas{
		ID: "canvas-1", UserID: "owner-1", TenantID: &tenantID,
		Permission: string(entity.TenantPermissionTeam), CanvasCategory: "agent_canvas",
	}
	if err := db.Create(canvas).Error; err != nil {
		t.Fatalf("seed canvas: %v", err)
	}

	svc := NewAgentWebhookCredentialService(db)
	firstToken, err := svc.Issue(t.Context(), permission.Subject{UserID: "caller-1"}, canvas)
	if err != nil {
		t.Fatalf("issue credential: %v", err)
	}
	if len(firstToken) != 43 {
		t.Fatalf("credential length = %d, want 43-character base64url token", len(firstToken))
	}
	var stored entity.AgentWebhookCredential
	if err := db.Where("canvas_id = ? AND user_id = ?", canvas.ID, "caller-1").Take(&stored).Error; err != nil {
		t.Fatalf("load stored credential: %v", err)
	}
	if stored.TokenHash == firstToken || len(stored.TokenHash) != 64 {
		t.Fatalf("stored hash = %q, raw token must not be stored", stored.TokenHash)
	}

	resolved, err := svc.Resolve(t.Context(), canvas.ID, firstToken)
	if err != nil {
		t.Fatalf("resolve credential: %v", err)
	}
	if resolved.UserID != "caller-1" || resolved.TenantID != tenantID {
		t.Fatalf("resolved principal = (%q, %q), want (caller-1, %q)", resolved.UserID, resolved.TenantID, tenantID)
	}

	secondToken, err := svc.Issue(t.Context(), permission.Subject{UserID: "caller-1"}, canvas)
	if err != nil {
		t.Fatalf("rotate credential: %v", err)
	}
	if secondToken == firstToken {
		t.Fatal("rotating a credential returned the same token")
	}
	if _, err := svc.Resolve(t.Context(), canvas.ID, firstToken); !errors.Is(err, ErrInvalidAgentWebhookCredential) {
		t.Fatalf("old token resolution error = %v, want invalid credential", err)
	}
	if _, err := svc.Resolve(t.Context(), canvas.ID, secondToken); err != nil {
		t.Fatalf("resolve rotated token: %v", err)
	}
	if _, err := svc.Resolve(t.Context(), "different-canvas", secondToken); !errors.Is(err, ErrInvalidAgentWebhookCredential) {
		t.Fatalf("credential used for another canvas error = %v, want invalid credential", err)
	}
	if err := db.Model(&entity.UserCanvas{}).Where("id = ?", canvas.ID).
		Update("permission", string(entity.TenantPermissionMe)).Error; err != nil {
		t.Fatalf("make canvas private: %v", err)
	}
	if _, err := svc.Resolve(t.Context(), canvas.ID, secondToken); !errors.Is(err, ErrInvalidAgentWebhookCredential) {
		t.Fatalf("revoked run permission error = %v, want invalid credential", err)
	}
	if err := db.Model(&entity.UserCanvas{}).Where("id = ?", canvas.ID).
		Update("permission", string(entity.TenantPermissionTeam)).Error; err != nil {
		t.Fatalf("restore tenant sharing: %v", err)
	}

	removed := entity.TenantMembershipStatusRemoved
	if err := db.Model(&entity.UserTenant{}).Where("user_id = ? AND tenant_id = ?", "caller-1", tenantID).Update("status", removed).Error; err != nil {
		t.Fatalf("remove caller membership: %v", err)
	}
	if _, err := svc.Resolve(t.Context(), canvas.ID, secondToken); !errors.Is(err, ErrInvalidAgentWebhookCredential) {
		t.Fatalf("removed member token resolution error = %v, want invalid credential", err)
	}
	if err := svc.Revoke(t.Context(), canvas.ID, "caller-1"); err != nil {
		t.Fatalf("revoke credential: %v", err)
	}
	if err := svc.Revoke(t.Context(), canvas.ID, "caller-1"); err != nil {
		t.Fatalf("repeat revoke: %v", err)
	}
	if _, err := svc.Resolve(t.Context(), canvas.ID, secondToken); !errors.Is(err, ErrInvalidAgentWebhookCredential) {
		t.Fatalf("revoked token resolution error = %v, want invalid credential", err)
	}
}
