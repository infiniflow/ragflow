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

package router

import (
	"github.com/gin-gonic/gin"

	"ragflow/internal/handler"
)

// RegisterChatbotRoutes wires the dialog (legacy chatbot) endpoints
// on the /api/v1/chatbots subtree. Mirrors python
//
//	@manager.route("/chatbots/<dialog_id>/completions")   bot_api.py:55
//	@manager.route("/chatbots/<dialog_id>/info")          bot_api.py:126
//
// The two bot route groups (chatbots + agentbots) cannot share a
// registrar because each carries a different <param_name>
// (dialog_id vs agent_id) and would otherwise register paths under
// the wrong group.
func RegisterChatbotRoutes(g *gin.RouterGroup, mw gin.HandlerFunc, h *handler.BotHandler) {
	if g == nil || h == nil {
		return
	}
	g.Use(mw)
	g.POST("/:dialog_id/completions", h.ChatbotCompletion)
	g.GET("/:dialog_id/info", h.ChatbotInfo)
}

// RegisterAgentbotRoutes wires the canvas-based agent endpoints on
// the /api/v1/agentbots subtree. Mirrors python
//
//	@manager.route("/agentbots/<agent_id>/completions")   bot_api.py:157
//	@manager.route("/agentbots/<agent_id>/inputs")        bot_api.py:239
//	@manager.route("/agentbots/<shared_id>/logs/<message_id>")  bot_api.py:251
func RegisterAgentbotRoutes(g *gin.RouterGroup, mw gin.HandlerFunc, h *handler.BotHandler) {
	if g == nil || h == nil {
		return
	}
	g.POST("/:agent_id/completions", h.AgentbotCompletion)
	g.GET("/:agent_id/inputs", h.AgentbotInputs)
	g.GET("/:agent_id/logs/:message_id", h.GetAgentbotLogs)
}
