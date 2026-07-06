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

// Package router contains the HTTP route registration helpers used by
// cmd/ragflow. This file is the dedicated registration site for the
// agent canvas endpoints described in plan §4.8.
package router

import (
	"github.com/gin-gonic/gin"

	"ragflow/internal/handler"
)

// RegisterAgentRoutes wires the Phase 5 agent endpoints onto an
// existing /agents RouterGroup. The orchestrator passes the v1 group's
// "/agents" sub-group here, so the function does not know about the
// v1 prefix itself.
//
// The existing GET /api/v1/agents (added in commit 0a7662cf3) is replaced
// by this registration so the route count, ordering and middleware all
// live in one place. The original GET is preserved verbatim at
// router.go:349 until the orchestrator swaps it for a call to this
// function.
func RegisterAgentRoutes(g *gin.RouterGroup, h *handler.AgentHandler) {
	if g == nil || h == nil {
		return
	}
	// Discovery / metadata.
	g.GET("/templates", h.ListAgentTemplates)
	g.GET("/prompts", h.Prompts)
	g.GET("/tags", h.ListAgentTags)

	// Agent CRUD.
	g.GET("", h.ListAgents)
	g.POST("", h.CreateAgent)
	g.GET("/:canvas_id", h.GetAgent)
	g.PUT("/:canvas_id", h.UpdateAgent)
	g.DELETE("/:canvas_id", h.DeleteAgent)
	g.POST("/:canvas_id/run", h.RunAgent)
	g.DELETE("/:canvas_id/run", h.CancelAgent)
	g.POST("/:canvas_id/publish", h.PublishAgent)
	g.PUT("/:canvas_id/tags", h.UpdateAgentTags)
	g.POST("/:canvas_id/reset", h.ResetAgent)

	// File operations.
	g.GET("/download", h.DownloadAgentFile)
	g.GET("/attachments/:attachment_id/download", h.DownloadAttachment)
	g.GET("/attachments/:attachment_id/preview", h.PreviewAttachment)
	g.POST("/:canvas_id/upload", h.UploadAgentFile)

	// Component introspection + debug.
	g.GET("/:canvas_id/components/:component_id/input-form", h.GetComponentInputForm)
	g.POST("/:canvas_id/components/:component_id/debug", h.DebugComponent)

	// Versions.
	g.GET("/:canvas_id/versions", h.ListVersions)
	g.GET("/:canvas_id/versions/:version_id", h.GetVersion)
	g.DELETE("/:canvas_id/versions/:version_id", h.DeleteVersion)

	// Sessions.
	g.GET("/:canvas_id/sessions", h.ListAgentSessions)
	g.POST("/:canvas_id/sessions", h.CreateAgentSession)
	g.GET("/:canvas_id/sessions/:session_id", h.GetAgentSession)
	g.DELETE("/:canvas_id/sessions", h.DeleteAgentSession)
	g.DELETE("/:canvas_id/sessions/:session_id", h.DeleteAgentSession)

	// Logs and webhook.
	g.GET("/:canvas_id/logs/:message_id", h.GetAgentLogs)
	g.GET("/:canvas_id/webhook/logs", h.GetAgentWebhookLogs)
	// Webhook trigger endpoints. The Python agent API
	// (api/apps/restful_apis/agent_api.py:1563-1564) registers six
	// HTTP methods on a single path. Gin has no Match() helper, so we
	// register each verb explicitly via registerAnyMethod. The handler
	// is identical for all six; semantics differ only by
	// c.Request.Method.
	registerAnyMethod(g, "/:canvas_id/webhook", h.Webhook)
	registerAnyMethod(g, "/:canvas_id/webhook/test", h.Webhook)

	// Top-level actions (no canvas id in path).
	// NOTE: `/chat/completion` (singular) is intentionally NOT registered.
	// The singular form was a historical typo in earlier Python releases —
	// no client, SDK, or doc ever called it, and the Python side
	// (api/apps/restful_apis/agent_api.py) has since removed the route.
	// See plan: .claude/plans/agent-api-gaps-go-port.md §Gap E.
	g.POST("/chat/completions", h.AgentChatCompletions)
	g.POST("/rerun", h.RerunAgent)
	g.POST("/test_db_connection", h.TestDBConnection)
}

// registerAnyMethod mirrors the Python
// `@manager.route(path, methods=["POST","GET","PUT","PATCH","DELETE","HEAD"])`
// pattern. Gin has no Match() helper, so we register each verb
// explicitly. The handler is identical for all six — semantics differ
// only by c.Request.Method.
//
// Centralising the registration here keeps RegisterAgentRoutes readable
// when both the production trigger and the test trigger share the same
// six-method shape.
func registerAnyMethod(g *gin.RouterGroup, path string, h gin.HandlerFunc) {
	if g == nil || h == nil {
		return
	}
	g.POST(path, h)
	g.GET(path, h)
	g.PUT(path, h)
	g.PATCH(path, h)
	g.DELETE(path, h)
	g.HEAD(path, h)
}
