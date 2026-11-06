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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
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
	supported     bool
	supportsTasks bool
	decided       bool
	expires       time.Time
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

func minerUV1SupportCacheKey(baseURL, token string) string {
	sum := sha256.Sum256([]byte(token))
	return baseURL + "\x00" + hex.EncodeToString(sum[:])
}

// minerUProbeGET reports the HTTP status. MinerUSupportsV1 only distinguishes
// 404/405 from every other status, so a truncated health body must not hide it.
func minerUProbeGET(ctx context.Context, client *http.Client, rawURL, token string) (int, error) {
	code, _, err := minerUGET(ctx, client, rawURL, token)
	if code != 0 {
		return code, nil
	}
	return 0, err
}

func minerUParseHealthVersion(body []byte) (supportsTasks bool, decided bool) {
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || strings.TrimSpace(payload.Version) == "" {
		return false, false
	}
	major, minor, _, ok := minerUParseSemver(payload.Version)
	if !ok {
		return false, false
	}
	return minerUV1VersionSupportsTasks(major, minor), true
}

// MinerUSupportsV1 reports whether baseURL speaks MinerU 4's /v1 HTTP API.
// 404/405 mean a 3.x /file_parse service. 401/403 still count as V1 (the
// route exists; credentials may be wrong).
func MinerUSupportsV1(ctx context.Context, baseURL, apiKey string) (bool, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return false, fmt.Errorf("MinerU base URL is empty")
	}
	token := minerUBearerFromAPIKey(apiKey)
	cacheKey := minerUV1SupportCacheKey(baseURL, token)
	if cached, ok := minerUV1SupportCache.Load(cacheKey); ok {
		entry := cached.(minerUV1SupportCacheEntry)
		if time.Now().Before(entry.expires) {
			return entry.supported, nil
		}
	}
	code, body, err := minerUGET(ctx, minerUHTTPClient(), baseURL+"/v1/health", token)
	if err != nil && code == 0 {
		return false, err
	}
	var supported bool
	switch code {
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		supported = false
	default:
		supported = true
	}
	var supportsTasks, decided bool
	if supported && code >= 200 && code < 300 {
		supportsTasks, decided = minerUParseHealthVersion(body)
	}
	minerUV1SupportCache.Store(cacheKey, minerUV1SupportCacheEntry{
		supported:     supported,
		supportsTasks: supportsTasks,
		decided:       decided,
		expires:       time.Now().Add(minerUV1SupportCacheTTL),
	})
	return supported, nil
}

func applyMinerUAuth(req *http.Request, token string) {
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// minerUHTTPStatusError carries the HTTP status of a non-2xx MinerU response
// so callers can branch on it (the Tasks dispatch only falls back to the
// legacy cycle on 404). Its text matches the previous plain fmt error.
type minerUHTTPStatusError struct {
	Status int
	Err    error
}

func (e *minerUHTTPStatusError) Error() string { return e.Err.Error() }
func (e *minerUHTTPStatusError) Unwrap() error { return e.Err }

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
		return &minerUHTTPStatusError{
			Status: resp.StatusCode,
			Err:    fmt.Errorf("MinerU V1 %s %s: HTTP %d: %s", method, rawURL, resp.StatusCode, string(payload)),
		}
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

// minerUParseSemver parses strings like "4.0.12", "v4.1.0", "4.0", "4.1.0-alpha".
// Prerelease tags and build metadata are stripped. Returns ok=false on unparseable inputs.
func minerUParseSemver(v string) (major, minor, patch int, ok bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if v == "" {
		return 0, 0, 0, false
	}
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	if len(parts) == 0 {
		return 0, 0, 0, false
	}
	var err error
	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, false
	}
	if len(parts) > 1 {
		minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, 0, false
		}
	}
	if len(parts) > 2 {
		patch, _ = strconv.Atoi(parts[2])
	}
	return major, minor, patch, true
}

func minerUV1VersionSupportsTasks(major, minor int) bool {
	if major > 4 {
		return true
	}
	if major == 4 {
		return minor >= 1
	}
	return false
}

