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

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/ingestion/task"
	"ragflow/internal/service"
)

// TestParseAgentLogs pins the contract between GetAgentLogs and the
// front-end DataFlow Log box. The Python pipeline (rag/flow/pipeline.py
// callback) stores a JSON *array* ([{component_id, trace:[...]}]); the
// front-end's useFetchMessageTrace (web/src/hooks/use-agent-request.ts)
// requires response.data to be that array. The original handler unmarshalled
// into a map[string]interface{} and silently dropped the array, leaving the
// Log box stuck on an empty SkeletonCard. A missing/empty/corrupt payload must
// still collapse to an empty object to mirror the Python get_agent_logs
// missing-key contract (api/apps/restful_apis/agent_api.py:1087).
func TestParseAgentLogs(t *testing.T) {
	cases := []struct {
		name     string
		payload  string
		wantKind byte // '[' for array, '{' for object
	}{
		{
			name:     "missing_key_returns_empty_object",
			payload:  "",
			wantKind: '{',
		},
		{
			name: "present_array_returns_array",
			payload: `[` +
				`{"component_id":"File","trace":[` +
				`{"progress":1,"message":"parsed","datetime":"10:00:00","timestamp":1.0,"elapsed_time":0}` +
				`]},` +
				`{"component_id":"END","trace":[{"progress":1,"message":"done","datetime":"10:00:01","timestamp":2.0,"elapsed_time":1.0}]}` +
				`]`,
			wantKind: '[',
		},
		{
			name:     "corrupt_payload_falls_back_to_empty_object",
			payload:  `{"component_id":`,
			wantKind: '{',
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseAgentLogs(tc.payload)
			b, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if len(b) == 0 || b[0] != tc.wantKind {
				t.Fatalf("want JSON kind %q, got %s", tc.wantKind, string(b))
			}
			if tc.wantKind == '[' {
				var arr []map[string]interface{}
				if err := json.Unmarshal(b, &arr); err != nil {
					t.Fatalf("round-trip: %v", err)
				}
				if arr[0]["component_id"] != "File" {
					t.Fatalf("component_id lost after round-trip: %v", arr[0])
				}
				if arr[len(arr)-1]["component_id"] != "END" {
					t.Fatalf("END marker lost after round-trip: %v", arr[len(arr)-1])
				}
			}
		})
	}
}

