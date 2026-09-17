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

// TestResolveModelContentLength_CompositeReference resolves context_length
// for a composite "model@provider" reference from the provider catalog.
func TestResolveModelContentLength_CompositeReference(t *testing.T) {
	if got := ResolveModelContentLength(t.Context(), nil, "", "gpt-4o@openai", "", ""); got != 128000 {
		t.Fatalf("ResolveModelContentLength(gpt-4o@openai) = %d, want 128000", got)
	}
}

// TestResolveModelContentLength_CompositeReferenceThreePart resolves
// context_length for a composite "model@instance@provider" reference — the
// instance-bearing form used by tenant model instances — from the provider
// catalog.
func TestResolveModelContentLength_CompositeReferenceThreePart(t *testing.T) {
	if got := ResolveModelContentLength(t.Context(), nil, "", "gpt-4o@default@openai", "", ""); got != 128000 {
		t.Fatalf("ResolveModelContentLength(gpt-4o@default@openai) = %d, want 128000", got)
	}
}

// TestResolveModelContentLength_MultiSegmentComposite documents that a
// reference with more than three "@" segments is still a composite ref: the
// leading segments are rejoined into the model name (preserving embedded '@',
// e.g. LM Studio chat models `name@q8_0@lmstudio@LM-Studio`), with instance
// and provider anchored on the right. When the catalog lookup fails it falls
// through to the driver+modelName fallback, and to 0 when no fallback exists.
func TestResolveModelContentLength_MultiSegmentComposite(t *testing.T) {
	if got := ResolveModelContentLength(t.Context(), nil, "", "a@b@c@d", "openai", "gpt-4o"); got != 128000 {
		t.Fatalf("ResolveModelContentLength(a@b@c@d, openai, gpt-4o) = %d, want 128000 (driver fallback)", got)
	}
	if got := ResolveModelContentLength(t.Context(), nil, "", "gpt-4o@default@openai@extra", "", ""); got != 0 {
		t.Fatalf("ResolveModelContentLength(gpt-4o@default@openai@extra) = %d, want 0", got)
	}
	modelName, instanceName, providerName, ok := splitCompositeModelRef("a@b@c@d")
	if !ok || modelName != "a@b" || instanceName != "c" || providerName != "d" {
		t.Fatalf("splitCompositeModelRef(a@b@c@d) = %q/%q/%q/%v, want a@b/c/d/true", modelName, instanceName, providerName, ok)
	}
}

// TestResolveModelContentLength_DriverModelFallback resolves context_length
// from the resolved driver + bare model name, which needs no database.
func TestResolveModelContentLength_DriverModelFallback(t *testing.T) {
	if got := ResolveModelContentLength(t.Context(), nil, "", "", "openai", "gpt-4o"); got != 128000 {
		t.Fatalf("ResolveModelContentLength(openai/gpt-4o) = %d, want 128000", got)
	}
}

// TestResolveModelContentLength_Unknown returns 0 for unknown references.
func TestResolveModelContentLength_Unknown(t *testing.T) {
	if got := ResolveModelContentLength(t.Context(), nil, "", "no-such-model@no-such-provider", "", ""); got != 0 {
		t.Fatalf("unknown model = %d, want 0", got)
	}
	if got := ResolveModelContentLength(t.Context(), nil, "", "", "", ""); got != 0 {
		t.Fatalf("empty reference = %d, want 0", got)
	}
}

// TestResolveModelContentLength_TenantModelUUID resolves context_length for a
// tenant_model UUID row through the provider catalog.
func TestResolveModelContentLength_TenantModelUUID(t *testing.T) {
	db := openModelContextTestDB(t)
	pushDB(t, db)
	ctx := t.Context()

	seedOpenAIChatModel(t, db, "")

	if got := ResolveModelContentLength(ctx, db, "tenant-1", "0123456789abcdef0123456789abcdef", "", ""); got != 128000 {
		t.Fatalf("ResolveModelContentLength(uuid) = %d, want 128000", got)
	}
}

// TestResolveModelContentLength_ExtraOverrideUUID verifies that a custom
// "max_tokens" context window in the tenant_model.extra wins over the
// provider catalog's context_length for a UUID reference.
func TestResolveModelContentLength_ExtraOverrideUUID(t *testing.T) {
	db := openModelContextTestDB(t)
	pushDB(t, db)
	ctx := t.Context()

	seedOpenAIChatModel(t, db, `{"max_tokens": 32000}`)

	if got := ResolveModelContentLength(ctx, db, "tenant-1", "0123456789abcdef0123456789abcdef", "", ""); got != 32000 {
		t.Fatalf("ResolveModelContentLength(uuid+extra) = %d, want 32000 (custom override wins over catalog 128000)", got)
	}
}

