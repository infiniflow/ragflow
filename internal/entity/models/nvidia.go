package models

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// NvidiaModel implements ModelDriver for Nvidia
type NvidiaModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

func (n NvidiaModel) ParseFile() {
	//TODO implement me
	panic("implement me")
}

// NewNvidiaModel creates a new Nvidia model instance
func NewNvidiaModel(baseURL map[string]string, urlSuffix URLSuffix) *NvidiaModel {
	return &NvidiaModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
			},
		},
	}
}

func (n NvidiaModel) NewInstance(baseURL map[string]string) ModelDriver {
	return &NvidiaModel{
		BaseURL:   baseURL,
		URLSuffix: n.URLSuffix,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
			},
		},
	}
}

func (n NvidiaModel) Name() string {
	return "nvidia"
}

func (n *NvidiaModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil {
		region = *apiConfig.Region
	}

	baseURL := n.BaseURL[region]
	if baseURL == "" {
		baseURL = n.BaseURL["default"]
	}
	url := fmt.Sprintf("%s/%s", baseURL, n.URLSuffix.Chat)

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
		if chatModelConfig.Thinking != nil {
			if *chatModelConfig.Thinking {
				reqBody["thinking"] = map[string]interface{}{"type": "enabled"}
			} else {
				reqBody["thinking"] = map[string]interface{}{"type": "disabled"}
			}
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
	if apiConfig != nil && apiConfig.ApiKey != nil {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	}

	resp, err := n.httpClient.Do(req)
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

	var result map[string]interface{}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
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
	if chatModelConfig != nil && chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
		reasonContent, ok = messageMap["reasoning_content"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid content format")
		}
		if reasonContent != "" && reasonContent[0] == '\n' {
			reasonContent = reasonContent[1:]
		}
	}

	chatResponse := &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}

	return chatResponse, nil
}

func (n *NvidiaModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL := n.BaseURL[region]
	if baseURL == "" {
		baseURL = n.BaseURL["default"]
	}
	url := fmt.Sprintf("%s/%s", baseURL, n.URLSuffix.Chat)

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
		"stream":   true,
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
		if modelConfig.Thinking != nil {
			if *modelConfig.Thinking {
				reqBody["thinking"] = map[string]interface{}{"type": "enabled"}
			} else {
				reqBody["thinking"] = map[string]interface{}{"type": "disabled"}
			}
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
	if apiConfig != nil && apiConfig.ApiKey != nil {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	}

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(line[5:])
		if data == "[DONE]" {
			break
		}

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

	endOfStream := "[DONE]"
	if err = sender(&endOfStream, nil); err != nil {
		return err
	}

	return scanner.Err()
}

type nvidiaEmbeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func (n NvidiaModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
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

	baseURL := n.BaseURL[region]
	if baseURL == "" {
		baseURL = n.BaseURL["default"]
	}
	if baseURL == "" {
		return nil, fmt.Errorf("nvidia: no base URL configured for region %q", region)
	}

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), n.URLSuffix.Embedding)

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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := n.httpClient.Do(req)
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
func (n NvidiaModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
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

	baseURL := n.BaseURL[region]
	if baseURL == "" {
		baseURL = n.BaseURL["default"]
	}
	if baseURL == "" {
		return nil, fmt.Errorf("nvidia: no base URL configured for region %q", region)
	}

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), n.URLSuffix.Rerank)

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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := n.httpClient.Do(req)
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
func (n *NvidiaModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}

func (z *NvidiaModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

// AudioSpeech convert audio to text
func (n *NvidiaModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, asrConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}

func (z *NvidiaModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

// OCRFile OCR file
func (m *NvidiaModel) OCRFile(modelName *string, fileContent *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

// ListModels calls /v1/models on the configured NVIDIA NIM base URL
// and returns the list of available model ids. The endpoint is
// OpenAI-compatible, so the parsing follows the same shape used by
// the moonshot, xai, and openai drivers.
func (n NvidiaModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL := n.BaseURL[region]
	if baseURL == "" {
		baseURL = n.BaseURL["default"]
	}
	if baseURL == "" {
		return nil, fmt.Errorf("nvidia: no base URL configured for region %q", region)
	}

	url := fmt.Sprintf("%s/%s", baseURL, n.URLSuffix.Models)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if apiConfig != nil && apiConfig.ApiKey != nil && *apiConfig.ApiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	}

	resp, err := n.httpClient.Do(req)
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

	var result map[string]interface{}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	data, ok := result["data"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid models list format")
	}

	models := make([]string, 0, len(data))
	for _, item := range data {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id, ok := m["id"].(string)
		if !ok {
			continue
		}
		models = append(models, id)
	}

	return models, nil
}

func (n NvidiaModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

// CheckConnection verifies that the configured NVIDIA NIM base URL
// is reachable and that the API key is accepted, by issuing a
// lightweight ListModels call. Mirrors the pattern used by the xai,
// moonshot, deepseek, aliyun, and gitee drivers.
func (n NvidiaModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := n.ListModels(apiConfig)
	return err
}
