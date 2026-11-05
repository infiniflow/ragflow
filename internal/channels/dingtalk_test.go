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
	ch, err := newDingTalkChannel(dingTalkAccount{AccountID: "account-1", ClientID: "client-1", ClientSecret: "secret-1"})
	if err != nil {
		t.Fatal(err)
	}

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
	allowChannelURLForTest(t, server.URL)

	ch, err := newDingTalkChannel(dingTalkAccount{AccountID: "account-1", ClientID: "client-1", ClientSecret: "secret-1"})
	if err != nil {
		t.Fatal(err)
	}
	ch.sessionWebhooks["chat-1"] = server.URL

	err = ch.Send(context.Background(), core.OutgoingMessage{ChatID: "chat-1", Text: "hello"})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if requestBody["msgtype"] != "markdown" {
		t.Fatalf("msgtype = %v, want markdown", requestBody["msgtype"])
	}
}

func TestDingTalkEnqueueFailureCancelsInflight(t *testing.T) {
	ch, err := newDingTalkChannel(dingTalkAccount{AccountID: "account-1", ClientID: "client-1", ClientSecret: "secret-1"})
	if err != nil {
		t.Fatal(err)
	}
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

func TestDingTalkStreamEnabledNeedsSwitchAndTemplate(t *testing.T) {
	for _, test := range []struct {
		name string
		cfg  map[string]any
		want bool
	}{
		{name: "switch and template", cfg: map[string]any{"stream": true, "card_template_id": "tpl-1"}, want: true},
		{name: "switch without template", cfg: map[string]any{"stream": true}, want: false},
		{name: "template without switch", cfg: map[string]any{"card_template_id": "tpl-1"}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := map[string]any{"client_id": "client-1", "client_secret": "secret-1"}
			for key, value := range test.cfg {
				cfg[key] = value
			}
			ch, err := newDingTalkChannelFromConfig("account-1", cfg)
			if err != nil {
				t.Fatalf("newDingTalkChannelFromConfig() error = %v", err)
			}
			if got := ch.StreamEnabled(); got != test.want {
				t.Fatalf("StreamEnabled() = %t, want %t", got, test.want)
			}
			if ch.account.StreamContentKey != "content" {
				t.Fatalf("stream content key = %q, want content", ch.account.StreamContentKey)
			}
		})
	}
}

