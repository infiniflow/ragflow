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
	"net/url"
	"ragflow/internal/common"
	"sort"
	"strings"
	"time"
)

const (
	nvidiaHostedAPIHost   = "integrate.api.nvidia.com"
	nvidiaCatalogURL      = "https://api.ngc.nvidia.com/v2/search/catalog/resources/ENDPOINT"
	nvidiaCatalogPageSize = 500
)

// NvidiaModel implements ModelDriver for Nvidia
type NvidiaModel struct {
	baseModel     BaseModel
	catalogURL    string
	hostedAPIHost string
}

// NewNvidiaModel creates a new Nvidia model instance
func NewNvidiaModel(baseURL map[string]string, urlSuffix URLSuffix) *NvidiaModel {
	return &NvidiaModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(false),
		},
		catalogURL:    nvidiaCatalogURL,
		hostedAPIHost: nvidiaHostedAPIHost,
	}
}

func (n *NvidiaModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewNvidiaModel(baseURL, n.baseModel.URLSuffix)
}

func (n *NvidiaModel) Name() string {
	return "nvidia"
}

func (n *NvidiaModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := n.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := n.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := fmt.Sprintf("%s/%s", resolvedBaseURL, n.baseModel.URLSuffix.Chat)
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)

	if chatModelConfig != nil {
		if chatModelConfig.Thinking != nil {
			if *chatModelConfig.Thinking {
				reqBody["thinking"] = map[string]interface{}{"type": "enabled"}
			} else {
				reqBody["thinking"] = map[string]interface{}{"type": "disabled"}
			}
		}
	}

	body, err := n.baseModel.doRequest(ctx, baseURL, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}
	return HandleNonStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

func (n *NvidiaModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := n.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}
	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := n.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	baseURL := fmt.Sprintf("%s/%s", resolvedBaseURL, n.baseModel.URLSuffix.Chat)
	reqBody := buildRequestBody(modelConfig, modelName, messages, true)

	if modelConfig != nil {
		if modelConfig.Thinking != nil {
			if *modelConfig.Thinking {
				reqBody["thinking"] = map[string]interface{}{"type": "enabled"}
			} else {
				reqBody["thinking"] = map[string]interface{}{"type": "disabled"}
			}
		}
	}

	reqBody["stream_options"] = map[string]any{"include_usage": true}

	return n.baseModel.doStreamRequest(ctx, baseURL, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		return HandleStreamingResponse(body, modelUsage, modelConfig, OpenAIParserConfig, sender)
	})
}

type nvidiaEmbeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func (n *NvidiaModel) Embed(ctx context.Context, modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := n.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := n.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}

	baseURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(resolvedBaseURL, "/"), n.baseModel.URLSuffix.Embedding)

	reqBody := map[string]interface{}{
		"model":           *modelName,
		"input":           texts,
		"input_type":      "query",
		"encoding_format": "float",
		"truncate":        "END",
	}
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		reqBody["dimensions"] = embeddingConfig.Dimension
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := n.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Nvidia embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed nvidiaEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var embeddings []EmbeddingData
	for _, dataElem := range parsed.Data {
		var embeddingData EmbeddingData
		embeddingData.Embedding = dataElem.Embedding
		embeddingData.Index = dataElem.Index
		embeddings = append(embeddings, embeddingData)
	}

	return embeddings, nil
}

// nvidiaRerankRequest mirrors the NIM /ranking request shape:
// query is an object with a "text" field, passages is an array of
// objects each with a "text" field. truncate=END matches the Python
// NvidiaRerank reference at rag/llm/rerank_model.py.
type nvidiaRerankRequest struct {
	Model    string             `json:"model"`
	Query    nvidiaRerankText   `json:"query"`
	Passages []nvidiaRerankText `json:"passages"`
	Truncate string             `json:"truncate,omitempty"`
	TopN     int                `json:"top_n"`
}

type nvidiaRerankText struct {
	Text string `json:"text"`
}

// nvidiaRerankResponse maps the NIM rankings array. Each entry pairs
// the original passage index with a logit score; the caller uses the
// index to restore original input order.
type nvidiaRerankResponse struct {
	Rankings []struct {
		Index int     `json:"index"`
		Logit float64 `json:"logit"`
	} `json:"rankings"`
}

