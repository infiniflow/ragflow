//go:build integration

package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type apiObject = map[string]any

type restResult struct {
	Status int
	JSON   map[string]any
	Body   []byte
	Header http.Header
}

var resourceSequence atomic.Uint64

func uniqueName(prefix string) string {
	return fmt.Sprintf("%s_%x_%d", prefix, time.Now().UnixNano(), resourceSequence.Add(1))
}

func restCall(t *testing.T, c *ServerTestConfig, method, path string, body any, query map[string]any) restResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// Cleanup calls must remain usable after the test context has been canceled.
	return restCallContext(t, ctx, c, method, path, body, query)
}

func restCallContext(t *testing.T, ctx context.Context, c *ServerTestConfig, method, path string, body any, query map[string]any) restResult {
	t.Helper()
	started := time.Now()
	headers := map[string]string{"Authorization": "Bearer " + c.APIKey}
	if c.APIKey == "" {
		headers["Authorization"] = " "
	}
	requestBody, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	requestURL, err := url.Parse(c.apiBase() + "/" + strings.TrimLeft(path, "/"))
	if err != nil {
		t.Fatal("invalid API URL")
	}
	params := requestURL.Query()
	for key, value := range query {
		params.Set(key, fmt.Sprint(value))
	}
	requestURL.RawQuery = params.Encode()
	resp, err := c.RequestJSONContext(ctx, method, path, body, query, headers)
	return readRESTResponse(t, c, method, requestURL.String(), diagnosticBody(requestBody, c.APIKey, c.Password), started, resp, err)
}

func readRESTResponse(t *testing.T, c *ServerTestConfig, method, requestURL, requestSummary string, started time.Time, resp *http.Response, err error) restResult {
	t.Helper()
	if err != nil {
		// url.Error includes the unsanitized URL; diagnostics already include its safe form.
		if urlErr, ok := errors.AsType[*url.Error](err); ok {
			err = urlErr.Err
		}
		t.Fatalf("%s %s payload=%s duration=%s transport error=%v", method, diagnosticURL(requestURL), requestSummary, time.Since(started), err)
	}
	defer resp.Body.Close()
	result := restResult{Status: resp.StatusCode, Header: resp.Header}
	result.Body, err = io.ReadAll(io.LimitReader(resp.Body, 4<<20+1))
	if err != nil {
		t.Fatalf("%s %s HTTP=%d duration=%s read: %v", method, diagnosticURL(resp.Request.URL.String()), resp.StatusCode, time.Since(started), err)
	}
	if len(result.Body) > 4<<20 {
		t.Fatalf("%s %s response exceeds 4 MiB", method, diagnosticURL(resp.Request.URL.String()))
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		_ = json.Unmarshal(result.Body, &result.JSON)
	}
	responseLog := diagnosticBody(result.Body, c.APIKey, c.Password)
	contentType := resp.Header.Get("Content-Type")
	binary := strings.HasPrefix(contentType, "image/") || strings.Contains(contentType, "application/octet-stream") || resp.Header.Get("Content-Disposition") != ""
	if binary || result.JSON == nil && !strings.Contains(contentType, "text/event-stream") {
		responseLog = fmt.Sprintf("bytes=%d content_type=%q disposition=%q sha256=%x", len(result.Body), resp.Header.Get("Content-Type"), resp.Header.Get("Content-Disposition"), sha256.Sum256(result.Body))
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		events, parseErr := parseSSE(strings.NewReader(string(result.Body)))
		preview := make([]any, 0, min(len(events), 5))
		for _, event := range events[:min(len(events), 5)] {
			var data any
			if json.Unmarshal([]byte(event.Data), &data) == nil {
				preview = append(preview, apiObject{"event": event.Event, "data": data})
			} else if event.Data == "[DONE]" {
				preview = append(preview, "[DONE]")
			} else {
				preview = append(preview, "[non-JSON SSE data omitted]")
			}
		}
		encoded, _ := json.Marshal(preview)
		responseLog = fmt.Sprintf("SSE bytes=%d events=%d parse_error=%v preview=%s", len(result.Body), len(events), parseErr, diagnosticBody(encoded, c.APIKey, c.Password))
	}
	t.Logf("%s %s payload=%s HTTP=%d code=%v response=%s duration=%s", method, diagnosticURL(requestURL), requestSummary, result.Status, result.JSON["code"], responseLog, time.Since(started))
	return result
}

