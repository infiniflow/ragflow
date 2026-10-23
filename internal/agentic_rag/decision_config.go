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
	"encoding/json"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// The judge used to be a process-wide singleton built from the environment.
// It is now resolved per run from what the operator configured in the UI:
//
//	chat.decision_id  ->  user_tenant.decision_id  ->  the env-configured backend
//
// The env backend stays as the last step so an existing deployment keeps its
// behaviour until it configures a model in the UI, and so a run that carries no
// tenant at all (tests, CLI) still has a judge.

// decisionScope is the identity a judge decision is made for. It travels on the
// context for the same reason the intent does: the filter is called from deep
// inside the tool chain, and threading two more parameters through every
// signature would touch code that has nothing to do with the judge.
type decisionScope struct {
	tenantID string
	chatID   string
}

type decisionScopeContextKey struct{}

// withDecisionScope attaches the tenant/chat the run belongs to.
func withDecisionScope(ctx context.Context, tenantID, chatID string) context.Context {
	return context.WithValue(ctx, decisionScopeContextKey{}, decisionScope{
		tenantID: strings.TrimSpace(tenantID),
		chatID:   strings.TrimSpace(chatID),
	})
}

// decisionScopeFrom returns the run's scope, or the zero value when the caller
// published none.
func decisionScopeFrom(ctx context.Context) decisionScope {
	scope, _ := ctx.Value(decisionScopeContextKey{}).(decisionScope)
	return scope
}

// decisionConfigTTL bounds how long a resolved backend is reused.
//
// Short on purpose: an operator who just picked a decision model in the UI
// should not wait for a restart, but the filter runs once per result set, so an
// uncached resolution would be three database reads per judgement. Thirty
// seconds bounds the staleness to something a human does not notice while
// keeping the read count negligible.
const decisionConfigTTL = 30 * time.Second

type decisionConfigEntry struct {
	backend   *decisionBackend
	expiresAt time.Time
}

var (
	decisionConfigMu    sync.Mutex
	decisionConfigCache = map[string]*decisionConfigEntry{}
)

// resolveDecisionBackend returns the judge for this run.
//
// A run that configures no decision model falls back to the env-configured
// backend, which is what every existing deployment uses. An unconfigured
// tenant is therefore not "a call that fails": it is exactly the run it was
// before the UI existed.
func resolveDecisionBackend(ctx context.Context) *decisionBackend {
	scope := decisionScopeFrom(ctx)

	modelID := decisionModelIDFor(ctx, scope)
	if modelID == "" {
		return decisionBackendFromEnv()
	}

	// The cache key carries the tenant: a model id resolved for one tenant
	// carries that tenant's credential and endpoint, and buildDecisionBackendFromModel's
	// ownership check only runs on a miss. Keyed by model id alone, a second
	// tenant that names the same id within the TTL would receive the first
	// tenant's backend without the check ever running.
	cacheKey := scope.tenantID + "\x00" + modelID

	decisionConfigMu.Lock()
	entry := decisionConfigCache[cacheKey]
	decisionConfigMu.Unlock()
	if entry != nil && time.Now().Before(entry.expiresAt) {
		return entry.backend
	}

	backend := buildDecisionBackendFromModel(ctx, scope, modelID)
	if backend == nil {
		// Resolution failed. Nothing is cached: a transient database error must
		// not be sticky for the whole TTL, and the caller falls back to the env
		// backend, which is a better judge than none.
		return decisionBackendFromEnv()
	}

	decisionConfigMu.Lock()
	decisionConfigCache[cacheKey] = &decisionConfigEntry{backend: backend, expiresAt: time.Now().Add(decisionConfigTTL)}
	decisionConfigMu.Unlock()
	return backend
}

// decisionModelIDFor resolves the configured model id: the chat's own override
// first, then the tenant default.
func decisionModelIDFor(ctx context.Context, scope decisionScope) string {
	if dao.DB == nil {
		// No database at all (CLI, unit tests): there is no configuration to
		// read, and dereferencing the nil handle would turn that into a panic
		// inside a tool call rather than into the env fallback.
		return ""
	}
	if scope.chatID != "" {
		var chat entity.Chat
		if err := dao.DB.Where("id = ?", scope.chatID).First(&chat).Error; err == nil {
			if id := resolvedDecisionID(chat.TenantDecisionID, chat.DecisionID); id != "" {
				return id
			}
		}
	}
	if scope.tenantID != "" {
		membership, err := dao.NewUserTenantDAO().GetOwnerByTenantID(ctx, dao.DB, scope.tenantID)
		if err == nil && membership != nil {
			if id := resolvedDecisionID(membership.TenantDecisionID, membership.DecisionID); id != "" {
				return id
			}
		}
	}
	return ""
}

