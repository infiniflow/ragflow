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
	"slices"
	"strings"

	"ragflow/internal/common"
)

// Qwen3-Reranker is a causal LM that scores "yes"/"no" for a prompt, so its
// query and documents only score correctly inside the model's chat template.
// The gateway forwards them verbatim, hence the client wraps them.
const (
	qwen3RerankPrefix = "<|im_start|>system\nJudge whether the Document meets the requirements based on the Query and the Instruct provided. " +
		"Note that the answer can only be \"yes\" or \"no\".<|im_end|>\n<|im_start|>user\n"
	qwen3RerankInstruct = "Given a web search query, retrieve relevant passages that answer the query"
	qwen3RerankSuffix   = "<|im_end|>\n<|im_start|>assistant\n<think>\n\n</think>\n\n"
)

// GreenNodeModel drives GreenNode MaaS (https://docs.greennode.ai), which
// serves chat, embedding and rerank models behind one OpenAI-compatible API.
// Chat and model listing come from the embedded OpenAIAPICompatibleModel;
// Embed and Rerank are overridden for the gateway's quirks. The default base
// URL is the pay-as-you-go endpoint; Token Plan keys use a different host, set
// through the instance base URL. The model list is scoped to the API key and
// the gateway allows 10 requests per minute per account across all models.
type GreenNodeModel struct {
	*OpenAIAPICompatibleModel
}

// NewGreenNodeModel creates a GreenNode driver.
func NewGreenNodeModel(baseURL map[string]string, urlSuffix URLSuffix) *GreenNodeModel {
	compatible := NewOpenAIAPICompatibleModel(baseURL, urlSuffix)
	compatible.VllmModel.baseModel.AllowEmptyAPIKey = false
	return &GreenNodeModel{OpenAIAPICompatibleModel: compatible}
}

// Name returns the driver name.
func (m *GreenNodeModel) Name() string {
	return "greennode"
}

// NewInstance returns a driver bound to the given base URL.
func (m *GreenNodeModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewGreenNodeModel(baseURL, m.baseModel.URLSuffix)
}

// greenNodeModelEntry is the part of a GET /models entry that decides how a
// model is typed. The gateway names the kind of each model (chat, embedding,
// rerank, ocr, ...) and lists the input modalities it accepts.
type greenNodeModelEntry struct {
	ID                  string `json:"id"`
	ModelType           string `json:"model_type"`
	Type                string `json:"type"`
	ContextLength       *int   `json:"context_length"`
	MaxCompletionTokens *int   `json:"max_completion_tokens"`
	Architecture        struct {
		InputModalities []string `json:"input_modalities"`
	} `json:"architecture"`
}

