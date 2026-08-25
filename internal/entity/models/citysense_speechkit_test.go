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

func newCitySenseForTest(baseURL string) *CitySenseSpeechKit {
	return NewCitySenseSpeechKitModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			ASR:    "api/v1/transcription/upload",
			Models: "api/v1/models",
		},
	)
}

func writeCitySenseTestAudio(t *testing.T, name string, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write test audio: %v", err)
	}
	return path
}

func TestCitySenseTranscribeAudioRequiresAPIKey(t *testing.T) {
	withSSRFBypass(t)
	file := writeCitySenseTestAudio(t, "a.wav", "data")
	model := "citysense-speech-kit-v1"
	_, err := newCitySenseForTest("http://unused").TranscribeAudio(context.Background(), &model, &file, &APIConfig{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Fatalf("expected api key error, got %v", err)
	}
}

func TestCitySenseTranscribeAudioMultipart(t *testing.T) {
	withSSRFBypass(t)
	var gotToken, gotModel, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/transcription/upload" {
			t.Errorf("path=%s want /api/v1/transcription/upload", r.URL.Path)
		}
		gotToken = r.Header.Get("X-Service-Token")
		gotCT = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
			return
		}
		gotModel = r.FormValue("model")
		// check MIME of file part
		f, fh, err := r.FormFile("file")
		if err != nil {
			t.Errorf("form file: %v", err)
			return
		}
		defer f.Close()
		if fh.Filename != "audio.wav" {
			t.Errorf("filename=%q want audio.wav", fh.Filename)
		}
		if ct := fh.Header.Get("Content-Type"); ct != "audio/wav" {
			t.Errorf("Content-Type=%q want audio/wav", ct)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "привет"})
	}))
	defer srv.Close()

	file := writeCitySenseTestAudio(t, "audio.wav", "fake wav")
	model := "citysense-speech-kit-v1"
	key := "secret"
	resp, err := newCitySenseForTest(srv.URL).TranscribeAudio(context.Background(), &model, &file, &APIConfig{ApiKey: &key}, nil, nil)
	if err != nil {
		t.Fatalf("TranscribeAudio: %v", err)
	}
	if resp.Text != "привет" {
		t.Fatalf("text=%q want привет", resp.Text)
	}
	if gotToken != "secret" {
		t.Errorf("X-Service-Token=%q want secret", gotToken)
	}
	if gotModel != "citysense-speech-kit-v1" {
		t.Errorf("model=%q want citysense-speech-kit-v1", gotModel)
	}
	if !strings.Contains(gotCT, "multipart/form-data") {
		t.Errorf("Content-Type=%q want multipart", gotCT)
	}
}

func TestCitySenseTranscribeAudioViaURL(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/transcription/transcribe" {
			t.Errorf("path=%s want /api/v1/transcription/transcribe", r.URL.Path)
		}
		if got := r.Header.Get("X-Service-Token"); got != "tok" {
			t.Errorf("token=%q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
			return
		}
		if body["mediaFileUrl"] != "https://example.com/a.mp3" {
			t.Errorf("mediaFileUrl=%v", body["mediaFileUrl"])
		}
		_, _ = io.WriteString(w, `{"text":"url transcript"}`)
	}))
	defer srv.Close()

	urlFile := "https://example.com/a.mp3"
	model := "citysense-speech-kit-v1"
	key := "tok"
	resp, err := newCitySenseForTest(srv.URL).TranscribeAudio(context.Background(), &model, &urlFile, &APIConfig{ApiKey: &key}, nil, nil)
	if err != nil {
		t.Fatalf("TranscribeAudio URL: %v", err)
	}
	if resp.Text != "url transcript" {
		t.Fatalf("text=%q", resp.Text)
	}
}

func TestCitySenseListModelsLiveAndFallback(t *testing.T) {
	withSSRFBypass(t)
	// live success
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Service-Token") != "k" {
			t.Errorf("token=%q", r.Header.Get("X-Service-Token"))
		}
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"citysense-speech-kit-v1","owned_by":"CitySense-SpeechKit"}]}`)
	}))
	defer srv.Close()
	key := "k"
	models, err := newCitySenseForTest(srv.URL).ListModels(context.Background(), &APIConfig{ApiKey: &key})
	if err != nil {
		t.Fatalf("ListModels live: %v", err)
	}
	if len(models) != 1 || models[0].Name != "citysense-speech-kit-v1" {
		t.Fatalf("models=%v", models)
	}
	// fallback: server returns 500 -> ListModels should fallback to synthetic, CheckConnection should fail
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv2.Close()
	models, err = newCitySenseForTest(srv2.URL).ListModels(context.Background(), &APIConfig{ApiKey: &key})
	if err != nil {
		t.Fatalf("ListModels fallback should not error, got %v", err)
	}
	if len(models) != 1 || models[0].Name != "citysense-speech-kit-v1" {
		t.Fatalf("fallback models=%v", models)
	}
	if err := newCitySenseForTest(srv2.URL).CheckConnection(context.Background(), &APIConfig{ApiKey: &key}); err == nil {
		t.Fatalf("CheckConnection should fail on 500")
	}
}

func TestCitySenseUnsupportedMethods(t *testing.T) {
	withSSRFBypass(t)
	driver := newCitySenseForTest("http://unused")
	ctx := context.Background()
	key := "k"
	api := &APIConfig{ApiKey: &key}
	model := "citysense-speech-kit-v1"
	text := "hi"
	checks := []struct {
		name string
		call func() error
	}{
		{"ChatWithMessages", func() error { _, err := driver.ChatWithMessages(ctx, "m", nil, api, nil, nil); return err }},
		{"Embed", func() error { _, err := driver.Embed(ctx, &model, EmbedRequest{}, api, nil, nil); return err }},
		{"Rerank", func() error { _, err := driver.Rerank(ctx, &model, RerankRequest{}, api, nil, nil); return err }},
		{"AudioSpeech", func() error { _, err := driver.AudioSpeech(ctx, &model, &text, api, nil, nil); return err }},
		{"OCRFile", func() error { _, err := driver.OCRFile(ctx, &model, nil, &text, api, nil, nil); return err }},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if err := c.call(); err == nil || !strings.Contains(err.Error(), "не поддерживается") {
				t.Fatalf("%s err=%v want unsupported", c.name, err)
			}
		})
	}
}
