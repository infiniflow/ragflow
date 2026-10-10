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
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"ragflow/internal/common"
)

// minerUV1PollInitial and minerUV1PollMax bound GET /v1/parse/jobs/{id} polling.
// Tests may shorten minerUV1PollInitial.
var (
	minerUV1PollInitial = 2 * time.Second
	minerUV1PollMax     = 30 * time.Second
)

// DefaultMinerUV1ParseTimeout is used when ParseMinerUV1 receives a non-positive timeout.
const DefaultMinerUV1ParseTimeout = 30 * time.Minute

type minerUV1SupportCacheEntry struct {
	supported bool
	expires   time.Time
}

var minerUV1SupportCache sync.Map

const minerUV1SupportCacheTTL = 5 * time.Minute

type minerUV1UploadResponse struct {
	ID            string            `json:"id"`
	Status        string            `json:"status"`
	UploadURL     string            `json:"upload_url"`
	UploadMethod  string            `json:"upload_method"`
	UploadHeaders map[string]string `json:"upload_headers"`
	File          *minerUV1FileRef  `json:"file"`
}

type minerUV1FileRef struct {
	ID string `json:"id"`
}

// minerUV1ArtifactRef maps parse-job output_files entries (file_id) and legacy test mocks (id).
type minerUV1ArtifactRef struct {
	ID     string `json:"id"`
	FileID string `json:"file_id"`
}

func (r minerUV1ArtifactRef) artifactID() string {
	if id := strings.TrimSpace(r.FileID); id != "" {
		return id
	}
	return strings.TrimSpace(r.ID)
}

type minerUV1JobResponse struct {
	JobID  string            `json:"job_id"`
	Status string            `json:"status"`
	Error  *minerUV1Error    `json:"error"`
	Files  []minerUV1JobFile `json:"files"`
}

type minerUV1Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type minerUV1JobFile struct {
	Name        string                         `json:"name"`
	Status      string                         `json:"status"`
	Error       *minerUV1Error                 `json:"error"`
	OutputFiles map[string]minerUV1ArtifactRef `json:"output_files"`
}

// MinerUV1Result is the downloadable parse output from a MinerU 4 V1 job.
type MinerUV1Result struct {
	Zip      []byte
	Markdown string
}

func minerUHTTPClient() *http.Client {
	return common.GetSchemeSafeHTTPClient()
}

func minerUBearerFromAPIKey(apiKey string) string {
	return MinerUBearerTokenFromAPIKey(apiKey)
}

// MinerUV1OCRMode maps mineru_parse_method (auto, txt, ocr) onto the V1 job field ocr_mode.
func MinerUV1OCRMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "auto", "txt", "ocr":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "auto"
	}
}

func minerUV1SupportCacheKey(baseURL, apiKey string) string {
	token := minerUBearerFromAPIKey(apiKey)
	sum := sha256.Sum256([]byte(token))
	return baseURL + "\x00" + hex.EncodeToString(sum[:])
}

func minerUProbeGET(ctx context.Context, client *http.Client, rawURL, token string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

// MinerUSupportsV1 reports whether baseURL speaks MinerU 4's /v1 HTTP API.
// 404/405 mean a 3.x /file_parse service. 401/403 still count as V1 (the
// route exists; credentials may be wrong).
func MinerUSupportsV1(ctx context.Context, baseURL, apiKey string) (bool, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return false, fmt.Errorf("MinerU base URL is empty")
	}
	cacheKey := minerUV1SupportCacheKey(baseURL, apiKey)
	if cached, ok := minerUV1SupportCache.Load(cacheKey); ok {
		entry := cached.(minerUV1SupportCacheEntry)
		if time.Now().Before(entry.expires) {
			return entry.supported, nil
		}
	}
	token := minerUBearerFromAPIKey(apiKey)
	code, err := minerUProbeGET(ctx, minerUHTTPClient(), baseURL+"/v1/health", token)
	if err != nil {
		return false, err
	}
	var supported bool
	switch code {
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		supported = false
	default:
		supported = true
	}
	minerUV1SupportCache.Store(cacheKey, minerUV1SupportCacheEntry{
		supported: supported,
		expires:   time.Now().Add(minerUV1SupportCacheTTL),
	})
	return supported, nil
}

func applyMinerUAuth(req *http.Request, token string) {
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func minerUJSON(ctx context.Context, client *http.Client, method, rawURL, token string, body any, dest any) error {
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
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read %s %s: %w", method, rawURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("MinerU V1 %s %s: HTTP %d: %s", method, rawURL, resp.StatusCode, string(payload))
	}
	if dest == nil {
		return nil
	}
	if err := json.Unmarshal(payload, dest); err != nil {
		return fmt.Errorf("MinerU V1 decode %s: %w; body: %s", rawURL, err, string(payload))
	}
	return nil
}

