package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// nvidiaChatCompletionBody returns a minimal OpenAI-compatible chat
// completion response accepted by HandleNonStreamingResponse.
func nvidiaChatCompletionBody() map[string]any {
	return map[string]any{
		"id":    "chatcmpl-test",
		"model": "nvidia-test",
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "hello",
				},
				"finish_reason": "stop",
			},
		},
	}
}

func newNvidiaSpecialModelForTest(baseURL string, special []URLSuffixSpecial) *NvidiaModel {
	return NewNvidiaModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:      "chat/completions",
			Embedding: "embeddings",
			Rerank:    "ranking",
			Models:    "models",
			Special:   special,
		},
	)
}

// markHostedAs pretends the httptest server is the NVIDIA hosted API so
// the special endpoint override is honored by usesHostedCatalog.
func markHostedAs(driver *NvidiaModel, serverURL string) {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		panic(err)
	}
	driver.hostedAPIHost = parsed.Hostname()
}

func TestNvidiaSpecialChatUsesSpecialURL(t *testing.T) {
	withSSRFBypass(t)
	const specialPath = "/v1/meta/llama-3.2-11b-vision-instruct"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
			return
		}
		if r.URL.Path != specialPath {
			t.Errorf("path = %s, want %s", r.URL.Path, specialPath)
			return
		}
		_ = json.NewEncoder(w).Encode(nvidiaChatCompletionBody())
	}))
	defer server.Close()

	driver := newNvidiaSpecialModelForTest(server.URL, []URLSuffixSpecial{
		{Name: "meta/llama-3.2-11b-vision-instruct", URL: server.URL + specialPath},
	})
	markHostedAs(driver, server.URL)

	apiKey := "test-key"
	resp, err := driver.ChatWithMessages(context.Background(), "meta/llama-3.2-11b-vision-instruct",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages() error = %v", err)
	}
	if resp == nil || resp.Answer == nil || *resp.Answer != "hello" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestNvidiaSpecialChatFallbackDefaultSuffix(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s, want /v1/chat/completions", r.URL.Path)
			return
		}
		_ = json.NewEncoder(w).Encode(nvidiaChatCompletionBody())
	}))
	defer server.Close()

	// No special entries: the model must be assembled from the default
	// base URL and the chat suffix.
	driver := newNvidiaSpecialModelForTest(server.URL+"/v1", nil)

	apiKey := "test-key"
	resp, err := driver.ChatWithMessages(context.Background(), "meta/llama-3.3-70b-instruct",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages() error = %v", err)
	}
	if resp == nil || resp.Answer == nil || *resp.Answer != "hello" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestNvidiaSpecialIgnoredForNonHostedBaseURL(t *testing.T) {
	withSSRFBypass(t)
	// hostedAPIHost stays at the real integrate.api.nvidia.com, so a
	// self-hosted base URL must not trigger the special endpoint even
	// when the model has an entry.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s, want /v1/chat/completions", r.URL.Path)
			return
		}
		_ = json.NewEncoder(w).Encode(nvidiaChatCompletionBody())
	}))
	defer server.Close()

	driver := newNvidiaSpecialModelForTest(server.URL+"/v1", []URLSuffixSpecial{
		{Name: "meta/llama-3.2-11b-vision-instruct", URL: server.URL + "/v1/should-not-be-called"},
	})

	apiKey := "test-key"
	_, err := driver.ChatWithMessages(context.Background(), "meta/llama-3.2-11b-vision-instruct",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages() error = %v", err)
	}
}

func TestNvidiaSpecialURLCaseInsensitive(t *testing.T) {
	withSSRFBypass(t)
	driver := newNvidiaSpecialModelForTest("https://integrate.api.nvidia.com/v1", []URLSuffixSpecial{
		{Name: "META/LLAMA-3.2-11B-VISION-INSTRUCT", URL: "https://integrate.api.nvidia.com/v1/meta/llama-3.2-11b-vision-instruct"},
	})

	got := driver.specialURL("meta/llama-3.2-11b-vision-instruct")
	want := "https://integrate.api.nvidia.com/v1/meta/llama-3.2-11b-vision-instruct"
	if got != want {
		t.Fatalf("specialURL() = %q, want %q", got, want)
	}
}

func TestBuildNvidiaSpecialURLsSkipsEmptyEntries(t *testing.T) {
	driver := newNvidiaSpecialModelForTest("https://integrate.api.nvidia.com/v1", []URLSuffixSpecial{
		{Name: "", URL: "https://integrate.api.nvidia.com/v1/empty-name"},
		{Name: "meta/llama-3.2-90b-vision-instruct", URL: "  "},
		{Name: "meta/llama-3.2-11b-vision-instruct", URL: "https://integrate.api.nvidia.com/v1/meta/llama-3.2-11b-vision-instruct"},
	})

	if got := driver.specialURL("meta/llama-3.2-11b-vision-instruct"); got == "" {
		t.Fatalf("expected valid special URL to be indexed")
	}
	if got := driver.specialURL("meta/llama-3.2-90b-vision-instruct"); got != "" {
		t.Fatalf("specialURL() = %q, want empty for blank URL", got)
	}
	if got := driver.specialURL(""); got != "" {
		t.Fatalf("specialURL() = %q, want empty for blank name", got)
	}
}
