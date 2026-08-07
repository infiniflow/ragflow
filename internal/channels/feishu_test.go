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
	"testing"
	"time"

	"ragflow/internal/channels/core"
)

func TestNewFeishuChannelFromConfigRequiresCredentials(t *testing.T) {
	if _, err := newFeishuChannelFromConfig("account-1", map[string]any{"app_id": "app-1"}); err == nil {
		t.Fatal("newFeishuChannelFromConfig succeeded without app_secret")
	}
}

func TestNewFeishuChannelFromConfigNormalizesConfig(t *testing.T) {
	ch, err := newFeishuChannelFromConfig("account-1", map[string]any{
		"app_id":       "app-1",
		"app_secret":   "secret-1",
		"domain":       "lark",
		"timeout_secs": "7",
	})
	if err != nil {
		t.Fatalf("newFeishuChannelFromConfig returned error: %v", err)
	}
	if ch.account.AppID != "app-1" {
		t.Fatalf("app id = %q, want app-1", ch.account.AppID)
	}
	if ch.account.AppSecret != "secret-1" {
		t.Fatalf("app secret = %q, want secret-1", ch.account.AppSecret)
	}
	if ch.account.Domain != "lark" {
		t.Fatalf("domain = %q, want lark", ch.account.Domain)
	}
	if ch.account.Timeout != 7*time.Second {
		t.Fatalf("timeout = %s, want 7s", ch.account.Timeout)
	}
}

func TestFeishuMessageText(t *testing.T) {
	raw := `{"text":"hello"}`
	if got := feishuMessageText(&raw); got != "hello" {
		t.Fatalf("feishuMessageText() = %q, want hello", got)
	}

	plain := "hello"
	if got := feishuMessageText(&plain); got != "hello" {
		t.Fatalf("feishuMessageText() = %q, want raw fallback", got)
	}
}

func TestFeishuEnqueueDoesNotMarkDroppedMessageSeen(t *testing.T) {
	ch := newFeishuChannel(feishuAccount{AccountID: "account-1", AppID: "app-1", AppSecret: "secret-1"})
	worker := &feishuWorker{queue: make(chan core.IncomingMessage, 1)}
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
