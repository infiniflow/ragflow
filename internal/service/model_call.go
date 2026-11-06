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

	"ragflow/internal/common"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/permission"
)

// ModelCallService provides the model-call API described in Model方法.pdf.
// It accepts only the caller identity, model reference, request data, and
// runtime configuration. Provider, instance, API configuration, and driver
// resolution are handled internally.
type ModelCallService struct {
	modelFactory *ModelFactory
}

// NewModelCallService creates a model-call service with the standard resolver.
func NewModelCallService() *ModelCallService {
	return &ModelCallService{modelFactory: NewModelFactory()}
}

// ChatToModelWithMessages sends messages to the model selected by modelRef.
func (s *ModelCallService) ChatToModelWithMessages(ctx context.Context, modelRef string, messages []modelModule.Message, config *modelModule.ChatConfig, usage *common.ModelUsage) (*modelModule.ChatResponse, common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return nil, code, err
	}
	chatModel, err := s.modelFactory.NewChatModel(permission.WithTenantID(ctx, tenantID), modelRef)
	if err != nil {
		return nil, common.CodeNotFound, err
	}
	if config == nil {
		config = &modelModule.ChatConfig{}
	}
	info := chatModel.Info()
	populateModelUsage(ctx, usage, info, chatModel.APIConfig, tenantID)

	response, err := chatModel.ChatWithMessages(ctx, messages, config, usage)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty chat response")
	}
	return response, common.CodeSuccess, nil
}
func (s *ModelCallService) ChatToModelStreamWithSender(ctx context.Context, modelRef string, messages []modelModule.Message, config *modelModule.ChatConfig, usage *common.ModelUsage, sender func(*string, *string) error) (common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return code, err
	}
	chatModel, err := s.modelFactory.NewChatModel(permission.WithTenantID(ctx, tenantID), modelRef)
	if err != nil {
		return common.CodeNotFound, err
	}
	if config == nil {
		config = &modelModule.ChatConfig{}
	}
	populateModelUsage(ctx, usage, chatModel.Info(), chatModel.APIConfig, tenantID)

	if err := chatModel.ChatStreamlyWithSender(ctx, messages, config, usage, sender); err != nil {
		return common.CodeServerError, err
	}
	return common.CodeSuccess, nil
}
func (s *ModelCallService) EmbedText(ctx context.Context, modelRef string, texts []string, config *modelModule.EmbeddingConfig) ([]modelModule.EmbeddingData, common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return nil, code, err
	}
	embeddingModel, err := s.modelFactory.NewEmbeddingModel(permission.WithTenantID(ctx, tenantID), modelRef)
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
func (s *ModelCallService) RerankDocument(ctx context.Context, modelRef, query string, documents []string, config *modelModule.RerankConfig) (*modelModule.RerankResponse, common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return nil, code, err
	}
	rerankModel, err := s.modelFactory.NewRerankModel(permission.WithTenantID(ctx, tenantID), modelRef)
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
func (s *ModelCallService) TranscribeAudio(ctx context.Context, modelRef string, audioFile *string, config *modelModule.ASRConfig) (*modelModule.ASRResponse, common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return nil, code, err
	}
	asrModel, err := s.modelFactory.NewASRModel(permission.WithTenantID(ctx, tenantID), modelRef)
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
func (s *ModelCallService) TranscribeAudioStream(ctx context.Context, modelRef string, audioFile *string, config *modelModule.ASRConfig, sender func(*string, *string) error) (common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return code, err
	}
	asrModel, err := s.modelFactory.NewASRModel(permission.WithTenantID(ctx, tenantID), modelRef)
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
func (s *ModelCallService) AudioSpeech(ctx context.Context, modelRef string, audioContent *string, config *modelModule.TTSConfig) (*modelModule.TTSResponse, common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return nil, code, err
	}
	var (
		ttsModel *modelModule.TTSModel
	)
	if strings.TrimSpace(modelRef) == "" {
		ttsModel, err = s.modelFactory.NewDefaultTTSModel(permission.WithTenantID(ctx, tenantID))
	} else {
		ttsModel, err = s.modelFactory.NewTTSModel(permission.WithTenantID(ctx, tenantID), modelRef)
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
	modelCtx := permission.WithTenantID(ctx, tenantID)
	var (
		ttsModel *modelModule.TTSModel
		err      error
	)
	if strings.TrimSpace(modelRef) == "" {
		ttsModel, err = s.modelFactory.NewDefaultTTSModel(modelCtx)
	} else {
		ttsModel, err = s.modelFactory.NewTTSModel(modelCtx, modelRef)
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
func (s *ModelCallService) AudioSpeechStream(ctx context.Context, modelRef string, audioContent *string, config *modelModule.TTSConfig, sender func(*string, *string) error) (common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return code, err
	}
	ttsModel, err := s.modelFactory.NewTTSModel(permission.WithTenantID(ctx, tenantID), modelRef)
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
func (s *ModelCallService) OCRFile(ctx context.Context, modelRef string, content []byte, url *string, config *modelModule.OCRConfig) (*modelModule.OCRFileResponse, common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return nil, code, err
	}
	ocrModel, err := s.modelFactory.NewOCRModel(permission.WithTenantID(ctx, tenantID), modelRef)
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
// modelRef. A zero model type resolves catalog-only capabilities such as doc_parse.
func (s *ModelCallService) ParseFile(ctx context.Context, modelRef string, content []byte, url *string, config *modelModule.ParseFileConfig) (*modelModule.ParseFileResponse, common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return nil, code, err
	}
	target, err := s.modelFactory.resolveConfig(permission.WithTenantID(ctx, tenantID), 0, modelRef)
	if err != nil {
		return nil, common.CodeNotFound, err
	}
	if target.ModelInfo == nil || !target.ModelInfo.ModelTypeMap["doc_parse"] {
		return nil, common.CodeNotFound, fmt.Errorf("model %q is not a document parser", modelRef)
	}
	if config == nil {
		config = &modelModule.ParseFileConfig{}
	}

	response, err := target.Driver.ParseFile(ctx, &target.ModelName, content, url, target.APIConfig, config, nil)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if response == nil {
		return nil, common.CodeServerError, errors.New("empty parse file response")
	}
	return response, common.CodeSuccess, nil
}

func (s *ModelCallService) tenantIDForRequest(ctx context.Context) (string, common.ErrorCode, error) {
	if s == nil || s.modelFactory == nil {
		return "", common.CodeServerError, errors.New("model call service is not initialized")
	}
	tenantID, err := requireCurrentTenantID(ctx)
	if err != nil {
		return "", common.CodeNotFound, err
	}
	return tenantID, common.CodeSuccess, nil
}

func populateModelUsage(ctx context.Context, usage *common.ModelUsage, info *modelModule.ModelInfo, apiConfig *modelModule.APIConfig, tenantID string) {
	if usage == nil || info == nil {
		return
	}
	usage.UserID = permission.UserIDFromContext(ctx)
	usage.TenantID = tenantID
	usage.ProviderName = info.ProviderName
	usage.InstanceID = info.InstanceID
	usage.ModelName = info.Name
	if apiConfig != nil && apiConfig.ApiKey != nil {
		usage.APIKey = *apiConfig.ApiKey
	}
}