// TestGetAgentLogs_E2EViaMiniredis exercises the full HTTP handler path
// (auth -> canvas-access -> Redis fetch -> shape) against an in-memory
// miniredis so we can assert the wire response shape the front-end depends on:
//   - present key  -> response.data is the JSON array ([...]) with the END marker
//   - missing key  -> response.data is an empty object ("{}"), matching the
//     Python get_agent_logs missing-key contract.
//
// The Redis client is injected via WithRedisGetter (a miniredis-backed
// go-redis client); the canvas-access gate is satisfied by a real in-memory
// UserCanvas row, mirroring TestGetAgentWebhookLogsReturnsEmptyPoll.
func TestGetAgentLogs_E2EViaMiniredis(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := t.Context()
	db := setupHandlerAgentsTestDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	db.Create(&entity.UserCanvas{ID: "c1", UserID: "u1", Title: sptr("Test")})

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)

	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	logKey := "c1-msg1-logs"
	arrayPayload := `[` +
		`{"component_id":"File","trace":[{"progress":1,"message":"parsed","datetime":"10:00:00","timestamp":1.0,"elapsed_time":0}]},` +
		`{"component_id":"END","trace":[{"progress":1,"message":"done","datetime":"10:00:01","timestamp":2.0,"elapsed_time":1.0}]}` +
		`]`
	if err = rdb.Set(ctx, logKey, arrayPayload, 0).Err(); err != nil {
		t.Fatalf("seed redis: %v", err)
	}

	h := NewAgentHandler(ctx, service.NewAgentService(), nil).
		WithRedisGetter(func(key string) (string, error) {
			return rdb.Get(ctx, key).Result()
		})

	run := func(messageID string) map[string]interface{} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/v1/agents/c1/logs/"+messageID, nil)
		c.Set("user", &entity.User{ID: "u1"})
		c.Set("user_id", "u1")
		c.Params = gin.Params{
			{Key: "canvas_id", Value: "c1"},
			{Key: "message_id", Value: messageID},
		}
		h.GetAgentLogs(c)
		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode (msg=%s): %v body=%s", messageID, err, w.Body.String())
		}
		return resp
	}

	// Present key -> data must be the array, with File first and END last.
	present := run("msg1")
	if code, _ := present["code"].(float64); int(code) != int(common.CodeSuccess) {
		t.Fatalf("present code=%v want 0 body=%s", code, mustJSON(present))
	}
	dataRaw, _ := json.Marshal(present["data"])
	trimmed := strings.TrimSpace(string(dataRaw))
	if len(trimmed) == 0 || trimmed[0] != '[' {
		t.Fatalf("present response.data must be a JSON array, got %s", string(dataRaw))
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal(dataRaw, &arr); err != nil {
		t.Fatalf("present data not array: %v body=%s", err, string(dataRaw))
	}
	if arr[0]["component_id"] != "File" {
		t.Errorf("first component_id=%v want File", arr[0]["component_id"])
	}
	if arr[len(arr)-1]["component_id"] != "END" {
		t.Errorf("last component_id=%v want END", arr[len(arr)-1]["component_id"])
	}

	// Missing key -> data must be an empty object (Python parity), code 0.
	missing := run("missing")
	if code, _ := missing["code"].(float64); int(code) != int(common.CodeSuccess) {
		t.Fatalf("missing code=%v want 0 body=%s", code, mustJSON(missing))
	}
	dataRawMissing, _ := json.Marshal(missing["data"])
	if strings.TrimSpace(string(dataRawMissing)) != "{}" {
		t.Fatalf("missing response.data must be {} (object), got %s", string(dataRawMissing))
	}
}

// clientConsidersComplete replicates the front-end completion predicate from
// web/src/pages/agent/hooks/use-fetch-pipeline-log.ts: the run is "really
// done" only when the LAST array element has component_id == "END" AND its
// first trace entry carries a non-empty message. If any part of that signal
// is missing, the Log box keeps polling forever.
func clientConsidersComplete(arr []map[string]interface{}) bool {
	if len(arr) == 0 {
		return false
	}
	last := arr[len(arr)-1]
	if last["component_id"] != "END" {
		return false
	}
	trace, ok := last["trace"].([]interface{})
	if !ok || len(trace) == 0 {
		return false
	}
	first, ok := trace[0].(map[string]interface{})
	if !ok {
		return false
	}
	msg, _ := first["message"].(string)
	return strings.TrimSpace(msg) != ""
}

