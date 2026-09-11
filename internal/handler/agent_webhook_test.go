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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// fakeCanvasLoader is a stand-in canvasLoader for webhook tests. It
// returns the canned canvas/run events configured at construction time
// without touching the database. Mirrors the pattern used by
// waitFakeAgentService at internal/handler/agent_wait_for_user_test.go.
type fakeCanvasLoader struct {
	canvas              *entity.UserCanvas
	err                 error
	runErr              error
	events              []canvas.RunEvent
	waitForCancellation bool
}

func (f *fakeCanvasLoader) LoadCanvasByID(_ context.Context, _, _ string) (*entity.UserCanvas, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.canvas, nil
}

func (f *fakeCanvasLoader) RunAgentWithWebhook(ctx context.Context, _, _ string, _ map[string]any) (<-chan canvas.RunEvent, error) {
	if f.runErr != nil {
		return nil, f.runErr
	}
	if f.waitForCancellation {
		out := make(chan canvas.RunEvent)
		go func() {
			<-ctx.Done()
			close(out)
		}()
		return out, nil
	}
	out := make(chan canvas.RunEvent, len(f.events))
	for _, e := range f.events {
		out <- e
	}
	close(out)
	return out, nil
}

// makeWebhookCanvas builds a minimal canvas with a Begin component
// whose params.mode == "Webhook" and the supplied params map. The
// `params` argument becomes webhook_cfg inside the handler.
//
// As of PR #14890 the webhook requires a security block — empty
// configs are rejected. Tests that don't care about auth inject
// an explicit anonymous-opt-in block (auth_type=none +
// allow_anonymous=true) so the handler proceeds to the
// schema/content-type checks under test.
func makeWebhookCanvas(id, userID, mode string, params map[string]any) *entity.UserCanvas {
	dsl := map[string]any{
		"components": map[string]any{
			"begin": map[string]any{
				"obj": map[string]any{
					"component_name": "Begin",
					"params": map[string]any{
						"mode": mode,
					},
				},
			},
		},
	}
	if params == nil {
		params = map[string]any{}
	}
	if _, ok := params["security"]; !ok {
		params["security"] = map[string]any{
			"auth_type":       "none",
			"allow_anonymous": true,
		}
	}
	for k, v := range params {
		dsl["components"].(map[string]any)["begin"].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)[k] = v
	}
	return &entity.UserCanvas{
		ID:             id,
		UserID:         userID,
		CanvasCategory: "agent_canvas",
		DSL:            entity.JSONMap(dsl),
	}
}

// webhookCtx builds a gin test context with the supplied method/path/body
// and a pre-set "user" so GetUser() returns success.
func webhookCtx(method, path, body, contentType string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	c.Request = httptest.NewRequest(method, path, reader)
	if contentType != "" {
		c.Request.Header.Set("Content-Type", contentType)
	}
	c.Set("user", &entity.User{ID: "u-1"})
	return c, w
}

// errBody extracts {code, message} from a 102 envelope response.
func errBody(t *testing.T, body []byte) (int, string) {
	t.Helper()
	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, body)
	}
	return env.Code, env.Message
}

// ---------- Phase 1: canvas loading / DataFlow rejection ----------

