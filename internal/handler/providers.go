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
	"errors"
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
			common.ErrorWithCode(c, int(common.CodeNotFound), err.Error())
			return
		}

		for _, provider := range providers {
			delete(provider, "url_suffix")
			delete(provider, "tags")
		}

		common.SuccessWithData(c, providers, "success")
		return
	}

	userID := c.GetString("user_id")

	// list tenant providers
	providers, errorCode, err := h.modelProviderService.ListProvidersOfTenant(userID)
	if err != nil {
		common.ResponseWithCodeData(c, errorCode, nil, err.Error())
		return
	}

	common.SuccessWithData(c, providers, "success")
	return
}

type AddProviderRequest struct {
	ProviderName string `json:"provider_name" binding:"required"`
}

func (h *ProviderHandler) AddProvider(c *gin.Context) {

	var req AddProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeBadRequest, false, err.Error())
		return
	}

	userID := c.GetString("user_id")

	errorCode, err := h.modelProviderService.AddModelProvider(req.ProviderName, userID)
	if err != nil {
		common.ErrorWithCode(c, int(errorCode), err.Error())
		return
	}

	common.SuccessWithMessage(c, "success")
}

func (h *ProviderHandler) DeleteProvider(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	userID := c.GetString("user_id")

	errorCode, err := h.modelProviderService.DeleteModelProvider(userID, providerName)
	if err != nil {
		common.ErrorWithCode(c, int(errorCode), err.Error())
		return
	}

	common.SuccessWithMessage(c, "success")
}

func (h *ProviderHandler) ShowProvider(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	provider, err := dao.GetModelProviderManager().GetProviderByName(providerName)
	if err != nil {
		common.ErrorWithCode(c, int(common.CodeNotFound), err.Error())
		return
	}
	common.SuccessWithData(c, provider, "success")
}

func (h *ProviderHandler) ListModels(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}
	providerModels, err := dao.GetModelProviderManager().ListModels(providerName)
	if err != nil {
		common.ErrorWithCode(c, int(common.CodeNotFound), err.Error())
		return
	}
	common.SuccessWithData(c, providerModels, "success")
}

func (h *ProviderHandler) ShowModel(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}
	modelName := c.Param("model_name")
	if modelName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Model name is required")
		return
	}
	model, err := dao.GetModelProviderManager().GetModelByName(providerName, modelName)
	if err != nil {
		common.ErrorWithCode(c, int(common.CodeNotFound), err.Error())
		return
	}

	common.SuccessWithData(c, model, "success")
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
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	var req CreateProviderInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}

	userID := c.GetString("user_id")

	_, err := h.modelProviderService.CreateProviderInstance(providerName, req.InstanceName, req.APIKey, req.BaseURL, req.Region, userID)
	if err != nil {
		common.ErrorWithCode(c, int(common.CodeServerError), err.Error())
		return
	}

	common.SuccessWithMessage(c, "success")
}

func (h *ProviderHandler) ListProviderInstances(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	userID := c.GetString("user_id")

	instances, errorCode, err := h.modelProviderService.ListProviderInstances(providerName, userID)
	if err != nil {
		common.ErrorWithCode(c, int(errorCode), err.Error())
		return
	}

	common.SuccessWithData(c, instances, "success")
}

func (h *ProviderHandler) ShowProviderInstance(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
		return
	}

	userID := c.GetString("user_id")

	// Get tenant ID from user
	instance, errorCode, err := h.modelProviderService.ShowProviderInstance(providerName, instanceName, userID)
	if err != nil {
		common.ErrorWithCode(c, int(errorCode), err.Error())
		return
	}

	common.SuccessWithData(c, instance, "success")
}

func (h *ProviderHandler) ShowInstanceBalance(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
		return
	}

	userID := c.GetString("user_id")

	// Get tenant ID from user
	balance, errorCode, err := h.modelProviderService.ShowInstanceBalance(providerName, instanceName, userID)
	if err != nil {
		common.ErrorWithCode(c, int(errorCode), err.Error())
		return
	}

	common.SuccessWithData(c, balance, "success")
}

func (h *ProviderHandler) CheckConnection(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	var req service.CheckConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, err.Error())
		return
	}

	userID := c.GetString("user_id")
	errCode, err := h.modelProviderService.CheckConnection(providerName, req.APIKey, req.Region, req.BaseURL, userID)
	if err != nil {
		common.ErrorWithCode(c, int(errCode), err.Error())
		return
	}

	common.SuccessWithMessage(c, "success")
}