// TestGetAgentLogs_EndSignalCompletion pins the exact signal the front-end
// polls for: the response array must end with an END element whose first
// trace message is non-empty. The handler must preserve that signal
// byte-for-byte through JSON round-tripping.
func TestGetAgentLogs_EndSignalCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := t.Context()

	db := setupHandlerAgentsTestDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	db.Create(&entity.UserCanvas{ID: "c1", UserID: "u1", Title: sptr("Test")})

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)

	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	// End element with a NON-empty message -> client must consider it done.
	goodPayload := `[` +
		`{"component_id":"File","trace":[{"progress":1,"message":"parsed","datetime":"10:00:00","timestamp":1.0,"elapsed_time":0}]},` +
		`{"component_id":"END","trace":[{"progress":1,"message":"run finished","datetime":"10:00:01","timestamp":2.0,"elapsed_time":1.0}]}` +
		`]`
	if err = rdb.Set(ctx, "c1-msg-good-logs", goodPayload, 0).Err(); err != nil {
		t.Fatalf("seed redis: %v", err)
	}

	// End element with an EMPTY message -> client must NOT consider it done
	// (would poll forever). Locks the contract that the debug writer must
	// emit a non-empty END message.
	badPayload := `[` +
		`{"component_id":"File","trace":[{"progress":1,"message":"parsed","datetime":"10:00:00","timestamp":1.0,"elapsed_time":0}]},` +
		`{"component_id":"END","trace":[{"progress":1,"message":"","datetime":"10:00:01","timestamp":2.0,"elapsed_time":1.0}]}` +
		`]`
	if err = rdb.Set(ctx, "c1-msg-bad-logs", badPayload, 0).Err(); err != nil {
		t.Fatalf("seed redis: %v", err)
	}

	h := NewAgentHandler(ctx, service.NewAgentService(), nil).
		WithRedisGetter(func(key string) (string, error) {
			return rdb.Get(ctx, key).Result()
		})

	call := func(messageID string) []map[string]interface{} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/v1/agents/c1/logs/"+messageID, nil)
		c.Set("user", &entity.User{ID: "u1"})
		c.Set("user_id", "u1")
		c.Params = gin.Params{
			{Key: "canvas_id", Value: "c1"},
			{Key: "message_id", Value: messageID},
		}
		h.GetAgentLogs(c)
		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode (msg=%s): %v body=%s", messageID, err, w.Body.String())
		}
		if code, _ := resp["code"].(float64); int(code) != int(common.CodeSuccess) {
			t.Fatalf("code=%v want 0 body=%s", code, mustJSON(resp))
		}
		dataRaw, _ := json.Marshal(resp["data"])
		var arr []map[string]interface{}
		if err := json.Unmarshal(dataRaw, &arr); err != nil {
			t.Fatalf("data not array (msg=%s): %v body=%s", messageID, err, string(dataRaw))
		}
		return arr
	}

	good := call("msg-good")
	if !clientConsidersComplete(good) {
		t.Fatalf("expected client to consider run complete for non-empty END message; arr=%s", mustJSON(good))
	}

	bad := call("msg-bad")
	if clientConsidersComplete(bad) {
		t.Fatalf("client must NOT consider run complete for empty END message (would poll forever); arr=%s", mustJSON(bad))
	}
}

// capturedStore is an in-memory task.DebugLogStore used to assert that
// runCanvasPipelineDebug actually wrote the debug log array under the expected key.
type capturedStore struct {
	mu   sync.Mutex
	data map[string]string
}

func (s *capturedStore) Set(ctx context.Context, key, value string, _ time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = map[string]string{}
	}
	s.data[key] = value
	return true
}

func (s *capturedStore) Get(ctx context.Context, key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[key]
	return v, ok
}

// fakeDebugExecutor is a debugExecutor stand-in for runCanvasPipelineDebug. It
// captures the sink attached via WithProgressSink and replays a couple of
// component lifecycle events on Execute, mirroring what the real
// PipelineExecutor emits through its progress callback.
type fakeDebugExecutor struct {
	capturedSink pipelinepkg.ProgressSink
	result       *task.PipelineResult
	runErr       error
}

func (f *fakeDebugExecutor) WithProgressSink(sink pipelinepkg.ProgressSink) *task.PipelineExecutor {
	f.capturedSink = sink
	return nil // return ignored by runCanvasPipelineDebug
}

func (f *fakeDebugExecutor) Execute(ctx context.Context) (*task.PipelineResult, error) {
	if f.capturedSink != nil {
		f.capturedSink.OnComponentTotal(ctx, "t1", 1)
		f.capturedSink.OnComponentProgress(ctx, pipelinepkg.ProgressEvent{
			TaskID: "t1", Component: "File", Phase: 0, Message: "File Started",
		})
		f.capturedSink.OnComponentProgress(ctx, pipelinepkg.ProgressEvent{
			TaskID: "t1", Component: "File", Phase: 1, Message: "File Done",
		})
	}
	return f.result, f.runErr
}

