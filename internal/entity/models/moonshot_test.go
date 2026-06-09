package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newMoonshotServer(t *testing.T, handler func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization=Bearer test-key, got %q", got)
			return
		}

		var body map[string]interface{}
		if r.Method == http.MethodPost {
			if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
				t.Errorf("expected Content-Type to start with application/json, got %q", got)
				return
			}
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
				return
			}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("unmarshal: %v\nraw=%s", err, string(raw))
				return
			}
		} else {
			if r.ContentLength > 0 {
				t.Errorf("expected %s request without body, ContentLength=%d", r.Method, r.ContentLength)
				return
			}
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
				return
			}
			if len(raw) != 0 {
				t.Errorf("expected %s request without body, got %q", r.Method, string(raw))
				return
			}
		}

		handler(t, r, body, w)
	}))
}

func newMoonshotForTest(baseURL string) *MoonshotModel {
	return NewMoonshotModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:    "chat/completions",
			Models:  "models",
			Balance: "users/me/balance",
		},
	)
}

func TestMoonshotNewInstancePreservesConfig(t *testing.T) {
	model := NewMoonshotModel(
		map[string]string{"default": "http://old.example"},
		URLSuffix{Chat: "chat", Models: "models", Balance: "balance"},
	)

	instance, ok := model.NewInstance(map[string]string{"default": "http://new.example"}).(*MoonshotModel)
	if !ok {
		t.Fatalf("NewInstance type=%T, want *MoonshotModel", instance)
	}
	if instance.baseModel.BaseURL["default"] != "http://new.example" {
		t.Errorf("BaseURL=%q", instance.baseModel.BaseURL["default"])
	}
	if instance.baseModel.URLSuffix.Chat != "chat" || instance.baseModel.URLSuffix.Models != "models" || instance.baseModel.URLSuffix.Balance != "balance" {
		t.Errorf("URLSuffix=%+v", instance.baseModel.URLSuffix)
	}
	if instance.baseModel.httpClient == nil {
		t.Error("httpClient is nil")
	}
}

