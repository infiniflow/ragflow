//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/logger"

	"go.uber.org/zap"
)

// AudioService handles text-to-speech and speech-to-text operations.
// It coordinates between HTTP handlers (chat.go) and the model layer
// (ModelProviderService) to provide TTS and ASR capabilities for chat endpoints.
//
// This service adapts the ModelDriver-based audio methods to the chat-level API
// contract expected by the Python backend's /chat/audio/speech and /chat/audio/transcription.
type AudioService struct {
	modelProviderService *ModelProviderService
	tenantDAO           *dao.TenantDAO
}

// NewAudioService creates a new AudioService with required dependencies.
func NewAudioService(modelProviderSvc *ModelProviderService) *AudioService {
	return &AudioService{
		modelProviderService: modelProviderSvc,
		tenantDAO:           dao.NewTenantDAO(),
	}
}

// ttsSentenceDelimiters contains the punctuation marks used to split text into
// segments for TTS synthesis, matching Python's re.split pattern:
//   r"[，。/《》？；：！\n\r:;]+"
var ttsSentenceDelimiters = ",。/《》？；：！\n\r:;"

// getTenantDefaultModelID resolves the tenant's default model identifier for
// the given model type (TTS or speech2text).
func (s *AudioService) getTenantDefaultModelID(tenantID string, modelType entity.ModelType) (string, error) {
	tenant, err := s.tenantDAO.GetByID(tenantID)
	if err != nil {
		return "", fmt.Errorf("Tenant not found")
	}

	var modelID string
	switch modelType {
	case entity.ModelTypeTTS:
		if tenant.TTSID == nil || *tenant.TTSID == "" {
			return "", fmt.Errorf("No default tts model is set.")
		}
		modelID = *tenant.TTSID
	case entity.ModelTypeSpeech2Text:
		modelID = tenant.ASRID
		if modelID == "" {
			return "", fmt.Errorf("No default speech2text model is set.")
		}
	default:
		return "", fmt.Errorf("Unsupported model type %s", modelType)
	}

	return modelID, nil
}

// resolveModelCredentials extracts the provider name, instance name, model name,
// API key, base URL, region from the tenant's default model configuration.
func (s *AudioService) resolveModelCredentials(ctx context.Context, tenantID, compositeModelName string) (providerName, instanceName, modelName string, apiConfig *models.APIConfig, err error) {
	// Parse composite model name (format: "model@instance@provider" or "model@provider")
	mName, iName, pName, parseErr := parseModelName(compositeModelName)
	if parseErr != nil {
		err = fmt.Errorf("invalid model name format %q: %w", compositeModelName, parseErr)
		return
	}

	// Look up tenant's LLM configuration to get provider and credentials
	tenantLLM, lookupErr := dao.NewTenantLLMDAO().GetByTenantAndModelName(tenantID, pName, mName)
	if lookupErr != nil {
		err = fmt.Errorf("model %q not found for tenant %s: %w", compositeModelName, tenantID, lookupErr)
		return
	}

	// Get tenant model provider to find instance
	tenantProvider, provErr := dao.NewTenantModelProviderDAO().GetByTenantIDAndProviderName(tenantID, pName)
	if provErr != nil {
		err = fmt.Errorf("provider %q not found for tenant: %w", pName, provErr)
		return
	}

	// Get provider instance for API key and extra config
	instance, instErr := dao.NewTenantModelInstanceDAO().GetByProviderIDAndInstanceName(tenantProvider.ID, iName)
	if instErr != nil {
		err = fmt.Errorf("instance %q not found: %w", iName, instErr)
		return
	}

	// Parse extra configuration (base_url, region, etc.)
	var extra map[string]string
	if instance.Extra != "" {
		if unmarshalErr := jsonUnmarshal(instance.Extra, &extra); unmarshalErr != nil {
			logger.Warn("Failed to parse instance extra config", zap.Error(unmarshalErr))
		}
	}

	apiConfig = &models.APIConfig{
		ApiKey: &instance.APIKey,
	}

	if r, ok := extra["region"]; ok && r != "" {
		apiConfig.Region = &r
	}
	if baseURL, ok := extra["base_url"]; ok && baseURL != "" {
		apiConfig.BaseURL = &baseURL
	}

	// Store tenant LLM ID for usage tracking later
	_ = tenantLLM // Will be used for usage tracking in future

	providerName = pName
	instanceName = iName
	modelName = mName
	return
}

// PrepareSpeech resolves the tenant's default TTS model and splits the
// text into segments that will be synthesized. All validation is done before
// the HTTP response body is committed so errors can be returned as clean JSON.
//
// Returns the model credentials needed for streaming synthesis.
func (s *AudioService) PrepareSpeech(ctx context.Context, tenantID, text, voice string) (*SpeechContext, []string, error) {
	modelID, err := s.getTenantDefaultModelID(tenantID, entity.ModelTypeTTS)
	if err != nil {
		return nil, nil, err
	}

	providerName, instanceName, modelName, apiConfig, err := s.resolveModelCredentials(ctx, tenantID, modelID)
	if err != nil {
		return nil, nil, err
	}

	ctxData := &SpeechContext{
		TenantID:    tenantID,
		ModelID:     &modelID,
		ProviderName: &providerName,
		InstanceName: &instanceName,
		ModelName:   &modelName,
		APIConfig:   apiConfig,
		Voice:       voice,
	}

	// Split the text into segments using the same delimiters as Python's
	// re.split(r"[，。/《》？；：！\n\r:;]+", text). This splits on any of these
	// punctuation characters and discards empty segments.
	var segments []string
	for _, seg := range strings.FieldsFunc(text, func(r rune) bool {
		return strings.ContainsRune(ttsSentenceDelimiters, r)
	}) {
		if seg = strings.TrimSpace(seg); seg != "" {
			segments = append(segments, seg)
		}
	}
	// If no delimiters were found (e.g., text without punctuation), treat whole text as one segment.
	if len(segments) == 0 && strings.TrimSpace(text) != "" {
		segments = []string{strings.TrimSpace(text)}
	}
	return ctxData, segments, nil
}

