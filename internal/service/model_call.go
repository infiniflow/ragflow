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
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
)

// ModelCallService provides the model-call API described in Model方法.pdf.
// It accepts only the caller identity, model reference, request data, and
// runtime configuration. Provider, instance, API configuration, and driver
// resolution are handled internally.
//
// The legacy methods on ModelProviderService remain untouched so this new API
// can be introduced without changing existing callers.
type ModelCallService struct {
	providerService *ModelProviderService
	modelSolver     *ModelSolver
}

// NewModelCallService creates a model-call service with the standard model
// provider and model resolver.
func NewModelCallService() *ModelCallService {
	providerService := NewModelProviderService()
	return &ModelCallService{
		providerService: providerService,
		modelSolver:     &ModelSolver{service: providerService},
	}
}

// ChatToModelWithMessages sends messages to the model selected by modelRef.
func (s *ModelCallService) ChatToModelWithMessages(ctx context.Context, modelRef, userID string, messages []modelModule.Message, config *modelModule.ChatConfig, usage *common.ModelUsage) (*modelModule.ChatResponse, common.ErrorCode, error) {
	target, tenantID, code, err := s.resolveTarget(ctx, modelRef, userID, entity.ModelTypeChat)
	if err != nil {
		return nil, code, err
	}
	if config == nil {
		config = &modelModule.ChatConfig{}
	}
	if target.ModelInfo != nil {
		config.ModelClass = target.ModelInfo.Class
		if config.Thinking == nil && target.ModelInfo.Thinking != nil {
			thinking := target.ModelInfo.Thinking.DefaultValue
			config.Thinking = &thinking
		}
	}
	populateModelUsage(usage, target, tenantID, userID)

	modelName := target.ModelName
	response, err := target.Driver.ChatWithMessages(ctx, modelName, messages, target.APIConfig, config, usage)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty chat response")
	}
	return response, common.CodeSuccess, nil
}

// ChatToModelStreamWithSender streams the response from the model selected by
// modelRef through sender.
func (s *ModelCallService) ChatToModelStreamWithSender(ctx context.Context, modelRef, userID string, messages []modelModule.Message, config *modelModule.ChatConfig, usage *common.ModelUsage, sender func(*string, *string) error) (common.ErrorCode, error) {
	target, tenantID, code, err := s.resolveTarget(ctx, modelRef, userID, entity.ModelTypeChat)
	if err != nil {
		return code, err
	}
	if config == nil {
		config = &modelModule.ChatConfig{}
	}
	if target.ModelInfo != nil {
		config.ModelClass = target.ModelInfo.Class
		if config.Thinking == nil && target.ModelInfo.Thinking != nil {
			thinking := target.ModelInfo.Thinking.DefaultValue
			config.Thinking = &thinking
		}
	}
	populateModelUsage(usage, target, tenantID, userID)

	modelName := target.ModelName
	if err := target.Driver.ChatStreamlyWithSender(ctx, modelName, messages, target.APIConfig, config, usage, sender); err != nil {
		return common.CodeServerError, err
	}
	return common.CodeSuccess, nil
}

// EmbedText embeds texts with the model selected by modelRef.
func (s *ModelCallService) EmbedText(ctx context.Context, modelRef, userID string, texts []string, config *modelModule.EmbeddingConfig) ([]modelModule.EmbeddingData, common.ErrorCode, error) {
	target, _, code, err := s.resolveTarget(ctx, modelRef, userID, entity.ModelTypeEmbedding)
	if err != nil {
		return nil, code, err
	}
	if config == nil {
		config = &modelModule.EmbeddingConfig{}
	}
	if err := validateEmbeddingModel(target.ModelInfo, config.Dimension, len(texts)); err != nil {
		return nil, common.CodeBadRequest, err
	}

	modelName := target.ModelName
	embeddings, err := target.Driver.Embed(ctx, &modelName, modelModule.EmbedRequest{Texts: texts}, target.APIConfig, config, nil)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if len(embeddings) == 0 {
		return nil, common.CodeServerError, errors.New("empty embed response")
	}
	return embeddings, common.CodeSuccess, nil
}

