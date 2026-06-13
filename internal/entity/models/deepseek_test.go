package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newDeepSeekForTest(baseURL string) *DeepSeekModel {
	return NewDeepSeekModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:    "chat/completions",
			Models:  "models",
			Balance: "user/balance",
		},
	)
}

func TestDeepSeekStreamDefaultsReasoningEffortWhenThinkingEnabled(t *testing.T) {
	var seen map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	}))
	defer srv.Close()

	apiKey := "test-key"
	thinking := true
	err := newDeepSeekForTest(srv.URL).ChatStreamlyWithSender(
		"deepseek-v4-pro",
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
	if seen["reasoning_effort"] != "high" {
		t.Errorf("reasoning_effort=%v want high", seen["reasoning_effort"])
	}
}
