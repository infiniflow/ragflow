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
	"os"
	"path/filepath"
	"testing"
	"time"

	"ragflow/internal/channels/core"
)

func TestStartRetryDelay(t *testing.T) {
	tests := []struct {
		name     string
		attempts int
		want     time.Duration
	}{
		{name: "zero attempt uses first delay", attempts: 0, want: initialStartRetryDelay},
		{name: "first attempt", attempts: 1, want: initialStartRetryDelay},
		{name: "second attempt doubles", attempts: 2, want: 2 * initialStartRetryDelay},
		{name: "fifth attempt", attempts: 5, want: 16 * initialStartRetryDelay},
		{name: "bounded", attempts: 20, want: maxStartRetryDelay},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := startRetryDelay(tt.attempts); got != tt.want {
				t.Fatalf("startRetryDelay(%d) = %s, want %s", tt.attempts, got, tt.want)
			}
		})
	}
}

func TestRecordStartFailure(t *testing.T) {
	rt := NewRuntime()
	now := time.Unix(100, 0)

	rt.recordStartFailure("account-1", "fp-1", now)
	first := rt.failed["account-1"]
	if first.attempts != 1 {
		t.Fatalf("attempts after first failure = %d, want 1", first.attempts)
	}
	if !first.nextRetryAt.Equal(now.Add(initialStartRetryDelay)) {
		t.Fatalf("nextRetryAt after first failure = %s, want %s", first.nextRetryAt, now.Add(initialStartRetryDelay))
	}

	rt.recordStartFailure("account-1", "fp-1", now)
	second := rt.failed["account-1"]
	if second.attempts != 2 {
		t.Fatalf("attempts after second failure = %d, want 2", second.attempts)
	}
	if !second.nextRetryAt.Equal(now.Add(2 * initialStartRetryDelay)) {
		t.Fatalf("nextRetryAt after second failure = %s, want %s", second.nextRetryAt, now.Add(2*initialStartRetryDelay))
	}

	rt.recordStartFailure("account-1", "fp-2", now)
	changed := rt.failed["account-1"]
	if changed.attempts != 1 {
		t.Fatalf("attempts after fingerprint change = %d, want 1", changed.attempts)
	}
}

func TestClearStartFailure(t *testing.T) {
	rt := NewRuntime()
	rt.recordStartFailure("account-1", "fp-1", time.Unix(100, 0))

	rt.clearStartFailure("account-1")

	if _, ok := rt.failed["account-1"]; ok {
		t.Fatal("clearStartFailure left failed entry behind")
	}
}

func TestGatewayWorkdirDefaultContainsNodeEntrypoint(t *testing.T) {
	t.Setenv("WHATSAPP_GATEWAY_WORKDIR", "")

	entry := filepath.Join(gatewayWorkdir(), "index.js")
	if _, err := os.Stat(entry); err != nil {
		t.Fatalf("default gateway entry %s is not available: %v", entry, err)
	}
}

func TestGatewayEnabledDefaultsToUserManaged(t *testing.T) {
	t.Setenv("WHATSAPP_GATEWAY_ENABLED", "")

	if gatewayEnabled() {
		t.Fatal("gatewayEnabled() = true, want false by default")
	}
}

func TestGatewayEnabledCanBeExplicitlyEnabled(t *testing.T) {
	t.Setenv("WHATSAPP_GATEWAY_ENABLED", "true")

	if !gatewayEnabled() {
		t.Fatal("gatewayEnabled() = false, want true")
	}
}

func TestGatewayEnabledRejectsInvalidValue(t *testing.T) {
	t.Setenv("WHATSAPP_GATEWAY_ENABLED", "maybe")

	if gatewayEnabled() {
		t.Fatal("gatewayEnabled() = true for invalid value, want false")
	}
}

func TestWhatsAppEnqueueDoesNotMarkDroppedMessageSeen(t *testing.T) {
	ch := newWhatsAppChannel(whatsappAccount{AccountID: "account-1"})
	worker := &whatsappChatWorker{queue: make(chan core.IncomingMessage, 1)}
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

func TestWhatsAppEnqueueMarksSeenAfterSuccessfulHandoff(t *testing.T) {
	ch := newWhatsAppChannel(whatsappAccount{AccountID: "account-1"})
	ch.workers["chat-1"] = &whatsappChatWorker{queue: make(chan core.IncomingMessage, 1)}
	msg := core.IncomingMessage{ChatID: "chat-1", MessageID: "message-1"}

	if ok := ch.enqueueIncoming(context.Background(), msg); !ok {
		t.Fatal("enqueueIncoming failed for an empty queue")
	}
	if _, seen := ch.seen[msg.MessageID]; !seen {
		t.Fatal("successfully handed off message was not marked seen")
	}
	if ok := ch.enqueueIncoming(context.Background(), msg); ok {
		t.Fatal("duplicate message was enqueued")
	}
}

func TestWhatsAppRetireChatWorkerRequiresCurrentEmptyWorker(t *testing.T) {
	ch := newWhatsAppChannel(whatsappAccount{AccountID: "account-1"})
	worker := &whatsappChatWorker{queue: make(chan core.IncomingMessage, 1)}
	worker.queue <- core.IncomingMessage{ChatID: "chat-1", MessageID: "message-1"}
	ch.workers["chat-1"] = worker

	if ch.retireChatWorker("chat-1", worker) {
		t.Fatal("retireChatWorker retired a non-empty worker")
	}
	if ch.workers["chat-1"] != worker {
		t.Fatal("non-empty worker was removed from ownership map")
	}

	<-worker.queue
	if !ch.retireChatWorker("chat-1", worker) {
		t.Fatal("retireChatWorker did not retire an empty current worker")
	}
	if _, ok := ch.workers["chat-1"]; ok {
		t.Fatal("empty retired worker remains in ownership map")
	}
}
