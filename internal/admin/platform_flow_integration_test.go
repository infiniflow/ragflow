//go:build integration

// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/permission"
	"ragflow/internal/service"
)

// TestPlatformModelAdminToTenantFlow configures a platform provider through the
// admin handlers and uses it as a tenant member on a real database: the model
// is listed, inherited as the default, called with the platform credential,
// and an upstream error that echoes the credential does not reach the tenant.
func TestPlatformModelAdminToTenantFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(common.EnvAllowAnyHost, "true")
	const secret = "sk-platform-flow-secret"
	var calls atomic.Int32
	var failNext atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if failNext.Load() {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":{"message":"upstream rejected key ` + secret + `"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	for _, tc := range []struct{ name, env string }{{"mysql", "RAGFLOW_TEST_MYSQL_DSN"}, {"postgres", "RAGFLOW_TEST_POSTGRES_DSN"}} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := os.Getenv(tc.env)
			if dsn == "" {
				t.Skipf("%s requires a disposable database", tc.env)
			}
			var dialect gorm.Dialector = postgres.Open(dsn)
			if tc.name == "mysql" {
				dialect = mysql.Open(dsn)
			}
			db, err := gorm.Open(dialect, &gorm.Config{TranslateError: true})
			if err != nil {
				t.Fatal(err)
			}
			pool, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = pool.Close() })
			tables := []any{&entity.SystemSettings{}, &entity.Tenant{}, &entity.UserTenant{}, &entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{}}
			for _, table := range tables {
				if db.Migrator().HasTable(table) {
					t.Fatalf("refusing populated database: %T exists", table)
				}
			}
			if db.Migrator().HasTable(&platformFlowUser{}) {
				t.Fatal("refusing populated database: user exists")
			}
			t.Cleanup(func() {
				if err := db.Migrator().DropTable(tables...); err != nil {
					t.Error(err)
				}
			})
			if err := db.AutoMigrate(tables...); err != nil {
				t.Fatal(err)
			}
			// Platform initialization checks that no user owns the reserved ID.
			if err := db.Migrator().CreateTable(&platformFlowUser{}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := db.Migrator().DropTable(&platformFlowUser{}); err != nil {
					t.Error(err)
				}
			})
			original := dao.DB
			dao.DB = db
			t.Cleanup(func() { dao.DB = original })
			if err := (dao.PlatformModelSettings{}).Initialize(t.Context(), db); err != nil {
				t.Fatal(err)
			}
			active, joined := "1", entity.TenantMembershipStatusJoined
			for _, row := range []any{
				&entity.Tenant{ID: "tenant-a", Status: &active},
				&entity.UserTenant{ID: "owner-a", UserID: "tenant-a", TenantID: "tenant-a", Role: string(permission.RoleOwner), InvitedBy: "tenant-a", Status: &joined},
				&entity.UserTenant{ID: "member-a", UserID: "member", TenantID: "tenant-a", Role: string(permission.RoleNormal), InvitedBy: "tenant-a", Status: &joined},
			} {
				if err := db.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}

			h := &Handler{service: NewService()}
			call := func(handler gin.HandlerFunc, method, path, body string, params gin.Params) json.RawMessage {
				t.Helper()
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Set("user_id", "admin-a")
				ctx.Params = params
				ctx.Request = httptest.NewRequest(method, path, strings.NewReader(body))
				ctx.Request.Header.Set("Content-Type", "application/json")
				handler(ctx)
				var resp struct {
					Code common.ErrorCode `json:"code"`
					Data json.RawMessage  `json:"data"`
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil || resp.Code != common.CodeSuccess {
					t.Fatalf("%s %s: %s", method, path, recorder.Body.String())
				}
				if strings.Contains(recorder.Body.String(), secret) {
					t.Fatalf("%s %s returned the credential", method, path)
				}
				return resp.Data
			}
			var provider struct{ ID string }
			_ = json.Unmarshal(call(h.CreatePlatformProvider, http.MethodPut, "/providers", `{"provider_name":"OpenAI"}`, nil), &provider)
			providerParam := gin.Params{{Key: "provider_id", Value: provider.ID}}
			var instance struct{ ID string }
			body := `{"instance_name":"shared","api_key":"` + secret + `","base_url":"` + upstream.URL + `/v1","model_info":[{"model_name":"gpt-flow","model_type":["chat"],"max_tokens":4096}]}`
			_ = json.Unmarshal(call(h.CreatePlatformInstance, http.MethodPost, "/instances", body, providerParam), &instance)
			instanceParams := append(providerParam, gin.Param{Key: "instance_id", Value: instance.ID})
			call(h.ShowPlatformInstance, http.MethodGet, "/instance", "", instanceParams)
			var models []struct {
				ModelID string `json:"model_id"`
			}
			_ = json.Unmarshal(call(h.ListPlatformModels, http.MethodGet, "/models", "", instanceParams), &models)
			if len(models) != 1 {
				t.Fatalf("models = %+v", models)
			}
			call(h.SetPlatformDefault, http.MethodPatch, "/models/default", `{"model_type":"chat","model_id":"`+models[0].ModelID+`"}`, nil)

			tenantCtx := permission.WithIdentity(t.Context(), "member", "tenant-a")
			listed, _, err := service.NewModelProviderService().ListModelsForUserInTenant(tenantCtx, "", "chat")
			if err != nil || len(listed) != 1 || listed[0]["source"] != "platform" || listed[0]["model_id"] != models[0].ModelID {
				t.Fatalf("tenant list = %+v, %v", listed, err)
			}
			if encoded, _ := json.Marshal(listed); strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), upstream.URL) {
				t.Fatalf("tenant list exposes platform configuration: %s", encoded)
			}
			defaults, err := service.NewUserTenantService().ListDefaultModels(tenantCtx)
			if err != nil {
				t.Fatal(err)
			}
			inherited := false
			for _, item := range defaults {
				if item.ModelType == "chat" && item.Source == "platform" && item.Enable && item.ModelID == models[0].ModelID {
					inherited = true
				}
			}
			if !inherited {
				t.Fatalf("defaults = %+v", defaults)
			}

			chat, err := service.NewModelFactory().NewDefaultChatModel(tenantCtx)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := chat.ChatWithMessages(tenantCtx, []modelModule.Message{{Role: "user", Content: "ping"}}, nil, nil)
			if err != nil || resp == nil || resp.Answer == nil || *resp.Answer != "pong" || calls.Load() == 0 {
				t.Fatalf("platform chat: %+v, %v", resp, err)
			}
			failNext.Store(true)
			_, err = chat.ChatWithMessages(tenantCtx, []modelModule.Message{{Role: "user", Content: "ping"}}, nil, nil)
			failNext.Store(false)
			if err == nil || strings.Contains(err.Error(), secret) {
				t.Fatalf("platform error reached the tenant: %v", err)
			}

			outsider := permission.WithIdentity(t.Context(), "outsider", "tenant-a")
			if _, err := service.NewModelFactory().NewChatModel(outsider, models[0].ModelID); err == nil {
				t.Fatal("non-member used a platform model")
			}
			call(h.UpdatePlatformModel, http.MethodPatch, "/model", `{"status":"inactive"}`, append(instanceParams, gin.Param{Key: "model_id", Value: models[0].ModelID}))
			if _, err := service.NewModelFactory().NewDefaultChatModel(tenantCtx); err == nil {
				t.Fatal("inactive platform default was used")
			}
			defaults, err = service.NewUserTenantService().ListDefaultModels(tenantCtx)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range defaults {
				if item.ModelType == "chat" && item.Enable {
					t.Fatalf("inactive default reported usable: %+v", item)
				}
			}
		})
	}
}

// platformFlowUser is the subset of the user table read by platform
// initialization; the full entity uses MySQL-only column types.
type platformFlowUser struct {
	ID string `gorm:"column:id;primaryKey;size:32"`
}

func (platformFlowUser) TableName() string { return "user" }
