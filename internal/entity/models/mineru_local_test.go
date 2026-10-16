//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//

package models

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMinerULocalParseFileResolvesServerURLFromProviderAPIKey(t *testing.T) {
	var gotServerURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		gotServerURL = r.FormValue("server_url")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"task_id":"task-1"}}`))
	}))
	defer server.Close()

	baseURL := server.URL
	driver := NewMinerLocalUModel(map[string]string{"default": baseURL}, URLSuffix{DocumentParse: "file_parse", Task: "tasks"})
	apiKey := `{"mineru_backend":"vlm-http-client","mineru_server_url":"http://vllm:30000"}`
	apiConfig := &APIConfig{
		BaseURL: &baseURL,
		ApiKey:  &apiKey,
	}
	backend := "vlm-http-client"
	_, err := driver.ParseFile(context.Background(), &backend, []byte("%PDF-1.4"), nil, apiConfig, &ParseFileConfig{}, nil)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if gotServerURL != "http://vllm:30000" {
		t.Fatalf("server_url = %q, want http://vllm:30000", gotServerURL)
	}
}

func TestMinerULocalParseFileRequestOverridesProviderJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(10 << 20)
		if r.FormValue("server_url") != "http://override:9" {
			http.Error(w, "bad server_url", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"task_id":"task-2"}`))
	}))
	defer server.Close()

	baseURL := server.URL
	driver := NewMinerLocalUModel(map[string]string{"default": baseURL}, URLSuffix{DocumentParse: "file_parse", Task: "tasks"})
	apiKey := `{"mineru_server_url":"http://vllm:30000"}`
	apiConfig := &APIConfig{BaseURL: &baseURL, ApiKey: &apiKey}
	cfg := &ParseFileConfig{ServerURL: "http://override:9"}
	_, err := driver.ParseFile(context.Background(), nil, []byte("x"), nil, apiConfig, cfg, nil)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
}

// Ensure multipart field names stay stable (regression guard).
func TestMinerULocalParseFileMultipartFieldNames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		names := map[string]bool{}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			names[part.FormName()] = true
			_ = part.Close()
		}
		if !names["files"] || !names["backend"] {
			http.Error(w, "missing fields", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"task_id":"t"}}`))
	}))
	defer server.Close()

	baseURL := server.URL
	driver := NewMinerLocalUModel(map[string]string{"default": baseURL}, URLSuffix{DocumentParse: "file_parse", Task: "tasks"})
	backend := "pipeline"
	_, err := driver.ParseFile(context.Background(), &backend, []byte("data"), nil, &APIConfig{BaseURL: &baseURL}, nil, nil)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
}

func TestMinerULocalCheckConnectionPlainKey(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	baseURL := server.URL
	driver := NewMinerLocalUModel(map[string]string{"default": baseURL}, URLSuffix{DocumentParse: "file_parse", Task: "tasks"})
	apiKey := "plain-secret"
	if err := driver.CheckConnection(context.Background(), &APIConfig{BaseURL: &baseURL, ApiKey: &apiKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
	if gotAuth != "Bearer plain-secret" {
		t.Fatalf("Authorization = %q, want Bearer plain-secret", gotAuth)
	}
}

// Provider JSON config carries no bearer token unless mineru_api_key /
// access_token is present — the health probe must not send the raw payload.
func TestMinerULocalCheckConnectionJSONConfigNoBearer(t *testing.T) {
	var gotAuth string
	var authSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, authSeen = r.Header["Authorization"]
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	baseURL := server.URL
	driver := NewMinerLocalUModel(map[string]string{"default": baseURL}, URLSuffix{DocumentParse: "file_parse", Task: "tasks"})
	apiKey := `{"mineru_backend":"vlm-http-client","mineru_server_url":"http://vllm:30000"}`
	if err := driver.CheckConnection(context.Background(), &APIConfig{BaseURL: &baseURL, ApiKey: &apiKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
	if authSeen && gotAuth != "" {
		t.Fatalf("unexpected Authorization header %q for JSON config without token", gotAuth)
	}
}

// Non-auth codes (e.g. 404 on a service without /health) still prove the
// endpoint is reachable.
func TestMinerULocalCheckConnectionNonAuthCodeIsReachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	baseURL := server.URL
	driver := NewMinerLocalUModel(map[string]string{"default": baseURL}, URLSuffix{DocumentParse: "file_parse", Task: "tasks"})
	if err := driver.CheckConnection(context.Background(), &APIConfig{BaseURL: &baseURL}); err != nil {
		t.Fatalf("CheckConnection on 404: %v", err)
	}
}

func TestMinerULocalCheckConnectionAuthFailure(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "denied", code)
		}))
		baseURL := server.URL
		driver := NewMinerLocalUModel(map[string]string{"default": baseURL}, URLSuffix{DocumentParse: "file_parse", Task: "tasks"})
		apiKey := "wrong-key"
		err := driver.CheckConnection(context.Background(), &APIConfig{BaseURL: &baseURL, ApiKey: &apiKey})
		server.Close()
		if err == nil {
			t.Fatalf("CheckConnection on HTTP %d: expected error", code)
		}
	}
}

func TestMinerULocalCheckConnectionUnreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	baseURL := server.URL
	server.Close()

	driver := NewMinerLocalUModel(map[string]string{"default": baseURL}, URLSuffix{DocumentParse: "file_parse", Task: "tasks"})
	if err := driver.CheckConnection(context.Background(), &APIConfig{BaseURL: &baseURL}); err == nil {
		t.Fatal("CheckConnection against a closed server: expected error")
	}
}

func TestMinerULocalCheckConnectionUsesMinerUAPIServerFromAPIKeyJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	empty := ""
	apiKey := `{"mineru_apiserver":"` + server.URL + `"}`
	driver := NewMinerLocalUModel(map[string]string{"default": ""}, URLSuffix{DocumentParse: "file_parse", Task: "tasks"})
	if err := driver.CheckConnection(context.Background(), &APIConfig{BaseURL: &empty, ApiKey: &apiKey}); err != nil {
		t.Fatalf("CheckConnection with mineru_apiserver in api_key JSON: %v", err)
	}
}

func TestMinerULocalOCRFileDelegatesToCheckConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	baseURL := server.URL
	driver := NewMinerLocalUModel(map[string]string{"default": baseURL}, URLSuffix{DocumentParse: "file_parse", Task: "tasks"})
	res, err := driver.OCRFile(context.Background(), nil, nil, nil, &APIConfig{BaseURL: &baseURL}, nil, nil)
	if err != nil {
		t.Fatalf("OCRFile: %v", err)
	}
	if res == nil {
		t.Fatal("OCRFile returned nil response")
	}
}