func (h *ProviderHandler) CheckInstanceConnection(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
		return
	}

	userID := c.GetString("user_id")

	instanceInfo, code, err := h.modelProviderService.ShowProviderInstance(providerName, instanceName, userID)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	apikey, _ := instanceInfo["api_key"].(string)
	region, _ := instanceInfo["region"].(string)
	baseURL, _ := instanceInfo["base_url"].(string)

	// Get tenant ID from user
	errorCode, err := h.modelProviderService.CheckConnection(providerName, apikey, region, baseURL, userID)
	if err != nil {
		common.ErrorWithCode(c, int(errorCode), err.Error())
		return
	}

	common.SuccessWithMessage(c, "success")
}

func (h *ProviderHandler) ListTasks(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
		return
	}

	userID := c.GetString("user_id")

	// Get tenant ID from user
	listTaskResponse, errorCode, err := h.modelProviderService.ListTasks(providerName, instanceName, userID)
	if err != nil {
		common.ErrorWithCode(c, int(errorCode), err.Error())
		return
	}

	common.SuccessWithData(c, listTaskResponse, "success")
}

func (h *ProviderHandler) ShowTask(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
		return
	}

	taskID := c.Param("task_id")
	if taskID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Task ID is required")
		return
	}

	userID := c.GetString("user_id")

	// Get tenant ID from user
	taskResponse, errorCode, err := h.modelProviderService.ShowTask(providerName, instanceName, taskID, userID)
	if err != nil {
		common.ErrorWithCode(c, int(errorCode), err.Error())
		return
	}

	common.SuccessWithData(c, taskResponse, "success")
}

type AlterProviderInstanceRequest struct {
	InstanceName string `json:"instance_name"`
	APIKey       string `json:"api_key"`
}

func (h *ProviderHandler) AlterProviderInstance(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
		return
	}

	var req AlterProviderInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, err.Error())
		return
	}

	userID := c.GetString("user_id")
	if userID == "" {
		common.ErrorWithCode(c, int(common.CodeUnauthorized), "Unauthorized")
		return
	}

	code, err := h.modelProviderService.AlterProviderInstance(userID, providerName, instanceName, req.InstanceName, req.APIKey)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithMessage(c, "success")
}

type DropProviderInstanceRequest struct {
	Instances []string `json:"instances" binding:"required"`
}

func (h *ProviderHandler) DropProviderInstance(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}
	var req DropProviderInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}

	userID := c.GetString("user_id")

	code, err := h.modelProviderService.DropProviderInstances(providerName, userID, req.Instances)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithMessage(c, "success")
}

func (h *ProviderHandler) ListInstanceModels(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}
	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
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
			common.ErrorWithCode(c, int(common.CodeServerError), err.Error())
			return
		}

		common.SuccessWithData(c, modelList, "success")
		return
	}

	modelInstances, err := h.modelProviderService.ListInstanceModels(providerName, instanceName, c.GetString("user_id"))
	if err != nil {
		common.ErrorWithCode(c, int(common.CodeNotFound), err.Error())
		return
	}
	common.SuccessWithData(c, modelInstances, "success")
}

type EnableOrDisableModelRequest struct {
	ModelID string `json:"model_id"`
	Status  string `json:"status"`
}

func (h *ProviderHandler) EnableOrDisableModel(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}

	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
		return
	}

	var req EnableOrDisableModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}

	userID := c.GetString("user_id")
	modelID := strings.TrimSpace(req.ModelID)
	modelName := strings.TrimPrefix(c.Param("model_name"), "/")
	modelName = strings.TrimSpace(modelName)
	if modelName == "" && modelID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "model_name or model_id is required")
		return
	}

	status := strings.TrimSpace(req.Status)
	if status != "active" && status != "inactive" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Status must be active or inactive")
		return
	}

	code, err := h.modelProviderService.UpdateModelStatus(providerName, instanceName, modelName, userID, modelID, status)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithMessage(c, "success")
}

func prepareProviderInstance(providerName, instanceName, reqProviderName, reqInstanceName string) error {
	if providerName == "" {
		return errors.New("Provider name is required")
	}

	if instanceName == "" {
		return errors.New("Instance name is required")
	}

	if reqProviderName != "" && !strings.EqualFold(reqProviderName, providerName) {
		return errors.New("Provider name does not match path")
	}

	if reqInstanceName != "" && !strings.EqualFold(reqInstanceName, instanceName) {
		return errors.New("Instance name does not match path")
	}

	return nil
}

