package document_test

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/handler"
	"ragflow/internal/service/dataset"
	"ragflow/internal/service/document"
)

func TestMetadataSelectionRoutesPersistDatasetScope(t *testing.T) {
	matchingCondition := `{"metadata_condition":{"conditions":[{"name":"author","comparison_operator":"is","value":"alice"}]}}`
	tests := []struct {
		name     string
		selector string
		dataset  string
		wantIDs  []string
		wantCode common.ErrorCode
	}{
		{name: "omitted_selector", wantIDs: []string{"doc-1", "doc-2", "doc-3"}},
		{name: "empty_selector", selector: `{}`, wantIDs: []string{"doc-1", "doc-2", "doc-3"}},
		{name: "null_selector", selector: `null`, wantIDs: []string{"doc-1", "doc-2", "doc-3"}},
		{name: "null_selector_fields", selector: `{"document_ids":null,"metadata_condition":null}`, wantIDs: []string{"doc-1", "doc-2", "doc-3"}},
		{name: "explicit_empty_ids", selector: `{"document_ids":[]}`},
		{name: "empty_ids_with_matching_condition", selector: `{"document_ids":[],"metadata_condition":{"conditions":[{"name":"author","comparison_operator":"is","value":"alice"}]}}`},
		{name: "matching_condition", selector: matchingCondition, wantIDs: []string{"doc-1"}},
		{name: "zero_matching_condition", selector: `{"metadata_condition":{"conditions":[{"name":"author","comparison_operator":"is","value":"nobody"}]}}`},
		{name: "empty_condition_object", selector: `{"metadata_condition":{}}`},
		{name: "empty_conditions", selector: `{"metadata_condition":{"conditions":[]}}`},
		{name: "null_conditions", selector: `{"metadata_condition":{"conditions":null}}`},
		{name: "ids_and_condition_intersection", selector: `{"document_ids":["doc-1","doc-2"],"metadata_condition":{"conditions":[{"name":"author","comparison_operator":"is","value":"alice"}]}}`, wantIDs: []string{"doc-1"}},
		{name: "empty_intersection", selector: `{"document_ids":["doc-2"],"metadata_condition":{"conditions":[{"name":"author","comparison_operator":"is","value":"alice"}]}}`},
		{name: "ids_with_empty_condition_object", selector: `{"document_ids":["doc-2"],"metadata_condition":{}}`, wantIDs: []string{"doc-2"}},
		{name: "foreign_document_id", selector: `{"document_ids":["doc-other"]}`, wantCode: common.CodeDataError},
		{name: "inaccessible_dataset", dataset: "dataset-foreign", wantCode: common.CodeDataError},
		{name: "empty_dataset", dataset: "dataset-empty"},
	}
	routes := []struct{ method, suffix string }{
		{http.MethodPost, "metadata/update"},
		{http.MethodPatch, "documents/metadatas"},
	}
	originalMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(originalMode) })
	for _, route := range routes {
		t.Run(route.method, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					store := setupMetadataSelectionRoute(t)
					before := make(map[string]map[string]any, len(store.rows))
					for id, row := range store.rows {
						before[id] = maps.Clone(row.fields)
					}
					svc := document.NewMetadataSelectionServiceForTest(store)
					h := handler.NewDocumentHandler(svc, dataset.NewDatasetService(), nil)
					router := gin.New()
					router.Use(func(c *gin.Context) { c.Set("user", &entity.User{ID: "user-1"}) })
					router.POST("/api/v1/datasets/:dataset_id/metadata/update", h.MetadataBatchUpdate)
					router.PATCH("/api/v1/datasets/:dataset_id/documents/metadatas", h.UpdateDocumentMetadatas)
					body := `{"updates":[{"key":"category","value":"paper"}],"deletes":[{"key":"obsolete"}]`
					if tt.selector != "" {
						body += `,"selector":` + tt.selector
					}
					body += `}`
					datasetID := tt.dataset
					if datasetID == "" {
						datasetID = "dataset-1"
					}
					request := httptest.NewRequest(route.method, "/api/v1/datasets/"+datasetID+"/"+route.suffix, strings.NewReader(body))
					request.Header.Set("Content-Type", "application/json")
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					var payload struct {
						Code common.ErrorCode                       `json:"code"`
						Data *document.BatchUpdateMetadatasResponse `json:"data"`
					}
					if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
						t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
					}
					if response.Code != http.StatusOK || payload.Code != tt.wantCode {
						t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body.String())
					}
					if tt.wantCode == common.CodeSuccess && (payload.Data == nil || payload.Data.Updated != len(tt.wantIDs) || payload.Data.MatchedDocs != len(tt.wantIDs)) {
						t.Errorf("result = %#v, want updated=%d matched=%d", payload.Data, len(tt.wantIDs), len(tt.wantIDs))
					}
					if tt.wantCode != common.CodeSuccess && store.searches != 0 {
						t.Errorf("rejected request performed %d metadata searches", store.searches)
					}
					for id, row := range store.rows {
						want := maps.Clone(before[id])
						selected := slices.Contains(tt.wantIDs, id)
						if selected {
							want["category"] = "paper"
							delete(want, "obsolete")
						}
						if !reflect.DeepEqual(row.fields, want) {
							t.Errorf("stored metadata for %s = %#v, want %#v", id, row.fields, want)
						}
						if store.written[id] != selected {
							t.Errorf("write for %s = %v, want %v", id, store.written[id], selected)
						}
						if row.dataset == "dataset-1" {
							got, err := svc.GetDocumentMetadataByID(t.Context(), id)
							if err != nil || !reflect.DeepEqual(got, want) {
								t.Errorf("service readback for %s = %#v, err=%v, want %#v", id, got, err, want)
							}
						}
					}
				})
			}
		})
	}
}

