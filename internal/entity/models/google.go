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
	"math"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"strings"

	"google.golang.org/genai"
)

type googleModelPage struct {
	items         []ModelListItem
	nextPageToken string
}

func collectGoogleModelNames(ctx context.Context, listPage func(context.Context, string) (googleModelPage, error)) ([]ListModelResponse, error) {
	var models []ModelListItem
	pageToken := ""

	for {
		page, err := listPage(ctx, pageToken)
		if err != nil {
			return nil, err
		}

		models = append(models, page.items...)
		if page.nextPageToken == "" {
			return finalizeGoogleModelList(ParseListModel(ModelList{Models: models})), nil
		}
		pageToken = page.nextPageToken
	}
}

// googleUsableActions lists the Gemini supportedActions RAGFlow can serve:
// generateContent covers chat/vision/tts, embedContent covers embedding.
var googleUsableActions = []string{"generateContent", "embedContent", "batchEmbedContents"}

// googleSupportsUsableAction reports whether the model supports at least one
// action RAGFlow can use. Models limited to other actions (image/video/music
// generation, question answering, etc.) are filtered out while listing so
// the list never offers models whose model type is unknown or unsupported.
func googleSupportsUsableAction(actions []string) bool {
	for _, action := range actions {
		for _, usable := range googleUsableActions {
			if action == usable {
				return true
			}
		}
	}
	return false
}

// finalizeGoogleModelList resolves model types for listed models and filters
// out unknown or unsupported model_type values at list-generation time,
// rather than letting them flow into AddModel where they would be stored as
// model_type = 0 and repaired later. Catalog types win; models missing from
// the static catalog fall back to name-hint inference; models that still
// have no supported type are dropped.
func finalizeGoogleModelList(list []ListModelResponse) []ListModelResponse {
	if list == nil {
		return nil
	}
	filtered := make([]ListModelResponse, 0, len(list))
	for _, item := range list {
		types := item.ModelTypes
		if len(types) == 0 {
			types = InferModelTypes(item.Name)
		}
		supported := make([]string, 0, len(types))
		for _, t := range types {
			if entity.ModelTypeFromString(t) != 0 {
				supported = append(supported, t)
			}
		}
		if len(supported) == 0 {
			continue
		}
		item.ModelTypes = supported
		filtered = append(filtered, item)
	}
	return filtered
}

var googleListModels = func(ctx context.Context, config *genai.ClientConfig) ([]ListModelResponse, error) {
	client, err := genai.NewClient(ctx, config)
	if err != nil {
		return nil, err
	}

	return collectGoogleModelNames(ctx, func(ctx context.Context, pageToken string) (googleModelPage, error) {
		models, err := client.Models.List(ctx, &genai.ListModelsConfig{PageToken: pageToken})
		if err != nil {
			return googleModelPage{}, err
		}

		var modelNames []ModelListItem
		for _, m := range models.Items {
			// Skip models limited to actions RAGFlow cannot serve
			// (e.g. imagen/veo generation-only models); see
			// finalizeGoogleModelList.
			if len(m.SupportedActions) > 0 && !googleSupportsUsableAction(m.SupportedActions) {
				continue
			}
			// Use the API model ID ("models/gemini-2.5-flash" →
			// "gemini-2.5-flash") so listed models match the static
			// catalog (model types / max_tokens) and are directly
			// usable in chat requests. Display names ("Gemini 2.5
			// Flash") are not accepted by the Gemini API.
			modelName := strings.TrimSpace(strings.TrimPrefix(m.Name, "models/"))
			if modelName == "" {
				modelName = strings.TrimSpace(m.DisplayName)
			}
			if modelName != "" {
				modelNames = append(modelNames, ModelListItem{
					ID:      modelName,
					OwnedBy: "Gemini",
				})
			}
		}
		return googleModelPage{items: modelNames, nextPageToken: models.NextPageToken}, nil
	})
}

// GoogleModel implements ModelDriver for Google AI
type GoogleModel struct {
	baseModel BaseModel
}