func prepareAddModelRequest(req *service.AddModelRequest, providerName, instanceName string) error {
	if err := prepareProviderInstance(providerName, instanceName, req.ProviderName, req.InstanceName); err != nil {
		return err
	}

	if len(req.Models) == 0 {
		return errors.New("Models are required")
	}

	for _, model := range req.Models {
		if model.ModelName == "" {
			return errors.New("Model name is required")
		}

		if len(model.ModelTypes) == 0 {
			return errors.New("Model type is required")
		}
	}

	req.ProviderName = providerName
	req.InstanceName = instanceName
	return nil
}

func (h *ProviderHandler) AddModel(c *gin.Context) {
	var req service.AddModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, err.Error())
		return
	}

	if err := prepareAddModelRequest(&req, c.Param("provider_name"), c.Param("instance_name")); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeDataError, nil, err.Error())
		return
	}

	userID := c.GetString("user_id")

	code, err := h.modelProviderService.AddModel(&req, userID)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.ErrorWithCode(c, int(code), "success")
}

type DropInstanceModelRequest struct {
	ModelIDs []string `json:"model_ids"`
	Models   []string `json:"models"`
}

func (h *ProviderHandler) DropInstanceModels(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
		return
	}
	instanceName := c.Param("instance_name")
	if instanceName == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
		return
	}

	var req DropInstanceModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}
	if len(req.ModelIDs) == 0 && len(req.Models) == 0 {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "model_ids or models is required")
		return
	}

	userID := c.GetString("user_id")

	code, err := h.modelProviderService.DropInstanceModels(providerName, instanceName, userID, req.ModelIDs, req.Models)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithMessage(c, "success")
}

type ChatToModelRequest struct {
	ProviderName *string                  `json:"provider_name"`
	InstanceName *string                  `json:"instance_name"`
	ModelName    *string                  `json:"model_name"`
	ModelID      *string                  `json:"model_id"`
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
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}

	if req.ModelID == nil {
		if req.ProviderName == nil || *req.ProviderName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
			return
		}

		if req.InstanceName == nil || *req.InstanceName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
			return
		}

		if req.ModelName == nil || *req.ModelName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Model name is required")
			return
		}
	} else {
		if *req.ModelID == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Model ID is empty")
			return
		}
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
		disableWriteDeadlineForSSE(c)
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

		// Stream response using sender function (the best performance, no channel)
		errorCode, err := h.modelProviderService.ChatToModelStreamWithSender(req.ProviderName, req.InstanceName, req.ModelName, req.ModelID, userID, messages, &apiConfig, &chatConfig, sender)

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
	response, errorCode, err = h.modelProviderService.ChatToModelWithMessages(req.ProviderName, req.InstanceName, req.ModelName, req.ModelID, userID, messages, &apiConfig, &chatConfig)

	if err != nil {
		common.ErrorWithCode(c, int(errorCode), err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":              0,
		"reasoning_content": response.ReasonContent,
		"answer":            response.Answer,
	})
}

type EmbedTextRequest struct {
	ProviderName *string  `json:"provider_name"`
	InstanceName *string  `json:"instance_name"`
	ModelName    *string  `json:"model_name"`
	ModelID      *string  `json:"model_id"`
	Texts        []string `json:"texts"`
	Dimension    int      `json:"dimension"`
}

func (h *ProviderHandler) EmbedText(c *gin.Context) {
	var req EmbedTextRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}

	if req.ModelID == nil {
		if req.ProviderName == nil || *req.ProviderName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
			return
		}

		if req.InstanceName == nil || *req.InstanceName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
			return
		}

		if req.ModelName == nil || *req.ModelName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Model name is required")
			return
		}
	} else {
		if *req.ModelID == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Model ID is empty")
			return
		}
	}

	userID := c.GetString("user_id")

	apiConfig := models.APIConfig{
		ApiKey: nil,
		Region: nil,
	}

	embeddingConfig := models.EmbeddingConfig{
		Dimension: req.Dimension,
	}

	// Non-stream response
	var response []models.EmbeddingData
	var errorCode common.ErrorCode
	var err error

	response, errorCode, err = h.modelProviderService.EmbedText(req.ProviderName, req.InstanceName, req.ModelName, req.ModelID, userID, req.Texts, &apiConfig, &embeddingConfig)
	if err != nil {
		common.ErrorWithCode(c, int(errorCode), err.Error())
		return
	}

	common.SuccessWithData(c, response, "success")
}

type RerankDocumentRequest struct {
	ProviderName *string  `json:"provider_name"`
	InstanceName *string  `json:"instance_name"`
	ModelName    *string  `json:"model_name"`
	ModelID      *string  `json:"model_id"`
	Query        string   `json:"query"`
	Documents    []string `json:"documents"`
	TopN         int      `json:"top_n"`
}

