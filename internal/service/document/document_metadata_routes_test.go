package document_test

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestMetadataBatchRoutesPersistOperationConditions(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/datasets/ds-1/metadata/update"},
		{http.MethodPatch, "/api/v1/datasets/ds-1/documents/metadatas"},
	}
	tests := []struct {
		name    string
		initial map[string]any
		body    string
		want    map[string]any
	}{
		{
			name: "keeps_published_status_and_deletes_only_old_tag",
			initial: map[string]any{
				"status": "published", "tags": []any{"old", "keep"},
			},
			body: `{"selector":{"document_ids":["doc-1"]},
				"updates":[{"key":"status","value":"archived","match":"draft"}],
				"deletes":[{"key":"tags","value":"old"}]}`,
			want: map[string]any{"status": "published", "tags": []any{"keep"}},
		},
		{
			name: "preserves_match_and_delete_value",
			initial: map[string]any{
				"tags": []any{"old", "keep"}, "mismatch": "keep", "status": "draft",
				"author": "alice", "remove": []any{"drop", "keep"}, "keep": "keep",
				"zero": 0, "false": false, "number_keep": 2, "boolean_keep": true,
			},
			body: `{"selector":{"document_ids":["doc-1"]},"updates":[
				{"key":"tags","value":"new","match":"old"},
				{"key":"mismatch","value":"wrong","match":"other"},
				{"key":"missing","value":"wrong","match":"other"},
				{"key":"status","value":"done","match":"draft"},
				{"key":"zero","value":1,"match":0},
				{"key":"false","value":true,"match":false},
				{"key":"number_keep","value":3,"match":0},
				{"key":"boolean_keep","value":false,"match":false}
			],"deletes":[
				{"key":"author","value":"alice"},
				{"key":"remove","value":"drop"},
				{"key":"keep","value":"other"}
			]}`,
			want: map[string]any{
				"tags": []any{"new", "keep"}, "mismatch": "keep", "status": "done",
				"remove": []any{"keep"}, "keep": "keep",
				"zero": float64(1), "false": true, "number_keep": 2, "boolean_keep": true,
			},
		},
		{
			name: "missing_null_and_empty_match_are_unconditional",
			initial: map[string]any{
				"missing": "old", "null": "old", "empty": "old",
				"missing_list": []any{"old"}, "null_list": []any{"old"}, "empty_list": []any{"old"},
			},
			body: `{"selector":{"document_ids":["doc-1"]},"updates":[
				{"key":"missing","value":"new"},
				{"key":"null","value":"new","match":null},
				{"key":"empty","value":"new","match":""},
				{"key":"missing_list","value":"new"},
				{"key":"null_list","value":"new","match":null},
				{"key":"empty_list","value":"new","match":""},
				{"key":"missing_new","value":"new"},
				{"key":"null_new","value":"new","match":null},
				{"key":"empty_new","value":"new","match":""}
			]}`,
			want: map[string]any{
				"missing": "new", "null": "new", "empty": "new",
				"missing_list": []any{"old", "new"}, "null_list": []any{"old", "new"}, "empty_list": []any{"old", "new"},
				"missing_new": "new", "null_new": "new", "empty_new": "new",
			},
		},
		{
			name: "missing_null_delete_whole_key_but_empty_value_compares",
			initial: map[string]any{
				"missing": []any{"old", "keep"}, "null": []any{"old", "keep"},
				"empty_list": []any{"", "keep"}, "empty_scalar": "", "keep_scalar": "keep",
			},
			body: `{"selector":{"document_ids":["doc-1"]},"deletes":[
				{"key":"missing"}, {"key":"null","value":null},
				{"key":"empty_list","value":""}, {"key":"empty_scalar","value":""},
				{"key":"keep_scalar","value":""}
			]}`,
			want: map[string]any{"empty_list": []any{"keep"}, "keep_scalar": "keep"},
		},
	}

	originalMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(originalMode) })
	for _, route := range routes {
		t.Run(route.method, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					setupMetadataRouteDB(t)
					store := &metadataRouteStore{fields: maps.Clone(tt.initial)}
					svc := document.NewMetadataDocumentServiceForTest(store)
					h := handler.NewDocumentHandler(svc, dataset.NewDatasetService(), nil)
					router := gin.New()
					router.Use(func(c *gin.Context) { c.Set("user", &entity.User{ID: "user-1"}) })
					router.POST("/api/v1/datasets/:dataset_id/metadata/update", h.MetadataBatchUpdate)
					router.PATCH("/api/v1/datasets/:dataset_id/documents/metadatas", h.UpdateDocumentMetadatas)

					request := httptest.NewRequest(route.method, route.path, strings.NewReader(tt.body))
					request.Header.Set("Content-Type", "application/json")
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					var payload struct {
						Code common.ErrorCode                      `json:"code"`
						Data document.BatchUpdateMetadatasResponse `json:"data"`
					}
					if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
						t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
					}
					if response.Code != http.StatusOK || payload.Code != common.CodeSuccess || payload.Data.Updated != 1 || payload.Data.MatchedDocs != 1 {
						t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body.String())
					}
					got, err := svc.GetDocumentMetadataByID(t.Context(), "doc-1")
					if err != nil {
						t.Fatalf("read persisted metadata: %v", err)
					}
					if !reflect.DeepEqual(got, tt.want) {
						t.Fatalf("persisted metadata = %#v, want %#v", got, tt.want)
					}
				})
			}
		})
	}
}

func setupMetadataRouteDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&entity.Document{}, &entity.Knowledgebase{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	status := string(entity.StatusValid)
	if err := db.Create(&entity.Knowledgebase{ID: "ds-1", TenantID: "user-1", Name: "metadata", EmbdID: "unused", Status: &status}).Error; err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if err := db.Create(&entity.Document{ID: "doc-1", KbID: "ds-1", ParserID: "naive", ParserConfig: entity.JSONMap{}, Status: &status}).Error; err != nil {
		t.Fatalf("create document: %v", err)
	}
	original := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = original })
}

// metadataRouteStore persists engine writes; operation conditions are evaluated
// exclusively by the real DocumentService, not by this storage fake.
type metadataRouteStore struct {
	engine.DocEngine
	fields map[string]any
}

func (s *metadataRouteStore) SearchMetadata(_ context.Context, request *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	if request.TenantID != "user-1" || request.Filter["kb_id"] != "ds-1" || !reflect.DeepEqual(request.Filter["id"], []string{"doc-1"}) {
		return nil, fmt.Errorf("unexpected metadata search: %#v", request)
	}
	return &types.SearchMetadataResult{MetadataRecords: []map[string]any{{"id": "doc-1", "kb_id": "ds-1", "meta_fields": s.fields}}}, nil
}

func (s *metadataRouteStore) MetadataStoreExists(context.Context, string) (bool, error) {
	return true, nil
}

func (s *metadataRouteStore) UpdateMetadata(_ context.Context, _, _ string, fields map[string]any, _ string) error {
	s.fields = maps.Clone(fields)
	return nil
}

func (s *metadataRouteStore) DeleteMetadataKeys(_ context.Context, _, _ string, keys []string, _ string) error {
	for _, key := range keys {
		delete(s.fields, key)
	}
	return nil
}
