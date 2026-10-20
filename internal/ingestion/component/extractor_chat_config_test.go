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

package component

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
)

// namedDriver is a ModelDriver stub that only knows its name; the chat-config
// resolvers touch nothing else on the driver.
type namedDriver struct {
	models.ModelDriver
	name string
}

func (d namedDriver) Name() string { return d.name }

// swapModelResolvers installs fake model resolvers for one test. Each argument
// is optional; a nil argument leaves the corresponding production resolver in
// place.
func swapModelResolvers(t *testing.T, byConfig, byType func(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType, ref string) (models.ModelDriver, string, *models.APIConfig, int, error)) {
	t.Helper()
	origConfig, origType := resolveModelConfig, resolveTenantModelByType
	t.Cleanup(func() {
		resolveModelConfig, resolveTenantModelByType = origConfig, origType
	})
	if byConfig != nil {
		resolveModelConfig = byConfig
	}
	if byType != nil {
		resolveTenantModelByType = func(ctx context.Context, db *gorm.DB, tenantID string, modelType entity.ModelType) (models.ModelDriver, string, *models.APIConfig, int, error) {
			return byType(ctx, db, tenantID, modelType, "")
		}
	}
}

func TestResolveExtractorChatConfig_CompositePath(t *testing.T) {
	swapModelResolvers(t, func(_ context.Context, _ *gorm.DB, tenantID string, modelType entity.ModelType, ref string) (models.ModelDriver, string, *models.APIConfig, int, error) {
		if tenantID != "tenant-1" {
			t.Errorf("tenantID = %q, want tenant-1", tenantID)
		}
		if modelType != entity.ModelTypeChat {
			t.Errorf("modelType = %v, want chat", modelType)
		}
		if ref != "gpt-4o@openai" {
			t.Errorf("ref = %q, want the composite reference", ref)
		}
		return namedDriver{name: "OpenAI"}, "gpt-4o", &models.APIConfig{ApiKey: ptr("sk-secret"), BaseURL: ptr("https://api.example.com")}, 0, nil
	}, nil)

	ctx := extractorStateCtx(t, "tenant-1")
	got, err := resolveExtractorChatConfig(ctx, nil, "gpt-4o@openai")
	if err != nil {
		t.Fatalf("resolveExtractorChatConfig: %v", err)
	}
	if got.driver != "openai" {
		t.Errorf("driver = %q, want the lowercased factory name", got.driver)
	}
	if got.modelName != "gpt-4o" {
		t.Errorf("modelName = %q, want gpt-4o", got.modelName)
	}
	if got.apiKey != "sk-secret" {
		t.Errorf("apiKey = %q, want the configured key", got.apiKey)
	}
	if got.baseURL != "https://api.example.com" {
		t.Errorf("baseURL = %q, want the configured base URL", got.baseURL)
	}
}

func TestResolveExtractorChatConfig_MixedCaseDriverAndNilAPIConfig(t *testing.T) {
	swapModelResolvers(t, func(_ context.Context, _ *gorm.DB, _ string, _ entity.ModelType, _ string) (models.ModelDriver, string, *models.APIConfig, int, error) {
		return namedDriver{name: "AzureOpenAI"}, "gpt-4o", nil, 0, nil
	}, nil)

	got, err := resolveExtractorChatConfig(extractorStateCtx(t, "tenant-1"), nil, "gpt-4o@azure")
	if err != nil {
		t.Fatalf("resolveExtractorChatConfig: %v", err)
	}
	if got.driver != "azureopenai" {
		t.Errorf("driver = %q, want azureopenai", got.driver)
	}
	if got.apiKey != "" || got.baseURL != "" {
		t.Errorf("apiKey/baseURL = %q/%q, want empty for a nil api config", got.apiKey, got.baseURL)
	}
}

func TestResolveExtractorChatConfig_ResolverErrorIsWrapped(t *testing.T) {
	swapModelResolvers(t, func(_ context.Context, _ *gorm.DB, _ string, _ entity.ModelType, ref string) (models.ModelDriver, string, *models.APIConfig, int, error) {
		return nil, "", nil, 0, errors.New("provider unreachable")
	}, nil)

	_, err := resolveExtractorChatConfig(extractorStateCtx(t, "tenant-1"), nil, "gpt-4o@openai")
	if err == nil {
		t.Fatal("expected an error when the resolver fails")
	}
	if !strings.Contains(err.Error(), "gpt-4o@openai") {
		t.Errorf("error %q does not name the model reference", err.Error())
	}
}

