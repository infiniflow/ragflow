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

package agentic_rag

import (
	"context"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

// setupPlatformDecision adds a shared SystemOne decision model and the
// platform settings that make platform models usable for tenant-1.
func setupPlatformDecision(t *testing.T, tenantStatus, instanceStatus string) {
	t.Helper()
	setupDecisionConfigDB(t)
	db := dao.DB
	if err := db.AutoMigrate(&entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	rows := []any{
		&entity.SystemSettings{Name: entity.PlatformProviderLockSetting, Source: "system", DataType: "string", Value: "ready"},
		&entity.Tenant{ID: "tenant-1", Status: &tenantStatus},
		&entity.TenantModelProvider{ID: "platform-prov", ProviderName: "SystemOne", TenantID: entity.PlatformTenantID, UserID: "admin"},
		&entity.TenantModelInstance{ID: "platform-inst", InstanceName: "shared", ProviderID: "platform-prov", APIKey: "platform-key", Status: instanceStatus, Extra: `{"base_url":"https://host.example/v1"}`},
		&entity.TenantModel{ID: "platform-decision", ModelName: "decision-preview", ProviderID: "platform-prov", InstanceID: "platform-inst", ModelType: int(entity.ModelTypeDecision), Status: "active"},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestPlatformDecisionModelIsUsedWhenSelected(t *testing.T) {
	setupPlatformDecision(t, "1", "active")
	id := "platform-decision"
	seedTenantDecisionDefault(t, dao.DB, "tenant-1", id, &id)
	// seedTenantDecisionDefault makes user "tenant-1" the joined owner.
	got := resolveDecisionBackend(withDecisionScope(context.Background(), "tenant-1", "", "tenant-1"))
	if got == nil || got.key != "platform-key" || !got.platform {
		t.Fatalf("selected platform decision model not used: %+v", got)
	}
}

func TestPlatformDecisionCacheRechecksIdentity(t *testing.T) {
	setupPlatformDecision(t, "1", "active")
	t.Setenv("RAGFLOW_DECISION_BACKEND", "")
	t.Setenv("RAGFLOW_DECISION_KEY", "")
	decisionConfigMu.Lock()
	decisionConfigCache = map[string]*decisionConfigEntry{}
	decisionConfigMu.Unlock()
	id := "platform-decision"
	seedTenantDecisionDefault(t, dao.DB, "tenant-1", id, &id)
	ctx := withDecisionScope(context.Background(), "tenant-1", "", "tenant-1")
	if got := resolveDecisionBackend(ctx); got == nil || !got.platform {
		t.Fatalf("authorized resolution: %+v", got)
	}
	mismatch := permission.WithIdentity(ctx, "tenant-1", "other-tenant")
	if got := resolveDecisionBackend(mismatch); got != nil && got.platform {
		t.Fatal("cached platform judge used with mismatched tenant")
	}
}

func TestPlatformDecisionCacheRejectsDisabledModelOrInstance(t *testing.T) {
	for _, table := range []string{"tenant_model", "tenant_model_instance"} {
		t.Run(table, func(t *testing.T) {
			setupPlatformDecision(t, "1", "active")
			decisionConfigMu.Lock()
			decisionConfigCache = map[string]*decisionConfigEntry{}
			decisionConfigMu.Unlock()
			id := "platform-decision"
			seedTenantDecisionDefault(t, dao.DB, "tenant-1", id, &id)
			ctx := withDecisionScope(context.Background(), "tenant-1", "", "tenant-1")
			if got := resolveDecisionBackend(ctx); got == nil || !got.platform {
				t.Fatalf("initial backend: %+v", got)
			}
			if err := dao.DB.Table(table).Where("1 = 1").Update("status", "inactive").Error; err != nil {
				t.Fatal(err)
			}
			if got := resolveDecisionBackend(ctx); got != nil && got.platform {
				t.Fatal("disabled platform backend was reused")
			}
		})
	}
}

func TestPlatformDecisionDefaultAppliesWithoutOwnChoice(t *testing.T) {
	setupPlatformDecision(t, "1", "active")
	seedTenantDecisionDefault(t, dao.DB, "tenant-1", "", nil)
	if err := (dao.PlatformModelSettings{}).SetDefault(context.Background(), dao.DB, "decision", "platform-decision"); err != nil {
		t.Fatal(err)
	}
	got := resolveDecisionBackend(withDecisionScope(context.Background(), "tenant-1", "", "tenant-1"))
	if got == nil || got.key != "platform-key" {
		t.Fatalf("platform default decision model not used: %+v", got)
	}
}

// A run without a user identity has no trusted task marker here, so the
// platform model is not used for it.
func TestPlatformDecisionModelNeedsIdentity(t *testing.T) {
	setupPlatformDecision(t, "1", "active")
	id := "platform-decision"
	seedTenantDecisionDefault(t, dao.DB, "tenant-1", id, &id)
	if got := buildDecisionBackendFromModel(context.Background(), decisionScope{tenantID: "tenant-1"}, id); got != nil {
		t.Fatalf("platform decision model used without an identity: %+v", got)
	}
}

func TestPlatformDecisionModelRequiresAccess(t *testing.T) {
	for _, tc := range []struct{ name, tenant, instance string }{
		{"inactive tenant", "0", "active"},
		{"inactive instance", "1", "inactive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupPlatformDecision(t, tc.tenant, tc.instance)
			id := "platform-decision"
			seedTenantDecisionDefault(t, dao.DB, "tenant-1", id, &id)
			got := buildDecisionBackendFromModel(context.Background(), decisionScope{tenantID: "tenant-1", userID: "tenant-1"}, id)
			if got != nil {
				t.Fatalf("unusable platform decision model resolved: %+v", got)
			}
		})
	}
}
