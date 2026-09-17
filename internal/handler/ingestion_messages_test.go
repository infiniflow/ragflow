package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	dataset "ragflow/internal/service/dataset"
)

func TestDatasetsHandlerListIngestionMessagesValidatesAndPages(t *testing.T) {
	db := setupIngestionMessagesHandlerDB(t)
	insertIngestionMessagesHandlerKB(t, db, "kb-1", "user-1")
	runCount := 1
	if err := db.Create(&entity.PipelineOperationLog{
		ID:              "run-1",
		DocumentID:      "doc-1",
		TenantID:        "user-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		DocumentName:    "doc.txt",
		DocumentSuffix:  ".txt",
		DocumentType:    "text",
		SourceFrom:      "local",
		TaskType:        string(entity.PipelineTaskTypeParse),
		OperationStatus: string(entity.TaskStatusRunning),
		RunCount:        &runCount,
	}).Error; err != nil {
		t.Fatalf("insert run: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := db.Model(&entity.IngestionTaskLog{}).Create(map[string]interface{}{
			"task_id":         "task-1",
			"pipeline_log_id": "run-1",
			"checkpoint":      entity.JSONMap{},
			"event_type":      dao.EventTypeMessage,
			"component":       "",
			"phase":           0,
			"message":         "detail",
		}).Error; err != nil {
			t.Fatalf("insert event: %v", err)
		}
	}

	router := newIngestionMessagesHandlerRouter()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/datasets/kb-1/ingestions/run-1/messages?limit=2", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Code common.ErrorCode `json:"code"`
		Data struct {
			Items []struct {
				ID int `json:"id"`
			} `json:"items"`
			OldestID      int  `json:"oldest_id"`
			NewestID      int  `json:"newest_id"`
			HasMoreBefore bool `json:"has_more_before"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal success response: %v", err)
	}
	if body.Code != common.CodeSuccess || len(body.Data.Items) != 2 || body.Data.Items[0].ID != 2 || body.Data.Items[1].ID != 3 || body.Data.OldestID != 2 || body.Data.NewestID != 3 || !body.Data.HasMoreBefore {
		t.Fatalf("success response = %+v, want latest two events", body)
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/datasets/kb-1/ingestions/run-1/messages?after_id=1&before_id=2", nil))
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal invalid response: %v", err)
	}
	if body.Code != common.CodeArgumentError {
		t.Fatalf("invalid cursor code = %d, want %d", body.Code, common.CodeArgumentError)
	}
}

func newIngestionMessagesHandlerRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewDatasetsHandler(dataset.NewDatasetService(), nil)
	r := gin.New()
	r.GET("/api/v1/datasets/:dataset_id/ingestions/:log_id/messages", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-1"})
		h.ListIngestionMessages(c)
	})
	return r
}

func setupIngestionMessagesHandlerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.PipelineOperationLog{}, &entity.IngestionTaskLog{}, &entity.User{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate test schema: %v", err)
	}
	original := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = original })
	return db
}

func insertIngestionMessagesHandlerKB(t *testing.T, db *gorm.DB, id, userID string) {
	t.Helper()
	status := string(entity.StatusValid)
	if err := db.Create(&entity.Knowledgebase{
		ID:           id,
		TenantID:     userID,
		Name:         "messages",
		EmbdID:       "BAAI/bge-large-zh-v1.5@Builtin",
		CreatedBy:    userID,
		Permission:   string(entity.TenantPermissionMe),
		ParserID:     string(entity.ParserTypeNaive),
		ParserConfig: entity.JSONMap{},
		Status:       &status,
	}).Error; err != nil {
		t.Fatalf("insert knowledgebase: %v", err)
	}
}
