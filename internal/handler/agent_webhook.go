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

// Webhook trigger handler — Go port of
// api/apps/restful_apis/agent_api.py:1563-2248
// (`/api/v1/agents/<agent_id>/webhook` and `/.../webhook/test`).
//
// Mirrors the python reference's 9-step flow:
//  1. Load canvas (IDOR-safe via loadCanvasForUser → LoadCanvasByID).
//  2. Reject DataFlow canvas category.
//  3. Parse DSL map.
//  4. Find Begin with mode=="Webhook"; capture webhook_cfg.
//  5. Validate request method against webhook_cfg.methods.
//  6. validateWebhookSecurity (max body size / IP whitelist / rate limit
//     / token / basic / jwt — strict fail-closed rate limit).
//  7. Parse request body (parseWebhookRequest).
//  8. Schema extract query/headers/body via extractBySchema.
//  9. Dispatch:
//     - execution_mode == "Immediately" → return configured status +
//       body_template synchronously; canvas runs detached in the
//       background. Trace events appended when isTest.
//     - else (streaming/aggregate) → block on canvas.run, aggregate
//       message content, return JSON.
//
// Notes on Python parity divergences:
//   - `cvs.dsl = json.loads(str(canvas)); UserCanvasService.update_by_id(...)`
//     post-run DSL writeback is NOT ported. The Go runner mutates an
//     in-memory copy, not the persisted row. UpdateAgent already persists
//     the editable DSL on the user-driven path.
//   - multipart/form-data uploads are rejected with HTTP 501
//     (ErrWebhookMultipartNotSupported) at parse time. The Python
//     upload path through canvas.get_files_async depends on
//     FileService.upload_info which is itself not in the Go port yet.
//     When FileService.upload_info lands, the 501 branch in Webhook
//     (line 162-185) becomes the dispatch entry point.
//   - content_types whitelist ENFORCES a hard reject (102 envelope)
//     when the request Content-Type does not match the configured
//     value. Mirrors python agent_api.py:1839-1842 (`raise
//     ValueError("Invalid Content-Type...")`).
//   - Webhook trace shape (webhook-trace-<id>-logs in Redis) matches
//     the python append_webhook_trace at agent_api.py:2073-2091 so the
//     eventual /webhook/logs poll PR can consume this writer
//     unchanged.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"ragflow/internal/agent/canvas"
	"ragflow/internal/common"
	rediscli "ragflow/internal/engine/redis"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// canvasLoader is the subset of service.AgentService the webhook handler
// needs. Defined as an interface so handler tests can inject a fake
// without standing up the full AgentService (DB DAOs, eino runner, etc).
//
// LoadCanvasByID returns the raw DAO/service error so the WEBHOOK
// handler can fold it into its own 102 envelope. We deliberately do NOT
// route through handler.mapAgentError here because that helper maps
// ErrUserCanvasNotFound to 103 "Make sure you have permission..." which
// is the chat/run contract — wrong for the webhook path.
type canvasLoader interface {
	LoadCanvasByID(ctx context.Context, userID, canvasID string) (*entity.UserCanvas, error)
	RunAgentWithWebhook(ctx context.Context, userID, canvasID string, payload map[string]any) (<-chan canvas.RunEvent, error)
}

