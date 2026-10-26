//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package agentic_rag

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestDecisionScopeRoundTrip(t *testing.T) {
	if got := decisionScopeFrom(context.Background()); got.tenantID != "" || got.chatID != "" {
		t.Fatalf("empty context carried %+v", got)
	}

	ctx := withDecisionScope(context.Background(), "tenant-1", "chat-1")
	got := decisionScopeFrom(ctx)
	if got.tenantID != "tenant-1" || got.chatID != "chat-1" {
		t.Fatalf("scope = %+v, want tenant-1/chat-1", got)
	}

	// A child context must not lose the scope: the filter runs inside tool
	// calls that derive their own contexts.
	if child := decisionScopeFrom(context.WithValue(ctx, "k", "v")); child.chatID != "chat-1" {
		t.Fatalf("child scope = %+v", child)
	}
}

// The whole point of the resolver is the fallback order. With no database (and
// so no configuration), a scoped run must get exactly the backend it got before
// the UI existed — not nil, which would silently turn the gate off.
func TestResolveDecisionBackendFallsBackToEnvWhenUnconfigured(t *testing.T) {
	srv := decisionStub(t, []float64{0.9, 0.9, 0.9, 0.9}, 200, nil)
	defer srv.Close()
	env := backendFor(t, srv.URL, 0.15)

	ctx := withDecisionScope(context.Background(), "tenant-1", "chat-1")
	got := resolveDecisionBackend(ctx)
	if got == nil {
		t.Fatal("resolveDecisionBackend returned nil; the env backend must survive")
	}
	if got != env {
		t.Fatalf("resolveDecisionBackend = %p, want the env backend %p", got, env)
	}
}

// A run with no scope at all must not be able to pick up somebody else's judge.
func TestResolveDecisionBackendWithoutScope(t *testing.T) {
	srv := decisionStub(t, []float64{0.9, 0.9, 0.9, 0.9}, 200, nil)
	defer srv.Close()
	env := backendFor(t, srv.URL, 0.15)

	if got := resolveDecisionBackend(context.Background()); got != env {
		t.Fatalf("resolveDecisionBackend = %p, want the env backend %p", got, env)
	}
}

func TestJoinDecisionURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://host/compatible-mode/v1", "https://host/compatible-mode/v1/systemone"},
		{"https://host/compatible-mode/v1/", "https://host/compatible-mode/v1/systemone"},
		{"https://host/compatible-mode/v1/systemone", "https://host/compatible-mode/v1/systemone"},
		{"", defaultDecisionURL},
	}
	for _, c := range cases {
		if got := joinDecisionURL(c.in, defaultDecisionURL); got != c.want {
			t.Errorf("joinDecisionURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The instance's own settings must not be needed for the backend to exist: a
// missing database is a state the filter has to survive, not crash on.
func TestDecisionModelIDForWithoutDatabase(t *testing.T) {
	if got := decisionModelIDFor(context.Background(), decisionScope{tenantID: "t", chatID: "c"}); got != "" {
		t.Fatalf("decisionModelIDFor = %q, want empty without a database", got)
	}
}

// The rest of the resolution order needs real rows, so these run against an
// in-memory database: the point is the SQL path — the chat override, the tenant
// default, the model-type guard and the tenant ownership check — which is where
// a mistake would silently hand one tenant another tenant's key.
func setupDecisionConfigDB(t *testing.T) {
	t.Helper()
	// A unique name per test: a bare ":memory:" is shared across gorm.Open calls
	// in this driver, so rows from one test would collide with the next.
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.Chat{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })
}

func seedTenantDecisionDefault(t *testing.T, db *gorm.DB, tenantID string, decisionID string, tenantDecisionID *string) {
	t.Helper()
	status := entity.TenantMembershipStatusJoined
	if err := db.Create(&entity.UserTenant{
		ID: "owner-" + tenantID, UserID: tenantID, TenantID: tenantID, Role: "owner", Status: &status,
		DecisionID: &decisionID, TenantDecisionID: tenantDecisionID,
	}).Error; err != nil {
		t.Fatalf("seed owner model settings: %v", err)
	}
}

func seedDecisionModel(t *testing.T, db *gorm.DB, tenantID, modelName string, modelType int) (providerID, instanceID, modelID string) {
	t.Helper()
	tag := strings.ReplaceAll(t.Name(), "/", "-")
	providerID = fmt.Sprintf("prov-%s-%s", tag, modelName)
	instanceID = fmt.Sprintf("inst-%s-%s", tag, modelName)
	modelID = fmt.Sprintf("model-%s-%s", tag, modelName)
	rows := []any{
		&entity.TenantModelProvider{ID: providerID, ProviderName: "SystemOne-" + modelName, TenantID: tenantID},
		&entity.TenantModelInstance{ID: instanceID, InstanceName: "qwen", ProviderID: providerID, APIKey: "key-" + modelName, Extra: `{"base_url":"https://host.example/v1"}`},
		&entity.TenantModel{ID: modelID, ModelName: modelName, ProviderID: providerID, InstanceID: instanceID, ModelType: modelType, Status: "active"},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return providerID, instanceID, modelID
}

// Tenant default, no chat override: the common case.
func TestResolveDecisionBackendUsesTenantDefault(t *testing.T) {
	setupDecisionConfigDB(t)
	db := dao.DB
	_, _, modelID := seedDecisionModel(t, db, "tenant-1", "decision-model-preview", int(entity.ModelTypeDecision))
	seedTenantDecisionDefault(t, db, "tenant-1", modelID, nil)
	if err := db.Create(&entity.Chat{ID: "chat-1", TenantID: "tenant-1", LLMSetting: entity.JSONMap{}, PromptConfig: entity.JSONMap{}, KBIDs: entity.JSONSlice{}}).Error; err != nil {
		t.Fatalf("seed chat: %v", err)
	}

	got := resolveDecisionBackend(withDecisionScope(context.Background(), "tenant-1", "chat-1"))
	if got == nil {
		t.Fatal("no backend resolved for a configured tenant")
	}
	if got.key != "key-decision-model-preview" {
		t.Errorf("key = %q, want the instance's key", got.key)
	}
	if got.model != "decision-model-preview" {
		t.Errorf("model = %q", got.model)
	}
	if want := "https://host.example/v1/systemone"; got.url != want {
		t.Errorf("url = %q, want %q", got.url, want)
	}
}

// The chat's own choice must win over the tenant default.
func TestResolveDecisionBackendPrefersChatOverride(t *testing.T) {
	setupDecisionConfigDB(t)
	db := dao.DB
	_, _, tenantModel := seedDecisionModel(t, db, "tenant-1", "tenant-model", int(entity.ModelTypeDecision))
	_, _, chatModel := seedDecisionModel(t, db, "tenant-1", "chat-model", int(entity.ModelTypeDecision))
	seedTenantDecisionDefault(t, db, "tenant-1", tenantModel, nil)
	if err := db.Create(&entity.Chat{ID: "chat-1", TenantID: "tenant-1", LLMSetting: entity.JSONMap{}, PromptConfig: entity.JSONMap{}, KBIDs: entity.JSONSlice{}, DecisionID: &chatModel}).Error; err != nil {
		t.Fatalf("seed chat: %v", err)
	}

	got := resolveDecisionBackend(withDecisionScope(context.Background(), "tenant-1", "chat-1"))
	if got == nil || got.model != "chat-model" {
		t.Fatalf("resolved %+v, want the chat's own model", got)
	}
}

// A model that is not a decision model must not be used as one: the picker is
// filtered by type, but the resolver runs against whatever the row says.
func TestResolveDecisionBackendRejectsWrongModelType(t *testing.T) {
	setupDecisionConfigDB(t)
	db := dao.DB
	_, _, modelID := seedDecisionModel(t, db, "tenant-1", "chat-model", int(entity.ModelTypeChat))
	seedTenantDecisionDefault(t, db, "tenant-1", modelID, nil)

	srv := decisionStub(t, []float64{0.9, 0.9, 0.9, 0.9}, 200, nil)
	defer srv.Close()
	env := backendFor(t, srv.URL, 0.15)

	if got := resolveDecisionBackend(withDecisionScope(context.Background(), "tenant-1", "")); got != env {
		t.Fatalf("resolved %p, want the env backend (model is not a decision model)", got)
	}
}

// Another tenant's model must not be reachable, even by id.
func TestResolveDecisionBackendRejectsForeignTenant(t *testing.T) {
	setupDecisionConfigDB(t)
	db := dao.DB
	_, _, modelID := seedDecisionModel(t, db, "tenant-2", "decision-model-preview", int(entity.ModelTypeDecision))
	seedTenantDecisionDefault(t, db, "tenant-1", modelID, nil)

	srv := decisionStub(t, []float64{0.9, 0.9, 0.9, 0.9}, 200, nil)
	defer srv.Close()
	env := backendFor(t, srv.URL, 0.15)

	if got := resolveDecisionBackend(withDecisionScope(context.Background(), "tenant-1", "")); got != env {
		t.Fatalf("resolved %p, want the env backend (model belongs to another tenant)", got)
	}
}

// The resolved row id wins over the value the operator typed. DecisionID holds
// the composite "name@provider@type" string, which the model lookup cannot key
// on; TenantDecisionID holds what that string resolved to at save time.
func TestDecisionModelIDPrefersResolvedRowID(t *testing.T) {
	setupDecisionConfigDB(t)
	db := dao.DB
	_, _, modelID := seedDecisionModel(t, db, "tenant-1", "resolved", int(entity.ModelTypeDecision))
	composite := "resolved@systemone@chat"
	seedTenantDecisionDefault(t, db, "tenant-1", composite, &modelID)

	got := decisionModelIDFor(context.Background(), decisionScope{tenantID: "tenant-1"})
	if got != modelID {
		t.Errorf("model id = %q, want the resolved row id %q", got, modelID)
	}
}

// A legacy row that stored the row id in DecisionID still resolves.
func TestDecisionModelIDFallsBackToRequestedValue(t *testing.T) {
	setupDecisionConfigDB(t)
	db := dao.DB
	_, _, modelID := seedDecisionModel(t, db, "tenant-1", "legacy", int(entity.ModelTypeDecision))
	seedTenantDecisionDefault(t, db, "tenant-1", modelID, nil)

	if got := decisionModelIDFor(context.Background(), decisionScope{tenantID: "tenant-1"}); got != modelID {
		t.Errorf("model id = %q, want %q", got, modelID)
	}
}

// The resolution cache must not hand one tenant's backend to another. Keyed by
// model id alone, the second tenant would skip the ownership check entirely and
// receive the first tenant's API key and endpoint.
func TestDecisionBackendCacheIsScopedToTenant(t *testing.T) {
	setupDecisionConfigDB(t)
	db := dao.DB
	t.Setenv("RAGFLOW_DECISION_BACKEND", "")
	t.Setenv("RAGFLOW_DECISION_KEY", "")

	_, _, modelID := seedDecisionModel(t, db, "tenant-a", "shared", int(entity.ModelTypeDecision))
	for _, tenantID := range []string{"tenant-a", "tenant-b"} {
		id := modelID
		seedTenantDecisionDefault(t, db, tenantID, "", &id)
	}

	if got := resolveDecisionBackend(withDecisionScope(context.Background(), "tenant-a", "")); got == nil {
		t.Fatal("tenant A owns the model and must resolve it")
	}
	if got := resolveDecisionBackend(withDecisionScope(context.Background(), "tenant-b", "")); got != nil {
		t.Fatalf("tenant B received a cached backend it does not own (key %q)", got.key)
	}
}
