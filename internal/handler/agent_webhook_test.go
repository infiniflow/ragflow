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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"github.com/gin-gonic/gin"

	"ragflow/internal/agent/canvas"
	dslpkg "ragflow/internal/agent/dsl"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
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
	loadUserID          string
	runUserID           string
	runCanvasID         string
	runContext          context.Context
	runPayload          map[string]any
	runStarted          chan string
	runCalls            atomic.Int32
}

type fakeAgentWebhookCredentialManager struct {
	credential   entity.AgentWebhookCredential
	token        string
	resolveErr   error
	issueToken   string
	issueErr     error
	resolveID    string
	resolveToken string
	revokedID    string
	revokedUser  string
}

func (f *fakeAgentWebhookCredentialManager) Issue(_ context.Context, _ permission.Subject, _ *entity.UserCanvas) (string, error) {
	return f.issueToken, f.issueErr
}

func (f *fakeAgentWebhookCredentialManager) Resolve(_ context.Context, canvasID, token string) (*entity.AgentWebhookCredential, error) {
	f.resolveID = canvasID
	f.resolveToken = token
	if f.resolveErr != nil || token != f.token {
		return nil, service.ErrInvalidAgentWebhookCredential
	}
	credential := f.credential
	credential.CanvasID = canvasID
	return &credential, nil
}

func (f *fakeAgentWebhookCredentialManager) Revoke(_ context.Context, canvasID, userID string) error {
	f.revokedID = canvasID
	f.revokedUser = userID
	return nil
}

func (f *fakeCanvasLoader) LoadCanvasByID(_ context.Context, userID, _ string) (*entity.UserCanvas, error) {
	f.loadUserID = userID
	if f.err != nil {
		return nil, f.err
	}
	return f.canvas, nil
}

func (f *fakeCanvasLoader) RunAgentWithWebhook(ctx context.Context, userID, canvasID string, payload map[string]any) (<-chan canvas.RunEvent, error) {
	f.runCalls.Add(1)
	f.runUserID = userID
	f.runCanvasID = canvasID
	f.runContext = ctx
	f.runPayload = payload
	if f.runStarted != nil {
		f.runStarted <- userID
	}
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

// webhookCtx builds a request carrying a test webhook credential and a user
// for the authenticated /webhook/test route.
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
	query := c.Request.URL.Query()
	query.Set("credential", "test-capability")
	c.Request.URL.RawQuery = query.Encode()
	pathParts := strings.Split(strings.Trim(path, "/"), "/")
	for i, part := range pathParts {
		if part == "agents" && i+1 < len(pathParts) {
			c.Params = gin.Params{{Key: "canvas_id", Value: pathParts[i+1]}}
			break
		}
	}
	if contentType != "" {
		c.Request.Header.Set("Content-Type", contentType)
	}
	c.Set("user", &entity.User{ID: "u-1"})
	return c, w
}

func newWebhookTestAgentHandler(loader canvasLoader) *AgentHandler {
	return &AgentHandler{
		loader: loader,
		webhookCredentials: &fakeAgentWebhookCredentialManager{
			credential: entity.AgentWebhookCredential{UserID: "u-1", TenantID: "u-1"},
			token:      "test-capability",
		},
	}
}

