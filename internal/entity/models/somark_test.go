package models

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSoMarkVerifyOCRConnectionStopsAfterTaskAccepted(t *testing.T) {
	withSSRFBypass(t)
	var submitCalls, pollCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/parse/async":
			submitCalls++
			reader, err := r.MultipartReader()
			if err != nil {
				t.Errorf("MultipartReader: %v", err)
				return
			}
			var apiKey string
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Errorf("NextPart: %v", err)
					return
				}
				value, _ := io.ReadAll(part)
				if part.FormName() == "api_key" {
					apiKey = string(value)
				}
				if part.FormName() == "file" && !strings.HasPrefix(string(value), "\x89PNG\r\n\x1a\n") {
					t.Errorf("probe file does not contain a PNG")
				}
			}
			if apiKey != "somark-test-key" {
				t.Errorf("api_key = %q, want configured key", apiKey)
			}
			_, _ = w.Write([]byte(`{"code":0,"message":"accepted","data":{"task_id":"task-verify"}}`))
		case "/api/v1/parse/async_check":
			pollCalls++
			// A 1x1 probe can legitimately fail parsing; model verification only
			// checks that SoMark accepted the authenticated submission.
			_, _ = w.Write([]byte(`{"code":0,"message":"查询成功","data":{"status":"FAILED"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	driver := NewSoMarkModel(
		map[string]string{"default": server.URL + "/api/v1"},
		URLSuffix{OCR: "parse/async", Tasks: "parse/async_check"},
	)
	apiKey := "somark-test-key"
	err := driver.VerifyOCRConnection(t.Context(), &APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("VerifyOCRConnection: %v", err)
	}
	if submitCalls != 1 || pollCalls != 0 {
		t.Fatalf("submit/poll calls = %d/%d, want 1/0", submitCalls, pollCalls)
	}
}

func TestSoMarkVerifyOCRConnectionReturnsSubmitError(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1107,"message":"invalid api key"}`))
	}))
	defer server.Close()

	driver := NewSoMarkModel(
		map[string]string{"default": server.URL + "/api/v1"},
		URLSuffix{OCR: "parse/async", Tasks: "parse/async_check"},
	)
	apiKey := "bad-key"
	err := driver.VerifyOCRConnection(t.Context(), &APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("VerifyOCRConnection error = %v, want invalid API key error", err)
	}
}

func TestSoMarkVerifyOCRConnectionAcceptsStringTaskID(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"message":"accepted","data":"task-verify"}`))
	}))
	defer server.Close()

	driver := NewSoMarkModel(
		map[string]string{"default": server.URL + "/api/v1"},
		URLSuffix{OCR: "parse/async", Tasks: "parse/async_check"},
	)
	apiKey := "somark-test-key"
	if err := driver.VerifyOCRConnection(t.Context(), &APIConfig{ApiKey: &apiKey}); err != nil {
		t.Fatalf("VerifyOCRConnection with string task_id: %v", err)
	}
}