type metadataSelectionRow struct {
	dataset string
	tenant  string
	fields  map[string]any
}

// This fake only stores and queries metadata. Selection and mutation semantics
// are exercised by the real handler, DAO, and DocumentService.
type metadataSelectionStore struct {
	engine.DocEngine
	rows     map[string]*metadataSelectionRow
	written  map[string]bool
	searches int
}

func setupMetadataSelectionRoute(t *testing.T) *metadataSelectionStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&entity.Document{}, &entity.Knowledgebase{}, &entity.UserTenant{}); err != nil {
		t.Fatal(err)
	}
	status := string(entity.StatusValid)
	for _, kb := range []entity.Knowledgebase{
		{ID: "dataset-1", TenantID: "user-1"}, {ID: "dataset-other", TenantID: "user-1"},
		{ID: "dataset-foreign", TenantID: "user-2"}, {ID: "dataset-empty", TenantID: "user-1"},
	} {
		kb.Name, kb.EmbdID, kb.Status = kb.ID, "unused", &status
		if err := db.Create(&kb).Error; err != nil {
			t.Fatal(err)
		}
	}
	store := &metadataSelectionStore{
		rows: map[string]*metadataSelectionRow{
			"doc-1":       {"dataset-1", "user-1", map[string]any{"author": "alice", "obsolete": "old"}},
			"doc-2":       {"dataset-1", "user-1", map[string]any{"author": "bob", "obsolete": "old"}},
			"doc-3":       {"dataset-1", "user-1", map[string]any{}},
			"doc-other":   {"dataset-other", "user-1", map[string]any{"author": "alice", "obsolete": "old"}},
			"doc-foreign": {"dataset-foreign", "user-2", map[string]any{"author": "alice", "obsolete": "old"}},
		},
		written: make(map[string]bool),
	}
	for id, row := range store.rows {
		if err := db.Create(&entity.Document{ID: id, KbID: row.dataset, ParserID: "naive", ParserConfig: entity.JSONMap{}, Status: &status}).Error; err != nil {
			t.Fatal(err)
		}
	}
	originalDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = originalDB })
	return store
}

func (s *metadataSelectionStore) SearchMetadata(_ context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	s.searches++
	var datasets []string
	switch value := req.Filter["kb_id"].(type) {
	case string:
		datasets = []string{value}
	case []string:
		datasets = value
	default:
		return nil, fmt.Errorf("unexpected dataset filter: %#v", req.Filter)
	}
	ids, restrictIDs := req.Filter["id"].([]string)
	result := &types.SearchMetadataResult{}
	for id, row := range s.rows {
		if row.tenant != req.TenantID || !slices.Contains(datasets, row.dataset) || len(row.fields) == 0 || (restrictIDs && !slices.Contains(ids, id)) {
			continue
		}
		result.MetadataRecords = append(result.MetadataRecords, map[string]any{"id": id, "kb_id": row.dataset, "meta_fields": maps.Clone(row.fields)})
	}
	return result, nil
}

func (s *metadataSelectionStore) MetadataStoreExists(context.Context, string) (bool, error) {
	return true, nil
}

func (s *metadataSelectionStore) UpdateMetadata(_ context.Context, id, datasetID string, fields map[string]any, tenantID string) error {
	row := s.rows[id]
	if row == nil || row.dataset != datasetID || row.tenant != tenantID {
		return fmt.Errorf("incorrect metadata write scope: id=%s dataset=%s tenant=%s", id, datasetID, tenantID)
	}
	s.written[id] = true
	row.fields = maps.Clone(fields)
	return nil
}

func (s *metadataSelectionStore) DeleteMetadataKeys(_ context.Context, id, datasetID string, keys []string, tenantID string) error {
	row := s.rows[id]
	if row == nil || row.dataset != datasetID || row.tenant != tenantID {
		return fmt.Errorf("incorrect metadata delete scope: id=%s dataset=%s tenant=%s", id, datasetID, tenantID)
	}
	s.written[id] = true
	for _, key := range keys {
		delete(row.fields, key)
	}
	return nil
}
