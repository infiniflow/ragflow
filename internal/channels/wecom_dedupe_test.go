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
)

func TestWeComMessageReservationDeduplicatesRetries(t *testing.T) {
	resetWeComDedupeForTest(t)

	if state := weComReserveMessage("account-1", "msg-1"); state != weComMessageAccepted {
		t.Fatal("first delivery must not be treated as a duplicate")
	}
	if state := weComReserveMessage("account-1", "msg-1"); state != weComMessageInFlight {
		t.Fatalf("in-flight retry state = %v, want in-flight", state)
	}
	weComMarkMessageProcessed("account-1", "msg-1")
	if state := weComReserveMessage("account-1", "msg-1"); state != weComMessageProcessed {
		t.Fatalf("processed retry state = %v, want processed", state)
	}
	if state := weComReserveMessage("account-1", "msg-2"); state != weComMessageAccepted {
		t.Fatal("a different message id must not be a duplicate")
	}
	if state := weComReserveMessage("account-1", ""); state != weComMessageAccepted {
		t.Fatal("an empty message id must never be deduplicated")
	}
	if state := weComReserveMessage("account-2", "msg-1"); state != weComMessageAccepted {
		t.Fatal("the dedupe window must be scoped per account")
	}
}

func TestWeComMessageSeenExpiresAfterWindow(t *testing.T) {
	resetWeComDedupeForTest(t)
	weComSeenMessagesMu.Lock()
	weComSeenMessages = map[string]time.Time{"account-1:msg-1": time.Now().Add(-weComDedupeWindow - time.Second)}
	weComInFlightMessages = map[string]struct{}{}
	weComSeenMessagesMu.Unlock()

	if state := weComReserveMessage("account-1", "msg-1"); state != weComMessageAccepted {
		t.Fatal("a message id older than the dedupe window must be handled again")
	}
}

func TestWeComForgetMessageAllowsRetryAfterFailure(t *testing.T) {
	resetWeComDedupeForTest(t)

	if state := weComReserveMessage("account-1", "msg-1"); state != weComMessageAccepted {
		t.Fatal("first delivery must not be treated as a duplicate")
	}
	weComForgetMessage("account-1", "msg-1")
	if state := weComReserveMessage("account-1", "msg-1"); state != weComMessageAccepted {
		t.Fatal("a message cleared after a retryable failure must be handled again")
	}
}

func TestWeComMessageStaysDeduplicatedUntilProcessed(t *testing.T) {
	resetWeComDedupeForTest(t)

	if state := weComReserveMessage("account-1", "msg-1"); state != weComMessageAccepted {
		t.Fatal("first delivery must not be treated as a duplicate")
	}
	if state := weComReserveMessage("account-1", "msg-1"); state != weComMessageInFlight {
		t.Fatalf("in-flight retry state = %v, want in-flight", state)
	}
	weComMarkMessageProcessed("account-1", "msg-1")
	if state := weComReserveMessage("account-1", "msg-1"); state != weComMessageProcessed {
		t.Fatalf("processed retry state = %v, want processed", state)
	}
}

func TestWeComWorkSlotsAreBounded(t *testing.T) {
	slots := make(chan struct{}, weComMaxConcurrentHandlers)
	for i := 0; i < weComMaxConcurrentHandlers; i++ {
		if !tryAcquireSlot(slots) {
			t.Fatalf("acquire %d of %d failed", i+1, weComMaxConcurrentHandlers)
		}
	}
	if tryAcquireSlot(slots) {
		t.Fatal("acquire beyond the concurrency limit must fail")
	}
	for i := 0; i < weComMaxConcurrentHandlers; i++ {
		releaseSlot(slots)
	}
	if !tryAcquireSlot(slots) {
		t.Fatal("releasing the work slots must allow a new handler")
	}
	releaseSlot(slots)
}

func resetWeComDedupeForTest(t *testing.T) {
	t.Helper()
	weComSeenMessagesMu.Lock()
	weComSeenMessages = map[string]time.Time{}
	weComInFlightMessages = map[string]struct{}{}
	weComSeenMessagesMu.Unlock()
	t.Cleanup(func() {
		weComSeenMessagesMu.Lock()
		weComSeenMessages = map[string]time.Time{}
		weComInFlightMessages = map[string]struct{}{}
		weComSeenMessagesMu.Unlock()
	})
}
