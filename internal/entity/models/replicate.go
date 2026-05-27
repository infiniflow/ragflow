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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const replicatePollInterval = time.Second

type ReplicateModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

func NewReplicateModel(baseURL map[string]string, urlSuffix URLSuffix) *ReplicateModel {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.DisableCompression = false
	transport.ResponseHeaderTimeout = 60 * time.Second

	return &ReplicateModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

func (r *ReplicateModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewReplicateModel(baseURL, r.URLSuffix)
}

func (r *ReplicateModel) Name() string {
	return "replicate"
}

type replicatePredictionURLs struct {
	Get    string `json:"get"`
	Stream string `json:"stream"`
}

type replicatePrediction struct {
	ID     string                  `json:"id"`
	Status string                  `json:"status"`
	Output interface{}             `json:"output"`
	Error  interface{}             `json:"error"`
	URLs   replicatePredictionURLs `json:"urls"`
}

type replicateModelsResponse struct {
	Results []struct {
		Owner string `json:"owner"`
		Name  string `json:"name"`
	} `json:"results"`
}

type replicateSSEEvent struct {
	event string
	data  string
}

func (r *ReplicateModel) baseURLForRegion(region string) (string, error) {
	base, ok := r.BaseURL[region]
	if !ok || base == "" {
		return "", fmt.Errorf("replicate: no base URL configured for region %q", region)
	}
	return strings.TrimSuffix(base, "/"), nil
}

func (r *ReplicateModel) endpoint(apiConfig *APIConfig, suffix string) (string, error) {
	region := "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := r.baseURLForRegion(region)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s", baseURL, suffix), nil
}

func replicateUsesVersionEndpoint(modelName string) bool {
	name := strings.TrimSpace(modelName)
	return !strings.Contains(name, "/") || strings.Contains(name, ":")
}

func (r *ReplicateModel) predictionEndpoint(apiConfig *APIConfig, modelName string) (string, string, error) {
	if replicateUsesVersionEndpoint(modelName) {
		endpoint, err := r.endpoint(apiConfig, r.URLSuffix.Chat)
		return endpoint, modelName, err
	}

	parts := strings.Split(modelName, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("replicate: official model name must be owner/name")
	}

	modelsPrefix := strings.TrimSuffix(r.URLSuffix.Models, "models")
	if modelsPrefix == "" {
		modelsPrefix = "v1/"
	}
	officialSuffix := fmt.Sprintf("%smodels/%s/%s/predictions",
		modelsPrefix,
		url.PathEscape(parts[0]),
		url.PathEscape(parts[1]),
	)
	endpoint, err := r.endpoint(apiConfig, officialSuffix)
	return endpoint, "", err
}

func replicateMessageContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(b)
	}
}

func replicatePromptFromMessages(messages []Message) (string, string) {
	var systemParts []string
	var promptParts []string
	nonSystemCount := 0
	for _, msg := range messages {
		content := replicateMessageContent(msg.Content)
		if msg.Role == "system" {
			systemParts = append(systemParts, content)
			continue
		}
		nonSystemCount++
		if nonSystemCount == 1 && msg.Role == "user" && len(messages) == len(systemParts)+1 {
			promptParts = append(promptParts, content)
			continue
		}
		promptParts = append(promptParts, fmt.Sprintf("%s: %s", msg.Role, content))
	}
	return strings.Join(promptParts, "\n"), strings.Join(systemParts, "\n\n")
}

func replicateInputFromMessages(messages []Message, chatModelConfig *ChatConfig) map[string]interface{} {
	prompt, systemPrompt := replicatePromptFromMessages(messages)
	input := map[string]interface{}{
		"prompt": prompt,
	}
	if systemPrompt != "" {
		input["system_prompt"] = systemPrompt
	}
	if chatModelConfig != nil {
		if chatModelConfig.MaxTokens != nil {
			input["max_new_tokens"] = *chatModelConfig.MaxTokens
		}
		if chatModelConfig.Temperature != nil {
			input["temperature"] = *chatModelConfig.Temperature
		}
		if chatModelConfig.TopP != nil {
			input["top_p"] = *chatModelConfig.TopP
		}
		// Replicate model inputs are model-specific. Forward only the
		// common prompt-model fields above; Stop is intentionally
		// omitted because upstream behavior is undefined for many
		// hosted models.
	}
	return input
}