// rawRESTCall shares bounded response handling and redaction with JSON requests.
func rawRESTCall(t *testing.T, c *ServerTestConfig, method, path string, body []byte, contentType string) restResult {
	t.Helper()
	summary := fmt.Sprintf("content_type=%q bytes=%d sha256=%x body=%s", contentType, len(body), sha256.Sum256(body), diagnosticBody(body, c.APIKey, c.Password))
	return rawRESTRequest(t, c, method, path, body, contentType, summary)
}

func rawRESTRequest(t *testing.T, c *ServerTestConfig, method, path string, body []byte, contentType, summary string) restResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	started := time.Now()
	requestURL := c.apiBase() + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("%s %s payload=%s create request: %v", method, diagnosticURL(requestURL), summary, err)
	}
	req.Header.Set("Content-Type", contentType)
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := chunkHTTPClient.Do(req)
	return readRESTResponse(t, c, method, requestURL, summary, started, resp, err)
}

type documentUpload struct {
	Name    string
	Content []byte
}

func multipartRESTDocuments(t *testing.T, c *ServerTestConfig, datasetID string, files []documentUpload) restResult {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	summary := make([]apiObject, 0, len(files))
	for _, file := range files {
		part, err := writer.CreateFormFile("file", file.Name)
		if err != nil {
			t.Fatalf("multipart file %q: %v", file.Name, err)
		}
		if _, err := part.Write(file.Content); err != nil {
			t.Fatalf("write multipart file %q: %v", file.Name, err)
		}
		summary = append(summary, apiObject{"name": file.Name, "size": len(file.Content)})
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(apiObject{"files": summary, "field": "file"})
	if err != nil {
		t.Fatal(err)
	}
	return rawRESTRequest(t, c, http.MethodPost, "/datasets/"+datasetID+"/documents", body.Bytes(), writer.FormDataContentType(), diagnosticBody(encoded, c.APIKey, c.Password))
}

func uploadRESTDocuments(t *testing.T, c *ServerTestConfig, datasetID string, files []documentUpload) []any {
	t.Helper()
	items := array(t, restOK(t, multipartRESTDocuments(t, c, datasetID, files))["data"])
	if len(items) != len(files) {
		t.Fatalf("uploaded documents=%d want=%d", len(items), len(files))
	}
	seen := map[string]bool{}
	for _, item := range items {
		id, ok := object(t, item)["id"].(string)
		if !ok || id == "" || seen[id] {
			t.Fatalf("invalid or duplicate uploaded document ID %v", object(t, item)["id"])
		}
		seen[id] = true
	}
	return items
}

func restCode(t *testing.T, r restResult, status, code int) {
	t.Helper()
	if r.Status != status {
		t.Fatalf("HTTP status=%d want=%d", r.Status, status)
	}
	actual, ok := r.JSON["code"].(float64)
	if !ok || actual != float64(code) {
		t.Fatalf("application code=%v want=%d (see request diagnostic)", r.JSON["code"], code)
	}
}
func restOK(t *testing.T, r restResult) map[string]any {
	t.Helper()
	restCode(t, r, 200, 0)
	return r.JSON
}

func cleanupResource(t *testing.T, c *ServerTestConfig, family, id string) {
	t.Helper()
	t.Cleanup(func() {
		path := "/" + family + "/" + id
		var body any
		if family == "chats" || family == "datasets" {
			path = "/" + family
			body = apiObject{"ids": []string{id}}
		}
		r := restCall(t, c, http.MethodDelete, path, body, nil)
		// Tests exercise deletion themselves; a subsequently missing resource is expected.
		code, ok := r.JSON["code"].(float64)
		// Dataset deletion reports 403 for an already absent ID. The read below
		// must still prove absence, so an actual permission failure cannot pass.
		missingDataset := family == "datasets" && code == 403
		if r.Status != 200 || !ok || (code != 0 && code != 102 && code != 103 && code != 404 && !missingDataset) {
			t.Errorf("cleanup %s/%s failed (see diagnostic)", family, id)
		}
		// A bulk-delete envelope can report partial failures. Prove absence rather
		// than treating a general error code as evidence that cleanup succeeded.
		switch family {
		case "agents":
			restCode(t, restCall(t, c, http.MethodGet, "/agents/"+id, nil, nil), 200, 103)
		case "chats":
			restCode(t, restCall(t, c, http.MethodGet, "/chats/"+id, nil, nil), 200, 404)
		case "datasets":
			result := restCall(t, c, http.MethodGet, "/datasets", nil, apiObject{"id": id})
			if result.Status == http.StatusOK && result.JSON["code"] == float64(404) {
				return
			}
			remaining := restOK(t, result)
			if len(array(t, remaining["data"])) != 0 {
				t.Errorf("cleanup left dataset %s present", id)
			}
		}
	})
}

func TestRESTHelperTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/cancel" {
			<-r.Context().Done()
			return
		}
		if r.Method != http.MethodPost || r.URL.Query().Get("keywords") != "hello & 中文" {
			t.Errorf("method=%s query=%s", r.Method, r.URL.RawQuery)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Errorf("unexpected authentication header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"data":{"id":"created"}}`)
	}))
	defer server.Close()
	c := &ServerTestConfig{HostAddress: server.URL, APIVersion: "v1", APIKey: "test-secret"}
	r := restCall(t, c, http.MethodPost, "/echo", apiObject{"password": "not-for-logs"}, apiObject{"keywords": "hello & 中文"})
	if object(t, restOK(t, r)["data"])["id"] != "created" {
		t.Fatal("response object was lost")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	resp, err := c.RequestJSONContext(ctx, http.MethodGet, "/cancel", nil, nil, nil)
	if resp != nil {
		resp.Body.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation error=%v", err)
	}
}

