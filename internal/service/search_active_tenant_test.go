//go:build !ee

package service

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func TestPrepareCompletionKeepsActiveTenantIdentity(t *testing.T) {
	setupSearchServiceTestDB(t)
	ctx := serviceTestContext(t, "user-1", "team-1")
	status := string(entity.StatusValid)
	if err := dao.DB.Create(&entity.UserTenant{
		ID: "team-owner", UserID: "team-1", TenantID: "team-1", Role: string(permission.RoleOwner), Status: &status,
	}).Error; err != nil {
		t.Fatalf("create tenant owner membership: %v", err)
	}
	if err := dao.DB.Create(&entity.Knowledgebase{
		ID: "kb-1", TenantID: "team-1", CreatedBy: "user-1", Name: "dataset", Permission: "team", Status: &status,
	}).Error; err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if err := dao.DB.Create(&entity.Search{
		ID: "search-1", TenantID: "team-1", CreatedBy: "user-1", Name: "search",
		SearchConfig: entity.JSONMap{"kb_ids": []interface{}{"kb-1"}}, Status: &status,
	}).Error; err != nil {
		t.Fatalf("create search: %v", err)
	}

	plan, code, err := NewSearchService().PrepareCompletion(ctx, "search-1", &SearchCompletionsRequest{Question: "question"})
	if err != nil {
		t.Fatalf("PrepareCompletion() error = %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("PrepareCompletion() code = %v, want success", code)
	}
	if plan.TenantID != "team-1" {
		t.Fatalf("TenantID = %q, want team-1", plan.TenantID)
	}
}
