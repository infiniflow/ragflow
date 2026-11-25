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

// extractSelectedImageOCR accepts an enrolled tenant-model ID or a gp- model
// group reference, not an engine alias or catalog-only composite. An explicit
// selection never substitutes the tenant default, another instance, local
// DeepDoc, or a global server URL.
func extractSelectedImageOCR(ctx context.Context, db *gorm.DB, tenantID, modelID string, binary []byte) (string, error) {
	if strings.TrimSpace(tenantID) == "" || db == nil {
		return "", fmt.Errorf("selected image OCR requires a tenant and database")
	}
	// Group failover: resolution and endpoint setup run once per attempt and
	// their errors are terminal; only the OCRFile invocation failure flips a
	// gp- reference's current member inactive and replays once.
	type selectedImageOCRTarget struct {
		driver    modelModule.ModelDriver
		modelName string
		apiConfig *modelModule.APIConfig
	}
	resolveTarget := func(ctx context.Context) (*selectedImageOCRTarget, error) {
		resolved, err := resolveModelConfigByRef(ctx, db, tenantID, entity.ModelTypeOCR, modelID)
		if err != nil {
			return nil, fmt.Errorf("selected image OCR model: %w", err)
		}
		model, instance := resolved.model, resolved.instance
		if instance.ProviderID != model.ProviderID {
			return nil, fmt.Errorf("selected image OCR instance does not belong to the model provider")
		}
		if instance.Status != "active" {
			return nil, fmt.Errorf("selected image OCR instance is disabled")
		}
		target := &selectedImageOCRTarget{driver: resolved.driver, modelName: model.ModelName, apiConfig: resolved.apiConfig}
		if !isPaddleOCRDriver(target.driver) {
			return nil, fmt.Errorf("selected image OCR requires a PaddleOCR model")
		}
		baseURL := strings.TrimSpace(stringValue(target.apiConfig.BaseURL))
		if baseURL == "" {
			baseURL = modelModule.ProviderJSONConfigValueFromAPIConfig(target.apiConfig,
				"paddleocr_base_url", "PADDLEOCR_BASE_URL", "paddleocr_api_url", "PADDLEOCR_API_URL")
		}
		if baseURL == "" && target.driver.Name() == "paddleocr" {
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
			return nil, fmt.Errorf("selected image OCR instance has no base URL")
		}
		target.apiConfig.BaseURL = &baseURL
		return target, nil
	}

	var text string
	err := dao.InvokeWithGroupFailover(ctx, db, tenantID, entity.ModelTypeOCR, modelID,
		resolveTarget, nil,
		func(ctx context.Context, target *selectedImageOCRTarget) error {
			response, invErr := target.driver.OCRFile(ctx, &target.modelName, binary, nil, target.apiConfig, nil, nil)
			if invErr != nil {
				return fmt.Errorf("selected image OCR: %w", invErr)
			}
			if response == nil || response.Text == nil {
				return fmt.Errorf("selected image OCR returned no text")
			}
			text = strings.TrimSpace(*response.Text)
			return nil
		}, nil)
	if err != nil {
		return "", err
	}
	return text, nil
}