// SpeechContext holds resolved model credentials for TTS streaming.
type SpeechContext struct {
	TenantID     string
	ModelID      *string
	ProviderName *string
	InstanceName *string
	ModelName    *string
	APIConfig    *models.APIConfig
	Voice        string
}

// StreamSpeech synthesizes each segment via the ModelProviderService and forwards audio chunks.
func (s *AudioService) StreamSpeech(ctx context.Context, speechCtx *SpeechContext, segments []string, sender func(chunk []byte) error) error {
	ttsConfig := &models.TTSConfig{}
	if speechCtx.Voice != "" {
		if ttsConfig.Params == nil {
			ttsConfig.Params = make(map[string]interface{})
		}
		ttsConfig.Params["voice"] = speechCtx.Voice
	}

	for _, segment := range segments {
		textPtr := segment
		_, code, err := s.modelProviderService.AudioSpeechStream(
			speechCtx.ProviderName,
			speechCtx.InstanceName,
			speechCtx.ModelName,
			speechCtx.ModelID,
			speechCtx.TenantID,
			&textPtr,
			speechCtx.APIConfig,
			ttsConfig,
			func(data *string, format *string) error {
				if data != nil && *data != "" {
					return sender([]byte(*data))
				}
				return nil
			},
		)
		if err != nil {
			return fmt.Errorf("AudioSpeech failed (code=%d): %w", code, err)
		}
	}
	return nil
}

// Speech is a convenience wrapper that prepares and streams in one call.
func (s *AudioService) Speech(ctx context.Context, tenantID, text, voice string, sender func(chunk []byte) error) error {
	ctxData, segments, err := s.PrepareSpeech(ctx, tenantID, text, voice)
	if err != nil {
		return err
	}
	return s.StreamSpeech(ctx, ctxData, segments, sender)
}

// StreamTranscription streams transcription events (SSE) using the tenant's
// default speech-to-text model via ModelProviderService.TranscribeAudioStream.
func (s *AudioService) StreamTranscription(ctx context.Context, tenantID, audioPath string, sender func(event entity.TranscriptionEvent) error) error {
	modelID, err := s.getTenantDefaultModelID(tenantID, entity.ModelTypeSpeech2Text)
	if err != nil {
		sendErr := sender(entity.TranscriptionEvent{Event: "error", Text: err.Error(), Streaming: false})
		if sendErr != nil {
			return fmt.Errorf("original error: %w, sender error: %v", err, sendErr)
		}
		return err
	}

	providerName, instanceName, modelName, apiConfig, err := s.resolveModelCredentials(ctx, tenantID, modelID)
	if err != nil {
		sendErr := sender(entity.TranscriptionEvent{Event: "error", Text: err.Error(), Streaming: false})
		if sendErr != nil {
			return fmt.Errorf("original error: %w, sender error: %v", err, sendErr)
		}
		return err
	}

	asrConfig := &models.ASRConfig{}

	audioFile := audioPath
	code, streamErr := s.modelProviderService.TranscribeAudioStream(
		&providerName,
		&instanceName,
		&modelName,
		&modelID,
		tenantID,
		&audioFile,
		apiConfig,
		asrConfig,
		func(text *string, streaming *bool) error {
			var evt entity.TranscriptionEvent
			if streaming != nil && *streaming {
				evt = entity.TranscriptionEvent{Event: "delta", Text: *text, Streaming: true}
			} else {
				evt = entity.TranscriptionEvent{Event: "final", Text: *text, Streaming: false}
			}
			return sender(evt)
		},
	)

	if streamErr != nil {
		// Try sending error event if not already done
		_ = sender(entity.TranscriptionEvent{Event: "error", Text: fmt.Sprintf("Transcription failed (code=%d): %s", code, streamErr.Error()), Streaming: false})
		return fmt.Errorf("TranscribeAudioStream failed (code=%d): %w", code, streamErr)
	}
	return nil
}

// Transcription transcribes the audio file at audioPath using the tenant's
// default speech-to-text model. Returns the recognized text.
func (s *AudioService) Transcription(ctx context.Context, tenantID, audioPath string) (string, error) {
	modelID, err := s.getTenantDefaultModelID(tenantID, entity.ModelTypeSpeech2Text)
	if err != nil {
		return "", err
	}

	providerName, instanceName, modelName, apiConfig, err := s.resolveModelCredentials(ctx, tenantID, modelID)
	if err != nil {
		return "", err
	}

	asrConfig := &models.ASRConfig{}
	audioFile := audioPath

	response, code, svcErr := s.modelProviderService.TranscribeAudio(
		&providerName,
		&instanceName,
		&modelName,
		&modelID,
		tenantID,
		&audioFile,
		apiConfig,
		asrConfig,
	)
	if svcErr != nil {
		return "", fmt.Errorf("TranscribeAudio failed (code=%d): %w", code, svcErr)
	}

	if response == nil {
		return "", fmt.Errorf("empty response from transcription service")
	}
	return response.Text, nil
}

// jsonUnmarshal is a helper that wraps json.Unmarshal for cleaner error handling.
func jsonUnmarshal(data string, target interface{}) error {
	return json.Unmarshal([]byte(data), target)
}