func replicateOutputToString(output interface{}) (string, error) {
	switch v := output.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case []interface{}:
		var b strings.Builder
		for _, item := range v {
			text, err := replicateOutputToString(item)
			if err != nil {
				return "", err
			}
			b.WriteString(text)
		}
		return b.String(), nil
	case map[string]interface{}:
		raw, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	default:
		return fmt.Sprint(v), nil
	}
}

func (r *ReplicateModel) createPrediction(ctx context.Context, url string, version string, input map[string]interface{}, stream bool, apiKey string, preferWait bool) (*replicatePrediction, error) {
	body := map[string]interface{}{
		"input":  input,
		"stream": stream,
	}
	if version != "" {
		body["version"] = version
	}

	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	if preferWait {
		req.Header.Set("Prefer", "wait=60")
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var prediction replicatePrediction
	if err = json.Unmarshal(bodyBytes, &prediction); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if prediction.Error != nil {
		return nil, fmt.Errorf("replicate: upstream error: %v", prediction.Error)
	}
	return &prediction, nil
}

func replicatePredictionDone(status string) bool {
	return replicatePredictionSucceeded(status) || status == "failed" || status == "canceled"
}

func replicatePredictionSucceeded(status string) bool {
	return status == "successful"
}

func (r *ReplicateModel) getPrediction(ctx context.Context, url string, apiKey string) (*replicatePrediction, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var prediction replicatePrediction
	if err = json.Unmarshal(body, &prediction); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if prediction.Error != nil {
		return nil, fmt.Errorf("replicate: upstream error: %v", prediction.Error)
	}
	return &prediction, nil
}

func (r *ReplicateModel) waitForPrediction(ctx context.Context, prediction *replicatePrediction, apiKey string) (*replicatePrediction, error) {
	if prediction == nil {
		return nil, fmt.Errorf("replicate: empty prediction response")
	}
	if replicatePredictionDone(prediction.Status) {
		return prediction, nil
	}
	if prediction.URLs.Get == "" {
		return nil, fmt.Errorf("replicate: prediction is %q and no polling URL was returned", prediction.Status)
	}

	ticker := time.NewTicker(replicatePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("replicate: prediction did not finish before timeout: %w", ctx.Err())
		case <-ticker.C:
			next, err := r.getPrediction(ctx, prediction.URLs.Get, apiKey)
			if err != nil {
				return nil, err
			}
			if replicatePredictionDone(next.Status) {
				return next, nil
			}
		}
	}
}

func (r *ReplicateModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	url, version, err := r.predictionEndpoint(apiConfig, modelName)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	prediction, err := r.createPrediction(ctx, url, version, replicateInputFromMessages(messages, chatModelConfig), false, *apiConfig.ApiKey, true)
	if err != nil {
		return nil, err
	}
	prediction, err = r.waitForPrediction(ctx, prediction, *apiConfig.ApiKey)
	if err != nil {
		return nil, err
	}
	if !replicatePredictionSucceeded(prediction.Status) {
		return nil, fmt.Errorf("replicate: prediction ended with status %q", prediction.Status)
	}

	answer, err := replicateOutputToString(prediction.Output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse prediction output: %w", err)
	}
	reasonContent := ""
	return &ChatResponse{Answer: &answer, ReasonContent: &reasonContent}, nil
}

func (r *ReplicateModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return fmt.Errorf("api key is required")
	}
	if strings.TrimSpace(modelName) == "" {
		return fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if chatModelConfig != nil && chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
		return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
	}

	url, version, err := r.predictionEndpoint(apiConfig, modelName)
	if err != nil {
		return err
	}

	prediction, err := r.createPrediction(context.Background(), url, version, replicateInputFromMessages(messages, chatModelConfig), true, *apiConfig.ApiKey, false)
	if err != nil {
		return err
	}
	if prediction.URLs.Stream == "" {
		ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
		defer cancel()
		prediction, err = r.waitForPrediction(ctx, prediction, *apiConfig.ApiKey)
		if err != nil {
			return err
		}
		answer, err := replicateOutputToString(prediction.Output)
		if err != nil {
			return fmt.Errorf("failed to parse prediction output: %w", err)
		}
		if answer != "" {
			if err := sender(&answer, nil); err != nil {
				return err
			}
		}
		endOfStream := "[DONE]"
		return sender(&endOfStream, nil)
	}

	return r.readPredictionStream(prediction.URLs.Stream, *apiConfig.ApiKey, sender)
}