func minerUSameOrigin(base, target string) bool {
	bu, err := url.Parse(base)
	if err != nil {
		return false
	}
	tu, err := url.Parse(target)
	if err != nil {
		return false
	}
	port := func(u *url.URL) string {
		if u.Port() != "" {
			return u.Port()
		}
		if strings.EqualFold(u.Scheme, "https") {
			return "443"
		}
		return "80"
	}
	return strings.EqualFold(bu.Scheme, tu.Scheme) &&
		strings.EqualFold(bu.Hostname(), tu.Hostname()) &&
		port(bu) == port(tu)
}

func minerUFilename(name string) string {
	base := path.Base(strings.TrimSpace(name))
	if base == "" || base == "." || base == "/" {
		return "document.pdf"
	}
	return base
}

func minerUMimeType(filename string) string {
	switch strings.ToLower(path.Ext(filename)) {
	case ".pdf":
		return "application/pdf"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".html", ".htm":
		return "text/html"
	default:
		return "application/octet-stream"
	}
}

func minerUV1Terminal(status string) bool {
	switch status {
	case "completed", "partial", "failed", "canceled":
		return true
	default:
		return false
	}
}

// ParseMinerUV1 runs the MinerU 4 V1 cycle: create upload → PUT bytes →
// complete → parse job → poll → download zip and/or markdown.
// ocrMode is the V1 ocr_mode (auto, txt, ocr); use MinerUV1OCRMode to normalize.
func ParseMinerUV1(ctx context.Context, baseURL, apiKey, filename string, content []byte, backend, ocrMode string, timeout time.Duration) (*MinerUV1Result, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("MinerU base URL is empty")
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("MinerU V1 requires file content")
	}
	if timeout <= 0 {
		timeout = DefaultMinerUV1ParseTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := minerUHTTPClient()
	token := minerUBearerFromAPIKey(apiKey)
	name := minerUFilename(filename)
	sum := sha256.Sum256(content)

	var created minerUV1UploadResponse
	if err := minerUJSON(ctx, client, http.MethodPost, baseURL+"/v1/uploads", token, map[string]any{
		"filename":  name,
		"bytes":     len(content),
		"mime_type": minerUMimeType(name),
		"purpose":   "parse",
		"sha256sum": hex.EncodeToString(sum[:]),
	}, &created); err != nil {
		return nil, fmt.Errorf("create upload: %w", err)
	}
	fileID := ""
	if created.File != nil {
		fileID = created.File.ID
	}
	switch created.Status {
	case "completed":
		if fileID == "" {
			return nil, fmt.Errorf("MinerU V1 upload completed without file.id")
		}
	case "pending":
		if created.UploadURL == "" {
			return nil, fmt.Errorf("MinerU V1 upload missing upload_url")
		}
		uploadURL, err := url.Parse(created.UploadURL)
		if err != nil {
			return nil, fmt.Errorf("invalid upload_url: %w", err)
		}
		if !uploadURL.IsAbs() {
			base, err := url.Parse(baseURL + "/")
			if err != nil {
				return nil, fmt.Errorf("invalid MinerU base URL: %w", err)
			}
			uploadURL = base.ResolveReference(uploadURL)
		}
		method := strings.ToUpper(strings.TrimSpace(created.UploadMethod))
		if method == "" {
			method = http.MethodPut
		}
		req, err := http.NewRequestWithContext(ctx, method, uploadURL.String(), bytes.NewReader(content))
		if err != nil {
			return nil, fmt.Errorf("create byte upload: %w", err)
		}
		for k, v := range created.UploadHeaders {
			req.Header.Set(k, v)
		}
		if minerUSameOrigin(baseURL, uploadURL.String()) {
			applyMinerUAuth(req, token)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("byte upload: %w", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("byte upload: HTTP %d", resp.StatusCode)
		}
		var completed minerUV1UploadResponse
		if err := minerUJSON(ctx, client, http.MethodPost, baseURL+"/v1/uploads/"+created.ID+"/complete", token, nil, &completed); err != nil {
			return nil, fmt.Errorf("complete upload: %w", err)
		}
		if completed.File == nil || completed.File.ID == "" {
			return nil, fmt.Errorf("MinerU V1 complete upload missing file.id")
		}
		fileID = completed.File.ID
	default:
		return nil, fmt.Errorf("MinerU V1 upload status %q", created.Status)
	}

	jobBody := map[string]any{
		"files": []map[string]any{
			{"source": map[string]any{"type": "file_id", "file_id": fileID}},
		},
		"tier":           MinerUTierFromBackend(backend),
		"output_formats": []string{"zip", "markdown"},
		"ocr_mode":       MinerUV1OCRMode(ocrMode),
	}
	var job minerUV1JobResponse
	if err := minerUJSON(ctx, client, http.MethodPost, baseURL+"/v1/parse/jobs", token, jobBody, &job); err != nil {
		return nil, fmt.Errorf("create parse job: %w", err)
	}
	if job.JobID == "" {
		return nil, fmt.Errorf("MinerU V1 parse job missing job_id")
	}

	pollWait := minerUV1PollInitial
	for !minerUV1Terminal(job.Status) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollWait):
		}
		if pollWait < minerUV1PollMax {
			pollWait *= 2
			if pollWait > minerUV1PollMax {
				pollWait = minerUV1PollMax
			}
		}
		if err := minerUJSON(ctx, client, http.MethodGet, baseURL+"/v1/parse/jobs/"+job.JobID, token, nil, &job); err != nil {
			return nil, fmt.Errorf("poll parse job: %w", err)
		}
	}
	switch job.Status {
	case "failed", "canceled":
		msg := job.Status
		if job.Error != nil && job.Error.Message != "" {
			msg = job.Error.Message
		}
		return nil, fmt.Errorf("MinerU V1 job %s: %s", job.Status, msg)
	}

	result := &MinerUV1Result{}
	var artifactErr error
	for _, file := range job.Files {
		if file.Status != "" && file.Status != "completed" {
			continue
		}
		if ref, ok := file.OutputFiles["zip"]; ok && ref.artifactID() != "" && len(result.Zip) == 0 {
			body, err := minerUV1DownloadFile(ctx, client, baseURL, token, ref.artifactID())
			if err != nil {
				if artifactErr == nil {
					artifactErr = fmt.Errorf("download zip: %w", err)
				}
			} else {
				result.Zip = body
			}
		}
		if ref, ok := file.OutputFiles["markdown"]; ok && ref.artifactID() != "" && result.Markdown == "" {
			body, err := minerUV1DownloadFile(ctx, client, baseURL, token, ref.artifactID())
			if err != nil {
				if artifactErr == nil {
					artifactErr = fmt.Errorf("download markdown: %w", err)
				}
			} else {
				result.Markdown = string(body)
			}
		}
	}
	if result.Markdown == "" && len(result.Zip) > 0 {
		if md, err := MinerUMarkdownFromZip(result.Zip); err == nil {
			result.Markdown = md
		}
	}
	if len(result.Zip) == 0 && strings.TrimSpace(result.Markdown) == "" {
		if artifactErr != nil {
			return nil, artifactErr
		}
		return nil, minerUV1NoArtifactsError(job)
	}
	return result, nil
}

