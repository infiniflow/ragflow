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
	"os"
	"path/filepath"
	"testing"
	"time"
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
