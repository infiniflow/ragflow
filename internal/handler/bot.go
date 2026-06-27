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
		jsonError(c, code, msg)
		return
	}
	dialogID := c.Param("dialog_id")
	if dialogID == "" {
		jsonError(c, common.CodeArgumentError, "`dialog_id` is required.")
		return
	}
	title, avatar, prologue, llmID, hasTavily, ec, err := h.botService.ChatbotInfo(
		c.Request.Context(), user.ID, dialogID)
	if err != nil {
		jsonError(c, ec, err.Error())
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
		jsonError(c, code, msg)
		return
	}
	agentID := c.Param("agent_id")
	if agentID == "" {
		jsonError(c, common.CodeArgumentError, "`agent_id` is required.")
		return
	}
	title, avatar, prologue, mode, inputs, ec, err := h.botService.AgentbotInputs(
		c.Request.Context(), user.ID, agentID)
	if err != nil {
		jsonError(c, ec, err.Error())
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
		jsonError(c, code, msg)
		return
	}
	agentID := c.Param("agent_id")
	if agentID == "" {
		jsonError(c, common.CodeArgumentError, "`agent_id` is required.")
		return
	}
	var body service.AgentbotCompletionRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			jsonError(c, common.CodeArgumentError, "Invalid request: "+err.Error())
			return
		}
	}
	events, ec, err := h.botService.AgentbotCompletion(
		c.Request.Context(), user.ID, agentID, body)
	if err != nil {
		jsonError(c, ec, err.Error())
		return
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	for ev := range events {
		switch ev.Type {
		case "message", "message_end":
			// The python iframe_completion wrapper flattens each
			// message chunk into a {code:0, data:{answer:...}}
			// frame, then terminates with {code:0, data:true}.
			// We forward the message Data as the assistant text
			// payload so the iframe SDK's `data.answer` parser
			// keeps working. The agentbot path uses
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
			if ev.Type == "message_end" {
				_ = service.WriteDoneFrame(c.Writer)
				return
			}
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
		jsonError(c, code, msg)
		return
	}
	dialogID := c.Param("dialog_id")
	if dialogID == "" {
		jsonError(c, common.CodeArgumentError, "`dialog_id` is required.")
		return
	}
	var body service.ChatbotCompletionRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			jsonError(c, common.CodeArgumentError, "Invalid request: "+err.Error())
			return
		}
	}
	frames, ec, err := h.botService.ChatbotCompletion(
		c.Request.Context(), user.ID, dialogID, body)
	if err != nil {
		jsonError(c, ec, err.Error())
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