func minerUV1NoArtifactsError(job minerUV1JobResponse) error {
	msg := fmt.Sprintf("MinerU V1 job %s (%s) completed without zip or markdown artifacts", job.JobID, job.Status)
	if fileMsg := minerUV1FirstFileErrorMessage(job.Files); fileMsg != "" {
		msg += ": " + fileMsg
	}
	return fmt.Errorf("%s", msg)
}

func minerUV1FirstFileErrorMessage(files []minerUV1JobFile) string {
	for _, file := range files {
		if file.Error != nil && strings.TrimSpace(file.Error.Message) != "" {
			return file.Error.Message
		}
	}
	return ""
}

func minerUV1DownloadHTTPClient(apiOrigin string) *http.Client {
	base := minerUHTTPClient()
	return &http.Client{
		Transport: base.Transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			if !minerUSameOrigin(apiOrigin, req.URL.String()) {
				req.Header.Del("Authorization")
			}
			return nil
		},
	}
}

func minerUV1DownloadFile(ctx context.Context, client *http.Client, baseURL, token, fileID string) ([]byte, error) {
	apiOrigin := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	dlClient := minerUV1DownloadHTTPClient(apiOrigin)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiOrigin+"/v1/files/"+fileID+"/content", nil)
	if err != nil {
		return nil, err
	}
	applyMinerUAuth(req, token)
	resp, err := dlClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// MinerUMarkdownFromZip prefers a .md member, then concatenates content_list.json text.
func MinerUMarkdownFromZip(zipBytes []byte) (string, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	var md string
	var contentList []byte
	for _, f := range zipReader.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if strings.Contains(name, "__MACOSX") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(rc)
		rc.Close()
		lower := strings.ToLower(name)
		if md == "" && strings.HasSuffix(lower, ".md") {
			md = string(body)
		}
		if strings.HasSuffix(lower, "content_list.json") {
			contentList = body
		}
	}
	if strings.TrimSpace(md) != "" {
		return md, nil
	}
	if len(contentList) == 0 {
		return "", fmt.Errorf("markdown and content_list.json not found in MinerU zip")
	}
	var items []map[string]any
	if err := json.Unmarshal(contentList, &items); err != nil {
		return "", fmt.Errorf("parse content_list.json: %w", err)
	}
	var parts []string
	for _, item := range items {
		if text, ok := item["text"].(string); ok && strings.TrimSpace(text) != "" {
			parts = append(parts, text)
			continue
		}
		if tb, ok := item["table_body"].(string); ok && strings.TrimSpace(tb) != "" {
			parts = append(parts, tb)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("content_list.json had no text")
	}
	return strings.Join(parts, "\n"), nil
}
