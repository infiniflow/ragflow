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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ragflow/internal/channels/core"
)

func TestNewLineChannelFromConfigRequiresCredentials(t *testing.T) {
	if _, err := newLineChannelFromConfig("account-1", map[string]any{"channel_secret": "secret"}); err == nil {
		t.Fatal("newLineChannelFromConfig succeeded without channel_access_token")
	}
}

func TestNewLineChannelFromConfigNormalizesConfig(t *testing.T) {
	ch, err := newLineChannelFromConfig("account-1", map[string]any{
		"channel_secret":       "secret-1",
		"channel_access_token": "token-1",
		"webhook_host":         "127.0.0.1",
		"webhook_port":         "3011",
		"timeout_secs":         "7",
	})
	if err != nil {
		t.Fatalf("newLineChannelFromConfig returned error: %v", err)
	}
	if ch.account.ChannelSecret != "secret-1" {
		t.Fatalf("channel secret = %q, want secret-1", ch.account.ChannelSecret)
	}
	if ch.account.ChannelAccessToken != "token-1" {
		t.Fatalf("channel access token = %q, want token-1", ch.account.ChannelAccessToken)
	}
	if ch.account.WebhookHost != "127.0.0.1" {
		t.Fatalf("webhook host = %q, want 127.0.0.1", ch.account.WebhookHost)
	}
	if ch.account.WebhookPort != 3011 {
		t.Fatalf("webhook port = %d, want 3011", ch.account.WebhookPort)
	}
	if ch.account.Timeout != 7*time.Second {
		t.Fatalf("timeout = %s, want 7s", ch.account.Timeout)
	}
}

func TestLineWebhookRejectsBadSignature(t *testing.T) {
	ch := newLineChannel(lineAccount{AccountID: "account-1", ChannelSecret: "secret", ChannelAccessToken: "token"})
	server := &lineWebhookServer{channels: map[string]*lineChannel{"account-1": ch}}
	req := httptest.NewRequest(http.MethodPost, "/line/account-1/webhook", strings.NewReader(`{"events":[]}`))
	req.Header.Set("X-Line-Signature", "bad")
	rec := httptest.NewRecorder()

	server.handleRequest(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestLineWebhookDispatchesTextMessage(t *testing.T) {
	body := `{"events":[{"type":"message","replyToken":"reply-1","source":{"type":"user","userId":"user-1"},"message":{"id":"message-1","type":"text","text":"hello"}}]}`
	ch := newLineChannel(lineAccount{AccountID: "account-1", ChannelSecret: "secret", ChannelAccessToken: "token"})
	server := &lineWebhookServer{channels: map[string]*lineChannel{"account-1": ch}}

	var got core.IncomingMessage
	handled := make(chan struct{}, 1)
	ch.SetMessageHandler(func(ctx context.Context, msg core.IncomingMessage) error {
		got = msg
		handled <- struct{}{}
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/line/account-1/webhook", strings.NewReader(body))
	req.Header.Set("X-Line-Signature", lineSignature("secret", body))
	rec := httptest.NewRecorder()

	server.handleRequest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	select {
	case <-handled:
	case <-time.After(time.Second):
		t.Fatal("message handler was not invoked")
	}
	if got.Channel != "line" || got.AccountID != "account-1" || got.ChatID != "user-1" || got.ChatType != "p2p" || got.MessageID != "message-1" || got.Text != "hello" {
		t.Fatalf("incoming message = %#v", got)
	}
	if token := ch.takeReplyToken("message-1"); token != "reply-1" {
		t.Fatalf("reply token = %q, want reply-1", token)
	}
	if token := ch.takeReplyToken("message-1"); token != "" {
		t.Fatalf("reply token was not single-use: %q", token)
	}
}

func TestLineEnqueueDoesNotMarkDroppedMessageSeen(t *testing.T) {
	ch := newLineChannel(lineAccount{AccountID: "account-1", ChannelSecret: "secret", ChannelAccessToken: "token"})
	worker := &lineWorker{queue: make(chan core.IncomingMessage, 1)}
	worker.queue <- core.IncomingMessage{MessageID: "queued"}
	ch.workers["chat-1"] = worker

	ok := ch.enqueueIncoming(core.IncomingMessage{
		ChatID:    "chat-1",
		MessageID: "dropped",
	})

	if ok {
		t.Fatal("enqueueIncoming succeeded for a full queue")
	}
	if _, seen := ch.seen["dropped"]; seen {
		t.Fatal("dropped message was marked seen")
	}
}

func lineSignature(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(body))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
