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

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/common"
	modelModule "ragflow/internal/entity/models"
)

// ChatLLMProvider is the provider interface for non-streaming chat calls.
type ChatLLMProvider interface {
	Chat(tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (*modelModule.ChatResponse, error)
}

// ChatLLM is the service wrapper for non-streaming chat calls.
type ChatLLM struct {
	provider ChatLLMProvider
}

func NewChatLLM(provider ChatLLMProvider) *ChatLLM {
	return &ChatLLM{provider: provider}
}

func (s *ChatLLM) Chat(tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (*modelModule.ChatResponse, error) {
	if s == nil || s.provider == nil {
		return nil, fmt.Errorf("chat LLM not configured")
	}
	return s.provider.Chat(tenantID, modelID, messages, config)
}

// TenantStreamingLLMProvider streams chat deltas after resolving tenant and model IDs.
type TenantStreamingLLMProvider interface {
	ChatStream(ctx context.Context, tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (<-chan string, error)
}

// TenantStreamingLLM is the service wrapper for tenant/model-aware streaming chat.
type TenantStreamingLLM struct {
	provider TenantStreamingLLMProvider
}

func NewTenantStreamingLLM(provider TenantStreamingLLMProvider) *TenantStreamingLLM {
	return &TenantStreamingLLM{provider: provider}
}

func (s *TenantStreamingLLM) ChatStream(ctx context.Context, tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (<-chan string, error) {
	if s == nil || s.provider == nil {
		return nil, fmt.Errorf("streaming LLM not configured")
	}
	return s.provider.ChatStream(ctx, tenantID, modelID, messages, config)
}

// ChunkRetrieverProvider abstracts chunk retrieval for retrieval_test behavior.
type ChunkRetrieverProvider interface {
	RetrievalTest(req *RetrievalTestRequest, userID string) (*RetrievalTestResponse, error)
}

// ChunkRetriever is the service wrapper for chunk retrieval.
type ChunkRetriever struct {
	provider ChunkRetrieverProvider
}

func NewChunkRetriever(provider ChunkRetrieverProvider) *ChunkRetriever {
	return &ChunkRetriever{provider: provider}
}

func (s *ChunkRetriever) RetrievalTest(req *RetrievalTestRequest, userID string) (*RetrievalTestResponse, error) {
	if s == nil || s.provider == nil {
		return nil, fmt.Errorf("chunk retriever not configured")
	}
	return s.provider.RetrievalTest(req, userID)
}

// SSEWriterProvider writes an SSE event to the client.
type SSEWriterProvider interface {
	Write(c *gin.Context, data string)
}

// SSEWriter is the service wrapper for SSE writes.
type SSEWriter struct {
	provider SSEWriterProvider
}

func NewSSEWriter(provider SSEWriterProvider) *SSEWriter {
	return &SSEWriter{provider: provider}
}

func (s *SSEWriter) Write(c *gin.Context, data string) {
	if s == nil || s.provider == nil {
		return
	}
	s.provider.Write(c, data)
}

// ModelProviderLLM wraps ModelProviderService to implement the LLM provider interfaces.
type ModelProviderLLM struct {
	Svc *ModelProviderService
}

func (r *ModelProviderLLM) Chat(tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (*modelModule.ChatResponse, error) {
	chatModel, err := r.Svc.GetChatModel(tenantID, modelID)
	if err != nil {
		return nil, err
	}
	return chatModel.ModelDriver.ChatWithMessages(*chatModel.ModelName, messages, chatModel.APIConfig, config)
}

// ChatStream implements TenantStreamingLLM.
func (r *ModelProviderLLM) ChatStream(ctx context.Context, tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (<-chan string, error) {
	chatModel, err := r.Svc.GetChatModel(tenantID, modelID)
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

// TenantStreamAdapter adapts TenantStreamingLLM to AskService's StreamingLLM.
type TenantStreamAdapter struct {
	LLM      *TenantStreamingLLM
	TenantID string
	ModelID  string
}

func (a *TenantStreamAdapter) ChatStream(ctx context.Context, messages []modelModule.Message, config *modelModule.ChatConfig) (<-chan string, error) {
	if a.LLM == nil {
		return nil, fmt.Errorf("streaming LLM not configured")
	}
	return a.LLM.ChatStream(ctx, a.TenantID, a.ModelID, messages, config)
}
