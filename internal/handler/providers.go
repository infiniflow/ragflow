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
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/service"
	"strings"

	"github.com/gin-gonic/gin"
)

// ProviderHandler provider handler
type ProviderHandler struct {
	userService *service.UserService
}

// NewProviderHandler create provider handler
func NewProviderHandler(userService *service.UserService) *ProviderHandler {
	return &ProviderHandler{
		userService: userService,
	}
}

func (h *ProviderHandler) ListPoolProviders(c *gin.Context) {

	keywords := ""
	if queryKeywords := c.Query("available"); queryKeywords != "" {
		keywords = queryKeywords
	}

	// convert keywords to small case
	keywords = strings.ToLower(keywords)
	if keywords == "true" {
		providers, err := dao.GetModelProviderManager().ListProviders()
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
			"data":    providers,
		})
	}
}

func (h *ProviderHandler) ShowPoolProvider(c *gin.Context) {
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

func (h *ProviderHandler) ListPoolModels(c *gin.Context) {
	providerName := c.Param("provider_name")
	if providerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Provider name is required",
		})
		return
	}
	models, err := dao.GetModelProviderManager().ListModels(providerName)
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
		"data":    models,
	})
}

func (h *ProviderHandler) ShowPoolModel(c *gin.Context) {
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
