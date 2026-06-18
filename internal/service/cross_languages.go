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
	"context"
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"

	"go.uber.org/zap"
)

// CrossLanguages translates a question into multiple languages using LLM.
// The model is fetched internally based on llmID:
//   - If llmID is empty, fetches tenant's default chat model
//   - If llmID is not empty, fetches the specified model (or image2text if type matches)
func CrossLanguages(ctx context.Context, tenantID string, llmID string, query string, languages []string) (string, error) {
	common.Debug("CrossLanguages invoked",
		zap.String("tenantID", tenantID),
		zap.String("llmID", llmID),
		zap.Strings("languages", languages))

	modelProviderSvc := NewModelProviderService()
	var chatModel *modelModule.ChatModel

	if llmID != "" {
		modelTypes, err := modelProviderSvc.GetModelTypeByName(tenantID, llmID)
		if err != nil {
			return query, fmt.Errorf("failed to get model type: %w", err)
		}
		resolvedType := entity.ModelTypeChat
		for _, mt := range modelTypes {
			if mt == entity.ModelTypeImage2Text {
				resolvedType = entity.ModelTypeImage2Text
				break
			}
		}
		driver, modelName, apiConfig, _, err := modelProviderSvc.GetModelConfigFromProviderInstance(tenantID, resolvedType, llmID)
		if err != nil {
			return query, fmt.Errorf("failed to get chat model: %w", err)
		}
		chatModel = modelModule.NewChatModel(driver, &modelName, apiConfig)
	} else {
		driver, modelName, apiConfig, _, err := modelProviderSvc.GetTenantDefaultModelByType(tenantID, entity.ModelTypeChat)
		if err != nil {
			return query, fmt.Errorf("failed to get default chat model: %w", err)
		}
		chatModel = modelModule.NewChatModel(driver, &modelName, apiConfig)
	}
	if chatModel == nil {
		return query, fmt.Errorf("failed to get chat model: nil chat model")
	}

	return crossLanguagesWithChatModel(ctx, chatModel, query, languages)
}
