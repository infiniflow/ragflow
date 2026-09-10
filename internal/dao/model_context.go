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

package dao

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// ResolveModelContentLength returns the chat model's effective context window
// (context_length) in tokens for modelRef — a tenant_model UUID or a
// composite "model@provider" / "model@instance@provider" reference — or 0
// when it cannot be resolved. tenantID scopes the composite-reference lookup
// to the tenant's own provider/instance/model rows. driver and modelName are
// an optional provider catalog fallback used when modelRef is not a
// resolvable tenant id (for example when no database is available); pass
// empty strings to skip it.
//
// Resolution order (mirrors Python's model_extra.get("max_tokens") semantics):
//  1. Tenant configuration first: when the tenant_model row is active and
//     carries a positive "max_tokens" override in its extra JSON, that custom
//     context window wins and no catalog data is read.
//  2. Only with no override, fall back to the provider catalog's
//     context_length (via the tenant row, the composite reference parts, or
//     the resolved driver + model name).
//
// This is the shared implementation behind the agent LLM component, the
// ingestion Extractor, and service.ModelProviderService — those packages
// cannot import each other (import cycles), so the lookup lives here.
func ResolveModelContentLength(ctx context.Context, db *gorm.DB, tenantID, modelRef, driver, modelName string) int {
	if db == nil {
		db = DB
	}
	pureName, instanceName, providerName, composite := splitCompositeModelRef(modelRef)

	// 1. Tenant-configured override: resolve the tenant_model row before any
	//    catalog read and return the custom context window when present.
	obj := lookupTenantModel(ctx, db, tenantID, modelRef, pureName, instanceName, providerName, composite)
	if obj != nil && obj.Status == "active" {
		if v := modelExtraMaxTokens(obj.Extra); v > 0 {
			return v
		}
	}

	// 2. Catalog fallbacks (only when there is no custom override).
	// 2a. Active tenant row → its provider row → catalog context_length.
	if obj != nil && obj.Status == "active" {
		if provider, err := NewTenantModelProviderDAO().GetByID(ctx, db, obj.ProviderID); err == nil && provider != nil {
			if mdl, err := GetModelProviderManager().GetModelByName(provider.ProviderName, obj.ModelName); err == nil && mdl.ContextLength != nil {
				return *mdl.ContextLength
			}
		}
	}
	// 2b. Composite reference with no tenant row → catalog by reference parts.
	if composite {
		if mdl, err := GetModelProviderManager().GetModelByName(providerName, pureName); err == nil && mdl.ContextLength != nil {
			return *mdl.ContextLength
		}
	}
	// 2c. Resolved driver + bare model name: fallback when modelRef is a
	//     tenant id that could not be resolved without a database.
	if driver != "" && modelName != "" {
		if mdl, err := GetModelProviderManager().GetModelByName(driver, modelName); err == nil && mdl.ContextLength != nil {
			return *mdl.ContextLength
		}
	}
	return 0
}

// lookupTenantModel resolves the tenant_model row for modelRef — by UUID, or
// for a composite reference through the tenant's provider/instance rows (chat
// model type). Returns nil when the row cannot be resolved.
func lookupTenantModel(ctx context.Context, db *gorm.DB, tenantID, modelRef, pureName, instanceName, providerName string, composite bool) *entity.TenantModel {
	if db == nil {
		return nil
	}
	if composite {
		if tenantID == "" {
			return nil
		}
		provider, err := NewTenantModelProviderDAO().GetByTenantIDAndProviderName(ctx, db, tenantID, providerName)
		if err != nil || provider == nil {
			return nil
		}
		instance, err := NewTenantModelInstanceDAO().GetByProviderIDAndInstanceName(ctx, db, provider.ID, instanceName)
		if err != nil || instance == nil || instance.Status != "active" {
			return nil
		}
		obj, err := NewTenantModelDAO().GetByProviderIDAndInstanceIDAndModelTypeAndModelName(ctx, db, provider.ID, instance.ID, int(entity.ModelTypeChat), pureName)
		if err != nil || obj == nil {
			return nil
		}
		return obj
	}
	if modelRef == "" {
		return nil
	}
	obj, err := NewTenantModelDAO().GetByID(ctx, db, modelRef)
	if err != nil || obj == nil {
		return nil
	}
	// UUIDs are globally unique and unguessable, and shared/joined-tenant
	// models are legitimate (Python's get_model_config_by_id and Go's
	// service.GetModelConfigByID both support them), so UUID resolution is
	// intentionally NOT scoped to tenantID. The per-model override and the
	// catalog fallback apply to whichever tenant holds the UUID.
	return obj
}

// modelExtraMaxTokens returns the per-model "max_tokens" override from the
// tenant_model.extra JSON, or 0 when absent/invalid/non-positive. Mirrors
// Python's model_extra.get("max_tokens") — the custom context-window length a
// user configures for a model. Both JSON numbers and numeric strings are
// accepted (some callers persist extra.max_tokens as a string).
func modelExtraMaxTokens(extra string) int {
	if strings.TrimSpace(extra) == "" {
		return 0
	}
	var m struct {
		MaxTokens json.RawMessage `json:"max_tokens"`
	}
	if err := json.Unmarshal([]byte(extra), &m); err != nil || len(m.MaxTokens) == 0 {
		return 0
	}
	// Accept JSON numbers (int or float, e.g. 32000 / 32000.0) and numeric
	// strings ("32000"); json.Number keeps the raw lexical form.
	var num json.Number
	if err := json.Unmarshal(m.MaxTokens, &num); err == nil {
		if f, err := strconv.ParseFloat(num.String(), 64); err == nil && f > 0 {
			return int(f)
		}
		return 0
	}
	var str string
	if err := json.Unmarshal(m.MaxTokens, &str); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(str)); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// splitCompositeModelRef splits a composite "model@provider" or
// "model@instance@provider" reference into its parts. The instance defaults
// to "default" for the two-part form.
func splitCompositeModelRef(ref string) (modelName, instanceName, providerName string, ok bool) {
	parts := strings.Split(ref, "@")
	switch len(parts) {
	case 2:
		return parts[0], "default", parts[1], true
	case 3:
		return parts[0], parts[1], parts[2], true
	}
	if len(parts) > 3 {
		// 4+ segments: any '@' embedded in the leftmost modelName component
		// must be preserved (e.g. LM Studio chat models
		// `name@q8_0@lmstudio@LM-Studio`). Rejoin the leading fields into the
		// model name, keeping instance and provider anchored on the right.
		n := len(parts)
		return strings.Join(parts[:n-2], "@"), parts[n-2], parts[n-1], true
	}
	return "", "", "", false
}
