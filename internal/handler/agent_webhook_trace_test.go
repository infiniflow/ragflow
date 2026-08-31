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
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

type webhookTraceResponseData struct {
	WebhookID   *string          `json:"webhook_id"`
	Events      []map[string]any `json:"events"`
	NextSinceTS float64          `json:"next_since_ts"`
	Finished    bool             `json:"finished"`
}

type webhookTraceResponse struct {
	Code    int                       `json:"code"`
	Data    *webhookTraceResponseData `json:"data"`
	Message string                    `json:"message"`
}

type miniredisWebhookTraceWriter struct {
	client *goredis.Client
}

func (w miniredisWebhookTraceWriter) Get(ctx context.Context, key string) (string, error) {
	value, err := w.client.Get(ctx, key).Result()
	if errors.Is(err, goredis.Nil) {
		return "", nil
	}
	return value, err
}

func (w miniredisWebhookTraceWriter) SetObj(ctx context.Context, key string, value any, ttl time.Duration) bool {
	payload, err := json.Marshal(value)
	if err != nil {
		return false
	}
	return w.client.Set(ctx, key, payload, ttl).Err() == nil
}

// newWebhookTraceTestHandler wires ownership storage and miniredis for HTTP tests.
func newWebhookTraceTestHandler(t *testing.T) (*AgentHandler, *goredis.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db := setupHandlerAgentsTestDB(t)
	originalDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = originalDB })
	if err := db.Create(&entity.UserCanvas{ID: "c1", UserID: "u1", Title: sptr("Test")}).Error; err != nil {
		t.Fatalf("create canvas: %v", err)
	}

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ctx := t.Context()
	h := NewAgentHandler(ctx, service.NewAgentService(), nil).
		WithRedisGetter(func(key string) (string, error) {
			value, getErr := rdb.Get(ctx, key).Result()
			if errors.Is(getErr, goredis.Nil) {
				return "", nil
			}
			return value, getErr
		})
	return h, rdb
}

// requestWebhookTrace invokes the real Gin handler and decodes its envelope.
func requestWebhookTrace(t *testing.T, h *AgentHandler, canvasID, userID string, query url.Values) webhookTraceResponse {
	t.Helper()
	path := "/api/v1/agents/" + canvasID + "/webhook/logs"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", path, nil)
	c.Set("user", &entity.User{ID: userID})
	c.Set("user_id", userID)
	c.Params = gin.Params{{Key: "canvas_id", Value: canvasID}}

	h.GetAgentWebhookLogs(c)

	var response webhookTraceResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
	}
	return response
}

// seedWebhookTrace writes the same Redis shape produced by appendWebhookTrace.
func seedWebhookTrace(t *testing.T, rdb *goredis.Client, canvasID string, webhooks map[string]any) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"webhooks": webhooks})
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}
	if err = rdb.Set(t.Context(), "webhook-trace-"+canvasID+"-logs", payload, 0).Err(); err != nil {
		t.Fatalf("seed trace: %v", err)
	}
}

func TestAppendWebhookTracePersistsReadableEvents(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ctx := t.Context()
	writer := miniredisWebhookTraceWriter{client: rdb}
	start := time.Unix(1_700_000_000, 0)
	appendWebhookTraceWithWriter(ctx, writer, "c1", start, canvas.RunEvent{
		Type:      "message",
		Data:      `{"content":"hello"}`,
		SessionID: "task-1",
	})
	appendWebhookTraceWithWriter(ctx, writer, "c1", start, canvas.RunEvent{
		Type: "finished",
		Data: `{"success":true}`,
	})

	const key = "webhook-trace-c1-logs"
	raw, err := rdb.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("read persisted trace: %v", err)
	}
	sinceTS := float64(start.Unix() - 1)
	discovery, err := pollWebhookTrace(raw, sinceTS, "")
	if err != nil || discovery.WebhookID == nil {
		t.Fatalf("discover persisted trace: result=%+v err=%v", discovery, err)
	}
	poll, err := pollWebhookTrace(raw, sinceTS, *discovery.WebhookID)
	if err != nil {
		t.Fatalf("poll persisted trace: %v", err)
	}
	if len(poll.Events) != 2 || poll.Events[0]["event"] != "message" || poll.Events[1]["event"] != "finished" {
		t.Fatalf("persisted events = %+v, want message and finished", poll.Events)
	}
	if !poll.Finished {
		t.Fatal("persisted trace should be finished")
	}
	if ttl := mr.TTL(key); ttl != 600*time.Second {
		t.Fatalf("trace TTL = %s, want 10m0s", ttl)
	}
}

