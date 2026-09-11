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
			httpClient:       NewDriverHTTPClient(false),
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
		State     string `json:"state"`
		ErrorMsg  string `json:"errorMsg"`
		ResultUrl struct {
			JsonUrl string `json:"jsonUrl"`
		} `json:"resultUrl"`
	} `json:"data"`
}

type paddleJsonlLine struct {
	LogId     string `json:"logId"`
	ErrorCode int    `json:"errorCode"`
	ErrorMsg  string `json:"errorMsg"`
	Result    struct {
		LayoutParsingResults []struct {
			Markdown struct {
				Text string `json:"text"`
			} `json:"markdown"`
		} `json:"layoutParsingResults"`
		OcrResults []struct {
			PrunedResult struct {
				RecTexts []string `json:"rec_texts"`
			} `json:"prunedResult"`
		} `json:"ocrResults"`
	} `json:"result"`
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

// logBody truncates an already-buffered response payload to maxErrorBodyBytes
// so it is safe for log fields and error strings.
func logBody(body []byte) string {
	if len(body) > maxErrorBodyBytes {
		return string(body[:maxErrorBodyBytes]) + "…(truncated)"
	}
	return string(body)
}

// appendOCRText appends the markdown or OCR text fragments carried by one
// result entry to fullMarkdown and reports whether any text was added.
// Entries whose errorCode != 0 are treated as errored rather than silently
// empty so a partial/errored page is not invisible to the caller.
func (p *PaddleOCRModel) appendOCRText(fullMarkdown *strings.Builder, entry paddleJsonlLine) (added, errored bool) {
	if entry.ErrorCode != 0 {
		return false, true
	}
	before := fullMarkdown.Len()
	for _, layoutRes := range entry.Result.LayoutParsingResults {
		fullMarkdown.WriteString(layoutRes.Markdown.Text)
		fullMarkdown.WriteString("\n\n")
	}

	// Fallback to ocrResults for models like PP-OCRv6
	if len(entry.Result.LayoutParsingResults) == 0 {
		for _, ocrRes := range entry.Result.OcrResults {
			for _, text := range ocrRes.PrunedResult.RecTexts {
				text = strings.TrimSpace(text)
				if text != "" {
					fullMarkdown.WriteString(text)
					fullMarkdown.WriteString("\n")
				}
			}
		}
	}
	return fullMarkdown.Len() > before, false
}

// parseOCRResultBody extracts markdown/OCR text from the downloaded result
// payload. The online service returns a JSON array of page objects (list of
// dict), while older endpoints may return newline-delimited JSON objects;
// both forms are accepted. It reports how the payload was interpreted
// (arrayParsed) together with parse accounting counters so callers can log
// why the result was empty: entries dropped by json.Unmarshal (skippedLines),
// decoded entries carrying no text fragment (emptyResultLines), entries whose
// errorCode != 0 (erroredLines), and entries that yielded text (contentLines).
func (p *PaddleOCRModel) parseOCRResultBody(rawBody []byte, fullMarkdown *strings.Builder) (arrayParsed bool, scannedLines, skippedLines, emptyResultLines, contentLines, erroredLines int, err error) {
	var entries []paddleJsonlLine
	err = json.Unmarshal(rawBody, &entries)
	if err == nil {
		arrayParsed = true
		scannedLines = len(entries)
		for _, entry := range entries {
			added, errored := p.appendOCRText(fullMarkdown, entry)
			switch {
			case errored:
				erroredLines++
				p.logErroredEntry(entry)
			case added:
				contentLines++
			default:
				emptyResultLines++
			}
		}
		return
	}

	scanner := bufio.NewScanner(bytes.NewReader(rawBody))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		scannedLines++

		var lineData paddleJsonlLine
		if err := json.Unmarshal([]byte(line), &lineData); err != nil {
			skippedLines++
			continue
		}
		added, errored := p.appendOCRText(fullMarkdown, lineData)
		switch {
		case errored:
			erroredLines++
			p.logErroredEntry(lineData)
		case added:
			contentLines++
		default:
			emptyResultLines++
		}
	}
	if err := scanner.Err(); err != nil {
		return false, 0, 0, 0, 0, 0, fmt.Errorf("error reading jsonl: %w", err)
	}
	// The array-parse error stored in err is expected on this path; the jsonl
	// fallback succeeded, so clear it before the bare return.
	return false, scannedLines, skippedLines, emptyResultLines, contentLines, erroredLines, nil
}