func (r *ReplicateModel) readPredictionStream(url string, apiKey string, sender func(*string, *string) error) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Accept", "text/event-stream")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	current := replicateSSEEvent{}
	sawDone := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			done, err := dispatchReplicateSSEEvent(current, sender)
			if err != nil {
				return err
			}
			if done {
				sawDone = true
				break
			}
			current = replicateSSEEvent{}
			continue
		}
		if strings.HasPrefix(line, "event:") {
			current.event = strings.TrimSpace(line[6:])
		}
		if strings.HasPrefix(line, "data:") {
			if current.data != "" {
				current.data += "\n"
			}
			data := line[5:]
			if strings.HasPrefix(data, " ") {
				data = data[1:]
			}
			current.data += data
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !sawDone && (current.event != "" || current.data != "") {
		done, err := dispatchReplicateSSEEvent(current, sender)
		if err != nil {
			return err
		}
		sawDone = done
	}
	if !sawDone {
		return fmt.Errorf("replicate: stream ended before done event")
	}

	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

func dispatchReplicateSSEEvent(event replicateSSEEvent, sender func(*string, *string) error) (bool, error) {
	switch event.event {
	case "output", "":
		if event.data == "" {
			return false, nil
		}
		return false, sender(&event.data, nil)
	case "error":
		return false, fmt.Errorf("replicate: upstream stream error: %s", event.data)
	case "done":
		return true, nil
	default:
		return false, nil
	}
}

func (r *ReplicateModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}

	url, err := r.endpoint(apiConfig, r.URLSuffix.Models)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result replicateModelsResponse
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	models := make([]string, 0, len(result.Results))
	for _, model := range result.Results {
		if model.Owner != "" && model.Name != "" {
			models = append(models, fmt.Sprintf("%s/%s", model.Owner, model.Name))
		}
	}
	return models, nil
}

func (r *ReplicateModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := r.ListModels(apiConfig)
	return err
}

// replicateEmbedInput shapes the request body for Replicate's standard
// embedding models (e.g. replicate/all-mpnet-base-v2). Per the
// canonical Replicate embedding schema published in the model's
// openapi_schema, the two input fields are:
//
//	text       — single string to encode (used when len(texts) == 1)
//	text_batch — JSON-formatted list of strings (used when len > 1)
//
// `text_batch` is `type: string` in the schema, so the JSON-encoded
// list itself is sent as a string value, NOT as a JSON array. Models
// that use different field names (e.g. nateraw/bge-large-en-v1.5's
// `texts`) are not currently supported by this driver; tenants on
// those should consult Replicate's OpenAPI schema and configure a
// compatible model in conf/models/replicate.json.
func replicateEmbedInput(texts []string) (map[string]interface{}, error) {
	switch len(texts) {
	case 0:
		return nil, fmt.Errorf("replicate: texts is empty")
	case 1:
		return map[string]interface{}{"text": texts[0]}, nil
	default:
		encoded, err := json.Marshal(texts)
		if err != nil {
			return nil, fmt.Errorf("failed to encode text_batch: %w", err)
		}
		return map[string]interface{}{"text_batch": string(encoded)}, nil
	}
}

// replicateEmbedOutputToVectors normalizes Replicate's two observed
// embedding-output shapes into []EmbeddingData aligned with the
// caller's input order:
//
//	[]{embedding: [floats]}   — the documented Embedding schema used
//	                            by replicate/all-mpnet-base-v2
//	[][floats]                — bare nested array used by some
//	                            community models
//
// The driver rejects mismatched cardinality (output length != input
// length) and non-numeric vector entries rather than silently
// truncate or pad, matching the defensive posture the n1n / CometAPI
// drivers already use.
func replicateEmbedOutputToVectors(output interface{}, n int) ([]EmbeddingData, error) {
	outputs, ok := output.([]interface{})
	if !ok {
		return nil, fmt.Errorf("replicate: expected output to be an array, got %T", output)
	}
	if len(outputs) != n {
		return nil, fmt.Errorf("replicate: expected %d embeddings, got %d", n, len(outputs))
	}

	vectors := make([]EmbeddingData, n)
	for i, item := range outputs {
		vec, err := replicateExtractEmbeddingVector(item)
		if err != nil {
			return nil, fmt.Errorf("replicate: output[%d]: %w", i, err)
		}
		vectors[i] = EmbeddingData{Embedding: vec, Index: i}
	}
	return vectors, nil
}

