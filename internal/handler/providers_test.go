package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func setupProviderHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.UserTenant{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
	); err != nil {
		t.Fatalf("failed to migrate provider handler tables: %v", err)
	}
	return db
}

func useProviderHandlerTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })
}

func seedProviderHandlerModel(t *testing.T, db *gorm.DB) {
	t.Helper()
	activeStatus := "1"
	rows := []interface{}{
		&entity.UserTenant{ID: "user-tenant-1", UserID: "user-1", TenantID: "tenant-1", Role: "owner", InvitedBy: "user-1", Status: &activeStatus},
		&entity.TenantModelProvider{ID: "provider-1", TenantID: "tenant-1", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "instance-1", ProviderID: "provider-1", InstanceName: "default", APIKey: "sk-test", Status: "active", Extra: "{}"},
		&entity.TenantModel{ID: "model-1", ProviderID: "provider-1", InstanceID: "instance-1", ModelName: "gpt-test", ModelType: "chat", Status: "active"},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("failed to seed %T: %v", row, err)
		}
	}
}

func newProviderHandlerRequest(t *testing.T, body map[string]interface{}, params ...gin.Param) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/providers/OpenAI/instances/default/models/gpt-test", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = req
	ctx.Params = params
	ctx.Set("user_id", "user-1")
	return ctx, recorder
}

func decodeProviderHandlerResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
	return body
}

func TestProviderHandlerEnableOrDisableModelRejectsMissingModelSelector(t *testing.T) {
	ctx, recorder := newProviderHandlerRequest(
		t,
		map[string]interface{}{"status": "active"},
		gin.Param{Key: "provider_name", Value: "OpenAI"},
		gin.Param{Key: "instance_name", Value: "default"},
	)

	NewProviderHandler(nil, service.NewModelProviderService()).EnableOrDisableModel(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	body := decodeProviderHandlerResponse(t, recorder)
	if common.ErrorCode(body["code"].(float64)) != common.CodeBadRequest {
		t.Fatalf("code = %v, want %v", body["code"], common.CodeBadRequest)
	}
}

func TestProviderHandlerEnableOrDisableModelRejectsInvalidStatus(t *testing.T) {
	ctx, recorder := newProviderHandlerRequest(
		t,
		map[string]interface{}{"status": "disabled"},
		gin.Param{Key: "provider_name", Value: "OpenAI"},
		gin.Param{Key: "instance_name", Value: "default"},
		gin.Param{Key: "model_name", Value: "gpt-test"},
	)

	NewProviderHandler(nil, service.NewModelProviderService()).EnableOrDisableModel(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	body := decodeProviderHandlerResponse(t, recorder)
	if common.ErrorCode(body["code"].(float64)) != common.CodeBadRequest {
		t.Fatalf("code = %v, want %v", body["code"], common.CodeBadRequest)
	}
}

func TestProviderHandlerEnableOrDisableModelUpdatesStatus(t *testing.T) {
	db := setupProviderHandlerTestDB(t)
	useProviderHandlerTestDB(t, db)
	seedProviderHandlerModel(t, db)

	ctx, recorder := newProviderHandlerRequest(
		t,
		map[string]interface{}{"status": "inactive"},
		gin.Param{Key: "provider_name", Value: "OpenAI"},
		gin.Param{Key: "instance_name", Value: "default"},
		gin.Param{Key: "model_name", Value: "gpt-test"},
	)

	NewProviderHandler(nil, service.NewModelProviderService()).EnableOrDisableModel(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	body := decodeProviderHandlerResponse(t, recorder)
	if common.ErrorCode(body["code"].(float64)) != common.CodeSuccess {
		t.Fatalf("code = %v, want %v", body["code"], common.CodeSuccess)
	}

	var got entity.TenantModel
	if err := db.Where("id = ?", "model-1").First(&got).Error; err != nil {
		t.Fatalf("failed to reload model: %v", err)
	}
	if got.Status != "inactive" {
		t.Fatalf("status = %q, want inactive", got.Status)
	}
}
