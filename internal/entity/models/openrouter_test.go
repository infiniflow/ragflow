package models

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newOpenRouterServer(t *testing.T, expectedPath string, handler func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectedPath {
			t.Errorf("path=%s, want %s", r.URL.Path, expectedPath)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q, want Bearer test-key", got)
			return
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("Content-Type=%q, want application/json", got)
			return
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}

		var body map[string]interface{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("unmarshal body: %v\nraw=%s", err, string(raw))
				return
			}
		}
		handler(t, r, body, w)
	}))
}

func newOpenRouterForTest(baseURL string) *OpenRouterModel {
	return NewOpenRouterModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:    "chat/completions",
			Models:  "models",
			ASR:     "audio/transcriptions",
			TTS:     "audio/speech",
			Balance: "credits",
		},
	)
}

func writeOpenRouterAudioFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write audio file: %v", err)
	}
	return path
}

func TestOpenRouterAudioFormatUsesConfiguredValue(t *testing.T) {
	if got := openRouterAudioFormat("sample.wav", &ASRConfig{Params: map[string]interface{}{"format": ".mp3"}}); got != "mp3" {
		t.Errorf("format=%q, want mp3", got)
	}
	if got := openRouterAudioFormat("sample.wav", &ASRConfig{Params: map[string]interface{}{"format": 123}}); got != "123" {
		t.Errorf("format=%q, want 123", got)
	}
}