// Rerank scores documents against the query using an NVIDIA NIM
// reranking model. Mirrors the Python NvidiaRerank class in
// rag/llm/rerank_model.py for payload shape (passages/query/logit).
// Defaults top_n to len(documents) so the API returns a score per
// input; callers may shrink it via RerankConfig.TopN, in which case
// only the top RerankConfig.TopN entries come back. Returned
// RerankResult entries are in the API's ranking order; callers that
// need original-input order should sort by Index. Same return-shape
// contract as the Aliyun and ZhipuAI Rerank drivers.
func (n *NvidiaModel) Rerank(ctx context.Context, modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	if err := n.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := n.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}

	baseURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(resolvedBaseURL, "/"), n.baseModel.URLSuffix.Rerank)

	topN := len(documents)
	if rerankConfig != nil && rerankConfig.TopN > 0 && rerankConfig.TopN < topN {
		topN = rerankConfig.TopN
	}

	passages := make([]nvidiaRerankText, len(documents))
	for i, doc := range documents {
		passages[i] = nvidiaRerankText{Text: doc}
	}

	reqBody := nvidiaRerankRequest{
		Model:    *modelName,
		Query:    nvidiaRerankText{Text: query},
		Passages: passages,
		Truncate: "END",
		TopN:     topN,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := n.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Nvidia rerank API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed nvidiaRerankResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	rerankResponse := RerankResponse{Data: make([]RerankResult, 0, len(parsed.Rankings))}
	for _, r := range parsed.Rankings {
		if r.Index < 0 || r.Index >= len(documents) {
			return nil, fmt.Errorf("unexpected rerank index %d for %d inputs", r.Index, len(documents))
		}
		rerankResponse.Data = append(rerankResponse.Data, RerankResult{
			Index:          r.Index,
			RelevanceScore: r.Logit,
		})
	}

	return &rerankResponse, nil
}

// TranscribeAudio transcribe audio
func (n *NvidiaModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}

func (n *NvidiaModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", n.Name())
}

// AudioSpeech convert text to audio
func (n *NvidiaModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}

func (n *NvidiaModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", n.Name())
}

// OCRFile OCR file
func (n *NvidiaModel) OCRFile(ctx context.Context, modelName *string, content []byte, baseURL *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}

// ParseFile parse file
func (n *NvidiaModel) ParseFile(ctx context.Context, modelName *string, content []byte, baseURL *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}

// ListModels calls /v1/models on the configured NVIDIA NIM base URL
// and returns the list of available model ids. The endpoint is
// OpenAI-compatible, so the parsing follows the same shape used by
// the moonshot, xai, and openai drivers.
func (n *NvidiaModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := n.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	resolvedBaseURL, err := n.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := fmt.Sprintf("%s/%s", strings.TrimRight(resolvedBaseURL, "/"), strings.TrimLeft(n.baseModel.URLSuffix.Models, "/"))

	modelListCtx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(modelListCtx, "GET", baseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	req.Header.Set("Accept", "application/json")

	resp, err := n.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Nvidia models API error: %s, body: %s", resp.Status, string(body))
	}

	// Parse response
	var modelList ModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if modelList.Models == nil {
		return nil, fmt.Errorf("invalid models list format")
	}

	var provider *Provider
	if pm := GetProviderManager(); pm != nil {
		provider = pm.FindProvider("NVIDIA")
	}
	models := parseNvidiaModelList(modelList, provider)
	if n.usesHostedCatalog(resolvedBaseURL) {
		catalogCtx, catalogCancel := context.WithTimeout(ctx, nonStreamCallTimeout)
		catalog, catalogErr := n.fetchHostedCatalog(catalogCtx)
		catalogCancel()
		if catalogErr != nil {
			return nil, catalogErr
		}
		models = filterNvidiaHostedModels(models, catalog, time.Now().UTC())
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("Nvidia models API returned no usable models")
	}
	return models, nil
}

type nvidiaCatalogLabel struct {
	Key              string   `json:"key"`
	Values           []string `json:"values"`
	UnresolvedValues []string `json:"unresolvedValues"`
}