// NewGoogleModel creates a new Google AI model instance
func NewGoogleModel(baseURL map[string]string, urlSuffix URLSuffix) *GoogleModel {
	return &GoogleModel{
		baseModel: BaseModel{
			BaseURL:   baseURL,
			URLSuffix: urlSuffix,
		},
	}
}

func (g *GoogleModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewGoogleModel(baseURL, g.baseModel.URLSuffix)
}

func (g *GoogleModel) Name() string {
	return "Gemini"
}

func (g *GoogleModel) clientConfig(apiKey string, apiConfig *APIConfig) *genai.ClientConfig {
	return &genai.ClientConfig{APIKey: apiKey, Backend: genai.BackendGeminiAPI, HTTPOptions: genai.HTTPOptions{BaseURL: g.baseURL(apiConfig)}}
}

func (g *GoogleModel) baseURL(apiConfig *APIConfig) string {
	baseURL, err := g.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		defaultConfig := &APIConfig{}
		defaultConfig.BaseURL = apiConfig.BaseURL

		baseURL, err = g.baseModel.GetBaseURL(defaultConfig)
		if err != nil {
			return ""
		}
	}
	return strings.TrimSpace(baseURL)
}

func googleSystemInstruction(messages []Message) (*genai.Content, error) {
	var parts []*genai.Part
	for _, msg := range messages {
		if msg.Role != "system" {
			continue
		}
		for _, part := range googleMessageParts(msg.Content) {
			if part.FileData != nil {
				return nil, fmt.Errorf("gemini: system message must be text only, got image content")
			}
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return nil, nil
	}
	return &genai.Content{Parts: parts}, nil
}

func googleChatContents(messages []Message) []*genai.Content {
	var contents []*genai.Content
	toolCallNames := make(map[string]string)

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			continue
		case "tool":
			name := toolCallNames[msg.ToolCallID]
			if name == "" {
				name = msg.ToolCallID
			}
			contents = append(contents, &genai.Content{
				Role: genai.RoleUser,
				Parts: []*genai.Part{{
					FunctionResponse: &genai.FunctionResponse{
						ID:       msg.ToolCallID,
						Name:     name,
						Response: googleFunctionResponse(msg.Content),
					},
				}},
			})
			continue
		}

		var role genai.Role
		switch msg.Role {
		case "model", "assistant":
			role = genai.RoleModel
		default:
			role = genai.RoleUser
		}

		parts := googleMessageParts(msg.Content)
		for _, toolCall := range msg.ToolCalls {
			id, _ := toolCall["id"].(string)
			fn, _ := toolCall["function"].(map[string]interface{})
			name, _ := fn["name"].(string)
			if name == "" {
				continue
			}
			args := map[string]any{}
			if arguments, ok := fn["arguments"].(string); ok && strings.TrimSpace(arguments) != "" {
				_ = json.Unmarshal([]byte(arguments), &args)
			}
			if id != "" {
				toolCallNames[id] = name
			}
			parts = append(parts, &genai.Part{FunctionCall: &genai.FunctionCall{
				ID:   id,
				Name: name,
				Args: args,
			}})
		}
		if len(parts) > 0 {
			contents = append(contents, genai.NewContentFromParts(parts, role))
		}
	}

	return contents
}

func googleMessageParts(content interface{}) []*genai.Part {
	switch c := content.(type) {
	case string:
		if c == "" {
			return nil
		}
		return []*genai.Part{genai.NewPartFromText(c)}
	case []interface{}:
		var parts []*genai.Part
		for _, item := range c {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			contentType, _ := itemMap["type"].(string)
			switch contentType {
			case "text":
				if text, ok := itemMap["text"].(string); ok && text != "" {
					parts = append(parts, genai.NewPartFromText(text))
				}
			case "image_url":
				if imgMap, ok := itemMap["image_url"].(map[string]interface{}); ok {
					if url, ok := imgMap["url"].(string); ok && url != "" {
						parts = append(parts, genai.NewPartFromURI(url, "image/jpeg"))
					}
				}
			}
		}
		return parts
	default:
		return nil
	}
}

