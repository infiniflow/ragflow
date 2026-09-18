package handler

import (
	"encoding/json"
	"fmt"
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

const (
	listDatasetsTestKBID       = "123e4567e89b12d3a456426614174000"
	listDatasetsTestKBIDDashed = "123e4567-e89b-12d3-a456-426614174000"
)

func setupListDatasetsTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.User{}, &entity.UserTenant{}, &entity.Tenant{}); err != nil {
		t.Fatalf("failed to migrate test schema: %v", err)
	}

	origDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = origDB })

	return db
}

func insertListDatasetsTestKB(t *testing.T, id, tenantID, name string) {
	t.Helper()

	status := string(entity.StatusValid)
	kb := &entity.Knowledgebase{
		ID:           id,
		TenantID:     tenantID,
		Name:         name,
		EmbdID:       "BAAI/bge-large-zh-v1.5@Builtin",
		CreatedBy:    tenantID,
		Permission:   string(entity.TenantPermissionMe),
		ParserID:     string(entity.ParserTypeNaive),
		ParserConfig: entity.JSONMap{"chunk_token_num": float64(128)},
		Status:       &status,
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert test kb: %v", err)
	}
}

func newListDatasetsTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewDatasetsHandler(dataset.NewDatasetService(), nil)
	r := gin.New()
	r.GET("/api/v1/datasets", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-1"})
		h.ListDatasets(c)
	})
	return r
}

type listDatasetsTestResponse struct {
	Code          int                      `json:"code"`
	Data          []map[string]interface{} `json:"data"`
	Message       string                   `json:"message"`
	TotalDatasets int64                    `json:"total_datasets"`
}

func getListDatasets(t *testing.T, r *gin.Engine, rawQuery string) listDatasetsTestResponse {
	t.Helper()

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/datasets?"+rawQuery, nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}

	var body listDatasetsTestResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, resp.Body.String())
	}
	return body
}

func TestDatasetsHandlerListDatasetsFiltersByIDs(t *testing.T) {
	setupListDatasetsTestDB(t)
	insertListDatasetsTestKB(t, listDatasetsTestKBID, "user-1", "Alpha")

	body := getListDatasets(t, newListDatasetsTestRouter(),
		fmt.Sprintf("ids=%s&page_size=1", listDatasetsTestKBIDDashed))

	if body.Code != int(common.CodeSuccess) {
		t.Fatalf("code=%d message=%q", body.Code, body.Message)
	}
	if body.TotalDatasets != 1 || len(body.Data) != 1 {
		t.Fatalf("expected exactly one dataset, got total=%d len=%d", body.TotalDatasets, len(body.Data))
	}
	if body.Data[0]["id"] != listDatasetsTestKBID {
		t.Fatalf("expected dataset id %q, got %#v", listDatasetsTestKBID, body.Data[0]["id"])
	}
}

func TestDatasetsHandlerListDatasetsRejectsInvalidIDInIDs(t *testing.T) {
	setupListDatasetsTestDB(t)

	body := getListDatasets(t, newListDatasetsTestRouter(), "ids=not-a-uuid")

	if body.Code != int(common.CodeArgumentError) {
		t.Fatalf("code=%d want=%d", body.Code, common.CodeArgumentError)
	}
	if body.Message != "Invalid UUID format" {
		t.Fatalf("message=%q want=%q", body.Message, "Invalid UUID format")
	}
}

func TestDatasetsHandlerListDatasetsRejectsDuplicateIDs(t *testing.T) {
	setupListDatasetsTestDB(t)

	rawQuery := fmt.Sprintf("ids=%s,%s", listDatasetsTestKBIDDashed, listDatasetsTestKBIDDashed)
	body := getListDatasets(t, newListDatasetsTestRouter(), rawQuery)

	if body.Code != int(common.CodeArgumentError) {
		t.Fatalf("code=%d want=%d", body.Code, common.CodeArgumentError)
	}
	expected := fmt.Sprintf("Duplicate ids: '%s'", listDatasetsTestKBID)
	if body.Message != expected {
		t.Fatalf("message=%q want=%q", body.Message, expected)
	}
}

// `sort` is a query key like any other on this endpoint, and the key gate runs
// before the ordering is read, so the gate has to know the name.
func TestDatasetsHandlerListDatasetsAcceptsSort(t *testing.T) {
	setupListDatasetsTestDB(t)
	insertListDatasetsTestKB(t, listDatasetsTestKBID, "user-1", "Alpha")

	body := getListDatasets(t, newListDatasetsTestRouter(), "sort=update_time:asc")

	if body.Code != int(common.CodeSuccess) {
		t.Fatalf("code=%d message=%q", body.Code, body.Message)
	}
	if body.TotalDatasets != 1 {
		t.Fatalf("expected the dataset to be listed, got total=%d", body.TotalDatasets)
	}
}

// A request `sort` can order is not rejected for the spelling of an `orderby` or
// `desc` that will not be read, while the same values without `sort` still are.
func TestDatasetsHandlerListDatasetsLegacyOrderingValidation(t *testing.T) {
	cases := []struct {
		name     string
		rawQuery string
		wantCode common.ErrorCode
		wantMsg  string
	}{
		{
			name:     "an unrecognised orderby alone is rejected",
			rawQuery: "orderby=nonsense",
			wantCode: common.CodeArgumentError,
			wantMsg:  "Input should be 'create_time' or 'update_time'",
		},
		{
			name:     "an unparseable desc alone is rejected",
			rawQuery: "desc=maybe",
			wantCode: common.CodeArgumentError,
			wantMsg:  "Input should be a valid boolean, unable to interpret input",
		},
		{
			name:     "sort carries an unrecognised orderby",
			rawQuery: "sort=update_time:asc&orderby=nonsense",
			wantCode: common.CodeSuccess,
		},
		{
			name:     "sort carries an unparseable desc",
			rawQuery: "sort=update_time:asc&desc=maybe",
			wantCode: common.CodeSuccess,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupListDatasetsTestDB(t)
			insertListDatasetsTestKB(t, listDatasetsTestKBID, "user-1", "Alpha")

			body := getListDatasets(t, newListDatasetsTestRouter(), tc.rawQuery)

			if body.Code != int(tc.wantCode) {
				t.Fatalf("code=%d want=%d message=%q", body.Code, tc.wantCode, body.Message)
			}
			if tc.wantMsg != "" && body.Message != tc.wantMsg {
				t.Fatalf("message=%q want=%q", body.Message, tc.wantMsg)
			}
		})
	}
}
