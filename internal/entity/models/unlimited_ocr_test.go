//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//

package models

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveUnlimitedOCRServerURL(t *testing.T) {
	t.Run("explicit BaseURL takes precedence", func(t *testing.T) {
		baseURL := "http://custom-ocr:18000/"
		apiConfig := &APIConfig{BaseURL: &baseURL}
		if got := ResolveUnlimitedOCRServerURL(apiConfig); got != "http://custom-ocr:18000" {
			t.Fatalf("got %q, want http://custom-ocr:18000", got)
		}
	})

	t.Run("extract from ApiKey JSON", func(t *testing.T) {
		apiKeyJSON := `{"unlimited_ocr_server_url":"http://ocr-node:18000/"}`
		apiConfig := &APIConfig{ApiKey: &apiKeyJSON}
		if got := ResolveUnlimitedOCRServerURL(apiConfig); got != "http://ocr-node:18000" {
			t.Fatalf("got %q, want http://ocr-node:18000", got)
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

	resp, err := driver.OCRFile(ctx, nil, []byte("fake"), nil, apiConfig, nil, nil)
	if err != nil {
		t.Fatalf("OCRFile failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil OCRFile response")
	}
}
