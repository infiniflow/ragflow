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
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"ragflow/internal/channels/core"
)

func TestNewQQBotChannelFromConfig(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		channel, err := newQQBotChannelFromConfig("account-1", map[string]any{
			"app_id":        "app-1",
			"client_secret": "secret-1",
		})
		if err != nil {
			t.Fatalf("newQQBotChannelFromConfig() error = %v", err)
		}
		if channel.account.BaseURL != defaultQQBotBaseURL {
			t.Fatalf("BaseURL = %q, want %q", channel.account.BaseURL, defaultQQBotBaseURL)
		}
		if channel.ChannelID() != "qqbot" || channel.AccountID() != "account-1" {
			t.Fatalf("channel identity = %s:%s, want qqbot:account-1", channel.ChannelID(), channel.AccountID())
		}
	})

	for _, cfg := range []map[string]any{
		{"client_secret": "secret-1"},
		{"app_id": "app-1"},
	} {
		if _, err := newQQBotChannelFromConfig("account-1", cfg); err == nil {
			t.Fatalf("newQQBotChannelFromConfig(%v) succeeded, want credential error", cfg)
		}
	}
}

func TestBuildChannelSupportsQQBot(t *testing.T) {
	channel, err := buildChannel("account-1", desiredChannel{
		channel: "qqbot",
		credential: map[string]any{
			"app_id":        "app-1",
			"client_secret": "secret-1",
		},
	})
	if err != nil {
		t.Fatalf("buildChannel() error = %v", err)
	}
	if channel.ChannelID() != "qqbot" {
		t.Fatalf("ChannelID() = %q, want qqbot", channel.ChannelID())
	}
}

func TestNormalizeQQBotTarget(t *testing.T) {
	tests := []struct {
		chatID   string
		wantType string
		wantID   string
	}{
		{chatID: "user-1", wantType: "c2c", wantID: "user-1"},
		{chatID: "c2c:user-1", wantType: "c2c", wantID: "user-1"},
		{chatID: "qqbot:c2c:user-1", wantType: "c2c", wantID: "user-1"},
		{chatID: "qqbot:group:group-1", wantType: "group", wantID: "group-1"},
		{chatID: "qqbot:dm:guild-1", wantType: "dm", wantID: "guild-1"},
		{chatID: "qqbot:channel:channel-1", wantType: "channel", wantID: "channel-1"},
	}
	for _, tt := range tests {
		t.Run(tt.chatID, func(t *testing.T) {
			gotType, gotID := normalizeQQBotTarget(tt.chatID)
			if gotType != tt.wantType || gotID != tt.wantID {
				t.Fatalf("normalizeQQBotTarget(%q) = (%q, %q), want (%q, %q)", tt.chatID, gotType, gotID, tt.wantType, tt.wantID)
			}
		})
	}
}

func TestQQBotSend(t *testing.T) {
	var requests []struct {
		path          string
		authorization string
		body          map[string]any
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode token request: %v", err)
			}
			if body["appId"] != "app-1" || body["clientSecret"] != "secret-1" {
				t.Errorf("token request body = %v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "token-1"})
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode send request: %v", err)
		}
		requests = append(requests, struct {
			path          string
			authorization string
			body          map[string]any
		}{r.URL.Path, r.Header.Get("Authorization"), body})
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer server.Close()

	channel := newQQBotChannel(qqBotAccount{
		AccountID:    "account-1",
		AppID:        "app-1",
		ClientSecret: "secret-1",
		BaseURL:      server.URL,
		TokenURL:     server.URL + "/token",
	})
	tests := []struct {
		chatID   string
		wantPath string
	}{
		{chatID: "qqbot:c2c:user-1", wantPath: "/v2/users/user-1/messages"},
		{chatID: "qqbot:group:group-1", wantPath: "/v2/groups/group-1/messages"},
		{chatID: "qqbot:dm:guild-1", wantPath: "/dms/guild-1/messages"},
		{chatID: "qqbot:channel:channel-1", wantPath: "/channels/channel-1/messages"},
	}
	for _, tt := range tests {
		if err := channel.Send(context.Background(), core.OutgoingMessage{
			ChatID:           tt.chatID,
			Text:             "answer",
			ReplyToMessageID: "message-1",
		}); err != nil {
			t.Fatalf("Send(%q) error = %v", tt.chatID, err)
		}
		request := requests[len(requests)-1]
		if request.path != tt.wantPath {
			t.Errorf("Send(%q) path = %q, want %q", tt.chatID, request.path, tt.wantPath)
		}
		if request.authorization != "QQBot token-1" {
			t.Errorf("Authorization = %q, want QQBot token-1", request.authorization)
		}
		if request.body["content"] != "answer" || request.body["msg_id"] != "message-1" || request.body["msg_type"] != float64(0) {
			t.Errorf("send body = %v", request.body)
		}
		if sequence, ok := request.body["msg_seq"].(float64); !ok || sequence < 1 || sequence > 65535 {
			t.Errorf("msg_seq = %v, want integer in [1, 65535]", request.body["msg_seq"])
		}
	}
}

func TestQQBotNormalizeIncomingEvent(t *testing.T) {
	channel := newQQBotChannel(qqBotAccount{AccountID: "account-1"})
	tests := []struct {
		name       string
		eventType  string
		data       map[string]any
		wantChatID string
		wantType   string
		wantSender string
	}{
		{
			name:       "c2c",
			eventType:  "C2C_MESSAGE_CREATE",
			data:       map[string]any{"id": "m1", "content": "hello", "author": map[string]any{"user_openid": "user-1"}},
			wantChatID: "qqbot:c2c:user-1",
			wantType:   "p2p",
			wantSender: "user-1",
		},
		{
			name:       "group",
			eventType:  "GROUP_AT_MESSAGE_CREATE",
			data:       map[string]any{"id": "m2", "group_openid": "group-1", "author": map[string]any{"member_openid": "member-1"}},
			wantChatID: "qqbot:group:group-1",
			wantType:   "group",
			wantSender: "member-1",
		},
		{
			name:       "direct message uses guild",
			eventType:  "DIRECT_MESSAGE_CREATE",
			data:       map[string]any{"id": "m3", "guild_id": "guild-1", "author": map[string]any{"id": "user-2"}},
			wantChatID: "qqbot:dm:guild-1",
			wantType:   "dm",
			wantSender: "user-2",
		},
		{
			name:       "channel",
			eventType:  "AT_MESSAGE_CREATE",
			data:       map[string]any{"id": "m4", "channel_id": "channel-1", "author": map[string]any{"id": "user-3"}},
			wantChatID: "qqbot:channel:channel-1",
			wantType:   "channel",
			wantSender: "user-3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := channel.normalizeIncomingEvent(tt.eventType, tt.data)
			if message == nil {
				t.Fatal("normalizeIncomingEvent() = nil")
			}
			if message.ChatID != tt.wantChatID || message.ChatType != tt.wantType || message.SenderID != tt.wantSender {
				t.Fatalf("message = %+v, want chat_id=%q chat_type=%q sender_id=%q", message, tt.wantChatID, tt.wantType, tt.wantSender)
			}
			if message.Channel != "qqbot" || message.AccountID != "account-1" || message.Raw == nil {
				t.Fatalf("message envelope = %+v", message)
			}
		})
	}
}

