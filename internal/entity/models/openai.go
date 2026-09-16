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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"ragflow/internal/common"
	"strconv"
	"strings"
)

// OpenAIModel implements ModelDriver for OpenAI (GPT models).
type OpenAIModel struct {
	baseModel BaseModel
}

// NewOpenAIModel creates a new OpenAI model instance.
func NewOpenAIModel(baseURL map[string]string, urlSuffix URLSuffix) *OpenAIModel {
	return &OpenAIModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(false),
		},
	}
}

func (o *OpenAIModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewOpenAIModel(baseURL, o.baseModel.URLSuffix)
}

func (o *OpenAIModel) Name() string {
	return "openai"
}

// ChatWithMessages sends multiple messages with roles and returns the response
func (o *OpenAIModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := o.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	baseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, o.baseModel.URLSuffix.Chat)

	// Build request body
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)

	if strings.Contains(strings.ToLower(modelName), "qwen3") {
		if chatModelConfig != nil && chatModelConfig.Thinking != nil {
			reqBody["enable_thinking"] = *chatModelConfig.Thinking
		} else {
			reqBody["enable_thinking"] = false
		}
	}

	body, err := o.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return HandleNonStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

// ChatStreamlyWithSender sends messages and streams the response
func (o *OpenAIModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := o.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	baseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, o.baseModel.URLSuffix.Chat)

	reqBody := buildRequestBody(chatModelConfig, modelName, messages, true)
	reqBody["stream_options"] = map[string]interface{}{
		"include_usage": true,
	}

	if chatModelConfig != nil {
		if chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
			return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
		}

	}

	// Qwen3 family: disable thinking unless explicitly enabled.
	if strings.Contains(strings.ToLower(modelName), "qwen3") {
		if chatModelConfig != nil && chatModelConfig.Thinking != nil {
			reqBody["enable_thinking"] = *chatModelConfig.Thinking
		} else {
			reqBody["enable_thinking"] = false
		}
	}

	return o.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		return HandleStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig, sender)
	})
}

type openaiEmbeddingResponse struct {
	Data   []openaiEmbeddingData `json:"data"`
	Model  string                `json:"model"`
	Object string                `json:"object"`
	Usage  openaiUsage           `json:"usage"`
}

type openaiEmbeddingData struct {
	Embedding []float64 `json:"embedding"`
	Object    string    `json:"object"`
	Index     int       `json:"index"`
}

type openaiUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func (o *OpenAIModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := o.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(request.Texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	baseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, o.baseModel.URLSuffix.Embedding)

	reqBody := map[string]interface{}{
		"model": *modelName,
		"input": request.Texts,
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

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := o.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := readModelErrorBody(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("OpenAI embeddings API error: %s, failed to read error response body: %w", resp.Status, err)
		}
		return nil, fmt.Errorf("OpenAI embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var parsed openaiEmbeddingResponse
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

// ListModels returns the list of model ids visible to the API key.
func (o *OpenAIModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := o.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	baseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", baseURL, o.baseModel.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// GET has no body, so Content-Type is not needed.
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := o.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := readModelErrorBody(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("API request failed with status %d; failed to read error response: %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := readModelResponseBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
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

// Balance is not exposed by the OpenAI API, so this returns "no such method".
func (o *OpenAIModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

// CheckConnection runs a lightweight ListModels call to verify the API key.
func (o *OpenAIModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := o.ListModels(ctx, apiConfig)
	if err != nil {
		return err
	}
	return nil
}

// Rerank calculates similarity scores between query and documents. OpenAI does
// not expose a rerank API, so this is left unimplemented.
func (o *OpenAIModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, Rerank not implemented", o.Name())
}

// TranscribeAudio transcribe audio
func (o *OpenAIModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, responseFormat, err := o.newOpenAIASRRequest(ctx, modelName, file, apiConfig, asrConfig, false)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")

	resp, err := o.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, err := readModelErrorBody(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("OpenAI ASR API error: %s, failed to read error response body: %w", resp.Status, err)
		}
		return nil, fmt.Errorf("OpenAI ASR API error: %s, body: %s", resp.Status, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return decodeOpenAIASRResponse(respBody, responseFormat)
}

func (o *OpenAIModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	req, responseFormat, err := o.newOpenAIASRRequest(ctx, modelName, file, apiConfig, asrConfig, true)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := o.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, err := readModelErrorBody(resp.Body)
		if err != nil {
			return fmt.Errorf("OpenAI ASR stream API error: %s, failed to read error response body: %w", resp.Status, err)
		}
		return fmt.Errorf("OpenAI ASR stream API error: %s, body: %s", resp.Status, string(respBody))
	}

	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
		response, err := decodeOpenAIASRResponse(respBody, responseFormat)
		if err != nil {
			return err
		}
		if response != nil && response.Text != "" {
			if err = sender(&response.Text, nil); err != nil {
				return err
			}
		}
		done := "[DONE]"
		return sender(&done, nil)
	}

	sentDelta := false
	if _, err = ParseSSEStream[struct {
		Type  string `json:"type"`
		Delta string `json:"delta"`
		Text  string `json:"text"`
	}](resp.Body, func(event struct {
		Type  string `json:"type"`
		Delta string `json:"delta"`
		Text  string `json:"text"`
	}) error {
		switch {
		case event.Delta != "":
			if err = sender(&event.Delta, nil); err != nil {
				return err
			}
			sentDelta = true
		case event.Type == "transcript.text.segment" && event.Text != "":
			if err = sender(&event.Text, nil); err != nil {
				return err
			}
			sentDelta = true
		case event.Type == "transcript.text.done" && !sentDelta && event.Text != "":
			if err = sender(&event.Text, nil); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error reading OpenAI ASR stream: %w", err)
	}

	done := "[DONE]"
	return sender(&done, nil)
}

func decodeOpenAIASRResponse(respBody []byte, responseFormat string) (*ASRResponse, error) {
	switch responseFormat {
	case "text", "srt", "vtt":
		return &ASRResponse{Text: string(respBody)}, nil
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w, body=%s", err, string(respBody))
	}

	return &ASRResponse{Text: result.Text}, nil
}

// AudioSpeech convert text to audio
func (o *OpenAIModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, _, err := o.newOpenAITTSRequest(ctx, modelName, audioContent, apiConfig, ttsConfig, false)
	if err != nil {
		return nil, err
	}

	resp, err := o.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := readModelErrorBody(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("OpenAI TTS API error: %s, failed to read error response body: %w", resp.Status, err)
		}
		return nil, fmt.Errorf("OpenAI TTS API error: %s, body: %s", resp.Status, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return &TTSResponse{Audio: body}, nil
}

func (o *OpenAIModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	req, streamFormat, err := o.newOpenAITTSRequest(ctx, modelName, audioContent, apiConfig, ttsConfig, true)
	if err != nil {
		return err
	}
	if streamFormat == "sse" {
		req.Header.Set("Accept", "text/event-stream")
	}

	resp, err := o.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := readModelErrorBody(resp.Body)
		if err != nil {
			return fmt.Errorf("OpenAI TTS stream API error: %s, failed to read error response body: %w", resp.Status, err)
		}
		return fmt.Errorf("OpenAI TTS stream API error: %s, body: %s", resp.Status, string(body))
	}

	if streamFormat == "sse" || strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return readOpenAITTSSSEStream(resp.Body, sender)
	}
	return readOpenAITTSRawStream(resp.Body, sender)
}

func (o *OpenAIModel) newOpenAIASRRequest(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, stream bool) (*http.Request, string, error) {
	if err := o.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, "", err
	}
	if modelName == nil || *modelName == "" {
		return nil, "", fmt.Errorf("model name is required")
	}
	if file == nil || *file == "" {
		return nil, "", fmt.Errorf("file is missing")
	}
	if strings.TrimSpace(o.baseModel.URLSuffix.ASR) == "" {
		return nil, "", fmt.Errorf("openai ASR URL suffix is required")
	}

	baseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, "", err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), strings.TrimPrefix(o.baseModel.URLSuffix.ASR, "/"))

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// codeql[go/path-injection] False positive: *file is the audio file path the caller passes in to upload. The user (or operator-supplied pipeline) explicitly chose this path, and the OS access check enforces permissions anyway.
	audioFile, err := os.Open(*file)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open audio file: %w", err)
	}
	defer audioFile.Close()

	part, err := writer.CreateFormFile("file", filepath.Base(*file))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create multipart file: %w", err)
	}
	if _, err = io.Copy(part, audioFile); err != nil {
		return nil, "", fmt.Errorf("failed to copy audio data: %w", err)
	}
	if err = writer.WriteField("model", *modelName); err != nil {
		return nil, "", fmt.Errorf("failed to write model field: %w", err)
	}

	responseFormat := ""
	if asrConfig != nil && asrConfig.Params != nil {
		if value, ok := asrConfig.Params["response_format"].(string); ok {
			responseFormat = value
		}
		for key, value := range asrConfig.Params {
			if stream && key == "stream" {
				continue
			}
			if err = writeOpenAIMultipartField(writer, key, value); err != nil {
				return nil, "", err
			}
		}
	}
	if stream {
		if err = writer.WriteField("stream", "true"); err != nil {
			return nil, "", fmt.Errorf("failed to write stream field: %w", err)
		}
	}

	if err = writer.Close(); err != nil {
		return nil, "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, &body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, responseFormat, nil
}

func (o *OpenAIModel) newOpenAITTSRequest(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, stream bool) (*http.Request, string, error) {
	if err := o.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, "", err
	}
	if modelName == nil || *modelName == "" {
		return nil, "", fmt.Errorf("model name is required")
	}
	if audioContent == nil || *audioContent == "" {
		return nil, "", fmt.Errorf("audio content is empty")
	}
	if strings.TrimSpace(o.baseModel.URLSuffix.TTS) == "" {
		return nil, "", fmt.Errorf("openai TTS URL suffix is required")
	}

	baseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, "", err
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), strings.TrimPrefix(o.baseModel.URLSuffix.TTS, "/"))

	reqBody := map[string]interface{}{
		"model": *modelName,
		"input": *audioContent,
	}

	if ttsConfig != nil && ttsConfig.Params != nil {
		for key, value := range ttsConfig.Params {
			reqBody[key] = value
		}
	}
	if ttsConfig != nil && ttsConfig.Format != "" {
		reqBody["response_format"] = ttsConfig.Format
	}
	if stream {
		if _, ok := reqBody["stream_format"]; !ok {
			reqBody["stream_format"] = "audio"
		}
	}

	voice, ok := reqBody["voice"]
	if !ok || voice == nil {
		return nil, "", fmt.Errorf("voice is required")
	}
	voiceString, ok := voice.(string)
	if !ok || strings.TrimSpace(voiceString) == "" {
		return nil, "", fmt.Errorf("voice is required")
	}

	streamFormat := ""
	if value, ok := reqBody["stream_format"].(string); ok {
		streamFormat = value
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	return req, streamFormat, nil
}

func readOpenAITTSSSEStream(body io.Reader, sender func(*string, *string) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		dataStr := strings.TrimSpace(line[6:])
		if dataStr == "" || dataStr == "[DONE]" {
			continue
		}

		var event struct {
			Type  string `json:"type"`
			Audio string `json:"audio"`
		}
		if err := json.Unmarshal([]byte(dataStr), &event); err != nil {
			continue
		}

		if event.Type == "speech.audio.delta" && event.Audio != "" {
			audioBytes, err := base64.StdEncoding.DecodeString(event.Audio)
			if err == nil && len(audioBytes) > 0 {
				chunk := string(audioBytes)
				if errSend := sender(&chunk, nil); errSend != nil {
					return errSend
				}
			}
		}
		if event.Type == "speech.audio.done" {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading OpenAI TTS stream: %w", err)
	}
	return nil
}

func readOpenAITTSRawStream(body io.Reader, sender func(*string, *string) error) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			if errSend := sender(&chunk, nil); errSend != nil {
				return errSend
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("error reading OpenAI TTS stream: %w", err)
		}
	}
}

func writeOpenAIMultipartField(writer *multipart.Writer, key string, value interface{}) error {
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
	case float32:
		val = strconv.FormatFloat(float64(v), 'f', -1, 32)
	case float64:
		val = strconv.FormatFloat(v, 'f', -1, 64)
	default:
		val = fmt.Sprintf("%v", v)
	}

	if err := writer.WriteField(key, val); err != nil {
		return fmt.Errorf("failed to write field %s: %w", key, err)
	}
	return nil
}

// OCRFile OCR file
func (o *OpenAIModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

// ParseFile parse file
func (o *OpenAIModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

func (o *OpenAIModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

func (o *OpenAIModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}
