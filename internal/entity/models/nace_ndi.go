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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/textproto"
	neturl "net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ragflow/internal/common"

	"go.uber.org/zap"
)

// NDI (Nace Document Intelligence) parses documents through the Nace documents
// API. A PDF is uploaded to the caller's Nace workspace with a single-use
// upload grant and parsed by a job, which is read until it ends. Every layout
// block comes back with its Markdown and a region given as fractions of the
// page. When the parse is over, the job and the uploaded file are deleted, so
// no copy of the document stays in the account.
const (
	naceNDIParseSuffix  = "documents/parse"
	naceNDIGrantsPath   = "documents/upload-grants"
	naceNDIJobsPath     = "documents/jobs"
	naceNDIFilesPath    = "documents/workspaces"
	naceNDIModelPrefix  = "ndi-parse-"
	naceNDIUploadPrefix = "ragflow/"

	// naceNDIPollWaitSeconds holds each job read until the job ends, up to
	// the 25 s the API allows.
	naceNDIPollWaitSeconds = 25
	// An account with zero retention refuses a create that does not wait, and
	// keeps no result to read later: it gets the result only in the create
	// answer, held up to naceNDIZeroRetentionWaitSeconds.
	naceNDIZeroRetentionCode        = "wait_required_for_zero_retention"
	naceNDIZeroRetentionWaitSeconds = 60

	// NDI fails a job whose page_ranges run far past the last page with the
	// code naceNDIOCRFailedCode and a message that starts with
	// naceNDINoPagesMessage ("No renderable pages in <file>.").
	naceNDIOCRFailedCode  = "ocr_failed"
	naceNDINoPagesMessage = "No renderable pages"

	naceNDIMaxPollInterval = 10 * time.Second
	// naceNDIMaxAttempts bounds the sends of one request: the first and two
	// retries. A cleanup DELETE is sent at most naceNDIDeleteAttempts times.
	naceNDIMaxAttempts    = 3
	naceNDIDeleteAttempts = 2
	// naceNDIMaxRetryAfter is the longest Retry-After honoured; a longer one
	// falls back to the usual backoff.
	naceNDIMaxRetryAfter = 30 * time.Second
	// naceNDIStatusOverloaded is NDI's 529 (service_disabled: the API is
	// paused), which carries a Retry-After like 429 and 503.
	naceNDIStatusOverloaded = 529
	naceNDIUploadTTL        = 600
	// naceNDIResponseHeaderTimeout is the default wait for an answer's
	// headers; a held read takes up to 25 s and a held create up to 60 s.
	// LLM_HTTP_RESPONSE_HEADER_TIMEOUT cannot set it below
	// naceNDIMinResponseHeaderTimeout, so a held call is not cut off before NDI
	// answers.
	naceNDIResponseHeaderTimeout    = 5 * time.Minute
	naceNDIMinResponseHeaderTimeout = (naceNDIZeroRetentionWaitSeconds + 30) * time.Second
	// naceNDIMaxUploadBytes is the largest PDF NDI accepts in one upload.
	naceNDIMaxUploadBytes = 256 << 20
	// naceNDIPDFHeaderWindow is how far into the file the PDF header may
	// start; PDF readers accept leading bytes within the first kilobyte.
	naceNDIPDFHeaderWindow = 1024
	// naceNDIMaxResponseBytes bounds one job read, which carries the whole
	// parse result.
	naceNDIMaxResponseBytes int64 = 64 << 20
	// naceNDIMaxErrorText bounds the provider text quoted in an error.
	naceNDIMaxErrorText = 300
	// NDI accepts up to naceNDIMaxRanges page ranges and naceNDIMaxRangePages
	// pages in all in one parse.
	naceNDIMaxRanges     = 200
	naceNDIMaxRangePages = 500
	// naceNDIMaxPages bounds the page list built from one result, so a page
	// number in the response cannot size an arbitrary allocation.
	naceNDIMaxPages = 10000
)

// naceNDIPollInterval is the first pause between job reads that come back
// unfinished; it grows up to naceNDIMaxPollInterval. naceNDIRetryDelay is the
// pause before a transient failure is retried. Tests shorten them.
var (
	naceNDIPollInterval = time.Second
	naceNDIRetryDelay   = time.Second
	// naceNDICleanupTimeout bounds the deletion of one job or file, so the
	// cleanup after a parse (the job, then the uploaded file) takes at most
	// twice this when NDI does not answer.
	naceNDICleanupTimeout = 30 * time.Second
	// naceNDIDeletePollDelay is the pause before a file deletion answered 202
	// without Retry-After is checked again.
	naceNDIDeletePollDelay = time.Second
	// naceNDIDeleteConfirmTimeout bounds the checks of a file deletion
	// answered 202, counted from the first DELETE. NDI finishes an accepted
	// deletion on its own, so the checks only confirm it for the log; with a
	// prompt 202 the file deletion ends within this time.
	naceNDIDeleteConfirmTimeout = 10 * time.Second
)

// naceNDIAPITransport sends every NDI API call (upload grant, parse job
// create, read and delete, file delete, connection check) and
// naceNDIUploadTransport the upload, which carries the document. Neither is
// wrapped in the provider logging transport: NDI requests and answers carry the
// upload URL and token, signed source links and the parsed document text, so
// LLM_DEBUG must not write them to the log. Both use naceNDISharedTransport
// unless a test sets them.
var naceNDIAPITransport, naceNDIUploadTransport http.RoundTripper

// naceNDISharedTransport is built on first use, like the shared provider
// clients, and honours the same LLM_HTTP_RESPONSE_HEADER_TIMEOUT override.
var naceNDISharedTransport = sync.OnceValue(naceNDINewTransport)

func naceNDINewTransport() http.RoundTripper {
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = base.Clone()
	}
	transport.TLSHandshakeTimeout = 30 * time.Second
	transport.ResponseHeaderTimeout = naceNDIResponseHeaderTimeout
	if raw := strings.TrimSpace(common.GetEnv(common.EnvLLMHTTPResponseHeaderTimeout)); raw != "" {
		if timeout, err := time.ParseDuration(raw); err == nil && timeout > 0 {
			transport.ResponseHeaderTimeout = max(timeout, naceNDIMinResponseHeaderTimeout)
		}
	}
	return transport
}

func naceNDITransport(transport http.RoundTripper) http.RoundTripper {
	if transport != nil {
		return transport
	}
	return naceNDISharedTransport()
}

// naceNDISchemeSafeTransport refuses a request whose URL scheme or host the
// repo's lenient guard rejects; private hosts are allowed, because the API
// base URL is the operator's choice.
type naceNDISchemeSafeTransport struct{ base http.RoundTripper }

