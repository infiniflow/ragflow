//
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

package component

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
)

func TestConfiguredAudioModelID(t *testing.T) {
	tests := []struct {
		name  string
		setup schema.ParserSetup
		want  string
	}{
		{
			name:  "audio uses vlm.llm_id",
			setup: schema.ParserSetup{"vlm": map[string]any{"llm_id": "whisper@openai"}},
			want:  "whisper@openai",
		},
		{
			name:  "empty setup",
			setup: schema.ParserSetup{},
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := configuredAudioModelID(tc.setup); got != tc.want {
				t.Fatalf("configuredAudioModelID() = %q, want %q", got, tc.want)
			}
		})
	}
}

// Models stored under Python's "Mistral OCR" provider name resolve to the
// Mistral driver and catalog entry, by tenant model id and by composite
// reference, enrolled or catalog-only. The instance key is passed through as
// stored; OCRFile reads the bare key from it.
func TestResolveModelConfigForStoredMistralOCRProvider(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const storedKey = `{"MISTRAL_OCR_API_KEY": "sk-env"}`
	for _, row := range []interface{}{
		&entity.TenantModelProvider{ID: "provider-mistral-ocr", TenantID: "tenant-1", ProviderName: "Mistral OCR"},
		&entity.TenantModelInstance{ID: "instance-mistral-ocr", ProviderID: "provider-mistral-ocr", InstanceName: "default", APIKey: storedKey, Status: "active", Extra: "{}"},
		&entity.TenantModel{ID: "model-mistral-ocr", ProviderID: "provider-mistral-ocr", InstanceID: "instance-mistral-ocr", ModelName: "mistral-ocr-latest", ModelType: int(entity.ModelTypeOCR), Status: "active"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}

	for _, tc := range []struct {
		name      string
		modelRef  string
		wantModel string
	}{
		{name: "tenant model id", modelRef: "model-mistral-ocr", wantModel: "mistral-ocr-latest"},
		{name: "enrolled composite", modelRef: "mistral-ocr-latest@default@Mistral OCR", wantModel: "mistral-ocr-latest"},
		{name: "catalog composite", modelRef: "mistral-ocr-2512@default@Mistral OCR", wantModel: "mistral-ocr-2512"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			driver, modelName, apiConfig, contextLength, err := defaultResolveModelConfig(t.Context(), db, "tenant-1", entity.ModelTypeOCR, tc.modelRef)
			if err != nil {
				t.Fatalf("resolve %q: %v", tc.modelRef, err)
			}
			if driver == nil || driver.Name() != "mistral" {
				t.Fatalf("driver = %v, want the mistral driver", driver)
			}
			if modelName != tc.wantModel {
				t.Fatalf("model name = %q, want %q", modelName, tc.wantModel)
			}
			// 8192 is the catalog max_tokens of the Mistral OCR models.
			if contextLength != 8192 {
				t.Fatalf("context length = %d, want 8192 from the Mistral catalog", contextLength)
			}
			if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey != storedKey {
				t.Fatalf("api config = %+v, want the stored key", apiConfig)
			}
		})
	}
}

// TestResolveModelConfigInaccessibleIDs checks that missing and foreign model
// IDs have the same public error and trigger no instance reads before access
// is established, regardless of the foreign model's state.
func TestResolveModelConfigInaccessibleIDs(t *testing.T) {
	for _, tc := range []struct {
		name, table, id, field string
		value                  any
	}{
		{name: "active foreign model"},
		{name: "disabled foreign model", table: "tenant_model", id: "model-b", field: "status", value: "inactive"},
		{name: "wrong foreign model type", table: "tenant_model", id: "model-b", field: "model_type", value: int(entity.ModelTypeChat)},
		{name: "missing foreign instance", table: "tenant_model", id: "model-b", field: "instance_id", value: "missing-instance"},
		{name: "disabled foreign instance", table: "tenant_model_instance", id: "instance-b", field: "status", value: "inactive"},
		{name: "mismatched foreign instance provider", table: "tenant_model_instance", id: "instance-b", field: "provider_id", value: "other-provider"},
		{name: "missing foreign provider", table: "tenant_model", id: "model-b", field: "provider_id", value: "missing-provider"},
		{name: "unknown foreign driver", table: "tenant_model_provider", id: "provider", field: "provider_name", value: "missing-driver"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := imageOCRFixture(t, "PaddleOCR.local", false)
			if tc.table != "" {
				if err := db.Table(tc.table).Where("id = ?", tc.id).Update(tc.field, tc.value).Error; err != nil {
					t.Fatal(err)
				}
			}
			counts := imageOCRModelLookupCounts(t, db)
			_, _, _, _, missingErr := defaultResolveModelConfig(t.Context(), db, "tenant-2", entity.ModelTypeOCR, "missing-model")
			driver, name, apiConfig, contextLength, err := defaultResolveModelConfig(t.Context(), db, "tenant-2", entity.ModelTypeOCR, "model-b")
			if missingErr == nil || err == nil {
				t.Fatalf("missing and foreign IDs must be rejected: missing=%v foreign=%v", missingErr, err)
			}
			if err.Error() != missingErr.Error() || !strings.Contains(strings.ToLower(err.Error()), "not found") {
				t.Errorf("foreign ID is distinguishable: foreign=%q missing=%q", err, missingErr)
			}
			if strings.Contains(err.Error(), "tenant-1") {
				t.Errorf("owner tenant ID leaked: %v", err)
			}
			if driver != nil || name != "" || apiConfig != nil || contextLength != 0 {
				t.Error("inaccessible resolution returned a partial configuration")
			}
			if counts["tenant_model_instance"] != 0 {
				t.Errorf("instance was queried before authorizing its model provider: counts=%v", counts)
			}
		})
	}
}

