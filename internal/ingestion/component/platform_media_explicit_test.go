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
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/progress"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/permission"
	"ragflow/internal/utility"
)

// platformMediaDB seeds a tenant with no own defaults, a disabled platform
// model (explicitly selected) and an active platform model set as the platform
// default for the same type.
func platformMediaDB(t *testing.T, modelType entity.ModelType, settingType string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&entity.Tenant{}, &entity.UserTenant{}, &entity.SystemSettings{}, &entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{}); err != nil {
		t.Fatal(err)
	}
	active := "1"
	rows := []any{
		&entity.SystemSettings{Name: entity.PlatformProviderLockSetting, Source: "system", DataType: "string", Value: "ready"},
		&entity.Tenant{ID: "tenant", Status: &active},
		&entity.UserTenant{ID: "owner", TenantID: "tenant", UserID: "owner", Role: "owner", Status: &active},
		&entity.TenantModelProvider{ID: "platform", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "instance", ProviderID: "platform", InstanceName: "default", APIKey: "secret", Status: "active", Extra: "{}"},
		&entity.TenantModel{ID: "selected-off", ProviderID: "platform", InstanceID: "instance", ModelName: "gpt-4o", ModelType: int(modelType), Status: "inactive"},
		&entity.TenantModel{ID: "platform-default", ProviderID: "platform", InstanceID: "instance", ModelName: "gpt-4o-mini", ModelType: int(modelType), Status: "active"},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := (dao.PlatformModelSettings{}).SetDefault(t.Context(), db, settingType, "platform-default"); err != nil {
		t.Fatal(err)
	}
	return db
}

// The real resolvers run; only the final driver is swapped so no network call
// is made. resolvedDefault records which model the default path picked.
func spyDefaultResolver(t *testing.T, drv modelModule.ModelDriver, resolvedDefault *string) {
	orig := resolveTenantModelByType
	t.Cleanup(func() { resolveTenantModelByType = orig })
	resolveTenantModelByType = func(ctx context.Context, db *gorm.DB, tenantID string, mt entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		_, name, cfg, n, err := defaultResolveTenantModelByType(ctx, db, tenantID, mt)
		if err != nil {
			return nil, "", nil, 0, err
		}
		*resolvedDefault = name
		return drv, name, cfg, n, nil
	}
}

func TestImageExplicitDisabledPlatformVLMDoesNotSwitch(t *testing.T) {
	db := platformMediaDB(t, entity.ModelTypeImage2Text, "vision")
	ctx := permission.WithTrustedModelTenant(t.Context(), "tenant")

	// Precondition: the explicit selection itself is rejected by the real resolver.
	if _, _, _, _, err := defaultResolveModelConfig(ctx, db, "tenant", entity.ModelTypeImage2Text, "selected-off"); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("precondition: explicit disabled platform VLM should be rejected, got %v", err)
	}

	var resolvedDefault string
	drv := &imagePromptCaptureDriver{}
	spyDefaultResolver(t, drv, &resolvedDefault)

	res, _, err := maybeDispatchImage(ctx, parser.NewPictureParser(), db, utility.FileTypeVISUAL, "photo.png", picturePNG(t),
		map[string]any{"tenant_id": "tenant"},
		map[string]schema.ParserSetup{"image": {"ocr_enabled": false}},
		true, visionSettings{modelID: "selected-off"})
	if err != nil || len(res.Warnings) == 0 {
		t.Fatalf("expected a warning with preserved parse result: err=%v warnings=%v", err, res.Warnings)
	}
	if resolvedDefault != "" || len(drv.captured) != 0 {
		t.Fatalf("explicit disabled VLM %q switched to default model %q (VLM called %d times)", "selected-off", resolvedDefault, len(drv.captured))
	}
}

func TestAudioExplicitDisabledPlatformASRDoesNotSwitch(t *testing.T) {
	db := platformMediaDB(t, entity.ModelTypeSpeech2Text, "asr")
	ctx := permission.WithTrustedModelTenant(t.Context(), "tenant")

	var resolvedDefault string
	drv := &audioTranscribeDriver{transcription: "from default"}
	spyDefaultResolver(t, drv, &resolvedDefault)

	res, _, err := maybeDispatchAudio(ctx, db, utility.FileTypeAURAL, "a.mp3", []byte("fake"),
		map[string]any{"tenant_id": "tenant"},
		map[string]schema.ParserSetup{"audio": {"vlm": map[string]any{"llm_id": "selected-off"}}})
	if err == nil || resolvedDefault != "" {
		t.Fatalf("explicit disabled ASR %q produced %#v via default %q", "selected-off", res.JSON, resolvedDefault)
	}
}

// TestPDFExplicitDisabledPlatformVLMWarns keeps the PDF parse when the selected
// vision model is unavailable, matching image files, and never uses the default.
func TestPDFExplicitDisabledPlatformVLMWarns(t *testing.T) {
	db := platformMediaDB(t, entity.ModelTypeImage2Text, "vision")
	ctx := permission.WithTrustedModelTenant(t.Context(), "tenant")

	var resolvedDefault string
	spyDefaultResolver(t, &imagePromptCaptureDriver{}, &resolvedDefault)

	result := parser.ParseResult{JSON: []parser.DocItem{
		parser.NewTextJSONItem("Before"),
		parser.TableItem{Table: &entity.TableData{Grid: [][]entity.TableCell{{{Text: "Cell"}}}}, Extra: map[string]any{"image": visionTestPNGBase64(t), "page_number": 1}},
	}}
	res, handled, err := maybeDispatchVisionEnhancement(ctx, db, utility.FileTypePDF, result,
		map[string]any{"tenant_id": "tenant"}, visionSettings{modelID: "selected-off"}, progress.Window{})
	if err != nil || handled {
		t.Fatalf("handled=%v err=%v, want the parse kept without enhancement", handled, err)
	}
	if resolvedDefault != "" {
		t.Fatalf("explicit disabled VLM switched to default model %q", resolvedDefault)
	}
	if len(res.JSON) != 2 || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], `"selected-off" is unavailable`) {
		t.Fatalf("result=%d items, warnings=%v", len(res.JSON), res.Warnings)
	}
}
