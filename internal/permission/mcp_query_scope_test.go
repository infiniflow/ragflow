//go:build !enterprise

package permission

import (
	"context"
	"net/url"
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCommunityMCPQueryScopeFiltersBeforeListing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.MCPServer{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate MCP permission tables: %v", err)
	}
	status := string(entity.StatusValid)
	if err := db.Create(&entity.UserTenant{ID: "owner-membership", UserID: "owner", TenantID: "tenant-1", Role: string(RoleOwner), Status: &status}).Error; err != nil {
		t.Fatalf("create owner membership: %v", err)
	}
	if err := db.Create(&entity.UserTenant{ID: "member-membership", UserID: "member", TenantID: "tenant-1", Role: string(RoleNormal), Status: &status}).Error; err != nil {
		t.Fatalf("create member membership: %v", err)
	}
	if err := db.Create(&entity.MCPServer{ID: "mcp-1", TenantID: "tenant-1", Name: "server", URL: "https://example.test", ServerType: "sse"}).Error; err != nil {
		t.Fatalf("create MCP server: %v", err)
	}

	checker := newCommunityDatabaseChecker(db)
	query := ScopeQuery{Kind: ResourceKindMCPServer, Operation: OperationRead}
	for _, test := range []struct {
		userID string
		want   int64
	}{
		{userID: "owner", want: 1},
		{userID: "member", want: 0},
	} {
		scope, err := checker.QueryScope(context.Background(), Subject{UserID: test.userID, TenantID: "tenant-1"}, query)
		if err != nil {
			t.Fatalf("QueryScope(%q): %v", test.userID, err)
		}
		var count int64
		if err := scope.Apply(db.Model(&entity.MCPServer{})).Count(&count).Error; err != nil {
			t.Fatalf("count MCP servers for %q: %v", test.userID, err)
		}
		if count != test.want {
			t.Errorf("QueryScope(%q) count = %d, want %d", test.userID, count, test.want)
		}
	}
}
