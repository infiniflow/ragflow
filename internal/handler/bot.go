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
	"fmt"

	"github.com/gin-gonic/gin"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/common"
	"ragflow/internal/engine/redis"
	"ragflow/internal/service"
)

// BotHandler is the handler for the public chatbot/agentbot
// endpoints mounted on /api/v1/chatbots/* and /api/v1/agentbots/*.
// The two route groups share BetaAuthMiddleware (set up at
// registration time via g.Use(mw)) and share the same handler
// struct because they are wired to the same BotService.
type BotHandler struct {
	botService botService
}

// botService is the subset of BotService used by these handlers. It
// is interface-typed so the test suite can inject a stub.
type botService interface {
	ChatbotInfo(ctx context.Context, tenantID, dialogID string) (
		title, avatar, prologue, llmID string, hasTavilyKey bool, ec common.ErrorCode, err error)
	AgentbotInputs(ctx context.Context, tenantID, agentID string) (
		title, avatar, prologue, mode string, inputs map[string]any,
		ec common.ErrorCode, err error)
	AgentbotCompletion(ctx context.Context, tenantID, agentID string, req service.AgentbotCompletionRequest) (
		<-chan canvas.RunEvent, common.ErrorCode, error)
	ChatbotCompletion(ctx context.Context, tenantID, dialogID string, req service.ChatbotCompletionRequest) (
		<-chan service.ChatbotSSEFrame, common.ErrorCode, error)
}

// NewBotHandler wires a BotHandler with the production BotService.
func NewBotHandler(svc *service.BotService) *BotHandler {
	return &BotHandler{botService: svc}
}

// ChatbotInfo GET /api/v1/chatbots/<dialog_id>/info
//
// Mirrors python bot_api.py:126-154. Returns the public metadata of
// a chatbot dialog (title, avatar, prologue, tavily key flag, llm_id).
func (h *BotHandler) ChatbotInfo(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	dialogID := c.Param("dialog_id")
	if dialogID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "`dialog_id` is required.")
		return
	}
	title, avatar, prologue, llmID, hasTavily, ec, err := h.botService.ChatbotInfo(
		c.Request.Context(), user.ID, dialogID)
	if err != nil {
		common.ResponseWithCodeData(c, ec, nil, err.Error())
		return
	}
	c.JSON(200, gin.H{
		"code": common.CodeSuccess,
		"data": gin.H{
			"title":          title,
			"avatar":         avatar,
			"prologue":       prologue,
			"has_tavily_key": hasTavily,
			"llm_id":         llmID,
		},
		"message": "success",
	})
}

// AgentbotInputs GET /api/v1/agentbots/<agent_id>/inputs
//
// Mirrors python bot_api.py:239-250. Returns the public metadata of
// an agentbot canvas (title, avatar, inputs, prologue, mode).
func (h *BotHandler) AgentbotInputs(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	agentID := c.Param("agent_id")
	if agentID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "`agent_id` is required.")
		return
	}
	title, avatar, prologue, mode, inputs, ec, err := h.botService.AgentbotInputs(
		c.Request.Context(), user.ID, agentID)
	if err != nil {
		common.ResponseWithCodeData(c, ec, nil, err.Error())
		return
	}
	c.JSON(200, gin.H{
		"code": common.CodeSuccess,
		"data": gin.H{
			"title":    title,
			"avatar":   avatar,
			"inputs":   inputs,
			"prologue": prologue,
			"mode":     mode,
		},
		"message": "success",
	})
}