type nvidiaCatalogAttribute struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type nvidiaCatalogResource struct {
	Name        string                   `json:"name"`
	DisplayName string                   `json:"displayName"`
	Labels      []nvidiaCatalogLabel     `json:"labels"`
	Attributes  []nvidiaCatalogAttribute `json:"attributes"`
}

type nvidiaCatalogGroup struct {
	GroupValue string                  `json:"groupValue"`
	TotalCount int                     `json:"totalCount"`
	Resources  []nvidiaCatalogResource `json:"resources"`
}

type nvidiaCatalogResponse struct {
	Results []nvidiaCatalogGroup `json:"results"`
}

func (n *NvidiaModel) usesHostedCatalog(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	return err == nil && strings.EqualFold(parsed.Hostname(), n.hostedAPIHost)
}

func (n *NvidiaModel) fetchHostedCatalog(ctx context.Context) (*nvidiaCatalogResponse, error) {
	parsed, err := url.Parse(n.catalogURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Nvidia endpoint catalog URL: %w", err)
	}

	var merged *nvidiaCatalogResponse
	endpointGroupIndex := -1
	totalCount := -1
	for page := 0; ; page++ {
		catalogQuery, err := json.Marshal(map[string]int{"page": page, "pageSize": nvidiaCatalogPageSize})
		if err != nil {
			return nil, fmt.Errorf("failed to encode Nvidia endpoint catalog query: %w", err)
		}
		pageURL := *parsed
		query := pageURL.Query()
		query.Set("q", string(catalogQuery))
		query.Set("group-labels-by-labelset", "true")
		pageURL.RawQuery = query.Encode()

		catalogPage, err := n.fetchHostedCatalogPage(ctx, pageURL.String())
		if err != nil {
			return nil, err
		}

		pageGroupIndex := -1
		for i := range catalogPage.Results {
			if catalogPage.Results[i].GroupValue == "ENDPOINT" {
				pageGroupIndex = i
				break
			}
		}
		if pageGroupIndex < 0 {
			return nil, fmt.Errorf("Nvidia endpoint catalog response is missing the ENDPOINT group")
		}

		pageGroup := catalogPage.Results[pageGroupIndex]
		if pageGroup.TotalCount < 0 {
			return nil, fmt.Errorf("Nvidia endpoint catalog returned invalid total count %d", pageGroup.TotalCount)
		}
		if merged == nil {
			merged = catalogPage
			endpointGroupIndex = pageGroupIndex
			totalCount = pageGroup.TotalCount
			merged.Results[endpointGroupIndex].Resources = nil
		} else if pageGroup.TotalCount != totalCount {
			return nil, fmt.Errorf("Nvidia endpoint catalog total count changed from %d to %d", totalCount, pageGroup.TotalCount)
		}

		merged.Results[endpointGroupIndex].Resources = append(merged.Results[endpointGroupIndex].Resources, pageGroup.Resources...)
		if len(pageGroup.Resources) < nvidiaCatalogPageSize {
			break
		}
	}

	if merged == nil || endpointGroupIndex < 0 {
		return nil, fmt.Errorf("Nvidia endpoint catalog returned no pages")
	}
	if len(merged.Results[endpointGroupIndex].Resources) < totalCount {
		return nil, fmt.Errorf(
			"Nvidia endpoint catalog returned %d of %d resources",
			len(merged.Results[endpointGroupIndex].Resources),
			totalCount,
		)
	}
	return merged, nil
}

func (n *NvidiaModel) fetchHostedCatalogPage(ctx context.Context, pageURL string) (*nvidiaCatalogResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Nvidia endpoint catalog request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := n.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request Nvidia endpoint catalog: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Nvidia endpoint catalog: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Nvidia endpoint catalog error: %s, body: %s", resp.Status, string(body))
	}

	var catalog nvidiaCatalogResponse
	if err = json.Unmarshal(body, &catalog); err != nil {
		return nil, fmt.Errorf("failed to parse Nvidia endpoint catalog: %w", err)
	}
	return &catalog, nil
}