// minerUV1ServerSupportsTasks probes GET /v1/health to determine whether the server
// speaks the 4.0 legacy upload/job protocol or the 4.1+ Tasks API.
// A recognized version < 4.1 selects the 4.0 cycle; version >= 4.1 selects Tasks.
// If /v1/health is unavailable or omits version, decided is false and the caller
// probes /v1/tasks directly, falling back on 404/400.
// Results are cached in minerUV1SupportCache so repeated calls and callers that
// precede ParseMinerUV1 with MinerUSupportsV1 do not probe /v1/health twice.
func minerUV1ServerSupportsTasks(ctx context.Context, client *http.Client, baseURL, token string) (supportsTasks bool, decided bool) {
	cacheKey := minerUV1SupportCacheKey(baseURL, token)
	if cached, ok := minerUV1SupportCache.Load(cacheKey); ok {
		entry := cached.(minerUV1SupportCacheEntry)
		if time.Now().Before(entry.expires) {
			return entry.supportsTasks, entry.decided
		}
	}
	code, body, err := minerUGET(ctx, client, baseURL+"/v1/health", token)
	if err != nil || code < 200 || code >= 300 {
		return false, false
	}
	supportsTasks, decided = minerUParseHealthVersion(body)
	minerUV1SupportCache.Store(cacheKey, minerUV1SupportCacheEntry{
		supported:     true,
		supportsTasks: supportsTasks,
		decided:       decided,
		expires:       time.Now().Add(minerUV1SupportCacheTTL),
	})
	return supportsTasks, decided
}

// ParseMinerUV1 runs the MinerU 4 V1 parse cycle. When GET /v1/health advertises
// a known server version, versions < 4.1 (such as 4.0.x) run the 4.0 upload →
// complete → parse job → poll cycle directly, avoiding the 4.1 Tasks API where
// 4.0 servers return HTTP 400 (validation error on the 4.1 manifest).
// When /v1/health is absent or omits version, the 4.1 Tasks API runs first,
// falling back to the 4.0 cycle when POST /v1/tasks returns 404 (unavailable)
// or 400 (schema validation error on 4.0 servers). When the version is known
// to be >= 4.1, errors from the Tasks API are returned directly.
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

	supportsTasks, decided := minerUV1ServerSupportsTasks(ctx, client, baseURL, token)
	if decided && !supportsTasks {
		return parseMinerUV1Legacy(ctx, client, token, baseURL, name, content, sum, backend, ocrMode)
	}

	result, err := parseMinerUTasks(ctx, client, token, baseURL, name, content, sum, backend, ocrMode)
	if err == nil {
		return result, nil
	}
	if decided && supportsTasks {
		return nil, err
	}
	if !errors.Is(err, errMinerUTasksUnavailable) {
		return nil, err
	}
	return parseMinerUV1Legacy(ctx, client, token, baseURL, name, content, sum, backend, ocrMode)
}

