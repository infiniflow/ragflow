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
	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
)

// ListVersionCatalog returns lightweight version metadata grouped by canvas.
// @Summary List agent or pipeline versions
// @Tags agents
// @Produce json
// @Param category query string true "Canvas category" Enums(dataflow,agent)
// @Success 200 {array} service.AgentVersionCatalogItem
// @Router /api/v1/agents/versions [get]
func (h *AgentHandler) ListVersionCatalog(c *gin.Context) {
	if _, code, msg := GetUser(c); code != common.CodeSuccess {
		common.RespondError(c, code, msg)
		return
	}
	var canvasCategory string
	switch c.Query("category") {
	case "dataflow":
		canvasCategory = "dataflow_canvas"
	case "agent":
		canvasCategory = "agent_canvas"
	default:
		common.RespondError(c, common.CodeBadRequest, "category must be dataflow or agent")
		return
	}
	items, err := h.agentService.ListVersionCatalog(c.Request.Context(), canvasCategory)
	if err != nil {
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}
	common.RespondSuccessList(c, items, "success", int64(len(items)))
}