func TestOpenRouterTranscribeAudioHappyPath(t *testing.T) {
	audio := []byte("RIFF test audio")
	srv := newOpenRouterServer(t, "/audio/transcriptions", func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
		}
		if body["model"] != "openai/whisper-large-v3" {
			t.Errorf("model=%v", body["model"])
		}
		if body["language"] != "en" {
			t.Errorf("language=%v", body["language"])
		}
		if body["temperature"] != 0.2 {
			t.Errorf("temperature=%v", body["temperature"])
		}

		inputAudio, ok := body["input_audio"].(map[string]interface{})
		if !ok {
			t.Errorf("input_audio=%T, want object", body["input_audio"])
			return
		}
		if inputAudio["data"] != base64.StdEncoding.EncodeToString(audio) {
			t.Errorf("input_audio.data=%v", inputAudio["data"])
		}
		if inputAudio["format"] != "wav" {
			t.Errorf("input_audio.format=%v, want wav", inputAudio["format"])
		}
		if _, ok := body["format"]; ok {
			t.Errorf("format should only be sent inside input_audio: %v", body["format"])
		}
		if inputAudio["data"] == "bad-data" {
			t.Errorf("input_audio should not be overwritten by params")
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"text": "hello from audio",
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	modelName := "openai/whisper-large-v3"
	file := writeOpenRouterAudioFile(t, "sample.wav", audio)
	resp, err := newOpenRouterForTest(srv.URL).TranscribeAudio(
		&modelName,
		&file,
		&APIConfig{ApiKey: &apiKey},
		&ASRConfig{Params: map[string]interface{}{
			"format":      "wav",
			"language":    "en",
			"temperature": 0.2,
			"model":       "wrong-model",
			"input_audio": map[string]interface{}{"data": "bad-data", "format": "bad"},
		}},
	)
	if err != nil {
		t.Fatalf("TranscribeAudio: %v", err)
	}
	if resp.Text != "hello from audio" {
		t.Errorf("Text=%q", resp.Text)
	}
}

func TestOpenRouterTranscribeAudioInfersFormat(t *testing.T) {
	srv := newOpenRouterServer(t, "/audio/transcriptions", func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		inputAudio, ok := body["input_audio"].(map[string]interface{})
		if !ok {
			t.Errorf("input_audio=%T, want object", body["input_audio"])
			return
		}
		if inputAudio["format"] != "mp3" {
			t.Errorf("input_audio.format=%v, want mp3", inputAudio["format"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"text": "ok"})
	})
	defer srv.Close()

	apiKey := "test-key"
	modelName := "openai/whisper-large-v3"
	file := writeOpenRouterAudioFile(t, "sample.mp3", []byte("audio"))
	_, err := newOpenRouterForTest(srv.URL).TranscribeAudio(&modelName, &file, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("TranscribeAudio: %v", err)
	}
}

func TestOpenRouterTranscribeAudioValidatesInputs(t *testing.T) {
	modelName := "openai/whisper-large-v3"
	apiKey := "test-key"
	file := "sample.wav"

	tests := []struct {
		name      string
		modelName *string
		file      *string
		apiConfig *APIConfig
		want      string
	}{
		{name: "api key", modelName: &modelName, file: &file, apiConfig: &APIConfig{}, want: "api key is required"},
		{name: "model", file: &file, apiConfig: &APIConfig{ApiKey: &apiKey}, want: "model name is required"},
		{name: "file", modelName: &modelName, apiConfig: &APIConfig{ApiKey: &apiKey}, want: "file is missing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newOpenRouterForTest("http://unused").TranscribeAudio(tt.modelName, tt.file, tt.apiConfig, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestOpenRouterTranscribeAudioValidatesASRSuffix(t *testing.T) {
	apiKey := "test-key"
	modelName := "openai/whisper-large-v3"
	file := "sample.wav"
	model := NewOpenRouterModel(map[string]string{"default": "http://unused"}, URLSuffix{})

	_, err := model.TranscribeAudio(&modelName, &file, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "OpenRouter ASR url suffix is missing") {
		t.Fatalf("err=%v", err)
	}
}

func TestOpenRouterTranscribeAudioHTTPError(t *testing.T) {
	srv := newOpenRouterServer(t, "/audio/transcriptions", func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	defer srv.Close()

	apiKey := "test-key"
	modelName := "openai/whisper-large-v3"
	file := writeOpenRouterAudioFile(t, "sample.wav", []byte("audio"))
	_, err := newOpenRouterForTest(srv.URL).TranscribeAudio(&modelName, &file, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil ||
		!strings.Contains(err.Error(), "OpenRouter ASR API error") ||
		!strings.Contains(err.Error(), "401 Unauthorized") ||
		!strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("err=%v", err)
	}
}

// TestOpenRouterChatStreamlyRequest verifies the streaming chat request is built
// correctly: every model (including qwen/glm) targets the standard chat endpoint,
// and reasoning is requested via OpenRouter's standard `reasoning` object rather
// than the non-standard `thinking` key that the API silently ignores.
func TestOpenRouterChatStreamlyRequest(t *testing.T) {
	var gotPath string
	var gotBody map[string]interface{}
	srv := newOpenRouterServer(t, "/chat/completions", func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		gotPath = r.URL.Path
		gotBody = body
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning\":\"think\"}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	})
	defer srv.Close()

	apiKey := "test-key"
	thinking := true
	var reasoning strings.Builder
	// "qwen_max" would have triggered the removed async-routing branch (empty suffix).
	err := newOpenRouterForTest(srv.URL).ChatStreamlyWithSender(
		"qwen_max",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Thinking: &thinking},
		func(content, reason *string) error {
			if reason != nil {
				reasoning.WriteString(*reason)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender error: %v", err)
	}
	if gotPath != "/chat/completions" {
		t.Errorf("path=%q, want /chat/completions (qwen must not route to async suffix)", gotPath)
	}
	if _, ok := gotBody["thinking"]; ok {
		t.Errorf("request still sends non-standard `thinking` key: %v", gotBody["thinking"])
	}
	reason, ok := gotBody["reasoning"].(map[string]interface{})
	if !ok {
		t.Fatalf("request missing standard `reasoning` object, body=%v", gotBody)
	}
	if reason["enabled"] != true {
		t.Errorf("reasoning.enabled=%v, want true", reason["enabled"])
	}
	if reasoning.String() != "think" {
		t.Errorf("streamed reasoning=%q, want %q", reasoning.String(), "think")
	}
}