// TestRespondWithDebugResult_Shape pins the wire contract the front-end reads
// after kicking off a debug run: response.data must carry both message_id (the
// polling key) and chunks (the rendered output). Empty chunks must stay an
// empty array (never null), and a run error must surface as a server-error
// code rather than success.
func TestRespondWithDebugResult_Shape(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Success with chunks: data.message_id + data.chunks both present.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	respondWithDebugResult(c, &task.PipelineResult{
		MessageID: "m-abc",
		Chunks:    []map[string]any{{"content": "hello"}},
	}, nil)
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if code, _ := resp["code"].(float64); int(code) != int(common.CodeSuccess) {
		t.Fatalf("code=%v want 0 body=%s", code, mustJSON(resp))
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data not object: %v", resp)
	}
	if data["message_id"] != "m-abc" {
		t.Fatalf("data.message_id=%v want m-abc", data["message_id"])
	}
	chunks, ok := data["chunks"].([]interface{})
	if !ok {
		t.Fatalf("data.chunks not array: %v", data["chunks"])
	}
	if len(chunks) != 1 {
		t.Fatalf("data.chunks len=%d want 1", len(chunks))
	}

	// Empty chunks: still an object with message_id and an empty array.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	respondWithDebugResult(c2, &task.PipelineResult{MessageID: "m-empty"}, nil)
	var resp2 map[string]interface{}
	if err := json.Unmarshal(w2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("decode empty: %v body=%s", err, w2.Body.String())
	}
	data2, ok := resp2["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data not object: %v", resp2)
	}
	if data2["message_id"] != "m-empty" {
		t.Fatalf("data.message_id=%v want m-empty", data2["message_id"])
	}
	chunks2, ok := data2["chunks"].([]interface{})
	if !ok {
		t.Fatalf("data.chunks not array: %v", data2["chunks"])
	}
	if len(chunks2) != 0 {
		t.Fatalf("data.chunks len=%d want 0", len(chunks2))
	}

	// Error: must NOT be a success code.
	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	respondWithDebugResult(c3, nil, errors.New("boom"))
	var resp3 map[string]interface{}
	if err := json.Unmarshal(w3.Body.Bytes(), &resp3); err != nil {
		t.Fatalf("decode err: %v body=%s", err, w3.Body.String())
	}
	if code, _ := resp3["code"].(float64); int(code) == int(common.CodeSuccess) {
		t.Fatalf("error case must not be success: %v", resp3)
	}
}

// TestRespondWithDebugResult_ErrorCarriesMessageID pins that a run failure
// still surfaces the polling key: the response must keep a non-success code
// (the existing contract forbids masking errors as success) BUT the error
// envelope's data must carry message_id so the front-end can poll the failure
// timeline that DebugLogSink.Flush already wrote to Redis.
func TestRespondWithDebugResult_ErrorCarriesMessageID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	respondWithDebugResult(c, &task.PipelineResult{MessageID: "m-err"}, errors.New("boom"))
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if code, _ := resp["code"].(float64); int(code) == int(common.CodeSuccess) {
		t.Fatalf("error case must not be success: %v", resp)
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("error envelope data must be an object carrying message_id: %v", resp)
	}
	if data["message_id"] != "m-err" {
		t.Fatalf("error envelope data.message_id=%v want m-err", data["message_id"])
	}
}