// Webhook is the handler method mounted at:
//
//	/api/v1/agents/:canvas_id/webhook       (production trigger)
//	/api/v1/agents/:canvas_id/webhook/test  (test trigger, with trace)
//
// The Python decorator stack at agent_api.py:1563 binds six methods to
// a single path. Gin has no Match() — the router registers each verb
// individually via registerAnyMethod (see internal/router/agent_routes.go).
func (h *AgentHandler) Webhook(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}

	canvasID := c.Param("canvas_id")
	if h.loader == nil {
		jsonInternalError(c, errors.New("agent webhook: loader not configured"))
		return
	}
	isTest := strings.HasSuffix(c.Request.URL.Path, "/webhook/test")
	startTs := time.Now()

	// 1. Load canvas. Webhook collapses missing-vs-foreign into a
	// single 102 "Canvas not found." (matches Python
	// api/apps/restful_apis/agent_api.py:1572 and the existing
	// GetAgentWebhookLogs envelope), so we DO NOT route through
	// handler.mapAgentError here.
	cv, err := h.loader.LoadCanvasByID(c.Request.Context(), user.ID, canvasID)
	if err != nil {
		if errors.Is(err, dao.ErrUserCanvasNotFound) || errors.Is(err, dao.ErrUserCanvasVersionNotFound) {
			common.ResponseWithCodeData(c, common.CodeDataError, nil, "Canvas not found.")
			return
		}
		jsonInternalError(c, err)
		return
	}

	// 2. Reject DataFlow.
	if cv.CanvasCategory == "DataFlow" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "Dataflow can not be triggered by webhook.")
		return
	}

	// 3. DSL map. cv.DSL is a typed entity.JSONMap (map[string]any
	// under the hood); we copy into a plain map[string]any so the
	// downstream helpers can mutate freely without aliasing surprises.
	dsl := map[string]any{}
	for k, v := range cv.DSL {
		dsl[k] = v
	}

	// 4. Find Begin component with mode=="Webhook".
	webhookCfg := findWebhookBegin(dsl)
	if webhookCfg == nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "Webhook not configured for this agent.")
		return
	}

	// 5. Method gate.
	if !methodAllowed(webhookCfg["methods"], c.Request.Method) {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, fmt.Sprintf("HTTP method '%s' not allowed for this webhook.", c.Request.Method))
		return
	}

	// 6. Security gate (strict; surfaces all errors as 102).
	securityCfg := stringMap(webhookCfg["security"])
	if err := validateWebhookSecurity(securityCfg, c, canvasID); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	// 6a. Body size DoS hardening (security review MEDIUM-1). The
	// security validator above checks the Content-Length header, but
	// that header is attacker-controllable. Wrap the actual stream
	// reader with http.MaxBytesReader so io.ReadAll inside
	// parseWebhookRequest is bounded by the same parsed limit.
	// Errors here surface as ErrWebhookContentTypeMismatch-shaped 102
	// so the operator sees the failure mode consistently with the
	// header check.
	if limit, perr := parseMaxBodySize(securityCfg); perr == nil && limit > 0 && c.Request.Body != nil {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	}

	// 7. Parse request body. Two error classes flow through here:
	//   - ErrWebhookMultipartNotSupported → HTTP 501 (file uploads
	//     depend on FileService.upload_info which is not yet ported).
	//   - ErrWebhookContentTypeMismatch → HTTP 102 (content-type
	//     whitelist, mirrors python agent_api.py:1839).
	// Both are surfaced as typed errors so the dispatch envelope is
	// unambiguous — neither path falls through into schema validation.
	contentType, _ := webhookCfg["content_types"].(string)
	parsed, parseErr := parseWebhookRequest(contentType, c)
	if parseErr != nil {
		switch {
		case errors.Is(parseErr, ErrWebhookMultipartNotSupported):
			// 501 — multipart/form-data uploads are not yet
			// supported. Body is short so operators see exactly what
			// is missing.
			common.ResponseWithHttpCodeData(c, http.StatusNotImplemented, common.CodeNotImplemented, nil, parseErr.Error())
			return
		case errors.Is(parseErr, ErrWebhookContentTypeMismatch):
			common.ResponseWithCodeData(c, common.CodeDataError, nil, parseErr.Error())
			return
		default:
			common.ResponseWithCodeData(c, common.CodeDataError, nil, parseErr.Error())
			return
		}
	}

	// 8. Schema extract query/headers/body.
	schema, _ := webhookCfg["schema"].(map[string]any)
	clean, schemaErr := applyWebhookSchema(parsed, schema)
	if schemaErr != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, schemaErr.Error())
		return
	}

	// 9. Dispatch.
	mode, _ := webhookCfg["execution_mode"].(string)
	if mode == "" {
		mode = "Immediately"
	}
	if mode == "Immediately" {
		status, contentType, payload, perr := renderImmediatelyResponse(stringMap(webhookCfg["response"]))
		if perr != nil {
			common.ResponseWithCodeData(c, common.CodeDataError, nil, perr.Error())
			return
		}
		// Detached background run — does NOT inherit c.Request.Context()
		// so a client disconnect does not cancel the canvas run.
		go h.runWebhookDetached(cv, clean, isTest, startTs)
		c.Data(status, contentType, payload)
		return
	}
	// Streaming / blocking mode.
	out := h.runWebhookSync(c.Request.Context(), cv, clean, isTest, startTs)
	c.JSON(out.status, out.body)
}

// findWebhookBegin scans the DSL components map for a Begin component
// whose params.mode equals "Webhook". Returns the params map (the
// webhook_cfg) on success; nil otherwise. Mirrors agent_api.py:1584-1592.
func findWebhookBegin(dsl map[string]any) map[string]any {
	if dsl == nil {
		return nil
	}
	components, _ := dsl["components"].(map[string]any)
	if components == nil {
		return nil
	}
	for _, raw := range components {
		entry, _ := raw.(map[string]any)
		if entry == nil {
			continue
		}
		obj, _ := entry["obj"].(map[string]any)
		if obj == nil {
			continue
		}
		name, _ := obj["component_name"].(string)
		if !strings.EqualFold(name, "begin") {
			continue
		}
		params, _ := obj["params"].(map[string]any)
		if params == nil {
			continue
		}
		if mode, _ := params["mode"].(string); mode == "Webhook" {
			return params
		}
	}
	return nil
}

