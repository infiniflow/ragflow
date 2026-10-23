package document

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// TestIsPresentationFile pins the name-based presentation guard used by the
// parser_id / chunk_method switch in document_dataset_update.go. A WPS
// presentation (dps/dpt) must be treated as a presentation on a par with ppt/
// pptx/pages, so the API allows switching its chunk_method to "presentation".
func TestIsPresentationFile(t *testing.T) {
	p := func(s string) *string { return &s }

	cases := []struct {
		name string
		in   *string
		want bool
	}{
		{"nil name is not presentation", nil, false},
		{"ppt is presentation", p("deck.ppt"), true},
		{"pptx is presentation", p("deck.pptx"), true},
		{"pages is presentation", p("deck.pages"), true},
		{"wps presentation dps is presentation", p("deck.dps"), true},
		{"wps presentation dpt is presentation", p("deck.dpt"), true},
		{"wps presentation dps is case-insensitive", p("DECK.DPS"), true},
		{"pdf is not presentation", p("report.pdf"), false},
		{"wps doc is not presentation", p("note.wps"), false},
		{"wps sheet et is not presentation", p("sheet.et"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPresentationFile(tc.in); got != tc.want {
				t.Fatalf("isPresentationFile(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestBatchUpdateDocumentStatusReturnsPermissionErrorWhenAllDenied(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Update("permission", string(entity.TenantPermissionMe)).Error; err != nil {
		t.Fatalf("make dataset private: %v", err)
	}
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	valid := string(entity.StatusValid)
	if err := db.Create(&entity.UserTenant{
		ID: "member-tenant-1", UserID: "member", TenantID: "tenant-1", Role: "normal", Status: &valid,
	}).Error; err != nil {
		t.Fatalf("create tenant member: %v", err)
	}

	svc := testDocumentService(t)
	ctx := documentTestActorContext(t.Context(), "member", "tenant-1")
	_, code, err := svc.BatchUpdateDocumentStatus(ctx, "member", "kb-1", "0", []string{"doc-1"})
	if err == nil {
		t.Fatal("expected permission error for an inaccessible document")
	}
	if code != common.CodeForbidden {
		t.Fatalf("code = %d, want %d; error: %v", code, common.CodeForbidden, err)
	}
}

func TestBatchUpdateDocumentStatusRetainsPartialFailure(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Update("permission", string(entity.TenantPermissionMe)).Error; err != nil {
		t.Fatalf("make dataset private: %v", err)
	}
	insertTestDoc(t, "owner-doc", "kb-1", 0, 0)
	insertTestDoc(t, "member-doc", "kb-1", 0, 0)
	if err := db.Model(&entity.Document{}).Where("id = ?", "member-doc").Update("created_by", "member").Error; err != nil {
		t.Fatalf("make one document member-owned: %v", err)
	}
	valid := string(entity.StatusValid)
	if err := db.Create(&entity.UserTenant{
		ID: "member-tenant-1", UserID: "member", TenantID: "tenant-1", Role: "normal", Status: &valid,
	}).Error; err != nil {
		t.Fatalf("create tenant member: %v", err)
	}

	svc := testDocumentService(t)
	ctx := documentTestActorContext(t.Context(), "member", "tenant-1")
	_, code, err := svc.BatchUpdateDocumentStatus(ctx, "member", "kb-1", "1", []string{"member-doc", "owner-doc"})
	if err == nil {
		t.Fatal("expected a partial failure")
	}
	if code != common.CodeServerError || err.Error() != "partial failure" {
		t.Fatalf("result code/error = %d/%v, want partial failure", code, err)
	}
}
