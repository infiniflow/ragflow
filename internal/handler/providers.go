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
	APIKey       string `json:"api_key" binding:"required"`
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

	_, err := h.modelProviderService.CreateProviderInstance(providerName, req.InstanceName, req.APIKey, userID, "default")
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

type ChatToModelRequest struct {
	Message  string `json:"message" binding:"required"`
	Stream   bool   `json:"stream"`
	Thinking bool   `json:"thinking"`
}

func (h *ProviderHandler) ChatToModel(c *gin.Context) {
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
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Model name is required",
		})
		return
	}

	var req ChatToModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	userID := c.GetString("user_id")

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

		chatConfig := models.ChatConfig{
			Thinking:    &req.Thinking,
			Stream:      &req.Stream,
			Stop:        &[]string{},
			DoSample:    nil,
			MaxTokens:   nil,
			Temperature: nil,
			TopP:        nil,
			Region:      nil,
		}

		// Stream response using sender function (best performance, no channel)
		errorCode := h.modelProviderService.ChatToModelStreamWithSender(providerName, instanceName, modelName, userID, req.Message, &chatConfig, sender)

		if errorCode != common.CodeSuccess {
			c.SSEvent("error", "stream failed")
		}
		return
	}

	chatConfig := models.ChatConfig{
		Thinking:    &req.Thinking,
		Stream:      &req.Stream,
		Stop:        &[]string{},
		DoSample:    nil,
		MaxTokens:   nil,
		Temperature: nil,
		TopP:        nil,
		Region:      nil,
	}

	// Non-stream response
	response, errorCode, err := h.modelProviderService.ChatToModel(providerName, instanceName, modelName, userID, req.Message, &chatConfig)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    errorCode,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": response,
	})
}