// TestResolveModelConfigAuthorizedModelValidation preserves selected instance
// credentials, max_tokens overrides, and status/type errors for owned models.
func TestResolveModelConfigAuthorizedModelValidation(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		value       any
		wantError   string
	}{
		{name: "active OCR"},
		{name: "disabled model", field: "status", value: "inactive", wantError: "disabled"},
		{name: "wrong model type", field: "model_type", value: int(entity.ModelTypeChat), wantError: "cannot be used as ocr"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := imageOCRFixture(t, "PaddleOCR.local", false)
			if err := db.Model(&entity.TenantModel{}).Where("id = ?", "model-b").
				Update("extra", `{"max_tokens":1234}`).Error; err != nil {
				t.Fatal(err)
			}
			if tc.field != "" {
				if err := db.Model(&entity.TenantModel{}).Where("id = ?", "model-b").Update(tc.field, tc.value).Error; err != nil {
					t.Fatal(err)
				}
			}
			driver, name, apiConfig, contextLength, err := defaultResolveModelConfig(t.Context(), db, "tenant-1", entity.ModelTypeOCR, "model-b")
			if tc.wantError != "" {
				if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.wantError) {
					t.Fatalf("authorized model validation changed: %v", err)
				}
				if driver != nil || name != "" || apiConfig != nil || contextLength != 0 {
					t.Fatal("invalid model returned a partial configuration")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if driver == nil || driver.Name() != "paddleocr.local" ||
				name != "PaddleOCR-VL-1.5" || contextLength != 1234 {
				t.Fatalf("authorized model changed: driver=%v name=%q contextLength=%d", driver, name, contextLength)
			}
			if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey != "fixture-key-b" ||
				apiConfig.BaseURL == nil || *apiConfig.BaseURL == "" {
				t.Fatal("selected instance configuration was not preserved")
			}
		})
	}
}

