package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func newZhipuAIForTest(baseURL string) *ZhipuAIModel {
	return NewZhipuAIModel(
		map[string]string{"default": baseURL},
		URLSuffix{ASR: "audio/transcriptions"},
	)
}

func writeZhipuAITestAudio(t *testing.T) string {
	t.Helper()

	file, err := os.CreateTemp(t.TempDir(), "speech-*.mp3")
	if err != nil {
		t.Fatalf("create temp audio: %v", err)
	}
	defer file.Close()

	if _, err = file.WriteString("fake audio"); err != nil {
		t.Fatalf("write temp audio: %v", err)
	}
	return file.Name()
}

func TestZhipuAITranscribeAudio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
			return
		}
		if r.URL.Path != "/audio/transcriptions" {
			t.Errorf("path=%s", r.URL.Path)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q", got)
			return
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "multipart/form-data") {
			t.Errorf("Content-Type=%q", got)
			return
		}

		if err := r.ParseMultipartForm(1024 * 1024); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
			return
		}
		if got := r.FormValue("model"); got != "glm-asr-2512" {
			t.Errorf("model=%q", got)
		}
		if got := r.MultipartForm.Value["model"]; len(got) != 1 {
			t.Errorf("model values=%v", got)
		}
		if got := r.FormValue("stream"); got != "false" {
			t.Errorf("stream=%q", got)
		}
		if got := r.MultipartForm.Value["stream"]; len(got) != 1 {
			t.Errorf("stream values=%v", got)
		}
		if got := r.FormValue("prompt"); got != "previous transcript" {
			t.Errorf("prompt=%q", got)
		}
		if got := r.FormValue("user_id"); got != "12345" {
			t.Errorf("user_id=%q", got)
		}
		if got := r.MultipartForm.Value["hotwords"]; len(got) != 2 || got[0] != "RAGFlow" || got[1] != "ZhipuAI" {
			t.Errorf("hotwords=%v", got)
		}
		if got := r.MultipartForm.Value["file"]; len(got) != 0 {
			t.Errorf("file values=%v", got)
		}
		if _, _, err := r.FormFile("file"); err != nil {
			t.Errorf("file field: %v", err)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]string{"text": "hello world"})
	}))
	defer srv.Close()

	apiKey := "test-key"
	modelName := "glm-asr-2512"
	file := writeZhipuAITestAudio(t)
	resp, err := newZhipuAIForTest(srv.URL).TranscribeAudio(
		&modelName,
		&file,
		&APIConfig{ApiKey: &apiKey},
		&ASRConfig{Params: map[string]interface{}{
			"prompt":    "previous transcript",
			"hotwords":  []string{"RAGFlow", "ZhipuAI"},
			"model":     "ignored-model",
			"stream":    true,
			"file":      "ignored-file",
			"user_id":   12345,
			"nil_value": nil,
		}},
	)
	if err != nil {
		t.Fatalf("TranscribeAudio: %v", err)
	}
	if resp.Text != "hello world" {
		t.Errorf("Text=%q", resp.Text)
	}
}

func TestZhipuAITranscribeAudioValidation(t *testing.T) {
	apiKey := "test-key"
	modelName := "glm-asr-2512"
	file := "speech.mp3"

	tests := []struct {
		name      string
		modelName *string
		file      *string
		apiConfig *APIConfig
		want      string
	}{
		{name: "missing api key", modelName: &modelName, file: &file, apiConfig: &APIConfig{}, want: "api key is required"},
		{name: "missing model", modelName: nil, file: &file, apiConfig: &APIConfig{ApiKey: &apiKey}, want: "model name is required"},
		{name: "missing file", modelName: &modelName, file: nil, apiConfig: &APIConfig{ApiKey: &apiKey}, want: "file is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newZhipuAIForTest("http://unused").TranscribeAudio(tt.modelName, tt.file, tt.apiConfig, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestZhipuAITranscribeAudioRequiresASRSuffix(t *testing.T) {
	apiKey := "test-key"
	modelName := "glm-asr-2512"
	file := writeZhipuAITestAudio(t)
	_, err := NewZhipuAIModel(map[string]string{"default": "http://unused"}, URLSuffix{}).TranscribeAudio(
		&modelName,
		&file,
		&APIConfig{ApiKey: &apiKey},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "ASR URL suffix is not configured") {
		t.Fatalf("error=%v", err)
	}
}

func TestZhipuAITranscribeAudioHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"bad request"}`)
	}))
	defer srv.Close()

	apiKey := "test-key"
	modelName := "glm-asr-2512"
	file := writeZhipuAITestAudio(t)
	_, err := newZhipuAIForTest(srv.URL).TranscribeAudio(&modelName, &file, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "ZhipuAI ASR API error: 400 Bad Request") {
		t.Fatalf("error=%v", err)
	}
}
