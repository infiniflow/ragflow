package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"
)

func configuredGPUStackForTest(t *testing.T, baseURL string) ModelDriver {
	t.Helper()
	var config struct {
		URLSuffix URLSuffix `json:"url_suffix"`
	}
	if err := json.Unmarshal(readProviderConfig(t, "gpustack.json"), &config); err != nil {
		t.Fatal(err)
	}
	driver, err := NewModelFactory().CreateModelDriver("GPUStack", map[string]string{"default": baseURL}, config.URLSuffix)
	if err != nil {
		t.Fatal(err)
	}
	return driver.NewInstance(map[string]string{"default": baseURL})
}

func TestGPUStackRerank(t *testing.T) {
	withSSRFBypass(t)
	for _, tc := range []struct {
		name     string
		basePath string
		config   *RerankConfig
		topN     int
	}{
		{"default", "", nil, 3},
		{"versioned base", "/v1/", &RerankConfig{}, 3},
		{"top n", "/v1", &RerankConfig{TopN: 2}, 2},
		{"clamped top n", "/", &RerankConfig{TopN: 10}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newGPUStackServer(t, "/v1/rerank", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
				if body["model"] != "qwen3-vl-reranker-2b" || body["query"] != "query" || body["top_n"] != float64(tc.topN) {
					t.Errorf("unexpected rerank request: %v", body)
				}
				if !reflect.DeepEqual(body["documents"], []interface{}{"first", "second", "third"}) {
					t.Errorf("documents=%v", body["documents"])
				}
				io.WriteString(w, `{"id":"rank-1","results":[{"index":2,"relevance_score":0.9},{"index":0,"relevance_score":0.1}],"usage":{"prompt_tokens":7,"total_tokens":7}}`)
			})
			defer srv.Close()
			model, key := "qwen3-vl-reranker-2b", "test-key"
			usage := &common.ModelUsage{}
			response, err := configuredGPUStackForTest(t, srv.URL+tc.basePath).Rerank(t.Context(), &model,
				RerankRequest{Query: "query", Documents: []string{"first", "second", "third"}}, &APIConfig{ApiKey: &key}, tc.config, usage)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(response.Data, []RerankResult{{Index: 2, RelevanceScore: 0.9}, {Index: 0, RelevanceScore: 0.1}}) {
				t.Fatalf("results=%+v", response.Data)
			}
			if usage.InputTokens != 7 || usage.TotalTokens != 7 || usage.Type != "rerank" || usage.RequestID != "rank-1" {
				t.Fatalf("usage=%+v", usage)
			}
		})
	}
}

func TestGPUStackRerankRejectsInvalidResponses(t *testing.T) {
	withSSRFBypass(t)
	for _, tc := range []struct{ name, body, message string }{
		{"invalid JSON", `{`, "failed to parse"},
		{"missing results", `{}`, "missing rerank results"},
		{"missing index", `{"results":[{"relevance_score":0.5}]}`, "requires index"},
		{"missing score", `{"results":[{"index":0}]}`, "relevance_score"},
		{"negative index", `{"results":[{"index":-1,"relevance_score":0.5}]}`, "out of range"},
		{"large index", `{"results":[{"index":2,"relevance_score":0.5}]}`, "out of range"},
		{"duplicate index", `{"results":[{"index":0,"relevance_score":0.5},{"index":0,"relevance_score":0.2}]}`, "duplicate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newGPUStackServer(t, "/v1/rerank", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) { io.WriteString(w, tc.body) })
			defer srv.Close()
			model, key := "reranker", "test-key"
			_, err := configuredGPUStackForTest(t, srv.URL).Rerank(t.Context(), &model, RerankRequest{Documents: []string{"a", "b"}}, &APIConfig{ApiKey: &key}, nil, nil)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("error=%v, want %q", err, tc.message)
			}
		})
	}
}

