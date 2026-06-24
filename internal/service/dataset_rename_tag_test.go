package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
)

type renameTagUpdateCall struct {
	Condition map[string]interface{}
	NewValue  map[string]interface{}
	BaseName  string
	DatasetID string
}

type renameTagMockEngine struct {
	engine.DocEngine
	updateCalls []renameTagUpdateCall
	updateErr   error
}

func (m *renameTagMockEngine) UpdateChunks(ctx context.Context, condition map[string]interface{}, newValue map[string]interface{}, baseName string, datasetID string) error {
	m.updateCalls = append(m.updateCalls, renameTagUpdateCall{
		Condition: condition,
		NewValue:  newValue,
		BaseName:  baseName,
		DatasetID: datasetID,
	})
	return m.updateErr
}

func (m *renameTagMockEngine) GetType() string                { return "mock" }
func (m *renameTagMockEngine) Ping(ctx context.Context) error { return nil }
func (m *renameTagMockEngine) Close() error                   { return nil }

func testDatasetServiceForRenameTag(t *testing.T, docEngine engine.DocEngine) *DatasetService {
	t.Helper()
	return &DatasetService{
		kbDAO:     dao.NewKnowledgebaseDAO(),
		docEngine: docEngine,
	}
}

func insertRenameTagKB(t *testing.T, datasetID, tenantID, permission string) {
	t.Helper()
	kb := &entity.Knowledgebase{
		ID:           datasetID,
		TenantID:     tenantID,
		Name:         "kb-" + datasetID[:6],
		EmbdID:       "embedding@OpenAI",
		CreatedBy:    tenantID,
		Permission:   permission,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		Status:       sptr(string(entity.StatusValid)),
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert test kb: %v", err)
	}
}

func TestDatasetServiceRenameTagSuccess(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "123e4567-e89b-12d3-a456-426614174000"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertRenameTagKB(t, kbID, "user-1", string(entity.TenantPermissionMe))

	docEngine := &renameTagMockEngine{}
	result, code, err := testDatasetServiceForRenameTag(t, docEngine).RenameTag(kbInput, "user-1", "old-tag ", "new-tag")
	if err != nil {
		t.Fatalf("RenameTag failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}
	if result["from"] != "old-tag" || result["to"] != "new-tag" {
		t.Fatalf("result=%v", result)
	}
	if len(docEngine.updateCalls) != 1 {
		t.Fatalf("update calls=%d want=1", len(docEngine.updateCalls))
	}

	call := docEngine.updateCalls[0]
	if call.BaseName != "ragflow_user-1" {
		t.Fatalf("baseName=%q want=%q", call.BaseName, "ragflow_user-1")
	}
	if call.DatasetID != kbID {
		t.Fatalf("datasetID=%q want=%q", call.DatasetID, kbID)
	}
	if got := call.Condition["tag_kwd"]; got != "old-tag" {
		t.Fatalf("condition tag_kwd=%v want=%q", got, "old-tag")
	}
	if got := call.Condition["kb_id"]; got != kbID {
		t.Fatalf("condition kb_id=%v want=%q", got, kbID)
	}

	remove, _ := call.NewValue["remove"].(map[string]interface{})
	add, _ := call.NewValue["add"].(map[string]interface{})
	if got := remove["tag_kwd"]; got != "old-tag" {
		t.Fatalf("remove tag_kwd=%v want=%q", got, "old-tag")
	}
	if got := add["tag_kwd"]; got != "new-tag" {
		t.Fatalf("add tag_kwd=%v want=%q", got, "new-tag")
	}
}

func TestDatasetServiceRenameTagUnauthorized(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "123e4567-e89b-12d3-a456-426614174000"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertRenameTagKB(t, kbID, "tenant-9", string(entity.TenantPermissionMe))

	_, code, err := testDatasetServiceForRenameTag(t, &renameTagMockEngine{}).RenameTag(kbInput, "user-1", "old", "new")
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	if code != common.CodeDataError {
		t.Fatalf("code=%d want=%d", code, common.CodeDataError)
	}
	if err.Error() != "No authorization." {
		t.Fatalf("error=%q want=%q", err.Error(), "No authorization.")
	}
}

func TestDatasetServiceRenameTagUpdateError(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "123e4567-e89b-12d3-a456-426614174000"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertRenameTagKB(t, kbID, "user-1", string(entity.TenantPermissionMe))

	docEngine := &renameTagMockEngine{updateErr: errors.New("boom")}
	_, code, err := testDatasetServiceForRenameTag(t, docEngine).RenameTag(kbInput, "user-1", "old", "new")
	if err == nil {
		t.Fatal("expected update error")
	}
	if code != common.CodeServerError {
		t.Fatalf("code=%d want=%d", code, common.CodeServerError)
	}
	if !strings.Contains(err.Error(), "failed to rename tag") {
		t.Fatalf("error=%q want contains %q", err.Error(), "failed to rename tag")
	}
}