func TestMoonshotChatForcesNonStreaming(t *testing.T) {
	srv := newMoonshotServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s, want /chat/completions", r.URL.Path)
		}
		if body["model"] != "kimi-k2.6" {
			t.Errorf("model=%v, want kimi-k2.6", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v, want false", body["stream"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content":           "pong",
					"reasoning_content": "\nthought",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := " test-key "
	stream := true
	thinking := true
	resp, err := newMoonshotForTest(srv.URL).ChatWithMessages(
		" kimi-k2.6 ",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream, Thinking: &thinking},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Errorf("Answer=%v, want pong", resp.Answer)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "thought" {
		t.Errorf("ReasonContent=%v, want thought", resp.ReasonContent)
	}
}

func TestMoonshotStreamForcesStreaming(t *testing.T) {
	srv := newMoonshotServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s, want /chat/completions", r.URL.Path)
		}
		if body["stream"] != true {
			t.Errorf("stream=%v, want true", body["stream"])
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("Accept=%q, want text/event-stream", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, strings.Join([]string{
			`data: {"choices":[{"delta":{"reasoning_content":"thinking"}}]}`,
			`data: {"choices":[{"delta":{"content":"hello"}}]}`,
			`data: [DONE]`,
			``,
		}, "\n"))
	})
	defer srv.Close()

	apiKey := "test-key"
	stream := false
	var content, reasoning []string
	var sawDone bool
	err := newMoonshotForTest(srv.URL).ChatStreamlyWithSender(
		"kimi-k2.6",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		func(answer, reason *string) error {
			if answer != nil {
				if *answer == "[DONE]" {
					sawDone = true
					return nil
				}
				content = append(content, *answer)
			}
			if reason != nil {
				reasoning = append(reasoning, *reason)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if got := strings.Join(content, ""); got != "hello" {
		t.Errorf("content=%q, want hello", got)
	}
	if got := strings.Join(reasoning, ""); got != "thinking" {
		t.Errorf("reasoning=%q, want thinking", got)
	}
	if !sawDone {
		t.Error("expected [DONE] sentinel")
	}
}

func TestMoonshotStreamDoesNotSendDoneAfterScannerError(t *testing.T) {
	srv := newMoonshotServer(t, func(t *testing.T, _ *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if body["stream"] != true {
			t.Errorf("stream=%v, want true", body["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: "+strings.Repeat("x", 1024*1024+1)+"\n")
	})
	defer srv.Close()

	apiKey := "test-key"
	var sawDone bool
	err := newMoonshotForTest(srv.URL).ChatStreamlyWithSender(
		"kimi-k2.6",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		func(answer, _ *string) error {
			if answer != nil && *answer == "[DONE]" {
				sawDone = true
			}
			return nil
		},
	)
	if err == nil {
		t.Fatal("expected scanner error")
	}
	if sawDone {
		t.Fatal("sender received [DONE] after scanner error")
	}
}

func TestMoonshotListModelsUsesBodylessGet(t *testing.T) {
	srv := newMoonshotServer(t, func(t *testing.T, r *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		if r.URL.Path != "/models" {
			t.Errorf("path=%s, want /models", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{
				{"id": "kimi-k2.6"},
				{"id": " moonshot-v1-8k "},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	models, err := newMoonshotForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if got := joinModelNames(models, ","); got != "kimi-k2.6,moonshot-v1-8k" {
		t.Errorf("models=%q", got)
	}
}

func TestMoonshotBalanceUsesBodylessGet(t *testing.T) {
	srv := newMoonshotServer(t, func(t *testing.T, r *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		if r.URL.Path != "/users/me/balance" {
			t.Errorf("path=%s, want /users/me/balance", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code":   0,
			"status": true,
			"data": map[string]float64{
				"available_balance": 49.5,
				"voucher_balance":   46.5,
				"cash_balance":      3,
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	balance, err := newMoonshotForTest(srv.URL).Balance(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if balance["balance"] != 49.5 {
		t.Errorf("balance=%v, want 49.5", balance["balance"])
	}
	if balance["currency"] != "CNY" {
		t.Errorf("currency=%v, want CNY", balance["currency"])
	}
}

func TestMoonshotRejectsMalformedResponses(t *testing.T) {
	apiKey := "test-key"
	tests := []struct {
		name     string
		response map[string]interface{}
		run      func(*MoonshotModel) error
	}{
		{
			name:     "models missing data",
			response: map[string]interface{}{"object": "list"},
			run: func(m *MoonshotModel) error {
				_, err := m.ListModels(&APIConfig{ApiKey: &apiKey})
				return err
			},
		},
		{
			name: "models empty id",
			response: map[string]interface{}{
				"data": []map[string]string{{"id": ""}},
			},
			run: func(m *MoonshotModel) error {
				_, err := m.ListModels(&APIConfig{ApiKey: &apiKey})
				return err
			},
		},
		{
			name: "balance missing available balance",
			response: map[string]interface{}{
				"data": map[string]float64{"cash_balance": 3},
			},
			run: func(m *MoonshotModel) error {
				_, err := m.Balance(&APIConfig{ApiKey: &apiKey})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newMoonshotServer(t, func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
				_ = json.NewEncoder(w).Encode(tt.response)
			})
			defer srv.Close()

			if err := tt.run(newMoonshotForTest(srv.URL)); err == nil {
				t.Fatal("expected malformed response error")
			}
		})
	}
}

func TestMoonshotValidatesInputs(t *testing.T) {
	apiKey := "test-key"
	emptyKey := " "
	send := func(*string, *string) error { return nil }

	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "chat api key",
			run: func() error {
				_, err := newMoonshotForTest("http://unused").ChatWithMessages("kimi-k2.6", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &emptyKey}, nil)
				return err
			},
			want: "api key is required",
		},
		{
			name: "chat model",
			run: func() error {
				_, err := newMoonshotForTest("http://unused").ChatWithMessages(" ", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
				return err
			},
			want: "model name is required",
		},
		{
			name: "stream api key",
			run: func() error {
				return newMoonshotForTest("http://unused").ChatStreamlyWithSender("kimi-k2.6", []Message{{Role: "user", Content: "x"}}, nil, nil, send)
			},
			want: "api key is required",
		},
		{
			name: "stream model",
			run: func() error {
				return newMoonshotForTest("http://unused").ChatStreamlyWithSender(" ", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, send)
			},
			want: "model name is required",
		},
		{
			name: "stream sender",
			run: func() error {
				return newMoonshotForTest("http://unused").ChatStreamlyWithSender("kimi-k2.6", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, nil)
			},
			want: "sender is required",
		},
		{
			name: "models api key",
			run: func() error {
				_, err := newMoonshotForTest("http://unused").ListModels(&APIConfig{})
				return err
			},
			want: "api key is required",
		},
		{
			name: "balance api key",
			run: func() error {
				_, err := newMoonshotForTest("http://unused").Balance(nil)
				return err
			},
			want: "api key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}