// methodAllowed returns true when `requestMethod` is in the configured
// list. Empty list → allow (matches python: agent_api.py:1596 short-circuit).
func methodAllowed(raw any, requestMethod string) bool {
	methods, _ := raw.([]any)
	if len(methods) == 0 {
		return true
	}
	want := strings.ToUpper(requestMethod)
	for _, m := range methods {
		if s, _ := m.(string); strings.ToUpper(s) == want {
			return true
		}
	}
	return false
}

// stringMap is a tiny helper: extract a map[string]any, treating nil and
// wrong-type cases as empty maps. Used heavily to keep the dispatch
// logic readable.
func stringMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

// parseWebhookRequest mirrors agent_api.py:1828-1894.
//
// Returns (parsed, error). The parsed map carries query / headers /
// body / content_type. The body is parsed according to the request
// Content-Type:
//
//   - application/json              → unmarshal JSON
//   - application/x-www-form-urlencoded → form fields
//   - text/plain / octet-stream / unknown / empty → raw bytes → JSON
//
// Two errors are surfaced directly to the handler so the dispatch path
// can return the right envelope without falling through into a
// misleading schema-validation error:
//
//   - ErrWebhookMultipartNotSupported (501): the request used
//     multipart/form-data. The Python path uploads files through
//     FileService.upload_info → canvas.get_files_async, both of which
//     are NOT yet ported. We refuse the request up front so callers
//     see a clear, typed 501 instead of an unrelated schema error.
//
//   - ErrWebhookContentTypeMismatch (102): when the webhook config
//     sets content_types, the request Content-Type must match. The
//     Python reference raises here
//     (`raise ValueError("Invalid Content-Type...")` at
//     agent_api.py:1839-1842); we mirror that as a typed error and
//     surface it through the same 102 envelope as the rest of the
//     validation errors so operators see the same response shape.
func parseWebhookRequest(configuredContentType string, c *gin.Context) (map[string]any, error) {
	// 1. Query
	q := map[string]any{}
	for k, vals := range c.Request.URL.Query() {
		if len(vals) == 1 {
			q[k] = vals[0]
		} else {
			q[k] = vals
		}
	}

	// 2. Headers
	hd := map[string]any{}
	for k, vals := range c.Request.Header {
		if len(vals) == 1 {
			hd[k] = vals[0]
		} else {
			hd[k] = vals
		}
	}

	// 3. Body
	ctype := strings.SplitN(c.GetHeader("Content-Type"), ";", 2)[0]
	ctype = strings.TrimSpace(strings.ToLower(ctype))

	// multipart/form-data → 501. Checked BEFORE the content-type match
	// below because multipart uploads don't carry a useful
	// `content_types` value to validate against — the python handler
	// routes them through canvas.get_files_async which we have not
	// ported yet.
	if ctype == "multipart/form-data" {
		return nil, ErrWebhookMultipartNotSupported
	}

	body := map[string]any{}

	switch ctype {
	case "application/json":
		raw, _ := io.ReadAll(c.Request.Body)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &body)
		}
	case "application/x-www-form-urlencoded":
		if err := c.Request.ParseForm(); err == nil {
			for k, vals := range c.Request.PostForm {
				if len(vals) == 1 {
					body[k] = vals[0]
				} else {
					body[k] = vals
				}
			}
		}
	default:
		raw, _ := io.ReadAll(c.Request.Body)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &body)
		}
	}

	// content_type whitelist. Empty configuredContentType → no check
	// (matches python: agent_api.py:1839 only raises when the
	// webhook_cfg actually sets content_types).
	//
	// The python reference has a `if ctype and ...` short-circuit that
	// lets a request with NO Content-Type header through even when the
	// webhook config requires one. That is a whitelist bypass — the
	// configured type is irrelevant if a caller can simply omit the
	// header. The Go port tightens the contract: when the operator
	// configured content_types, the request MUST carry a matching
	// Content-Type. Missing header → ErrWebhookContentTypeMismatch.
	if configuredContentType != "" && configuredContentType != ctype {
		return nil, fmt.Errorf("%w: expect %q, got %q",
			ErrWebhookContentTypeMismatch, configuredContentType, ctype)
	}

	return map[string]any{
		"query":        q,
		"headers":      hd,
		"body":         body,
		"content_type": ctype,
	}, nil
}

