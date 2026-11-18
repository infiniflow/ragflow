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

import (
	"encoding/json"
	"testing"
)

// TestPollWebhookTraceKeepsTerminalState verifies completion survives advancing the event cursor.
func TestPollWebhookTraceKeepsTerminalState(t *testing.T) {
	for _, outcome := range []struct {
		name    string
		success bool
	}{
		{"successful run", true},
		{"failed run", false},
	} {
		t.Run(outcome.name, func(t *testing.T) {
			store := webhookTraceStore{Webhooks: map[string]webhookTraceRun{
				"50": {StartTS: 50, Events: []map[string]any{
					{"ts": 51, "event": "message"},
					{"ts": 52, "event": "finished", "data": map[string]any{"success": outcome.success}},
					{"ts": 53, "event": "message", "data": map[string]any{"content": "late"}},
				}},
			}}
			raw, err := json.Marshal(store)
			if err != nil {
				t.Fatal(err)
			}
			id := encodeWebhookID("50")
			first, err := pollWebhookTrace(string(raw), 50, id)
			if err != nil || !first.Finished || len(first.Events) != 2 || first.NextSinceTS != 52 {
				t.Fatalf("first poll = %+v, error = %v", first, err)
			}
			data := first.Events[1]["data"].(map[string]any)
			if data["success"] != outcome.success {
				t.Fatalf("terminal outcome = %#v, want success=%t", data, outcome.success)
			}
			for _, cursor := range []float64{first.NextSinceTS, 60} {
				repeated, err := pollWebhookTrace(string(raw), cursor, id)
				if err != nil || !repeated.Finished || repeated.NextSinceTS != cursor {
					t.Fatalf("repeat at %v = %+v, error = %v", cursor, repeated, err)
				}
				if repeated.WebhookID == nil || *repeated.WebhookID != id || repeated.Events == nil || len(repeated.Events) != 0 {
					t.Fatalf("repeat at %v = %+v, want same run and no replayed or trailing events", cursor, repeated)
				}
			}
		})
	}
}

// TestPollWebhookTraceExhaustedActiveRun does not infer completion merely from an empty event delta.
func TestPollWebhookTraceExhaustedActiveRun(t *testing.T) {
	raw := `{"webhooks":{"50":{"start_ts":50,"events":[{"ts":51,"event":"message"}]}}}`
	result, err := pollWebhookTrace(raw, 51, encodeWebhookID("50"))
	if err != nil || result.Finished || result.NextSinceTS != 51 || result.Events == nil || len(result.Events) != 0 {
		t.Fatalf("active run = %+v, error = %v, want unfinished with no new events", result, err)
	}
}

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