func (h *ProviderHandler) RerankDocument(c *gin.Context) {
	var req RerankDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}

	if req.ModelID == nil {
		if req.ProviderName == nil || *req.ProviderName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
			return
		}

		if req.InstanceName == nil || *req.InstanceName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
			return
		}

		if req.ModelName == nil || *req.ModelName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Model name is required")
			return
		}
	} else {
		if *req.ModelID == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Model ID is empty")
			return
		}
	}

	userID := c.GetString("user_id")

	apiConfig := models.APIConfig{
		ApiKey: nil,
		Region: nil,
	}

	rerankConfig := models.RerankConfig{
		TopN: req.TopN,
	}

	// Non-stream response
	var response *models.RerankResponse
	var errorCode common.ErrorCode
	var err error

	response, errorCode, err = h.modelProviderService.RerankDocument(req.ProviderName, req.InstanceName, req.ModelName, req.ModelID, userID, req.Query, req.Documents, &apiConfig, &rerankConfig)
	if err != nil {
		common.ErrorWithCode(c, int(errorCode), err.Error())
		return
	}
	common.SuccessWithData(c, response.Data, "success")
}

type TranscribeAudioRequest struct {
	ProviderName *string           `json:"provider_name"`
	InstanceName *string           `json:"instance_name"`
	ModelName    *string           `json:"model_name"`
	ModelID      *string           `json:"model_id"`
	File         *string           `json:"file"`
	Language     []string          `json:"language"`
	Prompt       int               `json:"prompt"`
	Stream       bool              `json:"stream"`
	ASRConfig    *models.ASRConfig `json:"asr_config"`
}

func (h *ProviderHandler) TranscribeAudio(c *gin.Context) {
	var req TranscribeAudioRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}

	if req.ModelID == nil {
		if req.ProviderName == nil || *req.ProviderName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
			return
		}

		if req.InstanceName == nil || *req.InstanceName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
			return
		}

		if req.ModelName == nil || *req.ModelName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Model name is required")
			return
		}
	} else {
		if *req.ModelID == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Model ID is empty")
			return
		}
	}

	userID := c.GetString("user_id")

	apiConfig := models.APIConfig{
		ApiKey: nil,
		Region: nil,
	}

	asrConfig := models.ASRConfig{}
	if req.ASRConfig != nil {
		asrConfig = *req.ASRConfig
	}

	// Check if it's a stream request
	if req.Stream {
		// Set SSE headers
		disableWriteDeadlineForSSE(c)
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

		// Stream response using sender function ( the best performance, no channel)
		errorCode, err := h.modelProviderService.TranscribeAudioStream(req.ProviderName, req.InstanceName, req.ModelName, req.ModelID, userID, req.File, &apiConfig, &asrConfig, sender)
		if errorCode != common.CodeSuccess {
			c.SSEvent("error", err.Error())
		}
		return
	}

	// Non-stream response
	var response *models.ASRResponse
	var errorCode common.ErrorCode
	var err error

	response, errorCode, err = h.modelProviderService.TranscribeAudio(req.ProviderName, req.InstanceName, req.ModelName, req.ModelID, userID, req.File, &apiConfig, &asrConfig)
	if err != nil {
		common.ErrorWithCode(c, int(errorCode), err.Error())
		return
	}

	common.SuccessWithData(c, response, "success")
}

type AudioSpeechRequest struct {
	ProviderName *string           `json:"provider_name"`
	InstanceName *string           `json:"instance_name"`
	ModelName    *string           `json:"model_name"`
	ModelID      *string           `json:"model_id"`
	Text         *string           `json:"text"`
	Stream       bool              `json:"stream"`
	TTSConfig    *models.TTSConfig `json:"tts_config"`
}

func (h *ProviderHandler) AudioSpeech(c *gin.Context) {
	var req AudioSpeechRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}

	if req.ModelID == nil {
		if req.ProviderName == nil || *req.ProviderName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
			return
		}

		if req.InstanceName == nil || *req.InstanceName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
			return
		}

		if req.ModelName == nil || *req.ModelName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Model name is required")
			return
		}
	} else {
		if *req.ModelID == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Model ID is empty")
			return
		}
	}

	userID := c.GetString("user_id")

	apiConfig := models.APIConfig{
		ApiKey: nil,
		Region: nil,
	}

	ttsConfig := models.TTSConfig{}
	if req.TTSConfig != nil {
		ttsConfig = *req.TTSConfig
	}

	// Check if it's a stream request
	if req.Stream {
		// Set SSE headers
		disableWriteDeadlineForSSE(c)
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

		// Stream response using sender function ( the best performance, no channel)
		errorCode, err := h.modelProviderService.AudioSpeechStream(req.ProviderName, req.InstanceName, req.ModelName, req.ModelID, userID, req.Text, &apiConfig, &ttsConfig, sender)
		if errorCode != common.CodeSuccess {
			c.SSEvent("error", err.Error())
		}
		return
	}

	// Non-stream response
	var response *models.TTSResponse
	var errorCode common.ErrorCode
	var err error

	response, errorCode, err = h.modelProviderService.AudioSpeech(req.ProviderName, req.InstanceName, req.ModelName, req.ModelID, userID, req.Text, &apiConfig, &ttsConfig)
	if err != nil {
		common.ErrorWithCode(c, int(errorCode), err.Error())
		return
	}

	common.SuccessWithData(c, response, "success")
}

