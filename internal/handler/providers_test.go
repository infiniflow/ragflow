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
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service"
)

func TestListProviderModelsRequestAcceptsStringAPIKeyAndExtensions(t *testing.T) {
	var request listProviderModelsRequest
	err := json.Unmarshal([]byte(`{"api_key":"token","extensions":{"endpoint_type":"runtime"}}`), &request)
	if err != nil {
		t.Fatal(err)
	}
	if request.APIKey != "token" || request.Extensions["endpoint_type"] != "runtime" {
		t.Fatalf("request=%+v", request)
	}
}

func TestListProviderModelsRequestRejectsObjectAPIKey(t *testing.T) {
	var request listProviderModelsRequest
	if err := json.Unmarshal([]byte(`{"api_key":{"api_key":"token"}}`), &request); err == nil {
		t.Fatal("object api_key was accepted")
	}
}

func TestIsBedrockAPIKeyConfig(t *testing.T) {
	for _, providerName := range []string{"Bedrock", "bedrock", "BEDROCK"} {
		if !isBedrockAPIKeyConfig(providerName, `{"auth_mode":"bedrock_api_key","bedrock_api_key":"token"}`) {
			t.Fatalf("%s API key mode was not detected", providerName)
		}
	}
	if isBedrockAPIKeyConfig("Bedrock", `{"auth_mode":"access_key_secret"}`) {
		t.Fatal("SigV4 mode was misclassified as API key mode")
	}
}

func TestShouldListRemoteModelsKeepsDefaultEndpointBedrockOnly(t *testing.T) {
	if shouldListRemoteModels("token", "", false) {
		t.Fatal("non-Bedrock provider without base URL triggered remote discovery")
	}
	if !shouldListRemoteModels("token", "https://example.com", false) {
		t.Fatal("provider with API key and base URL did not trigger remote discovery")
	}
	if !shouldListRemoteModels("token", "", true) {
		t.Fatal("Bedrock API key did not use the default endpoint")
	}
}

func TestProviderListModelItemUsesFrontendContract(t *testing.T) {
	item := providerListModelItem(modelModule.ListModelResponse{Name: "model", ModelTypes: []string{"chat"}})
	if item["max_tokens"] != 8192 {
		t.Fatalf("max_tokens=%v, want 8192", item["max_tokens"])
	}
	if _, exists := item["max_output"]; exists {
		t.Fatal("legacy max_output field must not be emitted")
	}
	emptyTypes := providerListModelItem(modelModule.ListModelResponse{Name: "empty"})["model_types"]
	modelTypes, ok := emptyTypes.([]string)
	if !ok || len(modelTypes) != 0 {
		t.Fatalf("model_types=%#v, want an empty string array", emptyTypes)
	}
}

func TestBuildModelListAPIKeyMapsBedrockExtensions(t *testing.T) {
	for _, providerName := range []string{"Bedrock", "bedrock", "BEDROCK"} {
		t.Run(providerName, func(t *testing.T) {
			got, err := buildModelListAPIKey(providerName, "token", "ap-northeast-1", map[string]interface{}{
				"auth_mode":              "bedrock_api_key",
				"endpoint_type":          "runtime",
				"discovery_endpoint_url": "https://bedrock.ap-northeast-1.amazonaws.com",
			})
			if err != nil {
				t.Fatal(err)
			}
			var key map[string]interface{}
			if err = json.Unmarshal([]byte(got), &key); err != nil {
				t.Fatal(err)
			}
			if key["bedrock_api_key"] != "token" || key["bedrock_region"] != "ap-northeast-1" || key["bedrock_endpoint_type"] != "runtime" {
				t.Fatalf("key=%+v", key)
			}
			if key["bedrock_discovery_endpoint_url"] != "https://bedrock.ap-northeast-1.amazonaws.com" {
				t.Fatalf("key=%+v", key)
			}
		})
	}
}

func TestBuildModelListAPIKeyLeavesOtherProvidersUnchanged(t *testing.T) {
	got, err := buildModelListAPIKey("OpenAI", "token", "", map[string]interface{}{"endpoint_type": "runtime"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "token" {
		t.Fatalf("api key=%q, want token", got)
	}
}

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
		&entity.TenantModel{ID: "model-1", ProviderID: "provider-1", InstanceID: "instance-1", ModelName: "gpt-test", ModelType: int(entity.ModelTypeChat), Status: "active"},
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

func TestProviderHandlerAlterModelRejectsMissingModelSelector(t *testing.T) {
	ctx, recorder := newProviderHandlerRequest(
		t,
		map[string]interface{}{"status": "active"},
		gin.Param{Key: "provider_id_or_name", Value: "OpenAI"},
		gin.Param{Key: "instance_id_or_name", Value: "default"},
	)

	NewProviderHandler(nil, service.NewModelProviderService()).AlterModel(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	body := decodeProviderHandlerResponse(t, recorder)
	if common.ErrorCode(body["code"].(float64)) != common.CodeBadRequest {
		t.Fatalf("code = %v, want %v", body["code"], common.CodeBadRequest)
	}
}

func TestProviderHandlerAlterModelRejectsInvalidStatus(t *testing.T) {
	ctx, recorder := newProviderHandlerRequest(
		t,
		map[string]interface{}{"status": "disabled"},
		gin.Param{Key: "provider_id_or_name", Value: "OpenAI"},
		gin.Param{Key: "instance_id_or_name", Value: "default"},
		gin.Param{Key: "model_name", Value: "gpt-test"},
	)

	NewProviderHandler(nil, service.NewModelProviderService()).AlterModel(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	body := decodeProviderHandlerResponse(t, recorder)
	if common.ErrorCode(body["code"].(float64)) != common.CodeBadRequest {
		t.Fatalf("code = %v, want %v", body["code"], common.CodeBadRequest)
	}
}

func TestProviderHandlerAlterModelUpdatesStatus(t *testing.T) {
	db := setupProviderHandlerTestDB(t)
	useProviderHandlerTestDB(t, db)
	seedProviderHandlerModel(t, db)

	ctx, recorder := newProviderHandlerRequest(
		t,
		map[string]interface{}{"status": "inactive"},
		gin.Param{Key: "provider_id_or_name", Value: "OpenAI"},
		gin.Param{Key: "instance_id_or_name", Value: "default"},
		gin.Param{Key: "model_name", Value: "gpt-test"},
	)

	NewProviderHandler(nil, service.NewModelProviderService()).AlterModel(ctx)

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