func googleFunctionResponse(content interface{}) map[string]any {
	switch c := content.(type) {
	case map[string]any:
		return c
	case string:
		var response map[string]any
		if err := json.Unmarshal([]byte(c), &response); err == nil && response != nil {
			return response
		}
		return map[string]any{"output": c}
	default:
		return map[string]any{"output": c}
	}
}

func googleGenerateContentConfig(chatModelConfig *ChatConfig, systemInstruction *genai.Content) (*genai.GenerateContentConfig, error) {
	cfg := &genai.GenerateContentConfig{SystemInstruction: systemInstruction}
	if chatModelConfig != nil {
		if chatModelConfig.Temperature != nil {
			value := float32(*chatModelConfig.Temperature)
			cfg.Temperature = &value
		}
		if chatModelConfig.TopP != nil {
			value := float32(*chatModelConfig.TopP)
			cfg.TopP = &value
		}
		if chatModelConfig.MaxTokens != nil {
			if *chatModelConfig.MaxTokens < 0 || *chatModelConfig.MaxTokens > math.MaxInt32 {
				return nil, fmt.Errorf("gemini: max_tokens %d is out of range for int32", *chatModelConfig.MaxTokens)
			}
			cfg.MaxOutputTokens = int32(*chatModelConfig.MaxTokens)
		}
		if chatModelConfig.Stop != nil {
			cfg.StopSequences = *chatModelConfig.Stop
		}
		if tools := googleTools(chatModelConfig.Tools); len(tools) > 0 {
			cfg.Tools = tools
			cfg.ToolConfig = &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: googleFunctionCallingMode(chatModelConfig.ToolChoice)},
			}
		}
	}

	if cfg.SystemInstruction == nil && cfg.Temperature == nil && cfg.TopP == nil && cfg.MaxOutputTokens == 0 && len(cfg.StopSequences) == 0 && len(cfg.Tools) == 0 {
		return nil, nil
	}
	return cfg, nil
}

func googleFunctionCallingMode(toolChoice *string) genai.FunctionCallingConfigMode {
	if toolChoice == nil {
		return genai.FunctionCallingConfigModeAuto
	}
	switch strings.ToLower(strings.TrimSpace(*toolChoice)) {
	case "none":
		return genai.FunctionCallingConfigModeNone
	case "required", "any":
		return genai.FunctionCallingConfigModeAny
	default:
		return genai.FunctionCallingConfigModeAuto
	}
}

func googleTools(rawTools interface{}) []*genai.Tool {
	var declarations []*genai.FunctionDeclaration
	for _, rawTool := range normalizeToolList(rawTools) {
		toolMap, ok := rawTool.(map[string]interface{})
		if !ok {
			continue
		}
		fn, ok := toolMap["function"].(map[string]interface{})
		if !ok {
			fn = toolMap
		}
		name, _ := fn["name"].(string)
		if name == "" {
			continue
		}
		description, _ := fn["description"].(string)
		declaration := &genai.FunctionDeclaration{
			Name:        name,
			Description: description,
		}
		if parameters, ok := fn["parameters"]; ok {
			declaration.ParametersJsonSchema = parameters
		}
		declarations = append(declarations, declaration)
	}
	if len(declarations) == 0 {
		return nil
	}
	return []*genai.Tool{{FunctionDeclarations: declarations}}
}

func normalizeToolList(rawTools interface{}) []interface{} {
	switch tools := rawTools.(type) {
	case nil:
		return nil
	case []interface{}:
		return tools
	case []map[string]interface{}:
		result := make([]interface{}, 0, len(tools))
		for _, tool := range tools {
			result = append(result, tool)
		}
		return result
	default:
		return nil
	}
}