func TestGPUStackRerankValidation(t *testing.T) {
	model, blank := "reranker", " "
	for _, tc := range []struct {
		name      string
		model     *string
		documents []string
		suffix    string
		message   string
	}{
		{"empty documents", &model, nil, "v1/rerank", ""},
		{"missing model", nil, []string{"a"}, "v1/rerank", "model name"},
		{"blank model", &blank, []string{"a"}, "v1/rerank", "model name"},
		{"missing suffix", &model, []string{"a"}, "", "URL suffix"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			driver := NewGPUStackModel(nil, URLSuffix{Rerank: tc.suffix})
			result, err := driver.Rerank(t.Context(), tc.model, RerankRequest{Documents: tc.documents}, nil, nil, nil)
			if tc.message == "" {
				if err != nil || result == nil || len(result.Data) != 0 {
					t.Fatalf("result=%v, error=%v", result, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("error=%v, want %s", err, tc.message)
			}
		})
	}
}

func TestGPUStackTranscribeAudio(t *testing.T) {
	withSSRFBypass(t)
	path := filepath.Join(t.TempDir(), "sample.wav")
	if err := os.WriteFile(path, []byte("audio-data"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"json", "stream-json", "stream-sse"} {
		for _, authenticated := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/auth=%t", mode, authenticated), func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost || r.URL.Path != "/v1/audio/transcriptions" {
						t.Errorf("request=%s %s", r.Method, r.URL.Path)
					}
					wantAuth := ""
					if authenticated {
						wantAuth = "Bearer test-key"
					}
					if r.Header.Get("Authorization") != wantAuth {
						t.Errorf("auth=%q", r.Header.Get("Authorization"))
					}
					if err := r.ParseMultipartForm(1 << 20); err != nil {
						t.Error(err)
						return
					}
					defer r.MultipartForm.RemoveAll()
					file, header, err := r.FormFile("file")
					if err != nil {
						t.Error(err)
						return
					}
					defer file.Close()
					data, err := io.ReadAll(file)
					if err != nil || string(data) != "audio-data" || header.Filename != "sample.wav" {
						t.Errorf("audio=%q, filename=%s, error=%v", data, header.Filename, err)
					}
					if r.FormValue("model") != "whisper" || r.FormValue("language") != "zh" || r.FormValue("response_format") != "json" {
						t.Errorf("form=%v", r.Form)
					}
					if mode != "json" && r.FormValue("stream") != "true" {
						t.Error("stream field missing")
					}
					if mode == "stream-sse" {
						w.Header().Set("Content-Type", "text/event-stream")
						io.WriteString(w, "data: {\"type\":\"transcript.text.delta\",\"delta\":\"你好\"}\n\ndata: {\"type\":\"transcript.text.done\",\"text\":\"你好\"}\n\n")
					} else {
						w.Header().Set("Content-Type", "application/json")
						io.WriteString(w, `{"text":"你好"}`)
					}
				}))
				defer srv.Close()
				var apiConfig *APIConfig
				if authenticated {
					key := "test-key"
					apiConfig = &APIConfig{ApiKey: &key}
				}
				driver := configuredGPUStackForTest(t, srv.URL+"/v1/")
				model := "whisper"
				config := &ASRConfig{Params: map[string]interface{}{"language": "zh", "response_format": "json"}}
				if mode == "json" {
					response, err := driver.TranscribeAudio(t.Context(), &model, &path, apiConfig, config, nil)
					if err != nil {
						t.Fatal(err)
					}
					if response.Text != "你好" {
						t.Fatalf("text=%q", response.Text)
					}
				} else {
					var chunks []string
					err := driver.TranscribeAudioWithSender(t.Context(), &model, &path, apiConfig, config, nil, func(content, _ *string) error {
						if content != nil {
							chunks = append(chunks, *content)
						}
						return nil
					})
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(chunks, []string{"你好", "[DONE]"}) {
						t.Fatalf("chunks=%v", chunks)
					}
				}
			})
		}
	}
}

func TestGPUStackAudioSpeech(t *testing.T) {
	withSSRFBypass(t)
	for _, tc := range []struct {
		name   string
		config *TTSConfig
		format string
	}{
		{"backend default", nil, ""},
		{"format only", &TTSConfig{Format: "mp3"}, "mp3"},
		{"custom voice", &TTSConfig{Format: "wav", Params: map[string]interface{}{"voice": "Vivian", "speed": 1.2}}, "wav"},
		{"alloy", &TTSConfig{Params: map[string]interface{}{"voice": "alloy"}}, ""},
		{"named voice", &TTSConfig{Params: map[string]interface{}{"voice": "Chinese Female"}}, ""},
		{"voice id", &TTSConfig{Params: map[string]interface{}{"voice": map[string]interface{}{"id": "custom-voice"}}}, ""},
	} {
		for _, stream := range []bool{false, true} {
			for _, authenticated := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/stream=%t/auth=%t", tc.name, stream, authenticated), func(t *testing.T) {
					before, _ := json.Marshal(tc.config)
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodPost || r.URL.Path != "/v1/audio/speech" {
							t.Errorf("request=%s %s", r.Method, r.URL.Path)
						}
						wantAuth := ""
						if authenticated {
							wantAuth = "Bearer test-key"
						}
						if r.Header.Get("Authorization") != wantAuth {
							t.Errorf("auth=%q", r.Header.Get("Authorization"))
						}
						var body map[string]interface{}
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
							return
						}
						if body["model"] != "tts-model" || body["input"] != "你好" {
							t.Errorf("body=%v", body)
						}
						var wantVoice interface{}
						wantVoicePresent := false
						if tc.config != nil {
							wantVoice, wantVoicePresent = tc.config.Params["voice"]
						}
						voice, present := body["voice"]
						if present != wantVoicePresent || !reflect.DeepEqual(voice, wantVoice) {
							t.Errorf("voice=%v (present=%t), want %v (present=%t)", voice, present, wantVoice, wantVoicePresent)
						}
						if tc.config != nil && tc.config.Params["speed"] != nil && body["speed"] != tc.config.Params["speed"] {
							t.Errorf("speed=%v", body["speed"])
						}
						if tc.format != "" && body["response_format"] != tc.format {
							t.Errorf("audio settings=%v", body)
						}
						if stream && body["stream_format"] != "audio" {
							t.Errorf("stream_format=%v", body["stream_format"])
						}
						w.Header().Set("Content-Type", "audio/wav")
						io.WriteString(w, "audio-\x00bytes")
					}))
					defer srv.Close()
					var apiConfig *APIConfig
					if authenticated {
						key := "test-key"
						apiConfig = &APIConfig{ApiKey: &key}
					}
					driver := configuredGPUStackForTest(t, srv.URL+"/v1")
					model, input := "tts-model", "你好"
					var audio string
					if stream {
						err := driver.AudioSpeechWithSender(t.Context(), &model, &input, apiConfig, tc.config, nil, func(content, _ *string) error {
							if content != nil {
								audio += *content
							}
							return nil
						})
						if err != nil {
							t.Fatal(err)
						}
					} else {
						response, err := driver.AudioSpeech(t.Context(), &model, &input, apiConfig, tc.config, nil)
						if err != nil {
							t.Fatal(err)
						}
						audio = string(response.Audio)
					}
					if audio != "audio-\x00bytes" {
						t.Fatalf("audio=%q", audio)
					}
					after, _ := json.Marshal(tc.config)
					if string(before) != string(after) {
						t.Fatalf("caller config mutated: %s -> %s", before, after)
					}
				})
			}
		}
	}
}

