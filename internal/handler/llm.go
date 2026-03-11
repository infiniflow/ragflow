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
	"net/http"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/dao"
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
		jsonError(c, errorCode, errorMessage)
		return
	}

	tenantID := user.ID
	includeDetailsStr := c.DefaultQuery("include_details", "false")
	includeDetails := includeDetailsStr == "true"

	llms, err := h.llmService.GetMyLLMs(tenantID, includeDetails)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeExceptionError,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    llms,
	})
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
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req service.SetAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"message": "Invalid request: " + err.Error(),
			"data":    false,
		})
		return
	}

	tenantID := user.ID
	result, err := h.llmService.SetAPIKey(tenantID, &req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	if req.Verify {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeSuccess,
			"message": "success",
			"data":    result,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    true,
	})
}

// Factories get model provider factories
// @Summary Get Model Provider Factories
// @Description Get list of model provider factories
// @Tags llm
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {array} FactoryResponse
// @Router /v1/llm/factories [get]
func (h *LLMHandler) Factories(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	// Get model providers
	dao := dao.NewModelProviderDAO()
	providers := dao.GetAllProviders()

	// Filter out unwanted providers
	filtered := make([]FactoryResponse, 0)
	excluded := map[string]bool{
		"Youdao":    true,
		"FastEmbed": true,
		"BAAI":      true,
		"Builtin":   true,
	}

	for _, provider := range providers {
		if excluded[provider.Name] {
			continue
		}

		// Collect unique model types from LLMs
		modelTypes := make(map[string]bool)
		for _, llm := range provider.LLMs {
			modelTypes[llm.ModelType] = true
		}

		// Convert to slice
		modelTypeSlice := make([]string, 0, len(modelTypes))
		for mt := range modelTypes {
			modelTypeSlice = append(modelTypeSlice, mt)
		}

		// If no model types found, use defaults
		if len(modelTypeSlice) == 0 {
			modelTypeSlice = []string{"chat", "embedding", "rerank", "image2text", "speech2text", "tts", "ocr"}
		}

		filtered = append(filtered, FactoryResponse{
			Name:       provider.Name,
			Logo:       provider.Logo,
			Tags:       provider.Tags,
			Status:     provider.Status,
			Rank:       provider.Rank,
			ModelTypes: modelTypeSlice,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data": filtered,
	})
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
		jsonError(c, errorCode, errorMessage)
		return
	}

	tenantID := user.ID

	modelType := c.Query("model_type")

	llms, err := h.llmService.ListLLMs(tenantID, modelType)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeExceptionError,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data":    llms,
	})
}
