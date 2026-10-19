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
	"net/http"
	"os"
	"ragflow/internal/common"
	"strings"
)

// AimlapiModel implements ModelDriver for AI/ML API (aimlapi.com), an
// OpenAI-compatible gateway serving chat and embedding models from many
// vendors behind one key. Model ids keep their vendor prefix
// (e.g. "anthropic/claude-sonnet-5.5", "openai/text-embedding-3-small").
type AimlapiModel struct {
	baseModel BaseModel
}

const (
	// aimlapiAttributionHost is the only host partner attribution may reach,
	// compared for equality on the request's hostname: a suffix or substring
	// test would hand the partner id to "api.aimlapi.com.example.net" or
	// "notapi.aimlapi.com".
	aimlapiAttributionHost = "api.aimlapi.com"
	// aimlapiSource is the client identifier sent as X-AIMLAPI-Source.
	// AIMLAPI_SOURCE overrides it.
	aimlapiSource = "agent/ragflow"
	// aimlapiPartnerID is the partner id aimlapi.com issued for RAGFlow. Ids
	// are minted server-side; an unknown one is accepted and silently dropped.
	// AIMLAPI_PARTNER_ID overrides it (staging, testing).
	aimlapiPartnerID = "part_yNkcOvbGLtgxWLjy4sRysaer"
)

// aimlapiEnvOr returns the trimmed value of an environment variable when it is
// set — even to an empty string, which is how an override turns a header off —
// and the default otherwise. This matches common/aimlapi_utils.py from #17491,
// where os.environ.get(name, default) behaves the same way.
func aimlapiEnvOr(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return strings.TrimSpace(value)
	}
	return fallback
}

// NewAimlapiModel creates a new AI/ML API model instance.
func NewAimlapiModel(baseURL map[string]string, urlSuffix URLSuffix) *AimlapiModel {
	return &AimlapiModel{
		baseModel: BaseModel{
			BaseURL:        baseURL,
			URLSuffix:      urlSuffix,
			httpClient:     common.GetSSRFHTTPClient(),
			requestHeaders: applyAimlapiAttribution,
		},
	}
}

// applyAimlapiAttribution adds the attribution established in #17491 —
// X-AIMLAPI-Source and X-AIMLAPI-Partner-ID — but only when the request is
// bound for the aimlapi.com gateway itself. It reads the request's own URL, so
// a base URL override (a proxy, a staging host) moves the request and the
// attribution stops with it rather than leaking. Either header is omitted when
// its override is set to an empty value.
func applyAimlapiAttribution(req *http.Request) {
	if req == nil || req.URL == nil || !strings.EqualFold(req.URL.Hostname(), aimlapiAttributionHost) {
		return
	}
	if source := aimlapiEnvOr("AIMLAPI_SOURCE", aimlapiSource); source != "" {
		req.Header.Set("X-AIMLAPI-Source", source)
	}
	if partnerID := aimlapiEnvOr("AIMLAPI_PARTNER_ID", aimlapiPartnerID); partnerID != "" {
		req.Header.Set("X-AIMLAPI-Partner-ID", partnerID)
	}
}

func (a *AimlapiModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewAimlapiModel(baseURL, a.baseModel.URLSuffix)
}

func (a *AimlapiModel) Name() string {
	return "aimlapi.com"
}

func validateAimlapiModelName(modelName string) error {
	if strings.TrimSpace(modelName) == "" {
		return fmt.Errorf("model name is required")
	}
	return nil
}

// ChatWithMessages sends multiple messages with roles and returns the response.
//
// No vendor-specific thinking field is sent: reasoning controls differ per
// model behind the gateway, and an unknown field is refused by some of them.
func (a *AimlapiModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if err := validateAimlapiModelName(modelName); err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	url, err := a.baseModel.EndpointURL(apiConfig, a.baseModel.URLSuffix.Chat)
	if err != nil {
		return nil, err
	}

	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)

	body, err := a.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return HandleNonStreamingResponse(ctx, body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

// ChatStreamlyWithSender sends messages and streams the response.
func (a *AimlapiModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}
	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	if err := validateAimlapiModelName(modelName); err != nil {
		return err
	}
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if chatModelConfig != nil && chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
		// The body of this method only reads SSE; a non-stream JSON reply
		// would parse as an empty stream. Fail clearly instead.
		return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
	}

	url, err := a.baseModel.EndpointURL(apiConfig, a.baseModel.URLSuffix.Chat)
	if err != nil {
		return err
	}

	reqBody := buildRequestBody(chatModelConfig, modelName, messages, true)
	reqBody["stream_options"] = map[string]interface{}{"include_usage": true}

	return a.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		return HandleStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig, sender)
	})
}

type aimlapiEmbeddingData struct {
	Embedding []float64 `json:"embedding"`
	Object    string    `json:"object"`
	Index     int       `json:"index"`
}

type aimlapiEmbeddingResponse struct {
	Data   []aimlapiEmbeddingData `json:"data"`
	Model  string                 `json:"model"`
	Object string                 `json:"object"`
}

