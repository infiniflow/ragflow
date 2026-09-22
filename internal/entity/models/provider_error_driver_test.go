//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package models

import (
	"context"
	goerrors "errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
)

type fakeChatDriver struct {
	ModelDriver
	name      string
	chatErr   error
	streamErr error
}

func (f *fakeChatDriver) Name() string { return f.name }

func (f *fakeChatDriver) ChatWithMessages(context.Context, string, []Message, *APIConfig, *ChatConfig, *common.ModelUsage) (*ChatResponse, error) {
	return nil, f.chatErr
}

func (f *fakeChatDriver) ChatStreamlyWithSender(context.Context, string, []Message, *APIConfig, *ChatConfig, *common.ModelUsage, func(*string, *string) error) error {
	return f.streamErr
}

func (f *fakeChatDriver) NewInstance(map[string]string) ModelDriver {
	return &fakeChatDriver{name: f.name, chatErr: f.chatErr}
}

func TestWrapProviderChatErrors_TypesChatFailures(t *testing.T) {
	raw := goerrors.New("API request failed with status 429: slow down")
	d := WrapProviderChatErrors(&fakeChatDriver{name: "deepseek", chatErr: raw})

	_, err := d.ChatWithMessages(context.Background(), "deepseek-chat", nil, nil, nil, nil)
	le, ok := common.AsLLMError(err)
	if !ok {
		t.Fatalf("chat error not typed: %v", err)
	}
	if le.Kind != common.LLMErrorProvider || le.Provider != "deepseek" || le.Model != "deepseek-chat" || le.StatusCode != 429 {
		t.Errorf("attribution = %+v, want provider=deepseek model=deepseek-chat status=429 kind=provider", le)
	}
	if !goerrors.Is(err, raw) {
		t.Errorf("wrapped chain must reach the raw provider error: %v", err)
	}
}

func TestWrapProviderChatErrors_TypesStreamFailures(t *testing.T) {
	d := WrapProviderChatErrors(&fakeChatDriver{name: "openai", streamErr: goerrors.New("API request failed with status 401: bad key")})
	err := d.ChatStreamlyWithSender(context.Background(), "gpt", nil, nil, nil, nil, nil)
	le, ok := common.AsLLMError(err)
	if !ok || le.StatusCode != 401 || le.Model != "gpt" {
		t.Fatalf("stream error not typed with attribution: %v (%+v)", err, le)
	}
}

func TestWrapProviderChatErrors_PassesThroughNilAndContextErrors(t *testing.T) {
	d := WrapProviderChatErrors(&fakeChatDriver{name: "p"})
	if _, err := d.ChatWithMessages(context.Background(), "m", nil, nil, nil, nil); err != nil {
		t.Fatalf("success must stay nil, got %v", err)
	}
	for _, raw := range []error{
		context.Canceled,
		fmt.Errorf("http round trip: %w", context.DeadlineExceeded),
	} {
		d := WrapProviderChatErrors(&fakeChatDriver{name: "p", chatErr: raw})
		_, err := d.ChatWithMessages(context.Background(), "m", nil, nil, nil, nil)
		if _, typed := common.AsLLMError(err); typed {
			t.Errorf("caller control-flow error must stay untyped: %v", err)
		}
		if !goerrors.Is(err, goerrors.Unwrap(raw)) && !goerrors.Is(err, raw) {
			t.Errorf("pass-through must keep the original chain reachable: %v", err)
		}
	}
}

func TestWrapProviderChatErrors_IdempotentAndUnderlying(t *testing.T) {
	ollama := NewOllamaModel(nil, URLSuffix{})
	once := WrapProviderChatErrors(ollama)
	if WrapProviderChatErrors(once) != once {
		t.Error("double wrap must be a no-op")
	}
	if Underlying(once) != ModelDriver(ollama) {
		t.Error("Underlying must return the concrete wrapped driver")
	}
	// Callers that type-assert on the concrete implementation (e.g. the
	// vision invoker's Ollama check) see through the wrapper.
	if _, ok := Underlying(once).(*OllamaModel); !ok {
		t.Errorf("underlying type = %T, want *OllamaModel", Underlying(once))
	}
}

func TestWrapProviderChatErrors_NewInstanceStaysTyped(t *testing.T) {
	raw := goerrors.New("API request failed with status 402: quota")
	d := WrapProviderChatErrors(&fakeChatDriver{name: "p", chatErr: raw})
	derived := d.NewInstance(map[string]string{"default": "https://example.invalid"})
	_, err := derived.ChatWithMessages(context.Background(), "m", nil, nil, nil, nil)
	if _, ok := common.AsLLMError(err); !ok {
		t.Errorf("derived copy lost typing: %v", err)
	}
}

// TestRegisteredProviderChatErrorIsTyped exercises the full production
// construction path — InitProviderManager registration → GetPreconfiguredDriver
// → chat against a server that rejects the call — so the classification is
// proven on real drivers, not only via hand-built stub errors in component
// tests.
func TestRegisteredProviderChatErrorIsTyped(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":{"message":"Insufficient Balance"}}`))
	}))
	defer server.Close()

	dir, restore := setupProviderTestDir(t, "deepseek.json")
	defer restore()
	saved := providerManager
	defer func() { providerManager = saved }()
	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	driver, err := GetPreconfiguredDriver("deepseek", server.URL)
	if err != nil {
		t.Fatalf("GetPreconfiguredDriver: %v", err)
	}
	apiKey := "sk-test"
	_, err = driver.ChatWithMessages(context.Background(), "deepseek-chat",
		[]Message{{Role: "user", Content: "hi"}}, &APIConfig{ApiKey: &apiKey}, &ChatConfig{}, nil)
	if err == nil {
		t.Fatal("expected chat failure against 402 server")
	}
	le, ok := common.AsLLMError(err)
	if !ok {
		t.Fatalf("registered-driver chat error not typed: %v", err)
	}
	if le.Kind != common.LLMErrorProvider || le.StatusCode != 402 {
		t.Errorf("classification = %+v, want kind=provider status=402", le)
	}
	msg := le.UserMessage()
	if !strings.Contains(msg, "not RAGFlow") || !strings.Contains(msg, "Insufficient Balance") {
		t.Errorf("UserMessage must attribute to the model service and keep the reason: %s", msg)
	}
}