// TestGetAgentWebhookLogsPollsTraceIncrementally covers the complete UI poll flow.
func TestGetAgentWebhookLogsPollsTraceIncrementally(t *testing.T) {
	h, rdb := newWebhookTraceTestHandler(t)

	before := float64(time.Now().UnixNano()) / 1e9
	initial := requestWebhookTrace(t, h, "c1", "u1", url.Values{})
	after := float64(time.Now().UnixNano()) / 1e9
	if initial.Code != int(common.CodeSuccess) || initial.Data == nil {
		t.Fatalf("initial response = %+v", initial)
	}
	if initial.Data.WebhookID != nil || len(initial.Data.Events) != 0 || initial.Data.Finished {
		t.Fatalf("initial data = %+v, want empty unfinished cursor", initial.Data)
	}
	if initial.Data.NextSinceTS < before || initial.Data.NextSinceTS > after {
		t.Fatalf("initial next_since_ts = %f, want between %f and %f", initial.Data.NextSinceTS, before, after)
	}

	startTS := initial.Data.NextSinceTS + 1
	messageTS := startTS + 1
	finishedTS := startTS + 2
	trailingTS := startTS + 3
	startKey := strconv.FormatFloat(startTS, 'f', -1, 64)
	laterKey := strconv.FormatFloat(startTS+10, 'f', -1, 64)
	seedWebhookTrace(t, rdb, "c1", map[string]any{
		laterKey: map[string]any{
			"start_ts": startTS + 10,
			"events":   []any{},
		},
		startKey: map[string]any{
			"start_ts": startTS,
			"events": []any{
				map[string]any{
					"ts":      messageTS,
					"event":   "message",
					"data":    map[string]any{"content": "done"},
					"task_id": "task-1",
				},
				map[string]any{
					"ts":    finishedTS,
					"event": "finished",
					"data":  map[string]any{"success": true},
				},
				map[string]any{
					"ts":    trailingTS,
					"event": "message",
					"data":  map[string]any{"content": "late"},
				},
			},
		},
	})

	discovery := requestWebhookTrace(t, h, "c1", "u1", url.Values{
		"since_ts": {strconv.FormatFloat(initial.Data.NextSinceTS, 'f', -1, 64)},
	})
	if discovery.Data == nil || discovery.Data.WebhookID == nil {
		t.Fatalf("discovery data = %+v, want webhook id", discovery.Data)
	}
	if discovery.Data.NextSinceTS != startTS || len(discovery.Data.Events) != 0 || discovery.Data.Finished {
		t.Fatalf("discovery data = %+v, want earliest run cursor", discovery.Data)
	}

	poll := requestWebhookTrace(t, h, "c1", "u1", url.Values{
		"since_ts":   {strconv.FormatFloat(initial.Data.NextSinceTS, 'f', -1, 64)},
		"webhook_id": {*discovery.Data.WebhookID},
	})
	if poll.Data == nil ||
		len(poll.Data.Events) != 2 ||
		poll.Data.Events[1]["event"] != "finished" ||
		!poll.Data.Finished {
		t.Fatalf("poll data = %+v, want two events and finished", poll.Data)
	}
	if poll.Data.NextSinceTS != finishedTS {
		t.Errorf("poll next_since_ts = %f, want %f", poll.Data.NextSinceTS, finishedTS)
	}
	messageData, ok := poll.Data.Events[0]["data"].(map[string]any)
	if !ok {
		t.Fatalf("message data = %T, want object", poll.Data.Events[0]["data"])
	}
	if content, _ := messageData["content"].(string); content != "done" {
		t.Errorf("message content = %q, want done", content)
	}

	incremental := requestWebhookTrace(t, h, "c1", "u1", url.Values{
		"since_ts":   {strconv.FormatFloat(messageTS, 'f', -1, 64)},
		"webhook_id": {*discovery.Data.WebhookID},
	})
	if incremental.Data == nil || len(incremental.Data.Events) != 1 || incremental.Data.Events[0]["event"] != "finished" {
		t.Fatalf("incremental data = %+v, want only finished event", incremental.Data)
	}
	if !incremental.Data.Finished || incremental.Data.NextSinceTS != finishedTS {
		t.Fatalf("incremental completion = %+v", incremental.Data)
	}
}

