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
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/permission"
)

// ModelCallService provides the model-call API described in Model方法.pdf.
// It accepts only the caller identity, model reference, request data, and
// runtime configuration. Provider, instance, API configuration, and driver
// resolution are handled internally.
//
// Every invocation goes through dao.InvokeWithGroupFailover: a gp- group
// reference whose member fails the call flips inactive and the request retries
// once on the next member. Streaming methods only retry when the sender has
// not delivered anything yet; once data reached the client the error surfaces
// as-is.
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
	if config == nil {
		config = &modelModule.ChatConfig{}
	}
	var (
		response *modelModule.ChatResponse
		callCode = common.CodeNotFound
	)
	err = dao.InvokeWithGroupFailover(ctx, dao.DB, tenantID, entity.ModelTypeChat, modelRef,
		func(ctx context.Context) (*modelModule.ChatModel, error) {
			m, rerr := s.modelFactory.NewChatModel(permission.WithTenantID(ctx, tenantID), modelRef)
			return m, rerr
		}, nil,
		func(ctx context.Context, chatModel *modelModule.ChatModel) error {
			populateModelUsage(ctx, usage, chatModel.Info(), chatModel.APIConfig, tenantID)
			resp, invErr := chatModel.ChatWithMessages(ctx, messages, config, usage)
			callCode = common.CodeServerError
			if invErr != nil {
				return invErr
			}
			if resp == nil {
				return errors.New("empty chat response")
			}
			response = resp
			callCode = common.CodeSuccess
			return nil
		}, nil)
	if err != nil {
		return nil, callCode, err
	}
	return response, common.CodeSuccess, nil
}

func (s *ModelCallService) ChatToModelStreamWithSender(ctx context.Context, modelRef string, messages []modelModule.Message, config *modelModule.ChatConfig, usage *common.ModelUsage, sender func(*string, *string) error) (common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return code, err
	}
	if config == nil {
		config = &modelModule.ChatConfig{}
	}
	delivered := false
	wrappedSender := func(status, msg *string) error {
		delivered = true
		return sender(status, msg)
	}
	callCode := common.CodeNotFound
	err = dao.InvokeWithGroupFailover(ctx, dao.DB, tenantID, entity.ModelTypeChat, modelRef,
		func(ctx context.Context) (*modelModule.ChatModel, error) {
			return s.modelFactory.NewChatModel(permission.WithTenantID(ctx, tenantID), modelRef)
		}, nil,
		func(ctx context.Context, chatModel *modelModule.ChatModel) error {
			populateModelUsage(ctx, usage, chatModel.Info(), chatModel.APIConfig, tenantID)
			callCode = common.CodeServerError
			if invErr := chatModel.ChatStreamlyWithSender(ctx, messages, config, usage, wrappedSender); invErr != nil {
				return invErr
			}
			callCode = common.CodeSuccess
			return nil
		},
		func(error) bool { return !delivered })
	if err != nil {
		return callCode, err
	}
	return common.CodeSuccess, nil
}

func (s *ModelCallService) EmbedText(ctx context.Context, modelRef string, texts []string, config *modelModule.EmbeddingConfig) ([]modelModule.EmbeddingData, common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return nil, code, err
	}
	if config == nil {
		config = &modelModule.EmbeddingConfig{}
	}
	var (
		embeddings []modelModule.EmbeddingData
		callCode   = common.CodeNotFound
		invalid    bool
	)
	err = dao.InvokeWithGroupFailover(ctx, dao.DB, tenantID, entity.ModelTypeEmbedding, modelRef,
		func(ctx context.Context) (*modelModule.EmbeddingModel, error) {
			return s.modelFactory.NewEmbeddingModel(permission.WithTenantID(ctx, tenantID), modelRef)
		}, nil,
		func(ctx context.Context, embeddingModel *modelModule.EmbeddingModel) error {
			info := embeddingModel.Info()
			if info == nil || info.Catalog == nil {
				invalid = true
				callCode = common.CodeBadRequest
				return errors.New("embedding model metadata is unavailable")
			}
			if verr := validateEmbeddingModel(info.Catalog, config.Dimension, len(texts)); verr != nil {
				invalid = true
				callCode = common.CodeBadRequest
				return verr
			}
			callCode = common.CodeServerError
			embeds, invErr := embeddingModel.Embed(ctx, modelModule.EmbedRequest{Texts: texts}, config, nil)
			if invErr != nil {
				return invErr
			}
			if len(embeds) == 0 {
				return errors.New("empty embed response")
			}
			embeddings = embeds
			callCode = common.CodeSuccess
			return nil
		},
		func(error) bool { return !invalid })
	if err != nil {
		return nil, callCode, err
	}
	return embeddings, common.CodeSuccess, nil
}

