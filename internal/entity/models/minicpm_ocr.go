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
	"errors"
	"fmt"
	"math"
	"mime/multipart"
	"net/http"
	neturl "net/url"
	"sort"
	"strings"
	"time"

	"ragflow/internal/common"

	"go.uber.org/zap"
)

// MiniCPM-OCR parses documents through the ModelBest document-parse jobs API:
// the file is submitted with the job, the job is polled until it reaches a
// terminal state, and every page comes back as Markdown plus layout blocks
// whose boxes are pixels of that page's image.
const (
	miniCPMOCRMaxPollInterval = 10 * time.Second
	miniCPMOCRMaxPollFailures = 3
	miniCPMOCRCancelTimeout   = 30 * time.Second
	// miniCPMOCRMaxResponseBytes bounds one job poll. Every poll repeats the
	// pages finished so far, so the limit covers a whole document.
	miniCPMOCRMaxResponseBytes int64 = 64 << 20
)

// miniCPMOCRPollInterval is the first wait between job polls; it grows up to
// miniCPMOCRMaxPollInterval. Tests shorten it.
var miniCPMOCRPollInterval = 2 * time.Second

// MiniCPMOCRModel drives the MiniCPM-OCR document parsing API.
type MiniCPMOCRModel struct {
	baseModel BaseModel
}

// NewMiniCPMOCRModel creates a MiniCPM-OCR driver.
func NewMiniCPMOCRModel(baseURL map[string]string, urlSuffix URLSuffix) *MiniCPMOCRModel {
	return &MiniCPMOCRModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: common.GetSSRFHTTPClient(),
		},
	}
}

func (m *MiniCPMOCRModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewMiniCPMOCRModel(baseURL, m.baseModel.URLSuffix)
}

func (m *MiniCPMOCRModel) Name() string {
	return "minicpm-ocr"
}

