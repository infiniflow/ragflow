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

// PaddleOCR client for the async Job API.
// Mirrors Python's deepdoc/parser/paddleocr_parser.py:
//   - Submit job via POST /api/v2/ocr/jobs
//   - Poll with exponential backoff until "done" or "failed"
//   - Fetch result JSONL and extract text
//
// For image parsing, only parse_image() semantics are needed;
// parse_pdf() is handled by the PDF vision dispatch path.

package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
	"time"

	"ragflow/internal/common"
)

// imgTagPattern matches HTML <img> tags and <div> wrappers, mirroring
// Python's _MARKDOWN_IMAGE_PATTERN in deepdoc/parser/paddleocr_parser.py.
var imgTagPattern = regexp.MustCompile(
	`(?is)<div[^>]*>\s*<img[^>]*/>\s*</div>|<img[^>]*/>`,
)

const (
	defaultPaddleOCRBaseURL  = "https://paddleocr.aistudio-app.com"
	defaultPaddleOCRTimeout  = 600 * time.Second
	defaultPaddleOCRAlgo     = "PaddleOCR-VL"
	paddleOCRSubmitPath      = "/api/v2/ocr/jobs"
	paddleOCRPollInterval    = 3 * time.Second
	paddleOCRPollMultiplier  = 1.5
	paddleOCRPollMaxInterval = 15 * time.Second
)

// PaddleOCRClient talks to the PaddleOCR async Job API.
type PaddleOCRClient struct {
	BaseURL     string
	AccessToken string
	Algorithm   string
	Timeout     time.Duration
	httpClient  *http.Client
}

// NewPaddleOCRClientFromEnv creates a client from environment variables:
//
//	PADDLEOCR_BASE_URL       – base URL (default https://paddleocr.aistudio-app.com)
//	PADDLEOCR_ACCESS_TOKEN   – bearer token
//	PADDLEOCR_ALGORITHM      – algorithm name (default PaddleOCR-VL)
func NewPaddleOCRClientFromEnv() *PaddleOCRClient {
	baseURL := common.GetEnv(common.EnvPaddleOCRBaseUrl)
	if baseURL == "" {
		baseURL = defaultPaddleOCRBaseURL
	}
	return &PaddleOCRClient{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		AccessToken: common.GetEnv(common.EnvPaddleOCRAccessToken),
		Algorithm:   firstNonEmpty(common.GetEnv(common.EnvPaddleOCRAlgorithm), defaultPaddleOCRAlgo),
		Timeout:     defaultPaddleOCRTimeout,
		httpClient: &http.Client{
			Timeout: 30 * time.Second, // per-request timeout; polling uses deadline
		},
	}
}

// Enabled reports whether the client has a usable access token.
func (c *PaddleOCRClient) Enabled() bool {
	return c != nil && c.AccessToken != ""
}

// ParseImage submits the image to PaddleOCR and returns extracted text.
// Mirrors Python's PaddleOCRParser.parse_image().
func (c *PaddleOCRClient) ParseImage(binary []byte, filename string) (string, error) {
	deadline := time.Now().Add(c.Timeout)

	// Step 1: Submit job
	jobID, err := c.submitJob(binary, filename, deadline)
	if err != nil {
		return "", fmt.Errorf("paddleocr submit: %w", err)
	}

	// Step 2: Poll until done
	resultData, err := c.pollJob(jobID, deadline)
	if err != nil {
		return "", fmt.Errorf("paddleocr poll: %w", err)
	}

	// Step 3: Fetch result JSONL
	raw, err := c.fetchResult(resultData, deadline)
	if err != nil {
		return "", fmt.Errorf("paddleocr fetch: %w", err)
	}

	// Step 4: Parse result
	return c.extractImageText(raw), nil
}

