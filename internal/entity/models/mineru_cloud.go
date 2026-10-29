//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//

package models

import (
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

const minerUAgentMaxBytes = 10 << 20

// IsMinerUCloudBase reports whether baseURL is the hosted mineru.net site.
// That host serves the marketing frontend; document parsing goes through
// /api/v4 (token) or /api/v1/agent (no token), not /file_parse or /v1/health.
func IsMinerUCloudBase(baseURL string) bool {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "mineru.net" || strings.HasSuffix(host, ".mineru.net")
}

// minerUCloudModelVersion maps a self-hosted backend name onto the cloud
// model_version field (pipeline or vlm).
func minerUCloudModelVersion(backend string) string {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "", "pipeline", "basic":
		return "pipeline"
	default:
		return "vlm"
	}
}

type minerUCloudEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type minerUCloudBatchApply struct {
	BatchID  string   `json:"batch_id"`
	FileURLs []string `json:"file_urls"`
}

type minerUCloudBatchResult struct {
	ExtractResult []minerUCloudExtract `json:"extract_result"`
}

type minerUCloudExtract struct {
	State      string `json:"state"`
	FullZipURL string `json:"full_zip_url"`
	ErrMsg     string `json:"err_msg"`
}

type minerUCloudAgentApply struct {
	TaskID  string `json:"task_id"`
	FileURL string `json:"file_url"`
}

type minerUCloudAgentResult struct {
	State       string `json:"state"`
	MarkdownURL string `json:"markdown_url"`
	ErrMsg      string `json:"err_msg"`
}

// ParseMinerUCloud uploads a local file to mineru.net and waits for the result.
// A bearer token uses the precise /api/v4 batch API (zip). Without a token it
// uses the no-login agent API (markdown only, 10MB).
func ParseMinerUCloud(ctx context.Context, baseURL, apiKey, filename string, content []byte, backend, parseMethod, language string) (*MinerUV1Result, error) {
	origin, err := minerUCloudOrigin(baseURL)
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("MinerU cloud requires file content")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, DefaultMinerUV1ParseTimeout)
	defer cancel()

	token := minerUBearerFromAPIKey(apiKey)
	client := minerUHTTPClient()
	name := minerUFilename(filename)
	lang := strings.TrimSpace(language)
	if lang == "" {
		lang = "ch"
	}
	ocr := strings.EqualFold(strings.TrimSpace(parseMethod), "ocr")

	if token != "" {
		return parseMinerUCloudV4(ctx, client, origin, token, name, content, minerUCloudModelVersion(backend), lang, ocr)
	}
	if len(content) > minerUAgentMaxBytes {
		return nil, fmt.Errorf("MinerU agent API accepts files up to 10MB; set a mineru.net API token to use /api/v4")
	}
	return parseMinerUCloudAgent(ctx, client, origin, name, content, lang, ocr)
}

func minerUCloudOrigin(baseURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("MinerU cloud base URL is invalid")
	}
	return u.Scheme + "://" + u.Host, nil
}

func parseMinerUCloudV4(ctx context.Context, client *http.Client, origin, token, filename string, content []byte, modelVersion, language string, ocr bool) (*MinerUV1Result, error) {
	body := map[string]any{
		"files": []map[string]any{{
			"name":   filename,
			"is_ocr": ocr,
		}},
		"model_version":  modelVersion,
		"language":       language,
		"enable_formula": true,
		"enable_table":   true,
	}
	var applied minerUCloudBatchApply
	if err := minerUCloudJSON(ctx, client, http.MethodPost, origin+"/api/v4/file-urls/batch", token, body, &applied); err != nil {
		return nil, fmt.Errorf("apply upload url: %w", err)
	}
	if applied.BatchID == "" || len(applied.FileURLs) == 0 || strings.TrimSpace(applied.FileURLs[0]) == "" {
		return nil, fmt.Errorf("MinerU cloud apply upload missing batch_id or file_urls")
	}
	if err := minerUCloudPUT(ctx, client, applied.FileURLs[0], content); err != nil {
		return nil, fmt.Errorf("upload file: %w", err)
	}

	var batch minerUCloudBatchResult
	if err := minerUCloudPoll(ctx, func() (string, error) {
		batch = minerUCloudBatchResult{}
		if err := minerUCloudJSON(ctx, client, http.MethodGet, origin+"/api/v4/extract-results/batch/"+applied.BatchID, token, nil, &batch); err != nil {
			return "", err
		}
		if len(batch.ExtractResult) == 0 {
			return "pending", nil
		}
		item := batch.ExtractResult[0]
		if item.State == "failed" {
			msg := strings.TrimSpace(item.ErrMsg)
			if msg == "" {
				msg = "failed"
			}
			return "", fmt.Errorf("MinerU cloud task failed: %s", msg)
		}
		return item.State, nil
	}); err != nil {
		return nil, err
	}
	zipURL := ""
	if len(batch.ExtractResult) > 0 {
		zipURL = strings.TrimSpace(batch.ExtractResult[0].FullZipURL)
	}
	if zipURL == "" {
		return nil, fmt.Errorf("MinerU cloud task done without full_zip_url")
	}
	zipBytes, err := minerUCloudGET(ctx, client, zipURL)
	if err != nil {
		return nil, fmt.Errorf("download zip: %w", err)
	}
	result := &MinerUV1Result{Zip: zipBytes}
	if md, err := minerUMarkdownFromZip(zipBytes); err == nil {
		result.Markdown = md
	}
	return result, nil
}