func (m *MiniCPMOCRModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MiniCPMOCRModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

func (m *MiniCPMOCRModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MiniCPMOCRModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MiniCPMOCRModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MiniCPMOCRModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

func (m *MiniCPMOCRModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MiniCPMOCRModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

// OCRFile parses content (a PDF or image) or, when content is empty, the
// document at the http(s) URL. The response carries the page Markdown as
// text and the located layout blocks of every page.
func (m *MiniCPMOCRModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	remoteURL := ""
	if url != nil {
		remoteURL = strings.TrimSpace(*url)
	}
	if len(content) == 0 && !strings.HasPrefix(remoteURL, "http://") && !strings.HasPrefix(remoteURL, "https://") {
		return nil, fmt.Errorf("file url or content is required")
	}
	jobsURL, err := m.baseModel.EndpointURL(apiConfig, m.baseModel.URLSuffix.OCR)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, longOpCallTimeout)
	defer cancel()

	req, err := m.newSubmitRequest(ctx, jobsURL, strings.TrimSpace(*modelName), content, remoteURL, apiConfig)
	if err != nil {
		return nil, fmt.Errorf("MiniCPM-OCR: %w", err)
	}
	job, err := m.send(req)
	if err != nil {
		return nil, fmt.Errorf("MiniCPM-OCR: submit job: %w", err)
	}
	if job.ID == "" {
		return nil, fmt.Errorf("MiniCPM-OCR: submit response carries no job id")
	}
	common.Info("MiniCPM-OCR job submitted", zap.String("job_id", job.ID), zap.Int("content_bytes", len(content)))

	job, err = m.wait(ctx, jobsURL+"/"+neturl.PathEscape(job.ID), job, apiConfig)
	if err != nil {
		return nil, fmt.Errorf("MiniCPM-OCR: %w", err)
	}
	response := job.response()
	common.Info("MiniCPM-OCR job finished", zap.String("job_id", job.ID), zap.Int("page_count", len(response.Pages)), zap.Int("text_len", len(*response.Text)))
	return response, nil
}

// newSubmitRequest uploads content as multipart form data, or references a
// public document URL in a JSON body.
func (m *MiniCPMOCRModel) newSubmitRequest(ctx context.Context, jobsURL, modelName string, content []byte, remoteURL string, apiConfig *APIConfig) (*http.Request, error) {
	if len(content) == 0 {
		return m.baseModel.newJSONPostRequest(ctx, jobsURL, apiConfig, map[string]any{"model": modelName, "url": remoteURL})
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", modelName)
	part, err := writer.CreateFormFile("file", ocrUploadFilename(content))
	if err != nil {
		return nil, err
	}
	if _, err = part.Write(content); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, jobsURL, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	m.baseModel.applyAuth(req, apiConfig)
	return req, nil
}

// wait polls the job until it reaches a terminal state. Transient poll
// failures are retried a few times. A job given up on is cancelled on the
// server, which counts active jobs against the account.
func (m *MiniCPMOCRModel) wait(ctx context.Context, jobURL string, job *miniCPMOCRJob, apiConfig *APIConfig) (*miniCPMOCRJob, error) {
	interval := miniCPMOCRPollInterval
	failures := 0
	for {
		switch job.Status {
		case "Success":
			return job, nil
		case "Failed":
			return nil, fmt.Errorf("job %s failed: %s", job.ID, job.errorMessage())
		case "Cancelled":
			return nil, fmt.Errorf("job %s was cancelled", job.ID)
		case "Running":
		default:
			return nil, fmt.Errorf("job %s returned unknown status %q", job.ID, job.Status)
		}
		select {
		case <-time.After(interval):
		case <-ctx.Done():
			m.cancel(ctx, jobURL, apiConfig)
			return nil, ctx.Err()
		}
		interval = min(interval*3/2, miniCPMOCRMaxPollInterval)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, jobURL, nil)
		if err != nil {
			return nil, err
		}
		m.baseModel.applyAuth(req, apiConfig)
		next, err := m.send(req)
		if err != nil {
			if ctx.Err() == nil && miniCPMOCRTransient(err) && failures < miniCPMOCRMaxPollFailures {
				failures++
				common.Warn("MiniCPM-OCR: job poll failed, retrying", zap.String("job_id", job.ID), zap.Error(err))
				continue
			}
			m.cancel(ctx, jobURL, apiConfig)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("poll job %s: %w", job.ID, err)
		}
		failures = 0
		job = next
	}
}

// miniCPMOCRTransient reports whether a failed poll may succeed when retried:
// a transport error, rate limiting or a server error.
func miniCPMOCRTransient(err error) bool {
	var statusErr *APIStatusError
	if !errors.As(err, &statusErr) {
		return true
	}
	return statusErr.Status == http.StatusTooManyRequests || statusErr.Status >= http.StatusInternalServerError
}

// cancel asks the server to stop the job. It runs after ctx is done, so it
// uses a detached context bounded by its own timeout.
func (m *MiniCPMOCRModel) cancel(ctx context.Context, jobURL string, apiConfig *APIConfig) {
	cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), miniCPMOCRCancelTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cancelCtx, http.MethodDelete, jobURL, nil)
	if err == nil {
		m.baseModel.applyAuth(req, apiConfig)
		var resp *http.Response
		if resp, err = m.baseModel.httpClient.Do(req); err == nil {
			resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode > 299 {
				err = fmt.Errorf("API request failed with status %d", resp.StatusCode)
			}
		}
	}
	if err != nil {
		common.Warn("MiniCPM-OCR: failed to cancel job", zap.String("job_url", jobURL), zap.Error(err))
	}
}

// send performs req and decodes the job carried by a 2xx reply.
func (m *MiniCPMOCRModel) send(req *http.Request) (*miniCPMOCRJob, error) {
	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, err := readModelErrorBody(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("API request failed with status %d; failed to read error response: %w", resp.StatusCode, err)
		}
		return nil, &APIStatusError{Status: resp.StatusCode, Body: string(body)}
	}
	body, err := readModelResponseBodyLimited(resp.Body, miniCPMOCRMaxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	var job miniCPMOCRJob
	if err = json.Unmarshal(body, &job); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &job, nil
}

type miniCPMOCRJob struct {
	ID     string           `json:"id"`
	Status string           `json:"status"`
	Pages  []miniCPMOCRPage `json:"pages"`
	// Error is a message string or an object carrying one.
	Error json.RawMessage `json:"error"`
}