// TestRunCanvasPipelineDebug_ErrorStillExposesMessageID asserts the full write-side
// wiring when the executor fails: runCanvasPipelineDebug must (a) still return a
// non-nil result carrying MessageID even though exec.Execute returned nil, and
// (b) have already flushed the failure log under "{canvasID}-{messageID}-logs"
// so the front-end can poll that exact key. This is the scenario that broke
// before — the failure log was written but unreachable because message_id was
// dropped on the error path.
func TestRunCanvasPipelineDebug_ErrorStillExposesMessageID(t *testing.T) {
	ctx := t.Context()
	store := &capturedStore{}

	h := NewAgentHandler(ctx, service.NewAgentService(), nil).
		WithRedisStore(store).
		WithNewExecutor(func(taskCtx *task.TaskContext, canvasID string, docBulkSize int) (debugExecutor, error) {
			return &fakeDebugExecutor{
				// Simulate an executor that fails and returns no result.
				result: nil,
				runErr: errors.New("executor blew up"),
			}, nil
		})

	result, err := h.runCanvasPipelineDebug(ctx, &entity.User{ID: "u1"}, "c1", "f.txt", []byte("data"))
	if err == nil {
		t.Fatalf("expected run error")
	}
	if result == nil {
		t.Fatalf("result is nil; front-end would have no polling key on failure")
	}
	if result.MessageID == "" {
		t.Fatalf("result.MessageID empty on failure; front-end cannot poll the failure log")
	}

	// The failure log must be written under the composed key.
	key := "c1-" + result.MessageID + "-logs"
	raw, ok := store.Get(ctx, key)
	if !ok {
		t.Fatalf("failure log not written under key %q; store keys=%v", key, keysOf(store))
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		t.Fatalf("stored failure log not array: %v raw=%s", err, raw)
	}
	if !clientConsidersComplete(arr) {
		t.Fatalf("stored failure log not completion-complete (END last + non-empty msg); arr=%s", raw)
	}
}

// TestRunCanvasPipelineDebug_WiresMessageIDAndLog asserts the full write-side wiring
// of runCanvasPipelineDebug without a live Redis or real canvas: a fake executor
// emits component progress, the injected DebugLogSink flushes the
// [{component_id, trace}] array (with the END marker last) to the captured
// store under the key "{canvasID}-{messageID}-logs", and the returned result
// carries message_id so the front-end can poll that exact key. The array must
// satisfy the completion predicate (END last, non-empty END message) so the
// Log box stops polling.
func TestRunCanvasPipelineDebug_WiresMessageIDAndLog(t *testing.T) {
	ctx := t.Context()
	store := &capturedStore{}

	h := NewAgentHandler(ctx, service.NewAgentService(), nil).
		WithRedisStore(store).
		WithNewExecutor(func(taskCtx *task.TaskContext, canvasID string, docBulkSize int) (debugExecutor, error) {
			return &fakeDebugExecutor{
				result: &task.PipelineResult{
					Chunks: []map[string]any{{"content": "chunk-1"}},
				},
			}, nil
		})

	result, err := h.runCanvasPipelineDebug(ctx, &entity.User{ID: "u1"}, "c1", "f.txt", []byte("data"))
	if err != nil {
		t.Fatalf("runCanvasPipelineDebug: %v", err)
	}
	if result == nil {
		t.Fatalf("result is nil")
	}
	if result.MessageID == "" {
		t.Fatalf("result.MessageID empty; front-end would have no polling key")
	}

	// The log array must be written under the composed key.
	key := "c1-" + result.MessageID + "-logs"
	raw, ok := store.Get(ctx, key)
	if !ok {
		t.Fatalf("log not written under key %q; store keys=%v", key, keysOf(store))
	}

	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		t.Fatalf("stored log not array: %v raw=%s", err, raw)
	}
	if !clientConsidersComplete(arr) {
		t.Fatalf("stored log not completion-complete (END last + non-empty msg); arr=%s", raw)
	}
}

func keysOf(s *capturedStore) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	return keys
}

// miniredisDebugStore adapts a go-redis client to task.DebugLogStore so the
// write path can be exercised against a real (in-memory) Redis in tests. It is
// the production-faithful counterpart of capturedStore: capturedStore proves
// the key shape, miniredisDebugStore proves the bytes survive a real Redis
// round-trip that the read path consumes.
type miniredisDebugStore struct {
	rdb *goredis.Client
}

func (s miniredisDebugStore) Set(ctx context.Context, key, value string, ttl time.Duration) bool {
	if err := s.rdb.Set(ctx, key, value, ttl).Err(); err != nil {
		return false
	}
	return true
}

