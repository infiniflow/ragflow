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
	modelModule "ragflow/internal/entity/models"
)

// ModelCallService provides the model-call API described in Model方法.pdf.
// It accepts only the caller identity, model reference, request data, and
// runtime configuration. Provider, instance, API configuration, and driver
// resolution are handled internally.
type ModelCallService struct {
	providerService *ModelProviderService
	modelFactory    *ModelFactory
}

// NewModelCallService creates a model-call service with the standard model
// provider and model resolver.
func NewModelCallService() *ModelCallService {
	return NewModelCallServiceWithProviderService(NewModelProviderService(), NewModelFactory())
}

// NewModelCallServiceWithProviderService creates a model-call service using
// an existing provider service for provider metadata and balance operations.
func NewModelCallServiceWithProviderService(providerService *ModelProviderService, modelFactory *ModelFactory) *ModelCallService {
	if providerService == nil {
		providerService = NewModelProviderService()
	}
	if modelFactory == nil {
		modelFactory = NewModelFactory()
	}
	return &ModelCallService{
		providerService: providerService,
		modelFactory:    modelFactory,
	}
}

// ChatToModelWithMessages sends messages to the model selected by modelRef.
func (s *ModelCallService) ChatToModelWithMessages(ctx context.Context, modelRef, userID string, messages []modelModule.Message, config *modelModule.ChatConfig, usage *common.ModelUsage) (*modelModule.ChatResponse, common.ErrorCode, error) {
	access, tenantID, code, err := s.modelAccessForUser(ctx, userID)
	if err != nil {
		return nil, code, err
	}
	chatModel, err := s.modelFactory.NewChatModel(ctx, access, modelRef)
	if err != nil {
		return nil, common.CodeNotFound, err
	}
	if config == nil {
		config = &modelModule.ChatConfig{}
	}
	info := chatModel.Info()
	populateModelUsage(usage, info, chatModel.APIConfig, tenantID, userID)

	response, err := chatModel.ChatWithMessages(ctx, messages, config, usage)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty chat response")
	}
	return response, common.CodeSuccess, nil
}
func (s *ModelCallService) ChatToModelStreamWithSender(ctx context.Context, modelRef, userID string, messages []modelModule.Message, config *modelModule.ChatConfig, usage *common.ModelUsage, sender func(*string, *string) error) (common.ErrorCode, error) {
	access, tenantID, code, err := s.modelAccessForUser(ctx, userID)
	if err != nil {
		return code, err
	}
	chatModel, err := s.modelFactory.NewChatModel(ctx, access, modelRef)
	if err != nil {
		return common.CodeNotFound, err
	}
	if config == nil {
		config = &modelModule.ChatConfig{}
	}
	populateModelUsage(usage, chatModel.Info(), chatModel.APIConfig, tenantID, userID)

	if err := chatModel.ChatStreamlyWithSender(ctx, messages, config, usage, sender); err != nil {
		return common.CodeServerError, err
	}
	return common.CodeSuccess, nil
}
func (s *ModelCallService) EmbedText(ctx context.Context, modelRef, userID string, texts []string, config *modelModule.EmbeddingConfig) ([]modelModule.EmbeddingData, common.ErrorCode, error) {
	access, _, code, err := s.modelAccessForUser(ctx, userID)
	if err != nil {
		return nil, code, err
	}
	embeddingModel, err := s.modelFactory.NewEmbeddingModel(ctx, access, modelRef)
	if err != nil {
		return nil, common.CodeNotFound, err
	}
	if config == nil {
		config = &modelModule.EmbeddingConfig{}
	}
	info := embeddingModel.Info()
	if info == nil || info.Catalog == nil {
		return nil, common.CodeBadRequest, errors.New("embedding model metadata is unavailable")
	}
	if err := validateEmbeddingModel(info.Catalog, config.Dimension, len(texts)); err != nil {
		return nil, common.CodeBadRequest, err
	}

	embeddings, err := embeddingModel.Embed(ctx, modelModule.EmbedRequest{Texts: texts}, config, nil)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if len(embeddings) == 0 {
		return nil, common.CodeServerError, errors.New("empty embed response")
	}
	return embeddings, common.CodeSuccess, nil
}
func (s *ModelCallService) RerankDocument(ctx context.Context, modelRef, userID, query string, documents []string, config *modelModule.RerankConfig) (*modelModule.RerankResponse, common.ErrorCode, error) {
	access, _, code, err := s.modelAccessForUser(ctx, userID)
	if err != nil {
		return nil, code, err
	}
	rerankModel, err := s.modelFactory.NewRerankModel(ctx, access, modelRef)
	if err != nil {
		return nil, common.CodeNotFound, err
	}
	if config == nil {
		config = &modelModule.RerankConfig{}
	}

	response, err := rerankModel.Rerank(ctx, modelModule.RerankRequest{Query: query, Documents: documents}, config, nil)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty rerank response")
	}
	return response, common.CodeSuccess, nil
}
func (s *ModelCallService) TranscribeAudio(ctx context.Context, modelRef, userID string, audioFile *string, config *modelModule.ASRConfig) (*modelModule.ASRResponse, common.ErrorCode, error) {
	access, _, code, err := s.modelAccessForUser(ctx, userID)
	if err != nil {
		return nil, code, err
	}
	asrModel, err := s.modelFactory.NewASRModel(ctx, access, modelRef)
	if err != nil {
		return nil, common.CodeNotFound, err
	}
	if config == nil {
		config = &modelModule.ASRConfig{}
	}

	response, err := asrModel.Transcribe(ctx, audioFile, config, nil)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty transcribe response")
	}
	return response, common.CodeSuccess, nil
}
func (s *ModelCallService) TranscribeAudioStream(ctx context.Context, modelRef, userID string, audioFile *string, config *modelModule.ASRConfig, sender func(*string, *string) error) (common.ErrorCode, error) {
	access, _, code, err := s.modelAccessForUser(ctx, userID)
	if err != nil {
		return code, err
	}
	asrModel, err := s.modelFactory.NewASRModel(ctx, access, modelRef)
	if err != nil {
		return common.CodeNotFound, err
	}
	if config == nil {
		config = &modelModule.ASRConfig{}
	}
	if err := asrModel.TranscribeWithSender(ctx, audioFile, config, nil, sender); err != nil {
		return common.CodeServerError, err
	}
	return common.CodeSuccess, nil
}
func (s *ModelCallService) AudioSpeech(ctx context.Context, modelRef, userID string, audioContent *string, config *modelModule.TTSConfig) (*modelModule.TTSResponse, common.ErrorCode, error) {
	if s == nil || s.modelFactory == nil {
		return nil, common.CodeServerError, errors.New("model call service is not initialized")
	}
	tenantID, tenantErr := s.ownerTenantID(ctx, userID)
	access := ModelAccess{UserID: userID, TenantID: tenantID}
	if tenantErr != nil {
		if strings.TrimSpace(modelRef) != "" {
			return nil, common.CodeNotFound, tenantErr
		}
		// Some internal audio dispatchers pass a tenant ID in userID.
		access = ModelAccess{TenantID: userID}
		tenantID = userID
	}
	var (
		ttsModel *modelModule.TTSModel
		err      error
	)
	if strings.TrimSpace(modelRef) == "" {
		ttsModel, err = s.modelFactory.NewDefaultTTSModel(ctx, access)
	} else {
		ttsModel, err = s.modelFactory.NewTTSModel(ctx, access, modelRef)
	}
	if err != nil {
		return nil, common.CodeNotFound, err
	}
	if config == nil {
		config = &modelModule.TTSConfig{}
	}

	response, err := ttsModel.Speech(ctx, audioContent, config, nil)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty audio speech response")
	}
	return response, common.CodeSuccess, nil
}
func (s *ModelCallService) AudioSpeechForTenant(ctx context.Context, modelRef, tenantID string, audioContent *string, config *modelModule.TTSConfig) (*modelModule.TTSResponse, common.ErrorCode, error) {
	if s == nil || s.modelFactory == nil {
		return nil, common.CodeServerError, errors.New("model call service is not initialized")
	}
	access := ModelAccess{TenantID: tenantID}
	var (
		ttsModel *modelModule.TTSModel
		err      error
	)
	if strings.TrimSpace(modelRef) == "" {
		ttsModel, err = s.modelFactory.NewDefaultTTSModel(ctx, access)
	} else {
		ttsModel, err = s.modelFactory.NewTTSModel(ctx, access, modelRef)
	}
	if err != nil {
		return nil, common.CodeNotFound, err
	}
	if config == nil {
		config = &modelModule.TTSConfig{}
	}

	response, err := ttsModel.Speech(ctx, audioContent, config, nil)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty audio speech response")
	}
	return response, common.CodeSuccess, nil
}
func (s *ModelCallService) AudioSpeechStream(ctx context.Context, modelRef, userID string, audioContent *string, config *modelModule.TTSConfig, sender func(*string, *string) error) (common.ErrorCode, error) {
	access, _, code, err := s.modelAccessForUser(ctx, userID)
	if err != nil {
		return code, err
	}
	ttsModel, err := s.modelFactory.NewTTSModel(ctx, access, modelRef)
	if err != nil {
		return common.CodeNotFound, err
	}
	if config == nil {
		config = &modelModule.TTSConfig{}
	}
	if err := ttsModel.SpeechWithSender(ctx, audioContent, config, nil, sender); err != nil {
		return common.CodeServerError, err
	}
	return common.CodeSuccess, nil
}
func (s *ModelCallService) OCRFile(ctx context.Context, modelRef, userID string, content []byte, url *string, config *modelModule.OCRConfig) (*modelModule.OCRFileResponse, common.ErrorCode, error) {
	access, _, code, err := s.modelAccessForUser(ctx, userID)
	if err != nil {
		return nil, code, err
	}
	ocrModel, err := s.modelFactory.NewOCRModel(ctx, access, modelRef)
	if err != nil {
		return nil, common.CodeNotFound, err
	}
	if config == nil {
		config = &modelModule.OCRConfig{}
	}

	response, err := ocrModel.OCRFile(ctx, content, url, config, nil)
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

func (s *ModelCallService) modelAccessForUser(ctx context.Context, userID string) (ModelAccess, string, common.ErrorCode, error) {
	if s == nil || s.providerService == nil || s.modelFactory == nil {
		return ModelAccess{}, "", common.CodeServerError, errors.New("model call service is not initialized")
	}
	tenantID, err := s.ownerTenantID(ctx, userID)
	if err != nil {
		return ModelAccess{}, "", common.CodeNotFound, err
	}
	return ModelAccess{UserID: userID, TenantID: tenantID}, tenantID, common.CodeSuccess, nil
}

func populateModelUsage(usage *common.ModelUsage, info *modelModule.ModelInfo, apiConfig *modelModule.APIConfig, tenantID, userID string) {
	if usage == nil || info == nil {
		return
	}
	usage.UserID = userID
	usage.TenantID = tenantID
	usage.ProviderName = info.ProviderName
	usage.InstanceID = info.InstanceID
	usage.ModelName = info.Name
	if apiConfig != nil && apiConfig.ApiKey != nil {
		usage.APIKey = *apiConfig.ApiKey
	}
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