type miniCPMOCRPage struct {
	Metadata struct {
		PageIndex int `json:"page_index"`
	} `json:"metadata"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	OCR struct {
		ImageWidth  float64           `json:"image_width"`
		ImageHeight float64           `json:"image_height"`
		Blocks      []miniCPMOCRBlock `json:"blocks"`
		Rotation    struct {
			Applied          bool    `json:"applied"`
			OrientationAngle float64 `json:"orientation_angle"`
		} `json:"rotation"`
	} `json:"ocr"`
}

type miniCPMOCRBlock struct {
	Order   int       `json:"order"`
	Label   string    `json:"label"`
	BBox    []float64 `json:"bbox"`
	Content string    `json:"content"`
	Status  string    `json:"status"`
}

func (job *miniCPMOCRJob) errorMessage() string {
	var message string
	if json.Unmarshal(job.Error, &message) == nil && message != "" {
		return message
	}
	var detail struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(job.Error, &detail) == nil && detail.Message != "" {
		return detail.Message
	}
	return "no error detail"
}

// response joins the page Markdown in page order and keeps the blocks of every
// page in reading order. Blocks the API leaves out of the page Markdown
// (headers, footers and their images, page numbers, footnotes and margin
// text) are dropped, so the blocks and the text describe the same content.
//
// The API turns a sideways or upside-down page upright before recognizing it,
// and its boxes then locate content on the turned page. Such a page keeps its
// text but not its boxes, which would point elsewhere on the source page. A
// page that was only deskewed by a few degrees keeps its boxes.
func (job *miniCPMOCRJob) response() *OCRFileResponse {
	pages := append([]miniCPMOCRPage(nil), job.Pages...)
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].Metadata.PageIndex < pages[j].Metadata.PageIndex })

	response := &OCRFileResponse{}
	texts := make([]string, 0, len(pages))
	for _, page := range pages {
		ocrPage := OCRPage{Index: page.Metadata.PageIndex}
		turned := page.OCR.Rotation.Applied && math.Mod(page.OCR.Rotation.OrientationAngle, 360) != 0
		if turned {
			response.Warnings = append(response.Warnings, fmt.Sprintf("MiniCPM-OCR page %d was turned %g degrees for recognition; its coordinates are omitted", page.Metadata.PageIndex+1, page.OCR.Rotation.OrientationAngle))
		} else {
			ocrPage.Width, ocrPage.Height = page.OCR.ImageWidth, page.OCR.ImageHeight
		}
		blocks := append([]miniCPMOCRBlock(nil), page.OCR.Blocks...)
		sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].Order < blocks[j].Order })
		for _, block := range blocks {
			switch strings.ToLower(strings.TrimSpace(block.Label)) {
			case "header", "header_image", "footer", "footer_image", "number", "footnote", "aside_text":
				continue
			}
			if block.Status == "failed" {
				response.Warnings = append(response.Warnings, fmt.Sprintf("MiniCPM-OCR page %d: %s block %d was not recognized", page.Metadata.PageIndex+1, block.Label, block.Order))
			}
			bbox := block.BBox
			if turned {
				bbox = nil
			}
			ocrPage.Blocks = append(ocrPage.Blocks, OCRBlock{Text: block.Content, Label: block.Label, BBox: bbox})
		}
		response.Pages = append(response.Pages, ocrPage)
		if len(page.Choices) > 0 {
			if text := strings.TrimSpace(page.Choices[0].Message.Content); text != "" {
				texts = append(texts, text)
			}
		}
	}
	text := strings.Join(texts, "\n\n")
	response.Text = &text
	return response
}

func (m *MiniCPMOCRModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MiniCPMOCRModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MiniCPMOCRModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MiniCPMOCRModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

func (m *MiniCPMOCRModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MiniCPMOCRModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MiniCPMOCRModel) Decide(ctx context.Context, modelName *string, request *DecideRequest, apiConfig *APIConfig, modelUsage *common.ModelUsage) (*DecideResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}