// TestRunCanvasPipelineDebug_WriteThenReadViaMiniredis locks the write/read contract
// end-to-end: runCanvasPipelineDebug writes the debug log array under
// "{canvasID}-{messageID}-logs" into a real (miniredis) Redis via the injected
// DebugLogStore, and the SAME handler's GetAgentLogs reads it back through the
// injected redisGetter — proving the writer's key shape exactly matches the
// reader's and that the stored bytes round-trip into the [{component_id,
// trace}] array the front-end polls for (File first, END last, non-empty END
// message). This is the only test that would catch a key-format drift between
// the two sides, since the write test uses a capturedStore and the read test
// seeds Redis directly rather than going through the writer.
func TestRunCanvasPipelineDebug_WriteThenReadViaMiniredis(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := t.Context()

	db := setupHandlerAgentsTestDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })
	db.Create(&entity.UserCanvas{ID: "c1", UserID: "u1", Title: sptr("Test")})

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	// Both seams point at the same miniredis: the writer stores via
	// WithRedisStore and the reader fetches via WithRedisGetter, mirroring
	// production where both hit one Redis.
	h := NewAgentHandler(ctx, service.NewAgentService(), nil).
		WithRedisStore(miniredisDebugStore{rdb: rdb}).
		WithRedisGetter(func(key string) (string, error) {
			return rdb.Get(ctx, key).Result()
		}).
		WithNewExecutor(func(taskCtx *task.TaskContext, canvasID string, docBulkSize int) (debugExecutor, error) {
			return &fakeDebugExecutor{
				result: &task.PipelineResult{Chunks: []map[string]any{{"content": "chunk-1"}}},
			}, nil
		})

	// Write side.
	writeResult, werr := h.runCanvasPipelineDebug(ctx, &entity.User{ID: "u1"}, "c1", "f.txt", []byte("data"))
	if werr != nil {
		t.Fatalf("runCanvasPipelineDebug: %v", werr)
	}
	if writeResult == nil || writeResult.MessageID == "" {
		t.Fatalf("runCanvasPipelineDebug returned no message_id")
	}
	messageID := writeResult.MessageID

	// The key must exist in the same miniredis the reader will query.
	if !mr.Exists("c1-" + messageID + "-logs") {
		t.Fatalf("log key c1-%s-logs not present in redis after write", messageID)
	}

	// Read side: same handler, same miniredis, driven through the real
	// GetAgentLogs HTTP path.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/agents/c1/logs/"+messageID, nil)
	c.Set("user", &entity.User{ID: "u1"})
	c.Set("user_id", "u1")
	c.Params = gin.Params{
		{Key: "canvas_id", Value: "c1"},
		{Key: "message_id", Value: messageID},
	}
	h.GetAgentLogs(c)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode read response: %v body=%s", err, w.Body.String())
	}
	if code, _ := resp["code"].(float64); int(code) != int(common.CodeSuccess) {
		t.Fatalf("read code=%v want 0 body=%s", code, mustJSON(resp))
	}
	dataRaw, _ := json.Marshal(resp["data"])
	trimmed := strings.TrimSpace(string(dataRaw))
	if len(trimmed) == 0 || trimmed[0] != '[' {
		t.Fatalf("read response.data must be a JSON array, got %s", string(dataRaw))
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal(dataRaw, &arr); err != nil {
		t.Fatalf("read data not array: %v body=%s", err, string(dataRaw))
	}
	if arr[0]["component_id"] != "File" {
		t.Errorf("first component_id=%v want File", arr[0]["component_id"])
	}
	if arr[len(arr)-1]["component_id"] != "END" {
		t.Errorf("last component_id=%v want END", arr[len(arr)-1]["component_id"])
	}
	if !clientConsidersComplete(arr) {
		t.Errorf("stored log not completion-complete (END last + non-empty msg); arr=%s", string(dataRaw))
	}
}