func (s *ModelCallService) RerankDocument(ctx context.Context, modelRef, query string, documents []string, config *modelModule.RerankConfig) (*modelModule.RerankResponse, common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return nil, code, err
	}
	if config == nil {
		config = &modelModule.RerankConfig{}
	}
	var (
		response *modelModule.RerankResponse
		callCode = common.CodeNotFound
	)
	err = dao.InvokeWithGroupFailover(ctx, dao.DB, tenantID, entity.ModelTypeRerank, modelRef,
		func(ctx context.Context) (*modelModule.RerankModel, error) {
			return s.modelFactory.NewRerankModel(permission.WithTenantID(ctx, tenantID), modelRef)
		}, nil,
		func(ctx context.Context, rerankModel *modelModule.RerankModel) error {
			callCode = common.CodeServerError
			resp, invErr := rerankModel.Rerank(ctx, modelModule.RerankRequest{Query: query, Documents: documents}, config, nil)
			if invErr != nil {
				return invErr
			}
			if resp == nil {
				return errors.New("empty rerank response")
			}
			response = resp
			callCode = common.CodeSuccess
			return nil
		}, nil)
	if err != nil {
		return nil, callCode, err
	}
	return response, common.CodeSuccess, nil
}

func (s *ModelCallService) TranscribeAudio(ctx context.Context, modelRef string, audioFile *string, config *modelModule.ASRConfig) (*modelModule.ASRResponse, common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return nil, code, err
	}
	if config == nil {
		config = &modelModule.ASRConfig{}
	}
	var (
		response *modelModule.ASRResponse
		callCode = common.CodeNotFound
	)
	err = dao.InvokeWithGroupFailover(ctx, dao.DB, tenantID, entity.ModelTypeSpeech2Text, modelRef,
		func(ctx context.Context) (*modelModule.ASRModel, error) {
			return s.modelFactory.NewASRModel(permission.WithTenantID(ctx, tenantID), modelRef)
		}, nil,
		func(ctx context.Context, asrModel *modelModule.ASRModel) error {
			callCode = common.CodeServerError
			resp, invErr := asrModel.Transcribe(ctx, audioFile, config, nil)
			if invErr != nil {
				return invErr
			}
			if resp == nil {
				return errors.New("empty transcribe response")
			}
			response = resp
			callCode = common.CodeSuccess
			return nil
		}, nil)
	if err != nil {
		return nil, callCode, err
	}
	return response, common.CodeSuccess, nil
}

func (s *ModelCallService) TranscribeAudioStream(ctx context.Context, modelRef string, audioFile *string, config *modelModule.ASRConfig, sender func(*string, *string) error) (common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return code, err
	}
	if config == nil {
		config = &modelModule.ASRConfig{}
	}
	delivered := false
	wrappedSender := func(status, msg *string) error {
		delivered = true
		return sender(status, msg)
	}
	callCode := common.CodeNotFound
	err = dao.InvokeWithGroupFailover(ctx, dao.DB, tenantID, entity.ModelTypeSpeech2Text, modelRef,
		func(ctx context.Context) (*modelModule.ASRModel, error) {
			return s.modelFactory.NewASRModel(permission.WithTenantID(ctx, tenantID), modelRef)
		}, nil,
		func(ctx context.Context, asrModel *modelModule.ASRModel) error {
			callCode = common.CodeServerError
			if invErr := asrModel.TranscribeWithSender(ctx, audioFile, config, nil, wrappedSender); invErr != nil {
				return invErr
			}
			callCode = common.CodeSuccess
			return nil
		},
		func(error) bool { return !delivered })
	if err != nil {
		return callCode, err
	}
	return common.CodeSuccess, nil
}

func (s *ModelCallService) AudioSpeech(ctx context.Context, modelRef string, audioContent *string, config *modelModule.TTSConfig) (*modelModule.TTSResponse, common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return nil, code, err
	}
	if config == nil {
		config = &modelModule.TTSConfig{}
	}
	var (
		response *modelModule.TTSResponse
		callCode = common.CodeNotFound
	)
	err = dao.InvokeWithGroupFailover(ctx, dao.DB, tenantID, entity.ModelTypeTTS, modelRef,
		func(ctx context.Context) (*modelModule.TTSModel, error) {
			return s.resolveTTSModel(ctx, tenantID, modelRef)
		}, nil,
		func(ctx context.Context, ttsModel *modelModule.TTSModel) error {
			callCode = common.CodeServerError
			resp, invErr := ttsModel.Speech(ctx, audioContent, config, nil)
			if invErr != nil {
				return invErr
			}
			if resp == nil {
				return errors.New("empty audio speech response")
			}
			response = resp
			callCode = common.CodeSuccess
			return nil
		}, nil)
	if err != nil {
		return nil, callCode, err
	}
	return response, common.CodeSuccess, nil
}

