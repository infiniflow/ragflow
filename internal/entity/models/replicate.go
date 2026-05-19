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

func (r *ReplicateModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
}

func (r *ReplicateModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
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
