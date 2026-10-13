// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package handler

import "testing"

// TestPollWebhookTraceMissingRun distinguishes waiting for a trigger from losing an already selected trace.
func TestPollWebhookTraceMissingRun(t *testing.T) {
	id := encodeWebhookID("50")
	tests := []struct {
		name      string
		raw       string
		webhookID string
		finished  bool
	}{
		{"waiting without logs", "", "", false},
		{"waiting with blank logs", " \n", "", false},
		{"expired selected trace", "", id, true},
		{"blank selected trace", " \n", id, true},
		{"empty store", `{"webhooks":{}}`, id, true},
		{"another run remains", `{"webhooks":{"60":{"start_ts":60,"events":[]}}}`, id, true},
		{"selected run still active", `{"webhooks":{"50":{"start_ts":50,"events":[]}}}`, id, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := pollWebhookTrace(tt.raw, 51, tt.webhookID)
			if err != nil {
				t.Fatal(err)
			}
			if result.Finished != tt.finished || result.NextSinceTS != 51 {
				t.Fatalf("result = %+v, want finished=%t with unchanged cursor", result, tt.finished)
			}
			if tt.webhookID == "" {
				if result.WebhookID != nil {
					t.Fatalf("unexpected webhook ID: %v", result.WebhookID)
				}
			} else if result.WebhookID == nil || *result.WebhookID != tt.webhookID {
				t.Fatalf("result = %+v, want selected webhook ID preserved", result)
			}
			// Ending trace polling must not synthesize a successful execution event.
			if result.Events == nil || len(result.Events) != 0 {
				t.Fatalf("events = %#v, want an empty non-nil slice", result.Events)
			}
		})
	}
}