// parseMinerUV1Legacy runs the MinerU 4.0 cycle: create upload → PUT bytes →
// complete → parse job → poll → download zip and/or markdown.
func parseMinerUV1Legacy(ctx context.Context, client *http.Client, token, baseURL, name string, content []byte, sum [32]byte, backend, ocrMode string) (*MinerUV1Result, error) {
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
		if err := minerUPutUploadBytes(ctx, token, baseURL, created.UploadMethod, created.UploadURL, created.UploadHeaders, content); err != nil {
			return nil, err
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
			body, err := minerUDownloadFile(ctx, baseURL, token, baseURL+"/v1/files/"+ref.artifactID()+"/content")
			if err != nil {
				if artifactErr == nil {
					artifactErr = fmt.Errorf("download zip: %w", err)
				}
			} else {
				result.Zip = body
			}
		}
		if ref, ok := file.OutputFiles["markdown"]; ok && ref.artifactID() != "" && result.Markdown == "" {
			body, err := minerUDownloadFile(ctx, baseURL, token, baseURL+"/v1/files/"+ref.artifactID()+"/content")
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
		if md, err := minerUMarkdownFromZip(result.Zip); err == nil {
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

// minerUPutUploadBytes uploads the raw file bytes to an upload URL with the
// given method (PUT unless the server said otherwise), applying the supplied
// headers and attaching the bearer token only when the URL is same-origin
// with the API base, and stripping the token on any cross-origin redirect so
// signed object-storage links never see the token.
func minerUPutUploadBytes(ctx context.Context, token, baseURL, method, rawURL string, headers map[string]string, content []byte) error {
	uploadURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid upload_url: %w", err)
	}
	if !uploadURL.IsAbs() {
		base, err := url.Parse(baseURL + "/")
		if err != nil {
			return fmt.Errorf("invalid MinerU base URL: %w", err)
		}
		uploadURL = base.ResolveReference(uploadURL)
	}
	if method = strings.ToUpper(strings.TrimSpace(method)); method == "" {
		method = http.MethodPut
	}
	req, err := http.NewRequestWithContext(ctx, method, uploadURL.String(), bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("create byte upload: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	apiOrigin := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if minerUSameOrigin(apiOrigin, uploadURL.String()) {
		applyMinerUAuth(req, token)
	}
	resp, err := minerUV1DownloadHTTPClient(apiOrigin).Do(req)
	if err != nil {
		return fmt.Errorf("byte upload: %w", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("byte upload: HTTP %d", resp.StatusCode)
	}
	return nil
}

// minerUDownloadFile GETs a MinerU artifact URL. Legacy 4.0 output_files ids
// resolve to /v1/files/{id}/content; 4.1 tasks return the result_url directly
// (a Files URL on local services, a signed object-storage link on SaaS). The
// bearer token is attached only to same-origin URLs and stripped again on
// every cross-origin redirect.
func minerUDownloadFile(ctx context.Context, baseURL, token, rawURL string) ([]byte, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid download url: %w", err)
	}
	if !parsedURL.IsAbs() {
		base, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/")
		if err != nil {
			return nil, fmt.Errorf("invalid MinerU base URL: %w", err)
		}
		parsedURL = base.ResolveReference(parsedURL)
	}
	apiOrigin := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	dlClient := minerUV1DownloadHTTPClient(apiOrigin)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return nil, err
	}
	if minerUSameOrigin(apiOrigin, parsedURL.String()) {
		applyMinerUAuth(req, token)
	}
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

// errMinerUTasksUnavailable marks a MinerU server without the 4.1 Tasks API:
// POST /v1/tasks returned 404, so the caller falls back to the 4.0 legacy
// cycle instead of surfacing the routing error.
var errMinerUTasksUnavailable = errors.New("MinerU V1 tasks API unavailable")

// MinerU 4.1 Tasks API payload. Every response is a complete snapshot whose
// absent fields carry no value: an upload slot only exists while the file is
// actually waiting for bytes.
type minerUTaskUpload struct {
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt int64             `json:"expires_at"`
}

type minerUTaskFile struct {
	FileID    string            `json:"file_id"`
	FileName  string            `json:"file_name"`
	Status    string            `json:"status"`
	Upload    *minerUTaskUpload `json:"upload"`
	ResultURL string            `json:"result_url"`
	Error     *minerUV1Error    `json:"error"`
}

// minerUTaskResponse decodes both the 202 creation receipt and the GET detail
// snapshot.
type minerUTaskResponse struct {
	TaskID string           `json:"task_id"`
	Status string           `json:"status"`
	Error  *minerUV1Error   `json:"error"`
	Files  []minerUTaskFile `json:"files"`
}

// parseMinerUTasks runs the MinerU 4.1 Tasks cycle: POST /v1/tasks with a
// JSON manifest → PUT raw bytes to the returned upload URL (no completion
// call) → poll GET /v1/tasks/{id} → download the file's result_url.
// output_formats ["default"] yields one ZIP per input holding markdown.md
// plus the other general results, which minerUMarkdownFromZip unpacks.
// Creation returns errMinerUTasksUnavailable on 404 (absent) or 400 (4.0 servers
// validating against legacy schema) so ParseMinerUV1 can run the legacy 4.0
// cycle when version was not decided by health; every other creation failure
// is a real error (401/503 carry protocol meaning and must not be masked by a
// legacy retry).
func parseMinerUTasks(ctx context.Context, client *http.Client, token, baseURL, name string, content []byte, sum [32]byte, backend, ocrMode string) (*MinerUV1Result, error) {
	var created minerUTaskResponse
	if err := minerUJSON(ctx, client, http.MethodPost, baseURL+"/v1/tasks", token, map[string]any{
		"files": []map[string]any{{
			"file_name": name,
			"bytes":     len(content),
			"sha256sum": hex.EncodeToString(sum[:]),
		}},
		"tier":           MinerUTierFromBackend(backend),
		"ocr_mode":       MinerUV1OCRMode(ocrMode),
		"output_formats": []string{"default"},
	}, &created); err != nil {
		var statusErr *minerUHTTPStatusError
		if errors.As(err, &statusErr) && (statusErr.Status == http.StatusNotFound || statusErr.Status == http.StatusBadRequest) {
			return nil, fmt.Errorf("%w: %w", errMinerUTasksUnavailable, err)
		}
		return nil, fmt.Errorf("create task: %w", err)
	}
	if created.TaskID == "" {
		return nil, fmt.Errorf("MinerU V1 tasks creation missing task_id")
	}
	if len(created.Files) == 0 {
		return nil, fmt.Errorf("MinerU V1 tasks creation missing files")
	}

	// Upload only when a snapshot offers a slot: the receipt can arrive before
	// the upload is prepared, and a waiting file without an upload means poll
	// on. A successful PUT is never repeated.
	uploaded := false
	if upload := created.Files[0].Upload; upload != nil {
		if err := minerUPutUploadBytes(ctx, token, baseURL, http.MethodPut, upload.URL, upload.Headers, content); err != nil {
			return nil, err
		}
		uploaded = true
	}

	task := &created
	pollWait := minerUV1PollInitial
	for !minerUV1Terminal(task.Status) {
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
		var detail minerUTaskResponse
		if err := minerUJSON(ctx, client, http.MethodGet, baseURL+"/v1/tasks/"+url.PathEscape(created.TaskID), token, nil, &detail); err != nil {
			return nil, fmt.Errorf("poll task: %w", err)
		}
		task = &detail
		if !uploaded && len(task.Files) > 0 && task.Files[0].Upload != nil {
			if err := minerUPutUploadBytes(ctx, token, baseURL, http.MethodPut, task.Files[0].Upload.URL, task.Files[0].Upload.Headers, content); err != nil {
				return nil, err
			}
			uploaded = true
		}
	}
	switch task.Status {
	case "failed", "canceled":
		return nil, minerUTaskStatusError(task.Status, task.Error, task.Files)
	}

	result := &MinerUV1Result{}
	var markdownParts []string
	for _, file := range task.Files {
		if file.Status != "completed" || file.ResultURL == "" {
			continue
		}
		body, err := minerUDownloadFile(ctx, baseURL, token, file.ResultURL)
		if err != nil {
			return nil, fmt.Errorf("download task result: %w", err)
		}
		if len(result.Zip) == 0 {
			result.Zip = body
		}
		if md, err := minerUMarkdownFromZip(body); err == nil && strings.TrimSpace(md) != "" {
			markdownParts = append(markdownParts, md)
		}
	}
	if len(markdownParts) == 1 {
		result.Markdown = markdownParts[0]
	} else if len(markdownParts) > 1 {
		for i := range markdownParts {
			markdownParts[i] = strings.TrimRight(markdownParts[i], "\r\n")
		}
		result.Markdown = strings.Join(markdownParts, "\n\n") + "\n"
	}
	if len(result.Zip) == 0 && strings.TrimSpace(result.Markdown) == "" {
		return nil, minerUTaskStatusError(task.Status, task.Error, task.Files)
	}
	return result, nil
}

// minerUTaskStatusError names a terminal task status and its first error:
// the snapshot-level error wins over per-file errors, matching how the
// response nests them.
func minerUTaskStatusError(status string, taskErr *minerUV1Error, files []minerUTaskFile) error {
	msg := status
	if taskErr != nil && strings.TrimSpace(taskErr.Message) != "" {
		msg = taskErr.Message
	} else {
		for i := range files {
			if files[i].Error != nil && strings.TrimSpace(files[i].Error.Message) != "" {
				msg = files[i].Error.Message
				break
			}
		}
	}
	return fmt.Errorf("MinerU V1 task %s: %s", status, msg)
}

// minerUMarkdownFromZip prefers a .md member, then content_list.json text,
// then MinerU 4 structured_content.json block text.
func minerUMarkdownFromZip(zipBytes []byte) (string, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	var md string
	var contentList, structured []byte
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
		if len(contentList) == 0 && strings.HasSuffix(lower, "content_list.json") {
			contentList = body
		}
		if len(structured) == 0 && strings.HasSuffix(lower, "structured_content.json") {
			structured = body
		}
	}
	if strings.TrimSpace(md) != "" {
		return md, nil
	}
	var contentErr error
	if len(contentList) > 0 {
		text, err := minerUMarkdownFromContentList(contentList)
		if err == nil && strings.TrimSpace(text) != "" {
			return text, nil
		}
		contentErr = err
	}
	if len(structured) > 0 {
		return minerUMarkdownFromStructured(structured)
	}
	if contentErr != nil {
		return "", contentErr
	}
	return "", fmt.Errorf("markdown, content_list.json, and structured_content.json not found in MinerU zip")
}

func minerUMarkdownFromContentList(raw []byte) (string, error) {
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
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

func minerUMarkdownFromStructured(raw []byte) (string, error) {
	var doc struct {
		Pages []struct {
			Blocks []map[string]any `json:"blocks"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("parse structured_content.json: %w", err)
	}
	var parts []string
	for _, page := range doc.Pages {
		for _, block := range page.Blocks {
			typ, _ := block["type"].(string)
			switch strings.ToLower(strings.TrimSpace(typ)) {
			case "header", "footer", "page_number", "page-header", "page-footer", "page_header", "page_footer", "aside_text", "discarded":
				continue
			}
			if content, _ := block["content"].(string); strings.TrimSpace(content) != "" {
				parts = append(parts, content)
			}
			parts = append(parts, minerUAnnotationContents(block["captions"])...)
			parts = append(parts, minerUAnnotationContents(block["footnotes"])...)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("structured_content.json had no text")
	}
	return strings.Join(parts, "\n"), nil
}

func minerUAnnotationContents(raw any) []string {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range items {
		switch value := item.(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				out = append(out, value)
			}
		case map[string]any:
			if text, _ := value["content"].(string); strings.TrimSpace(text) != "" {
				out = append(out, text)
			}
		}
	}
	return out
}