func TestDingTalkSendStreamCreatesAndFinishesCard(t *testing.T) {
	type apiCall struct {
		method string
		path   string
		body   map[string]any
	}
	var calls []apiCall
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		calls = append(calls, apiCall{method: r.Method, path: r.URL.Path, body: body})
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.0/oauth2/accessToken":
			_, _ = w.Write([]byte(`{"accessToken":"token-1","expireIn":7200}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		}
	}))
	defer server.Close()
	allowChannelURLForTest(t, server.URL)

	ch, err := newDingTalkChannel(dingTalkAccount{
		AccountID:      "account-1",
		ClientID:       "client-1",
		ClientSecret:   "secret-1",
		APIBaseURL:     server.URL,
		Stream:         true,
		CardTemplateID: "tpl-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	ch.peers["chat-1"] = dingTalkPeer{chatType: "2"}

	text := make(chan string, 2)
	text <- "Hel"
	text <- "Hello"
	close(text)

	if err := ch.SendStream(context.Background(), core.OutgoingMessage{ChatID: "chat-1"}, text); err != nil {
		t.Fatalf("SendStream() error = %v", err)
	}

	var createBody map[string]any
	var streamCalls []map[string]any
	for _, call := range calls {
		switch call.path {
		case "/v1.0/card/instances/createAndDeliver":
			createBody = call.body
		case "/v1.0/card/streaming":
			streamCalls = append(streamCalls, call.body)
		}
	}
	if createBody == nil {
		t.Fatalf("no card create call, calls = %+v", calls)
	}
	if createBody["cardTemplateId"] != "tpl-1" {
		t.Fatalf("cardTemplateId = %v, want tpl-1", createBody["cardTemplateId"])
	}
	if createBody["openSpaceId"] != "dtv1.card//IM_GROUP.chat-1" {
		t.Fatalf("openSpaceId = %v, want the group space", createBody["openSpaceId"])
	}
	cardParamMap := weComMapValue(createBody["cardData"])["cardParamMap"]
	if weComMapValue(cardParamMap)["flowStatus"] != "1" {
		t.Fatalf("cardParamMap = %+v, want flowStatus 1", cardParamMap)
	}
	if _, ok := weComMapValue(cardParamMap)["content"]; !ok {
		t.Fatalf("cardParamMap = %+v, want the streaming content key", cardParamMap)
	}

	if len(streamCalls) == 0 {
		t.Fatal("no streaming call was made")
	}
	outTrackID, _ := createBody["outTrackId"].(string)
	var content string
	for index, call := range streamCalls {
		if call["outTrackId"] != outTrackID {
			t.Fatalf("streaming call %d outTrackId = %v, want %q", index, call["outTrackId"], outTrackID)
		}
		if call["key"] != "content" {
			t.Fatalf("streaming call %d key = %v, want content", index, call["key"])
		}
		if call["isFull"] != false {
			t.Fatalf("streaming call %d isFull = %v, want an append", index, call["isFull"])
		}
		content += weComStringValue(call["content"])
		wantFinal := index == len(streamCalls)-1
		if call["isFinalize"] != wantFinal {
			t.Fatalf("streaming call %d isFinalize = %v, want %t", index, call["isFinalize"], wantFinal)
		}
	}
	if content != "Hello" {
		t.Fatalf("streamed content = %q, want the complete answer", content)
	}
}

func TestDingTalkSendStreamNeedsSenderForDirectChat(t *testing.T) {
	ch, err := newDingTalkChannel(dingTalkAccount{
		AccountID:      "account-1",
		ClientID:       "client-1",
		ClientSecret:   "secret-1",
		Stream:         true,
		CardTemplateID: "tpl-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	ch.peers["chat-1"] = dingTalkPeer{chatType: "1"}

	text := make(chan string, 1)
	text <- "Hello"
	close(text)

	if err := ch.SendStream(context.Background(), core.OutgoingMessage{ChatID: "chat-1"}, text); err == nil {
		t.Fatal("SendStream succeeded without the sender staff id")
	}
}

func TestDingTalkSendStreamMarksCardFailed(t *testing.T) {
	type apiCall struct {
		method string
		path   string
		body   map[string]any
	}
	var calls []apiCall
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		calls = append(calls, apiCall{method: r.Method, path: r.URL.Path, body: body})
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.0/oauth2/accessToken":
			_, _ = w.Write([]byte(`{"accessToken":"token-1","expireIn":7200}`))
		case "/v1.0/card/streaming":
			_, _ = w.Write([]byte(`{"success":false,"code":"InvalidParameter","message":"rejected"}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		}
	}))
	defer server.Close()
	allowChannelURLForTest(t, server.URL)

	ch, err := newDingTalkChannel(dingTalkAccount{
		AccountID:      "account-1",
		ClientID:       "client-1",
		ClientSecret:   "secret-1",
		APIBaseURL:     server.URL,
		Stream:         true,
		CardTemplateID: "tpl-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	ch.peers["chat-1"] = dingTalkPeer{chatType: "2"}

	text := make(chan string, 2)
	text <- "Hello"
	close(text)

	if err := ch.SendStream(context.Background(), core.OutgoingMessage{ChatID: "chat-1"}, text); err == nil {
		t.Fatal("SendStream succeeded although the card update was rejected")
	}

	var createBody, failBody map[string]any
	for _, call := range calls {
		switch call.path {
		case "/v1.0/card/instances/createAndDeliver":
			createBody = call.body
		case "/v1.0/card/instances":
			if call.method == http.MethodPut {
				failBody = call.body
			}
		}
	}
	if createBody == nil {
		t.Fatalf("no card create call, calls = %+v", calls)
	}
	if failBody == nil {
		t.Fatalf("card was not marked failed, calls = %+v", calls)
	}
	if failBody["outTrackId"] != createBody["outTrackId"] {
		t.Fatalf("failed card outTrackId = %v, want %v", failBody["outTrackId"], createBody["outTrackId"])
	}
	paramMap := weComMapValue(weComMapValue(failBody["cardData"])["cardParamMap"])
	if paramMap["flowStatus"] != "5" {
		t.Fatalf("cardParamMap = %+v, want flowStatus 5", paramMap)
	}
	// The first update was rejected, so the card never received text.
	if paramMap["content"] != "" {
		t.Fatalf("failed card content = %v, want the text that actually reached the card", paramMap["content"])
	}
}