func parseMinerUCloudAgent(ctx context.Context, client *http.Client, origin, filename string, content []byte, language string, ocr bool) (*MinerUV1Result, error) {
	body := map[string]any{
		"file_name":      filename,
		"language":       language,
		"enable_table":   true,
		"enable_formula": true,
		"is_ocr":         ocr,
	}
	var applied minerUCloudAgentApply
	if err := minerUCloudJSON(ctx, client, http.MethodPost, origin+"/api/v1/agent/parse/file", "", body, &applied); err != nil {
		return nil, fmt.Errorf("apply agent upload: %w", err)
	}
	if applied.TaskID == "" || strings.TrimSpace(applied.FileURL) == "" {
		return nil, fmt.Errorf("MinerU agent apply missing task_id or file_url")
	}
	if err := minerUCloudPUT(ctx, client, applied.FileURL, content); err != nil {
		return nil, fmt.Errorf("upload file: %w", err)
	}

	var task minerUCloudAgentResult
	if err := minerUCloudPoll(ctx, func() (string, error) {
		task = minerUCloudAgentResult{}
		if err := minerUCloudJSON(ctx, client, http.MethodGet, origin+"/api/v1/agent/parse/"+applied.TaskID, "", nil, &task); err != nil {
			return "", err
		}
		if task.State == "failed" {
			msg := strings.TrimSpace(task.ErrMsg)
			if msg == "" {
				msg = "failed"
			}
			return "", fmt.Errorf("MinerU agent task failed: %s", msg)
		}
		return task.State, nil
	}); err != nil {
		return nil, err
	}
	mdURL := strings.TrimSpace(task.MarkdownURL)
	if mdURL == "" {
		return nil, fmt.Errorf("MinerU agent task done without markdown_url")
	}
	md, err := minerUCloudGET(ctx, client, mdURL)
	if err != nil {
		return nil, fmt.Errorf("download markdown: %w", err)
	}
	if strings.TrimSpace(string(md)) == "" {
		return nil, fmt.Errorf("MinerU agent markdown is empty")
	}
	return &MinerUV1Result{Markdown: string(md)}, nil
}

func minerUCloudPoll(ctx context.Context, step func() (string, error)) error {
	wait := minerUV1PollInitial
	for {
		state, err := step()
		if err != nil {
			return err
		}
		if state == "done" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		if wait < minerUV1PollMax {
			wait *= 2
			if wait > minerUV1PollMax {
				wait = minerUV1PollMax
			}
		}
	}
}

func minerUCloudJSON(ctx context.Context, client *http.Client, method, rawURL, token string, body any, dest any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	applyMinerUAuth(req, token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read %s: %w", rawURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, minerUCloudErrorText(payload))
	}
	var envelope minerUCloudEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return fmt.Errorf("decode %s: %w; body: %s", rawURL, err, minerUCloudErrorText(payload))
	}
	if envelope.Code != 0 {
		msg := strings.TrimSpace(envelope.Msg)
		if msg == "" {
			msg = "unknown error"
		}
		return fmt.Errorf("MinerU cloud code %d: %s", envelope.Code, msg)
	}
	if dest == nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, dest); err != nil {
		return fmt.Errorf("decode %s data: %w", rawURL, err)
	}
	return nil
}

func minerUCloudPUT(ctx context.Context, client *http.Client, rawURL string, content []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, rawURL, bytes.NewReader(content))
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, minerUCloudErrorText(payload))
	}
	return nil
}

func minerUCloudGET(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, minerUCloudErrorText(payload))
	}
	return payload, nil
}

func minerUCloudErrorText(payload []byte) string {
	text := strings.TrimSpace(string(payload))
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "<!doctype") || strings.HasPrefix(lower, "<html") {
		return "HTML error page"
	}
	if len(text) > 300 {
		return text[:300]
	}
	return text
}
