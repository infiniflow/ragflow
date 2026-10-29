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
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"ragflow/internal/common"

	"go.uber.org/zap"
)

type PaddleOCRModel struct {
	baseModel BaseModel
}

func NewPaddleOCRModel(baseURL map[string]string, urlSuffix URLSuffix) *PaddleOCRModel {
	return &PaddleOCRModel{
		baseModel: BaseModel{
			BaseURL:          baseURL,
			URLSuffix:        urlSuffix,
			AllowEmptyAPIKey: true,
			httpClient:       common.GetSSRFHTTPClient(),
		},
	}
}

func (p *PaddleOCRModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewPaddleOCRModel(baseURL, p.baseModel.URLSuffix)
}

func (p *PaddleOCRModel) Name() string {
	return "paddleocr"
}

func (p *PaddleOCRModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", p.Name())
}

type paddleSubmitResponse struct {
	Data struct {
		JobId string `json:"jobId"`
	} `json:"data"`
}

type paddlePollResponse struct {
	Data struct {
		State         string `json:"state"`
		ResultJSONURL string `json:"resultJsonUrl"`
		ErrorMsg      string `json:"errorMsg"`
		ResultUrl     struct {
			JsonUrl string `json:"jsonUrl"`
		} `json:"resultUrl"`
	} `json:"data"`
}

// maxErrorBodyBytes caps how much of a failed HTTP response body is read and
// logged so an oversized or erroneous payload cannot consume unbounded memory
// or produce unbounded log lines / error strings.
const maxErrorBodyBytes = 64 * 1024

// readErrorBody drains at most maxErrorBodyBytes of an error response body for
// logging. It must be called before the body is otherwise consumed.
func readErrorBody(body io.Reader) string {
	data, _ := io.ReadAll(io.LimitReader(body, maxErrorBodyBytes))
	return string(data)
}

