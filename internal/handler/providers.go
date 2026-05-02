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
	"fmt"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity/models"
	"ragflow/internal/service"
	"strings"

	"github.com/gin-gonic/gin"
)

// ProviderHandler provider handler
type ProviderHandler struct {
	userService          *service.UserService
	modelProviderService *service.ModelProviderService
	userTenantDAO        *dao.UserTenantDAO
}

// NewProviderHandler create provider handler
func NewProviderHandler(userService *service.UserService, modelProviderService *service.ModelProviderService) *ProviderHandler {
	return &ProviderHandler{
		userService:          userService,
		modelProviderService: modelProviderService,
		userTenantDAO:        dao.NewUserTenantDAO(),
	}
}

func (h *ProviderHandler) ListProviders(c *gin.Context) {

	keywords := ""
	if queryKeywords := c.Query("available"); queryKeywords != "" {
		keywords = queryKeywords
	}

	// convert keywords to small case
	keywords = strings.ToLower(keywords)
	if keywords == "true" {
		// list pool providers
		providers, err := dao.GetModelProviderManager().ListProviders()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeNotFound,
				"message": err.Error(),
			})
			return
		}

		for _, provider := range providers {
			delete(provider, "url_suffix")
			delete(provider, "tags")
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "success",
			"data":    providers,
		})
		return
	}

	userID := c.GetString("user_id")

	// list tenant providers
	providers, errorCode, err := h.modelProviderService.ListProvidersOfTenant(userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    errorCode,
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    providers,
	})
	return
}

type AddProviderRequest struct {
	ProviderName string `json:"provider_name" binding:"required"`
}

func (h *ProviderHandler) AddProvider(c *gin.Context) {

	var req AddProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
			"data":    false,
		})
		return
	}

	userID := c.GetString("user_id")

	errorCode, err := h.modelProviderService.AddModelProvider(req.ProviderName, userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    errorCode,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
	})
}

func (h *ProviderHandler) DeleteProvider(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	userID := c.GetString("user_id")

	errorCode, err := h.modelProviderService.DeleteModelProvider(providerName, userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    errorCode,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
	})
}

func (h *ProviderHandler) ShowProvider(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	provider, err := dao.GetModelProviderManager().GetProviderByName(providerName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeNotFound,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    provider,
	})
}

func (h *ProviderHandler) ListModels(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}
	providerModels, err := dao.GetModelProviderManager().ListModels(providerName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeNotFound,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    providerModels,
	})
}

func (h *ProviderHandler) ShowModel(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}
	modelName := c.Param("model_name")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Model name is required",
		})
		return
	}
	model, err := dao.GetModelProviderManager().GetModelByName(providerName, modelName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeNotFound,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    model,
	})
}

type CreateProviderInstanceRequest struct {
	InstanceName string `json:"instance_name" binding:"required"`
	APIKey       string `json:"api_key"`
	BaseURL      string `json:"base_url"`
	Region       string `json:"region"`
}

func (h *ProviderHandler) CreateProviderInstance(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	var req CreateProviderInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	// Check if instance name is "default"
	if req.InstanceName == "default" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": "Instance name cannot be 'default'",
		})
		return
	}

	userID := c.GetString("user_id")

	_, err := h.modelProviderService.CreateProviderInstance(providerName, req.InstanceName, req.APIKey, req.BaseURL, req.Region, userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
	})
}

func (h *ProviderHandler) ListProviderInstances(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	userID := c.GetString("user_id")

	instances, errorCode, err := h.modelProviderService.ListProviderInstances(providerName, userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    errorCode,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    instances,
	})
}

func (h *ProviderHandler) ShowProviderInstance(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Instance name is required",
		})
		return
	}

	userID := c.GetString("user_id")

	// Get tenant ID from user
	instance, errorCode, err := h.modelProviderService.ShowProviderInstance(providerName, instanceName, userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    errorCode,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    instance,
	})
}

func (h *ProviderHandler) ShowInstanceBalance(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Instance name is required",
		})
		return
	}

	userID := c.GetString("user_id")

	// Get tenant ID from user
	balance, errorCode, err := h.modelProviderService.ShowInstanceBalance(providerName, instanceName, userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    errorCode,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    balance,
	})
}

func (h *ProviderHandler) CheckProviderConnection(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Instance name is required",
		})
		return
	}

	userID := c.GetString("user_id")

	// Get tenant ID from user
	errorCode, err := h.modelProviderService.CheckProviderConnection(providerName, instanceName, userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    errorCode,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
	})
}

type AlterProviderInstanceRequest struct {
	LLMName string `json:"llm_name" binding:"required"`
}

func (h *ProviderHandler) AlterProviderInstance(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Instance name is required",
		})
		return
	}

	var req AlterProviderInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeUnauthorized,
			"message": "Unauthorized",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeNotFound,
		"message": "success",
	})
}

type DropProviderInstanceRequest struct {
	Instances []string `json:"instances" binding:"required"`
}

func (h *ProviderHandler) DropProviderInstance(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}
	var req DropProviderInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	userID := c.GetString("user_id")

	_, err := h.modelProviderService.DropProviderInstances(providerName, userID, req.Instances)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
	})
}

