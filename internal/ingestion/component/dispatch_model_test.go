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
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

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