func (p *PaddleOCRModel) OCRFile(ctx context.Context, modelName *string, content []byte, fileURL *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	if err := p.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	apiConfig = paddleOCRResolvedAPIConfig(apiConfig)

	if (content == nil || len(content) == 0) && (fileURL == nil || *fileURL == "") {
		return nil, fmt.Errorf("content and fileURL cannot be both empty")
	}

	url, err := p.baseModel.EndpointURL(apiConfig, p.baseModel.URLSuffix.OCR)
	if err != nil {
		return nil, err
	}

	optionalPayload := map[string]bool{
		"useDocOrientationClassify": false,
		"useDocUnwarping":           false,
		"useChartRecognition":       false,
	}
	optBytes, _ := json.Marshal(optionalPayload)

	// One generous deadline bounds the whole OCR operation (submit + poll +
	// result download), so the poll loop below can no longer spin forever.
	ctx, cancel := context.WithTimeout(ctx, longOpCallTimeout)
	defer cancel()

	var req *http.Request

	if fileURL != nil && strings.HasPrefix(*fileURL, "http") {
		reqData := map[string]interface{}{
			"fileUrl":         *fileURL,
			"model":           *modelName,
			"optionalPayload": optionalPayload,
		}
		jsonData, err := json.Marshal(reqData)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal json: %w", err)
		}
		req, err = http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
	} else {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		_ = writer.WriteField("model", *modelName)
		_ = writer.WriteField("optionalPayload", string(optBytes))

		part, err := writer.CreateFormFile("file", paddleOCRUploadFilename(content))
		if err != nil {
			return nil, fmt.Errorf("failed to create form file: %w", err)
		}
		part.Write(content)
		writer.Close()

		req, err = http.NewRequestWithContext(ctx, "POST", url, body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
	}

	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	req.Header.Set("Client-Platform", "ragflow")

	common.Info("paddleocr submit: sending job",
		zap.String("driver", p.Name()),
		zap.String("url", url),
		zap.String("model", *modelName),
		zap.Int("content_bytes", len(content)),
		zap.Bool("by_url", fileURL != nil && strings.HasPrefix(*fileURL, "http")))

	resp, err := p.baseModel.httpClient.Do(req)
	if err != nil {
		common.Error("paddleocr submit: request failed",
			err,
			zap.String("driver", p.Name()),
			zap.String("url", url))
		return nil, fmt.Errorf("failed to submit job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody := readErrorBody(resp.Body)
		common.Error("paddleocr submit: non-200",
			fmt.Errorf("status %d", resp.StatusCode),
			zap.String("driver", p.Name()),
			zap.String("url", url),
			zap.Int("status", resp.StatusCode),
			zap.String("body", errBody))
		return nil, fmt.Errorf("submit job failed: %s", errBody)
	}
	respBody, _ := io.ReadAll(resp.Body)

	var submitResp paddleSubmitResponse
	if err := json.Unmarshal(respBody, &submitResp); err != nil {
		common.Error("paddleocr submit: parse failed",
			err,
			zap.String("driver", p.Name()),
			zap.String("url", url))
		return nil, fmt.Errorf("failed to parse submit response: %w", err)
	}

	jobId := submitResp.Data.JobId
	if jobId == "" {
		return nil, fmt.Errorf("failed to get jobId from response")
	}

	common.Info("paddleocr submit: ok",
		zap.String("driver", p.Name()),
		zap.String("url", url),
		zap.String("job_id", jobId))

	pollUrl := fmt.Sprintf("%s/%s", url, jobId)
	var jsonlUrl string

	pollInterval := 3 * time.Second
	const pollMultiplier = 1.5
	maxPollInterval := 15 * time.Second

	attempt := 0
	for {
		attempt++
		pollReq, err := http.NewRequestWithContext(ctx, "GET", pollUrl, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create poll request: %w", err)
		}
		if auth := BearerAuth(apiConfig); auth != "" {
			pollReq.Header.Set("Authorization", auth)
		}
		pollReq.Header.Set("Client-Platform", "ragflow")

		pollResp, err := p.baseModel.httpClient.Do(pollReq)
		if err != nil {
			common.Error("paddleocr poll: request failed",
				err,
				zap.String("job_id", jobId),
				zap.Int("attempt", attempt))
			return nil, fmt.Errorf("failed to poll job status: %w", err)
		}

		if pollResp.StatusCode != http.StatusOK {
			errBody := readErrorBody(pollResp.Body)
			pollResp.Body.Close()
			common.Error("paddleocr poll: non-200",
				fmt.Errorf("status %d", pollResp.StatusCode),
				zap.String("job_id", jobId),
				zap.Int("attempt", attempt),
				zap.Int("status", pollResp.StatusCode),
				zap.String("body", errBody))
			return nil, fmt.Errorf("poll job failed: %s", errBody)
		}
		pollBody, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var pollData paddlePollResponse
		if err = json.Unmarshal(pollBody, &pollData); err != nil {
			common.Error("paddleocr poll: parse failed",
				err,
				zap.String("job_id", jobId),
				zap.Int("attempt", attempt))
			return nil, fmt.Errorf("failed to parse poll response: %w", err)
		}

		// end if 'done' or 'failed'
		state := pollData.Data.State
		if state == "done" {
			common.Info("paddleocr poll: done",
				zap.String("job_id", jobId),
				zap.Int("attempt", attempt),
				zap.String("state", state))
			jsonlUrl = pollData.Data.ResultUrl.JsonUrl
			if jsonlUrl == "" {
				jsonlUrl = pollData.Data.ResultJSONURL
			}
			break
		} else if state == "failed" {
			common.Error("paddleocr poll: failed",
				fmt.Errorf("job failed on server"),
				zap.String("job_id", jobId),
				zap.Int("attempt", attempt),
				zap.String("state", state),
				zap.String("error_msg", pollData.Data.ErrorMsg))
			return nil, fmt.Errorf("ocr job failed on server: %s", pollData.Data.ErrorMsg)
		}

		common.Debug("paddleocr poll: in progress",
			zap.String("job_id", jobId),
			zap.Int("attempt", attempt),
			zap.String("state", state),
			zap.Duration("next_poll_in", pollInterval))

		// Exponential backoff
		pollInterval = min(time.Duration(float64(pollInterval)*pollMultiplier), maxPollInterval)

		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			common.Error("paddleocr poll: context done while waiting",
				ctx.Err(),
				zap.String("job_id", jobId),
				zap.Int("attempt", attempt),
				zap.String("last_state", state))
			return nil, ctx.Err()
		}
	}

	if jsonlUrl == "" {
		return nil, fmt.Errorf("job done but jsonl url is empty")
	}

	common.Info("paddleocr result: downloading",
		zap.String("job_id", jobId),
		zap.String("jsonl_url", jsonlUrl))

	resReq, err := http.NewRequestWithContext(ctx, "GET", jsonlUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for jsonl: %w", err)
	}

	resResp, err := p.baseModel.httpClient.Do(resReq)
	if err != nil {
		common.Error("paddleocr result: download request failed",
			err,
			zap.String("job_id", jobId),
			zap.String("jsonl_url", jsonlUrl))
		return nil, fmt.Errorf("failed to download jsonl result: %w", err)
	}
	defer resResp.Body.Close()

	if resResp.StatusCode != http.StatusOK {
		common.Error("paddleocr result: non-200",
			fmt.Errorf("status %d", resResp.StatusCode),
			zap.String("job_id", jobId),
			zap.String("jsonl_url", jsonlUrl),
			zap.Int("status", resResp.StatusCode))
		return nil, fmt.Errorf("failed to download jsonl, status: %d", resResp.StatusCode)
	}

	common.Info("paddleocr result: downloaded",
		zap.String("job_id", jobId),
		zap.String("jsonl_url", jsonlUrl))

	rawBody, err := io.ReadAll(resResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read result body: %w", err)
	}

	response, err := parsePaddleOCRResultBody(rawBody)
	if err != nil {
		common.Error("paddleocr result: parse failed", err,
			zap.String("driver", p.Name()), zap.String("job_id", jobId),
			zap.Int("body_bytes", len(rawBody)))
		return nil, err
	}
	if len(response.Warnings) > 0 {
		common.Warn("paddleocr result: partial failure", zap.String("job_id", jobId), zap.Strings("warnings", response.Warnings))
	}
	extractedText := *response.Text
	common.Info("paddleocr result: parsed",
		zap.String("job_id", jobId), zap.Int("body_bytes", len(rawBody)),
		zap.Int("text_len", len(extractedText)), zap.Int("page_count", len(response.Pages)))

	// OCR text can contain sensitive document content; keep any preview out of
	// default (Info) logs and only surface it when debug logging is enabled.
	preview := extractedText
	if len(preview) > 200 {
		preview = preview[:200]
	}
	common.Debug("paddleocr result: text preview",
		zap.String("job_id", jobId),
		zap.String("text_preview", preview))

	return response, nil
}