func replicateExtractEmbeddingVector(item interface{}) ([]float64, error) {
	switch v := item.(type) {
	case []interface{}:
		return replicateFloatsFromInterface(v)
	case map[string]interface{}:
		raw, ok := v["embedding"]
		if !ok {
			return nil, fmt.Errorf("missing 'embedding' field; got keys %v", replicateKeys(v))
		}
		arr, ok := raw.([]interface{})
		if !ok {
			return nil, fmt.Errorf("embedding field is %T, expected array", raw)
		}
		return replicateFloatsFromInterface(arr)
	default:
		return nil, fmt.Errorf("unsupported item type %T", item)
	}
}

func replicateFloatsFromInterface(arr []interface{}) ([]float64, error) {
	floats := make([]float64, len(arr))
	for i, v := range arr {
		f, ok := v.(float64)
		if !ok {
			return nil, fmt.Errorf("element %d is %T, expected number", i, v)
		}
		floats[i] = f
	}
	return floats, nil
}

func replicateKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Embed turns a list of texts into embedding vectors via Replicate's
// prediction API. Replicate's embedding surface is the same async
// /v1/predictions or /v1/models/{owner}/{name}/predictions endpoint
// the chat path already uses: create a prediction with `Prefer: wait`
// to skip the polling round-trip when possible, fall back to
// waitForPrediction for predictions that don't finish in the wait
// window. The driver targets the canonical Replicate embedding
// schema (input.text / input.text_batch, output is an array of
// {embedding: [floats]} objects); see replicateEmbedInput and
// replicateEmbedOutputToVectors for details.
func (r *ReplicateModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}

	url, version, err := r.predictionEndpoint(apiConfig, *modelName)
	if err != nil {
		return nil, err
	}

	input, err := replicateEmbedInput(texts)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	prediction, err := r.createPrediction(ctx, url, version, input, false, *apiConfig.ApiKey, true)
	if err != nil {
		return nil, err
	}
	prediction, err = r.waitForPrediction(ctx, prediction, *apiConfig.ApiKey)
	if err != nil {
		return nil, err
	}
	if !replicatePredictionSucceeded(prediction.Status) {
		return nil, fmt.Errorf("replicate: prediction ended with status %q", prediction.Status)
	}

	return replicateEmbedOutputToVectors(prediction.Output, len(texts))
}

// replicateRerankInput shapes the request body for Replicate's
// canonical bge-style reranker schema. The documented input is a
// single string field `input_list` carrying a JSON-encoded list of
// `[query, passage]` pairs; the model returns a flat list of
// numeric scores, one per pair, in the same order.
//
// See yxzwayne/bge-reranker-v2-m3's openapi_schema + default_example
// at https://replicate.com/yxzwayne/bge-reranker-v2-m3. Other
// reranker models on Replicate (sesamo-srl/bge-reranker-v2-m3,
// ninehills/bge-reranker-large) follow compatible
// pair-list-in-string conventions; this driver targets the
// canonical shape and leaves model-specific adapters for future
// PRs if other schemas are needed.
func replicateRerankInput(query string, documents []string) (map[string]interface{}, error) {
	if len(documents) == 0 {
		return nil, fmt.Errorf("replicate: documents is empty")
	}
	pairs := make([][2]string, len(documents))
	for i, doc := range documents {
		pairs[i] = [2]string{query, doc}
	}
	encoded, err := json.Marshal(pairs)
	if err != nil {
		return nil, fmt.Errorf("failed to encode input_list: %w", err)
	}
	return map[string]interface{}{"input_list": string(encoded)}, nil
}

