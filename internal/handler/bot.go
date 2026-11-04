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
	"strconv"

	"github.com/gin-gonic/gin"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/common"
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
	ChatbotInfo(ctx context.Context, dialogID string) (
		title, avatar, prologue, llmID string, hasWebSearch bool, ec common.ErrorCode, err error)
	AgentbotInputs(ctx context.Context, agentID string) (
		title, avatar, prologue, mode string, inputs map[string]any,
		ec common.ErrorCode, err error)
	AgentbotCompletion(ctx context.Context, agentID string, req service.AgentbotCompletionRequest) (
		<-chan canvas.RunEvent, common.ErrorCode, error)
	AgentbotLogs(ctx context.Context, agentID, messageID string) (map[string]any, common.ErrorCode, error)
	ChatbotCompletion(ctx context.Context, dialogID string, req service.ChatbotCompletionRequest) (
		<-chan service.ChatbotSSEFrame, common.ErrorCode, error)
}

// NewBotHandler wires a BotHandler with the production BotService.
func NewBotHandler(svc *service.BotService) *BotHandler {
	return &BotHandler{botService: svc}
}

// ChatbotInfo GET /api/v1/chatbots/<dialog_id>/info
//
// Mirrors python bot_api.py:126-154. Returns the public metadata of
// a chatbot dialog (title, avatar, prologue, web search flag, llm_id).
func (h *BotHandler) ChatbotInfo(c *gin.Context) {
	dialogID := c.Param("dialog_id")
	if dialogID == "" {
		common.RespondError(c, common.CodeArgumentError, "`dialog_id` is required.")
		return
	}
	title, avatar, prologue, llmID, hasWebSearch, ec, err := h.botService.ChatbotInfo(
		c.Request.Context(), dialogID)
	if err != nil {
		if respondPermissionErrorIf(c, err, true) {
			return
		}
		common.RespondError(c, ec, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{
		"title":                   title,
		"avatar":                  avatar,
		"prologue":                prologue,
		"has_tavily_key":          hasWebSearch,
		"has_web_search_provider": hasWebSearch,
		"llm_id":                  llmID,
	}, "success")
}

// AgentbotInputs GET /api/v1/agentbots/<agent_id>/inputs
//
// Mirrors python bot_api.py:239-250. Returns the public metadata of
// an agentbot canvas (title, avatar, inputs, prologue, mode).
func (h *BotHandler) AgentbotInputs(c *gin.Context) {
	agentID := c.Param("agent_id")
	if agentID == "" {
		common.RespondError(c, common.CodeArgumentError, "`agent_id` is required.")
		return
	}
	title, avatar, prologue, mode, inputs, ec, err := h.botService.AgentbotInputs(
		c.Request.Context(), agentID)
	if err != nil {
		common.RespondError(c, ec, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{
		"title":    title,
		"avatar":   avatar,
		"inputs":   inputs,
		"prologue": prologue,
		"mode":     mode,
	}, "success")
}

// AgentbotCompletion POST /api/v1/agentbots/<agent_id>/completions
//
// Mirrors python bot_api.py:157 (canvas_service.completion wrapper).
// The URL-bound agent_id is authoritative — the body must NOT
// override it.
//
// The shared/embedded agent chat page (web/src/pages/agent/share)
// parses the stream with the same use-send-message.ts parser as the
// in-app agent chat, so frames must carry the agent canvas envelope
// — {event, message_id, session_id, task_id, created_at, data} —
// terminated by `data:[DONE]`. Forwarding raw canvas.RunEvent via
// WriteChatbotRunEvent keeps this endpoint byte-compatible with the
// python canvas_service.completion output (which yields every
// canvas event, including node_* telemetry used by the "Thinking"
// log panel).
func (h *BotHandler) AgentbotCompletion(c *gin.Context) {
	agentID := c.Param("agent_id")
	if agentID == "" {
		common.RespondError(c, common.CodeArgumentError, "`agent_id` is required.")
		return
	}
	var body service.AgentbotCompletionRequest
	// ContentLength != 0 (not > 0) so chunked requests carrying a
	// valid JSON body with ContentLength == -1 still bind. The old
	// `> 0` guard silently dropped those payloads and the canvas
	// then ran with empty inputs.
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			common.RespondError(c, common.CodeArgumentError, "Invalid request: "+err.Error())
			return
		}
	}
	if body.Release == nil {
		if rawRelease, ok := c.GetQuery("release"); ok {
			release, err := strconv.ParseBool(rawRelease)
			if err != nil {
				common.RespondError(c, common.CodeArgumentError, "Invalid release value: "+err.Error())
				return
			}
			body.Release = &release
		}
	}
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	events, ec, err := h.botService.AgentbotCompletion(
		ctx, agentID, body)
	if err != nil {
		common.RespondError(c, ec, err.Error())
		return
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	doneSent := false
	for ev := range events {
		if ev.Type == "done" {
			doneSent = true
		}
		if err := service.WriteChatbotRunEvent(c.Writer, ev); err != nil {
			return
		}
	}
	if !doneSent {
		_ = service.WriteChatbotRunEvent(c.Writer, canvas.RunEvent{Type: "done"})
	}
}

// ChatbotCompletion POST /api/v1/chatbots/<dialog_id>/completions
//
// Mirrors python bot_api.py:55 (async_iframe_completion). Streams
// SSE frames in the Python envelope shape. The streaming helper
// lives in service/bot_completion.go.
func (h *BotHandler) ChatbotCompletion(c *gin.Context) {
	dialogID := c.Param("dialog_id")
	if dialogID == "" {
		common.RespondError(c, common.CodeArgumentError, "`dialog_id` is required.")
		return
	}
	var body service.ChatbotCompletionRequest
	// ContentLength != 0 (not > 0) so chunked requests carrying a
	// valid JSON body with ContentLength == -1 still bind. The old
	// `> 0` guard silently dropped those payloads and the chatbot
	// then ran with empty session_id/question.
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			common.RespondError(c, common.CodeArgumentError, "Invalid request: "+err.Error())
			return
		}
	}
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	frames, ec, err := h.botService.ChatbotCompletion(
		ctx, dialogID, body)
	if err != nil {
		if respondPermissionErrorIf(c, err, true) {
			return
		}
		common.RespondError(c, ec, err.Error())
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

// GetAgentbotLogs GET /api/v1/agentbots/<agent_id>/logs/<message_id>
//
// Beta-token sibling of GetAgentLogs. The shared/embedded chat page's
// "Thinking" button hits this endpoint because the share flow authenticates
// with a beta APIToken (no session JWT). Mirrors python bot_api.py:agent_bot_logs.
func (h *BotHandler) GetAgentbotLogs(c *gin.Context) {
	agentIDStr := c.Param("agent_id")
	if agentIDStr == "" {
		common.RespondError(c, common.CodeArgumentError, "agent_id is required")
		return
	}
	messageID := c.Param("message_id")
	if messageID == "" {
		common.RespondError(c, common.CodeArgumentError, "message_id is required")
		return
	}
	// A beta token with DialogID is restricted to that agent. Tokens without
	// DialogID remain tenant-scoped and are checked by AgentbotLogs through
	// the existing Canvas access guard.
	if boundAgentID, ok := c.Get("agent_id"); ok {
		boundAgentIDStr, ok := boundAgentID.(string)
		if ok && boundAgentIDStr != "" && boundAgentIDStr != agentIDStr {
			common.RespondError(c, common.CodeUnauthorized, "API token is not authorized for this agent.")
			return
		}
	}
	data, ec, err := h.botService.AgentbotLogs(c.Request.Context(), agentIDStr, messageID)
	if err != nil {
		common.RespondError(c, ec, err.Error())
		return
	}
	common.RespondSuccess(c, data, "success")
}