// resolvedDecisionID prefers the resolved tenant_model row id over the value the
// operator typed.
//
// DecisionID holds whatever the caller sent — typically the composite
// "name@provider@type" string — while TenantDecisionID holds the row id that
// string resolved to at save time, which is what the model lookup keys on. Older
// rows stored the row id in DecisionID, so that stays the fallback rather than
// being ignored.
func resolvedDecisionID(rowID, requested *string) string {
	if rowID != nil {
		if id := strings.TrimSpace(*rowID); id != "" {
			return id
		}
	}
	if requested != nil {
		return strings.TrimSpace(*requested)
	}
	return ""
}

// buildDecisionBackendFromModel turns a configured model row into a backend.
//
// The credential and the endpoint live on the model's INSTANCE, not on the
// model row: several tenants can share one instance, and re-keying the instance
// must not mean re-picking the model. Returns nil when the row is missing, of
// the wrong type, or belongs to another tenant — all of which mean "this
// configuration is not usable", not "the judge is down".
func buildDecisionBackendFromModel(ctx context.Context, scope decisionScope, modelID string) *decisionBackend {
	if dao.DB == nil {
		return nil
	}

	var model entity.TenantModel
	if err := dao.DB.Where("id = ? AND status = ?", modelID, "active").First(&model).Error; err != nil {
		common.WarnCtx(ctx, "agentic_rag: configured decision model not found, falling back to the env backend",
			zap.String("model_id", modelID), zap.Error(err))
		return nil
	}
	if entity.ModelType(model.ModelType).Has(entity.ModelTypeDecision) == false {
		common.WarnCtx(ctx, "agentic_rag: configured model is not a decision model, falling back to the env backend",
			zap.String("model_id", modelID), zap.Int("model_type", model.ModelType))
		return nil
	}

	var instance entity.TenantModelInstance
	if err := dao.DB.Where("id = ?", model.InstanceID).First(&instance).Error; err != nil {
		common.WarnCtx(ctx, "agentic_rag: decision model instance not found, falling back to the env backend",
			zap.String("instance_id", model.InstanceID), zap.Error(err))
		return nil
	}

	// The model row carries no tenant column, so ownership is checked through
	// its provider. Skipping the check when the run has no tenant is deliberate:
	// a CLI or test run has no tenant to match and must not lose its judge.
	if scope.tenantID != "" {
		var provider entity.TenantModelProvider
		if err := dao.DB.Where("id = ?", instance.ProviderID).First(&provider).Error; err != nil || provider.TenantID != scope.tenantID {
			common.WarnCtx(ctx, "agentic_rag: decision model belongs to another tenant, falling back to the env backend",
				zap.String("model_id", modelID), zap.String("tenant_id", scope.tenantID))
			return nil
		}
	}

	var extra map[string]string
	if instance.Extra != "" {
		if err := json.Unmarshal([]byte(instance.Extra), &extra); err != nil {
			common.WarnCtx(ctx, "agentic_rag: decision model instance has unreadable extra, falling back to the env backend",
				zap.String("instance_id", instance.ID), zap.Error(err))
			return nil
		}
	}
	baseURL := strings.TrimSpace(extra["base_url"])
	if baseURL == "" {
		common.WarnCtx(ctx, "agentic_rag: decision model instance has no base_url, falling back to the env backend",
			zap.String("instance_id", instance.ID))
		return nil
	}
	key := strings.TrimSpace(instance.APIKey)
	if key == "" {
		// Without a key every judgement would 401, so this instance cannot judge
		// at all. Falling back beats spending a request per passage to learn it.
		common.WarnCtx(ctx, "agentic_rag: decision model instance has no api_key, falling back to the env backend",
			zap.String("instance_id", instance.ID))
		return nil
	}

	backend := newDecisionBackend(joinDecisionURL(baseURL, defaultDecisionURL), key, model.ModelName)
	common.InfoCtx(ctx, "agentic_rag: decision model resolved from configuration",
		zap.String("model_id", modelID),
		zap.String("model_name", model.ModelName),
		zap.String("instance", instance.InstanceName),
		zap.String("provider_id", instance.ProviderID),
		zap.String("chat_id", scope.chatID),
		zap.String("tenant_id", scope.tenantID))
	return backend
}

// joinDecisionURL appends the System One path to the instance's base URL,
// tolerating a base URL that already carries it.
func joinDecisionURL(baseURL, fallbackURL string) string {
	base := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return fallbackURL
	}
	path := "systemone"
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		if strings.EqualFold(base[idx+1:], path) {
			return base
		}
	}
	return base + "/" + path
}