// TestGetAgentWebhookLogsHandlesMissingAndInvalidState covers empty and stale cursors.
func TestGetAgentWebhookLogsHandlesMissingAndInvalidState(t *testing.T) {
	h, rdb := newWebhookTraceTestHandler(t)

	missing := requestWebhookTrace(t, h, "c1", "u1", url.Values{"since_ts": {"42"}})
	if missing.Data == nil || missing.Data.WebhookID != nil || missing.Data.NextSinceTS != 42 || missing.Data.Finished {
		t.Fatalf("missing trace data = %+v", missing.Data)
	}

	seedWebhookTrace(t, rdb, "c1", map[string]any{
		"50": map[string]any{"start_ts": 50, "events": []any{}},
	})
	forged := requestWebhookTrace(t, h, "c1", "u1", url.Values{
		"since_ts":   {"42"},
		"webhook_id": {"forged-id"},
	})
	if forged.Data == nil || forged.Data.WebhookID == nil || *forged.Data.WebhookID != "forged-id" || !forged.Data.Finished {
		t.Fatalf("forged id data = %+v, want finished invalid cursor", forged.Data)
	}

	invalidSince := requestWebhookTrace(t, h, "c1", "u1", url.Values{"since_ts": {"not-a-number"}})
	if invalidSince.Data == nil || invalidSince.Data.NextSinceTS <= 0 || invalidSince.Data.Finished {
		t.Fatalf("invalid since_ts data = %+v, want a fresh cursor", invalidSince.Data)
	}
}

// TestGetAgentWebhookLogsRedactsRedisFailures covers corrupt data and backend errors.
func TestGetAgentWebhookLogsRedactsRedisFailures(t *testing.T) {
	h, rdb := newWebhookTraceTestHandler(t)
	if err := rdb.Set(t.Context(), "webhook-trace-c1-logs", `{"webhooks":`, 0).Err(); err != nil {
		t.Fatalf("seed corrupt trace: %v", err)
	}

	corrupt := requestWebhookTrace(t, h, "c1", "u1", url.Values{"since_ts": {"0"}})
	if corrupt.Code != int(common.CodeServerError) || corrupt.Message != common.CodeServerError.Message() {
		t.Fatalf("corrupt response = %+v", corrupt)
	}

	h.WithRedisGetter(func(string) (string, error) {
		return "", errors.New("redis password=secret")
	})
	failed := requestWebhookTrace(t, h, "c1", "u1", url.Values{"since_ts": {"0"}})
	if failed.Code != int(common.CodeServerError) || strings.Contains(failed.Message, "password=secret") {
		t.Fatalf("redis failure response = %+v", failed)
	}
}

// TestGetAgentWebhookLogsChecksOwnershipBeforeRedis prevents cross-user trace probes.
func TestGetAgentWebhookLogsChecksOwnershipBeforeRedis(t *testing.T) {
	h, _ := newWebhookTraceTestHandler(t)
	redisCalls := 0
	h.WithRedisGetter(func(string) (string, error) {
		redisCalls++
		return "", nil
	})

	response := requestWebhookTrace(t, h, "c1", "other-user", url.Values{"since_ts": {"0"}})
	if response.Code != int(common.CodeDataError) || response.Message != "Canvas not found." {
		t.Fatalf("ownership response = %+v", response)
	}
	if redisCalls != 0 {
		t.Fatalf("redis calls = %d, want 0 before ownership succeeds", redisCalls)
	}
}

// TestEncodeWebhookIDUsesStableEncoding pins deterministic cursor output.
func TestEncodeWebhookIDUsesStableEncoding(t *testing.T) {
	const want = "7a7-Rfe0PSB5OwV10qD7SWcmrtbFhfQKTZajRny8STM"
	if got := encodeWebhookID("123.5"); got != want {
		t.Fatalf("encodeWebhookID = %q, want %q", got, want)
	}
}