func (t naceNDISchemeSafeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := common.AssertURLSchemeSafe(req.URL.String()); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

// ndiClient is the client of every NDI API call. It follows no redirect: the
// redirect answer itself comes back, as a refused call, so neither the key,
// the idempotency key nor a request body goes to another place.
func (m *NaceModel) ndiClient() *http.Client {
	return &http.Client{
		Transport:     naceClientTransport{base: naceNDISchemeSafeTransport{base: naceNDITransport(naceNDIAPITransport)}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// naceNDIError is a refused NDI request. NDI answers
// {"error":{"code","message","retryable"},"request_id"}. Error() carries the
// code and a shortened message only; the request id goes to the log.
type naceNDIError struct {
	Status    int
	Code      string
	Message   string
	Retryable bool
	RequestID string
	// RetryAfter is the wait a 429, 503 or 529 asked for; 0 when it gave
	// none.
	RetryAfter time.Duration
	// Err is the failure that kept the error body from being read.
	Err error
}

func (e *naceNDIError) Unwrap() error { return e.Err }

func (e *naceNDIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "status %d", e.Status)
	if e.Code != "" {
		b.WriteString(": " + e.Code)
	}
	if e.Message != "" {
		b.WriteString(": " + e.Message)
	}
	return b.String()
}

type naceNDIErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	RequestID string `json:"request_id"`
}

type naceNDIJob struct {
	JobID  string              `json:"job_id"`
	Status string              `json:"status"`
	Error  *naceNDIErrorDetail `json:"error"`
	Result json.RawMessage     `json:"result"`
}

type naceNDIFile struct {
	FileID      string `json:"file_id"`
	WorkspaceID string `json:"workspace_id"`
	// Path is nil when the receipt names no path.
	Path *string `json:"path"`
}

type naceNDIParseResult struct {
	Document struct {
		// PageCount is the number of pages parsed: the whole document, or
		// the pages page_ranges selected.
		PageCount int            `json:"page_count"`
		Markdown  string         `json:"markdown"`
		Blocks    []naceNDIBlock `json:"blocks"`
	} `json:"document"`
	Disclosures []struct {
		Code   string `json:"code"`
		Target string `json:"target"`
	} `json:"disclosures"`
}

type naceNDIBlock struct {
	Type         string `json:"type"`
	Content      string `json:"content"`
	MarkdownSpan *struct {
		Page      int `json:"page"`
		CharStart int `json:"char_start"`
		CharEnd   int `json:"char_end"`
	} `json:"markdown_span"`
	Location *struct {
		Kind string `json:"kind"`
		// Page is the page number in the whole document, also when
		// page_ranges selected some pages.
		Page int `json:"page"`
		// Polygons are lists of [x, y] points, each a fraction of the page.
		Polygons [][][]float64 `json:"polygons"`
	} `json:"location"`
}

// naceNDISelection is the page selection of one parse.
type naceNDISelection struct {
	// Send is sent as page_ranges; nil parses the whole document.
	Send [][]int
	// Keep is the pages kept from the result; nil keeps every page.
	Keep [][]int
	// Whole says why the whole document is parsed for a selection that Send
	// does not carry; the warning about dropped pages names it.
	Whole string
}

// VerifyOCRConnection checks the key against the model catalog, so verifying
// an NDI model does not run (and bill) a parse. It uses the NDI client, as
// the parse does: no payload log, no redirect, provider text cleaned.
func (m *NaceModel) VerifyOCRConnection(ctx context.Context, apiConfig *APIConfig) error {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}
	modelsURL, err := m.baseModel.EndpointURL(apiConfig, m.baseModel.URLSuffix.Models)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()
	var list naceModelList
	err = m.ndiCall(ctx, m.ndiClient(), naceNDIMaxAttempts, func(ctx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
		if err != nil {
			return nil, err
		}
		m.baseModel.applyAuth(req, apiConfig)
		return req, nil
	}, &list)
	if err != nil {
		return fmt.Errorf("nace: %w", err)
	}
	if list.Models == nil {
		return fmt.Errorf("nace: invalid model list format")
	}
	return nil
}

// OCRFile parses a PDF with NDI at the effort the model name selects
// (ndi-parse-low, ndi-parse-medium or ndi-parse-high). content is uploaded to
// the key's Nace workspace and deleted after the parse; with no content, a
// public https PDF link is parsed in place. ocrConfig.Pages selects the pages
// to parse.
func (m *NaceModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	if BearerAuth(apiConfig) == "" {
		return nil, fmt.Errorf("nace: api key is required")
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("nace: model name is required")
	}
	mode, err := naceNDIParseMode(*modelName)
	if err != nil {
		return nil, err
	}
	var source map[string]any
	if len(content) == 0 {
		remoteURL := ""
		if url != nil {
			remoteURL = strings.TrimSpace(*url)
		}
		if content != nil && remoteURL == "" {
			return nil, fmt.Errorf("nace: the file is empty (0 bytes)")
		}
		if source, err = naceNDIURLSource(remoteURL); err != nil {
			return nil, err
		}
	} else if !naceNDIIsPDF(content) {
		return nil, fmt.Errorf("nace: NDI OCR models read PDF files only")
	} else if len(content) > naceNDIMaxUploadBytes {
		return nil, fmt.Errorf("nace: file of %d bytes exceeds the %d-byte upload limit", len(content), naceNDIMaxUploadBytes)
	}
	selection := naceNDIPageSelection(ocrConfig)
	suffix := m.baseModel.URLSuffix.OCR
	if strings.TrimSpace(suffix) == "" {
		suffix = naceNDIParseSuffix
	}
	parseURL, err := m.baseModel.EndpointURL(apiConfig, suffix)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, longOpCallTimeout)
	defer cancel()

	if source == nil {
		file, err := m.ndiUpload(ctx, apiConfig, content)
		if err != nil {
			return nil, fmt.Errorf("nace: upload: %w", err)
		}
		defer m.ndiDeleteFile(ctx, apiConfig, file)
		source = map[string]any{"type": "workspace_file", "workspace_id": file.WorkspaceID, "file_id": file.FileID}
	}

	job, err := m.ndiParse(ctx, apiConfig, parseURL, source, mode, selection.Send)
	// NDI fails a job whose page_ranges run far past the last page with
	// ocr_failed "No renderable pages", at no charge. Only then is the whole
	// document parsed (and billed) once and filtered to the selection; any
	// other failure is returned.
	var ended *naceNDIJobEnded
	if selection.Send != nil && errors.As(err, &ended) && ended.code == naceNDIOCRFailedCode && strings.HasPrefix(ended.message, naceNDINoPagesMessage) {
		common.Warn("Nace NDI: the selected pages failed to parse; parsing the whole document", zap.Error(err))
		selection.Send, selection.Whole = nil, "NDI could not parse the selected pages (ocr_failed)"
		job, err = m.ndiParse(ctx, apiConfig, parseURL, source, mode, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("nace: parse: %w", err)
	}
	if len(job.Result) == 0 || string(job.Result) == "null" {
		return nil, fmt.Errorf("nace: parse job %s returned no result", job.JobID)
	}
	var result naceNDIParseResult
	if err := json.Unmarshal(job.Result, &result); err != nil {
		return nil, fmt.Errorf("nace: parse job %s: decode result: %w", job.JobID, err)
	}
	response := naceNDIResponse(&result, selection)
	common.Info("Nace NDI parse finished", zap.String("job_id", job.JobID), zap.Int("page_count", len(response.Pages)), zap.Int("text_len", len(*response.Text)))
	return response, nil
}

// naceNDIParseMode maps an NDI model name to its parse_mode.
func naceNDIParseMode(modelName string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(modelName))
	if mode, ok := strings.CutPrefix(name, naceNDIModelPrefix); ok {
		switch mode {
		case "low", "medium", "high":
			return mode, nil
		}
	}
	return "", fmt.Errorf("nace: %q is not an NDI OCR model; use ndi-parse-low, ndi-parse-medium or ndi-parse-high", modelName)
}

