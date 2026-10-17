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

package service

import (
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// mistralOCRStoredKey is the api_key Python's "Mistral OCR" factory stored when
// provisioned from the environment; the model migration copies it verbatim.
const mistralOCRStoredKey = `{"MISTRAL_OCR_API_KEY": "sk-env", "MISTRAL_OCR_BASE_URL": "https://api.mistral.ai/v1"}`

// seedStoredMistralOCRProvider seeds a tenant whose provider row carries the
// "Mistral OCR" name, with mistral-ocr-latest enrolled as an ocr model.
// mistral-ocr-2512 is left unenrolled so composite references to it resolve
// through the catalog.
func seedStoredMistralOCRProvider(t *testing.T, db *gorm.DB) {
	t.Helper()
	activeStatus := "1"
	rows := []interface{}{
		&entity.UserTenant{ID: "user-tenant-1", UserID: "user-1", TenantID: "tenant-1", Role: "owner", InvitedBy: "user-1", Status: &activeStatus},
		&entity.TenantModelProvider{ID: "provider-mistral-ocr", TenantID: "tenant-1", ProviderName: "Mistral OCR"},
		&entity.TenantModelInstance{ID: "instance-mistral-ocr", ProviderID: "provider-mistral-ocr", InstanceName: "default", APIKey: mistralOCRStoredKey, Status: "active", Extra: "{}"},
		&entity.TenantModel{ID: "model-mistral-ocr", ProviderID: "provider-mistral-ocr", InstanceID: "instance-mistral-ocr", ModelName: "mistral-ocr-latest", ModelType: int(entity.ModelTypeOCR), Status: "active"},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("failed to seed %T: %v", row, err)
		}
	}
}

// Model references stored under the "Mistral OCR" provider name resolve to the
// Mistral driver and catalog entry, whether they name the tenant model by id
// or by composite, enrolled or catalog-only. The target keeps the stored
// provider name and the stored key.
func TestModelFactoryResolvesStoredMistralOCRProvider(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	seedStoredMistralOCRProvider(t, db)

	access := ModelAccess{TenantID: "tenant-1", UserID: "user-1"}
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
			target, err := NewModelFactory().resolveConfig(t.Context(), access, entity.ModelTypeOCR, tc.modelRef)
			if err != nil {
				t.Fatalf("resolveConfig(%q): %v", tc.modelRef, err)
			}
			if target.Driver == nil || target.Driver.Name() != "mistral" {
				t.Fatalf("driver = %v, want the mistral driver", target.Driver)
			}
			if target.ModelName != tc.wantModel || target.ModelInfo == nil || !target.ModelInfo.ModelTypeMap["ocr"] {
				t.Fatalf("model = %q / %+v, want %s from the catalog", target.ModelName, target.ModelInfo, tc.wantModel)
			}
			if target.ProviderName != "Mistral OCR" {
				t.Fatalf("provider name = %q, want the stored name", target.ProviderName)
			}
			if target.APIConfig == nil || target.APIConfig.ApiKey == nil || *target.APIConfig.ApiKey != mistralOCRStoredKey {
				t.Fatalf("api key = %v, want the stored key", target.APIConfig)
			}
		})
	}
}

// ResolveModelType reads the catalog types of a composite reference that is
// not enrolled under the "Mistral OCR" provider row.
func TestModelFactoryResolveModelTypeForStoredMistralOCRProvider(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	seedStoredMistralOCRProvider(t, db)

	types, err := NewModelFactory().ResolveModelType(t.Context(), ModelAccess{TenantID: "tenant-1", UserID: "user-1"}, "mistral-ocr-2512@default@Mistral OCR")
	if err != nil {
		t.Fatalf("ResolveModelType: %v", err)
	}
	if len(types) != 1 || types[0] != entity.ModelTypeOCR {
		t.Fatalf("types = %v, want [ocr]", types)
	}
}

// Tool support falls back to the catalog entry of the mapped provider.
func TestCatalogToolSupportForStoredMistralOCRProvider(t *testing.T) {
	if !catalogToolSupport("Mistral", "mistral-large-latest") {
		t.Skip("Mistral catalog no longer lists tool support for mistral-large-latest")
	}
	if !catalogToolSupport("Mistral OCR", "mistral-large-latest") {
		t.Fatal(`catalogToolSupport("Mistral OCR", "mistral-large-latest") = false, want the Mistral catalog value`)
	}
}

// The model call service resolves a "Mistral OCR" model by tenant model id and
// by composite reference.
func TestModelCallServiceResolvesStoredMistralOCRProvider(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	seedStoredMistralOCRProvider(t, db)

	svc := NewModelCallService()
	for _, modelRef := range []string{"model-mistral-ocr", "mistral-ocr-latest@default@Mistral OCR"} {
		t.Run(modelRef, func(t *testing.T) {
			info, err := svc.resolveModelInfo(t.Context(), modelRef, "user-1")
			if err != nil {
				t.Fatalf("resolveModelInfo(%q): %v", modelRef, err)
			}
			if info.ProviderInfo == nil || info.ProviderInfo.Name != "Mistral" {
				t.Fatalf("provider info = %+v, want the Mistral catalog entry", info.ProviderInfo)
			}
			if info.ModelInfo == nil || info.ModelInfo.Name != "mistral-ocr-latest" {
				t.Fatalf("model info = %+v, want mistral-ocr-latest", info.ModelInfo)
			}
			if info.ProviderEntity == nil || info.ProviderEntity.ProviderName != "Mistral OCR" {
				t.Fatalf("provider entity = %+v, want the stored row", info.ProviderEntity)
			}
		})
	}
}

// The mapping is limited to model resolution: the provider list still skips a
// "Mistral OCR" row, as it does for any name the catalog does not serve.
func TestListProvidersOfTenantSkipsStoredMistralOCRProvider(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	seedStoredMistralOCRProvider(t, db)
	if err := db.Create(&entity.TenantModelProvider{ID: "provider-openai", TenantID: "tenant-1", ProviderName: "OpenAI"}).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}

	providers, code, err := NewModelProviderService().ListProvidersOfTenant(t.Context(), "user-1")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListProvidersOfTenant = (%v, %v)", code, err)
	}
	var names []string
	for _, provider := range providers {
		names = append(names, provider["name"].(string))
	}
	if len(names) != 1 || names[0] != "OpenAI" {
		t.Fatalf("providers = %v, want only OpenAI", names)
	}
}