type OCRFileRequest struct {
	ProviderName *string `json:"provider_name"`
	InstanceName *string `json:"instance_name"`
	ModelName    *string `json:"model_name"`
	ModelID      *string `json:"model_id"`
	Content      []byte  `json:"content"`
	URL          *string `json:"url"`
}

func (h *ProviderHandler) OCRFile(c *gin.Context) {
	var req OCRFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}

	if req.ModelID == nil {
		if req.ProviderName == nil || *req.ProviderName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
			return
		}

		if req.InstanceName == nil || *req.InstanceName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
			return
		}

		if req.ModelName == nil || *req.ModelName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Model name is required")
			return
		}
	} else {
		if *req.ModelID == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Model ID is empty")
			return
		}
	}

	userID := c.GetString("user_id")

	apiConfig := models.APIConfig{
		ApiKey: nil,
		Region: nil,
	}

	OCRConfig := models.OCRConfig{}

	// Non-stream response
	var response *models.OCRFileResponse
	var errorCode common.ErrorCode
	var err error

	response, errorCode, err = h.modelProviderService.OCRFile(req.ProviderName, req.InstanceName, req.ModelName, req.ModelID, userID, req.Content, req.URL, &apiConfig, &OCRConfig)
	if err != nil {
		common.ErrorWithCode(c, int(errorCode), err.Error())
		return
	}

	common.SuccessWithData(c, response, "success")
}

type ParseFileRequest struct {
	ProviderName *string `json:"provider_name"`
	InstanceName *string `json:"instance_name"`
	ModelName    *string `json:"model_name"`
	ModelID      *string `json:"model_id"`
	Content      []byte  `json:"content"`
	URL          *string `json:"url"`
}

func (h *ProviderHandler) ParseFile(c *gin.Context) {
	var req ParseFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}

	if req.ModelID == nil {
		if req.ProviderName == nil || *req.ProviderName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Provider name is required")
			return
		}

		if req.InstanceName == nil || *req.InstanceName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Instance name is required")
			return
		}

		if req.ModelName == nil || *req.ModelName == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Model name is required")
			return
		}
	} else {
		if *req.ModelID == "" {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, "Model ID is empty")
			return
		}
	}

	userID := c.GetString("user_id")

	apiConfig := models.APIConfig{
		ApiKey: nil,
		Region: nil,
	}

	parseFileConfig := models.ParseFileConfig{}

	// Non-stream response
	var response *models.ParseFileResponse
	var errorCode common.ErrorCode
	var err error

	response, errorCode, err = h.modelProviderService.ParseFile(req.ProviderName, req.InstanceName, req.ModelName, req.ModelID, userID, req.Content, req.URL, &apiConfig, &parseFileConfig)
	if err != nil {
		common.ErrorWithCode(c, int(errorCode), err.Error())
		return
	}

	common.SuccessWithData(c, response, "success")
}

// ListTenantAddedModels is the response handler for GET /api/v1/models.
// It is the Go port of Python's
// api/apps/restful_apis/models_api.py:get_added_models and feeds
// web/src/hooks/use-llm-request.tsx → useFetchAllAddedModels. The data
// shape is the array form (one row per (provider × instance × llm) with
// model_type: string[]), matching the IAddedModel interface in
// web/src/interfaces/database/llm.ts:64-71.
//
// The previous contract routed this path to TenantHandler.GetModels →
// TenantService.ListTenantDefaultModels, which only enumerates the 6-7
// default tenant fields and returned `[]` for any tenant without
// defaults, breaking the front-end's "View Models" list. The Go port
// has no writers for tenant_model, so this endpoint must be driven by
// the factory catalog cross-referenced with the tenant's instance list —
// see service.ModelProviderService.ListTenantAddedModels.
func (h *ProviderHandler) ListTenantAddedModels(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	modelType := c.Query("type")

	addedModels, code, err := h.modelProviderService.ListTenantAddedModels(user.ID, modelType)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithData(c, addedModels, "success")
}
