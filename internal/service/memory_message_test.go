package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
)

func TestIsMessageDocumentNotFound(t *testing.T) {
	if !isMessageDocumentNotFound(fmt.Errorf("wrapped: %w", enginetypes.ErrDocumentNotFound)) {
		t.Fatal("expected wrapped document-not-found error to be recognized")
	}

	if isMessageDocumentNotFound(errors.New("index does not exist")) {
		t.Fatal("expected unrelated backend error to remain a server error")
	}
}

func TestRequireMemoryAccessReturnsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := ctx.Err()
	if _, gotErr := NewMemoryService().requireMemoryAccess(ctx, "user-1", "memory-1"); !errors.Is(gotErr, err) {
		t.Fatalf("requireMemoryAccess error = %v, want %v", gotErr, err)
	}
}

type memoryMessageDocEngine struct {
	fakeChatDocEngine
	searchReq   *enginetypes.SearchRequest
	searchResp  *enginetypes.SearchResult
	updateCond  map[string]interface{}
	updateValue map[string]interface{}
	updateBase  string
	updateID    string
}

func (e *memoryMessageDocEngine) Search(ctx context.Context, req *enginetypes.SearchRequest) (*enginetypes.SearchResult, error) {
	e.searchReq = req
	if e.searchResp != nil {
		return e.searchResp, nil
	}
	return &enginetypes.SearchResult{}, nil
}

func (e *memoryMessageDocEngine) UpdateChunks(ctx context.Context, condition map[string]interface{}, newValue map[string]interface{}, baseName string, datasetID string) error {
	e.updateCond = condition
	e.updateValue = newValue
	e.updateBase = baseName
	e.updateID = datasetID
	return nil
}

func setupMemoryMessageTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Memory{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("failed to migrate memory test tables: %v", err)
	}

	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() {
		dao.DB = orig
	})
}

func seedMemoryMessages(t *testing.T) {
	t.Helper()

	memories := []*entity.Memory{
		{
			ID:               "mem-owned",
			Name:             "Owned",
			TenantID:         "user-1",
			MemoryType:       dao.MemoryTypeRaw,
			StorageType:      "table",
			EmbdID:           "embd-1",
			LLMID:            "llm-1",
			Permissions:      string(TenantPermissionMe),
			ForgettingPolicy: string(ForgettingPolicyFIFO),
		},
		{
			ID:               "mem-other",
			Name:             "Other",
			TenantID:         "user-2",
			MemoryType:       dao.MemoryTypeRaw,
			StorageType:      "table",
			EmbdID:           "embd-2",
			LLMID:            "llm-2",
			Permissions:      string(TenantPermissionMe),
			ForgettingPolicy: string(ForgettingPolicyFIFO),
		},
	}
	for _, memory := range memories {
		if err := dao.DB.Create(memory).Error; err != nil {
			t.Fatalf("seed memory %s: %v", memory.ID, err)
		}
	}
}

func TestGetMessagesFiltersAccessibleMemoryAndBuildsRecentSearch(t *testing.T) {
	setupMemoryMessageTestDB(t)
	seedMemoryMessages(t)

	docEngine := &memoryMessageDocEngine{
		searchResp: &enginetypes.SearchResult{
			Total: 1,
			Chunks: []map[string]interface{}{
				{
					"message_id":   int64(12),
					"message_type": "raw",
					"memory_id":    "mem-owned",
					"user_id":      "user-1",
					"agent_id":     "agent-1",
					"session_id":   "session-1",
					"valid_at":     float64(123),
					"status":       1,
					"content":      "hello",
					"extra":        "should be dropped",
				},
			},
		},
	}
	svc := &MemoryService{memoryDAO: dao.NewMemoryDAO(), docEngine: docEngine}

	got, code, err := svc.GetMessages(context.Background(), []string{"mem-owned", "mem-other"}, "user-1", "agent-1", "session-1", 3)
	if err != nil {
		t.Fatalf("GetMessages error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code = %v, want %v", code, common.CodeSuccess)
	}
	if len(got) != 1 || got[0]["content"] != "hello" {
		t.Fatalf("unexpected messages: %+v", got)
	}
	if _, ok := got[0]["extra"]; ok {
		t.Fatalf("unexpected non-selected field in response: %+v", got[0])
	}

	req := docEngine.searchReq
	if req == nil {
		t.Fatal("expected doc engine search request")
	}
	if !reflect.DeepEqual(req.IndexNames, []string{"memory_user-1"}) {
		t.Fatalf("IndexNames = %v, want [memory_user-1]", req.IndexNames)
	}
	if len(req.KbIDs) != 0 {
		t.Fatalf("KbIDs = %v, want empty for memory message search", req.KbIDs)
	}
	if !reflect.DeepEqual(req.Filter["memory_id"], []string{"mem-owned"}) {
		t.Fatalf("memory_id filter = %v, want [mem-owned]", req.Filter["memory_id"])
	}
	if req.Filter["agent_id"] != "agent-1" || req.Filter["session_id"] != "session-1" {
		t.Fatalf("unexpected filter: %+v", req.Filter)
	}
	if req.Limit != 3 {
		t.Fatalf("Limit = %d, want 3", req.Limit)
	}
	if req.OrderBy == nil || len(req.OrderBy.Fields) != 1 || req.OrderBy.Fields[0].Field != "valid_at" || req.OrderBy.Fields[0].Type != enginetypes.SortDesc {
		t.Fatalf("unexpected order by: %+v", req.OrderBy)
	}
}