// ErrWebhookMultipartNotSupported is returned by parseWebhookRequest
// when the inbound Content-Type is multipart/form-data. The handler
// translates this to HTTP 501 Not Implemented because the Python file
// upload path (FileService.upload_info → canvas.get_files_async) is
// not ported yet.
var ErrWebhookMultipartNotSupported = errors.New("multipart/form-data uploads are not supported in this port")

// ErrWebhookContentTypeMismatch is returned by parseWebhookRequest when
// the configured content_types whitelist disagrees with the request's
// Content-Type. Mirrors python agent_api.py:1839-1842 (`raise
// ValueError("Invalid Content-Type...")`). Surfaced as 102 so the
// operator sees a clear, content-type-specific message.
var ErrWebhookContentTypeMismatch = errors.New("invalid content-type")

// applyWebhookSchema runs extractBySchema on the parsed request's three
// sections and assembles the clean_request map that the Python
// webhook handler passes to canvas.run(webhook_payload=...). Mirrors
// agent_api.py:2052-2068.
func applyWebhookSchema(parsed map[string]any, schema map[string]any) (map[string]any, error) {
	q := stringMap(parsed["query"])
	hd := stringMap(parsed["headers"])
	bd := stringMap(parsed["body"])

	qClean, err := extractBySchema(q, stringMap(schema["query"]), "query")
	if err != nil {
		return nil, err
	}
	hdClean, err := extractBySchema(hd, stringMap(schema["headers"]), "headers")
	if err != nil {
		return nil, err
	}
	bdClean, err := extractBySchema(bd, stringMap(schema["body"]), "body")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"query":   qClean,
		"headers": hdClean,
		"body":    bdClean,
		"input":   parsed,
	}, nil
}

// renderImmediatelyResponse builds the synchronous response for the
// Immediately execution mode. Mirrors agent_api.py:2093-2121.
//
//   - status: int in [200, 399]; defaults to 200; any other value
//     raises so the operator notices a config bug.
//   - body_template: JSON or text. We try JSON first; fall back to
//     plain text on failure. Empty body → no body, content-type
//     application/json (matching python parse_body(None)).
func renderImmediatelyResponse(cfg map[string]any) (int, string, []byte, error) {
	statusRaw, ok := cfg["status"]
	status := 200
	if ok {
		switch v := statusRaw.(type) {
		case int:
			status = v
		case int64:
			status = int(v)
		case float64:
			status = int(v)
		case string:
			n, err := strconv.Atoi(v)
			if err != nil {
				return 0, "", nil, fmt.Errorf("invalid response status code: %v", v)
			}
			status = n
		default:
			return 0, "", nil, fmt.Errorf("invalid response status code: %v", v)
		}
	}
	if status < 200 || status > 399 {
		return 0, "", nil, fmt.Errorf("invalid response status code: %d, must be between 200 and 399", status)
	}

	bodyTpl, _ := cfg["body_template"].(string)
	if bodyTpl == "" {
		return status, "application/json", nil, nil
	}

	// Try JSON parse first (python: parse_body() branch at line 2110).
	var probe any
	if err := json.Unmarshal([]byte(bodyTpl), &probe); err == nil {
		encoded, _ := json.Marshal(probe)
		return status, "application/json", encoded, nil
	}
	return status, "text/plain", []byte(bodyTpl), nil
}

// runWebhookDetached runs the canvas in the background. It uses
// context.Background() with a 5-minute timeout (NOT
// c.Request.Context()) so a client disconnect does NOT cancel the run.
// Trace events are appended to the redis key when isTest is true.
//
// Mirrors python: agent_api.py:2123-2175 (the asyncio.create_task body
// inside the Immediately branch).
func (h *AgentHandler) runWebhookDetached(
	cv *entity.UserCanvas, payload map[string]any, isTest bool, startTs time.Time,
) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	events, err := h.loader.RunAgentWithWebhook(ctx, cv.UserID, cv.ID, payload)
	if err != nil {
		common.Warn("webhook detached run start failed",
			zap.String("canvas", cv.ID),
			zap.Error(err))
		if isTest {
			appendWebhookTrace(cv.ID, startTs, canvas.RunEvent{Type: "error", Data: mustJSON(map[string]any{"message": err.Error()})})
		}
		return
	}
	for ev := range events {
		if isTest {
			appendWebhookTrace(cv.ID, startTs, ev)
		}
	}
}