// TestResolveModelConfigLookupFailureIsNotMissing injects a database failure
// and checks that it remains distinguishable from a missing model, without a
// composite fallback masking the failure.
func TestResolveModelConfigLookupFailureIsNotMissing(t *testing.T) {
	db, _ := imageOCRFixture(t, "PaddleOCR.local", false)
	lookupErr := errors.New("fixture database unavailable")
	if err := db.Callback().Query().Before("gorm:query").Register("test:fail_model_lookup", func(tx *gorm.DB) {
		if tx.Statement.Table == "tenant_model" {
			tx.AddError(lookupErr)
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, err := defaultResolveModelConfig(t.Context(), db, "tenant-1", entity.ModelTypeOCR, "model-b")
	if !errors.Is(err, lookupErr) {
		t.Fatalf("database failure was masked as missing/inaccessible or a composite fallback: %v", err)
	}
}

// TestResolveModelConfigByIDScopedQuerySQL captures the real dispatcher's DryRun
// SQL to verify tenant scoping and separate tenant/model bind parameters.
// Quote-bearing IDs must remain values, not become SQL conditions.
func TestResolveModelConfigByIDScopedQuerySQL(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	var queries []struct {
		sql  string
		vars []any
	}
	if err := db.Callback().Query().After("gorm:query").Register("test:capture_model_scope_sql", func(tx *gorm.DB) {
		if tx.DryRun && tx.Statement.Table == "tenant_model" {
			queries = append(queries, struct {
				sql  string
				vars []any
			}{sql: tx.Statement.SQL.String(), vars: append([]any(nil), tx.Statement.Vars...)})
		}
	}); err != nil {
		t.Fatal(err)
	}
	const tenantID = "tenant-' OR 1 = 1 --"
	const modelID = "model-' OR 1 = 1 --"
	// Exercise the real ID branch, rather than rebuilding its query here.
	// DryRun does not hydrate records, so only the generated SQL is inspected.
	defaultResolveModelConfig(t.Context(), db.Session(&gorm.Session{DryRun: true}), tenantID, entity.ModelTypeOCR, modelID)

	if len(queries) != 1 {
		t.Fatalf("model queries=%d, want one scoped lookup: %+v", len(queries), queries)
	}
	query := queries[0]
	const wantSQL = "SELECT * FROM `tenant_model` WHERE provider_id IN (SELECT `id` FROM `tenant_model_provider` WHERE tenant_id = ?) AND id = ? LIMIT 1"
	if query.sql != wantSQL {
		t.Errorf("scoped SQL=%q, want %q", query.sql, wantSQL)
	}
	if len(query.vars) != 2 || query.vars[0] != tenantID || query.vars[1] != modelID {
		t.Errorf("bound vars=%#v, want tenant ID then model ID", query.vars)
	}
	if strings.Contains(query.sql, tenantID) || strings.Contains(query.sql, modelID) {
		t.Errorf("an ID was interpolated into SQL: %q", query.sql)
	}
}

// TestResolveModelConfigChainedDBStatementIsolationFixedOnly checks statement
// isolation, caller-scope retention, and reuse for provider/instance lookups.
// It is a fixed-implementation regression, not an old-implementation dynamic repro.
// Do not run against the shared-Statement subquery version: SQL generation may
// recurse into the same Statement before an assertion can report the failure.
func TestResolveModelConfigChainedDBStatementIsolationFixedOnly(t *testing.T) {
	db, _ := imageOCRFixture(t, "PaddleOCR.local", false)
	baseURL := imageOCRInstanceBaseURL(t, db)
	// Where returns a clone0 handle whose Statement must remain caller-owned.
	chainedDB := db.Where("1 = ?", 1)
	statement := chainedDB.Statement
	originalModel := statement.Model
	originalSelects := append([]string(nil), statement.Selects...)
	originalWhere := clause.Where{Exprs: []clause.Expression{
		clause.Expr{SQL: "1 = ?", Vars: []any{1}},
	}}
	assertCallerUnchanged := func() {
		t.Helper()
		if chainedDB.Statement != statement {
			t.Fatal("resolver replaced the caller Statement")
		}
		if !reflect.DeepEqual(statement.Model, originalModel) {
			t.Fatalf("caller Model changed: got %T, want %T", statement.Model, originalModel)
		}
		if !reflect.DeepEqual(statement.Selects, originalSelects) {
			t.Fatalf("caller Selects changed: got %v, want %v", statement.Selects, originalSelects)
		}
		if got := statement.Clauses["WHERE"].Expression; !reflect.DeepEqual(got, originalWhere) {
			t.Fatalf("caller WHERE changed: got %#v, want %#v", got, originalWhere)
		}
	}
	assertCallerUnchanged()

	driver, name, apiConfig, _, err := defaultResolveModelConfig(t.Context(), chainedDB, "tenant-1", entity.ModelTypeOCR, "model-b")
	if err != nil {
		t.Fatalf("resolve selected model with chained DB: %v", err)
	}
	if driver == nil || driver.Name() != "paddleocr.local" || name != "PaddleOCR-VL-1.5" {
		t.Fatalf("selected model changed: driver=%v name=%q", driver, name)
	}
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey != "fixture-key-b" ||
		apiConfig.BaseURL == nil || *apiConfig.BaseURL != baseURL {
		t.Fatal("selected instance configuration changed")
	}
	assertCallerUnchanged()

	provider, err := dao.NewTenantModelProviderDAO().GetByID(t.Context(), chainedDB, "provider")
	if err != nil {
		t.Fatalf("reuse chained DB for provider lookup: %v", err)
	}
	if provider.ID != "provider" || provider.TenantID != "tenant-1" || provider.ProviderName != "PaddleOCR.local" {
		t.Fatalf("provider lookup used a polluted Statement: %+v", provider)
	}
	assertCallerUnchanged()

	instance, err := dao.NewTenantModelInstanceDAO().GetByID(t.Context(), chainedDB, "instance-b")
	if err != nil {
		t.Fatalf("reuse chained DB for instance lookup: %v", err)
	}
	if instance.ID != "instance-b" || instance.ProviderID != "provider" || instance.APIKey != "fixture-key-b" {
		t.Fatal("instance lookup used a polluted Statement")
	}
	assertCallerUnchanged()

	// Isolation must preserve caller scopes, not clear them with NewDB.
	_, _, _, _, err = defaultResolveModelConfig(t.Context(), db.Where("1 = ?", 0), "tenant-1", entity.ModelTypeOCR, "model-b")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("resolver dropped the caller's rejecting scope: %v", err)
	}
}