func TestPublicWebhookRunsAsCredentialIdentityWithoutUserToken(t *testing.T) {
	tenantID := "tenant-1"
	canvas := makeWebhookCanvas("c1", "canvas-owner", "Webhook", map[string]any{
		"execution_mode": "Streaming",
		"schema": map[string]any{"query": map[string]any{
			"properties": map[string]any{"value": map[string]any{"type": "string"}},
		}},
	})
	canvas.TenantID = &tenantID
	loader := &fakeCanvasLoader{canvas: canvas, runStarted: make(chan string, 1)}
	credentials := &fakeAgentWebhookCredentialManager{
		credential: entity.AgentWebhookCredential{UserID: "webhook-caller", TenantID: tenantID},
		token:      "external-capability",
	}
	h := &AgentHandler{loader: loader, webhookCredentials: credentials}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", `{}`, "application/json")
	c.Delete("user")
	c.Request.URL.RawQuery = "credential=external-capability&value=visible"

	h.Webhook(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s; want 200", w.Code, w.Body.String())
	}
	if credentials.resolveID != "c1" || credentials.resolveToken != "external-capability" {
		t.Fatalf("credential lookup = (%q, %q), want path canvas and URL credential", credentials.resolveID, credentials.resolveToken)
	}
	if loader.loadUserID != "webhook-caller" {
		t.Fatalf("canvas loaded as %q, want bound webhook caller", loader.loadUserID)
	}
	select {
	case userID := <-loader.runStarted:
		if userID != "webhook-caller" {
			t.Fatalf("agent ran as %q, want bound webhook caller", userID)
		}
	case <-time.After(time.Second):
		t.Fatal("webhook did not start the agent")
	}
	if subject, ok := permission.SubjectFromContext(loader.runContext); !ok || subject.TenantID != tenantID {
		t.Fatalf("run tenant = %q, want %q", subject.TenantID, tenantID)
	}
	input := loader.runPayload["input"].(map[string]any)
	query := input["query"].(map[string]any)
	if _, leaked := query["credential"]; leaked {
		t.Fatalf("webhook credential leaked into Agent query input: %#v", query)
	}
	if query["value"] != "visible" {
		t.Fatalf("webhook query input = %#v, want the non-auth query field", query)
	}
}

func TestPublicWebhookRejectsMissingOrInvalidCredential(t *testing.T) {
	for _, token := range []string{"", "wrong-capability"} {
		t.Run("token="+token, func(t *testing.T) {
			loader := &fakeCanvasLoader{canvas: makeWebhookCanvas("c1", "canvas-owner", "Webhook", nil)}
			credentials := &fakeAgentWebhookCredentialManager{token: "valid-capability"}
			h := &AgentHandler{loader: loader, webhookCredentials: credentials}
			c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", `{}`, "application/json")
			c.Delete("user")
			c.Request.Header.Set("Authorization", "Bearer user-api-key")
			if token != "" {
				c.Request.URL.RawQuery = "credential=" + token
			}

			h.Webhook(c)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
			}
			if loader.runCalls.Load() != 0 || loader.loadUserID != "" {
				t.Fatalf("invalid credential reached canvas load/run: user=%q runs=%d", loader.loadUserID, loader.runCalls.Load())
			}
		})
	}
}

