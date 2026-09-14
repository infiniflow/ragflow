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

package channels

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"

	"ragflow/internal/channels/core"
)

func TestNewDingTalkChannelFromConfigRequiresCredentials(t *testing.T) {
	if _, err := newDingTalkChannelFromConfig("account-1", map[string]any{"client_id": "client-1"}); err == nil {
		t.Fatal("newDingTalkChannelFromConfig succeeded without client_secret")
	}
}

func TestNewDingTalkChannelFromConfigNormalizesConfig(t *testing.T) {
	ch, err := newDingTalkChannelFromConfig("account-1", map[string]any{
		"clientId":     "client-1",
		"clientSecret": "secret-1",
		"api_base_url": "https://dingtalk.example",
		"timeout_secs": "7",
	})
	if err != nil {
		t.Fatalf("newDingTalkChannelFromConfig returned error: %v", err)
	}
	if ch.account.ClientID != "client-1" {
		t.Fatalf("client id = %q, want client-1", ch.account.ClientID)
	}
	if ch.account.ClientSecret != "secret-1" {
		t.Fatalf("client secret = %q, want secret-1", ch.account.ClientSecret)
	}
	if ch.account.APIBaseURL != "https://dingtalk.example" {
		t.Fatalf("api base url = %q, want https://dingtalk.example", ch.account.APIBaseURL)
	}
	if ch.account.Timeout != 7*time.Second {
		t.Fatalf("timeout = %s, want 7s", ch.account.Timeout)
	}
}

func TestDingTalkExtractText(t *testing.T) {
	tests := []struct {
		name string
		data map[string]any
		want string
	}{
		{name: "text object", data: map[string]any{"text": map[string]any{"content": "hello"}}, want: "hello"},
		{name: "text string", data: map[string]any{"text": "hello"}, want: "hello"},
		{name: "content object text", data: map[string]any{"content": map[string]any{"text": "hello"}}, want: "hello"},
		{name: "content object recognition", data: map[string]any{"content": map[string]any{"recognition": "hello"}}, want: "hello"},
		{name: "content string", data: map[string]any{"content": "hello"}, want: "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dingTalkExtractText(tt.data); got != tt.want {
				t.Fatalf("dingTalkExtractText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDingTalkBotDedupKey(t *testing.T) {
	if got := dingTalkBotDedupKey(&chatbot.BotCallbackDataModel{MsgId: "msg-1"}, "hello"); got != "msg:msg-1" {
		t.Fatalf("dedup key = %q, want msg:msg-1", got)
	}

	got := dingTalkBotDedupKey(&chatbot.BotCallbackDataModel{
		ConversationId: "conv-1",
		SenderId:       "sender-1",
		CreateAt:       1000,
	}, "hello")
	if got != "fallback:conv-1:sender-1:1000:aaf4c61ddcc5e8a2" {
		t.Fatalf("fallback dedup key = %q", got)
	}
}

func TestDingTalkNormalizeBotMessage(t *testing.T) {
	ch := newDingTalkChannel(dingTalkAccount{AccountID: "account-1", ClientID: "client-1", ClientSecret: "secret-1"})

	incoming, dedupKey, ok := ch.normalizeBotMessage(&chatbot.BotCallbackDataModel{
		ConversationId:   "chat-1",
		ConversationType: "2",
		MsgId:            "msg-1",
		SenderId:         "sender-1",
		SessionWebhook:   "https://webhook.example",
		Text:             chatbot.BotCallbackDataTextModel{Content: "hello"},
	})
	if !ok {
		t.Fatal("normalizeBotMessage failed")
	}
	if incoming.Channel != "dingtalk" || incoming.AccountID != "account-1" || incoming.ChatID != "chat-1" || incoming.MessageID != "msg-1" || incoming.SenderID != "sender-1" || incoming.Text != "hello" {
		t.Fatalf("unexpected incoming message: %+v", incoming)
	}
	if dedupKey != "msg:msg-1" {
		t.Fatalf("dedup key = %q, want msg:msg-1", dedupKey)
	}
	if ch.sessionWebhooks["chat-1"] != "https://webhook.example" {
		t.Fatal("sessionWebhook was not cached")
	}
}

func TestDingTalkSendUsesCachedSessionWebhook(t *testing.T) {
	requestBody := map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ch := newDingTalkChannel(dingTalkAccount{AccountID: "account-1", ClientID: "client-1", ClientSecret: "secret-1"})
	ch.sessionWebhooks["chat-1"] = server.URL

	err := ch.Send(context.Background(), core.OutgoingMessage{ChatID: "chat-1", Text: "hello"})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if requestBody["msgtype"] != "markdown" {
		t.Fatalf("msgtype = %v, want markdown", requestBody["msgtype"])
	}
}

func TestDingTalkEnqueueFailureCancelsInflight(t *testing.T) {
	ch := newDingTalkChannel(dingTalkAccount{AccountID: "account-1", ClientID: "client-1", ClientSecret: "secret-1"})
	worker := &dingTalkWorker{queue: make(chan dingTalkQueuedMessage, 1)}
	worker.queue <- dingTalkQueuedMessage{incoming: core.IncomingMessage{MessageID: "queued"}}
	ch.workers["chat-1"] = worker
	ch.inflight["msg:drop"] = time.Now()

	ok := ch.enqueueIncoming(context.Background(), dingTalkQueuedMessage{
		incoming: core.IncomingMessage{ChatID: "chat-1", MessageID: "drop"},
		dedupKey: "msg:drop",
	})
	if ok {
		t.Fatal("enqueueIncoming succeeded for a full queue")
	}
	ch.cancelInflight("msg:drop")
	if _, exists := ch.inflight["msg:drop"]; exists {
		t.Fatal("inflight key remains after enqueue failure")
	}
}