// TestWebhook_RejectsUnknownCanvas pins the 102 "Canvas not found."
// envelope when LoadCanvasByID returns ErrUserCanvasNotFound. This is
// the deliberate divergence from mapAgentError (which would surface 103).
func TestWebhook_RejectsUnknownCanvas(t *testing.T) {
	h := &AgentHandler{loader: &fakeCanvasLoader{err: dao.ErrUserCanvasNotFound}}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", `{}`, "application/json")

	h.Webhook(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != int(common.CodeDataError) {
		t.Errorf("code = %d, want %d (DataError)", code, common.CodeDataError)
	}
	if msg != "Canvas not found." {
		t.Errorf("message = %q, want %q", msg, "Canvas not found.")
	}
}

// TestWebhook_DataFlowRejected pins the second guard: a canvas whose category
// is DataFlow must NOT be triggered by an external webhook. The handler returns
// CodeDataError with "Dataflow can not be triggered by webhook." — mirroring
// Python agent_api.py:1786. DataFlow canvases are ingestion pipelines, not
// interactive agents, and have no chat/debug surface over the webhook entry.
func TestWebhook_DataFlowRejected(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", nil)
	cv.CanvasCategory = "dataflow_canvas"
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", `{}`, "application/json")

	h.Webhook(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != int(common.CodeDataError) {
		t.Errorf("code = %d, want %d (CodeDataError)", code, common.CodeDataError)
	}
	if msg != "Dataflow can not be triggered by webhook." {
		t.Errorf("message = %q, want %q", msg, "Dataflow can not be triggered by webhook.")
	}
}

// TestWebhook_RejectsMissingWebhookConfig: DSL has no Begin with
// mode="Webhook".
func TestWebhook_RejectsMissingWebhookConfig(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Manual", nil) // wrong mode
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", `{}`, "application/json")

	h.Webhook(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != int(common.CodeDataError) {
		t.Errorf("code = %d, want %d", code, common.CodeDataError)
	}
	if msg != "Webhook not configured for this agent." {
		t.Errorf("message = %q", msg)
	}
}

// ---------- Phase 5: method gate ----------

// TestWebhook_RejectsDisallowedMethod covers the methods list gate.
// methods:["POST"] + a GET request → 102.
func TestWebhook_RejectsDisallowedMethod(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"methods": []any{"POST"},
	})
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}
	c, w := webhookCtx("GET", "/api/v1/agents/c1/webhook", ``, "")

	h.Webhook(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != int(common.CodeDataError) {
		t.Errorf("code = %d, want %d", code, common.CodeDataError)
	}
	want := "HTTP method 'GET' not allowed for this webhook."
	if msg != want {
		t.Errorf("message = %q, want %q", msg, want)
	}
}

// ---------- Phase 6: security ----------

// TestWebhook_TokenAuthPasses pins the happy path for token auth.
func TestWebhook_TokenAuthPasses(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"security": map[string]any{
			"auth_type": "token",
			"token": map[string]any{
				"token_header": "X-Webhook-Token",
				"token_value":  "s3cret",
			},
		},
		"execution_mode": "Immediately",
		"response": map[string]any{
			"status": 200,
		},
	})
	loader := &fakeCanvasLoader{canvas: cv}
	h := &AgentHandler{loader: loader}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", `{}`, "application/json")
	c.Request.Header.Set("X-Webhook-Token", "s3cret")

	h.Webhook(c)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

// TestWebhook_TokenAuthFails: header value mismatch → 102.
func TestWebhook_TokenAuthFails(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"security": map[string]any{
			"auth_type": "token",
			"token": map[string]any{
				"token_header": "X-Webhook-Token",
				"token_value":  "s3cret",
			},
		},
	})
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", `{}`, "application/json")
	c.Request.Header.Set("X-Webhook-Token", "wrong")

	h.Webhook(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != int(common.CodeDataError) {
		t.Errorf("code = %d, want %d", code, common.CodeDataError)
	}
	if msg != "invalid token authentication" {
		t.Errorf("message = %q, want %q", msg, "invalid token authentication")
	}
}

// TestWebhook_BodySizeLimit: 1kb ceiling + ~2kb Content-Length → 102.
func TestWebhook_BodySizeLimit(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"security": map[string]any{
			"max_body_size": "1kb",
		},
	})
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook",
		strings.Repeat("a", 2048), "text/plain")

	h.Webhook(c)

	code, _ := errBody(t, w.Body.Bytes())
	if code != int(common.CodeDataError) {
		t.Errorf("code = %d, want %d", code, common.CodeDataError)
	}
}

// TestWebhook_BodySizeLimitTooLarge rejects config bugs >10MB.
func TestWebhook_BodySizeLimitTooLarge(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"security": map[string]any{
			"max_body_size": "11mb",
		},
	})
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", `{}`, "application/json")

	h.Webhook(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != int(common.CodeDataError) {
		t.Errorf("code = %d, want %d", code, common.CodeDataError)
	}
	if !strings.Contains(msg, "exceeds maximum") {
		t.Errorf("message = %q, want contains 'exceeds maximum'", msg)
	}
}