func TestWebhookCredentialManagementReturnsOneTimeURLAndRevokesCurrentUser(t *testing.T) {
	manager := &fakeAgentWebhookCredentialManager{issueToken: "issued-capability"}
	h := &AgentHandler{
		loader:             &fakeCanvasLoader{canvas: makeWebhookCanvas("c1", "u-1", "Webhook", nil)},
		webhookCredentials: manager,
	}
	c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook/credentials", "", "")
	h.CreateWebhookCredential(c)
	var response struct {
		Code common.ErrorCode `json:"code"`
		Data struct {
			Credential string `json:"credential"`
			WebhookURL string `json:"webhook_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode credential response: %v; body=%s", err, w.Body.String())
	}
	if w.Code != http.StatusOK || response.Code != common.CodeSuccess {
		t.Fatalf("create status=%d code=%d body=%s", w.Code, response.Code, w.Body.String())
	}
	if response.Data.Credential != manager.issueToken || !strings.Contains(response.Data.WebhookURL, "credential=issued-capability") {
		t.Fatalf("credential response = %+v, want one-time credential URL", response.Data)
	}

	c, w = webhookCtx("DELETE", "/api/v1/agents/c1/webhook/credentials", "", "")
	h.RevokeWebhookCredential(c)
	if w.Code != http.StatusOK || manager.revokedID != "c1" || manager.revokedUser != "u-1" {
		t.Fatalf("revoke status=%d identity=(%q,%q), body=%s", w.Code, manager.revokedID, manager.revokedUser, w.Body.String())
	}
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

// TestWebhookHeaderSchemaAcceptsMixedCase verifies real request parsing and schema validation before dispatch.
func TestWebhookHeaderSchemaAcceptsMixedCase(t *testing.T) {
	for _, path := range []string{"/api/v1/agents/c1/webhook", "/api/v1/agents/c1/webhook/test"} {
		t.Run(path, func(t *testing.T) {
			cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
				"execution_mode": "Streaming",
				"schema": map[string]any{"headers": map[string]any{
					"properties": map[string]any{"x-request-id": map[string]any{"type": "string"}},
					"required":   []string{"x-request-id"},
				}},
			})
			loader := &fakeCanvasLoader{canvas: cv}
			h := newWebhookTestAgentHandler(loader)
			h.webhookTraceAppender = func(context.Context, string, time.Time, canvas.RunEvent) {}
			c, w := webhookCtx("POST", path, `{}`, "application/json")
			c.Request.Header.Set("X-Request-ID", "request-123")
			h.Webhook(c)
			if w.Code != http.StatusOK || loader.runCalls.Load() != 1 || !strings.Contains(w.Body.String(), `"success":true`) {
				t.Fatalf("status=%d runs=%d response=%s", w.Code, loader.runCalls.Load(), w.Body.String())
			}
		})
	}
}

func TestWebhookRunsAsCredentialCaller(t *testing.T) {
	for _, executionMode := range []string{"Streaming", "Immediately"} {
		t.Run(executionMode, func(t *testing.T) {
			cv := makeWebhookCanvas("c1", "canvas-owner", "Webhook", map[string]any{
				"execution_mode": executionMode,
			})
			loader := &fakeCanvasLoader{canvas: cv, runStarted: make(chan string, 1)}
			h := newWebhookTestAgentHandler(loader)
			h.webhookCredentials = &fakeAgentWebhookCredentialManager{
				credential: entity.AgentWebhookCredential{UserID: "webhook-caller", TenantID: "canvas-owner"},
				token:      "test-capability",
			}
			c, w := webhookCtx("POST", "/api/v1/agents/c1/webhook", `{}`, "application/json")
			c.Delete("user")

			h.Webhook(c)

			if loader.loadUserID != "webhook-caller" {
				t.Fatalf("canvas loaded as %q, want credential-bound caller", loader.loadUserID)
			}
			select {
			case runUserID := <-loader.runStarted:
				if runUserID != "webhook-caller" {
					t.Fatalf("agent ran as %q, want credential-bound caller", runUserID)
				}
			case <-time.After(time.Second):
				t.Fatal("webhook did not start the agent")
			}
			if loader.runCanvasID != cv.ID {
				t.Fatalf("agent ran canvas %q, want %q", loader.runCanvasID, cv.ID)
			}
			if executionMode == "Streaming" && w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
			}
		})
	}
}

// TestWebhook_RejectsUnknownCanvas pins the 102 "Canvas not found."
// envelope when LoadCanvasByID returns ErrUserCanvasNotFound. This is
// the deliberate divergence from mapAgentError (which would surface 103).
func TestWebhook_RejectsUnknownCanvas(t *testing.T) {
	h := newWebhookTestAgentHandler(&fakeCanvasLoader{err: dao.ErrUserCanvasNotFound})
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
	h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv})
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
	h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv})
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
	h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv})
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
	h := newWebhookTestAgentHandler(loader)
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
	h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv})
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
	h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv})
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
	h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv})
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
	h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv})
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
	h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv})
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
	h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv})
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
	h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv})
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
	h := newWebhookTestAgentHandler(loader)
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
	h := newWebhookTestAgentHandler(loader)
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
			h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv, events: []canvas.RunEvent{tt.event}})
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
	h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv, runErr: startErr})
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
			h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv, events: tt.events, runErr: tt.runErr})
			var trace []canvas.RunEvent
			h.webhookTraceAppender = func(_ context.Context, _ string, _ time.Time, ev canvas.RunEvent) {
				trace = append(trace, ev)
			}

			h.runWebhookDetached(t.Context(), "u-1", cv, map[string]any{}, true, time.Now())

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
			h := newWebhookTestAgentHandler(tt.loader)
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
			h.runWebhookDetachedWithContext(ctx, "u-1", cv, map[string]any{}, true, time.Now(), "session-1")

			if len(trace) < 2 || trace[0].Type != "error" || !strings.Contains(trace[0].Data, "timed out") {
				t.Fatalf("timeout trace = %+v, want error followed by failed finished event", trace)
			}
			assertWebhookFinishedTrace(t, trace, false)
		})
	}
}

func TestRunWebhookSyncRecordsCancelledTrace(t *testing.T) {
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", nil)
	h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv, waitForCancellation: true})
	var trace []canvas.RunEvent
	h.webhookTraceAppender = func(ctx context.Context, _ string, _ time.Time, ev canvas.RunEvent) {
		if err := ctx.Err(); err != nil {
			t.Errorf("trace context error = %v, want nil", err)
		}
		trace = append(trace, ev)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result := h.runWebhookSync(ctx, "u-1", cv, map[string]any{}, true, time.Now())

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
				h.runWebhookDetachedWithContext(ctx, "u-1", cv, map[string]any{}, true, time.Now(), "session-1")
			},
		},
		{
			name: "sync",
			run: func(t *testing.T, h *AgentHandler, ctx context.Context, cv *entity.UserCanvas) {
				result := h.runWebhookSync(ctx, "u-1", cv, map[string]any{}, true, time.Now())
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
			h := newWebhookTestAgentHandler(loader)
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
	h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv})
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

func TestWebhook_FindWebhookBegin_NormalizedCanvas(t *testing.T) {
	canvas := makeWebhookCanvas("c1", "u-1", "Webhook", nil)
	normalized := dslpkg.NormalizeForCanvas(canvas.DSL)
	params := findWebhookBegin(normalized)
	if params == nil || params["mode"] != "Webhook" {
		t.Fatalf("findWebhookBegin(normalized canvas) = %v, want Webhook params", params)
	}
}

// ---------- Regression tests for issues raised in code review ----------

type webhookReadErrorCase struct {
	contentType string
	failure     string
}

// TestWebhookRejectsBodyReadErrors checks that neither execution mode starts an Agent after a failed read.
func TestWebhookRejectsBodyReadErrors(t *testing.T) {
	requests := []webhookReadErrorCase{
		{"application/json", "oversized"},
		{"application/json", "interrupted"},
		{"text/plain", "oversized"},
		{"text/plain", "interrupted"},
		{"application/x-www-form-urlencoded", "oversized"},
		{"application/x-www-form-urlencoded", "interrupted"},
	}
	targets := []struct {
		mode string
		path string
	}{
		{"Immediately", "/api/v1/agents/c1/webhook"},
		{"Immediately", "/api/v1/agents/c1/webhook/test"},
		{"Streaming", "/api/v1/agents/c1/webhook"},
		{"Streaming", "/api/v1/agents/c1/webhook/test"},
	}
	for _, request := range requests {
		for _, target := range targets {
			name := request.contentType + "/" + target.mode + "/" + target.path + "/" + request.failure
			t.Run(name, func(t *testing.T) {
				assertWebhookRejectsBodyReadError(t, request, target.mode, target.path)
			})
		}
	}
}

// assertWebhookRejectsBodyReadError checks the response and absence of Agent dispatch.
func assertWebhookRejectsBodyReadError(t *testing.T, request webhookReadErrorCase, mode, path string) {
	t.Helper()
	cv := makeWebhookCanvas("c1", "u-1", "Webhook", map[string]any{
		"execution_mode": mode,
		"security": map[string]any{
			"auth_type": "none", "allow_anonymous": true, "max_body_size": "1kb",
		},
	})
	loader := &fakeCanvasLoader{canvas: cv}
	h := newWebhookTestAgentHandler(loader)
	c, w := webhookCtx("POST", path, "", request.contentType)
	// Exercise the stream limit, not the Content-Length pre-check.
	c.Request.ContentLength = -1
	c.Request.TransferEncoding = []string{"chunked"}
	body := `{"value":"ok"}`
	if request.contentType == "application/x-www-form-urlencoded" {
		body = "value=ok"
	}
	var reader io.Reader
	wantMessage := "body too large"
	if request.failure == "oversized" {
		reader = strings.NewReader(body + strings.Repeat(" ", 1001))
	} else {
		// Even a valid prefix must be rejected if the stream fails afterward.
		reader = io.MultiReader(strings.NewReader(body), iotest.ErrReader(io.ErrUnexpectedEOF))
		wantMessage = "unexpected EOF"
	}
	c.Request.Body = io.NopCloser(reader)

	h.Webhook(c)

	if w.Code != http.StatusOK {
		t.Errorf("HTTP status = %d, want existing 200 error envelope", w.Code)
	}
	code, msg := errBody(t, w.Body.Bytes())
	if code != int(common.CodeDataError) || !strings.Contains(msg, wantMessage) {
		t.Errorf("response = %s, want data error containing %q", w.Body.String(), wantMessage)
	}
	if calls := loader.runCalls.Load(); calls != 0 {
		t.Errorf("Agent started %d times after body read failure", calls)
	}
}

// TestParseWebhookRequestReadErrors verifies that partial payloads are discarded and error causes survive wrapping.
func TestParseWebhookRequestReadErrors(t *testing.T) {
	for _, contentType := range []string{"application/json", "text/plain", "application/x-www-form-urlencoded"} {
		for _, oversized := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/oversized=%t", contentType, oversized), func(t *testing.T) {
				c, w := webhookCtx("POST", "/", "", contentType)
				body := `{"value":"ok"}`
				if contentType == "application/x-www-form-urlencoded" {
					body = "value=ok"
				}
				c.Request.Body = io.NopCloser(io.MultiReader(strings.NewReader(body), iotest.ErrReader(io.ErrUnexpectedEOF)))
				if oversized {
					c.Request.Body = http.MaxBytesReader(w, io.NopCloser(strings.NewReader(body+strings.Repeat(" ", 1001))), 1000)
				}
				parsed, err := parseWebhookRequest("", c)
				if err == nil || parsed != nil {
					t.Fatalf("parsed = %v, err = %v; want no payload and a read error", parsed, err)
				}
				if oversized {
					var sizeErr *http.MaxBytesError
					if !errors.As(err, &sizeErr) {
						t.Fatalf("error = %v, want MaxBytesError", err)
					}
				} else if !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatalf("error = %v, want unexpected EOF", err)
				}
			})
		}
	}
}

// TestParseWebhookRequestAtBodyLimit keeps the configured size limit inclusive for all body formats.
func TestParseWebhookRequestAtBodyLimit(t *testing.T) {
	for _, contentType := range []string{"application/json", "text/plain", "application/x-www-form-urlencoded"} {
		t.Run(contentType, func(t *testing.T) {
			want := strings.Repeat("a", 988)
			body := `{"value":"` + want + `"}`
			if contentType == "application/x-www-form-urlencoded" {
				want = strings.Repeat("a", 994)
				body = "value=" + want
			}
			if len(body) != 1000 {
				t.Fatalf("body length = %d, want 1000", len(body))
			}
			c, w := webhookCtx("POST", "/", body, contentType)
			c.Request.ContentLength = -1
			c.Request.Body = http.MaxBytesReader(w, c.Request.Body, 1000)
			parsed, err := parseWebhookRequest("", c)
			if err != nil {
				t.Fatalf("parse body at limit: %v", err)
			}
			value, ok := parsed["body"].(map[string]any)["value"].(string)
			if !ok || value != want {
				t.Fatalf("parsed body value = %q", value)
			}
		})
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
	h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv})
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
	h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv})
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
	h := newWebhookTestAgentHandler(loader)
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
	h := newWebhookTestAgentHandler(&fakeCanvasLoader{canvas: cv})
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
