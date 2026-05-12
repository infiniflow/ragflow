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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// VoyageModel implements ModelDriver for Voyage AI.
//
// Voyage AI exposes a focused REST API at https://api.voyageai.com/v1
// with embedding (/embeddings) and reranking (/rerank) only — no chat,
// no streaming, no /v1/models, no balance. This driver covers Embed
// and Rerank with real implementations and returns "no such method"
// for every other ModelDriver method.
//
// Wire shape, captured live:
//
//   Embed response:  {object, data:[{object,embedding,index,text}], model, usage}
//   Rerank response: {object, data:[{relevance_score,index}], model, usage}
//
// Rerank uses top_k as the request param name (not top_n like
// Aliyun/SiliconFlow); the driver translates RerankConfig.TopN to
// top_k on the wire.
type VoyageModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

// NewVoyageModel creates a new Voyage AI model instance.
//
// We clone http.DefaultTransport so we keep Go's defaults for
// ProxyFromEnvironment, DialContext (with KeepAlive), HTTP/2,
// TLSHandshakeTimeout, and ExpectContinueTimeout, and only override
// the connection-pool fields we care about.
func NewVoyageModel(baseURL map[string]string, urlSuffix URLSuffix) *VoyageModel {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.DisableCompression = false
	transport.ResponseHeaderTimeout = 60 * time.Second

	return &VoyageModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

func (v *VoyageModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewVoyageModel(baseURL, v.URLSuffix)
}

func (v *VoyageModel) Name() string {
	return "voyage"
}

// baseURLForRegion returns the base URL for the given region, or an
// error if no entry exists. Single-region for Voyage but kept here
// for consistency with other drivers.
func (v *VoyageModel) baseURLForRegion(region string) (string, error) {
	base, ok := v.BaseURL[region]
	if !ok || base == "" {
		return "", fmt.Errorf("voyage: no base URL configured for region %q", region)
	}
	return base, nil
}

// voyageKnownModels is the list of models we ship in
// conf/models/voyage.json. Voyage does not expose a /v1/models
// endpoint, so ListModels and CheckConnection synthesize the list
// from this constant.
var voyageKnownModels = []string{
	"voyage-3.5",
	"voyage-3.5-lite",
	"voyage-3-large",
	"voyage-code-3",
	"voyage-law-2",
	"voyage-finance-2",
	"rerank-2",
	"rerank-2-lite",
}

type voyageEmbeddingData struct {
	Embedding []float64 `json:"embedding"`
	Object    string    `json:"object"`
	Index     int       `json:"index"`
}

type voyageEmbeddingResponse struct {
	Object string                `json:"object"`
	Data   []voyageEmbeddingData `json:"data"`
	Model  string                `json:"model"`
}

// Embed turns a list of texts into embedding vectors using the
// Voyage AI /v1/embeddings endpoint. Output is one vector per input,
// in the same order the inputs were given.
func (v *VoyageModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := v.baseURLForRegion(region)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", baseURL, v.URLSuffix.Embedding)

	reqBody := map[string]interface{}{
		"model": *modelName,
		"input": texts,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Voyage embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed voyageEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Reorder by the reported index so the output always lines up with
	// the input texts. Reject duplicates (silent overwrite would hide
	// a malformed response) and out-of-range indices (silent panic on
	// slice growth would mask the bug).
	embeddings := make([]EmbeddingData, len(texts))
	filled := make([]bool, len(texts))
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= len(texts) {
			return nil, fmt.Errorf("voyage: response index %d out of range for %d inputs", item.Index, len(texts))
		}
		if filled[item.Index] {
			return nil, fmt.Errorf("voyage: duplicate embedding index %d in response", item.Index)
		}
		embeddings[item.Index] = EmbeddingData{
			Embedding: item.Embedding,
			Index:     item.Index,
		}
		filled[item.Index] = true
	}
	for i, ok := range filled {
		if !ok {
			return nil, fmt.Errorf("voyage: missing embedding for input index %d", i)
		}
	}

	return embeddings, nil
}

type voyageRerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopK      int      `json:"top_k"`
}

type voyageRerankResponse struct {
	Object string `json:"object"`
	Data   []struct {
		RelevanceScore float64 `json:"relevance_score"`
		Index          int     `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
}

// Rerank calculates similarity scores between a query and a list of
// documents using Voyage AI's /v1/rerank endpoint. Unlike many other
// rerank APIs that use `top_n`, Voyage uses `top_k` as the request
// parameter; the driver translates RerankConfig.TopN -> top_k.
func (v *VoyageModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := v.baseURLForRegion(region)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), v.URLSuffix.Rerank)

	topK := len(documents)
	if rerankConfig != nil && rerankConfig.TopN > 0 {
		topK = rerankConfig.TopN
	}

	reqBody := voyageRerankRequest{
		Model:     *modelName,
		Query:     query,
		Documents: documents,
		TopK:      topK,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Voyage rerank API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed voyageRerankResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	rerankResponse := &RerankResponse{}
	for _, r := range parsed.Data {
		if r.Index < 0 || r.Index >= len(documents) {
			return nil, fmt.Errorf("voyage: rerank result index %d out of range for %d documents", r.Index, len(documents))
		}
		rerankResponse.Data = append(rerankResponse.Data, RerankResult{
			Index:          r.Index,
			RelevanceScore: r.RelevanceScore,
		})
	}

	return rerankResponse, nil
}

// ListModels returns the static list of supported Voyage models.
// Voyage does not expose a /v1/models endpoint, so this is sourced
// from voyageKnownModels rather than the network. A subsequent
// embed/rerank call will validate that the chosen model actually
// works for the tenant's API key.
func (v *VoyageModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}
	models := make([]string, len(voyageKnownModels))
	copy(models, voyageKnownModels)
	return models, nil
}

// CheckConnection runs a one-input embedding call against voyage-3.5
// to verify both the API key and the network path. Without /v1/models,
// this is the cheapest way to confirm the integration works
// end-to-end before a tenant tries an actual workload.
func (v *VoyageModel) CheckConnection(apiConfig *APIConfig) error {
	model := "voyage-3.5"
	_, err := v.Embed(&model, []string{"ping"}, apiConfig, nil)
	return err
}

// ChatWithMessages is not exposed by the Voyage AI API.
func (v *VoyageModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}

func (v *VoyageModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", v.Name())
}

// Balance is not exposed by the Voyage AI API.
func (v *VoyageModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}

// TranscribeAudio / AudioSpeech / OCRFile: Voyage does not host any of
// these surfaces.
func (v *VoyageModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}

func (v *VoyageModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", v.Name())
}

func (v *VoyageModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, asrConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}

func (v *VoyageModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", v.Name())
}

func (v *VoyageModel) OCRFile(modelName *string, fileContent *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}
