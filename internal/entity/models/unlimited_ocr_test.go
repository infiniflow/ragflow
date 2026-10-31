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
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResolveUnlimitedOCRServerURL(t *testing.T) {
	t.Run("read from BaseURL", func(t *testing.T) {
		raw := "http://custom-unlimited:18000/"
		cfg := &APIConfig{BaseURL: &raw}
		if got := ResolveUnlimitedOCRServerURL(cfg); got != "http://custom-unlimited:18000" {
			t.Fatalf("expected http://custom-unlimited:18000, got %s", got)
		}
	})

	t.Run("read from ApiKey JSON", func(t *testing.T) {
		jsonKey := `{"unlimited_ocr_server_url":"http://json-unlimited:18000"}`
		cfg := &APIConfig{ApiKey: &jsonKey}
		if got := ResolveUnlimitedOCRServerURL(cfg); got != "http://json-unlimited:18000" {
			t.Fatalf("expected http://json-unlimited:18000, got %s", got)
		}
	})

	t.Run("fallback default when nil", func(t *testing.T) {
		got := ResolveUnlimitedOCRServerURL(nil)
		if got == "" {
			t.Fatal("expected non-empty default URL")
		}
	})
}

func TestUnlimitedOCRModel_CheckConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/v1/health" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"status":"ok","version":"4.0.12","engine":"Unlimited-OCR-2"}`)
			return
		}
		if r.URL.Path == "/file_parse" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"code":0,"task_id":"task_123"}`)
			return
		}
		if r.URL.Path == "/tasks/task_123" || r.URL.Path == "/tasks/task_123/result" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"code":0,"state":"done","results":{"upload_document.pdf":{"md_content":"# Recognized Document\n\nSample content"}}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	driver := NewUnlimitedOCRModel(
		map[string]string{"default": server.URL},
		URLSuffix{DocumentParse: "file_parse", Task: "tasks"},
	)

	apiConfig := &APIConfig{BaseURL: &server.URL}
	ctx := context.Background()

	if err := driver.CheckConnection(ctx, apiConfig); err != nil {
		t.Fatalf("CheckConnection failed: %v", err)
	}

	models, err := driver.ListModels(ctx, apiConfig)
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(models) != 1 || models[0].Name != "Unlimited-OCR-2" {
		t.Fatalf("unexpected models: %#v", models)
	}

	// Verification probe test (empty content)
	verifyResp, err := driver.OCRFile(ctx, nil, nil, nil, apiConfig, nil, nil)
	if err != nil {
		t.Fatalf("verification probe OCRFile failed: %v", err)
	}
	if verifyResp == nil {
		t.Fatal("expected non-nil OCRFile response for probe")
	}

	// Actual OCR request test
	ocrResp, err := driver.OCRFile(ctx, nil, []byte("fake-pdf-content"), nil, apiConfig, nil, nil)
	if err != nil {
		t.Fatalf("OCRFile failed: %v", err)
	}
	if ocrResp == nil || ocrResp.Text == nil || *ocrResp.Text == "" {
		t.Fatalf("expected non-empty OCR text, got %#v", ocrResp)
	}
	if *ocrResp.Text != "# Recognized Document\n\nSample content" {
		t.Fatalf("unexpected OCR text: %q", *ocrResp.Text)
	}
}

func TestUnlimitedOCRModel_CheckConnection_Auth(t *testing.T) {
	const validToken = "secret-token-123"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+validToken {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"error":"unauthorized"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"ok","version":"4.0.12","engine":"Unlimited-OCR-2"}`)
	}))
	defer server.Close()

	driver := NewUnlimitedOCRModel(
		map[string]string{"default": server.URL},
		URLSuffix{DocumentParse: "file_parse", Task: "tasks"},
	)
	ctx := context.Background()

	// Without API key -> 401 failure
	noKeyCfg := &APIConfig{BaseURL: &server.URL}
	if err := driver.CheckConnection(ctx, noKeyCfg); err == nil {
		t.Fatal("expected authentication error, got nil")
	}

	// With plain token API key -> success
	tok := validToken
	validCfg := &APIConfig{BaseURL: &server.URL, ApiKey: &tok}
	if err := driver.CheckConnection(ctx, validCfg); err != nil {
		t.Fatalf("expected success with valid token, got: %v", err)
	}

	// With JSON API key -> success
	jsonKey := fmt.Sprintf(`{"unlimited_ocr_api_key":"%s"}`, validToken)
	jsonCfg := &APIConfig{BaseURL: &server.URL, ApiKey: &jsonKey}
	if err := driver.CheckConnection(ctx, jsonCfg); err != nil {
		t.Fatalf("expected success with JSON token, got: %v", err)
	}
}

func TestUnlimitedOCRModel_ParseFile_TaskID_Fallback(t *testing.T) {
	ctx := context.Background()

	t.Run("nested task_id in data", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"task_id":"nested_123"},"task_id":"top_999"}`)
		}))
		defer server.Close()

		driver := NewUnlimitedOCRModel(
			map[string]string{"default": server.URL},
			URLSuffix{DocumentParse: "file_parse", Task: "tasks"},
		)
		cfg := &APIConfig{BaseURL: &server.URL}
		resp, err := driver.ParseFile(ctx, nil, []byte("dummy"), nil, cfg, nil, nil)
		if err != nil {
			t.Fatalf("ParseFile failed: %v", err)
		}
		if resp.TaskID != "nested_123" {
			t.Fatalf("expected nested_123, got %s", resp.TaskID)
		}
	})

	t.Run("top-level task_id when data is object without task_id", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"status":"queued"},"task_id":"top_999"}`)
		}))
		defer server.Close()

		driver := NewUnlimitedOCRModel(
			map[string]string{"default": server.URL},
			URLSuffix{DocumentParse: "file_parse", Task: "tasks"},
		)
		cfg := &APIConfig{BaseURL: &server.URL}
		resp, err := driver.ParseFile(ctx, nil, []byte("dummy"), nil, cfg, nil, nil)
		if err != nil {
			t.Fatalf("ParseFile failed: %v", err)
		}
		if resp.TaskID != "top_999" {
			t.Fatalf("expected top_999, got %s", resp.TaskID)
		}
	})
}

func TestUnlimitedOCRModel_OCRFile_AuthFailureStopsPolling(t *testing.T) {
	ctx := context.Background()
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health", "/v1/health":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"status":"ok","version":"4.0.12","engine":"Unlimited-OCR-2"}`)
		case "/file_parse":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"code":0,"task_id":"task_auth_fail"}`)
		case "/tasks/task_auth_fail/result":
			callCount++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"code":401,"message":"unauthorized token"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	driver := NewUnlimitedOCRModel(
		map[string]string{"default": server.URL},
		URLSuffix{DocumentParse: "file_parse", Task: "tasks"},
	)
	cfg := &APIConfig{BaseURL: &server.URL}
	content := []byte("%PDF-1.4 test document content that exceeds 100 bytes of data for real testing")
	start := time.Now()
	_, err := driver.OCRFile(ctx, nil, content, nil, cfg, nil, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrTaskAuthorization) {
		t.Fatalf("expected ErrTaskAuthorization, got: %v", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("polling took %s, expected immediate return on 401", elapsed)
	}
	if callCount > 2 {
		t.Fatalf("ShowTask called %d times, expected immediate stop", callCount)
	}
}