// naceNDIIsPDF reports whether content carries a PDF header within the first
// kilobyte.
func naceNDIIsPDF(content []byte) bool {
	return bytes.Contains(content[:min(len(content), naceNDIPDFHeaderWindow)], []byte("%PDF-"))
}

// naceNDIURLSource references a public https PDF. NDI picks the parser from
// file_name, so the link must name a .pdf file.
func naceNDIURLSource(remoteURL string) (map[string]any, error) {
	if remoteURL == "" {
		return nil, fmt.Errorf("nace: file content or url is required")
	}
	parsed, err := neturl.Parse(remoteURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" {
		return nil, fmt.Errorf("nace: NDI reads links over https only")
	}
	name := path.Base(parsed.Path)
	if !strings.EqualFold(path.Ext(name), ".pdf") {
		return nil, fmt.Errorf("nace: NDI OCR models read PDF files only; the link must name a .pdf file")
	}
	return map[string]any{"type": "url", "url": remoteURL, "file_name": name}, nil
}

// naceNDIPageSelection turns the parser's 1-based, inclusive page ranges into
// NDI page_ranges. A selection larger than one parse accepts is not sent: the
// whole document is parsed and the result is filtered to the selection.
func naceNDIPageSelection(ocrConfig *OCRConfig) naceNDISelection {
	if ocrConfig == nil {
		return naceNDISelection{}
	}
	var ranges [][]int
	for _, r := range ocrConfig.Pages {
		if len(r) == 2 && r[0] >= 1 && r[1] >= r[0] {
			ranges = append(ranges, []int{r[0], r[1]})
		}
	}
	if len(ranges) == 0 {
		return naceNDISelection{}
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i][0] < ranges[j][0] })
	merged := ranges[:1]
	for _, r := range ranges[1:] {
		last := merged[len(merged)-1]
		if r[0] <= last[1] || r[0]-1 == last[1] {
			last[1] = max(last[1], r[1])
			continue
		}
		merged = append(merged, r)
	}
	total := 0
	for _, r := range merged {
		total += min(r[1]-r[0]+1, naceNDIMaxRangePages+1)
		if total > naceNDIMaxRangePages {
			break
		}
	}
	if len(merged) <= naceNDIMaxRanges && total <= naceNDIMaxRangePages {
		return naceNDISelection{Send: merged, Keep: merged}
	}
	return naceNDISelection{Keep: merged, Whole: fmt.Sprintf("the page selection is larger than one parse accepts (%d ranges, %d pages)", naceNDIMaxRanges, naceNDIMaxRangePages)}
}

func naceNDIPageSelected(ranges [][]int, page int) bool {
	if len(ranges) == 0 {
		return true
	}
	for _, r := range ranges {
		if page >= r[0] && page <= r[1] {
			return true
		}
	}
	return false
}

