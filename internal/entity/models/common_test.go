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

func TestNormalizeModelFamily(t *testing.T) {
	tests := []struct {
		name  string
		input *string
		want  string
	}{
		{name: "nil", input: nil, want: ""},
		{name: "empty", input: modelFamilyTestString(""), want: ""},
		{name: "qwen3", input: modelFamilyTestString("qwen3"), want: "qwen3"},
		{name: "qwen3 hyphen variant", input: modelFamilyTestString("qwen3-8b"), want: "qwen3"},
		{name: "qwen3 dot variant", input: modelFamilyTestString("qwen3.5-4b"), want: "qwen3"},
		{name: "provider-prefixed qwen3", input: modelFamilyTestString("qwen/qwen3-8b"), want: "qwen3"},
		{name: "case-varied qwen3", input: modelFamilyTestString("Qwen/Qwen3.5-4B"), want: "qwen3"},
		{name: "provider-prefixed non-qwen", input: modelFamilyTestString("deepseek/deepseek-r1"), want: "deepseek"},
		{name: "qwen plus not qwen3", input: modelFamilyTestString("qwen-plus"), want: "qwen"},
		{name: "provider-prefixed qwen2.5 not qwen3", input: modelFamilyTestString("qwen/qwen2.5-32b-instruct"), want: "qwen2.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeModelFamily(tt.input); got != tt.want {
				t.Fatalf("NormalizeModelFamily()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetThinkingAndAnswerExtractsQwenThinking(t *testing.T) {
	content := "<think>\nplan</think>\nanswer"

	for _, modelType := range []string{
		"qwen3",
		"qwen3-8b",
		"qwen/qwen3",
		"qwen/qwen3-8b",
		"Qwen/Qwen3.5-4B",
	} {
		t.Run(modelType, func(t *testing.T) {
			thinking, answer := GetThinkingAndAnswer(&modelType, &content)
			if thinking == nil || *thinking != "plan" {
				t.Fatalf("thinking=%v, want plan", thinking)
			}
			if answer == nil || *answer != "answer" {
				t.Fatalf("answer=%v, want answer", answer)
			}
		})
	}
}

func TestGetThinkingAndAnswerLeavesUnknownModelFamiliesUnchanged(t *testing.T) {
	content := "<think>\nplan</think>\nanswer"

	for _, modelType := range []string{
		"deepseek",
		"deepseek/deepseek-r1",
		"qwen-plus",
		"qwen/qwen2.5-32b-instruct",
	} {
		t.Run(modelType, func(t *testing.T) {
			thinking, answer := GetThinkingAndAnswer(&modelType, &content)
			if thinking != nil {
				t.Fatalf("thinking=%v, want nil", thinking)
			}
			if answer != &content {
				t.Fatalf("answer pointer changed")
			}
		})
	}
}

func TestGetThinkingAndAnswerHandlesNilInputs(t *testing.T) {
	thinking, answer := GetThinkingAndAnswer(nil, nil)
	if thinking != nil || answer != nil {
		t.Fatalf("GetThinkingAndAnswer(nil, nil)=(%v, %v), want nils", thinking, answer)
	}

	content := "<think>\nplan</think>\nanswer"
	thinking, answer = GetThinkingAndAnswer(nil, &content)
	if thinking != nil {
		t.Fatalf("thinking=%v, want nil", thinking)
	}
	if answer != &content {
		t.Fatalf("answer pointer changed")
	}

	modelType := "qwen3"
	thinking, answer = GetThinkingAndAnswer(&modelType, nil)
	if thinking != nil {
		t.Fatalf("thinking=%v, want nil", thinking)
	}
	if answer != nil {
		t.Fatalf("answer=%v, want nil", answer)
	}
}

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
