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
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// TestResolveModelContentLength_CompositeReference resolves content_length
// for a composite "model@provider" reference from the provider catalog.
func TestResolveModelContentLength_CompositeReference(t *testing.T) {
	if got := ResolveModelContentLength(t.Context(), nil, "gpt-4o@openai", "", ""); got != 128000 {
		t.Fatalf("ResolveModelContentLength(gpt-4o@openai) = %d, want 128000", got)
	}
}

// TestResolveModelContentLength_CompositeReferenceThreePart resolves
// content_length for a composite "model@instance@provider" reference — the
// instance-bearing form used by tenant model instances — from the provider
// catalog.
func TestResolveModelContentLength_CompositeReferenceThreePart(t *testing.T) {
	if got := ResolveModelContentLength(t.Context(), nil, "gpt-4o@default@openai", "", ""); got != 128000 {
		t.Fatalf("ResolveModelContentLength(gpt-4o@default@openai) = %d, want 128000", got)
	}
}

// TestResolveModelContentLength_TooManyParts documents that a reference with
// more than two "@" separators is not treated as a composite ref: it falls
// through to the driver+modelName fallback, and to 0 when no fallback is
// supplied. A known catalog model with an extra separator is used for the
// no-fallback assertion so a parser regression (accepting excessive
// separators) fails loudly instead of resolving to 0 by coincidence.
func TestResolveModelContentLength_TooManyParts(t *testing.T) {
	if got := ResolveModelContentLength(t.Context(), nil, "a@b@c@d", "openai", "gpt-4o"); got != 128000 {
		t.Fatalf("ResolveModelContentLength(a@b@c@d, openai, gpt-4o) = %d, want 128000 (driver fallback)", got)
	}
	if got := ResolveModelContentLength(t.Context(), nil, "gpt-4o@default@openai@extra", "", ""); got != 0 {
		t.Fatalf("ResolveModelContentLength(gpt-4o@default@openai@extra) = %d, want 0", got)
	}
	if _, _, _, ok := splitCompositeModelRef("a@b@c@d"); ok {
		t.Fatal("splitCompositeModelRef(a@b@c@d) accepted an excessive-separator reference")
	}
}

// TestResolveModelContentLength_DriverModelFallback resolves content_length
// from the resolved driver + bare model name, which needs no database.
func TestResolveModelContentLength_DriverModelFallback(t *testing.T) {
	if got := ResolveModelContentLength(t.Context(), nil, "", "openai", "gpt-4o"); got != 128000 {
		t.Fatalf("ResolveModelContentLength(openai/gpt-4o) = %d, want 128000", got)
	}
}

// TestResolveModelContentLength_Unknown returns 0 for unknown references.
func TestResolveModelContentLength_Unknown(t *testing.T) {
	if got := ResolveModelContentLength(t.Context(), nil, "no-such-model@no-such-provider", "", ""); got != 0 {
		t.Fatalf("unknown model = %d, want 0", got)
	}
	if got := ResolveModelContentLength(t.Context(), nil, "", "", ""); got != 0 {
		t.Fatalf("empty reference = %d, want 0", got)
	}
}

// TestResolveModelContentLength_TenantModelUUID resolves content_length for a
// tenant_model UUID row through the provider catalog.
func TestResolveModelContentLength_TenantModelUUID(t *testing.T) {
	db := openModelContextTestDB(t)
	pushDB(t, db)
	ctx := t.Context()

	if err := db.Create(&entity.TenantModelProvider{
		ID:           "provider-openai",
		ProviderName: "OpenAI",
		TenantID:     "tenant-1",
	}).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if err := db.Create(&entity.TenantModel{
		ID:         "0123456789abcdef0123456789abcdef",
		ProviderID: "provider-openai",
		InstanceID: "instance-1",
		ModelName:  "gpt-4o",
		ModelType:  int(entity.ModelTypeChat),
		Status:     "active",
	}).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}

	if got := ResolveModelContentLength(ctx, db, "0123456789abcdef0123456789abcdef", "", ""); got != 128000 {
		t.Fatalf("ResolveModelContentLength(uuid) = %d, want 128000", got)
	}
}

// TestResolveModelContentLength_InactiveTenantModel falls through to the
// composite/catalog paths when the tenant model row is not active.
func TestResolveModelContentLength_InactiveTenantModel(t *testing.T) {
	db := openModelContextTestDB(t)
	pushDB(t, db)
	ctx := t.Context()

	if err := db.Create(&entity.TenantModelProvider{
		ID:           "provider-openai",
		ProviderName: "OpenAI",
		TenantID:     "tenant-1",
	}).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if err := db.Create(&entity.TenantModel{
		ID:         "0123456789abcdef0123456789abcdef",
		ProviderID: "provider-openai",
		InstanceID: "instance-1",
		ModelName:  "gpt-4o",
		ModelType:  int(entity.ModelTypeChat),
		Status:     "inactive",
	}).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}

	// The UUID is not a composite ref, so an inactive row yields 0.
	if got := ResolveModelContentLength(ctx, db, "0123456789abcdef0123456789abcdef", "", ""); got != 0 {
		t.Fatalf("inactive tenant model = %d, want 0", got)
	}
}

func openModelContextTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.TenantModelProvider{}, &entity.TenantModel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}