func googleToolCalls(functionCalls []*genai.FunctionCall) []map[string]interface{} {
	if len(functionCalls) == 0 {
		return nil
	}
	toolCalls := make([]map[string]interface{}, 0, len(functionCalls))
	for idx, functionCall := range functionCalls {
		if functionCall == nil || functionCall.Name == "" {
			continue
		}
		id := functionCall.ID
		if id == "" {
			id = fmt.Sprintf("gemini-call-%d", idx)
		}
		arguments, err := json.Marshal(functionCall.Args)
		if err != nil {
			arguments = []byte("{}")
		}
		toolCalls = append(toolCalls, map[string]interface{}{
			"id":   id,
			"type": "function",
			"function": map[string]interface{}{
				"name":      functionCall.Name,
				"arguments": string(arguments),
			},
		})
	}
	return toolCalls
}

// googleUsageFromMetadata converts the SDK's
// GenerateContentResponseUsageMetadata into the package's TokenUsage.
// It returns nil when the metadata is absent so callers can pass the
// result directly to recordResponseUsage without a separate presence
// check. Per the SDK, TotalTokenCount is the authoritative sum of
// prompt + candidates + tool-use prompt + thoughts, so we sum the
// per-bucket counts the same way and use TotalTokenCount as-is when
// it is present. ToolUsePromptTokenCount is treated as part of the
// prompt (it is input billed to the user even though the SDK counts
// it separately from PromptTokenCount).
func googleUsageFromMetadata(m *genai.GenerateContentResponseUsageMetadata) *TokenUsage {
	if m == nil {
		return nil
	}
	in := int(m.PromptTokenCount + m.ToolUsePromptTokenCount)
	out := int(m.CandidatesTokenCount + m.ThoughtsTokenCount)
	total := int(m.TotalTokenCount)
	if in == 0 && out == 0 && total == 0 {
		return nil
	}
	if total == 0 {
		total = in + out
	}
	return &TokenUsage{
		PromptTokens:     in,
		CompletionTokens: out,
		TotalTokens:      total,
	}
}

func (g *GoogleModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, fmt.Errorf("model name is empty")
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	client, err := genai.NewClient(ctx, g.clientConfig(strings.TrimSpace(*apiConfig.ApiKey), apiConfig))
	if err != nil {
		return nil, err
	}

	contents := googleChatContents(messages)
	if len(contents) == 0 {
		return nil, fmt.Errorf("gemini: no conversational message after excluding system messages")
	}
	systemInstruction, err := googleSystemInstruction(messages)
	if err != nil {
		return nil, err
	}
	generateContentConfig, err := googleGenerateContentConfig(chatModelConfig, systemInstruction)
	if err != nil {
		return nil, err
	}

	// Generate content (non-streaming)
	response, err := client.Models.GenerateContent(ctx, modelName, contents, generateContentConfig)
	if err != nil {
		return nil, err
	}

	// Extract text from response
	answer := response.Text()
	recordResponseUsage(modelUsage, response.ResponseID, googleUsageFromMetadata(response.UsageMetadata), "chat")

	return &ChatResponse{Answer: &answer, ToolCalls: googleToolCalls(response.FunctionCalls())}, nil
}