func TestRESTHelperMultipartAndBinary(t *testing.T) {
	files := []documentUpload{{Name: "first.txt", Content: []byte("first\x00document")}, {Name: "second.md", Content: []byte("# Second\n")}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("raw request did not use API key")
		}
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", `attachment; filename="first.txt"`)
			_, _ = w.Write(files[0].Content)
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer r.MultipartForm.RemoveAll()
		parts := r.MultipartForm.File["file"]
		if len(parts) != len(files) {
			t.Errorf("multipart files=%d want=%d", len(parts), len(files))
			return
		}
		for i, part := range parts {
			f, err := part.Open()
			if err != nil {
				t.Error(err)
				return
			}
			content, err := io.ReadAll(f)
			f.Close()
			if err != nil || part.Filename != files[i].Name || !bytes.Equal(content, files[i].Content) {
				t.Errorf("multipart file %d changed: name=%q read_error=%v", i, part.Filename, err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"data":[{"id":"first"},{"id":"second"}]}`)
	}))
	t.Cleanup(server.Close)
	c := &ServerTestConfig{HostAddress: server.URL, APIVersion: "v1", APIKey: "test-secret", AuthToken: "must-not-be-used"}
	uploadRESTDocuments(t, c, "dataset", files)
	r := rawRESTCall(t, c, http.MethodGet, "/documents/first", nil, "")
	if r.Status != http.StatusOK || r.JSON != nil || !bytes.Equal(r.Body, files[0].Content) || r.Header.Get("Content-Disposition") != `attachment; filename="first.txt"` {
		t.Fatal("binary response bytes or headers were lost")
	}
}

func TestRESTHelperDatasetCleanup(t *testing.T) {
	for _, tc := range []struct{ name, deleted, body string }{
		{"empty_list", `{"code":0,"data":{"success_count":1,"errors":[]}}`, `{"code":0,"data":[]}`},
		{"missing_id", `{"code":0,"data":{"success_count":1,"errors":[]}}`, `{"code":404,"message":"Resource not found"}`},
		{"already_deleted", `{"code":403,"message":"Resource not found"}`, `{"code":404,"message":"Resource not found"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var deleted, verified atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodDelete && r.URL.Path == "/api/v1/datasets" {
					deleted.Store(true)
					_, _ = io.WriteString(w, tc.deleted)
					return
				}
				if !deleted.Load() || r.Method != http.MethodGet || r.URL.Query().Get("id") != "dataset-id" {
					t.Error("cleanup did not delete then verify the selected dataset")
				}
				verified.Store(true)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(server.Close)
			t.Cleanup(func() {
				if !deleted.Load() || !verified.Load() {
					t.Error("dataset cleanup did not execute both requests")
				}
			})
			config := &ServerTestConfig{HostAddress: server.URL, APIVersion: "v1", APIKey: "test-secret"}
			cleanupResource(t, config, "datasets", "dataset-id")
		})
	}
}