// paddleOCRUploadFilename preserves the content-derived format expected by the
// job API without trusting a user-supplied filename. Unknown data stays PDF.
func paddleOCRUploadFilename(content []byte) string {
	// net/http's sniffer does not recognize TIFF, which the image parser accepts.
	if bytes.HasPrefix(content, []byte("II*\x00")) || bytes.HasPrefix(content, []byte("MM\x00*")) {
		return "document.tiff"
	}
	switch http.DetectContentType(content) {
	case "image/png":
		return "document.png"
	case "image/jpeg":
		return "document.jpg"
	case "image/gif":
		return "document.gif"
	case "image/webp":
		return "document.webp"
	case "image/bmp":
		return "document.bmp"
	case "image/tiff":
		return "document.tiff"
	default:
		return "document.pdf"
	}
}

func (p *PaddleOCRModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	return fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

func (p *PaddleOCRModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}

// paddleOCRConfigFromAPIKey parses the tenant PaddleOCR api_key payload the
// same way Python's PaddleOCROcrModel does: the api_key is a JSON object
// carrying the server endpoint, access token and algorithm, either with the
// lower-case keys written by the model UI or the upper-case env-style keys
// written by the env auto-provisioning, optionally nested under an "api_key"
// key. A non-JSON api_key — the PaddleOCR.local plain bearer token — yields
// zero values and the caller falls back to plain-text semantics. base_url
// prefers paddleocr_base_url over paddleocr_api_url, mirroring Python.
func paddleOCRConfigFromAPIKey(apiKey string) (baseURL, accessToken, algorithm string) {
	if strings.TrimSpace(apiKey) == "" {
		return "", "", ""
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(apiKey), &raw); err != nil {
		return "", "", ""
	}
	if nested, ok := raw["api_key"].(map[string]interface{}); ok {
		raw = nested
	}
	get := func(key string) string {
		for _, k := range []string{key, strings.ToUpper(key)} {
			if value, _ := raw[k].(string); strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
		return ""
	}
	baseURL = get("paddleocr_base_url")
	if baseURL == "" {
		baseURL = get("paddleocr_api_url")
	}
	return baseURL, get("paddleocr_access_token"), get("paddleocr_algorithm")
}

// paddleOCRResolvedAPIConfig resolves the wire config for the PaddleOCR
// drivers: the bearer token comes from the api_key JSON payload when present,
// and the base url falls back from the instance field through the payload to
// the PADDLEOCR_BASE_URL / PADDLEOCR_API_URL env vars. A plain-text api_key
// passes through untouched; an empty base url is left for the driver's catalog
// default.
func paddleOCRResolvedAPIConfig(apiConfig *APIConfig) *APIConfig {
	if apiConfig == nil {
		return nil
	}
	apiKey := ""
	if apiConfig.ApiKey != nil {
		apiKey = *apiConfig.ApiKey
	}
	baseURL := ""
	if apiConfig.BaseURL != nil {
		baseURL = strings.TrimSpace(*apiConfig.BaseURL)
	}
	keyBaseURL, keyAccessToken, _ := paddleOCRConfigFromAPIKey(apiKey)
	if keyAccessToken != "" {
		apiKey = keyAccessToken
	}
	if baseURL == "" {
		baseURL = keyBaseURL
	}
	if baseURL == "" {
		baseURL = strings.TrimSpace(common.GetEnv(common.EnvPaddleOCRBaseUrl))
	}
	if baseURL == "" {
		baseURL = strings.TrimSpace(common.GetEnv(common.EnvPaddleOCRAPIURL))
	}
	return &APIConfig{ApiKey: &apiKey, Region: apiConfig.Region, BaseURL: &baseURL}
}

// paddleOCRAlgorithm resolves the OCR algorithm the drivers advertise: an
// explicit OCRConfig override (parser setup) wins, then the api_key payload,
// then the PADDLEOCR_ALGORITHM env var, then the PaddleOCR-VL default.
func paddleOCRAlgorithm(ocrConfig *OCRConfig, apiConfig *APIConfig) string {
	if ocrConfig != nil {
		if algorithm := strings.TrimSpace(ocrConfig.Algorithm); algorithm != "" {
			return algorithm
		}
	}
	apiKey := ""
	if apiConfig != nil && apiConfig.ApiKey != nil {
		apiKey = *apiConfig.ApiKey
	}
	if _, _, algorithm := paddleOCRConfigFromAPIKey(apiKey); strings.TrimSpace(algorithm) != "" {
		return strings.TrimSpace(algorithm)
	}
	if algorithm := strings.TrimSpace(common.GetEnv(common.EnvPaddleOCRAlgorithm)); algorithm != "" {
		return algorithm
	}
	return "PaddleOCR-VL"
}

func (p *PaddleOCRModel) Decide(ctx context.Context, modelName *string, request *DecideRequest, apiConfig *APIConfig, modelUsage *common.ModelUsage) (*DecideResponse, error) {
	return nil, fmt.Errorf("%s, no such method", p.Name())
}
