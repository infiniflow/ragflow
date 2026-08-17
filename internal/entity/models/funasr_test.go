package models

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newFunASRForTest(baseURL string) *FunASR {
	return NewFunASRModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			ASR:    "audio/transcriptions",
			Models: "models",
		},
	)
}

func writeFunASRTestAudio(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audio.wav")
	if err := os.WriteFile(path, []byte("test audio"), 0o600); err != nil {
		t.Fatalf("write test audio: %v", err)
	}
	return path
}

func TestFunASRTranscribeAudioWithoutAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
		}
		if r.URL.Path != "/audio/transcriptions" {
			t.Errorf("path=%s, want /audio/transcriptions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization=%q, want no header", got)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart form: %v", err)
			return
		}
		if got := r.FormValue("model"); got != "fun-asr-nano" {
			t.Errorf("model=%q, want fun-asr-nano", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "hello"})
	}))
	defer srv.Close()

	modelName := " fun-asr-nano "
	file := writeFunASRTestAudio(t)
	resp, err := newFunASRForTest(srv.URL).TranscribeAudio(
		context.Background(), &modelName, &file, &APIConfig{}, nil, nil,
	)
	if err != nil {
		t.Fatalf("TranscribeAudio: %v", err)
	}
	if resp == nil || resp.Text != "hello" {
		t.Fatalf("response=%v, want text hello", resp)
	}
}

func TestFunASRTranscribeAudioRequiresModelName(t *testing.T) {
	withSSRFBypass(t)
	file := writeFunASRTestAudio(t)
	apiKey := "test-key"
	blankModelName := "   "
	for _, tc := range []struct {
		name      string
		modelName *string
	}{
		{name: "nil", modelName: nil},
		{name: "whitespace", modelName: &blankModelName},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newFunASRForTest("http://unused").TranscribeAudio(
				context.Background(), tc.modelName, &file, &APIConfig{ApiKey: &apiKey}, nil, nil,
			)
			if err == nil || !strings.Contains(err.Error(), "model name is missing") {
				t.Fatalf("expected missing-model-name error, got %v", err)
			}
		})
	}
}

func TestFunASRListModelsWithoutAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		if r.URL.Path != "/models" {
			t.Errorf("path=%s, want /models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization=%q, want no header", got)
		}
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"fun-asr-nano","owned_by":"funasr"}]}`)
	}))
	defer srv.Close()

	models, err := newFunASRForTest(srv.URL).ListModels(context.Background(), &APIConfig{})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 || models[0].Name != "fun-asr-nano" {
		t.Fatalf("models=%v, want fun-asr-nano", models)
	}
}

func TestFunASRListModelsSendsAuthWhenProvided(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization=%q, want Bearer secret", got)
		}
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"fun-asr-nano"}]}`)
	}))
	defer srv.Close()

	apiKey := " secret "
	if _, err := newFunASRForTest(srv.URL).ListModels(
		context.Background(), &APIConfig{ApiKey: &apiKey},
	); err != nil {
		t.Fatalf("ListModels: %v", err)
	}
}
