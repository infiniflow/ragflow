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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type XiaomiModel struct {
	baseModel BaseModel
}

func NewXiaomiModel(baseURL map[string]string, urlSuffix URLSuffix) *XiaomiModel {
	return &XiaomiModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (x *XiaomiModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewXiaomiModel(baseURL, x.baseModel.URLSuffix)
}

func (x *XiaomiModel) Name() string {
	return "xiaomi"
}

func (x *XiaomiModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if err := x.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := x.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, x.baseModel.URLSuffix.Chat)

	// Convert messages to API format
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// Build request body
	reqBody := map[string]interface{}{
		"model":       modelName,
		"messages":    apiMessages,
		"stream":      false,
		"temperature": 1,
	}

	if chatModelConfig != nil {
		if chatModelConfig.Stream != nil {
			reqBody["stream"] = *chatModelConfig.Stream
		}

		if chatModelConfig.MaxTokens != nil {
			reqBody["max_completion_tokens"] = *chatModelConfig.MaxTokens
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
				reqBody["thinking"] = map[string]interface{}{
					"type": "enabled",
				}
			} else {
				reqBody["thinking"] = map[string]interface{}{
					"type": "disabled",
				}
			}
		}
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
	req.Header.Set("api-key", *apiConfig.ApiKey)

	resp, err := x.baseModel.httpClient.Do(req)
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

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
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
	reasonContent, _ = messageMap["reasoning_content"].(string)
	if reasonContent == "" && chatModelConfig != nil && chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
		// If reasoning_content not in response, try parsing from content tags
		reasoning, answer := GetThinkingAndAnswer(chatModelConfig.ModelClass, &content)
		if reasoning != nil {
			reasonContent = *reasoning
			content = *answer
		}
	}
	// if first char of reasonContent is \n remove the '\n'
	if reasonContent != "" && reasonContent[0] == '\n' {
		reasonContent = reasonContent[1:]
	}

	chatResponse := &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}

	return chatResponse, nil
}

func (x *XiaomiModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	if err := x.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if modelName == "" {
		return fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	baseURL, err := x.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", baseURL, x.baseModel.URLSuffix.Chat)

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
		if modelConfig.MaxTokens != nil {
			reqBody["max_completion_tokens"] = *modelConfig.MaxTokens
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
				reqBody["thinking"] = map[string]interface{}{
					"type": "enabled",
				}
			} else {
				reqBody["thinking"] = map[string]interface{}{
					"type": "disabled",
				}
			}
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), streamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", *apiConfig.ApiKey)

	resp, err := x.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// SSE parsing: read line by line
	if _, err := ParseSSEStreamTolerant[map[string]interface{}](resp.Body, func(event map[string]interface{}) error {
		choices, ok := event["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			return nil
		}

		firstChoice, ok := choices[0].(map[string]interface{})
		if !ok {
			return nil
		}

		delta, ok := firstChoice["delta"].(map[string]interface{})
		if !ok {
			return nil
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

		return nil
	}); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}

	// Send [DONE] marker for OpenAI compatibility
	endOfStream := "[DONE]"
	if err = sender(&endOfStream, nil); err != nil {
		return err
	}

	return nil
}

func (x *XiaomiModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("no such method %s", x.Name())
}

func (x *XiaomiModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("no such method %s", x.Name())
}

func (x *XiaomiModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), longOpCallTimeout)
	defer cancel()

	req, err := x.newXiaomiASRRequest(ctx, modelName, file, apiConfig, asrConfig, false)
	if err != nil {
		return nil, err
	}

	resp, err := x.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Xiaomi ASR API error: %s, body: %s", resp.Status, string(body))
	}

	var result xiaomiChatCompletionResponse
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Xiaomi ASR response: %w, body=%s", err, string(body))
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in Xiaomi ASR response")
	}

	return &ASRResponse{Text: result.Choices[0].Message.Content}, nil
}

func (x *XiaomiModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), streamCallTimeout)
	defer cancel()

	req, err := x.newXiaomiASRRequest(ctx, modelName, file, apiConfig, asrConfig, true)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := x.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Xiaomi ASR stream API error: %s, body: %s", resp.Status, string(body))
	}

	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
		response, err := decodeXiaomiASRResponse(body)
		if err != nil {
			return err
		}
		if response.Text != "" {
			if err = sender(&response.Text, nil); err != nil {
				return err
			}
		}
		done := "[DONE]"
		return sender(&done, nil)
	}

	return readXiaomiASRStream(resp.Body, sender)
}

type xiaomiChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string              `json:"content"`
			Audio   *xiaomiAudioPayload `json:"audio"`
		} `json:"message"`
	} `json:"choices"`
}

type xiaomiChatCompletionChunk struct {
	Choices []struct {
		Delta struct {
			Content *string             `json:"content"`
			Audio   *xiaomiAudioPayload `json:"audio"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

type xiaomiAudioPayload struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

func (x *XiaomiModel) newXiaomiASRRequest(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, stream bool) (*http.Request, error) {
	if err := x.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if file == nil || *file == "" {
		return nil, fmt.Errorf("file is missing")
	}
	if strings.TrimSpace(x.baseModel.URLSuffix.Chat) == "" {
		return nil, fmt.Errorf("xiaomi chat URL suffix is required")
	}

	// codeql[go/path-injection] False positive: *file is the audio file path the caller passes in to upload. The user (or operator-supplied pipeline) explicitly chose this path, and the OS access check enforces permissions anyway.
	audio, err := os.ReadFile(*file)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio file: %w", err)
	}

	mimeType := xiaomiAudioMIMEType(*file, audio, asrConfig)
	reqBody := map[string]interface{}{
		"model": *modelName,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "input_audio",
						"input_audio": map[string]interface{}{
							"data": fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(audio)),
						},
					},
				},
			},
		},
		"asr_options": xiaomiASROptions(asrConfig),
	}
	if stream {
		reqBody["stream"] = true
	}
	if asrConfig != nil && asrConfig.Params != nil {
		for key, value := range asrConfig.Params {
			switch key {
			case "asr_options", "language", "mime", "mime_type", "model", "messages", "stream":
				continue
			default:
				reqBody[key] = value
			}
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	baseURL, err := x.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), strings.TrimPrefix(x.baseModel.URLSuffix.Chat, "/"))

	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", *apiConfig.ApiKey)

	return req, nil
}

func xiaomiASROptions(asrConfig *ASRConfig) map[string]interface{} {
	options := map[string]interface{}{"language": "auto"}
	if asrConfig == nil || asrConfig.Params == nil {
		return options
	}
	if rawOptions, ok := asrConfig.Params["asr_options"].(map[string]interface{}); ok {
		for key, value := range rawOptions {
			options[key] = value
		}
	}
	if language, ok := asrConfig.Params["language"]; ok && language != nil && fmt.Sprint(language) != "" {
		options["language"] = language
	}
	return options
}

func xiaomiAudioMIMEType(file string, audio []byte, asrConfig *ASRConfig) string {
	if asrConfig != nil && asrConfig.Params != nil {
		for _, key := range []string{"mime_type", "mime"} {
			if value, ok := asrConfig.Params[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	if detected := mime.TypeByExtension(strings.ToLower(filepath.Ext(file))); detected != "" {
		return detected
	}
	if len(audio) > 0 {
		return http.DetectContentType(audio)
	}
	return "application/octet-stream"
}

func decodeXiaomiASRResponse(body []byte) (*ASRResponse, error) {
	var result xiaomiChatCompletionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Xiaomi ASR response: %w, body=%s", err, string(body))
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in Xiaomi ASR response")
	}
	return &ASRResponse{Text: result.Choices[0].Message.Content}, nil
}

func readXiaomiASRStream(body io.Reader, sender func(*string, *string) error) error {
	if _, err := ParseSSEStreamTolerant[xiaomiChatCompletionChunk](body, func(chunk xiaomiChatCompletionChunk) error {
		if len(chunk.Choices) == 0 {
			return nil
		}

		content := chunk.Choices[0].Delta.Content
		if content != nil && *content != "" {
			if err := sender(content, nil); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error reading Xiaomi ASR stream: %w", err)
	}

	done := "[DONE]"
	return sender(&done, nil)
}

func (x *XiaomiModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), longOpCallTimeout)
	defer cancel()

	req, err := x.newXiaomiTTSRequest(ctx, modelName, audioContent, apiConfig, ttsConfig, false)
	if err != nil {
		return nil, err
	}

	resp, err := x.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Xiaomi TTS API error: %s, body: %s", resp.Status, string(body))
	}

	return decodeXiaomiTTSResponse(body)
}

func (x *XiaomiModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), streamCallTimeout)
	defer cancel()

	req, err := x.newXiaomiTTSRequest(ctx, modelName, audioContent, apiConfig, ttsConfig, true)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := x.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Xiaomi TTS stream API error: %s, body: %s", resp.Status, string(body))
	}

	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
		audio, err := decodeXiaomiTTSResponse(body)
		if err != nil {
			return err
		}
		if len(audio.Audio) > 0 {
			chunk := base64.StdEncoding.EncodeToString(audio.Audio)
			return sender(&chunk, nil)
		}
		return nil
	}

	return readXiaomiTTSStream(resp.Body, sender)
}

func (x *XiaomiModel) newXiaomiTTSRequest(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, stream bool) (*http.Request, error) {
	if err := x.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if audioContent == nil || *audioContent == "" {
		return nil, fmt.Errorf("audio content is empty")
	}
	if strings.TrimSpace(x.baseModel.URLSuffix.Chat) == "" {
		return nil, fmt.Errorf("xiaomi chat URL suffix is required")
	}

	reqBody := map[string]interface{}{
		"model": *modelName,
		"messages": []map[string]interface{}{
			{
				"role":    "assistant",
				"content": *audioContent,
			},
		},
		"audio": xiaomiTTSOptions(ttsConfig),
	}
	if stream {
		reqBody["stream"] = true
	}
	if ttsConfig != nil && ttsConfig.Params != nil {
		for key, value := range ttsConfig.Params {
			switch key {
			case "audio", "format", "voice", "model", "messages", "stream":
				continue
			default:
				reqBody[key] = value
			}
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	baseURL, err := x.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), strings.TrimPrefix(x.baseModel.URLSuffix.Chat, "/"))

	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", *apiConfig.ApiKey)

	return req, nil
}

func xiaomiTTSOptions(ttsConfig *TTSConfig) map[string]interface{} {
	options := map[string]interface{}{
		"format": "wav",
		"voice":  "mimo_default",
	}
	if ttsConfig == nil {
		return options
	}
	if ttsConfig.Format != "" {
		options["format"] = ttsConfig.Format
	}
	if ttsConfig.Params == nil {
		return options
	}
	if rawOptions, ok := ttsConfig.Params["audio"].(map[string]interface{}); ok {
		for key, value := range rawOptions {
			options[key] = value
		}
	}
	if format, ok := ttsConfig.Params["format"]; ok && format != nil && fmt.Sprint(format) != "" {
		options["format"] = format
	}
	if voice, ok := ttsConfig.Params["voice"]; ok && voice != nil && fmt.Sprint(voice) != "" {
		options["voice"] = voice
	}
	return options
}

func decodeXiaomiTTSResponse(body []byte) (*TTSResponse, error) {
	var result xiaomiChatCompletionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Xiaomi TTS response: %w, body=%s", err, string(body))
	}
	if len(result.Choices) == 0 || result.Choices[0].Message.Audio == nil || result.Choices[0].Message.Audio.Data == "" {
		return nil, fmt.Errorf("no audio data in Xiaomi TTS response")
	}

	audio, err := decodeXiaomiAudioData(result.Choices[0].Message.Audio.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Xiaomi TTS audio: %w", err)
	}
	return &TTSResponse{Audio: audio}, nil
}

func readXiaomiTTSStream(body io.Reader, sender func(*string, *string) error) error {
	if _, err := ParseSSEStreamTolerant[xiaomiChatCompletionChunk](body, func(chunk xiaomiChatCompletionChunk) error {
		if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Audio == nil || chunk.Choices[0].Delta.Audio.Data == "" {
			return nil
		}
		audioData := chunk.Choices[0].Delta.Audio.Data
		if err := sender(&audioData, nil); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error reading Xiaomi TTS stream: %w", err)
	}
	return nil
}

func decodeXiaomiAudioData(data string) ([]byte, error) {
	if comma := strings.Index(data, ","); strings.HasPrefix(data, "data:") && comma >= 0 {
		data = data[comma+1:]
	}
	return base64.StdEncoding.DecodeString(data)
}

func (x *XiaomiModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("no such method %s", x.Name())
}

func (x *XiaomiModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("no such method %s", x.Name())
}

func (x *XiaomiModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	return nil, fmt.Errorf("no such method %s", x.Name())
}

func (x *XiaomiModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method %s", x.Name())
}

func (x *XiaomiModel) CheckConnection(apiConfig *APIConfig) error {
	if err := x.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}
	_, err := x.baseModel.GetBaseURL(apiConfig)
	return err
}

func (x *XiaomiModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("no such method %s", x.Name())
}

func (x *XiaomiModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("no such method %s", x.Name())
}