func (s *ModelCallService) AudioSpeechForTenant(ctx context.Context, modelRef, tenantID string, audioContent *string, config *modelModule.TTSConfig) (*modelModule.TTSResponse, common.ErrorCode, error) {
	if s == nil || s.modelFactory == nil {
		return nil, common.CodeServerError, errors.New("model call service is not initialized")
	}
	if config == nil {
		config = &modelModule.TTSConfig{}
	}
	var (
		response *modelModule.TTSResponse
		callCode = common.CodeNotFound
	)
	err := dao.InvokeWithGroupFailover(ctx, dao.DB, tenantID, entity.ModelTypeTTS, modelRef,
		func(ctx context.Context) (*modelModule.TTSModel, error) {
			return s.resolveTTSModel(ctx, tenantID, modelRef)
		}, nil,
		func(ctx context.Context, ttsModel *modelModule.TTSModel) error {
			callCode = common.CodeServerError
			resp, invErr := ttsModel.Speech(ctx, audioContent, config, nil)
			if invErr != nil {
				return invErr
			}
			if resp == nil {
				return errors.New("empty audio speech response")
			}
			response = resp
			callCode = common.CodeSuccess
			return nil
		}, nil)
	if err != nil {
		return nil, callCode, err
	}
	return response, common.CodeSuccess, nil
}

func (s *ModelCallService) AudioSpeechStream(ctx context.Context, modelRef string, audioContent *string, config *modelModule.TTSConfig, sender func(*string, *string) error) (common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return code, err
	}
	if config == nil {
		config = &modelModule.TTSConfig{}
	}
	delivered := false
	wrappedSender := func(status, msg *string) error {
		delivered = true
		return sender(status, msg)
	}
	callCode := common.CodeNotFound
	err = dao.InvokeWithGroupFailover(ctx, dao.DB, tenantID, entity.ModelTypeTTS, modelRef,
		func(ctx context.Context) (*modelModule.TTSModel, error) {
			return s.resolveTTSModel(ctx, tenantID, modelRef)
		}, nil,
		func(ctx context.Context, ttsModel *modelModule.TTSModel) error {
			callCode = common.CodeServerError
			if invErr := ttsModel.SpeechWithSender(ctx, audioContent, config, nil, wrappedSender); invErr != nil {
				return invErr
			}
			callCode = common.CodeSuccess
			return nil
		},
		func(error) bool { return !delivered })
	if err != nil {
		return callCode, err
	}
	return common.CodeSuccess, nil
}

func (s *ModelCallService) OCRFile(ctx context.Context, modelRef string, content []byte, url *string, config *modelModule.OCRConfig) (*modelModule.OCRFileResponse, common.ErrorCode, error) {
	tenantID, code, err := s.tenantIDForRequest(ctx)
	if err != nil {
		return nil, code, err
	}
	if config == nil {
		config = &modelModule.OCRConfig{}
	}
	var (
		response *modelModule.OCRFileResponse
		callCode = common.CodeNotFound
	)
	err = dao.InvokeWithGroupFailover(ctx, dao.DB, tenantID, entity.ModelTypeOCR, modelRef,
		func(ctx context.Context) (*modelModule.OCRModel, error) {
			return s.modelFactory.NewOCRModel(permission.WithTenantID(ctx, tenantID), modelRef)
		}, nil,
		func(ctx context.Context, ocrModel *modelModule.OCRModel) error {
			callCode = common.CodeServerError
			resp, invErr := ocrModel.OCRFile(ctx, content, url, config, nil)
			if invErr != nil {
				return invErr
			}
			if resp == nil {
				return errors.New("empty OCR response")
			}
			response = resp
			callCode = common.CodeSuccess
			return nil
		}, nil)
	if err != nil {
		return nil, callCode, err
	}
	return response, common.CodeSuccess, nil
}

// ParseFile parses content or url with the document parser selected by
// modelRef. A zero model type resolves catalog-only capabilities such as
// doc_parse. Group failover does not apply: doc_parse capability is catalog
// metadata rather than one of the pickable model types, so there is no member
// to flip.
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

// resolveTTSModel resolves the requested TTS model, falling back to the
// tenant default when modelRef is empty.
func (s *ModelCallService) resolveTTSModel(ctx context.Context, tenantID, modelRef string) (*modelModule.TTSModel, error) {
	modelCtx := permission.WithTenantID(ctx, tenantID)
	if strings.TrimSpace(modelRef) == "" {
		return s.modelFactory.NewDefaultTTSModel(modelCtx)
	}
	return s.modelFactory.NewTTSModel(modelCtx, modelRef)
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
	if usage.UserEmail == "" {
		usage.UserEmail = userEmailForUsage(ctx, usage.UserID)
	}
}

// userEmailForUsage best-effort resolves the acting user's email so every
// usage row carries a human-readable identity alongside UserID. Callers that
// already know the email (e.g. the model_chat handler) keep theirs; lookup
// failures leave the field empty and never interrupt the model call.
func userEmailForUsage(ctx context.Context, userID string) string {
	if userID == "" {
		return ""
	}
	user, err := dao.NewUserDAO().GetByID(ctx, dao.DB, userID)
	if err != nil {
		return ""
	}
	return user.Email
}