func TestSearchMessageFiltersAccessibleMemoryAndDefaultsStatus(t *testing.T) {
	setupMemoryMessageTestDB(t)
	seedMemoryMessages(t)

	docEngine := &memoryMessageDocEngine{
		searchResp: &enginetypes.SearchResult{
			Total: 1,
			Chunks: []map[string]interface{}{
				{
					"message_id":   int64(13),
					"message_type": "raw",
					"memory_id":    "mem-owned",
					"user_id":      "user-1",
					"agent_id":     "agent-1",
					"session_id":   "session-1",
					"valid_at":     int64(456),
					"status":       1,
					"content":      "matched",
				},
			},
		},
	}
	svc := &MemoryService{memoryDAO: dao.NewMemoryDAO(), docEngine: docEngine}
	filter := map[string]interface{}{
		"memory_id":  []string{"mem-owned", "mem-other"},
		"agent_id":   "agent-1",
		"session_id": "session-1",
		"user_id":    "user-1",
	}
	params := map[string]interface{}{
		"query":                      "",
		"similarity_threshold":       0.2,
		"keywords_similarity_weight": 0.7,
		"top_n":                      5,
	}

	got, code, err := svc.SearchMessage(context.Background(), "user-1", filter, params)
	if err != nil {
		t.Fatalf("SearchMessage error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code = %v, want %v", code, common.CodeSuccess)
	}
	if len(got) != 1 || got[0]["content"] != "matched" {
		t.Fatalf("unexpected search result: %+v", got)
	}

	req := docEngine.searchReq
	if req == nil {
		t.Fatal("expected doc engine search request")
	}
	if !reflect.DeepEqual(req.Filter["memory_id"], []string{"mem-owned"}) {
		t.Fatalf("memory_id filter = %v, want [mem-owned]", req.Filter["memory_id"])
	}
	if req.Filter["status"] != 1 {
		t.Fatalf("status filter = %v, want 1", req.Filter["status"])
	}
	if req.Filter["agent_id"] != "agent-1" || req.Filter["session_id"] != "session-1" || req.Filter["user_id"] != "user-1" {
		t.Fatalf("unexpected filter: %+v", req.Filter)
	}
	if len(req.MatchExprs) != 0 {
		t.Fatalf("empty query should not build match expressions, got %+v", req.MatchExprs)
	}
	if req.Limit != 5 {
		t.Fatalf("Limit = %d, want 5", req.Limit)
	}
}

func TestUpdateMessageUpdatesStatusByMessageDocID(t *testing.T) {
	setupMemoryMessageTestDB(t)
	seedMemoryMessages(t)

	docEngine := &memoryMessageDocEngine{}
	svc := &MemoryService{memoryDAO: dao.NewMemoryDAO(), docEngine: docEngine}

	ok, err := svc.UpdateMessage(context.Background(), "user-1", "mem-owned", 42, true)
	if err != nil {
		t.Fatalf("UpdateMessage error: %v", err)
	}
	if !ok {
		t.Fatal("UpdateMessage returned false")
	}
	if docEngine.updateBase != "memory_user-1" {
		t.Fatalf("baseName = %q, want memory_user-1", docEngine.updateBase)
	}
	if docEngine.updateID != "mem-owned" {
		t.Fatalf("datasetID = %q, want mem-owned", docEngine.updateID)
	}
	if docEngine.updateCond["id"] != "mem-owned_42" {
		t.Fatalf("condition = %+v, want id mem-owned_42", docEngine.updateCond)
	}
	if docEngine.updateValue["status"] != 1 {
		t.Fatalf("status update = %+v, want status 1", docEngine.updateValue)
	}
}
