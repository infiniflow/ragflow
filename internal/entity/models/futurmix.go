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

package models

import (
	"context"
	"fmt"
	"io"
	"ragflow/internal/common"
	"strings"
)

// FuturMixModel implements ModelDriver for FuturMix
type FuturMixModel struct {
	baseModel BaseModel
}

// NewFuturMixModel creates a new FuturMix model instance.
func NewFuturMixModel(baseURL map[string]string, urlSuffix URLSuffix) *FuturMixModel {
	return &FuturMixModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(false),
		},
	}
}

func (f *FuturMixModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewFuturMixModel(baseURL, f.baseModel.URLSuffix)
}

func (f *FuturMixModel) Name() string {
	return "futurmix"
}

func (f *FuturMixModel) endpointURL(region, suffix string) (string, error) {
	baseURL, err := f.baseModel.GetBaseURL(&APIConfig{Region: &region})
	if err != nil {
		return "", err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	return fmt.Sprintf("%s/%s", baseURL, strings.TrimLeft(suffix, "/")), nil
}

// ChatWithMessages sends a non-streaming chat completion
func (f *FuturMixModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := f.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := f.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, f.baseModel.URLSuffix.Chat)

	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)
	if chatModelConfig != nil && chatModelConfig.Thinking != nil {
		if *chatModelConfig.Thinking {
			reqBody["thinking"] = map[string]any{
				"type": "enabled",
			}
		} else {
			reqBody["thinking"] = map[string]any{
				"type": "disabled",
			}
		}
	}

	body, err := f.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return HandleNonStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

// ChatStreamlyWithSender sends a streaming chat completion
func (f *FuturMixModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := f.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := f.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, f.baseModel.URLSuffix.Chat)

	reqBody := buildRequestBody(chatModelConfig, modelName, messages, true)
	reqBody["stream_options"] = map[string]any{
		"include_usage": true,
	}
	if chatModelConfig != nil && chatModelConfig.Thinking != nil {
		if *chatModelConfig.Thinking {
			reqBody["thinking"] = map[string]any{
				"type": "enabled",
			}
		} else {
			reqBody["thinking"] = map[string]any{
				"type": "disabled",
			}
		}
	}

	return f.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		return HandleStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig, sender)
	})
}

// Embed is not exposed by the FuturMix API per the public docs.
func (f *FuturMixModel) Embed(ctx context.Context, modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", f.Name())
}

// Rerank is not exposed by the FuturMix API per the public docs.
func (f *FuturMixModel) Rerank(ctx context.Context, modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", f.Name())
}

// ListModels is not documented as a public endpoint by FuturMix.
func (f *FuturMixModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	return nil, fmt.Errorf("%s, no such method", f.Name())
}

// CheckConnection is not exposed by the FuturMix API.
func (f *FuturMixModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	return fmt.Errorf("%s, no such method", f.Name())
}

// Balance is not exposed by the FuturMix public API.
func (f *FuturMixModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", f.Name())
}

// TranscribeAudio is not exposed by the FuturMix API per the docs.
func (f *FuturMixModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", f.Name())
}

func (f *FuturMixModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", f.Name())
}

// AudioSpeech is not exposed by the FuturMix API per the docs.
func (f *FuturMixModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", f.Name())
}

func (f *FuturMixModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", f.Name())
}

// OCRFile is not exposed by the FuturMix API per the docs.
func (f *FuturMixModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", f.Name())
}

// ParseFile is not exposed by the FuturMix API per the docs.
func (f *FuturMixModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", f.Name())
}

// ListTasks is not exposed by the FuturMix API per the docs.
func (f *FuturMixModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", f.Name())
}

// ShowTask is not exposed by the FuturMix API per the docs.
func (f *FuturMixModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", f.Name())
}
