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
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

// ProviderHandler provider handler
type ModelHandler struct {
	modelProviderService *service.ModelProviderService
}

// NewProviderHandler create provider handler
func NewModelHandler(modelProviderService *service.ModelProviderService) *ModelHandler {
	return &ModelHandler{
		modelProviderService: modelProviderService,
	}
}

type ShowModelRequest struct {
	ModelName *string `json:"model_name"`
	Page      int     `json:"page"`
	PageSize  int     `json:"page_size"`
}

func (h *ModelHandler) ListAllModels(c *gin.Context) {

	var req ShowModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		println("JSON bind error: %v (type: %T)", err, err)
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	if req.ModelName == nil {
		// list models
		page := req.Page
		pageSize := req.PageSize

		// list tenant models
		models, err := h.modelProviderService.ListAllModels(page, pageSize)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeDataError,
				"message": err.Error(),
				"data":    nil,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "success",
			"data":    models,
		})
	} else {
		// show model
		model, err := h.modelProviderService.ShowModel(*req.ModelName)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeDataError,
				"message": err.Error(),
				"data":    nil,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "success",
			"data":    model,
		})
	}

	return
}
