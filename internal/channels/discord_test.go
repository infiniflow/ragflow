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
	"testing"
	"time"

	"ragflow/internal/channels/core"
)

func TestNewDiscordChannelFromConfigRequiresToken(t *testing.T) {
	if _, err := newDiscordChannelFromConfig("account-1", map[string]any{}); err == nil {
		t.Fatal("newDiscordChannelFromConfig succeeded without token")
	}
}

func TestNewDiscordChannelFromConfigNormalizesConfig(t *testing.T) {
	ch, err := newDiscordChannelFromConfig("account-1", map[string]any{
		"token":           "Bot token-1",
		"api_base_url":    "https://discord.example/api",
		"gateway_url":     "wss://gateway.example",
		"timeout_secs":    "7",
		"gateway_intents": "4096",
	})
	if err != nil {
		t.Fatalf("newDiscordChannelFromConfig returned error: %v", err)
	}
	if ch.account.Token != "token-1" {
		t.Fatalf("token = %q, want %q", ch.account.Token, "token-1")
	}
	if ch.account.APIBaseURL != "https://discord.example/api" {
		t.Fatalf("api base URL = %q", ch.account.APIBaseURL)
	}
	if ch.account.GatewayURL != "wss://gateway.example" {
		t.Fatalf("gateway URL = %q", ch.account.GatewayURL)
	}
	if ch.account.Timeout != 7*time.Second {
		t.Fatalf("timeout = %s, want 7s", ch.account.Timeout)
	}
	if ch.account.Intents != 4096 {
		t.Fatalf("intents = %d, want 4096", ch.account.Intents)
	}
}

func TestDiscordChatType(t *testing.T) {
	dm := 1
	thread := 11
	group := 0

	tests := []struct {
		name        string
		guildID     string
		channelType *int
		want        string
	}{
		{name: "dm channel", channelType: &dm, want: "p2p"},
		{name: "thread channel", guildID: "guild-1", channelType: &thread, want: "thread"},
		{name: "guild text channel", guildID: "guild-1", channelType: &group, want: "group"},
		{name: "guild fallback", guildID: "guild-1", want: "group"},
		{name: "direct fallback", want: "p2p"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := discordChatType(tt.guildID, tt.channelType); got != tt.want {
				t.Fatalf("discordChatType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDiscordEnqueueDoesNotMarkDroppedMessageSeen(t *testing.T) {
	ch := newDiscordChannel(discordAccount{AccountID: "account-1", Token: "token-1"})
	worker := &discordChatWorker{queue: make(chan core.IncomingMessage, 1)}
	worker.queue <- core.IncomingMessage{MessageID: "queued"}
	ch.workers["chat-1"] = worker

	ok := ch.enqueueIncoming(context.Background(), core.IncomingMessage{
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