// TestResolveModelContentLength_ExtraOverrideComposite verifies the custom
// context window override for a composite reference resolved through the
// tenant's provider/instance/model rows.
func TestResolveModelContentLength_ExtraOverrideComposite(t *testing.T) {
	db := openModelContextTestDB(t)
	pushDB(t, db)
	ctx := t.Context()

	seedOpenAIChatModel(t, db, `{"max_tokens": 32000}`)

	if got := ResolveModelContentLength(ctx, db, "tenant-1", "gpt-4o@OpenAI", "", ""); got != 32000 {
		t.Fatalf("ResolveModelContentLength(composite+extra) = %d, want 32000", got)
	}
}

// TestResolveModelContentLength_CustomModelExtraComposite is the core
// custom-model scenario: a model name that is NOT in the provider catalog but
// carries a tenant-configured "max_tokens" override must resolve to that
// override (the catalog cannot provide a value).
func TestResolveModelContentLength_CustomModelExtraComposite(t *testing.T) {
	db := openModelContextTestDB(t)
	pushDB(t, db)
	ctx := t.Context()

	seedCustomChatModel(t, db, "my-local-model", `{"max_tokens": 32000}`)

	if got := ResolveModelContentLength(ctx, db, "tenant-1", "my-local-model@OpenAI", "", ""); got != 32000 {
		t.Fatalf("ResolveModelContentLength(custom+extra) = %d, want 32000", got)
	}
	// Without the override the custom model is unknown to the catalog → 0.
	if err := db.Model(&entity.TenantModel{}).
		Where("id = ?", "0123456789abcdef0123456789abcdef").
		Update("extra", "").Error; err != nil {
		t.Fatalf("clear custom model extra: %v", err)
	}
	if got := ResolveModelContentLength(ctx, db, "tenant-1", "my-local-model@OpenAI", "", ""); got != 0 {
		t.Fatalf("custom model without extra = %d, want 0", got)
	}
}

// TestResolveModelContentLength_InactiveInstanceCompositeFallsBackToCatalog
// verifies that a composite reference whose instance is inactive does not
// apply the override or the tenant-row catalog read: it falls back to the
// catalog by reference parts (gpt-4o → 128000).
func TestResolveModelContentLength_InactiveInstanceCompositeFallsBackToCatalog(t *testing.T) {
	db := openModelContextTestDB(t)
	pushDB(t, db)
	ctx := t.Context()

	seedOpenAIChatModel(t, db, `{"max_tokens": 32000}`)
	if err := db.Model(&entity.TenantModelInstance{}).
		Where("id = ?", "instance-1").
		Update("status", "inactive").Error; err != nil {
		t.Fatalf("set instance inactive: %v", err)
	}

	if got := ResolveModelContentLength(ctx, db, "tenant-1", "gpt-4o@OpenAI", "", ""); got != 128000 {
		t.Fatalf("inactive-instance composite = %d, want catalog 128000", got)
	}
}

// TestResolveModelContentLength_InactiveModelCompositeFallsBackToCatalog
// verifies that a composite reference whose model row is inactive does not
// apply the override or the tenant-row catalog read; it falls back to the
// catalog by reference parts (gpt-4o → 128000).
func TestResolveModelContentLength_InactiveModelCompositeFallsBackToCatalog(t *testing.T) {
	db := openModelContextTestDB(t)
	pushDB(t, db)
	ctx := t.Context()

	seedOpenAIChatModel(t, db, `{"max_tokens": 32000}`)
	if err := db.Model(&entity.TenantModel{}).
		Where("id = ?", "0123456789abcdef0123456789abcdef").
		Update("status", "inactive").Error; err != nil {
		t.Fatalf("set model inactive: %v", err)
	}

	if got := ResolveModelContentLength(ctx, db, "tenant-1", "gpt-4o@OpenAI", "", ""); got != 128000 {
		t.Fatalf("inactive-model composite = %d, want catalog 128000", got)
	}
}

// TestResolveModelContentLength_ExtraOverrideStringForm verifies that a
// max_tokens override persisted as a JSON string is still honored (some
// callers persist extra.max_tokens as a string).
func TestResolveModelContentLength_ExtraOverrideStringForm(t *testing.T) {
	db := openModelContextTestDB(t)
	pushDB(t, db)
	ctx := t.Context()

	seedOpenAIChatModel(t, db, `{"max_tokens": "32000"}`)

	if got := ResolveModelContentLength(ctx, db, "tenant-1", "0123456789abcdef0123456789abcdef", "", ""); got != 32000 {
		t.Fatalf("ResolveModelContentLength(string-form extra) = %d, want 32000", got)
	}
}

