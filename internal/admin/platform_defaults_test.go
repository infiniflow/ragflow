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
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"ragflow/internal/common"
	"ragflow/internal/dao"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
)

func TestSetPlatformDefaultRequiresModelIDField(t *testing.T) {
	db := testutil.SetupTestDB(t, &entity.SystemSettings{})
	previous := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = previous })
	if err := (dao.PlatformModelSettings{}).SetDefault(t.Context(), db, "chat", "existing"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body     string
		wantCode int
		want     string
	}{
		{`{"model_type":"chat"}`, int(common.CodeArgumentError), "existing"},
		{`{"model_type":"chat","model_id":null}`, int(common.CodeSuccess), ""},
	} {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewBufferString(tc.body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		(&Handler{}).SetPlatformDefault(ctx)
		var response struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil || response.Code != tc.wantCode {
			t.Fatalf("body=%s response=%s err=%v", tc.body, rec.Body, err)
		}
		got, err := (dao.PlatformModelSettings{}).Default(t.Context(), db, "chat")
		if err != nil || got != tc.want {
			t.Fatalf("default=%q err=%v want=%q", got, err, tc.want)
		}
	}
}

func TestPlatformDefaultsValidateScopeTypeStatusAndClear(t *testing.T) {
	db := testutil.SetupTestDB(t, &entity.SystemSettings{}, &entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{})
	rows := []any{
		&entity.TenantModelProvider{ID: "platform", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "platform"},
		&entity.TenantModelProvider{ID: "tenant", TenantID: "tenant", UserID: "owner", ProviderName: "tenant"},
		&entity.TenantModelInstance{ID: "active", ProviderID: "platform", InstanceName: "active", Status: "active"},
		&entity.TenantModelInstance{ID: "inactive", ProviderID: "platform", InstanceName: "inactive", Status: "inactive"},
		&entity.TenantModelInstance{ID: "tenant-instance", ProviderID: "tenant", InstanceName: "active", Status: "active"},
		&entity.TenantModel{ID: "valid", ProviderID: "platform", InstanceID: "active", ModelName: "chat", ModelType: int(entity.ModelTypeChat), Status: "active"},
		&entity.TenantModel{ID: "asr", ProviderID: "platform", InstanceID: "active", ModelName: "asr", ModelType: int(entity.ModelTypeSpeech2Text), Status: "active"},
		&entity.TenantModel{ID: "wrong-scope", ProviderID: "tenant", InstanceID: "tenant-instance", ModelName: "chat", ModelType: int(entity.ModelTypeChat), Status: "active"},
		&entity.TenantModel{ID: "inactive-model", ProviderID: "platform", InstanceID: "active", ModelName: "inactive", ModelType: int(entity.ModelTypeChat), Status: "inactive"},
		&entity.TenantModel{ID: "inactive-instance", ProviderID: "platform", InstanceID: "inactive", ModelName: "chat", ModelType: int(entity.ModelTypeChat), Status: "active"},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ kind, id string }{{"chat", "wrong-scope"}, {"chat", "inactive-model"}, {"chat", "inactive-instance"}, {"embedding", "valid"}, {"unknown", "valid"}, {"chat", "missing"}} {
		if err := setPlatformDefault(t.Context(), db, tc.kind, tc.id); !errors.Is(err, errInvalidPlatformDefault) {
			t.Fatalf("set %s/%s: %v", tc.kind, tc.id, err)
		}
	}
	if err := setPlatformDefault(t.Context(), db, "speech2text", "asr"); err != nil {
		t.Fatal(err)
	}
	items, err := listPlatformDefaults(t.Context(), db)
	if err != nil || len(items) != 1 || items[0].ModelType != "asr" {
		t.Fatalf("alias: %+v, %v", items, err)
	}
	if err := setPlatformDefault(t.Context(), db, "speech2text", ""); err != nil {
		t.Fatal(err)
	}
	if err := setPlatformDefault(t.Context(), db, "chat", "valid"); err != nil {
		t.Fatal(err)
	}
	items, err = listPlatformDefaults(t.Context(), db)
	if err != nil || len(items) != 1 || items[0].ModelID != "valid" || items[0].ModelType != "chat" {
		t.Fatalf("defaults: %+v %v", items, err)
	}
	if err := setPlatformDefault(t.Context(), db, "chat", ""); err != nil {
		t.Fatal(err)
	}
	items, err = listPlatformDefaults(t.Context(), db)
	if err != nil || len(items) != 0 {
		t.Fatalf("cleared defaults: %+v %v", items, err)
	}
}
