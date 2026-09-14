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
	"encoding/json"
	"fmt"
	"io"
	"ragflow/internal/common"
	"strings"
)

// sparkModelVersions maps the catalog model names to the version identifiers
// the XunFei Spark HTTP API expects in the request body's "model" field.
// Mirrors SparkChat.model2version in rag/llm/chat_model.py.
var sparkModelVersions = map[string]string{
	"Spark-Max":       "generalv3.5",
	"Spark-Max-32K":   "max-32k",
	"Spark-Lite":      "lite",
	"Spark-Pro":       "generalv3",
	"Spark-Pro-128K":  "pro-128k",
	"Spark-4.0-Ultra": "4.0Ultra",
}

func resolveSparkModel(modelName string) string {
	if version, ok := sparkModelVersions[modelName]; ok {
		return version
	}
	return modelName
}

// resolveBearerToken extracts the credential used as the Bearer token. The
// instance stores the XunFei credential bundle (API password, APPID, API
// secret, API key) as a JSON object string; the Spark HTTP API authenticates
// with the bundle's spark_api_password.
func resolveBearerToken(apiConfig *APIConfig) string {
	if apiConfig == nil || apiConfig.ApiKey == nil {
		return ""
	}
	key := strings.TrimSpace(*apiConfig.ApiKey)
	if strings.HasPrefix(key, "{") {
		var bundle map[string]interface{}
		if err := json.Unmarshal([]byte(key), &bundle); err == nil {
			if password, ok := bundle["spark_api_password"].(string); ok && password != "" {
				return password
			}
		}
	}
	return key
}

type XunFeiModel struct {
	baseModel BaseModel
}

func NewXunFeiModel(baseURL map[string]string, urlSuffix URLSuffix) *XunFeiModel {
	return &XunFeiModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(false),
			// The Spark HTTP API authenticates with the credential bundle's
			// spark_api_password, not the raw stored key.
			authHeader: func(cfg *APIConfig) (string, string) {
				return "Authorization", "Bearer " + resolveBearerToken(cfg)
			},
		},
	}
}

func (x *XunFeiModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewXunFeiModel(baseURL, x.baseModel.URLSuffix)
}

func (x *XunFeiModel) Name() string {
	return "XunFei Spark"
}

func (x *XunFeiModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := x.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := x.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, x.baseModel.URLSuffix.Chat)
	reqBody := buildRequestBody(chatModelConfig, resolveSparkModel(modelName), messages, false)

	if chatModelConfig != nil {
		if chatModelConfig.Thinking != nil {
			if *chatModelConfig.Thinking {
				reqBody["thinking"] = map[string]interface{}{
					"type": "enabled",
				}
			} else {
				reqBody["thinking"] = map[string]interface{}{
					"type": "disabled",
				}
			}
		}
	}

	body, err := x.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return HandleNonStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

func (x *XunFeiModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := x.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := x.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, x.baseModel.URLSuffix.Chat)

	reqBody := buildRequestBody(modelConfig, resolveSparkModel(modelName), messages, true)

	if modelConfig != nil {
		if modelConfig.Thinking != nil {
			if *modelConfig.Thinking {
				reqBody["thinking"] = map[string]interface{}{
					"type": "enabled",
				}
			} else {
				reqBody["thinking"] = map[string]interface{}{
					"type": "disabled",
				}
			}
		}
	}

	// XunFei's OpenAI-compatible endpoint does not document support for
	// stream_options.include_usage, but its streaming responses carry a
	// usage object in the final chunk by default, so usage reporting works
	// without it.
	return x.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		return HandleStreamingResponse(body, modelUsage, modelConfig, OpenAIParserConfig, sender)
	})
}

func (x *XunFeiModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := x.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	resolvedBaseURL, err := x.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, x.baseModel.URLSuffix.Models)

	body, err := x.baseModel.doGetRequest(ctx, url, apiConfig, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	// Parse response
	var modelList ModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if modelList.Models == nil {
		return nil, fmt.Errorf("invalid models list format")
	}

	return ParseListModel(modelList), nil
}

func (x *XunFeiModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	if err := x.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	// Verify the credential bundle with a minimal chat request against the
	// free Spark-Lite model.
	maxTokens := 1
	chatConfig := &ChatConfig{MaxTokens: &maxTokens}
	_, err := x.ChatWithMessages(ctx, "Spark-Lite", []Message{{Role: "user", Content: "Hi"}}, apiConfig, chatConfig, nil)
	return err
}

func (x *XunFeiModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}