func TestResolveExtractorChatConfig_NoTenantOrStateYieldsEmptyConfig(t *testing.T) {
	swapModelResolvers(t, func(_ context.Context, _ *gorm.DB, _ string, _ entity.ModelType, _ string) (models.ModelDriver, string, *models.APIConfig, int, error) {
		t.Error("resolver must not run without a tenant_id")
		return nil, "", nil, 0, nil
	}, nil)

	for name, ctx := range map[string]context.Context{
		"empty tenant_id": extractorStateCtx(t, ""),
		"no canvas state": t.Context(),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := resolveExtractorChatConfig(ctx, nil, "gpt-4o@openai")
			if err != nil {
				t.Fatalf("resolveExtractorChatConfig: %v", err)
			}
			if got != (extractorChatConfig{}) {
				t.Fatalf("config = %+v, want the empty config", got)
			}
		})
	}
}

func TestResolveExtractorChatDefaultConfig_TenantDefault(t *testing.T) {
	swapModelResolvers(t, nil, func(_ context.Context, _ *gorm.DB, tenantID string, modelType entity.ModelType, _ string) (models.ModelDriver, string, *models.APIConfig, int, error) {
		if tenantID != "tenant-1" || modelType != entity.ModelTypeChat {
			t.Errorf("resolver args = (%q, %v)", tenantID, modelType)
		}
		return namedDriver{name: "SiliconFlow"}, "bge-m3", &models.APIConfig{ApiKey: ptr("sk-default"), BaseURL: ptr("https://api.siliconflow.cn")}, 0, nil
	})

	got := resolveExtractorChatDefaultConfig(extractorStateCtx(t, "tenant-1"), nil)
	if got.driver != "siliconflow" {
		t.Errorf("driver = %q, want siliconflow", got.driver)
	}
	if got.modelName != "bge-m3" {
		t.Errorf("modelName = %q, want bge-m3", got.modelName)
	}
	if got.apiKey != "sk-default" || got.baseURL != "https://api.siliconflow.cn" {
		t.Errorf("apiKey/baseURL = %q/%q, want the configured credentials", got.apiKey, got.baseURL)
	}
}

func TestResolveExtractorChatDefaultConfig_FailuresYieldEmptyConfig(t *testing.T) {
	t.Run("resolver error", func(t *testing.T) {
		swapModelResolvers(t, nil, func(_ context.Context, _ *gorm.DB, _ string, _ entity.ModelType, _ string) (models.ModelDriver, string, *models.APIConfig, int, error) {
			return nil, "", nil, 0, errors.New("no default model")
		})
		if got := resolveExtractorChatDefaultConfig(extractorStateCtx(t, "tenant-1"), nil); got != (extractorChatConfig{}) {
			t.Fatalf("config = %+v, want the empty config on resolver error", got)
		}
	})

	t.Run("nil driver", func(t *testing.T) {
		swapModelResolvers(t, nil, func(_ context.Context, _ *gorm.DB, _ string, _ entity.ModelType, _ string) (models.ModelDriver, string, *models.APIConfig, int, error) {
			return nil, "bge-m3", nil, 0, nil
		})
		if got := resolveExtractorChatDefaultConfig(extractorStateCtx(t, "tenant-1"), nil); got != (extractorChatConfig{}) {
			t.Fatalf("config = %+v, want the empty config on a nil driver", got)
		}
	})

	t.Run("no tenant", func(t *testing.T) {
		swapModelResolvers(t, nil, func(_ context.Context, _ *gorm.DB, _ string, _ entity.ModelType, _ string) (models.ModelDriver, string, *models.APIConfig, int, error) {
			t.Error("resolver must not run without a tenant_id")
			return nil, "", nil, 0, nil
		})
		if got := resolveExtractorChatDefaultConfig(t.Context(), nil); got != (extractorChatConfig{}) {
			t.Fatalf("config = %+v, want the empty config without canvas state", got)
		}
	})
}
