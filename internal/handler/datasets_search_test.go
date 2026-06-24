package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupDatasetsSearchHandlerDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.UserTenant{}, &entity.Search{}); err != nil {
		t.Fatalf("failed to migrate sqlite: %v", err)
	}
	return db
}

func pushDatasetsSearchHandlerDB(t *testing.T, testDB *gorm.DB) {
	t.Helper()

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() {
		dao.DB = orig
	})
}

func setupDatasetSearchRouter(t *testing.T, userID string) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: userID})
	})
	h := NewDatasetsHandler(service.NewDatasetService(), nil)
	r.POST("/api/v1/datasets/:dataset_id/search", h.SearchDataset)
	return r
}

func TestDatasetsHandlerSearchDatasetRejectsMissingQuestion(t *testing.T) {
	db := setupDatasetsSearchHandlerDB(t)
	pushDatasetsSearchHandlerDB(t, db)

	r := setupDatasetSearchRouter(t, "tenant-1")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets/kb-1/search", strings.NewReader(`{"question":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if resp["code"] != float64(101) {
		t.Fatalf("code = %v, want 101", resp["code"])
	}
	if resp["message"] != "question is required" {
		t.Fatalf("message = %v", resp["message"])
	}
}
