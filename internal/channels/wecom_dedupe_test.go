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

func TestWeComMessageSeenDeduplicatesRetries(t *testing.T) {
	weComSeenMessagesMu.Lock()
	weComSeenMessages = map[string]time.Time{}
	weComSeenMessagesMu.Unlock()
	t.Cleanup(func() {
		weComSeenMessagesMu.Lock()
		weComSeenMessages = map[string]time.Time{}
		weComSeenMessagesMu.Unlock()
	})

	if weComMessageSeen("account-1", "msg-1") {
		t.Fatal("first delivery must not be treated as a duplicate")
	}
	if !weComMessageSeen("account-1", "msg-1") {
		t.Fatal("WeCom retry of the same message id must be deduplicated")
	}
	if weComMessageSeen("account-1", "msg-2") {
		t.Fatal("a different message id must not be a duplicate")
	}
	if weComMessageSeen("account-1", "") {
		t.Fatal("an empty message id must never be deduplicated")
	}
	if weComMessageSeen("account-2", "msg-1") {
		t.Fatal("the dedupe window must be scoped per account")
	}
}

func TestWeComMessageSeenExpiresAfterWindow(t *testing.T) {
	weComSeenMessagesMu.Lock()
	weComSeenMessages = map[string]time.Time{"account-1:msg-1": time.Now().Add(-weComDedupeWindow - time.Second)}
	weComSeenMessagesMu.Unlock()
	t.Cleanup(func() {
		weComSeenMessagesMu.Lock()
		weComSeenMessages = map[string]time.Time{}
		weComSeenMessagesMu.Unlock()
	})

	if weComMessageSeen("account-1", "msg-1") {
		t.Fatal("a message id older than the dedupe window must be handled again")
	}
}