func (h *ProviderHandler) ListInstanceModels(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}
	instanceName := c.Param("instance_name")
	if instanceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Instance name is required",
		})
		return
	}

	keywords := ""
	if queryKeywords := c.Query("supported"); queryKeywords != "" {
		keywords = queryKeywords
	}

	// convert keywords to small case
	keywords = strings.ToLower(keywords)
	if keywords == "true" {
		// list supported models

		modelList, err := h.modelProviderService.ListSupportedModels(providerName, instanceName, c.GetString("user_id"))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeServerError,
				"message": err.Error(),
			})
			return
		}

		var modelResponse []map[string]string
		for _, modelName := range modelList {
			modelResponse = append(modelResponse, map[string]string{
				"model_name": modelName,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "success",
			"data":    modelResponse,
		})
		return
	}

	modelInstances, err := h.modelProviderService.ListInstanceModels(providerName, instanceName, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeNotFound,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    modelInstances,
	})
}

type EnableOrDisableModelRequest struct {
	Status string `json:"status" binding:"required"`
}

func (h *ProviderHandler) EnableOrDisableModel(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Instance name is required",
		})
		return
	}

	modelName := c.Param("model_name")
	if modelName != "" {
		modelName = strings.TrimPrefix(modelName, "/")
	}
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Model name is required",
		})
		return
	}

	var req EnableOrDisableModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	userID := c.GetString("user_id")

	_, err := h.modelProviderService.UpdateModelStatus(providerName, instanceName, modelName, userID, req.Status)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
	})
}

func (h *ProviderHandler) AddCustomModel(c *gin.Context) {
	var req service.AddCustomModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	if req.ProviderName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	if req.InstanceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Instance name is required",
		})
		return
	}

	if req.ModelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Model name is required",
		})
		return
	}

	if req.ModelTypes == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Model type is required",
		})
		return
	}

	userID := c.GetString("user_id")

	errorCode, err := h.modelProviderService.AddCustomModel(&req, userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    errorCode,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": common.CodeSuccess,
	})

}

type DropInstanceModelRequest struct {
	Models []string `json:"models" binding:"required"`
}

func (h *ProviderHandler) DropInstanceModels(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}
	instanceName := c.Param("instance_name")
	if instanceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Instance name is required",
		})
		return
	}

	var req DropInstanceModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	userID := c.GetString("user_id")

	_, err := h.modelProviderService.DropInstanceModels(providerName, instanceName, userID, req.Models)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
	})
}

type ChatToModelRequest struct {
	ProviderName *string                  `json:"provider_name"`
	InstanceName *string                  `json:"instance_name"`
	ModelName    *string                  `json:"model_name"`
	Messages     []map[string]interface{} `json:"messages"`
	Stream       bool                     `json:"stream"`
	Thinking     bool                     `json:"thinking"`
	Effort       *string                  `json:"effort"`
	Verbosity    *string                  `json:"verbosity"`
}

func (h *ProviderHandler) ChatToModel(c *gin.Context) {
	var req ChatToModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	if req.ProviderName == nil || *req.ProviderName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}

	if req.InstanceName == nil || *req.InstanceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Instance name is required",
		})
		return
	}

	if req.ModelName == nil || *req.ModelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Model name is required",
		})
		return
	}

	userID := c.GetString("user_id")

	if !req.Thinking {
		req.Effort = nil
		req.Verbosity = nil
	}

	apiConfig := models.APIConfig{
		ApiKey: nil,
		Region: nil,
	}

	chatConfig := models.ChatConfig{
		Thinking:    &req.Thinking,
		Stream:      &req.Stream,
		Vision:      nil,
		Stop:        &[]string{},
		DoSample:    nil,
		MaxTokens:   nil,
		Temperature: nil,
		TopP:        nil,
		Effort:      req.Effort,
		Verbosity:   req.Verbosity,
	}

	// Check if it's a stream request
	if req.Stream {
		// Set SSE headers
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Writer.WriteHeader(http.StatusOK)
		c.Writer.Flush()

		// Create sender function that writes directly to response
		sender := func(content, reasoningContent *string) error {
			// Check for [DONE] marker (OpenAI compatible)
			if content != nil {
				if *content == "[DONE]" {
					c.SSEvent("done", "[DONE]")
					return nil
				}
				message := fmt.Sprintf("[MESSAGE]%s", *content)
				c.SSEvent("message", message)
				c.Writer.Flush()
			}

			if reasoningContent != nil {
				message := fmt.Sprintf("[REASONING]%s", *reasoningContent)
				c.SSEvent("message", message)
				c.Writer.Flush()
			}

			//logger.Info(data)
			return nil
		}

		// Convert []map[string]interface{} to []models.Message
		messages := make([]models.Message, len(req.Messages))
		for i, msg := range req.Messages {
			role, _ := msg["role"].(string)
			content := msg["content"]
			messages[i] = models.Message{Role: role, Content: content}
		}

		// Stream response using sender function (best performance, no channel)
		errorCode, err := h.modelProviderService.ChatToModelStreamWithSender(*req.ProviderName, *req.InstanceName, *req.ModelName, userID, messages, &apiConfig, &chatConfig, sender)

		if errorCode != common.CodeSuccess {
			c.SSEvent("error", err.Error())
		}
		return
	}

	// Non-stream response
	var response *models.ChatResponse
	var errorCode common.ErrorCode
	var err error

	// Convert []map[string]interface{} to []models.Message
	messages := make([]models.Message, len(req.Messages))
	for i, msg := range req.Messages {
		role, _ := msg["role"].(string)
		content := msg["content"]
		messages[i] = models.Message{Role: role, Content: content}
	}
	response, errorCode, err = h.modelProviderService.ChatToModelWithMessages(*req.ProviderName, *req.InstanceName, *req.ModelName, userID, messages, &apiConfig, &chatConfig)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    errorCode,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":              0,
		"reasoning_content": response.ReasonContent,
		"answer":            response.Answer,
	})
}
