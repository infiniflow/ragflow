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