// RerankDocument reranks documents with the model selected by modelRef.
func (s *ModelCallService) RerankDocument(ctx context.Context, modelRef, userID, query string, documents []string, config *modelModule.RerankConfig) (*modelModule.RerankResponse, common.ErrorCode, error) {
	target, _, code, err := s.resolveTarget(ctx, modelRef, userID, entity.ModelTypeRerank)
	if err != nil {
		return nil, code, err
	}
	if config == nil {
		config = &modelModule.RerankConfig{}
	}

	modelName := target.ModelName
	rerankModel := modelModule.NewRerankModel(target.Driver, &modelName, target.APIConfig, target.MaxTokens)
	response, err := rerankModel.Rerank(ctx, modelModule.RerankRequest{Query: query, Documents: documents}, target.APIConfig, config, nil)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty rerank response")
	}
	return response, common.CodeSuccess, nil
}

// TranscribeAudio converts audioFile to text with the model selected by
// modelRef.
func (s *ModelCallService) TranscribeAudio(ctx context.Context, modelRef, userID string, audioFile *string, config *modelModule.ASRConfig) (*modelModule.ASRResponse, common.ErrorCode, error) {
	target, _, code, err := s.resolveTarget(ctx, modelRef, userID, entity.ModelTypeSpeech2Text)
	if err != nil {
		return nil, code, err
	}
	if config == nil {
		config = &modelModule.ASRConfig{}
	}

	modelName := target.ModelName
	response, err := target.Driver.TranscribeAudio(ctx, &modelName, audioFile, target.APIConfig, config, nil)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty transcribe response")
	}
	return response, common.CodeSuccess, nil
}

// TranscribeAudioStream streams the transcription from the model selected by
// modelRef through sender.
func (s *ModelCallService) TranscribeAudioStream(ctx context.Context, modelRef, userID string, audioFile *string, config *modelModule.ASRConfig, sender func(*string, *string) error) (common.ErrorCode, error) {
	target, _, code, err := s.resolveTarget(ctx, modelRef, userID, entity.ModelTypeSpeech2Text)
	if err != nil {
		return code, err
	}
	if config == nil {
		config = &modelModule.ASRConfig{}
	}

	modelName := target.ModelName
	if err := target.Driver.TranscribeAudioWithSender(ctx, &modelName, audioFile, target.APIConfig, config, nil, sender); err != nil {
		return common.CodeServerError, err
	}
	return common.CodeSuccess, nil
}

// AudioSpeech converts audioContent to speech with the model selected by
// modelRef.
func (s *ModelCallService) AudioSpeech(ctx context.Context, modelRef, userID string, audioContent *string, config *modelModule.TTSConfig) (*modelModule.TTSResponse, common.ErrorCode, error) {
	target, _, code, err := s.resolveTarget(ctx, modelRef, userID, entity.ModelTypeTTS)
	if err != nil {
		return nil, code, err
	}
	if config == nil {
		config = &modelModule.TTSConfig{}
	}

	modelName := target.ModelName
	response, err := target.Driver.AudioSpeech(ctx, &modelName, audioContent, target.APIConfig, config, nil)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty audio speech response")
	}
	return response, common.CodeSuccess, nil
}

// AudioSpeechStream streams synthesized audio from the model selected by
// modelRef through sender.
func (s *ModelCallService) AudioSpeechStream(ctx context.Context, modelRef, userID string, audioContent *string, config *modelModule.TTSConfig, sender func(*string, *string) error) (common.ErrorCode, error) {
	target, _, code, err := s.resolveTarget(ctx, modelRef, userID, entity.ModelTypeTTS)
	if err != nil {
		return code, err
	}
	if config == nil {
		config = &modelModule.TTSConfig{}
	}

	modelName := target.ModelName
	if err := target.Driver.AudioSpeechWithSender(ctx, &modelName, audioContent, target.APIConfig, config, nil, sender); err != nil {
		return common.CodeServerError, err
	}
	return common.CodeSuccess, nil
}

// OCRFile extracts text from content or url with the model selected by
// modelRef.
func (s *ModelCallService) OCRFile(ctx context.Context, modelRef, userID string, content []byte, url *string, config *modelModule.OCRConfig) (*modelModule.OCRFileResponse, common.ErrorCode, error) {
	target, _, code, err := s.resolveTarget(ctx, modelRef, userID, entity.ModelTypeOCR)
	if err != nil {
		return nil, code, err
	}
	if config == nil {
		config = &modelModule.OCRConfig{}
	}

	modelName := target.ModelName
	response, err := target.Driver.OCRFile(ctx, &modelName, content, url, target.APIConfig, config, nil)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty OCR response")
	}
	return response, common.CodeSuccess, nil
}