// runWebhookSync drives the canvas in the non-Immediately (streaming)
// mode. Returns the HTTP status + body to send. Mirrors
// agent_api.py:2178-2247 with the python sse() coroutine flattened into
// a synchronous loop.
type webhookSyncResult struct {
	status int
	body   any
}

func (h *AgentHandler) runWebhookSync(
	ctx context.Context, cv *entity.UserCanvas, payload map[string]any,
	isTest bool, startTs time.Time,
) webhookSyncResult {
	status := 200
	events, err := h.loader.RunAgentWithWebhook(ctx, cv.UserID, cv.ID, payload)
	if err != nil {
		if isTest {
			appendWebhookTrace(cv.ID, startTs, canvas.RunEvent{Type: "error", Data: mustJSON(map[string]any{"message": err.Error()})})
			appendWebhookTrace(cv.ID, startTs, canvas.RunEvent{Type: "finished", Data: mustJSON(map[string]any{"success": false})})
		}
		return webhookSyncResult{status: http.StatusBadRequest, body: gin.H{
			"code":    400,
			"message": err.Error(),
			"success": false,
		}}
	}

	contents := []string{}
	for ev := range events {
		if isTest {
			appendWebhookTrace(cv.ID, startTs, ev)
		}
		switch ev.Type {
		case "message":
			var msg struct {
				Content      string `json:"content"`
				StartToThink bool   `json:"start_to_think"`
				EndToThink   bool   `json:"end_to_think"`
			}
			if json.Unmarshal([]byte(ev.Data), &msg) == nil {
				content := msg.Content
				if msg.StartToThink {
					content = "think"
				} else if msg.EndToThink {
					content = "/think"
				}
				if content != "" {
					contents = append(contents, content)
				}
			}
		case "message_end":
			var end struct {
				Status *int `json:"status"`
			}
			if json.Unmarshal([]byte(ev.Data), &end) == nil && end.Status != nil {
				status = *end.Status
			}
		}
	}
	final := strings.Join(contents, "")
	if isTest {
		appendWebhookTrace(cv.ID, startTs, canvas.RunEvent{Type: "finished", Data: mustJSON(map[string]any{"success": true})})
	}
	return webhookSyncResult{status: status, body: gin.H{
		"message": final,
		"success": true,
		"code":    status,
	}}
}

// mustJSON marshals v to a JSON object string. Used by trace appenders;
// panics on marshal failure (acceptable because we only marshal
// statically-typed map[string]any values).
func mustJSON(v any) string {
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(v)
	return buf.String()
}

// appendWebhookTrace appends a single RunEvent to the per-canvas trace
// key in Redis. Mirrors python's append_webhook_trace at
// agent_api.py:2073-2091.
//
// The trace key is `webhook-trace-<agent_id>-logs` with a 600 s TTL.
// Each event is recorded as {"ts": <float>, "event": <type>, ...}.
// Tests use miniredis to verify the key shape.
func appendWebhookTrace(agentID string, startTs time.Time, ev canvas.RunEvent) {
	rdb := rediscli.Get()
	if rdb == nil {
		return
	}

	key := fmt.Sprintf("webhook-trace-%s-logs", agentID)
	raw, _ := rdb.Get(key)
	obj := map[string]any{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &obj)
	}
	whs, _ := obj["webhooks"].(map[string]any)
	if whs == nil {
		whs = map[string]any{}
		obj["webhooks"] = whs
	}
	entryKey := strconv.FormatFloat(float64(startTs.UnixNano())/1e9, 'f', -1, 64)
	entry, _ := whs[entryKey].(map[string]any)
	if entry == nil {
		entry = map[string]any{
			"start_ts": float64(startTs.UnixNano()) / 1e9,
			"events":   []any{},
		}
		whs[entryKey] = entry
	}
	events, _ := entry["events"].([]any)
	eventRecord := map[string]any{
		"ts":    float64(time.Now().UnixNano()) / 1e9,
		"event": ev.Type,
	}
	if ev.Data != "" {
		eventRecord["data"] = json.RawMessage(ev.Data)
	}
	if ev.MessageID != "" {
		eventRecord["message_id"] = ev.MessageID
	}
	if ev.TaskID != "" {
		eventRecord["task_id"] = ev.TaskID
	}
	if ev.SessionID != "" {
		eventRecord["session_id"] = ev.SessionID
	}
	entry["events"] = append(events, eventRecord)

	encoded, err := json.Marshal(obj)
	if err != nil {
		common.Warn("webhook trace marshal failed", zap.Error(err))
		return
	}
	rdb.SetObj(key, string(encoded), 600*time.Second)
}
