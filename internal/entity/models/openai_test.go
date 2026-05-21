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

func newOpenAIForTest(baseURL string) *OpenAIModel {
	return NewOpenAIModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:      "chat/completions",
			Models:    "models",
			Embedding: "embeddings",
			ASR:       "audio/transcriptions",
			TTS:       "audio/speech",
		},
	)
}

func TestOpenAIConfigAdvertisedAudioModelsHaveSuffixes(t *testing.T) {
	raw, err := os.ReadFile("../../../conf/models/openai.json")
	if err != nil {
		t.Fatalf("read openai config: %v", err)
	}

	var cfg struct {
		URLSuffix URLSuffix `json:"url_suffix"`
		Models    []struct {
			Name       string   `json:"name"`
			ModelTypes []string `json:"model_types"`
		} `json:"models"`
	}
	if err = json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal openai config: %v", err)
	}

	if cfg.URLSuffix.ASR != "audio/transcriptions" {
		t.Fatalf("ASR suffix=%q, want audio/transcriptions", cfg.URLSuffix.ASR)
	}
	if cfg.URLSuffix.TTS != "audio/speech" {
		t.Fatalf("TTS suffix=%q, want audio/speech", cfg.URLSuffix.TTS)
	}

	var hasASR, hasTTS bool
	for _, model := range cfg.Models {
		for _, modelType := range model.ModelTypes {
			if model.Name == "whisper-1" && modelType == "asr" {
				hasASR = true
			}
			if model.Name == "tts-1" && modelType == "tts" {
				hasTTS = true
			}
		}
	}
	if !hasASR {
		t.Fatal("openai config should advertise whisper-1 as ASR")
	}
	if !hasTTS {
		t.Fatal("openai config should advertise tts-1 as TTS")
	}
}

func TestOpenAITranscribeAudioPostsMultipartToAudioEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
		}
		if r.URL.Path != "/audio/transcriptions" {
			t.Errorf("path=%s, want /audio/transcriptions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q, want Bearer test-key", got)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "multipart/form-data") {
			t.Errorf("Content-Type=%q, want multipart/form-data", got)
		}

		if err := r.ParseMultipartForm(1024 * 1024); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		if got := r.FormValue("model"); got != "whisper-1" {
			t.Errorf("model=%q, want whisper-1", got)
		}
		if got := r.FormValue("language"); got != "en" {
			t.Errorf("language=%q, want en", got)
		}
		if got := r.FormValue("temperature"); got != "0.2" {
			t.Errorf("temperature=%q, want 0.2", got)
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read upload: %v", err)
		}
		if string(content) != "audio-bytes" {
			t.Errorf("file content=%q, want audio-bytes", string(content))
		}

		_ = json.NewEncoder(w).Encode(map[string]string{"text": "hello world"})
	}))
	defer srv.Close()

	audioPath := t.TempDir() + "/sample.wav"
	if err := os.WriteFile(audioPath, []byte("audio-bytes"), 0600); err != nil {
		t.Fatalf("write audio fixture: %v", err)
	}

	apiKey := "test-key"
	model := "whisper-1"
	resp, err := newOpenAIForTest(srv.URL).TranscribeAudio(
		&model,
		&audioPath,
		&APIConfig{ApiKey: &apiKey},
		&ASRConfig{Params: map[string]interface{}{
			"language":    "en",
			"temperature": 0.2,
		}},
	)
	if err != nil {
		t.Fatalf("TranscribeAudio: %v", err)
	}
	if resp.Text != "hello world" {
		t.Fatalf("Text=%q, want hello world", resp.Text)
	}
}

func TestOpenAIAudioSpeechPostsJSONToAudioEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
		}
		if r.URL.Path != "/audio/speech" {
			t.Errorf("path=%s, want /audio/speech", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q, want Bearer test-key", got)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("Content-Type=%q, want application/json", got)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["model"] != "tts-1" {
			t.Errorf("model=%v, want tts-1", body["model"])
		}
		if body["input"] != "hello" {
			t.Errorf("input=%v, want hello", body["input"])
		}
		if body["voice"] != "alloy" {
			t.Errorf("voice=%v, want alloy", body["voice"])
		}
		if body["response_format"] != "wav" {
			t.Errorf("response_format=%v, want wav", body["response_format"])
		}
		if body["speed"] != float64(1.25) {
			t.Errorf("speed=%v, want 1.25", body["speed"])
		}

		_, _ = w.Write([]byte("audio-bytes"))
	}))
	defer srv.Close()

	apiKey := "test-key"
	model := "tts-1"
	input := "hello"
	resp, err := newOpenAIForTest(srv.URL).AudioSpeech(
		&model,
		&input,
		&APIConfig{ApiKey: &apiKey},
		&TTSConfig{
			Format: "wav",
			Params: map[string]interface{}{
				"voice": "alloy",
				"speed": 1.25,
			},
		},
	)
	if err != nil {
		t.Fatalf("AudioSpeech: %v", err)
	}
	if string(resp.Audio) != "audio-bytes" {
		t.Fatalf("Audio=%q, want audio-bytes", string(resp.Audio))
	}
}

func TestOpenAIAudioSpeechRequiresVoice(t *testing.T) {
	apiKey := "test-key"
	model := "tts-1"
	input := "hello"

	_, err := newOpenAIForTest("http://unused").AudioSpeech(
		&model,
		&input,
		&APIConfig{ApiKey: &apiKey},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "voice is required") {
		t.Fatalf("err=%v, want voice is required", err)
	}
}

func TestOpenAIAudioSpeechWithSenderUnsupported(t *testing.T) {
	apiKey := "test-key"
	model := "tts-1"
	input := "hello"

	err := newOpenAIForTest("http://unused").AudioSpeechWithSender(
		&model,
		&input,
		&APIConfig{ApiKey: &apiKey},
		&TTSConfig{Params: map[string]interface{}{"voice": "alloy"}},
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "streaming TTS not implemented") {
		t.Fatalf("err=%v, want streaming TTS not implemented", err)
	}
}
