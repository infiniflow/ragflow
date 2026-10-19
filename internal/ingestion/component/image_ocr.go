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
	"fmt"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
)

// extractSelectedImageOCR accepts an enrolled tenant-model ID, not an engine
// alias or catalog-only composite. An explicit selection never substitutes the
// tenant default, another instance, local DeepDoc, or a global server URL.
func extractSelectedImageOCR(ctx context.Context, db *gorm.DB, tenantID, modelID string, binary []byte) (string, error) {
	if strings.TrimSpace(tenantID) == "" || db == nil {
		return "", fmt.Errorf("selected image OCR requires a tenant and database")
	}
	resolved, err := resolveModelConfigByID(ctx, db, tenantID, entity.ModelTypeOCR, modelID)
	if err != nil {
		return "", fmt.Errorf("selected image OCR model: %w", err)
	}
	model, instance := resolved.model, resolved.instance
	if instance.ProviderID != model.ProviderID {
		return "", fmt.Errorf("selected image OCR instance does not belong to the model provider")
	}
	if instance.Status != "active" {
		return "", fmt.Errorf("selected image OCR instance is disabled")
	}
	driver, modelName, apiConfig := resolved.driver, model.ModelName, resolved.apiConfig
	if !isPaddleOCRDriver(driver) {
		return "", fmt.Errorf("selected image OCR requires a PaddleOCR model")
	}
	baseURL := strings.TrimSpace(stringValue(apiConfig.BaseURL))
	if baseURL == "" {
		baseURL = modelModule.ProviderJSONConfigValueFromAPIConfig(apiConfig,
			"paddleocr_base_url", "PADDLEOCR_BASE_URL", "paddleocr_api_url", "PADDLEOCR_API_URL")
	}
	if baseURL == "" && driver.Name() == "paddleocr" {
		if provider := dao.GetModelProviderManager().FindProvider("PaddleOCR"); provider != nil {
			baseURL = strings.TrimSpace(provider.URL["default"])
			if baseURL != "" {
				// An explicitly selected cloud model may still use its catalog
				// endpoint. Make that choice observable without logging secrets.
				common.WarnCtx(ctx, "selected image OCR falls back to cloud provider default endpoint",
					zap.String("tenant_id", tenantID),
					zap.String("model_id", model.ID),
					zap.String("instance_id", instance.ID))
			}
		}
	}
	if baseURL == "" {
		return "", fmt.Errorf("selected image OCR instance has no base URL")
	}
	apiConfig.BaseURL = &baseURL
	response, err := driver.OCRFile(ctx, &modelName, binary, nil, apiConfig,
		&modelModule.OCRConfig{Algorithm: modelName}, nil)
	if err != nil {
		return "", fmt.Errorf("selected image OCR: %w", err)
	}
	if response == nil || response.Text == nil {
		return "", fmt.Errorf("selected image OCR returned no text")
	}
	return strings.TrimSpace(*response.Text), nil
}
