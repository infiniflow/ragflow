package component

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudwego/eino/schema"

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
	chatModel := models.NewEinoChatModel(
		models.NewChatModel(driver, &modelName, &models.APIConfig{ApiKey: &apiKey}),
		nil,
	)
	response, err := chatModel.Generate(context.Background(), []*schema.Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if response.Content != "ok" {
		t.Errorf("content = %q, want ok", response.Content)
	}
	if got := <-requestPath; got != "/compatible-mode/v1/chat/completions" {
		t.Errorf("request path = %q, want /compatible-mode/v1/chat/completions", got)
	}
}
