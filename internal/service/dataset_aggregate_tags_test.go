package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
)

type aggregateTagsMockEngine struct {
	engine.DocEngine
	searchResults      map[string]*types.SearchResult
	pagedSearchResults map[string]map[int]*types.SearchResult
	searchErr          error
	requests           []*types.SearchRequest
}

func (m *aggregateTagsMockEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
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
	if byOffset, ok := m.pagedSearchResults[req.IndexNames[0]]; ok {
		if res, ok := byOffset[req.Offset]; ok {
			return res, nil
		}
		return &types.SearchResult{}, nil
	}
	if res, ok := m.searchResults[req.IndexNames[0]]; ok {
		return res, nil
	}
	return &types.SearchResult{}, nil
}

func (m *aggregateTagsMockEngine) GetAggregation(chunks []map[string]interface{}, fieldName string) []map[string]interface{} {
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

func (m *aggregateTagsMockEngine) GetType() string { return "mock" }

func (m *aggregateTagsMockEngine) Ping(ctx context.Context) error { return nil }

func (m *aggregateTagsMockEngine) Close() error { return nil }

func testDatasetServiceForAggregateTags(t *testing.T, docEngine engine.DocEngine) *DatasetService {
	t.Helper()
	return &DatasetService{
		kbDAO:     dao.NewKnowledgebaseDAO(),
		docEngine: docEngine,
	}
}

func insertAggregateTagsKB(t *testing.T, datasetID, tenantID, permission string, docNum int64) {
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

func insertAggregateTagsMembership(t *testing.T, tenantID, userID string) {
	t.Helper()
	row := &entity.UserTenant{
		ID:        tenantID + "-" + userID,
		UserID:    userID,
		TenantID:  tenantID,
		Role:      "member",
		InvitedBy: tenantID,
		Status:    sptr(string(entity.StatusValid)),
	}
	if err := dao.DB.Create(row).Error; err != nil {
		t.Fatalf("insert user_tenant: %v", err)
	}
}

func aggregateTagsResultMap(rows []map[string]interface{}) map[string]int {
	result := make(map[string]int, len(rows))
	for _, row := range rows {
		tag, _ := row["value"].(string)
		count, _ := row["count"].(int)
		result[tag] = count
	}
	return result
}

func TestDatasetServiceAggregateTagsMergesAcrossTenants(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kb1Input := "123e4567-e89b-12d3-a456-426614174000"
	kb2Input := "223e4567-e89b-12d3-a456-426614174001"
	kb1ID := strings.ReplaceAll(kb1Input, "-", "")
	kb2ID := strings.ReplaceAll(kb2Input, "-", "")

	insertAggregateTagsKB(t, kb1ID, "user-1", string(entity.TenantPermissionMe), 2)
	insertAggregateTagsKB(t, kb2ID, "tenant-2", string(entity.TenantPermissionTeam), 1)
	insertAggregateTagsMembership(t, "tenant-2", "user-1")

	docEngine := &aggregateTagsMockEngine{
		searchResults: map[string]*types.SearchResult{
			"ragflow_user-1": {
				Chunks: []map[string]interface{}{
					{"tag_kwd": "finance###urgent"},
					{"tag_kwd": "finance"},
				},
			},
			"ragflow_tenant-2": {
				Chunks: []map[string]interface{}{
					{"tag_kwd": "urgent###internal"},
				},
			},
		},
	}

	result, code, err := testDatasetServiceForAggregateTags(t, docEngine).AggregateTags([]string{kb1Input, kb2Input}, "user-1")
	if err != nil {
		t.Fatalf("AggregateTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}

	got := aggregateTagsResultMap(result)
	want := map[string]int{
		"finance":  2,
		"urgent":   2,
		"internal": 1,
	}
	if len(got) != len(want) {
		t.Fatalf("result len=%d want=%d result=%v", len(got), len(want), got)
	}
	for tag, wantCount := range want {
		if got[tag] != wantCount {
			t.Fatalf("tag %q count=%d want=%d all=%v", tag, got[tag], wantCount, got)
		}
	}

	if len(docEngine.requests) != 2 {
		t.Fatalf("search requests=%d want=2", len(docEngine.requests))
	}

	requestByIndex := make(map[string]*types.SearchRequest, len(docEngine.requests))
	for _, req := range docEngine.requests {
		if len(req.IndexNames) != 1 {
			t.Fatalf("IndexNames=%v want single entry", req.IndexNames)
		}
		requestByIndex[req.IndexNames[0]] = req
		if req.Offset != 0 {
			t.Fatalf("Offset=%d want=0", req.Offset)
		}
		if req.Limit != 10000 {
			t.Fatalf("Limit=%d want=10000", req.Limit)
		}
		if len(req.SelectFields) != 1 || req.SelectFields[0] != "tag_kwd" {
			t.Fatalf("SelectFields=%v want [tag_kwd]", req.SelectFields)
		}
	}

	if req := requestByIndex["ragflow_user-1"]; req == nil || len(req.KbIDs) != 1 || req.KbIDs[0] != kb1ID {
		t.Fatalf("request for ragflow_user-1 = %#v, want kbIDs=[%s]", req, kb1ID)
	}
	if req := requestByIndex["ragflow_tenant-2"]; req == nil || len(req.KbIDs) != 1 || req.KbIDs[0] != kb2ID {
		t.Fatalf("request for ragflow_tenant-2 = %#v, want kbIDs=[%s]", req, kb2ID)
	}
}

func TestDatasetServiceAggregateTagsPagesThroughAllChunks(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "723e4567-e89b-12d3-a456-426614174006"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertAggregateTagsKB(t, kbID, "user-1", string(entity.TenantPermissionMe), 10002)

	firstPage := make([]map[string]interface{}, 10000)
	for i := range firstPage {
		firstPage[i] = map[string]interface{}{"tag_kwd": "finance"}
	}

	docEngine := &aggregateTagsMockEngine{
		pagedSearchResults: map[string]map[int]*types.SearchResult{
			"ragflow_user-1": {
				0: {
					Chunks: firstPage,
					Total:  10002,
				},
				10000: {
					Chunks: []map[string]interface{}{
						{"tag_kwd": "finance"},
						{"tag_kwd": "urgent"},
					},
					Total: 10002,
				},
			},
		},
	}

	result, code, err := testDatasetServiceForAggregateTags(t, docEngine).AggregateTags([]string{kbInput}, "user-1")
	if err != nil {
		t.Fatalf("AggregateTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}

	got := aggregateTagsResultMap(result)
	if got["finance"] != 10001 {
		t.Fatalf("finance count=%d want=10001 result=%v", got["finance"], got)
	}
	if got["urgent"] != 1 {
		t.Fatalf("urgent count=%d want=1 result=%v", got["urgent"], got)
	}

	if len(docEngine.requests) != 2 {
		t.Fatalf("search requests=%d want=2", len(docEngine.requests))
	}
	if docEngine.requests[0].Offset != 0 {
		t.Fatalf("first request offset=%d want=0", docEngine.requests[0].Offset)
	}
	if docEngine.requests[1].Offset != 10000 {
		t.Fatalf("second request offset=%d want=10000", docEngine.requests[1].Offset)
	}
}

func TestDatasetServiceAggregateTagsRejectsUnauthorizedDataset(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "323e4567-e89b-12d3-a456-426614174002"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertAggregateTagsKB(t, kbID, "tenant-9", string(entity.TenantPermissionMe), 1)

	_, code, err := testDatasetServiceForAggregateTags(t, &aggregateTagsMockEngine{}).AggregateTags([]string{kbInput}, "user-1")
	if err == nil {
		t.Fatal("expected authorization error")
	}
	if code != common.CodeDataError {
		t.Fatalf("code=%d want=%d", code, common.CodeDataError)
	}
	if err.Error() != "No authorization for dataset '"+kbID+"'" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetServiceAggregateTagsRequiresDocumentEngine(t *testing.T) {
	_, code, err := testDatasetServiceForAggregateTags(t, nil).AggregateTags([]string{"123e4567-e89b-12d3-a456-426614174000"}, "user-1")
	if err == nil {
		t.Fatal("expected missing doc engine error")
	}
	if code != common.CodeServerError {
		t.Fatalf("code=%d want=%d", code, common.CodeServerError)
	}
	if err.Error() != "Document engine is not initialized" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetServiceAggregateTagsSkipsDatasetsWithoutDocuments(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	emptyInput := "423e4567-e89b-12d3-a456-426614174003"
	liveInput := "523e4567-e89b-12d3-a456-426614174004"
	emptyID := strings.ReplaceAll(emptyInput, "-", "")
	liveID := strings.ReplaceAll(liveInput, "-", "")

	insertAggregateTagsKB(t, emptyID, "user-1", string(entity.TenantPermissionMe), 0)
	insertAggregateTagsKB(t, liveID, "user-1", string(entity.TenantPermissionMe), 1)

	docEngine := &aggregateTagsMockEngine{
		searchResults: map[string]*types.SearchResult{
			"ragflow_user-1": {
				Chunks: []map[string]interface{}{
					{"tag_kwd": "alpha###beta"},
				},
			},
		},
	}

	result, code, err := testDatasetServiceForAggregateTags(t, docEngine).AggregateTags([]string{emptyInput, liveInput}, "user-1")
	if err != nil {
		t.Fatalf("AggregateTags failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%d want=%d", code, common.CodeSuccess)
	}
	if len(docEngine.requests) != 1 {
		t.Fatalf("search requests=%d want=1", len(docEngine.requests))
	}
	if len(docEngine.requests[0].KbIDs) != 1 || docEngine.requests[0].KbIDs[0] != liveID {
		t.Fatalf("KbIDs=%v want [%s]", docEngine.requests[0].KbIDs, liveID)
	}

	got := aggregateTagsResultMap(result)
	if got["alpha"] != 1 || got["beta"] != 1 {
		t.Fatalf("unexpected result=%v", got)
	}
}

func TestDatasetServiceAggregateTagsReturnsSearchError(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	kbInput := "623e4567-e89b-12d3-a456-426614174005"
	kbID := strings.ReplaceAll(kbInput, "-", "")
	insertAggregateTagsKB(t, kbID, "user-1", string(entity.TenantPermissionMe), 1)

	docEngine := &aggregateTagsMockEngine{searchErr: errors.New("boom")}
	_, code, err := testDatasetServiceForAggregateTags(t, docEngine).AggregateTags([]string{kbInput}, "user-1")
	if err == nil {
		t.Fatal("expected search error")
	}
	if code != common.CodeServerError {
		t.Fatalf("code=%d want=%d", code, common.CodeServerError)
	}
	if !strings.Contains(err.Error(), "failed to aggregate tags: boom") {
		t.Fatalf("unexpected error: %v", err)
	}
}