// TestResolveModelContentLength_NonNumericStringExtraFallsBackToCatalog
// verifies that a non-numeric max_tokens string is treated as "no override".
func TestResolveModelContentLength_NonNumericStringExtraFallsBackToCatalog(t *testing.T) {
	db := openModelContextTestDB(t)
	pushDB(t, db)
	ctx := t.Context()

	seedOpenAIChatModel(t, db, `{"max_tokens": "huge"}`)

	if got := ResolveModelContentLength(ctx, db, "tenant-1", "0123456789abcdef0123456789abcdef", "", ""); got != 128000 {
		t.Fatalf("ResolveModelContentLength(non-numeric string extra) = %d, want catalog 128000", got)
	}
}

// TestResolveModelContentLength_InvalidExtraFallsBackToCatalog verifies that
// an unparsable extra JSON is treated as "no override" and the catalog
// context_length is used.
func TestResolveModelContentLength_InvalidExtraFallsBackToCatalog(t *testing.T) {
	db := openModelContextTestDB(t)
	pushDB(t, db)
	ctx := t.Context()

	seedOpenAIChatModel(t, db, `{bad json`)

	if got := ResolveModelContentLength(ctx, db, "tenant-1", "0123456789abcdef0123456789abcdef", "", ""); got != 128000 {
		t.Fatalf("ResolveModelContentLength(invalid extra) = %d, want catalog 128000", got)
	}
}

// TestResolveModelContentLength_ZeroExtraFallsBackToCatalog verifies that a
// non-positive max_tokens override is ignored in favor of the catalog.
func TestResolveModelContentLength_ZeroExtraFallsBackToCatalog(t *testing.T) {
	db := openModelContextTestDB(t)
	pushDB(t, db)
	ctx := t.Context()

	seedOpenAIChatModel(t, db, `{"max_tokens": 0}`)

	if got := ResolveModelContentLength(ctx, db, "tenant-1", "0123456789abcdef0123456789abcdef", "", ""); got != 128000 {
		t.Fatalf("ResolveModelContentLength(zero extra) = %d, want catalog 128000", got)
	}
}

// TestResolveModelContentLength_InactiveTenantModel falls through to the
// composite/catalog paths when the tenant model row is not active — even when
// it carries a max_tokens override.
func TestResolveModelContentLength_InactiveTenantModel(t *testing.T) {
	db := openModelContextTestDB(t)
	pushDB(t, db)
	ctx := t.Context()

	seedOpenAIChatModel(t, db, `{"max_tokens": 32000}`)
	// Flip the row to inactive.
	if err := db.Model(&entity.TenantModel{}).
		Where("id = ?", "0123456789abcdef0123456789abcdef").
		Update("status", "inactive").Error; err != nil {
		t.Fatalf("set inactive: %v", err)
	}

	// The UUID is not a composite ref, so an inactive row yields 0.
	if got := ResolveModelContentLength(ctx, db, "tenant-1", "0123456789abcdef0123456789abcdef", "", ""); got != 0 {
		t.Fatalf("inactive tenant model = %d, want 0", got)
	}
}

// seedOpenAIChatModel seeds an active OpenAI gpt-4o tenant model (catalog
// context_length 128000) plus its provider and default instance. extra is the
// tenant_model.extra JSON ("" for none).
func seedOpenAIChatModel(t *testing.T, db *gorm.DB, extra string) {
	t.Helper()
	seedChatModel(t, db, "provider-openai", "OpenAI", "gpt-4o", extra)
}

// seedCustomChatModel seeds an active tenant model whose name is NOT in the
// provider catalog (custom/local model scenario).
func seedCustomChatModel(t *testing.T, db *gorm.DB, modelName, extra string) {
	t.Helper()
	seedChatModel(t, db, "provider-openai", "OpenAI", modelName, extra)
}

func seedChatModel(t *testing.T, db *gorm.DB, providerID, providerName, modelName, extra string) {
	t.Helper()
	if err := db.Create(&entity.TenantModelProvider{
		ID:           providerID,
		ProviderName: providerName,
		TenantID:     "tenant-1",
	}).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if err := db.Create(&entity.TenantModelInstance{
		ID:           "instance-1",
		ProviderID:   providerID,
		InstanceName: "default",
		Status:       "active",
	}).Error; err != nil {
		t.Fatalf("create instance: %v", err)
	}
	if err := db.Create(&entity.TenantModel{
		ID:         "0123456789abcdef0123456789abcdef",
		ProviderID: providerID,
		InstanceID: "instance-1",
		ModelName:  modelName,
		ModelType:  int(entity.ModelTypeChat),
		Status:     "active",
		Extra:      extra,
	}).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}
}

func openModelContextTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}