// replicateRerankOutputToScores normalizes Replicate's two observed
// rerank-output shapes into a []float64 aligned with the caller's
// document order:
//
//	[]float64        — flat scores array, used by
//	                   yxzwayne/bge-reranker-v2-m3 (canonical)
//	{ "scores": [..] } — wrapped object, used by ninehills/bge-reranker-large
//
// Rejects mismatched cardinality and non-numeric scores rather than
// silently truncate, matching the defensive posture the Embed
// implementation already uses.
func replicateRerankOutputToScores(output interface{}, n int) ([]float64, error) {
	if scores, ok := output.([]interface{}); ok {
		return replicateScoresFromInterface(scores, n)
	}
	if obj, ok := output.(map[string]interface{}); ok {
		raw, present := obj["scores"]
		if !present {
			return nil, fmt.Errorf("replicate: rerank output missing 'scores' field; got keys %v", replicateKeys(obj))
		}
		arr, ok := raw.([]interface{})
		if !ok {
			return nil, fmt.Errorf("replicate: rerank output.scores is %T, expected array", raw)
		}
		return replicateScoresFromInterface(arr, n)
	}
	return nil, fmt.Errorf("replicate: expected rerank output to be an array or object, got %T", output)
}

func replicateScoresFromInterface(arr []interface{}, n int) ([]float64, error) {
	if len(arr) != n {
		return nil, fmt.Errorf("replicate: expected %d rerank scores, got %d", n, len(arr))
	}
	out := make([]float64, n)
	for i, v := range arr {
		f, ok := v.(float64)
		if !ok {
			return nil, fmt.Errorf("replicate: rerank score %d is %T, expected number", i, v)
		}
		out[i] = f
	}
	return out, nil
}

// Rerank scores a query against a list of documents via Replicate's
// prediction API. The driver targets bge-reranker-v2-m3-style models
// (the most widely-published rerank schema on Replicate) and reuses
// the existing createPrediction + waitForPrediction plumbing from
// the chat and embed paths.
//
// Replicate rerank model outputs are raw similarity scores — they
// are NOT normalized to [0, 1] like Cohere or Voyage rerank
// responses. Higher scores still indicate stronger relevance; the
// driver passes the raw value through without rescaling so callers
// can compare against per-model thresholds, but the RelevanceScore
// field should not be assumed to be a probability.
func (r *ReplicateModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}

	url, version, err := r.predictionEndpoint(apiConfig, *modelName)
	if err != nil {
		return nil, err
	}

	input, err := replicateRerankInput(query, documents)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	prediction, err := r.createPrediction(ctx, url, version, input, false, *apiConfig.ApiKey, true)
	if err != nil {
		return nil, err
	}
	prediction, err = r.waitForPrediction(ctx, prediction, *apiConfig.ApiKey)
	if err != nil {
		return nil, err
	}
	if !replicatePredictionSucceeded(prediction.Status) {
		return nil, fmt.Errorf("replicate: prediction ended with status %q", prediction.Status)
	}

	scores, err := replicateRerankOutputToScores(prediction.Output, len(documents))
	if err != nil {
		return nil, err
	}

	// Build the canonical RerankResponse with one entry per input
	// document. Optional top_n trimming sorts by score descending
	// and keeps the highest-ranking documents; otherwise return all
	// scores in original document order, matching how Voyage's
	// driver in this package surfaces its results.
	topN := len(documents)
	if rerankConfig != nil && rerankConfig.TopN > 0 && rerankConfig.TopN < topN {
		topN = rerankConfig.TopN
	}
	results := make([]RerankResult, len(documents))
	for i, score := range scores {
		results[i] = RerankResult{Index: i, RelevanceScore: score}
	}
	if topN < len(results) {
		// Sort by score descending, stable on index to keep deterministic
		// ordering for ties.
		sort.SliceStable(results, func(a, b int) bool {
			if results[a].RelevanceScore == results[b].RelevanceScore {
				return results[a].Index < results[b].Index
			}
			return results[a].RelevanceScore > results[b].RelevanceScore
		})
		results = results[:topN]
	}
	return &RerankResponse{Data: results}, nil
}


func (r *ReplicateModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
}

func (r *ReplicateModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
}

func (r *ReplicateModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", r.Name())
}

func (r *ReplicateModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
}

func (r *ReplicateModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", r.Name())
}

func (r *ReplicateModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
}

func (r *ReplicateModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
}

func (r *ReplicateModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
}

func (r *ReplicateModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
}