// logErroredEntry makes an entry with a non-zero errorCode visible instead of
// silently dropping the page as empty.
func (p *PaddleOCRModel) logErroredEntry(entry paddleJsonlLine) {
	common.Warn("paddleocr result: entry errored",
		zap.String("log_id", entry.LogId),
		zap.Int("error_code", entry.ErrorCode),
		zap.String("error_msg", entry.ErrorMsg))
}

func (p *PaddleOCRModel) OCRFile(ctx context.Context, modelName *string, content []byte, fileURL *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	if err := p.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if (content == nil || len(content) == 0) && (fileURL == nil || *fileURL == "") {
		return nil, fmt.Errorf("content and fileURL cannot be both empty")
	}

	resolvedBaseURL, err := p.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, p.baseModel.URLSuffix.OCR)

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

		part, err := writer.CreateFormFile("file", "document.pdf")
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

	var fullMarkdown strings.Builder
	arrayParsed, scannedLines, skippedLines, emptyResultLines, contentLines, erroredLines, err := p.parseOCRResultBody(rawBody, &fullMarkdown)
	if err != nil {
		return nil, err
	}

	extractedText := strings.TrimSpace(fullMarkdown.String())
	if extractedText == "" {
		common.Warn("paddleocr result: parsed empty text",
			zap.String("job_id", jobId),
			zap.Bool("array_parsed", arrayParsed),
			zap.Int("body_bytes", len(rawBody)),
			zap.Int("scanned_lines", scannedLines),
			zap.Int("skipped_lines", skippedLines),
			zap.Int("errored_lines", erroredLines),
			zap.Int("empty_result_lines", emptyResultLines),
			zap.Int("content_lines", contentLines))
		return nil, fmt.Errorf("paddleocr result: parsed empty text (scanned_lines=%d, skipped_lines=%d, errored_lines=%d, empty_result_lines=%d, content_lines=%d)",
			scannedLines, skippedLines, erroredLines, emptyResultLines, contentLines)
	}
	common.Info("paddleocr result: parsed",
		zap.String("job_id", jobId),
		zap.Bool("array_parsed", arrayParsed),
		zap.Int("body_bytes", len(rawBody)),
		zap.Int("text_len", len(extractedText)),
		zap.Int("scanned_lines", scannedLines),
		zap.Int("skipped_lines", skippedLines),
		zap.Int("errored_lines", erroredLines),
		zap.Int("empty_result_lines", emptyResultLines),
		zap.Int("content_lines", contentLines))
	// OCR text can contain sensitive document content; keep any preview out of
	// default (Info) logs and only surface it when debug logging is enabled.
	preview := extractedText
	if len(preview) > 200 {
		preview = preview[:200]
	}
	common.Debug("paddleocr result: text preview",
		zap.String("job_id", jobId),
		zap.String("text_preview", preview))

	return &OCRFileResponse{Text: &extractedText}, nil
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

// PaddleOCRConfigFromAPIKey parses the tenant PaddleOCR api_key payload the
// same way Python's PaddleOCROcrModel does: the api_key is a JSON object
// carrying paddleocr_base_url / paddleocr_api_url, paddleocr_access_token and
// paddleocr_algorithm, optionally nested under an "api_key" key. A non-JSON
// api_key — the PaddleOCR.local plain bearer token — yields zero values and
// the caller falls back to plain-text semantics. base_url prefers
// paddleocr_base_url over paddleocr_api_url, mirroring Python.
func PaddleOCRConfigFromAPIKey(apiKey string) (baseURL, accessToken, algorithm string) {
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
		value, _ := raw[key].(string)
		return strings.TrimSpace(value)
	}
	baseURL = get("paddleocr_base_url")
	if baseURL == "" {
		baseURL = get("paddleocr_api_url")
	}
	return baseURL, get("paddleocr_access_token"), get("paddleocr_algorithm")
}