func filterNvidiaHostedModels(models []ListModelResponse, catalog *nvidiaCatalogResponse, now time.Time) []ListModelResponse {
	if catalog == nil {
		return nil
	}

	var resources []nvidiaCatalogResource
	for _, group := range catalog.Results {
		if group.GroupValue != "ENDPOINT" {
			continue
		}
		if group.TotalCount < 0 || len(group.Resources) < group.TotalCount {
			return nil
		}
		resources = group.Resources
		break
	}
	if resources == nil {
		return nil
	}

	activeEndpoints := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		if !nvidiaCatalogResourceIsActive(resource, now) {
			continue
		}
		publishers := nvidiaCatalogLabelValues(resource, "publisher", true)
		displayName := resource.DisplayName
		if displayName == "" {
			displayName = resource.Name
		}
		if len(publishers) == 0 || displayName == "" {
			continue
		}
		activeEndpoints[nvidiaCatalogKey(publishers[0], displayName)] = struct{}{}
	}

	filtered := make([]ListModelResponse, 0, len(models))
	for _, model := range models {
		publisher, modelName, found := strings.Cut(model.Name, "/")
		if !found {
			continue
		}
		if _, ok := activeEndpoints[nvidiaCatalogKey(publisher, modelName)]; ok {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func nvidiaCatalogResourceIsActive(resource nvidiaCatalogResource, now time.Time) bool {
	if !containsNvidiaCatalogValue(nvidiaCatalogLabelValues(resource, "nimType", false), "Free Endpoint") {
		return false
	}
	deprecation := ""
	for _, attribute := range resource.Attributes {
		if attribute.Key == "DEPRECATION" {
			deprecation = attribute.Value
			break
		}
	}
	if deprecation == "" {
		return true
	}
	cutoff, err := time.Parse("01/02/2006", deprecation)
	if err != nil {
		return false
	}
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return cutoff.After(today)
}

func nvidiaCatalogLabelValues(resource nvidiaCatalogResource, key string, unresolved bool) []string {
	for _, label := range resource.Labels {
		if label.Key != key {
			continue
		}
		if unresolved {
			return label.UnresolvedValues
		}
		return label.Values
	}
	return nil
}

func nvidiaCatalogKey(publisher, modelName string) string {
	publisher = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(publisher)), "_", "-")
	return publisher + "\x00" + modelName
}

func containsNvidiaCatalogValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func parseNvidiaModelList(modelList ModelList, provider *Provider) []ListModelResponse {
	const defaultMaxTokens = 8192

	models := make([]ListModelResponse, 0, len(modelList.Models))
	seen := make(map[string]struct{}, len(modelList.Models))
	for _, item := range modelList.Models {
		modelName := strings.TrimSpace(item.ID)
		if modelName == "" {
			continue
		}
		if _, ok := seen[modelName]; ok {
			continue
		}
		seen[modelName] = struct{}{}

		response := ListModelResponse{Name: modelName}
		var preset *Model
		if provider != nil {
			for _, model := range provider.Models {
				if strings.EqualFold(model.Name, modelName) {
					preset = model
					break
				}
			}
		}
		if preset != nil {
			response.MaxOutput = preset.MaxOutput
			response.ModelTypes = append([]string(nil), preset.ModelTypes...)
			response.Thinking = preset.Thinking
			response.MaxDimension = preset.MaxDimension
			response.MaxBatchSize = preset.MaxBatchSize
			response.Dimensions = append([]int(nil), preset.Dimensions...)
		} else {
			maxTokens := defaultMaxTokens
			response.MaxOutput = &maxTokens
			response.ModelTypes = InferModelTypes(modelName)
		}
		if len(response.ModelTypes) == 0 {
			response.ModelTypes = InferModelTypes(modelName)
		}

		models = append(models, response)
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})
	return models
}

func (n *NvidiaModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

// CheckConnection verifies that the configured NVIDIA NIM base URL
// is reachable and that the API key is accepted, by issuing a
// lightweight ListModels call. Mirrors the pattern used by the xai,
// moonshot, deepseek, aliyun, and gitee drivers.
func (n *NvidiaModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := n.ListModels(ctx, apiConfig)
	return err
}

func (n *NvidiaModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}

func (n *NvidiaModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}
