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

package admin

import "testing"

func TestMessageQueueServiceStatus(t *testing.T) {
	tests := []struct {
		name        string
		connState   string
		wantStatus  string
		wantMessage string
	}{
		{name: "connected maps to shared alive vocabulary", connState: "CONNECTED", wantStatus: "alive", wantMessage: ""},
		{name: "closed keeps raw state as message", connState: "CLOSED", wantStatus: "timeout", wantMessage: "CLOSED"},
		{name: "disconnected keeps raw state as message", connState: "DISCONNECTED", wantStatus: "timeout", wantMessage: "DISCONNECTED"},
		{name: "nil connection diagnostic becomes message", connState: "NATS connection is nil, engine not properly initialized", wantStatus: "timeout", wantMessage: "NATS connection is nil, engine not properly initialized"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, message := messageQueueServiceStatus(tt.connState)
			if status != tt.wantStatus || message != tt.wantMessage {
				t.Errorf("messageQueueServiceStatus(%q) = (%q, %q), want (%q, %q)",
					tt.connState, status, message, tt.wantStatus, tt.wantMessage)
			}
		})
	}
}