// AgentbotCompletion POST /api/v1/agentbots/<agent_id>/completions
//
// Mirrors python bot_api.py:157 (canvas_service.completion wrapper).
// Streams SSE frames in the Python envelope shape. The URL-bound
// agent_id is authoritative — the body must NOT override it.
//
// Each canvas.RunEvent is re-formatted into the Python
// {code, message, data} envelope: a "message" event's Data string is
// treated as the assistant text, "message_end" terminates the
// stream with the python completion marker.
func (h *BotHandler) AgentbotCompletion(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	agentID := c.Param("agent_id")
	if agentID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "`agent_id` is required.")
		return
	}
	var body service.AgentbotCompletionRequest
	// ContentLength != 0 (not > 0) so chunked requests carrying a
	// valid JSON body with ContentLength == -1 still bind. The old
	// `> 0` guard silently dropped those payloads and the canvas
	// then ran with empty inputs.
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
				"Invalid request: "+err.Error())
			return
		}
	}
	events, ec, err := h.botService.AgentbotCompletion(
		c.Request.Context(), user.ID, agentID, body)
	if err != nil {
		common.ResponseWithCodeData(c, ec, nil, err.Error())
		return
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	for ev := range events {
		switch ev.Type {
		case "message":
			// The python iframe_completion wrapper flattens each
			// message chunk into a {code:0, data:{answer:...}}
			// frame. We forward the message Data as the assistant
			// text payload so the iframe SDK's `data.answer`
			// parser keeps working. The agentbot path uses
			// WriteAgentbotFrame (a thin alias for
			// WriteChatbotFrame) to keep the two paths visually
			// distinct in the handler.
			frame := service.ChatbotSSEFrame{
				Data:      ev.Data,
				Reference: map[string]any{},
				SessionID: ev.SessionID,
			}
			if err := service.WriteAgentbotFrame(c.Writer, frame); err != nil {
				return
			}
		case "message_end", "done":
			// Terminator events. message_end occasionally carries
			// a final payload (e.g. structured output); forward
			// it as a final answer frame when present, then close
			// the stream with the standard python completion
			// marker. A bare `done` event closes the stream
			// directly.
			if ev.Data != "" {
				frame := service.ChatbotSSEFrame{
					Data:      ev.Data,
					Reference: map[string]any{},
					SessionID: ev.SessionID,
				}
				_ = service.WriteAgentbotFrame(c.Writer, frame)
			}
			_ = service.WriteDoneFrame(c.Writer)
			return
		default:
			// Non-message events (node_started, node_finished, …)
			// are silently dropped on the agentbot path. The
			// python canvas_service.completion wrapper only
			// forwards the assistant text frames, not the run
			// telemetry; we mirror that behaviour so external
			// widgets see the same wire shape.
		}
	}
}

// ChatbotCompletion POST /api/v1/chatbots/<dialog_id>/completions
//
// Mirrors python bot_api.py:55 (async_iframe_completion). Streams
// SSE frames in the Python envelope shape. The streaming helper
// lives in service/bot_completion.go.
func (h *BotHandler) ChatbotCompletion(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	dialogID := c.Param("dialog_id")
	if dialogID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "`dialog_id` is required.")
		return
	}
	var body service.ChatbotCompletionRequest
	// ContentLength != 0 (not > 0) so chunked requests carrying a
	// valid JSON body with ContentLength == -1 still bind. The old
	// `> 0` guard silently dropped those payloads and the chatbot
	// then ran with empty session_id/question.
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
				"Invalid request: "+err.Error())
			return
		}
	}
	frames, ec, err := h.botService.ChatbotCompletion(
		c.Request.Context(), user.ID, dialogID, body)
	if err != nil {
		common.ResponseWithCodeData(c, ec, nil, err.Error())
		return
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	for f := range frames {
		if f.Done {
			if err := service.WriteDoneFrame(c.Writer); err != nil {
				return
			}
			continue
		}
		if err := service.WriteChatbotFrame(c.Writer, f); err != nil {
			return
		}
	}
}

// GetAgentbotLogs GET /api/v1/agentbots/<shared_id>/logs/<message_id>
//
// Beta-token sibling of GetAgentLogs. The shared/embedded chat
// page's "Thinking" button hits this endpoint because the share
// flow authenticates with a beta APIToken (no session JWT) and
// the regular /api/v1/agents/<id>/logs/<msg> requires @login_required.
// Mirrors python bot_api.py:agent_bot_logs (PR #15238).
//
// The <shared_id> path segment is the value the client passed in
// the URL (typically the beta token in the share flow); the real
// agent_id used to build the Redis key
// (`<agent_id>-<message_id>-logs`) is read from the APIToken
// looked up by the beta middleware and stashed in the gin
// context as "agent_id". The endpoint never trusts the URL
// segment for the data lookup — using the middleware-resolved
// agent_id prevents a probe that swaps a victim's shared_id to
// read another agent's logs.
func (h *BotHandler) GetAgentbotLogs(c *gin.Context) {
	if _, code, msg := GetUser(c); code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	agentID, _ := c.Get("agent_id")
	agentIDStr, _ := agentID.(string)
	if agentIDStr == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "API token is not bound to an agent.")
		return
	}
	messageID := c.Param("message_id")
	if messageID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "message_id is required")
		return
	}
	key := fmt.Sprintf("%s-%s-logs", agentIDStr, messageID)
	payload, rerr := redis.Get().Get(key)
	// Surface Redis / decode failures instead of silently returning
	// `{code: 0, data: {}}` — the previous form made the endpoint
	// indistinguishable from "logs not yet written", which masked
	// real outages and corrupted payloads from operators (PR review
	// round 5, Major #6).
	if rerr != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, nil, "failed to read agent logs")
		return
	}
	data := map[string]interface{}{}
	if payload != "" {
		if uerr := json.Unmarshal([]byte(payload), &data); uerr != nil {
			common.ResponseWithCodeData(c, common.CodeServerError, nil, "failed to decode agent logs")
			return
		}
	}
	c.JSON(200, gin.H{
		"code":    common.CodeSuccess,
		"data":    data,
		"message": "success",
	})
}
