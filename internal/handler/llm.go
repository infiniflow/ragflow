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

// FactoryResponse represents a model provider factory
type FactoryResponse struct {
	Name       string   `json:"name"`
	Logo       string   `json:"logo"`
	Tags       string   `json:"tags"`
	Status     string   `json:"status"`
	Rank       string   `json:"rank"`
	ModelTypes []string `json:"model_types"`
}

// LLMHandler LLM handler
type LLMHandler struct {
	llmService  *service.LLMService
	userService *service.UserService
}

// NewLLMHandler create LLM handler
func NewLLMHandler(llmService *service.LLMService, userService *service.UserService) *LLMHandler {
	return &LLMHandler{
		llmService:  llmService,
		userService: userService,
	}
}

// GetMyLLMs get my LLMs
// @Summary Get My LLMs
// @Description Get LLM list for current tenant
// @Tags llm
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param include_details query string false "Include detailed fields" default(false)
// @Success 200 {object} map[string]interface{}
// @Router /v1/llm/my_llms [get]
func (h *LLMHandler) GetMyLLMs(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	tenantID := user.ID
	includeDetailsStr := c.DefaultQuery("include_details", "false")
	includeDetails := includeDetailsStr == "true"

	llms, err := h.llmService.GetMyLLMs(tenantID, includeDetails)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, false, err.Error())
		return
	}

	common.SuccessWithData(c, llms, "success")
}

// SetAPIKey set API key for a LLM factory
// @Summary Set API Key
// @Description Set API key for a LLM factory and test connectivity
// @Tags llm
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body service.SetAPIKeyRequest true "API Key configuration"
// @Success 200 {object} map[string]interface{}
// @Router /v1/llm/set_api_key [post]
func (h *LLMHandler) SetAPIKey(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req service.SetAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, false, "Invalid request: "+err.Error())
		return
	}

	tenantID := user.ID
	result, err := h.llmService.SetAPIKey(tenantID, &req)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, false, err.Error())
		return
	}

	if req.Verify {
		common.SuccessWithData(c, result, "success")
		return
	}

	common.SuccessWithData(c, true, "success")
}

// ListApp lists LLMs grouped by factory
// @Summary List LLMs
// @Description Get list of LLMs grouped by factory with availability info
// @Tags llm
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param model_type query string false "Filter by model type"
// @Success 200 {object} map[string][]service.LLMListItem
// @Router /v1/llm/list [get]
func (h *LLMHandler) ListApp(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	tenantID := user.ID

	modelType := c.Query("model_type")

	llms, err := h.llmService.ListLLMs(tenantID, modelType)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, false, err.Error())
		return
	}

	common.SuccessWithData(c, llms, "success")
}
