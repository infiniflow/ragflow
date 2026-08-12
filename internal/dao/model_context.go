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
	"strings"

	"gorm.io/gorm"
)

// ResolveModelContentLength returns the chat model's context window
// (content_length) in tokens for modelRef — a tenant_model UUID or a
// composite "model@provider" / "model@instance@provider" reference — or 0
// when it cannot be resolved. driver and modelName are an optional provider
// catalog fallback used when modelRef is not a resolvable tenant id (for
// example when no database is available); pass empty strings to skip it.
//
// This is the shared implementation behind the agent LLM component, the
// ingestion Extractor, and service.ModelProviderService — those packages
// cannot import each other (import cycles), so the lookup lives here.
func ResolveModelContentLength(ctx context.Context, db *gorm.DB, modelRef, driver, modelName string) int {
	// 1. Composite "model@provider" / "model@instance@provider" reference:
	//    look up the provider catalog directly. A composite reference cannot
	//    be a tenant-model UUID, so resolve it before touching the database.
	if pureName, _, providerName, ok := splitCompositeModelRef(modelRef); ok {
		if mdl, err := GetModelProviderManager().GetModelByName(providerName, pureName); err == nil && mdl.ContentLength != nil {
			return *mdl.ContentLength
		}
	}
	if db == nil {
		db = DB
	}
	// 2. Tenant model UUID: read content_length from its provider catalog row.
	if db != nil && modelRef != "" {
		if obj, err := NewTenantModelDAO().GetByID(ctx, db, modelRef); err == nil && obj != nil && obj.Status == "active" {
			if provider, err := NewTenantModelProviderDAO().GetByID(ctx, db, obj.ProviderID); err == nil && provider != nil {
				if mdl, err := GetModelProviderManager().GetModelByName(provider.ProviderName, obj.ModelName); err == nil && mdl.ContentLength != nil {
					return *mdl.ContentLength
				}
			}
		}
	}
	// 3. Resolved driver + bare model name: fallback when modelRef is a
	//    tenant id that could not be resolved without a database.
	if driver != "" && modelName != "" {
		if mdl, err := GetModelProviderManager().GetModelByName(driver, modelName); err == nil && mdl.ContentLength != nil {
			return *mdl.ContentLength
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
	return "", "", "", false
}
