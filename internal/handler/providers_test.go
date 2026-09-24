package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func TestDropProviderInstanceRequestRequiresNonEmptyInstances(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		valid   bool
	}{
		{name: "missing", payload: `{}`, valid: false},
		{name: "empty", payload: `{"instances":[]}`, valid: false},
		{name: "empty element", payload: `{"instances":[""]}`, valid: false},
		{name: "instance", payload: `{"instances":["instance-a"]}`, valid: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req DropProviderInstanceRequest
			if err := json.Unmarshal([]byte(tt.payload), &req); err != nil {
				t.Fatal(err)
			}
			err := binding.Validator.ValidateStruct(&req)
			if (err == nil) != tt.valid {
				t.Fatalf("validation error = %v, valid = %v", err, tt.valid)
			}
		})
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

func TestFilterUnsupportedProviders(t *testing.T) {
	providers := []map[string]interface{}{
		{"name": "OpenAI"},
		{"name": "MinerU.Net"},
		{"name": "MinerU"},
	}

	got := filterUnsupportedProviders(providers)

	names := make([]string, 0, len(got))
	for _, provider := range got {
		name, ok := provider["name"].(string)
		if !ok {
			t.Fatalf("provider without name: %v", provider)
		}
		names = append(names, name)
	}
	want := []string{"OpenAI", "MinerU"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
}

// indexProviderModels keys a merged list by its normalized name.
func indexProviderModels(t *testing.T, list []map[string]interface{}) map[string]map[string]interface{} {
	t.Helper()
	indexed := make(map[string]map[string]interface{}, len(list))
	for _, m := range list {
		name, ok := m["name"].(string)
		if !ok {
			t.Fatalf("model without name: %v", m)
		}
		key := providerModelKey(name)
		if _, dup := indexed[key]; dup {
			t.Fatalf("duplicate model %q in merged list", name)
		}
		indexed[key] = m
	}
	return indexed
}

func TestMergeProviderModelsMatchesNamesCaseInsensitively(t *testing.T) {
	static := []map[string]interface{}{
		{"name": "gpt-4o", "model_types": []string{"chat"}, "max_tokens": 4096},
		{"name": "text-embedding", "model_types": []string{"embedding"}},
	}
	remote := []map[string]interface{}{
		{"name": "GPT-4o", "model_types": []string{"chat"}, "max_tokens": 128000},
		{"name": "gpt-4o-mini ", "model_types": []string{"chat"}, "max_tokens": 16384},
	}

	got := indexProviderModels(t, mergeProviderModels(static, remote))
	if len(got) != 3 {
		t.Fatalf("merged %d models, want 3: %v", len(got), got)
	}

	merged := got["gpt-4o"]
	if name := merged["name"]; name != "gpt-4o" {
		t.Errorf("name = %v, want the catalog spelling gpt-4o", name)
	}
	if maxTokens := merged["max_tokens"]; maxTokens != 4096 {
		t.Errorf("max_tokens = %v, want the catalog value 4096", maxTokens)
	}

	trimmed, ok := got["gpt-4o-mini"]
	if !ok {
		t.Fatalf("remote-only model not kept: %v", got)
	}
	if name := trimmed["name"]; name != "gpt-4o-mini" {
		t.Errorf("name = %v, want trimmed gpt-4o-mini", name)
	}
}

func TestMergeProviderModelsInheritsCatalogTypesOnConflict(t *testing.T) {
	static := []map[string]interface{}{
		{"name": "rerank-1", "model_types": []string{"rerank"}, "max_tokens": 1024},
	}
	remote := []map[string]interface{}{{"name": "Rerank-1"}}

	got := indexProviderModels(t, mergeProviderModels(static, remote))
	merged, ok := got["rerank-1"]
	if !ok {
		t.Fatalf("merged = %v, want a single rerank-1 entry", got)
	}
	if types := providerModelMapTypes(merged); !reflect.DeepEqual(types, []string{"rerank"}) {
		t.Errorf("model_types = %v, want [rerank]", types)
	}
	if maxTokens := merged["max_tokens"]; maxTokens != 1024 {
		t.Errorf("max_tokens = %v, want 1024", maxTokens)
	}
}

func TestValidateInstanceName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{name: "my_instance", valid: true},
		{name: "Instance123", valid: true},
		{name: "_123", valid: true},
		{name: "my-instance", valid: true},
		{name: "-my-instance-1", valid: true},
		{name: "", valid: false},
		{name: "my instance", valid: false},
		{name: "实例", valid: false},
		{name: "instância", valid: false},
		{name: "instance!", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateInstanceName(tt.name); (err == nil) != tt.valid {
				t.Fatalf("validateInstanceName(%q) error = %v, valid = %v", tt.name, err, tt.valid)
			}
		})
	}
}

func TestProviderHandlerCreateProviderInstanceRejectsInvalidInstanceName(t *testing.T) {
	ctx, recorder := newProviderHandlerRequest(
		t,
		map[string]interface{}{"instance_name": "my instance!"},
		gin.Param{Key: "provider_id_or_name", Value: "OpenAI"},
	)

	NewProviderHandler(nil, service.NewModelProviderService()).CreateProviderInstance(ctx)

	body := decodeProviderHandlerResponse(t, recorder)
	if common.ErrorCode(body["code"].(float64)) != common.CodeBadRequest {
		t.Fatalf("code = %v, want %v", body["code"], common.CodeBadRequest)
	}
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