// ChatStreamlyWithSender sends messages and streams response via sender function (best performance, no channel)
func (g *GoogleModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if strings.TrimSpace(modelName) == "" {
		return fmt.Errorf("model name is empty")
	}
	if sender == nil {
		return fmt.Errorf("sender is nil")
	}

	client, err := genai.NewClient(ctx, g.clientConfig(strings.TrimSpace(*apiConfig.ApiKey), apiConfig))
	if err != nil {
		return err
	}

	contents := googleChatContents(messages)
	if len(contents) == 0 {
		return fmt.Errorf("gemini: no conversational message after excluding system messages")
	}
	systemInstruction, err := googleSystemInstruction(messages)
	if err != nil {
		return err
	}
	generateContentConfig, err := googleGenerateContentConfig(chatModelConfig, systemInstruction)
	if err != nil {
		return err
	}
	var toolCalls []map[string]interface{}

	// Capture the most recent UsageMetadata across the stream so we
	// can record it once after the iterator finishes. Each chunk may
	// carry partial counts; the SDK's authoritative total is in the
	// final chunk's UsageMetadata.
	var streamUsage *TokenUsage
	for response, err := range client.Models.GenerateContentStream(
		ctx,
		modelName,
		contents,
		generateContentConfig,
	) {
		if err != nil {
			return err
		}

		toolCalls = append(toolCalls, googleToolCalls(response.FunctionCalls())...)

		content := response.Text()

		var responseContent string
		if chatModelConfig != nil && chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
			if len(response.Candidates) > 0 &&
				response.Candidates[0].Content != nil &&
				len(response.Candidates[0].Content.Parts) > 0 {
				responseContent = response.Candidates[0].Content.Parts[0].Text
			}
		}

		if responseContent != "" {
			common.Info(fmt.Sprintf("Thinking: %s", responseContent))
			if err = sender(nil, &responseContent); err != nil {
				return err
			}
		}

		if content != "" {
			common.Info(fmt.Sprintf("Answer: %s", content))
			if err = sender(&content, nil); err != nil {
				return err
			}
		}

		if u := googleUsageFromMetadata(response.UsageMetadata); u != nil {
			streamUsage = u
		}
	}

	if streamUsage != nil {
		// Use the shared applyStreamUsage path so chatConfig.UsageResult
		// and the clickhouse collection happen together — matching the
		// behaviour of every other streaming driver — and so we do
		// not collect the same usage twice.
		applyStreamUsage(chatModelConfig, modelUsage, streamUsage)
	}

	if chatModelConfig != nil && len(toolCalls) > 0 {
		chatModelConfig.ToolCallsResult = &toolCalls
	}

	return err
}

// Embed generates embeddings for a batch of texts using the Gemini embeddings API.
// The SDK routes to batchEmbedContents internally, so all texts are sent in one request.
func (g *GoogleModel) Embed(ctx context.Context, modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if len(texts) == 0 {
		return nil, fmt.Errorf("texts is empty")
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	client, err := genai.NewClient(ctx, g.clientConfig(strings.TrimSpace(*apiConfig.ApiKey), apiConfig))
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	contents := make([]*genai.Content, len(texts))
	for i, text := range texts {
		contents[i] = genai.NewContentFromText(text, genai.RoleUser)
	}

	var cfg *genai.EmbedContentConfig
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		dim := int32(embeddingConfig.Dimension)
		cfg = &genai.EmbedContentConfig{OutputDimensionality: &dim}
	}

	resp, err := client.Models.EmbedContent(ctx, *modelName, contents, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to embed content: %w", err)
	}

	if len(resp.Embeddings) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(resp.Embeddings))
	}

	result := make([]EmbeddingData, len(resp.Embeddings))
	for i, emb := range resp.Embeddings {
		vec := make([]float64, len(emb.Values))
		for j, v := range emb.Values {
			vec[j] = float64(v)
		}
		result[i] = EmbeddingData{
			Embedding: vec,
			Index:     i,
		}
	}

	return result, nil
}

func (g *GoogleModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := g.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	return googleListModels(ctx, g.clientConfig(strings.TrimSpace(*apiConfig.ApiKey), apiConfig))
}

func (g *GoogleModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

func (g *GoogleModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := g.ListModels(ctx, apiConfig)
	return err
}

// Rerank calculates similarity scores between query and documents
func (g *GoogleModel) Rerank(ctx context.Context, modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, Rerank not implemented", g.Name())
}

// TranscribeAudio transcribe audio
func (g *GoogleModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GoogleModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", g.Name())
}

// AudioSpeech convert text to audio
func (g *GoogleModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GoogleModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", g.Name())
}

// OCRFile OCR file
func (g *GoogleModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

// ParseFile parse file
func (g *GoogleModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GoogleModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}

func (g *GoogleModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", g.Name())
}
