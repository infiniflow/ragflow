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
	"ragflow/internal/service"
)

// PluginHandler serves the /plugin/* HTTP routes.
type PluginHandler struct {
	pluginService *service.PluginService
}

// NewPluginHandler creates a plugin handler.
func NewPluginHandler(pluginService *service.PluginService) *PluginHandler {
	return &PluginHandler{
		pluginService: pluginService,
	}
}

// ListLLMTools handles GET /v1/plugin/tools.
//
// @Summary  List LLM tool plugins
// @Description  Return the metadata of every embedded LLM tool plugin. Matches
// @Description  the response of the Python GET /v1/plugin/tools endpoint.
// @Tags     plugin
// @Produce  json
// @Security ApiKeyAuth
// @Success  200 {object} map[string]interface{}
// @Router   /v1/plugin/tools [get]
func (h *PluginHandler) ListLLMTools(c *gin.Context) {
	if _, errorCode, errorMessage := GetUser(c); errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	common.SuccessWithData(c, h.pluginService.ListLLMTools(), "SUCCESS")
}
