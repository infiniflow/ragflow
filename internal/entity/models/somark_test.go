package models

import (
	"bytes"
	"image/png"
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
				if part.FormName() == "file" {
					probe, decodeErr := png.Decode(bytes.NewReader(value))
					if decodeErr != nil {
						t.Errorf("decode verification PNG: %v", decodeErr)
					} else if bounds := probe.Bounds(); bounds.Dx() != 1 || bounds.Dy() != 1 {
						t.Errorf("verification PNG bounds = %v, want 1x1", bounds)
					}
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
		_, _ = w.Write([]byte(`{"code":1107,"message":"无效的API密钥"}`))
	}))
	defer server.Close()

	driver := NewSoMarkModel(
		map[string]string{"default": server.URL + "/api/v1"},
		URLSuffix{OCR: "parse/async", Tasks: "parse/async_check"},
	)
	apiKey := "bad-key"
	err := driver.VerifyOCRConnection(t.Context(), &APIConfig{ApiKey: &apiKey})
	if err == nil || err.Error() != "API request rejected (provider error code=1107)" {
		t.Fatalf("VerifyOCRConnection error = %v, want normalized provider error", err)
	}
	if strings.Contains(err.Error(), "无效的API密钥") {
		t.Fatalf("VerifyOCRConnection error = %q, must not expose provider-specific localized text", err)
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

func TestSoMarkPollFailureIgnoresGenericQuerySuccessMessage(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/parse/async":
			_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"task-failed"}}`))
		case "/api/v1/parse/async_check":
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
	modelName := "SoMark-model"
	_, err := driver.OCRFile(t.Context(), &modelName, []byte("%PDF-1.4"), nil, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil {
		t.Fatal("OCRFile error = nil, want failed task error")
	}
	if !strings.Contains(err.Error(), "task-failed") || !strings.Contains(err.Error(), "status=FAILED") {
		t.Fatalf("OCRFile error = %q, want task id and failed status", err)
	}
	if strings.Contains(err.Error(), "查询成功") {
		t.Fatalf("OCRFile error = %q, must not report generic query success as task failure reason", err)
	}
	if !strings.Contains(err.Error(), "no task error details") {
		t.Fatalf("OCRFile error = %q, want clear missing task-error details", err)
	}
}

func TestSoMarkPollFailureReturnsTaskErrorDetails(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/parse/async":
			_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"task-failed"}}`))
		case "/api/v1/parse/async_check":
			_, _ = w.Write([]byte(`{"code":0,"message":"查询成功","data":{"status":"FAILED","error_message":"invalid API key"}}`))
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
	modelName := "SoMark-model"
	_, err := driver.OCRFile(t.Context(), &modelName, []byte("%PDF-1.4"), nil, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid API key") {
		t.Fatalf("OCRFile error = %v, want task error details", err)
	}
	if strings.Contains(err.Error(), "查询成功") {
		t.Fatalf("OCRFile error = %q, must not report generic query success as task failure reason", err)
	}
}
