package component

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ragflow/internal/entity/models"
)

func TestNewChatModelDriverPreservesProviderChatSuffix(t *testing.T) {
	if err := models.InitProviderManager("../../../conf/models"); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	requestPath := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	driver, err := newChatModelDriver("Tongyi-Qianwen", server.URL)
	if err != nil {
		t.Fatalf("newChatModelDriver: %v", err)
	}
	apiKey := "test-key"
	modelName := "qwen-flash"
	resp, err := driver.ChatWithMessages(modelName, []models.Message{{Role: "user", Content: "hi"}}, &models.APIConfig{ApiKey: &apiKey}, &models.ChatConfig{}, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "ok" {
		t.Errorf("answer = %v, want ok", resp.Answer)
	}
	if got := <-requestPath; got != "/compatible-mode/v1/chat/completions" {
		t.Errorf("request path = %q, want /compatible-mode/v1/chat/completions", got)
	}
}