// submitJob POSTs the image file to /api/v2/ocr/jobs and returns the job ID.
func (c *PaddleOCRClient) submitJob(binary []byte, filename string, deadline time.Time) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Write the image file field
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(binary); err != nil {
		return "", err
	}

	// Write form fields
	if err := w.WriteField("model", c.Algorithm); err != nil {
		return "", err
	}
	optionalPayload := map[string]any{"formatBlockContent": true}
	payloadBytes, _ := json.Marshal(optionalPayload)
	if err := w.WriteField("optionalPayload", string(payloadBytes)); err != nil {
		return "", err
	}
	w.Close()

	url := c.BaseURL + paddleOCRSubmitPath
	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Client-Platform", "ragflow")
	if c.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	}
	req = req.WithContext(withDeadline(req.Context(), deadline))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var submitResp struct {
		Data struct {
			JobID string `json:"jobId"`
		} `json:"data"`
		JobID string `json:"jobId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&submitResp); err != nil {
		return "", fmt.Errorf("decode submit response: %w", err)
	}
	jobID := submitResp.Data.JobID
	if jobID == "" {
		jobID = submitResp.JobID
	}
	if jobID == "" {
		return "", fmt.Errorf("no jobId in submit response")
	}
	slog.Info("paddleocr: job submitted", "jobId", jobID)
	return jobID, nil
}

// pollJob polls the job status until it reaches "done" or "failed".
// Uses exponential backoff: 3s initial, 1.5x multiplier, 15s max.
// Returns the final poll response data.
func (c *PaddleOCRClient) pollJob(jobID string, deadline time.Time) (map[string]any, error) {
	pollURL := fmt.Sprintf("%s/%s", c.BaseURL+paddleOCRSubmitPath, jobID)
	interval := paddleOCRPollInterval

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("job %s timed out after %v", jobID, c.Timeout)
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("job %s timed out", jobID)
		}

		req, err := http.NewRequest(http.MethodGet, pollURL, nil)
		if err != nil {
			return nil, err
		}
		if c.AccessToken != "" {
			req.Header.Set("Authorization", "Bearer "+c.AccessToken)
		}
		req.Header.Set("Client-Platform", "ragflow")
		req = req.WithContext(withDeadline(req.Context(), deadline))

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("poll %s: %w", jobID, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			return nil, fmt.Errorf("poll HTTP %d: %s", resp.StatusCode, string(body))
		}

		var pollResp struct {
			Data struct {
				State    string `json:"state"`
				ErrorMsg string `json:"errorMsg"`
			} `json:"data"`
			State    string `json:"state"`
			ErrorMsg string `json:"errorMsg"`
		}
		bodyBytes, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(bodyBytes, &pollResp); err != nil {
			return nil, fmt.Errorf("decode poll response: %w", err)
		}
		state := pollResp.Data.State
		if state == "" {
			state = pollResp.State
		}

		switch state {
		case "done":
			slog.Info("paddleocr: job done", "jobId", jobID)
			// Return full data so caller can extract resultJsonUrl
			var fullData map[string]any
			json.Unmarshal(bodyBytes, &fullData)
			return fullData, nil
		case "failed":
			errMsg := pollResp.Data.ErrorMsg
			if errMsg == "" {
				errMsg = pollResp.ErrorMsg
			}
			return nil, fmt.Errorf("job %s failed: %s", jobID, errMsg)
		}

		// Exponential backoff
		sleepTime := interval
		if remaining < sleepTime {
			sleepTime = remaining
		}
		time.Sleep(sleepTime)
		interval = time.Duration(float64(interval) * paddleOCRPollMultiplier)
		if interval > paddleOCRPollMaxInterval {
			interval = paddleOCRPollMaxInterval
		}
	}
}

// fetchResult downloads and parses the JSONL result from resultJsonUrl.
func (c *PaddleOCRClient) fetchResult(pollData map[string]any, deadline time.Time) ([]map[string]any, error) {
	data, _ := pollData["data"].(map[string]any)
	if data == nil {
		data = pollData
	}
	resultJSONURL, _ := data["resultJsonUrl"].(string)
	if resultJSONURL == "" {
		if resultURL, ok := data["resultUrl"].(map[string]any); ok {
			resultJSONURL, _ = resultURL["jsonUrl"].(string)
		}
	}
	if resultJSONURL == "" {
		return nil, fmt.Errorf("no resultJsonUrl in completion response")
	}

	req, err := http.NewRequest(http.MethodGet, resultJSONURL, nil)
	if err != nil {
		return nil, err
	}
	if c.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	}
	req = req.WithContext(withDeadline(req.Context(), deadline))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch result: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("fetch result HTTP %d: %s", resp.StatusCode, string(body))
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read result: %w", err)
	}

	// Parse JSONL: one JSON object per line
	var lines []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			return nil, fmt.Errorf("parse JSONL line: %w", err)
		}
		lines = append(lines, obj)
	}
	return lines, nil
}

// extractImageText extracts text from PaddleOCR JSONL results.
// Mirrors Python's parse_image() text extraction:
//
//	layoutParsingResults[].prunedResult.parsing_res_list[].block_content
//	Fallback: ocrResults[].prunedResult.rec_texts[]
func (c *PaddleOCRClient) extractImageText(resultLines []map[string]any) string {
	var texts []string

	for _, line := range resultLines {
		result, _ := line["result"].(map[string]any)
		if result == nil {
			continue
		}
		// Primary: layoutParsingResults
		if lpr, ok := result["layoutParsingResults"].([]any); ok {
			for _, lr := range lpr {
				lrMap, _ := lr.(map[string]any)
				if lrMap == nil {
					continue
				}
				pruned, _ := lrMap["prunedResult"].(map[string]any)
				if pruned == nil {
					continue
				}
				if prl, ok := pruned["parsing_res_list"].([]any); ok {
					for _, block := range prl {
						blockMap, _ := block.(map[string]any)
						if blockMap == nil {
							continue
						}
						content, _ := blockMap["block_content"].(string)
						content = strings.TrimSpace(content)
						// Remove markdown image blocks
						content = removeMarkdownImages(content)
						if content != "" {
							texts = append(texts, content)
						}
					}
				}
			}
		}
	}

	// Fallback: ocrResults for text-only models (e.g. PP-OCRv6)
	if len(texts) == 0 {
		for _, line := range resultLines {
			result, _ := line["result"].(map[string]any)
			if result == nil {
				continue
			}
			if ocr, ok := result["ocrResults"].([]any); ok {
				for _, o := range ocr {
					ocrMap, _ := o.(map[string]any)
					if ocrMap == nil {
						continue
					}
					pruned, _ := ocrMap["prunedResult"].(map[string]any)
					if pruned == nil {
						continue
					}
					if recTexts, ok := pruned["rec_texts"].([]any); ok {
						for _, t := range recTexts {
							if s, ok := t.(string); ok {
								s = strings.TrimSpace(s)
								if s != "" {
									texts = append(texts, s)
								}
							}
						}
					}
				}
			}
		}
	}

	if len(texts) == 0 {
		return ""
	}
	return strings.Join(texts, "\n")
}

// removeMarkdownImages strips markdown image syntax (![](...)) and HTML
// img/div blocks, mirroring Python's _remove_images_from_markdown.
func removeMarkdownImages(md string) string {
	// Strip HTML <img> and <div><img/></div> blocks
	md = imgTagPattern.ReplaceAllString(md, "")
	return strings.TrimSpace(md)
}

// withDeadline applies a deadline to a context, respecting the existing
// deadline if it is sooner.
func withDeadline(ctx context.Context, deadline time.Time) context.Context {
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		return ctx
	}
	newCtx, _ := context.WithDeadline(ctx, deadline)
	return newCtx
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