// ndiUpload stores content in the key's workspace and returns the stored
// file. Every attempt asks for a new grant, because a grant works once, and
// sends the file to the same path: when an earlier attempt stored the file but
// its answer was lost, NDI answers with the file already there. When no
// attempt returns the file although one may have stored it, the file is sent
// once more after ctx has ended, to learn its id, and deleted.
func (m *NaceModel) ndiUpload(ctx context.Context, apiConfig *APIConfig, content []byte) (*naceNDIFile, error) {
	id, err := naceNDIRandomID()
	if err != nil {
		return nil, err
	}
	filePath := naceNDIUploadPrefix + id + ".pdf"
	mayHaveStored := false
	for attempt := 1; ; attempt++ {
		file, unknown, err := m.ndiUploadOnce(ctx, apiConfig, filePath, content)
		if err == nil {
			return file, nil
		}
		mayHaveStored = mayHaveStored || unknown
		if attempt >= naceNDIMaxAttempts || ctx.Err() != nil || !naceNDITransient(err) {
			if mayHaveStored {
				m.ndiRemoveUpload(ctx, apiConfig, filePath, content)
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}
		common.Warn("Nace NDI: upload failed, retrying", zap.Int("attempt", attempt), zap.Error(err))
		select {
		case <-time.After(naceNDIRetryWait(ctx, err, attempt)):
		case <-ctx.Done():
			if mayHaveStored {
				m.ndiRemoveUpload(ctx, apiConfig, filePath, content)
			}
			return nil, ctx.Err()
		}
	}
}

// ndiUploadOnce asks for a grant and sends the file once. unknown reports
// that the file may be stored although no file came back.
func (m *NaceModel) ndiUploadOnce(ctx context.Context, apiConfig *APIConfig, filePath string, content []byte) (file *naceNDIFile, unknown bool, err error) {
	grantURL, err := m.baseModel.EndpointURL(apiConfig, naceNDIGrantsPath)
	if err != nil {
		return nil, false, err
	}
	size := len(content)
	var grant struct {
		WorkspaceID string `json:"workspace_id"`
		UploadURL   string `json:"upload_url"`
		Token       string `json:"token"`
	}
	err = m.ndiCall(ctx, m.ndiClient(), naceNDIMaxAttempts, func(ctx context.Context) (*http.Request, error) {
		return m.baseModel.newJSONPostRequest(ctx, grantURL, apiConfig, map[string]any{
			"path": filePath, "ttl_seconds": naceNDIUploadTTL, "total_size_bytes": size,
		})
	}, &grant)
	if err != nil {
		// NDI refuses a standalone upload grant on an account with zero
		// retention (422 invalid_request).
		var apiErr *naceNDIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusUnprocessableEntity && apiErr.Code == "invalid_request" {
			return nil, false, fmt.Errorf("upload grant refused; an account with zero retention cannot upload files for parsing (turn zero retention off, or parse a public PDF by URL): %w", err)
		}
		return nil, false, fmt.Errorf("upload grant: %w", err)
	}
	if grant.Token == "" || grant.WorkspaceID == "" {
		return nil, false, fmt.Errorf("upload grant carries no token or workspace")
	}
	if err := naceNDICheckUploadURL(ctx, grant.UploadURL, grantURL); err != nil {
		return nil, false, err
	}
	metadata, err := json.Marshal(map[string]any{"path": filePath, "total_size_bytes": size})
	if err != nil {
		return nil, false, err
	}
	// The multipart framing is built around the file, so the body reads the
	// file in place instead of a copy.
	var frame bytes.Buffer
	writer := multipart.NewWriter(&frame)
	if err := writer.WriteField("metadata", string(metadata)); err != nil {
		return nil, false, err
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="document.pdf"`)
	header.Set("Content-Type", "application/pdf")
	if _, err := writer.CreatePart(header); err != nil {
		return nil, false, err
	}
	headLen := frame.Len()
	if err := writer.Close(); err != nil {
		return nil, false, err
	}
	head, tail := frame.Bytes()[:headLen], frame.Bytes()[headLen:]
	body := func() io.ReadCloser {
		return io.NopCloser(io.MultiReader(bytes.NewReader(head), bytes.NewReader(content), bytes.NewReader(tail)))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, grant.UploadURL, body())
	if err != nil {
		return nil, false, naceNDIRedactError(err)
	}
	req.ContentLength = int64(len(head) + len(content) + len(tail))
	req.GetBody = func() (io.ReadCloser, error) { return body(), nil }
	req.Header.Set("Content-Type", writer.FormDataContentType())
	// The grant's token authorizes this one upload; the API key is not sent.
	req.Header.Set("X-Upload-Token", grant.Token)
	// A redirect must not carry the token on.
	client := &http.Client{
		Transport:     naceNDITransport(naceNDIUploadTransport),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	// An upload whose context ended before it started sent nothing, so it
	// stored nothing.
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	var job naceNDIJob
	if err := naceNDIDo(client, req, &job); err != nil {
		var apiErr *naceNDIError
		return nil, !errors.As(err, &apiErr) || naceNDITransient(err), err
	}
	finished, err := m.ndiWait(ctx, apiConfig, &job)
	if err != nil {
		var ended *naceNDIJobEnded
		return nil, !errors.As(err, &ended), err
	}
	var result struct {
		File naceNDIFile `json:"file"`
	}
	if err := json.Unmarshal(finished.Result, &result); err != nil {
		return nil, true, fmt.Errorf("decode upload result: %w", err)
	}
	if result.File.FileID == "" || result.File.WorkspaceID == "" {
		return nil, true, fmt.Errorf("upload result carries no file")
	}
	// The ids are used for an authenticated parse and delete, so a receipt for
	// another workspace or path (an empty one included) is refused, and its
	// ids are not used.
	if result.File.WorkspaceID != grant.WorkspaceID || (result.File.Path != nil && *result.File.Path != filePath) {
		return nil, false, fmt.Errorf("upload result names a file other than the one uploaded")
	}
	return &result.File, false, nil
}

// naceNDICheckUploadURL accepts the upload URL a grant names when it is the
// API's own origin (scheme, host and port), or when it is https and resolves
// to public addresses only (unless ALLOW_ANY_HOST is set), as the repo's
// strict SSRF guard requires of cloud providers. The API origin is not
// checked, because the API base URL is the operator's choice; the upload host
// is the server's. As with the repo's strict transport, the connection
// resolves the host again.
func naceNDICheckUploadURL(ctx context.Context, uploadURL, apiURL string) error {
	parsed, err := neturl.Parse(strings.TrimSpace(uploadURL))
	if err != nil || parsed.Hostname() == "" {
		return fmt.Errorf("upload grant carries no valid upload_url")
	}
	if SameOrigin(uploadURL, apiURL) {
		return nil
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return fmt.Errorf("refusing upload_url over %s", parsed.Scheme)
	}
	if _, _, err := common.AssertURLSafeWithContext(ctx, uploadURL); err != nil {
		return fmt.Errorf("refusing upload_url host %s: %w", parsed.Hostname(), err)
	}
	return nil
}

// ndiRemoveUpload deletes a file that an upload may have stored without
// returning it. Sending the file to its path again returns the stored file;
// if none was stored, the new one is deleted at once.
func (m *NaceModel) ndiRemoveUpload(ctx context.Context, apiConfig *APIConfig, filePath string, content []byte) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), naceNDICleanupTimeout)
	defer cancel()
	file, _, err := m.ndiUploadOnce(cleanupCtx, apiConfig, filePath, content)
	if err != nil {
		common.Warn("Nace NDI: an upload may be left in the workspace", zap.String("path", filePath), zap.Error(err))
		return
	}
	m.ndiDeleteFile(ctx, apiConfig, file)
}

// ndiParse creates the parse job and reads it until it ends. The job is
// created without waiting, so its id is known at once: whatever happens next,
// the job is deleted on return, which cancels it while it runs (NDI does not
// bill a cancelled job) and erases its result once that is in memory.
func (m *NaceModel) ndiParse(ctx context.Context, apiConfig *APIConfig, parseURL string, source map[string]any, mode string, pageRanges [][]int) (*naceNDIJob, error) {
	body := map[string]any{
		"source":     source,
		"parse_mode": mode,
		"output": map[string]any{
			"formats":              []string{"markdown", "blocks"},
			"table_format":         "markdown",
			"include_page_markers": false,
		},
		"figures": map[string]any{"mode": "include"},
	}
	if len(pageRanges) > 0 {
		ranges := make([]map[string]int, 0, len(pageRanges))
		for _, r := range pageRanges {
			ranges = append(ranges, map[string]int{"start": r[0], "end": r[1]})
		}
		body["page_ranges"] = ranges
	}
	job, err := m.ndiCreate(ctx, apiConfig, parseURL, body, 0)
	zeroRetention := false
	var apiErr *naceNDIError
	if errors.As(err, &apiErr) && apiErr.Code == naceNDIZeroRetentionCode {
		zeroRetention = true
		job, err = m.ndiCreate(ctx, apiConfig, parseURL, body, naceNDIZeroRetentionWaitSeconds)
	}
	if err != nil {
		return nil, err
	}
	jobURL, err := m.ndiJobURL(apiConfig, job.JobID)
	if err != nil {
		return nil, err
	}
	defer m.ndiDeleteJob(ctx, apiConfig, jobURL, job.JobID)
	common.Info("Nace NDI parse job created", zap.String("job_id", job.JobID), zap.String("status", job.Status))
	if zeroRetention && (job.Status == "queued" || job.Status == "running") {
		return nil, fmt.Errorf("job %s outlasted the %d s wait; the account has zero retention, so NDI keeps no result to read later and the job was cancelled", job.JobID, naceNDIZeroRetentionWaitSeconds)
	}
	return m.ndiWait(ctx, apiConfig, job)
}

// ndiCreate sends the create call, holding it up to waitSeconds. The
// idempotency key makes a retried create return the same job instead of
// starting (and billing) a second one. A 2xx answer that is cut short or does
// not decode may still name the job; that job is deleted.
func (m *NaceModel) ndiCreate(ctx context.Context, apiConfig *APIConfig, parseURL string, body map[string]any, waitSeconds int) (*naceNDIJob, error) {
	key, err := naceNDIRandomID()
	if err != nil {
		return nil, err
	}
	createURL := parseURL + "?wait_seconds=" + strconv.Itoa(waitSeconds)
	var job naceNDIJob
	err = m.ndiCall(ctx, m.ndiClient(), naceNDIMaxAttempts, func(ctx context.Context) (*http.Request, error) {
		req, err := m.baseModel.newJSONPostRequest(ctx, createURL, apiConfig, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Idempotency-Key", "ragflow-"+key)
		return req, nil
	}, &job)
	if err != nil {
		jobID := job.JobID
		var bad *naceNDIBadAnswer
		if jobID == "" && errors.As(err, &bad) {
			jobID = bad.jobID
		}
		if jobURL, urlErr := m.ndiJobURL(apiConfig, jobID); jobID != "" && urlErr == nil {
			m.ndiDeleteJob(ctx, apiConfig, jobURL, jobID)
		}
		return nil, err
	}
	if job.JobID == "" {
		return nil, fmt.Errorf("response carries no job id")
	}
	return &job, nil
}

func (m *NaceModel) ndiJobURL(apiConfig *APIConfig, jobID string) (string, error) {
	if jobID == "" {
		return "", fmt.Errorf("response carries no job id")
	}
	return m.baseModel.EndpointURL(apiConfig, naceNDIJobsPath+"/"+neturl.PathEscape(jobID))
}

// ndiWait reads job until it ends. Each read is held on the server until the
// job ends or the wait runs out.
func (m *NaceModel) ndiWait(ctx context.Context, apiConfig *APIConfig, job *naceNDIJob) (*naceNDIJob, error) {
	pollURL := ""
	interval := naceNDIPollInterval
	for polled := false; ; polled = true {
		switch job.Status {
		case "succeeded":
			return job, nil
		case "failed":
			return nil, naceNDIJobFailure(job)
		case "cancelled":
			return nil, &naceNDIJobEnded{text: fmt.Sprintf("job %s was cancelled", job.JobID)}
		case "queued", "running":
		default:
			return nil, fmt.Errorf("job %s returned unknown status %q", job.JobID, naceNDIClip(job.Status))
		}
		if pollURL == "" {
			jobURL, err := m.ndiJobURL(apiConfig, job.JobID)
			if err != nil {
				return nil, err
			}
			pollURL = jobURL + "?wait_seconds=" + strconv.Itoa(naceNDIPollWaitSeconds)
		}
		if polled {
			select {
			case <-time.After(interval):
			case <-ctx.Done():
			}
			interval = min(interval*2, naceNDIMaxPollInterval)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var next naceNDIJob
		err := m.ndiCall(ctx, m.ndiClient(), naceNDIMaxAttempts, func(ctx context.Context) (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
			if err != nil {
				return nil, err
			}
			m.baseModel.applyAuth(req, apiConfig)
			return req, nil
		}, &next)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("poll job %s: %w", job.JobID, err)
		}
		if next.JobID == "" {
			next.JobID = job.JobID
		}
		job = &next
	}
}

// naceNDIJobEnded is a job that ended without a result: it failed or was
// cancelled.
type naceNDIJobEnded struct {
	text string
	// code and message are the error of a failed job.
	code, message string
}

func (e *naceNDIJobEnded) Error() string { return e.text }

func naceNDIJobFailure(job *naceNDIJob) error {
	if job.Error == nil {
		return &naceNDIJobEnded{text: fmt.Sprintf("job %s failed with no error detail", job.JobID)}
	}
	if job.Error.RequestID != "" {
		common.Warn("Nace NDI: job failed", zap.String("job_id", naceNDIClip(job.JobID)), zap.String("code", naceNDIClip(job.Error.Code)), zap.String("request_id", naceNDIClip(job.Error.RequestID)))
	}
	detail := naceNDIClip(job.Error.Code)
	if message := naceNDIClip(job.Error.Message); message != "" {
		if detail != "" {
			detail += ": "
		}
		detail += message
	}
	return &naceNDIJobEnded{text: fmt.Sprintf("job %s failed: %s", job.JobID, detail), code: job.Error.Code, message: job.Error.Message}
}

// ndiDeleteJob deletes the parse job: it cancels a running job and erases a
// finished job's result.
func (m *NaceModel) ndiDeleteJob(ctx context.Context, apiConfig *APIConfig, jobURL, jobID string) {
	if err := m.ndiDelete(ctx, apiConfig, jobURL, false); err != nil {
		common.Warn("Nace NDI: failed to delete parse job", zap.String("job_id", jobID), zap.Error(err))
	}
}

// ndiDeleteFile removes the uploaded file from the workspace, whether or not
// the parse succeeded.
func (m *NaceModel) ndiDeleteFile(ctx context.Context, apiConfig *APIConfig, file *naceNDIFile) {
	fileURL, err := m.baseModel.EndpointURL(apiConfig, naceNDIFilesPath+"/"+neturl.PathEscape(file.WorkspaceID)+"/files/"+neturl.PathEscape(file.FileID))
	if err == nil {
		err = m.ndiDelete(ctx, apiConfig, fileURL, true)
	}
	if err != nil {
		common.Warn("Nace NDI: failed to delete uploaded file", zap.String("file_id", file.FileID), zap.Error(err))
	}
}

// ndiDelete runs after ctx may have ended, so it uses a detached context
// bounded by naceNDICleanupTimeout; the DELETE is sent at most twice in it.
// With confirm, a deletion answered 202 (still deleting) is checked again,
// one send per check, after Retry-After (or naceNDIDeletePollDelay) until NDI
// confirms it (200, 204, or 404 after the 202). A check that fails
// transiently does not end the checks. Every check and wait ends
// naceNDIDeleteConfirmTimeout after the first DELETE was sent: when the
// asked wait reaches that point, it waits half the time left and
// sends one last check. So one ndiDelete ends within naceNDICleanupTimeout.
func (m *NaceModel) ndiDelete(ctx context.Context, apiConfig *APIConfig, target string, confirm bool) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), naceNDICleanupTimeout)
	defer cancel()
	checkCtx, cancelChecks := context.WithTimeout(cleanupCtx, naceNDIDeleteConfirmTimeout)
	defer cancelChecks()
	send := func(ctx context.Context, attempts int) (reply naceNDIDeleteReply, err error) {
		err = m.ndiCall(ctx, m.ndiClient(), attempts, func(ctx context.Context) (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodDelete, target, nil)
			if err != nil {
				return nil, err
			}
			m.baseModel.applyAuth(req, apiConfig)
			return req, nil
		}, &reply)
		return reply, err
	}
	reply, err := send(cleanupCtx, naceNDIDeleteAttempts)
	if err != nil || !confirm || reply.status != http.StatusAccepted {
		return err
	}
	deadline, _ := checkCtx.Deadline()
	for last := false; !last; {
		wait := reply.retryAfter
		var apiErr *naceNDIError
		if errors.As(err, &apiErr) {
			wait = apiErr.RetryAfter
		}
		if wait <= 0 {
			wait = naceNDIDeletePollDelay
		}
		left := time.Until(deadline)
		if left <= 0 {
			break
		}
		if wait >= left {
			wait, last = left/2, true
		}
		time.Sleep(wait)
		reply, err = send(checkCtx, 1)
		switch {
		case err == nil && reply.status != http.StatusAccepted:
			return nil
		case errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound:
			return nil
		case err != nil && !naceNDITransient(err) && checkCtx.Err() == nil:
			return err
		}
	}
	return naceNDIUnconfirmed(err)
}

// naceNDIUnconfirmed reports a file deletion that NDI accepted (202) but did
// not confirm in time; err is the failure of the last check, if any.
func naceNDIUnconfirmed(err error) error {
	if err != nil {
		return fmt.Errorf("deletion not confirmed: %w", err)
	}
	return fmt.Errorf("deletion not confirmed: still in progress after %v", naceNDIDeleteConfirmTimeout)
}

// naceNDIDeleteReply keeps the status and Retry-After of a DELETE answer.
type naceNDIDeleteReply struct {
	status     int
	retryAfter time.Duration
}

// ndiCall sends the request build makes and decodes a 2xx JSON reply into out.
// A transport failure, 408, 429, a 5xx or an error NDI marks retryable is
// sent again, up to attempts sends in all.
func (m *NaceModel) ndiCall(ctx context.Context, client *http.Client, attempts int, build func(context.Context) (*http.Request, error), out any) error {
	for attempt := 1; ; attempt++ {
		req, err := build(ctx)
		if err != nil {
			return naceNDIRedactError(err)
		}
		err = naceNDIDo(client, req, out)
		if err == nil || attempt >= attempts || ctx.Err() != nil || !naceNDITransient(err) {
			return err
		}
		common.Warn("Nace NDI: request failed, retrying", zap.String("method", req.Method), zap.String("path", req.URL.Path), zap.Error(err))
		select {
		case <-time.After(naceNDIRetryWait(ctx, err, attempt)):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// naceNDIRetryWait is the pause before sending a failed request again: the
// Retry-After of a 429, 503 or 529 when it is at most naceNDIMaxRetryAfter and
// ends before ctx does, else a backoff of attempt times naceNDIRetryDelay.
func naceNDIRetryWait(ctx context.Context, err error, attempt int) time.Duration {
	var apiErr *naceNDIError
	if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 && apiErr.RetryAfter <= naceNDIMaxRetryAfter {
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > apiErr.RetryAfter {
			return apiErr.RetryAfter
		}
	}
	return time.Duration(attempt) * naceNDIRetryDelay
}

// naceNDIRetryAfter reads a Retry-After value in seconds or as an HTTP date;
// 0 when it is missing, invalid or not in the future.
func naceNDIRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(min(max(seconds, 0), 1<<20)) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 0
}

func naceNDIDo(client *http.Client, req *http.Request, out any) error {
	resp, err := client.Do(req)
	if err != nil {
		return naceNDIRedactError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, readErr := readModelErrorBody(resp.Body)
		if readErr != nil {
			return &naceNDIError{Status: resp.StatusCode, Message: "failed to read error response", Err: readErr}
		}
		apiErr := naceNDIErrorFrom(resp.StatusCode, body)
		switch resp.StatusCode {
		case http.StatusTooManyRequests, http.StatusServiceUnavailable, naceNDIStatusOverloaded:
			apiErr.RetryAfter = naceNDIRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		}
		if apiErr.RequestID != "" {
			common.Warn("Nace NDI: request refused", zap.String("method", req.Method), zap.String("path", req.URL.Path), zap.Int("status", apiErr.Status), zap.String("code", apiErr.Code), zap.String("request_id", apiErr.RequestID))
		}
		return apiErr
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, naceNDIMaxResponseBytes+1))
	if err == nil && int64(len(body)) > naceNDIMaxResponseBytes {
		err = fmt.Errorf("response body exceeds %d bytes", naceNDIMaxResponseBytes)
	}
	if err != nil {
		return &naceNDIBadAnswer{err: fmt.Errorf("failed to read response: %w", err), jobID: naceNDIPartialJobID(body)}
	}
	if reply, ok := out.(*naceNDIDeleteReply); ok {
		reply.status, reply.retryAfter = resp.StatusCode, naceNDIRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		return nil
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		// Malformed JSON fills nothing, so the job id is read leniently, as
		// from a cut-short answer.
		return &naceNDIBadAnswer{err: fmt.Errorf("failed to parse response: %w", err), jobID: naceNDIPartialJobID(body)}
	}
	return nil
}

// naceNDIBadAnswer is a 2xx answer that was cut short or did not decode: the
// call may have taken effect although its result is unknown. jobID is the job
// id read from the part that arrived or from the malformed answer, if any.
type naceNDIBadAnswer struct {
	err   error
	jobID string
}

// naceNDIJobIDPattern finds the job id near the start of a cut-short answer.
var naceNDIJobIDPattern = regexp.MustCompile(`"job_id"\s*:\s*"([^"\\]{1,200})"`)

func naceNDIPartialJobID(body []byte) string {
	if match := naceNDIJobIDPattern.FindSubmatch(body[:min(len(body), 4096)]); match != nil {
		return string(match[1])
	}
	return ""
}

func (e *naceNDIBadAnswer) Error() string { return e.err.Error() }
func (e *naceNDIBadAnswer) Unwrap() error { return e.err }

// naceNDIErrorFrom keeps the provider's error code and a shortened message.
func naceNDIErrorFrom(status int, body []byte) *naceNDIError {
	var envelope struct {
		Error     *naceNDIErrorDetail `json:"error"`
		RequestID string              `json:"request_id"`
	}
	if json.Unmarshal(body, &envelope) == nil && envelope.Error != nil {
		requestID := envelope.RequestID
		if requestID == "" {
			requestID = envelope.Error.RequestID
		}
		return &naceNDIError{Status: status, Code: naceNDIClip(envelope.Error.Code), Message: naceNDIClip(envelope.Error.Message), Retryable: envelope.Error.Retryable, RequestID: naceNDIClip(requestID)}
	}
	// A body that is not an NDI error (a proxy page, for example) is not
	// quoted: the status says enough.
	return &naceNDIError{Status: status}
}

// naceNDIURLPattern matches a URL inside provider text, naceNDIQueryPattern
// the query of a relative one, and naceNDISecretPatterns credentials: bearer
// tokens, Nace keys and token, signature or key values.
var (
	naceNDIURLPattern     = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>]+`)
	naceNDIQueryPattern   = regexp.MustCompile(`\?[^\s"'<>]+`)
	naceNDISecretPatterns = []struct {
		pattern *regexp.Regexp
		replace string
	}{
		{regexp.MustCompile(`(?i)\bbearer\s+[^\s"',;]+`), "Bearer [redacted]"},
		{regexp.MustCompile(`(?i)\bnace_sk_[^\s"',;&]*`), "[redacted]"},
		{regexp.MustCompile(`(?i)(^|[^a-z0-9_-])((?:x-)?(?:(?:upload|access|refresh|id|auth|session)[-_]?)?token|(?:x-)?(?:api[-_]?)?key|(?:client[-_]?)?secret|password|(?:x-amz-)?signature|sig)(["']?\s*[:=]\s*["']?)[^\s"'&,;]+`), "${1}${2}${3}[redacted]"},
	}
)

// naceNDIClip collapses white space, drops the query, fragment and user info
// of every URL and the query of every relative URL in provider text (a URL
// may be signed), masks credentials, and shortens the text to
// naceNDIMaxErrorText characters.
func naceNDIClip(text string) string {
	text = naceNDIURLPattern.ReplaceAllStringFunc(text, naceNDIRedactURL)
	text = naceNDIQueryPattern.ReplaceAllString(text, "")
	for _, secret := range naceNDISecretPatterns {
		text = secret.pattern.ReplaceAllString(text, secret.replace)
	}
	runes := []rune(strings.Join(strings.Fields(text), " "))
	if len(runes) > naceNDIMaxErrorText {
		return string(runes[:naceNDIMaxErrorText]) + "..."
	}
	return string(runes)
}

// naceNDIRedactError drops the query, fragment and user info from the URL a
// transport error quotes (an upload URL may be signed), and clips the text of
// the failure it wraps, which may quote a URL too. The failure stays
// reachable, so errors.Is still finds context.Canceled.
func naceNDIRedactError(err error) error {
	var urlErr *neturl.Error
	if !errors.As(err, &urlErr) {
		return err
	}
	redacted := *urlErr
	redacted.URL = naceNDIRedactURL(urlErr.URL)
	if urlErr.Err != nil {
		redacted.Err = &naceNDIClippedError{text: naceNDIClip(urlErr.Err.Error()), err: urlErr.Err}
	}
	return &redacted
}

// naceNDIClippedError shows clipped text for an error it still wraps.
type naceNDIClippedError struct {
	text string
	err  error
}

func (e *naceNDIClippedError) Error() string { return e.text }
func (e *naceNDIClippedError) Unwrap() error { return e.err }

// Timeout keeps url.Error.Timeout working through the wrapper.
func (e *naceNDIClippedError) Timeout() bool {
	var timeout interface{ Timeout() bool }
	return errors.As(e.err, &timeout) && timeout.Timeout()
}

func naceNDIRedactURL(raw string) string {
	parsed, err := neturl.Parse(raw)
	if err != nil {
		return "(invalid url)"
	}
	parsed.User, parsed.RawQuery, parsed.ForceQuery, parsed.Fragment, parsed.RawFragment = nil, "", false, "", ""
	return parsed.String()
}

// naceNDITransient reports whether a failed request may succeed when sent
// again. A reply that could not be decoded is not retried.
func naceNDITransient(err error) bool {
	var apiErr *naceNDIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable || apiErr.Status == http.StatusRequestTimeout ||
			apiErr.Status == http.StatusTooManyRequests || apiErr.Status >= http.StatusInternalServerError
	}
	var transportErr *neturl.Error
	return errors.As(err, &transportErr)
}

func naceNDIRandomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// naceNDIBlockLabel maps an NDI block type to the OCR block label the PDF
// projection reads. Page furniture (running headers and footers, page numbers,
// margin text and page footnotes) is dropped, as for the other OCR providers.
// An unknown type is kept as text.
func naceNDIBlockLabel(blockType string) (label string, keep, known bool) {
	switch strings.ToLower(strings.TrimSpace(blockType)) {
	case "heading", "title":
		return "paragraph_title", true, true
	case "text", "paragraph", "list", "ref_text", "table_footnote":
		return "text", true, true
	case "table":
		return "table", true, true
	case "figure", "image", "chart":
		return "figure", true, true
	case "image_caption", "figure_caption":
		return "figure_title", true, true
	case "table_caption":
		return "table_title", true, true
	case "equation":
		return "formula", true, true
	case "formula_number":
		return "formula_number", true, true
	case "page_header", "header", "footer", "page_footer", "page_number", "aside_text", "page_footnote":
		return "", false, true
	}
	return "text", true, false
}

// naceNDIBlockPage is the block's page number in the whole document: from its
// location, else from its Markdown span; 0 when neither names one.
func naceNDIBlockPage(block naceNDIBlock) int {
	if block.Location != nil && block.Location.Page > 0 {
		return block.Location.Page
	}
	if block.MarkdownSpan != nil {
		return block.MarkdownSpan.Page
	}
	return 0
}

