package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

func TestDatasetServiceListTagsDatasetLookupError(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	kbID := "123e4567e89b12d3a456426614174000"
	insertListTagsKB(t, kbID, "user-1", string(entity.TenantPermissionMe), 1)
	wantErr := errors.New("dataset lookup failed")
	queries := 0
	if err := db.Callback().Query().Before("gorm:query").Register("list_tags_lookup_failure", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*entity.Knowledgebase); ok {
			queries++
			// Authorization reads the dataset first; fail the subsequent lookup.
			if queries == 2 {
				tx.AddError(wantErr)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	docEngine := &listTagsMockEngine{chunkStoreExists: true}
	_, code, err := (&DatasetService{kbDAO: dao.NewKnowledgebaseDAO(), docEngine: docEngine}).ListTags(t.Context(), kbID, "user-1")
	if queries != 2 || code != common.CodeServerError || !errors.Is(err, wantErr) {
		t.Fatalf("queries=%d code=%d err=%v", queries, code, err)
	}
	if len(docEngine.requests) != 0 {
		t.Fatal("dataset lookup failure queried the document engine")
	}
}

type listTagsMockEngine struct {
	engine.DocEngine
	searchResults    map[string]*types.SearchResult
	searchErr        error
	requests         []*types.SearchRequest
	chunkStoreExists bool
	chunkStoreErr    error
}

func (m *listTagsMockEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	cloned := &types.SearchRequest{
		IndexNames:   append([]string(nil), req.IndexNames...),
		KbIDs:        append([]string(nil), req.KbIDs...),
		Offset:       req.Offset,
		Limit:        req.Limit,
		SelectFields: append([]string(nil), req.SelectFields...),
	}
	m.requests = append(m.requests, cloned)
	if len(req.IndexNames) == 0 {
		return &types.SearchResult{}, nil
	}
	if res, ok := m.searchResults[req.IndexNames[0]]; ok {
		return res, nil
	}
	return &types.SearchResult{}, nil
}

func (m *listTagsMockEngine) GetAggregation(chunks []map[string]interface{}, fieldName string) []map[string]interface{} {
	counts := make(map[string]int)
	for _, chunk := range chunks {
		raw, ok := chunk[fieldName]
		if !ok || raw == nil {
			continue
		}
		switch value := raw.(type) {
		case string:
			for _, tag := range strings.Split(value, "###") {
				tag = strings.TrimSpace(tag)
				if tag != "" {
					counts[tag]++
				}
			}
		case []string:
			for _, tag := range value {
				tag = strings.TrimSpace(tag)
				if tag != "" {
					counts[tag]++
				}
			}
		}
	}

	result := make([]map[string]interface{}, 0, len(counts))
	for tag, count := range counts {
		result = append(result, map[string]interface{}{
			"key":   tag,
			"count": count,
		})
	}
	return result
}

func (m *listTagsMockEngine) ChunkStoreExists(ctx context.Context, baseName, datasetID string) (bool, error) {
	if m.chunkStoreErr != nil {
		return false, m.chunkStoreErr
	}
	return m.chunkStoreExists, nil
}

func (m *listTagsMockEngine) GetType() string { return "mock" }

func (m *listTagsMockEngine) Ping(ctx context.Context) error { return nil }

func (m *listTagsMockEngine) Close() error { return nil }

func testDatasetServiceForListTags(t *testing.T, docEngine engine.DocEngine) *DatasetService {
	t.Helper()
	return &DatasetService{
		kbDAO:     dao.NewKnowledgebaseDAO(),
		docEngine: docEngine,
	}
}

func insertListTagsKB(t *testing.T, datasetID, tenantID, permission string, docNum int64) {
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
		DocNum:       docNum,
		Status:       sptr(string(entity.StatusValid)),
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert test kb: %v", err)
	}
}

func TestDatasetServiceListTagsSuccess(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "123e4567-e89b-12d3-a456-426614174000"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertListTagsKB(t, kbID, "user-1", string(entity.TenantPermissionMe), 2)

	docEngine := &listTagsMockEngine{
		chunkStoreExists: true,
		searchResults: map[string]*types.SearchResult{
			"ragflow_user-1": {
				Chunks: []map[string]interface{}{
					{"tag_kwd": "finance###urgent"},
					{"tag_kwd": "finance"},
				},
			},
		},
	}
	ctx := t.Context()

	result, code, err := testDatasetServiceForListTags(t, docEngine).ListTags(ctx, kbInput, "user-1")
	if err != nil {
		t.Fatalf("ListTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}
	if len(result) != 2 {
		t.Fatalf("len(result)=%d want=2 result=%v", len(result), result)
	}
	encoded, err := json.Marshal(result)
	if err != nil || string(encoded) != `[["finance",2],["urgent",1]]` {
		t.Fatalf("JSON=%s err=%v; want Python tag/count pairs", encoded, err)
	}
	if len(docEngine.requests) != 1 {
		t.Fatalf("search requests=%d want=1", len(docEngine.requests))
	}
	req := docEngine.requests[0]
	if len(req.IndexNames) != 1 || req.IndexNames[0] != "ragflow_user-1" {
		t.Fatalf("IndexNames=%v want [ragflow_user-1]", req.IndexNames)
	}
	if len(req.KbIDs) != 1 || req.KbIDs[0] != kbID {
		t.Fatalf("KbIDs=%v want [%s]", req.KbIDs, kbID)
	}
}

func TestDatasetServiceListTagsReturnsEmptyWhenChunkStoreMissing(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "123e4567-e89b-12d3-a456-426614174000"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertListTagsKB(t, kbID, "user-1", string(entity.TenantPermissionMe), 1)

	docEngine := &listTagsMockEngine{chunkStoreExists: false}

	ctx := t.Context()

	result, code, err := testDatasetServiceForListTags(t, docEngine).ListTags(ctx, kbInput, "user-1")
	if err != nil {
		t.Fatalf("ListTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}
	if len(result) != 0 {
		t.Fatalf("len(result)=%d want=0 result=%v", len(result), result)
	}
	encoded, err := json.Marshal(result)
	if err != nil || string(encoded) != `[]` {
		t.Fatalf("JSON=%s err=%v; want empty array", encoded, err)
	}
	if len(docEngine.requests) != 0 {
		t.Fatalf("search requests=%d want=0", len(docEngine.requests))
	}
}

func TestDatasetServiceListTagsRejectsUnauthorizedDataset(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "123e4567-e89b-12d3-a456-426614174000"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertListTagsKB(t, kbID, "tenant-9", string(entity.TenantPermissionMe), 1)

	docEngine := &listTagsMockEngine{chunkStoreExists: true}

	ctx := t.Context()

	_, code, err := testDatasetServiceForListTags(t, docEngine).ListTags(ctx, kbInput, "user-1")
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	if code != common.CodeDataError {
		t.Fatalf("code=%d want=%d", code, common.CodeDataError)
	}
	if err.Error() != "no authorization" {
		t.Fatalf("error=%q want=%q", err.Error(), "no authorization")
	}
}

func TestDatasetServiceListTagsReturnsChunkStoreError(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "123e4567-e89b-12d3-a456-426614174000"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertListTagsKB(t, kbID, "user-1", string(entity.TenantPermissionMe), 1)

	docEngine := &listTagsMockEngine{
		chunkStoreErr: errors.New("boom"),
	}

	ctx := t.Context()

	_, code, err := testDatasetServiceForListTags(t, docEngine).ListTags(ctx, kbInput, "user-1")
	if err == nil {
		t.Fatal("expected chunk store error")
	}
	if code != common.CodeServerError {
		t.Fatalf("code=%d want=%d", code, common.CodeServerError)
	}
	if !strings.Contains(err.Error(), "failed to inspect chunk store: boom") {
		t.Fatalf("err=%q want contains %q", err.Error(), "failed to inspect chunk store: boom")
	}
}

func TestDatasetServiceListTagsSearchFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		engine *listTagsMockEngine
	}{
		{"search error", &listTagsMockEngine{chunkStoreExists: true, searchErr: errors.New("backend unavailable")}},
		{"nil result", &listTagsMockEngine{chunkStoreExists: true, searchResults: map[string]*types.SearchResult{"ragflow_user-1": nil}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			kbID := "123e4567e89b12d3a456426614174000"
			insertListTagsKB(t, kbID, "user-1", string(entity.TenantPermissionMe), 1)
			result, code, err := testDatasetServiceForListTags(t, tc.engine).ListTags(t.Context(), kbID, "user-1")
			if err == nil || code != common.CodeServerError || result != nil {
				t.Fatalf("result=%v code=%d err=%v; want server error without partial data", result, code, err)
			}
		})
	}
}