// ListModels lists the models of the API key through GET /models and types
// each one by the gateway's own `model_type`: chat (plus vision when it takes
// image input), embedding or rerank. Models of a kind RAGFlow has no slot for
// (ocr, anything unknown) are left out, so they cannot be picked as chat
// models. A gateway that does not report `model_type` (a Token Plan host, for
// instance) falls back to name-based inference, which the catalog can still
// override.
func (m *GreenNodeModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	url, err := m.baseModel.EndpointURL(apiConfig, m.baseModel.URLSuffix.Models)
	if err != nil {
		return nil, err
	}
	body, err := m.baseModel.doGetRequest(ctx, url, apiConfig, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	var envelope struct {
		Data []greenNodeModelEntry `json:"data"`
	}
	if err = json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if envelope.Data == nil {
		return nil, fmt.Errorf("invalid models list format")
	}

	// ParseListModel adds the catalog's limits and embedding metadata for
	// known ids; the types are then set here from the gateway.
	entries := make([]greenNodeModelEntry, 0, len(envelope.Data))
	list := ModelList{Models: make([]ModelListItem, 0, len(envelope.Data))}
	for _, entry := range envelope.Data {
		entry.ID = strings.TrimSpace(entry.ID)
		if entry.ID == "" {
			continue
		}
		entries = append(entries, entry)
		list.Models = append(list.Models, ModelListItem{
			ID:            entry.ID,
			ContextLength: entry.ContextLength,
			MaxTokens:     entry.MaxCompletionTokens,
		})
	}

	parsed := ParseListModel(list)
	models := make([]ListModelResponse, 0, len(parsed))
	for i, model := range parsed {
		entry := entries[i]
		kind := strings.ToLower(strings.TrimSpace(entry.ModelType))
		if kind == "" {
			kind = strings.ToLower(strings.TrimSpace(entry.Type))
		}
		switch kind {
		case "":
			model.ModelTypes = InferModelTypes(model.Name)
			model.ModelTypesInferred = true
		case modelTypeChat:
			model.ModelTypes = []string{modelTypeChat}
			if slices.Contains(entry.Architecture.InputModalities, "image") {
				model.ModelTypes = append(model.ModelTypes, modelTypeVision)
			}
			model.ModelTypesInferred = false
		case modelTypeEmbedding, modelTypeRerank:
			model.ModelTypes = []string{kind}
			model.ModelTypesInferred = false
		default:
			continue
		}
		models = append(models, model)
	}
	return models, nil
}

// CheckConnection verifies the key by listing its models.
func (m *GreenNodeModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := m.ListModels(ctx, apiConfig)
	return err
}

// Embed sends a /embeddings request. The requested dimension is deliberately
// not forwarded: bge-m3 ignores `dimensions` and qwen3-embedding-8b rejects it,
// so each model answers with its fixed dimension.
func (m *GreenNodeModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if len(request.Texts) == 0 {
		return []EmbeddingData{}, nil
	}
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	url, err := m.baseModel.EndpointURL(apiConfig, m.baseModel.URLSuffix.Embedding)
	if err != nil {
		return nil, err
	}

	body, err := m.baseModel.doRequest(ctx, url, apiConfig, map[string]any{
		"model": *modelName,
		"input": request.Texts,
	}, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	var parsed vllmEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	embeddings := make([]EmbeddingData, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		embeddings = append(embeddings, EmbeddingData{Embedding: item.Embedding, Index: item.Index})
	}

	recordResponseUsage(modelUsage, parsed.ID, &TokenUsage{
		PromptTokens: parsed.Usage.PromptTokens,
		TotalTokens:  parsed.Usage.TotalTokens,
	}, "embedding")

	return embeddings, nil
}

// Rerank sends a Cohere/Jina-style /rerank request. top_n defaults to the
// number of documents so every input gets a score. Qwen3 rerankers get their
// query and documents wrapped in the model's prompt template first.
func (m *GreenNodeModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if len(request.Documents) == 0 {
		return &RerankResponse{}, nil
	}
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	url, err := m.baseModel.EndpointURL(apiConfig, m.baseModel.URLSuffix.Rerank)
	if err != nil {
		return nil, err
	}

	topN := len(request.Documents)
	if rerankConfig != nil && rerankConfig.TopN > 0 && rerankConfig.TopN < topN {
		topN = rerankConfig.TopN
	}

	query, documents := request.Query, request.Documents
	if strings.Contains(strings.ToLower(*modelName), "qwen3-reranker") {
		query, documents = wrapQwen3RerankInputs(query, documents)
	}

	body, err := m.baseModel.doRequest(ctx, url, apiConfig, map[string]any{
		"model":     *modelName,
		"query":     query,
		"documents": documents,
		"top_n":     topN,
	}, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	var parsed vllmRerankResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	response := RerankResponse{Data: make([]RerankResult, 0, len(parsed.Results))}
	for _, r := range parsed.Results {
		if r.Index < 0 || r.Index >= len(request.Documents) {
			return nil, fmt.Errorf("unexpected rerank index %d for %d inputs", r.Index, len(request.Documents))
		}
		response.Data = append(response.Data, RerankResult{Index: r.Index, RelevanceScore: r.RelevanceScore})
	}

	recordResponseUsage(modelUsage, parsed.ID, &TokenUsage{
		PromptTokens: parsed.Usage.PromptTokens,
		TotalTokens:  parsed.Usage.TotalTokens,
	}, "rerank")

	return &response, nil
}

// wrapQwen3RerankInputs applies the Qwen3-Reranker prompt template.
func wrapQwen3RerankInputs(query string, documents []string) (string, []string) {
	wrappedQuery := qwen3RerankPrefix + "<Instruct>: " + qwen3RerankInstruct + "\n<Query>: " + query + "\n"
	wrapped := make([]string, len(documents))
	for i, document := range documents {
		wrapped[i] = "<Document>: " + document + qwen3RerankSuffix
	}
	return wrappedQuery, wrapped
}