// naceNDIBox is the rectangle around every polygon point of a page region,
// as [left, top, right, bottom] fractions of the page.
func naceNDIBox(block naceNDIBlock) []float64 {
	if block.Location == nil || block.Location.Kind != "page_region" {
		return nil
	}
	left, top, right, bottom := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, polygon := range block.Location.Polygons {
		for _, point := range polygon {
			if len(point) != 2 || math.IsNaN(point[0]) || math.IsNaN(point[1]) || math.IsInf(point[0], 0) || math.IsInf(point[1], 0) {
				return nil
			}
			left, right = math.Min(left, point[0]), math.Max(right, point[0])
			top, bottom = math.Min(top, point[1]), math.Max(bottom, point[1])
		}
	}
	if right <= left || bottom <= top {
		return nil
	}
	return []float64{left, top, right, bottom}
}

// naceNDIResponse groups the blocks by page, in reading order, with
// Normalized boxes. Pages are listed from the first up to the last page parsed
// or holding a block, blank ones included, and no further than
// naceNDIMaxPages. Blocks outside selection.Keep are dropped. Text is the kept
// blocks' content in reading order, so dropped blocks and pages stay out of
// it. A result without blocks keeps its Markdown as one text block
// (naceNDIMarkdownOnly).
func naceNDIResponse(result *naceNDIParseResult, selection naceNDISelection) *OCRFileResponse {
	doc := result.Document
	response := &OCRFileResponse{}
	for _, disclosure := range result.Disclosures {
		warning := "Nace NDI: " + naceNDIClip(disclosure.Code)
		if disclosure.Target != "" {
			warning += " (" + naceNDIClip(disclosure.Target) + ")"
		}
		response.Warnings = append(response.Warnings, warning)
	}
	// A truncated result's Markdown is a preview, a prefix of the whole: a
	// span inside it still gives a block its text, and one past it gives none.
	markdown := []rune(doc.Markdown)
	pages := make([]OCRPage, min(max(doc.PageCount, 0), naceNDIMaxPages))
	overCap := doc.PageCount > naceNDIMaxPages
	outside := false
	var texts []string
	unknown := map[string]bool{}
	// NDI may label all text of a page aside_text (a page with /Rotate 90, for
	// example). Margin text is dropped only on a page that keeps other blocks;
	// on any other page it is kept as text, so the page does not come out
	// empty.
	// A kept block counts only when it has a body the PDF projection indexes
	// (OCRBlockBody): an empty text block does not.
	keptPages := map[int]bool{}
	for _, block := range doc.Blocks {
		if _, keep, _ := naceNDIBlockLabel(block.Type); keep && OCRBlockBody(naceNDIBlockText(block, markdown)) != "" {
			keptPages[naceNDIBlockPage(block)] = true
		}
	}
	for _, block := range doc.Blocks {
		label, keep, known := naceNDIBlockLabel(block.Type)
		page := naceNDIBlockPage(block)
		if !keep && strings.EqualFold(strings.TrimSpace(block.Type), "aside_text") && page >= 1 && !keptPages[page] {
			label, keep = "text", true
		}
		if !keep {
			continue
		}
		text := naceNDIBlockText(block, markdown)
		if page < 1 {
			response.Warnings = append(response.Warnings, fmt.Sprintf("Nace NDI: %s block without a page dropped", naceNDIClip(block.Type)))
			continue
		}
		if !naceNDIPageSelected(selection.Keep, page) {
			outside = true
			continue
		}
		if page > naceNDIMaxPages {
			overCap = true
			continue
		}
		if !known && !unknown[block.Type] {
			unknown[block.Type] = true
			response.Warnings = append(response.Warnings, fmt.Sprintf("Nace NDI: unknown block type %q kept as text", naceNDIClip(block.Type)))
		}
		for len(pages) < page {
			pages = append(pages, OCRPage{})
		}
		pages[page-1].Blocks = append(pages[page-1].Blocks, OCRBlock{Text: text, Label: label, BBox: naceNDIBox(block)})
		if strings.TrimSpace(text) != "" {
			texts = append(texts, text)
		}
	}
	if overCap {
		common.Warn("Nace NDI: result exceeds the page limit", zap.Int("page_count", doc.PageCount), zap.Int("max_pages", naceNDIMaxPages))
		response.Warnings = append(response.Warnings, fmt.Sprintf("Nace NDI: only the first %d pages were kept", naceNDIMaxPages))
	}
	if outside && selection.Send == nil {
		response.Warnings = append(response.Warnings, fmt.Sprintf("Nace NDI: %s, so the whole document was parsed and billed and only the selected pages were kept", selection.Whole))
	}
	for i := range pages {
		pages[i].Index = i
		pages[i].Normalized = true
	}
	response.Pages = pages
	text := strings.Join(texts, "\n\n")
	if len(doc.Blocks) == 0 {
		text = naceNDIMarkdownOnly(response, doc.Markdown, doc.PageCount, selection)
	}
	response.Text = &text
	return response
}