func TestQQBotGatewayIdentifiesAndDispatches(t *testing.T) {
	upgrader := websocket.Upgrader{}
	identify := make(chan map[string]any, 1)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()
		if err := conn.WriteJSON(map[string]any{"op": 10, "d": map[string]any{"heartbeat_interval": 60000}}); err != nil {
			t.Errorf("write hello: %v", err)
			return
		}
		var auth map[string]any
		if err := conn.ReadJSON(&auth); err != nil {
			t.Errorf("read identify: %v", err)
			return
		}
		identify <- auth
		_ = conn.WriteJSON(map[string]any{"op": 0, "s": 1, "t": "READY", "d": map[string]any{"session_id": "session-1"}})
		_ = conn.WriteJSON(map[string]any{
			"op": 0, "s": 2, "t": "C2C_MESSAGE_CREATE",
			"d": map[string]any{"id": "message-1", "content": "hello", "author": map[string]any{"user_openid": "user-1"}},
		})
		_ = conn.WriteJSON(map[string]any{"op": 7})
	}))
	defer server.Close()

	channel := newQQBotChannel(qqBotAccount{AccountID: "account-1"})
	incoming := make(chan core.IncomingMessage, 1)
	channel.SetMessageHandler(func(_ context.Context, message core.IncomingMessage) error {
		incoming <- message
		return nil
	})
	gatewayURL := "ws" + strings.TrimPrefix(server.URL, "http")
	if err := channel.runGatewaySession(context.Background(), gatewayURL, "token-1"); err == nil || !strings.Contains(err.Error(), "reconnect") {
		t.Fatalf("runGatewaySession() error = %v, want reconnect request", err)
	}

	select {
	case auth := <-identify:
		if auth["op"] != float64(2) {
			t.Fatalf("identify op = %v, want 2", auth["op"])
		}
		data, _ := auth["d"].(map[string]any)
		if data["token"] != "QQBot token-1" || data["intents"] != float64(qqBotIntents) {
			t.Fatalf("identify data = %v", data)
		}
	default:
		t.Fatal("gateway did not receive identify payload")
	}
	select {
	case message := <-incoming:
		if message.ChatID != "qqbot:c2c:user-1" || message.MessageID != "message-1" || message.Text != "hello" {
			t.Fatalf("incoming message = %+v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for inbound message")
	}
	channel.mu.Lock()
	sessionID := channel.sessionID
	sequence := channel.sequence
	channel.mu.Unlock()
	if sessionID != "session-1" || sequence == nil || *sequence != 2 {
		t.Fatalf("gateway state = session %q sequence %v", sessionID, sequence)
	}
}

func TestQQBotGatewayResumesExistingSession(t *testing.T) {
	upgrader := websocket.Upgrader{}
	resume := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()
		_ = conn.WriteJSON(map[string]any{"op": 10, "d": map[string]any{"heartbeat_interval": 60000}})
		var auth map[string]any
		if err := conn.ReadJSON(&auth); err != nil {
			t.Errorf("read resume: %v", err)
			return
		}
		resume <- auth
		_ = conn.WriteJSON(map[string]any{"op": 7})
	}))
	defer server.Close()

	channel := newQQBotChannel(qqBotAccount{AccountID: "account-1"})
	channel.sessionID = "session-1"
	channel.setSequence(42)
	gatewayURL := "ws" + strings.TrimPrefix(server.URL, "http")
	if err := channel.runGatewaySession(context.Background(), gatewayURL, "token-1"); err == nil {
		t.Fatal("runGatewaySession() succeeded, want reconnect request")
	}

	select {
	case auth := <-resume:
		if auth["op"] != float64(6) {
			t.Fatalf("resume op = %v, want 6", auth["op"])
		}
		data, _ := auth["d"].(map[string]any)
		if data["token"] != "QQBot token-1" || data["session_id"] != "session-1" || data["seq"] != float64(42) {
			t.Fatalf("resume data = %v", data)
		}
	default:
		t.Fatal("gateway did not receive resume payload")
	}
}
