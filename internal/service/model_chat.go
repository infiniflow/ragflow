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

	"go.uber.org/zap"

	"ragflow/internal/common"
	modelModule "ragflow/internal/entity/models"
)

func (m *ModelProviderService) Chat(tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (*modelModule.ChatResponse, error) {
	chatModel, err := m.GetChatModel(tenantID, modelID)
	if err != nil {
		return nil, err
	}
	return chatModel.ModelDriver.ChatWithMessages(*chatModel.ModelName, messages, chatModel.APIConfig, config)
}

func (m *ModelProviderService) ChatStream(ctx context.Context, tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (<-chan string, error) {
	chatModel, err := m.GetChatModel(tenantID, modelID)
	if err != nil {
		return nil, err
	}
	return chatStreamWithContext(ctx, chatModel, messages, config), nil
}

func chatStreamWithContext(ctx context.Context, chatModel *modelModule.ChatModel, messages []modelModule.Message, config *modelModule.ChatConfig) <-chan string {
	ch := make(chan string, 256)
	go func() {
		defer close(ch)
		if err := chatModel.ModelDriver.ChatStreamlyWithSender(*chatModel.ModelName, messages, chatModel.APIConfig, config,
			func(delta *string, _ *string) error {
				if delta == nil {
					return nil
				}
				select {
				case ch <- *delta:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}); err != nil {
			if err == context.Canceled || err == context.DeadlineExceeded {
				return
			}
			common.Warn("ChatStreamlyWithSender returned error", zap.Error(err))
		}
	}()
	return ch
}

// TenantStreamAdapter adapts tenant/model-aware chat streaming to AskService.
type TenantStreamAdapter struct {
	LLM      *ModelProviderService
	TenantID string
	ModelID  string
}

func (a *TenantStreamAdapter) ChatStream(ctx context.Context, messages []modelModule.Message, config *modelModule.ChatConfig) (<-chan string, error) {
	if a.LLM == nil {
		return nil, fmt.Errorf("streaming LLM not configured")
	}
	return a.LLM.ChatStream(ctx, a.TenantID, a.ModelID, messages, config)
}