// TestWebhook_BasicAuthPasses / TestWebhook_BasicAuthFails exercise the
// HTTP Basic branch.
func TestWebhook_BasicAuthPasses(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"security": map[string]any{
			"auth_type": "basic",
			"basic_auth": map[string]any{
				"username": "alice",
				"password": "wonderland",
			},
		},
		"execution_mode": "Immediately",
		"response":       map[string]any{"status": 200},
	})
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", `{}`, "application/json")
	c.Request.SetBasicAuth("alice", "wonderland")

	h.Webhook(c)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestWebhook_BasicAuthFails(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"security": map[string]any{
			"auth_type": "basic",
			"basic_auth": map[string]any{
				"username": "alice",
				"password": "wonderland",
			},
		},
	})
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", `{}`, "application/json")
	c.Request.SetBasicAuth("alice", "wrong")

	h.Webhook(c)

	code, _ := errBody(t, w.Body.Bytes())
	if code != int(common.CodeDataError) {
		t.Errorf("code = %d, want %d", code, common.CodeDataError)
	}
}

// ---------- Phase 8: schema extraction ----------

// TestWebhook_SchemaExtractionRequired: missing required field → 102.
func TestWebhook_SchemaExtractionRequired(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"schema": map[string]any{
			"query": map[string]any{
				"properties": map[string]any{
					"q": map[string]any{"type": "string"},
				},
				"required": []any{"q"},
			},
		},
		"execution_mode": "Immediately",
		"response":       map[string]any{"status": 200},
	})
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook?other=v", ``, "")

	h.Webhook(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != int(common.CodeDataError) {
		t.Errorf("code = %d, want %d", code, common.CodeDataError)
	}
	if !strings.Contains(msg, "missing required field") {
		t.Errorf("message = %q, want contains 'missing required field'", msg)
	}
}

// TestWebhook_SchemaExtractionTypeMismatch: non-numeric in number field.
func TestWebhook_SchemaExtractionTypeMismatch(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"schema": map[string]any{
			"body": map[string]any{
				"properties": map[string]any{
					"n": map[string]any{"type": "number"},
				},
				"required": []any{"n"},
			},
		},
		"execution_mode": "Immediately",
		"response":       map[string]any{"status": 200},
	})
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", `{"n":"abc"}`, "application/json")

	h.Webhook(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != int(common.CodeDataError) {
		t.Errorf("code = %d, want %d", code, common.CodeDataError)
	}
	if !strings.Contains(msg, "type mismatch") && !strings.Contains(msg, "auto-cast") {
		t.Errorf("message = %q, want contains 'type mismatch' or 'auto-cast'", msg)
	}
}

// ---------- Phase 9: dispatch ----------

// TestWebhook_ImmediatelyReturnsConfiguredStatus confirms the
// Immediately mode returns the configured status code and runs the
// canvas detached.
func TestWebhook_ImmediatelyReturnsConfiguredStatus(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"execution_mode": "Immediately",
		"response": map[string]any{
			"status":        201,
			"body_template": `{"ok":true}`,
		},
	})
	loader := &fakeCanvasLoader{
		canvas: cv,
		events: []canvas.RunEvent{
			{Type: "message", Data: `{"content":"hi"}`},
			{Type: "done", Data: ""},
		},
	}
	h := &AgentHandler{loader: loader}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", `{}`, "application/json")

	h.Webhook(c)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Errorf("body = %q, want contains '\"ok\":true'", w.Body.String())
	}
}

// TestWebhook_DefaultModeAggregatesContent covers the non-Immediately
// path: streaming message + message_end events get concatenated.
func TestWebhook_DefaultModeAggregatesContent(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"execution_mode": "Streaming",
	})
	loader := &fakeCanvasLoader{
		canvas: cv,
		events: []canvas.RunEvent{
			{Type: "message", Data: `{"content":"hello "}`},
			{Type: "message", Data: `{"content":"world"}`},
			{Type: "message_end", Data: `{"status":200}`},
			{Type: "done", Data: ""},
		},
	}
	h := &AgentHandler{loader: loader}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", `{}`, "application/json")

	h.Webhook(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Message != "hello world" {
		t.Errorf("aggregated message = %q, want %q", body.Message, "hello world")
	}
	if body.Code != 200 {
		t.Errorf("code = %d, want 200", body.Code)
	}
}

