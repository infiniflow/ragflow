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
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"ragflow/internal/channels/core"
)

type telegramRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f telegramRoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestNewTelegramChannelFromConfigRequiresToken(t *testing.T) {
	if _, err := newTelegramChannelFromConfig("account-1", map[string]any{}); err == nil {
		t.Fatal("newTelegramChannelFromConfig accepted a missing token")
	}
}

func TestTelegramChatType(t *testing.T) {
	tests := map[string]string{
		"private":    "p2p",
		"group":      "group",
		"supergroup": "group",
		"channel":    "channel",
		"":           "unknown",
		"custom":     "custom",
	}
	for input, want := range tests {
		if got := telegramChatType(input); got != want {
			t.Fatalf("telegramChatType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTelegramHandleUpdateMapsCaptionAndIgnoresBots(t *testing.T) {
	ch := newTelegramChannel(telegramAccount{AccountID: "account-1", Token: "token"})
	messages := make(chan core.IncomingMessage, 1)
	ch.SetMessageHandler(func(_ context.Context, msg core.IncomingMessage) error {
		messages <- msg
		return nil
	})

	ch.handleUpdate(context.Background(), telegramUpdate{
		UpdateID: 1,
		Message: &telegramMessage{
			MessageID: 42,
			From:      &telegramUser{ID: 7},
			Chat:      telegramChat{ID: -10, Type: "supergroup"},
			Caption:   "caption text",
		},
	})
	ch.handleUpdate(context.Background(), telegramUpdate{
		UpdateID: 2,
		Message: &telegramMessage{
			MessageID: 43,
			From:      &telegramUser{ID: 8, IsBot: true},
		},
	})

	select {
	case msg := <-messages:
		if msg.ChatID != "-10" || msg.ChatType != "group" || msg.MessageID != "42" || msg.SenderID != "7" || msg.Text != "caption text" {
			t.Fatalf("unexpected incoming message: %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for incoming message")
	}

	select {
	case msg := <-messages:
		t.Fatalf("bot message was dispatched: %+v", msg)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestTelegramHandleUpdateMapsChannelPostWithoutSender(t *testing.T) {
	ch := newTelegramChannel(telegramAccount{AccountID: "account-1", Token: "token"})
	messages := make(chan core.IncomingMessage, 1)
	ch.SetMessageHandler(func(_ context.Context, msg core.IncomingMessage) error {
		messages <- msg
		return nil
	})

	ch.handleUpdate(context.Background(), telegramUpdate{
		ChannelPost: &telegramMessage{
			MessageID: 42,
			Chat:      telegramChat{ID: -100123, Type: "channel"},
			Text:      "channel post",
		},
	})

	select {
	case msg := <-messages:
		if msg.ChatID != "-100123" || msg.ChatType != "channel" || msg.MessageID != "42" || msg.SenderID != "-100123" || msg.Text != "channel post" {
			t.Fatalf("unexpected incoming message: %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for channel post")
	}
}

func TestTelegramCallAPISanitizesTransportError(t *testing.T) {
	const token = "sensitive-bot-token"
	transportErr := &url.Error{
		Op:  "Post",
		URL: "https://api.telegram.org/bot" + token + "/sendMessage",
		Err: errors.New("connection refused"),
	}
	ch := newTelegramChannel(telegramAccount{AccountID: "account-1", Token: token})
	ch.client = &http.Client{Transport: telegramRoundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})}

	err := ch.callAPI(context.Background(), "sendMessage", map[string]any{}, nil)
	if err == nil {
		t.Fatal("callAPI() error = nil")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("callAPI() exposed token in error: %v", err)
	}
	if !strings.Contains(err.Error(), "sendMessage") {
		t.Fatalf("callAPI() error = %q, want method context", err)
	}
	if !errors.Is(err, transportErr) {
		t.Fatalf("callAPI() error does not unwrap transport error: %v", err)
	}
}

func TestTelegramSendUsesReplyParameters(t *testing.T) {
	request := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			t.Errorf("unexpected Telegram API path: %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		request <- payload
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer server.Close()

	ch := newTelegramChannel(telegramAccount{
		AccountID:  "account-1",
		Token:      "token",
		APIBaseURL: server.URL,
	})
	if err := ch.Send(context.Background(), core.OutgoingMessage{
		ChatID:           "-100",
		Text:             "answer",
		ReplyToMessageID: "12",
	}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	payload := <-request
	if payload["chat_id"] != float64(-100) || payload["text"] != "answer" {
		t.Fatalf("unexpected send payload: %+v", payload)
	}
	reply, ok := payload["reply_parameters"].(map[string]any)
	if !ok || reply["message_id"] != float64(12) || reply["allow_sending_without_reply"] != true {
		t.Fatalf("unexpected reply parameters: %+v", payload["reply_parameters"])
	}
}

func TestTelegramSendIgnoresInvalidChatID(t *testing.T) {
	ch := newTelegramChannel(telegramAccount{AccountID: "account-1", Token: "token"})
	if err := ch.Send(context.Background(), core.OutgoingMessage{ChatID: "invalid", Text: "answer"}); err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}
}

func TestTelegramStartAndStopPollsUpdates(t *testing.T) {
	updateReturned := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/deleteWebhook"):
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			if !updateReturned {
				updateReturned = true
				_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":9,"message":{"message_id":3,"from":{"id":4,"is_bot":false},"chat":{"id":5,"type":"private"},"text":"hello"}}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
		default:
			t.Errorf("unexpected Telegram API path: %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer server.Close()

	ch := newTelegramChannel(telegramAccount{
		AccountID:  "account-1",
		Token:      "token",
		APIBaseURL: server.URL,
	})
	message := make(chan core.IncomingMessage, 1)
	ch.SetMessageHandler(func(_ context.Context, msg core.IncomingMessage) error {
		message <- msg
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := ch.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case msg := <-message:
		if msg.Text != "hello" || msg.MessageID != "3" || ch.offset != 10 {
			t.Fatalf("unexpected polled message: %+v, offset=%d", msg, ch.offset)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for polled message")
	}

	if err := ch.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}
