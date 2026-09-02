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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const webhookIDSecret = "webhook_id_secret"

type webhookTracePoll struct {
	WebhookID   *string          `json:"webhook_id"`
	Events      []map[string]any `json:"events"`
	NextSinceTS float64          `json:"next_since_ts"`
	Finished    bool             `json:"finished"`
}

type webhookTraceStore struct {
	Webhooks map[string]webhookTraceRun `json:"webhooks"`
}

type webhookTraceRun struct {
	StartTS float64          `json:"start_ts"`
	Events  []map[string]any `json:"events"`
}

// parseWebhookSinceTS parses an optional finite timestamp cursor.
func parseWebhookSinceTS(raw string) (float64, bool) {
	if strings.TrimSpace(raw) == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

// pollWebhookTrace advances one webhook trace polling step.
func pollWebhookTrace(raw string, sinceTS float64, webhookID string) (webhookTracePoll, error) {
	empty := newWebhookTracePoll(nil, sinceTS, false)
	if strings.TrimSpace(raw) == "" {
		return empty, nil
	}

	var store webhookTraceStore
	if err := json.Unmarshal([]byte(raw), &store); err != nil {
		return webhookTracePoll{}, fmt.Errorf("decode webhook trace: %w", err)
	}
	if webhookID == "" {
		startKey, startTS, ok := nextWebhookTraceRun(store.Webhooks, sinceTS)
		if !ok {
			return empty, nil
		}
		encodedID := encodeWebhookID(startKey)
		return newWebhookTracePoll(&encodedID, startTS, false), nil
	}

	realID, ok := decodeWebhookID(webhookID, store.Webhooks)
	if !ok {
		return newWebhookTracePoll(&webhookID, sinceTS, true), nil
	}
	return collectWebhookTraceEvents(store.Webhooks[realID], sinceTS, webhookID), nil
}

// newWebhookTracePoll keeps empty event lists serialized as [] instead of null.
func newWebhookTracePoll(webhookID *string, nextSinceTS float64, finished bool) webhookTracePoll {
	return webhookTracePoll{
		WebhookID:   webhookID,
		Events:      make([]map[string]any, 0),
		NextSinceTS: nextSinceTS,
		Finished:    finished,
	}
}

// nextWebhookTraceRun selects the earliest run that started after the cursor.
func nextWebhookTraceRun(webhooks map[string]webhookTraceRun, sinceTS float64) (string, float64, bool) {
	var selectedKey string
	var selectedTS float64
	found := false
	for startKey := range webhooks {
		startTS, err := strconv.ParseFloat(startKey, 64)
		if err != nil || startTS <= sinceTS || (found && startTS >= selectedTS) {
			continue
		}
		selectedKey, selectedTS, found = startKey, startTS, true
	}
	return selectedKey, selectedTS, found
}

// encodeWebhookID creates an opaque run cursor. Canvas ownership is checked
// before the cursor is decoded.
func encodeWebhookID(startTS string) string {
	mac := hmac.New(sha256.New, []byte(webhookIDSecret))
	_, _ = mac.Write([]byte(startTS))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// decodeWebhookID resolves an opaque cursor without exposing trace timestamps.
func decodeWebhookID(webhookID string, webhooks map[string]webhookTraceRun) (string, bool) {
	for startTS := range webhooks {
		if hmac.Equal([]byte(encodeWebhookID(startTS)), []byte(webhookID)) {
			return startTS, true
		}
	}
	return "", false
}

// collectWebhookTraceEvents returns only events newer than the supplied cursor.
func collectWebhookTraceEvents(run webhookTraceRun, sinceTS float64, webhookID string) webhookTracePoll {
	result := newWebhookTracePoll(&webhookID, sinceTS, false)
	for _, event := range run.Events {
		eventTS := webhookTraceEventTimestamp(event)
		if eventTS <= sinceTS {
			continue
		}
		result.Events = append(result.Events, event)
		if eventTS > result.NextSinceTS {
			result.NextSinceTS = eventTS
		}
		if eventType, _ := event["event"].(string); eventType == "finished" {
			result.Finished = true
			break
		}
	}
	return result
}

// webhookTraceEventTimestamp returns zero when an event has no numeric timestamp.
func webhookTraceEventTimestamp(event map[string]any) float64 {
	switch value := event["ts"].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		parsed, _ := value.Float64()
		return parsed
	default:
		return 0
	}
}