func TestGPUStackInferenceErrors(t *testing.T) {
	withSSRFBypass(t)
	path := filepath.Join(t.TempDir(), "sample.wav")
	if err := os.WriteFile(path, []byte("audio-data"), 0600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("unexpected authorization")
		}
		http.Error(w, "backend unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	driver := configuredGPUStackForTest(t, srv.URL)
	model, text := "model", "text"
	sender := func(*string, *string) error { t.Error("sender called for failed response"); return nil }
	for name, call := range map[string]func() error{
		"rerank": func() error {
			_, err := driver.Rerank(t.Context(), &model, RerankRequest{Documents: []string{"a"}}, nil, nil, nil)
			return err
		},
		"asr": func() error { _, err := driver.TranscribeAudio(t.Context(), &model, &path, nil, nil, nil); return err },
		"asr stream": func() error {
			return driver.TranscribeAudioWithSender(t.Context(), &model, &path, nil, nil, nil, sender)
		},
		"tts":        func() error { _, err := driver.AudioSpeech(t.Context(), &model, &text, nil, nil, nil); return err },
		"tts stream": func() error { return driver.AudioSpeechWithSender(t.Context(), &model, &text, nil, nil, nil, sender) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil || !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "backend unavailable") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestGPUStackAudioSenderErrors(t *testing.T) {
	withSSRFBypass(t)
	path := filepath.Join(t.TempDir(), "sample.wav")
	if err := os.WriteFile(path, []byte("audio-data"), 0600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/transcriptions") {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"text":"hello"}`)
		} else {
			io.WriteString(w, "audio")
		}
	}))
	defer srv.Close()
	driver := configuredGPUStackForTest(t, srv.URL)
	model, text := "model", "text"
	want := errors.New("receiver stopped")
	sender := func(*string, *string) error { return want }
	for _, err := range []error{
		driver.TranscribeAudioWithSender(t.Context(), &model, &path, nil, nil, nil, sender),
		driver.AudioSpeechWithSender(t.Context(), &model, &text, nil, nil, nil, sender),
	} {
		if !errors.Is(err, want) {
			t.Errorf("error=%v, want %v", err, want)
		}
	}
}

func TestGPUStackAudioSpeechSurfacesVoiceValidation(t *testing.T) {
	withSSRFBypass(t)
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if body["voice"] != "unsupported-speaker" {
					t.Errorf("voice=%v", body["voice"])
				}
				http.Error(w, "voice is not supported by this model", http.StatusBadRequest)
			}))
			defer srv.Close()
			driver := configuredGPUStackForTest(t, srv.URL)
			model, text := "tts-model", "hello"
			config := &TTSConfig{Params: map[string]interface{}{"voice": "unsupported-speaker"}}
			var err error
			if stream {
				err = driver.AudioSpeechWithSender(t.Context(), &model, &text, nil, config, nil, func(*string, *string) error {
					t.Error("sender called for rejected voice")
					return nil
				})
			} else {
				_, err = driver.AudioSpeech(t.Context(), &model, &text, nil, config, nil)
			}
			if err == nil || !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "voice is not supported by this model") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