// Embed turns a list of texts into embedding vectors.
func (a *AimlapiModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if len(request.Texts) == 0 {
		return []EmbeddingData{}, nil
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}

	url, err := a.baseModel.EndpointURL(apiConfig, a.baseModel.URLSuffix.Embedding)
	if err != nil {
		return nil, err
	}

	reqBody := map[string]any{
		"model": *modelName,
		"input": request.Texts,
	}
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		reqBody["dimensions"] = embeddingConfig.Dimension
	}

	body, err := a.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	var parsed aimlapiEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	embeddings := make([]EmbeddingData, len(request.Texts))
	filled := make([]bool, len(request.Texts))
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= len(request.Texts) {
			return nil, fmt.Errorf("aimlapi.com: response index %d out of range for %d inputs", item.Index, len(request.Texts))
		}
		if filled[item.Index] {
			return nil, fmt.Errorf("aimlapi.com: duplicate embedding index %d in response", item.Index)
		}
		embeddings[item.Index] = EmbeddingData{Embedding: item.Embedding, Index: item.Index}
		filled[item.Index] = true
	}
	for i, ok := range filled {
		if !ok {
			return nil, fmt.Errorf("aimlapi.com: missing embedding for input index %d", i)
		}
	}

	return embeddings, nil
}

// aimlapiModelRow is one row of the gateway's GET /v1/models. The list mixes
// every endpoint family; `type` is the only field that tells them apart.
type aimlapiModelRow struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Info struct {
		ContextLength *int `json:"contextLength"`
	} `json:"info"`
}

// aimlapiModelFamily maps a gateway row type onto how this driver lists it:
// "chat" and "embedding" for the families it can call, "infer" for a row the
// gateway published without a type (kept, typed from its name like any other
// catalog miss), and "" for families it cannot call (image, video, audio,
// batches, responses). "openai/chat-completions" and "anthropic/messages" are
// both chat: one call shape serves both.
func aimlapiModelFamily(rowType string) string {
	switch strings.TrimSpace(rowType) {
	case "openai/chat-completions", "anthropic/messages":
		return "chat"
	case "openai/embeddings":
		return "embedding"
	case "":
		return "infer"
	}
	return ""
}

// ListModels returns the live chat and embedding roster. GET /v1/models needs
// no key; the request still goes through the shared helper, so it carries
// attribution like every other call.
func (a *AimlapiModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	url, err := a.baseModel.EndpointURL(apiConfig, a.baseModel.URLSuffix.Models)
	if err != nil {
		return nil, err
	}

	body, err := a.baseModel.doGetRequest(ctx, url, apiConfig, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Data []aimlapiModelRow `json:"data"`
	}
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// The same id is published once per family, so merge by id: the two
	// chat types would otherwise list one model twice.
	familyByID := map[string]string{}
	list := ModelList{Object: "list"}
	for _, row := range parsed.Data {
		id := strings.TrimSpace(row.ID)
		family := aimlapiModelFamily(row.Type)
		if id == "" || family == "" {
			continue
		}
		if prev, seen := familyByID[id]; seen {
			// a typed row outranks an untyped one for the same id
			if prev == "infer" && family != "infer" {
				familyByID[id] = family
			}
			continue
		}
		familyByID[id] = family
		list.Models = append(list.Models, ModelListItem{ID: id, ContextLength: row.Info.ContextLength})
	}

	models := ParseListModel(list)
	// The gateway says which rows are embeddings; do not leave that to a
	// guess from the model name.
	for i := range models {
		if familyByID[models[i].Name] == "embedding" {
			models[i].ModelTypes = []string{"embedding"}
		}
	}
	return models, nil
}

// Balance returns the key's own record from GET /v1/key: its scopes, its
// spending limit and its month-to-date usage in US dollars.
func (a *AimlapiModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	url, err := a.baseModel.EndpointURL(apiConfig, a.baseModel.URLSuffix.Balance)
	if err != nil {
		return nil, err
	}

	body, err := a.baseModel.doGetRequest(ctx, url, apiConfig, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Data map[string]interface{} `json:"data"`
	}
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if parsed.Data == nil {
		return nil, fmt.Errorf("aimlapi.com: key response has no data")
	}
	return parsed.Data, nil
}

// CheckConnection verifies the API key. GET /v1/models answers without a key,
// so it cannot tell a valid key from an invalid one; GET /v1/key can, and
// answers 401 for a key it does not know.
func (a *AimlapiModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := a.Balance(ctx, apiConfig)
	return err
}

func (a *AimlapiModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AimlapiModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AimlapiModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", a.Name())
}

func (a *AimlapiModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AimlapiModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", a.Name())
}

func (a *AimlapiModel) OCRFile(ctx context.Context, modelName *string, content []byte, baseURL *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AimlapiModel) ParseFile(ctx context.Context, modelName *string, content []byte, baseURL *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AimlapiModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AimlapiModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}