// naceNDIMarkdownOnly keeps the Markdown of a result without blocks as one
// text block, so ingestion, which reads page blocks, still gets the text. The
// result carries no page markers, so the block goes on the first page parsed.
// When the selection is applied here and does not cover every page, the
// Markdown cannot be filtered and is dropped with a warning.
func naceNDIMarkdownOnly(response *OCRFileResponse, markdown string, pageCount int, selection naceNDISelection) string {
	if strings.TrimSpace(markdown) == "" {
		return ""
	}
	page := 1
	switch {
	case selection.Send != nil:
		page = selection.Send[0][0]
	case selection.Keep != nil && !naceNDIPageRangeCovers(selection.Keep, 1, max(pageCount, 1)):
		response.Warnings = append(response.Warnings, "Nace NDI: the result has no layout blocks and its Markdown cannot be limited to the selected pages, so it was dropped")
		return ""
	}
	if page > naceNDIMaxPages {
		return ""
	}
	for len(response.Pages) < page {
		response.Pages = append(response.Pages, OCRPage{Index: len(response.Pages), Normalized: true})
	}
	response.Pages[page-1].Blocks = append(response.Pages[page-1].Blocks, OCRBlock{Text: markdown, Label: "text"})
	response.Warnings = append(response.Warnings, fmt.Sprintf("Nace NDI: the result has no layout blocks; its Markdown is kept as one text block on page %d", page))
	return markdown
}

// naceNDIPageRangeCovers reports whether one of the merged ranges holds every
// page from first to last.
func naceNDIPageRangeCovers(ranges [][]int, first, last int) bool {
	for _, r := range ranges {
		if r[0] <= first && r[1] >= last {
			return true
		}
	}
	return false
}

// naceNDIBlockText is the block's content or, when it has none, the Markdown
// its span covers (counted in characters).
func naceNDIBlockText(block naceNDIBlock, markdown []rune) string {
	if span := block.MarkdownSpan; block.Content == "" && span != nil && span.CharStart >= 0 && span.CharStart <= span.CharEnd && span.CharEnd <= len(markdown) {
		return string(markdown[span.CharStart:span.CharEnd])
	}
	return block.Content
}
