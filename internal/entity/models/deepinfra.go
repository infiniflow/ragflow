package models

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"ragflow/internal/common"
	"slices"
	"strconv"
	"strings"
	"time"
)

type DeepInfraModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

func NewDeepInfraModel(baseURL map[string]string, urlSuffix URLSuffix) *DeepInfraModel {
	return &DeepInfraModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Timeout: time.Second * 120,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     time.Second * 90,
				DisableCompression:  false,
			},
		},
	}
}

func (d *DeepInfraModel) NewInstance(baseURL map[string]string) ModelDriver {
	return &DeepInfraModel{
		BaseURL:   baseURL,
		URLSuffix: d.URLSuffix,
		httpClient: &http.Client{
			Timeout: time.Second * 120,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     time.Second * 90,
				DisableCompression:  false,
			},
		},
	}
}

func (d *DeepInfraModel) Name() string {
	return "deepinfra"
}

func (d *DeepInfraModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", d.BaseURL[region], d.URLSuffix.Chat)

	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	reqBody := map[string]interface{}{
		"model":    modelName,
		"messages": apiMessages,
		"stream":   false,
	}

	if chatModelConfig != nil {
		if chatModelConfig.Stream != nil {
			reqBody["stream"] = *chatModelConfig.Stream
		}

		if chatModelConfig.MaxTokens != nil {
			reqBody["max_tokens"] = *chatModelConfig.MaxTokens
		}

		if chatModelConfig.Temperature != nil {
			reqBody["temperature"] = *chatModelConfig.Temperature
		}

		if chatModelConfig.TopP != nil {
			reqBody["top_p"] = *chatModelConfig.TopP
		}

		if chatModelConfig.Stop != nil {
			reqBody["stop"] = *chatModelConfig.Stop
		}

		if chatModelConfig.Effort != nil {
			reqBody["reasoning_effort"] = *chatModelConfig.Effort
		}

		if chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
			reasoningMap := map[string]interface{}{
				"enabled": true,
			}
			if chatModelConfig.Effort != nil {
				reasoningMap["effort"] = *chatModelConfig.Effort
			}
			reqBody["reasoning"] = reasoningMap
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse result
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	firstChoice, ok := choices[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid choice format")
	}

	messageMap, ok := firstChoice["message"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid message format")
	}

	content, ok := messageMap["content"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid content format")
	}

	var reasonContent string
	if rc, ok := messageMap["reasoning_content"].(string); ok {
		reasonContent = rc
	}

	chatResponse := &ChatResponse{
		Answer: &content,
	}
	if reasonContent != "" {
		chatResponse.ReasonContent = &reasonContent
	}

	return chatResponse, nil
}

func (d *DeepInfraModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", d.BaseURL[region], d.URLSuffix.Chat)

	// Convert messages to API format
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// Build request body with streaming enabled
	reqBody := map[string]interface{}{
		"model":       modelName,
		"messages":    apiMessages,
		"stream":      true,
		"temperature": 1,
	}

	if modelConfig != nil {
		if modelConfig.Stream != nil {
			reqBody["stream"] = *modelConfig.Stream
		}

		if modelConfig.MaxTokens != nil {
			reqBody["max_tokens"] = *modelConfig.MaxTokens
		}

		if modelConfig.Temperature != nil {
			reqBody["temperature"] = *modelConfig.Temperature
		}

		if modelConfig.DoSample != nil {
			reqBody["do_sample"] = *modelConfig.DoSample
		}

		if modelConfig.TopP != nil {
			reqBody["top_p"] = *modelConfig.TopP
		}

		if modelConfig.Stop != nil {
			reqBody["stop"] = *modelConfig.Stop
		}

		if modelConfig.Effort != nil {
			reqBody["reasoning_effort"] = *modelConfig.Effort
		}

		if modelConfig.Thinking != nil && *modelConfig.Thinking {
			reasoningMap := map[string]interface{}{
				"enabled": true,
			}
			if modelConfig.Effort != nil {
				reasoningMap["effort"] = *modelConfig.Effort
			}
			reqBody["reasoning"] = reasoningMap
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// SSE parsing: read line by line
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		common.Info(line)

		// SSE data line starts with "data:"
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		// Extract JSON after "data:"
		data := strings.TrimSpace(line[5:])

		// [DONE] marks the end of stream
		if data == "[DONE]" {
			break
		}

		// Parse the JSON event
		var event map[string]interface{}
		if err = json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		choices, ok := event["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			continue
		}

		firstChoice, ok := choices[0].(map[string]interface{})
		if !ok {
			continue
		}

		delta, ok := firstChoice["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		reasoningContent, ok := delta["reasoning_content"].(string)
		if ok && reasoningContent != "" {
			if err := sender(nil, &reasoningContent); err != nil {
				return err
			}
		}

		content, ok := delta["content"].(string)
		if ok && content != "" {
			if err := sender(&content, nil); err != nil {
				return err
			}
		}

		finishReason, ok := firstChoice["finish_reason"].(string)
		if ok && finishReason != "" {
			break
		}
	}

	// Send [DONE] marker for OpenAI compatibility
	endOfStream := "[DONE]"
	if err = sender(&endOfStream, nil); err != nil {
		return err
	}

	return scanner.Err()
}

func (d *DeepInfraModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if len(texts) == 0 {
		return []EmbeddingData{}, fmt.Errorf("texts is empty")
	}

	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", d.BaseURL[region], d.URLSuffix.Embedding)

	reqBody := map[string]interface{}{
		"model": *modelName,
		"input": texts,
	}

	if embeddingConfig != nil && embeddingConfig.Dimension >= 32 {
		reqBody["dimensions"] = embeddingConfig.Dimension
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DeepInfra embedding API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}

	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// 组装 RAGFlow 需要的返回格式
	var embeddings []EmbeddingData
	for _, data := range parsed.Data {
		embeddings = append(embeddings, EmbeddingData{
			Embedding: data.Embedding,
			Index:     data.Index,
		})
	}

	return embeddings, nil
}

// deepinfraRerankResponse is the JSON body returned by DeepInfra reranker models.
type deepinfraRerankResponse struct {
	Scores []float64 `json:"scores"`
}

// Rerank scores documents against a query using DeepInfra's inference endpoint.
// The model id is part of the URL path (e.g. Qwen/Qwen3-Reranker-4B). The API
// returns one score per input document; RerankConfig.TopN is enforced client-side
// by keeping the highest-scoring entries when TopN is less than len(documents).
func (d *DeepInfraModel) Rerank(
	modelName *string,
	query string,
	documents []string,
	apiConfig *APIConfig,
	rerankConfig *RerankConfig,
) (*RerankResponse, error) {
	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}
	baseURL := d.BaseURL[region]
	if baseURL == "" {
		return nil, fmt.Errorf("deepinfra: no base URL configured for region %q", region)
	}

	// Reranker model ids may contain slashes (e.g. Qwen/Qwen3-Reranker-4B).
	url := fmt.Sprintf("%s/%s/%s", strings.TrimSuffix(baseURL, "/"), d.URLSuffix.Rerank, *modelName)

	reqBody := map[string]interface{}{
		"query":     query,
		"documents": documents,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DeepInfra rerank API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var parsed deepinfraRerankResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if len(parsed.Scores) != len(documents) {
		return nil, fmt.Errorf("deepinfra: expected %d scores, got %d", len(documents), len(parsed.Scores))
	}

	results := make([]RerankResult, len(parsed.Scores))
	for i, score := range parsed.Scores {
		results[i] = RerankResult{
			Index:          i,
			RelevanceScore: score,
		}
	}

	topN := len(results)
	if rerankConfig != nil && rerankConfig.TopN > 0 && rerankConfig.TopN < topN {
		topN = rerankConfig.TopN
		slices.SortFunc(results, func(a, b RerankResult) int {
			if a.RelevanceScore > b.RelevanceScore {
				return -1
			}
			if a.RelevanceScore < b.RelevanceScore {
				return 1
			}
			return 0
		})
		results = results[:topN]
	}

	return &RerankResponse{Data: results}, nil
}

func (d *DeepInfraModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("DeepInfra API key is missing")
	}

	if file == nil || *file == "" {
		return nil, fmt.Errorf("file is missing")
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is missing")
	}

	var region = "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", d.BaseURL[region], d.URLSuffix.ASR)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("model", *modelName); err != nil {
		return nil, fmt.Errorf("failed to write model field: %w", err)
	}

	// Open File
	audioFile, err := os.Open(*file)
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer audioFile.Close()

	part, err := writer.CreateFormFile("file", filepath.Base(*file))
	if err != nil {
		return nil, fmt.Errorf("failed to create multipart file: %w", err)
	}

	if _, err = io.Copy(part, audioFile); err != nil {
		return nil, fmt.Errorf("failed to copy audio data: %w", err)
	}

	// get config
	if asrConfig != nil && asrConfig.Params != nil {
		for key, value := range asrConfig.Params {
			var val string

			switch v := value.(type) {
			case string:
				val = v
			case bool:
				val = strconv.FormatBool(v)
			case int:
				val = strconv.Itoa(v)
			case int64:
				val = strconv.FormatInt(v, 10)
			case float64:
				val = strconv.FormatFloat(v, 'f', -1, 64)
			case float32:
				val = strconv.FormatFloat(float64(v), 'f', -1, 32)
			default:
				val = fmt.Sprintf("%v", v)
			}

			if err := writer.WriteField(key, val); err != nil {
				return nil, fmt.Errorf("failed to write field %s: %w", key, err)
			}
		}
	}

	if err = writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequest("POST", url, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DeepInfra ASR error: %s - %s", resp.Status, string(respBody))
	}

	// Parse result
	var result struct {
		Text string `json:"text"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &ASRResponse{
		Text: result.Text,
	}, nil
}

func (d *DeepInfraModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s no such method", d.Name())
}

func (d *DeepInfraModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("DeepInfra API key is missing")
	}

	if audioContent == nil || *audioContent == "" {
		return nil, fmt.Errorf("text content is missing")
	}

	var region = "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	reqBody := map[string]interface{}{
		"text": *audioContent,
	}
	voiceID := ""

	if ttsConfig != nil && ttsConfig.Params != nil {
		if v, ok := ttsConfig.Params["voice_id"].(string); ok && v != "" {
			voiceID = v
		} else if v, ok := ttsConfig.Params["voice"].(string); ok && v != "" {
			voiceID = v
		}

		for key, value := range ttsConfig.Params {
			if key != "voice_id" && key != "voice" {
				reqBody[key] = value
			}
		}
	}

	if voiceID == "" {
		return nil, fmt.Errorf("voice_id is missing (must be provided in params or model name)")
	}

	// URL: https://api.deepinfra.com/v1/text-to-speech/{voice_id}
	url := fmt.Sprintf("%s/%s/%s", d.BaseURL[region], d.URLSuffix.TTS, voiceID)

	if ttsConfig != nil && ttsConfig.Format != "" {
		reqBody["output_format"] = ttsConfig.Format
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DeepInfra TTS error: status %d - %s", resp.StatusCode, string(body))
	}

	return &TTSResponse{Audio: body}, nil
}

func (d *DeepInfraModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return fmt.Errorf("DeepInfra API key is missing")
	}

	if audioContent == nil || *audioContent == "" {
		return fmt.Errorf("text content is missing")
	}

	var region = "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	voiceID := ""

	reqBody := map[string]interface{}{
		"text": *audioContent,
	}

	if ttsConfig != nil && ttsConfig.Params != nil {
		if v, ok := ttsConfig.Params["voice_id"].(string); ok && v != "" {
			voiceID = v
		} else if v, ok := ttsConfig.Params["voice"].(string); ok && v != "" {
			voiceID = v
		}

		for key, value := range ttsConfig.Params {
			if key != "voice_id" && key != "voice" {
				reqBody[key] = value
			}
		}
	}

	if voiceID == "" {
		return fmt.Errorf("voice_id is missing")
	}

	// URL: https://api.deepinfra.com/v1/text-to-speech/{voice_id}/stream
	url := fmt.Sprintf("%s/%s/%s/stream", d.BaseURL[region], d.URLSuffix.TTS, voiceID)

	if ttsConfig != nil && ttsConfig.Format != "" {
		reqBody["output_format"] = ttsConfig.Format
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DeepInfra TTS Stream error: status %d - %s", resp.StatusCode, string(body))
	}

	buffer := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buffer)

		if n > 0 {
			chunkStr := string(buffer[:n])
			if sendErr := sender(&chunkStr, nil); sendErr != nil {
				return sendErr
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading stream: %w", err)
		}
	}

	endOfStream := "[DONE]"
	if err = sender(&endOfStream, nil); err != nil {
		return err
	}

	return nil
}

func (d *DeepInfraModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s no such method", d.Name())
}

func (d *DeepInfraModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s no such method", d.Name())
}

func (d *DeepInfraModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", d.BaseURL[region], d.URLSuffix.Models)

	reqBody := map[string]interface{}{}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("GET", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to read response: %s", string(body))
	}

	// Parse response
	var result []struct {
		ModelName string `json:"model_name"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	models := make([]string, 0)
	for _, model := range result {
		if model.ModelName != "" {
			models = append(models, model.ModelName)
		}
	}

	return models, nil
}

func (d *DeepInfraModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", d.BaseURL[region], d.URLSuffix.Balance)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to read response: %s", string(body))
	}

	var result struct {
		Balance interface{} `json:"stripe_balance"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return map[string]interface{}{
		"balance":  result.Balance,
		"currence": "USD",
	}, nil
}

func (d *DeepInfraModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := d.ListModels(apiConfig)
	return err
}

func (d *DeepInfraModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s no such method", d.Name())
}

func (d *DeepInfraModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s no such method", d.Name())
}