func TestWebhook_DefaultModePropagatesTerminalEvents(t *testing.T) {
	tests := []struct {
		name        string
		event       canvas.RunEvent
		wantStatus  int
		wantMessage string
		forbidden   string
	}{
		{
			name:        "public error",
			event:       canvas.RunEvent{Type: "error", Data: `{"message":"retrieval backend failed"}`},
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "retrieval backend failed",
		},
		{
			name:        "internal error",
			event:       canvas.RunEvent{Type: "error", Data: `{"message":"postgres password=secret","kind":"internal"}`},
			wantStatus:  http.StatusInternalServerError,
			wantMessage: canvas.InternalRunErrorMessage,
			forbidden:   "password=secret",
		},
		{
			name:        "cancelled",
			event:       canvas.RunEvent{Type: "cancelled", Data: `{"message":"Agent run was cancelled."}`},
			wantStatus:  http.StatusConflict,
			wantMessage: "Agent run was cancelled.",
		},
		{
			name:        "waiting for user",
			event:       canvas.RunEvent{Type: "waiting_for_user", Data: `{"cpn_id":"input-1","tips":"Please choose an option."}`},
			wantStatus:  http.StatusConflict,
			wantMessage: "Please choose an option.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
				"execution_mode": "Streaming",
			})
			h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv, events: []canvas.RunEvent{tt.event}}}
			var trace []canvas.RunEvent
			h.webhookTraceAppender = func(_ context.Context, _ string, _ time.Time, ev canvas.RunEvent) {
				trace = append(trace, ev)
			}
			c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook/test", `{}`, "application/json")

			h.Webhook(c)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tt.wantStatus, w.Body.String())
			}
			var body struct {
				Message string `json:"message"`
				Success bool   `json:"success"`
				Code    int    `json:"code"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Success {
				t.Fatal("terminal event must not be reported as success")
			}
			if body.Code != tt.wantStatus {
				t.Errorf("code = %d, want %d", body.Code, tt.wantStatus)
			}
			if !strings.Contains(body.Message, tt.wantMessage) {
				t.Errorf("message = %q, want contains %q", body.Message, tt.wantMessage)
			}
			if tt.forbidden != "" && strings.Contains(w.Body.String(), tt.forbidden) {
				t.Fatalf("response leaked internal error details: %s", w.Body.String())
			}
			if tt.forbidden != "" {
				for _, ev := range trace {
					if strings.Contains(ev.Data, tt.forbidden) {
						t.Fatalf("trace leaked internal error details: %+v", trace)
					}
				}
			}
			assertWebhookFinishedTrace(t, trace, false)
		})
	}
}

func TestWebhook_DefaultModeRedactsStartError(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"execution_mode": "Streaming",
	})
	startErr := errors.Join(errors.New("dial mysql password=secret"), service.ErrAgentStorageError)
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv, runErr: startErr}}
	var trace []canvas.RunEvent
	h.webhookTraceAppender = func(_ context.Context, _ string, _ time.Time, ev canvas.RunEvent) {
		trace = append(trace, ev)
	}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook/test", `{}`, "application/json")

	h.Webhook(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), canvas.InternalRunErrorMessage) {
		t.Errorf("body missing safe internal error message: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "password=secret") {
		t.Fatalf("response leaked internal error details: %s", w.Body.String())
	}
	if len(trace) == 0 || strings.Contains(trace[0].Data, "password=secret") {
		t.Fatalf("trace leaked internal error details: %+v", trace)
	}
	assertWebhookFinishedTrace(t, trace, false)
}

func TestRunWebhookDetachedAppendsFinishedTrace(t *testing.T) {
	tests := []struct {
		name        string
		events      []canvas.RunEvent
		runErr      error
		wantSuccess bool
		forbidden   string
	}{
		{
			name: "success",
			events: []canvas.RunEvent{
				{Type: "message", Data: `{"content":"done"}`},
				{Type: "done"},
			},
			wantSuccess: true,
		},
		{
			name:        "runner error",
			events:      []canvas.RunEvent{{Type: "error", Data: `{"message":"LLM failed"}`}},
			wantSuccess: false,
		},
		{
			name:        "start error",
			runErr:      errors.Join(errors.New("redis password=secret"), service.ErrAgentStorageError),
			wantSuccess: false,
			forbidden:   "password=secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cv := makeWebhookCanvas("c1", "u-1", "Webhook", nil)
			h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv, events: tt.events, runErr: tt.runErr}}
			var trace []canvas.RunEvent
			h.webhookTraceAppender = func(_ context.Context, _ string, _ time.Time, ev canvas.RunEvent) {
				trace = append(trace, ev)
			}

			h.runWebhookDetached(t.Context(), cv, map[string]any{}, true, time.Now())

			assertWebhookFinishedTrace(t, trace, tt.wantSuccess)
			if tt.forbidden != "" {
				for _, ev := range trace {
					if strings.Contains(ev.Data, tt.forbidden) {
						t.Fatalf("trace leaked internal error details: %+v", trace)
					}
				}
			}
		})
	}
}

func TestRunWebhookDetachedRecordsTimeoutTrace(t *testing.T) {
	tests := []struct {
		name   string
		loader *fakeCanvasLoader
	}{
		{name: "during start", loader: &fakeCanvasLoader{runErr: context.DeadlineExceeded}},
		{name: "during run", loader: &fakeCanvasLoader{waitForCancellation: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cv := makeWebhookCanvas("c1", "u-1", "Webhook", nil)
			tt.loader.canvas = cv
			h := &AgentHandler{loader: tt.loader}
			var trace []canvas.RunEvent
			h.webhookTraceAppender = func(ctx context.Context, _ string, _ time.Time, ev canvas.RunEvent) {
				if err := ctx.Err(); err != nil {
					t.Errorf("trace context error = %v, want nil", err)
				}
				trace = append(trace, ev)
			}

			ctx, cancel := context.WithTimeout(
				service.WithAgentSessionID(t.Context(), "session-1"),
				time.Nanosecond,
			)
			defer cancel()
			<-ctx.Done()
			h.runWebhookDetachedWithContext(ctx, cv, map[string]any{}, true, time.Now(), "session-1")

			if len(trace) < 2 || trace[0].Type != "error" || !strings.Contains(trace[0].Data, "timed out") {
				t.Fatalf("timeout trace = %+v, want error followed by failed finished event", trace)
			}
			assertWebhookFinishedTrace(t, trace, false)
		})
	}
}

func TestRunWebhookSyncRecordsCancelledTrace(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", nil)
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv, waitForCancellation: true}}
	var trace []canvas.RunEvent
	h.webhookTraceAppender = func(ctx context.Context, _ string, _ time.Time, ev canvas.RunEvent) {
		if err := ctx.Err(); err != nil {
			t.Errorf("trace context error = %v, want nil", err)
		}
		trace = append(trace, ev)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result := h.runWebhookSync(ctx, cv, map[string]any{}, true, time.Now())

	if result.status != http.StatusConflict {
		t.Errorf("status = %d, want %d", result.status, http.StatusConflict)
	}
	if len(trace) < 2 || trace[0].Type != "cancelled" || !strings.Contains(trace[0].Data, "cancelled") {
		t.Fatalf("cancelled trace = %+v, want cancelled followed by failed finished event", trace)
	}
	assertWebhookFinishedTrace(t, trace, false)
}

func TestWebhookBufferedTerminalEventAfterCancellationUsesCleanupContext(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T, *AgentHandler, context.Context, *entity.UserCanvas)
	}{
		{
			name: "detached",
			run: func(_ *testing.T, h *AgentHandler, ctx context.Context, cv *entity.UserCanvas) {
				h.runWebhookDetachedWithContext(ctx, cv, map[string]any{}, true, time.Now(), "session-1")
			},
		},
		{
			name: "sync",
			run: func(t *testing.T, h *AgentHandler, ctx context.Context, cv *entity.UserCanvas) {
				result := h.runWebhookSync(ctx, cv, map[string]any{}, true, time.Now())
				if result.status != http.StatusConflict {
					t.Errorf("status = %d, want %d", result.status, http.StatusConflict)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cv := makeWebhookCanvas("c1", "u-1", "Webhook", nil)
			loader := &fakeCanvasLoader{
				canvas: cv,
				events: []canvas.RunEvent{{
					Type: "cancelled",
					Data: `{"message":"Agent run was cancelled."}`,
				}},
			}
			h := &AgentHandler{loader: loader}
			var trace []canvas.RunEvent
			h.webhookTraceAppender = func(ctx context.Context, _ string, _ time.Time, ev canvas.RunEvent) {
				if err := ctx.Err(); err != nil {
					t.Errorf("trace context error = %v, want nil", err)
				}
				trace = append(trace, ev)
			}

			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			tt.run(t, h, ctx, cv)

			if len(trace) < 2 || trace[0].Type != "cancelled" {
				t.Fatalf("cancelled trace = %+v, want cancelled followed by failed finished event", trace)
			}
			assertWebhookFinishedTrace(t, trace, false)
		})
	}
}

func assertWebhookFinishedTrace(t *testing.T, events []canvas.RunEvent, wantSuccess bool) {
	t.Helper()
	if len(events) == 0 || events[len(events)-1].Type != "finished" {
		t.Fatalf("trace must end with finished; trace=%+v", events)
	}
	finishedCount := 0
	for _, ev := range events {
		if ev.Type != "finished" {
			continue
		}
		finishedCount++
		var data struct {
			Success bool `json:"success"`
		}
		if err := json.Unmarshal([]byte(ev.Data), &data); err != nil {
			t.Fatalf("decode finished trace: %v; data=%q", err, ev.Data)
		}
		if data.Success != wantSuccess {
			t.Errorf("finished success = %v, want %v", data.Success, wantSuccess)
		}
	}
	if finishedCount != 1 {
		t.Fatalf("finished event count = %d, want 1; trace=%+v", finishedCount, events)
	}
}

// TestWebhook_InvalidResponseStatusRange rejects bad config (status 500).
func TestWebhook_InvalidResponseStatusRange(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"execution_mode": "Immediately",
		"response":       map[string]any{"status": 500},
	})
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", `{}`, "application/json")

	h.Webhook(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != int(common.CodeDataError) {
		t.Errorf("code = %d, want %d", code, common.CodeDataError)
	}
	if !strings.Contains(msg, "must be between 200 and 399") {
		t.Errorf("message = %q, want contains 'must be between 200 and 399'", msg)
	}
}

// TestWebhook_FindWebhookBegin_NilDSL ensures the helper is defensive.
func TestWebhook_FindWebhookBegin_NilDSL(t *testing.T) {
	if got := findWebhookBegin(nil); got != nil {
		t.Errorf("findWebhookBegin(nil) = %v, want nil", got)
	}
}

// TestWebhook_FindWebhookBegin_NoComponents covers DSL with no components.
func TestWebhook_FindWebhookBegin_NoComponents(t *testing.T) {
	if got := findWebhookBegin(map[string]any{}); got != nil {
		t.Errorf("findWebhookBegin(empty) = %v, want nil", got)
	}
}

// ---------- Regression tests for issues raised in code review ----------

// TestWebhook_BodySizeStreamBounded pins the security MEDIUM-1
// hardening: when max_body_size is configured, an oversized body is
// rejected. The 102 envelope carries code=CodeDataError; HTTP itself
// stays at 200 (matches the existing /api/v1 error envelope shape).
func TestWebhook_BodySizeStreamBounded(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"security": map[string]any{
			"max_body_size": "1kb",
		},
	})
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook",
		strings.Repeat("a", 2048), "text/plain")

	h.Webhook(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != int(common.CodeDataError) {
		t.Errorf("envelope code = %d, want %d (CodeDataError)", code, common.CodeDataError)
	}
	// Either message is acceptable — the Content-Length pre-check
	// ("request body too large") OR the MaxBytesReader runtime check
	// ("http: request body too large").
	if !strings.Contains(msg, "body too large") {
		t.Errorf("message = %q, want contains 'body too large'", msg)
	}
}

//
// These three tests cover the behaviour the implementation summary
// claimed but did NOT actually enforce. They ensure the dispatch path
// returns the documented envelope for multipart and content-type
// mismatch cases.

// TestWebhook_MultipartReturns501 pins the contract that
// multipart/form-data uploads are refused with HTTP 501 BEFORE the
// schema-extraction phase runs. Before the fix, the handler silently
// stuffed __multipart_unsupported__ into body and continued; operators
// saw a confusing schema-validation error instead of the documented
// "not implemented" envelope.
func TestWebhook_MultipartReturns501(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"execution_mode": "Immediately",
		"response":       map[string]any{"status": 200},
	})
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook",
		"--boundary\r\nContent-Disposition: form-data; name=\"k\"\r\n\r\nv\r\n--boundary--\r\n",
		"multipart/form-data; boundary=boundary")

	h.Webhook(c)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "not supported") {
		t.Errorf("body should mention 'not supported', got %q", w.Body.String())
	}
}

// TestWebhook_ContentTypeMismatchReturns102 pins the contract that
// content_types whitelist enforces a HARD REJECT (HTTP 102 envelope),
// matching python agent_api.py:1839-1842. Before the fix, the
// mismatch was silently recorded as __content_type_mismatch__ in the
// body and the request flowed into schema validation — a real
// whitelist bypass.
func TestWebhook_ContentTypeMismatchReturns102(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"content_types":  "application/json",
		"execution_mode": "Immediately",
		"response":       map[string]any{"status": 200},
	})
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}
	// text/plain where the config demands application/json.
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", "raw body", "text/plain")

	h.Webhook(c)

	if w.Code != http.StatusOK {
		t.Fatalf("envelope status = %d, want 200 (the envelope itself succeeds; the code field carries 102)", w.Code)
	}
	code, msg := errBody(t, w.Body.Bytes())
	if code != int(common.CodeDataError) {
		t.Errorf("envelope code = %d, want %d (CodeDataError)", code, common.CodeDataError)
	}
	if !strings.Contains(msg, "invalid content-type") || !strings.Contains(msg, "application/json") {
		t.Errorf("message = %q, want contains 'invalid content-type' and 'application/json'", msg)
	}
}

// TestWebhook_ContentTypesEmptyAllowsAnything confirms the inverse:
// when webhook_cfg does NOT set content_types, any Content-Type is
// accepted (matches python: agent_api.py:1839 only raises when the
// config actually sets content_types).
func TestWebhook_ContentTypesEmptyAllowsAnything(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		// content_types deliberately omitted
		"execution_mode": "Immediately",
		"response":       map[string]any{"status": 200},
	})
	loader := &fakeCanvasLoader{canvas: cv}
	h := &AgentHandler{loader: loader}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", `{"k":"v"}`, "text/plain")

	h.Webhook(c)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

// TestWebhook_ContentTypeMissingHeaderRejected pins the gap from the
// review: when content_types is configured, a request that OMITS the
// Content-Type header entirely must be rejected with 102. The python
// reference has a `if ctype and ...` short-circuit that lets such
// requests through — a real whitelist bypass. The Go port tightens
// this: an operator who configured content_types gets an enforceable
// contract; the caller MUST send the header.
func TestWebhook_ContentTypeMissingHeaderRejected(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"content_types":  "application/json",
		"execution_mode": "Immediately",
		"response":       map[string]any{"status": 200},
	})
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", `{"k":"v"}`, "")
	// ↑ "" content-type means the header is absent. gin will not
	// produce a default Content-Type on a POST so this is a clean
	// "no header" test case.

	h.Webhook(c)

	if w.Code != http.StatusOK {
		t.Fatalf("envelope status = %d, want 200 (the envelope itself succeeds; the code field carries 102)", w.Code)
	}
	code, msg := errBody(t, w.Body.Bytes())
	if code != int(common.CodeDataError) {
		t.Errorf("envelope code = %d, want %d (CodeDataError)", code, common.CodeDataError)
	}
	if !strings.Contains(msg, "invalid content-type") {
		t.Errorf("message = %q, want contains 'invalid content-type'", msg)
	}
	if !strings.Contains(msg, "application/json") {
		t.Errorf("message = %q, want contains 'application/json'", msg)
	}
}

// (No helper needed at the bottom of this file; helper functions
// inline above.)
// _unused previously lived here as a placeholder; deleted during
// cleanup (code-review MEDIUM-2).
