package models

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBaseModelDoRequestAuthorizationHeader(t *testing.T) {
	tests := []struct {
		name       string
		apiConfig  *APIConfig
		wantHeader string
	}{
		{name: "nil api config"},
		{name: "nil api key", apiConfig: &APIConfig{}},
		{name: "blank api key", apiConfig: &APIConfig{ApiKey: modelFamilyTestString("  ")}},
		{name: "api key", apiConfig: &APIConfig{ApiKey: modelFamilyTestString(" key-1 ")}, wantHeader: "Bearer key-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != tt.wantHeader {
					t.Fatalf("Authorization = %q, want %q", got, tt.wantHeader)
				}
				if got := r.Header.Get("Content-Type"); got != "application/json" {
					t.Fatalf("Content-Type = %q, want application/json", got)
				}
				fmt.Fprint(w, `{"ok":true}`)
			}))
			defer server.Close()

			model := &BaseModel{httpClient: server.Client(), AllowEmptyAPIKey: true}
			if _, err := model.doRequest(context.Background(), server.URL, tt.apiConfig, map[string]any{"ok": true}, time.Second); err != nil {
				t.Fatalf("doRequest() error = %v", err)
			}
		})
	}
}

func TestBaseModelDoStreamRequestAllowsMissingAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q, want empty", got)
		}
		fmt.Fprint(w, `data: {"ok":true}`)
	}))
	defer server.Close()

	model := &BaseModel{httpClient: server.Client(), AllowEmptyAPIKey: true}
	err := model.doStreamRequest(context.Background(), server.URL, nil, map[string]any{"ok": true}, time.Second, func(body io.ReadCloser) error {
		_, err := io.ReadAll(body)
		return err
	})
	if err != nil {
		t.Fatalf("doStreamRequest() error = %v", err)
	}
}

func modelFamilyTestString(value string) *string {
	return &value
}
