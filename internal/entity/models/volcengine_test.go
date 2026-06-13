package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newVolcEngineServer(t *testing.T, handler func(t *testing.T, r *http.Request, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(t, r, w)
	}))
}

func newVolcEngineForTest(baseURL string) *VolcEngine {
	return NewVolcEngine(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:      "chat/completions",
			Files:     "files",
			Embedding: "embeddings/multimodal",
			Models:    "/models",
		},
	)
}

func TestVolcEngineConfigDeclaresModelsSuffix(t *testing.T) {
	var provider struct {
		URLSuffix URLSuffix `json:"url_suffix"`
	}

	for _, candidate := range []string{
		filepath.Join("..", "..", "..", "conf", "models", "volcengine.json"),
		filepath.Join("conf", "models", "volcengine.json"),
	} {
		data, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(data, &provider); err != nil {
			t.Fatalf("unmarshal %s: %v", candidate, err)
		}
		if provider.URLSuffix.Models != "models" {
			t.Fatalf("models suffix=%q, want models", provider.URLSuffix.Models)
		}
		return
	}

	t.Fatal("could not locate conf/models/volcengine.json")
}

func TestVolcEngineListModelsHappyPath(t *testing.T) {
	srv := newVolcEngineServer(t, func(t *testing.T, r *http.Request, w http.ResponseWriter) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s", r.Method)
		}
		if r.URL.Path != "/models" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object": "list",
			"data": [
				{"id": "doubao-seed-2-0-pro-260215", "owned_by": "volcengine"},
				{"id": "doubao-embedding-vision-251215"}
			]
		}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	models, err := newVolcEngineForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(models, ",") != "doubao-seed-2-0-pro-260215@volcengine,doubao-embedding-vision-251215" {
		t.Errorf("models=%v", models)
	}
}

func TestVolcEngineListModelsRejectsProviderError(t *testing.T) {
	srv := newVolcEngineServer(t, func(t *testing.T, r *http.Request, w http.ResponseWriter) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newVolcEngineForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "401 Unauthorized") || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("expected provider error with status and body, got %v", err)
	}
}

func TestVolcEngineListModelsRequiresModelsSuffix(t *testing.T) {
	apiKey := "test-key"
	model := NewVolcEngine(map[string]string{"default": "http://unused"}, URLSuffix{})

	_, err := model.ListModels(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "models URL suffix is not configured") {
		t.Fatalf("expected missing models suffix error, got %v", err)
	}
}

func TestVolcEngineStreamDefaultsReasoningEffortWhenThinkingEnabled(t *testing.T) {
	var seen map[string]interface{}
	srv := newVolcEngineServer(t, func(t *testing.T, r *http.Request, w http.ResponseWriter) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		if err := json.Unmarshal(raw, &seen); err != nil {
			t.Errorf("unmarshal request body: %v\nraw=%s", err, string(raw))
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	})
	defer srv.Close()

	apiKey := "test-key"
	thinking := true
	err := newVolcEngineForTest(srv.URL).ChatStreamlyWithSender(
		"doubao-seed-2-0-pro-260215",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Thinking: &thinking},
		func(*string, *string) error { return nil },
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}

	thinkingBody, ok := seen["thinking"].(map[string]interface{})
	if !ok {
		t.Fatalf("thinking body=%v, want object", seen["thinking"])
	}
	if thinkingBody["type"] != "enabled" {
		t.Errorf("thinking.type=%v want enabled", thinkingBody["type"])
	}
	if seen["reasoning_effort"] != "medium" {
		t.Errorf("reasoning_effort=%v want medium", seen["reasoning_effort"])
	}
}

func TestVolcEngineChatDoesNotExpectReasoningContentWhenEffortDisablesThinking(t *testing.T) {
	var seen map[string]interface{}
	srv := newVolcEngineServer(t, func(t *testing.T, r *http.Request, w http.ResponseWriter) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		if err := json.Unmarshal(raw, &seen); err != nil {
			t.Errorf("unmarshal request body: %v\nraw=%s", err, string(raw))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	})
	defer srv.Close()

	apiKey := "test-key"
	thinking := true
	effort := "none"
	resp, err := newVolcEngineForTest(srv.URL).ChatWithMessages(
		"doubao-seed-2-0-pro-260215",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Thinking: &thinking, Effort: &effort},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp == nil || resp.Answer == nil || *resp.Answer != "ok" {
		t.Fatalf("answer=%v, want ok", resp)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "" {
		t.Fatalf("reasonContent=%v, want empty", resp.ReasonContent)
	}

	thinkingBody, ok := seen["thinking"].(map[string]interface{})
	if !ok {
		t.Fatalf("thinking body=%v, want object", seen["thinking"])
	}
	if thinkingBody["type"] != "disabled" {
		t.Errorf("thinking.type=%v want disabled", thinkingBody["type"])
	}
	if seen["reasoning_effort"] != "minimal" {
		t.Errorf("reasoning_effort=%v want minimal", seen["reasoning_effort"])
	}
}