// ParseFile parses content or url with the document parser selected by
// modelRef. doc_parse is a provider catalog capability rather than a member
// of entity.ModelType, so this method uses the existing model-info resolver.
func (s *ModelCallService) ParseFile(ctx context.Context, modelRef, userID string, content []byte, url *string, config *modelModule.ParseFileConfig) (*modelModule.ParseFileResponse, common.ErrorCode, error) {
	info, err := s.resolveModelInfo(ctx, modelRef, userID)
	if err != nil {
		return nil, common.CodeNotFound, err
	}
	if info.ModelInfo == nil || !info.ModelInfo.ModelTypeMap["doc_parse"] {
		return nil, common.CodeNotFound, fmt.Errorf("model %q is not a document parser", modelRef)
	}
	if config == nil {
		config = &modelModule.ParseFileConfig{}
	}

	modelDriver := info.ProviderInfo.ModelDriver
	if info.ModelEntity != nil {
		if info.ModelEntity.Status != "active" {
			return nil, common.CodeNotFound, errors.New("model is inactive")
		}
		region := ""
		baseURL := ""
		if info.APIConfig != nil {
			if info.APIConfig.Region != nil {
				region = *info.APIConfig.Region
			}
			if info.APIConfig.BaseURL != nil {
				baseURL = *info.APIConfig.BaseURL
			}
		}
		modelDriver, err = newModelDriverForBaseURL(info.ProviderInfo.ModelDriver, info.ProviderEntity.ProviderName, region, baseURL)
		if err != nil {
			return nil, common.CodeServerError, err
		}
	}

	modelName := info.ModelInfo.Name
	if info.ModelEntity != nil && info.ModelEntity.ModelName != "" {
		modelName = info.ModelEntity.ModelName
	}
	response, err := modelDriver.ParseFile(ctx, &modelName, content, url, info.APIConfig, config, nil)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty parse file response")
	}
	return response, common.CodeSuccess, nil
}

func (s *ModelCallService) resolveTarget(ctx context.Context, modelRef, userID string, modelType entity.ModelType) (*ModelTarget, string, common.ErrorCode, error) {
	if s == nil || s.providerService == nil || s.modelSolver == nil {
		return nil, "", common.CodeServerError, errors.New("model call service is not initialized")
	}
	tenantID, err := s.ownerTenantID(ctx, userID)
	if err != nil {
		return nil, "", common.CodeNotFound, err
	}
	target, err := s.modelSolver.ResolveModelConfig(ctx, tenantID, modelType, modelRef)
	if err != nil {
		return nil, "", common.CodeNotFound, err
	}
	return target, tenantID, common.CodeSuccess, nil
}

func (s *ModelCallService) ownerTenantID(ctx context.Context, userID string) (string, error) {
	if strings.TrimSpace(userID) == "" {
		return "", errors.New("user id is required")
	}
	if s.providerService.userTenantDAO == nil {
		return "", errors.New("user tenant service is not initialized")
	}
	tenants, err := s.providerService.userTenantDAO.GetByUserIDAndRole(ctx, dao.DB, userID, "owner")
	if err != nil {
		return "", err
	}
	if len(tenants) == 0 || tenants[0] == nil || strings.TrimSpace(tenants[0].TenantID) == "" {
		return "", fmt.Errorf("no owner tenant found for user %s", userID)
	}
	return tenants[0].TenantID, nil
}

func populateModelUsage(usage *common.ModelUsage, target *ModelTarget, tenantID, userID string) {
	if usage == nil {
		return
	}
	usage.UserID = userID
	usage.TenantID = tenantID
	usage.ProviderName = target.ProviderName
	usage.ModelName = target.ModelName
	if target.APIConfig != nil && target.APIConfig.ApiKey != nil {
		usage.APIKey = *target.APIConfig.ApiKey
	}
}

func (s *ModelCallService) resolveModelInfo(ctx context.Context, modelRef, userID string) (*ModelInstanceAndProviderInfo, error) {
	if s == nil || s.providerService == nil {
		return nil, errors.New("model call service is not initialized")
	}
	modelRef = strings.TrimSpace(modelRef)
	if modelRef == "" {
		return nil, errors.New("model ref is required")
	}

	model, err := s.providerService.modelDAO.GetByID(ctx, dao.DB, modelRef)
	if err == nil && model != nil {
		return s.providerService.getModelInstanceAndProviderByID(ctx, &modelRef, userID, nil)
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	modelName, instanceName, providerName, err := parseModelName(modelRef)
	if err != nil {
		return nil, err
	}
	return s.providerService.getModelInstanceAndProviderByName(ctx, &providerName, &instanceName, &modelName, userID, nil)
}
