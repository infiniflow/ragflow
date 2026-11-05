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
	"ragflow/internal/common"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

// ModelHandler ProviderHandler provider handler
type ModelHandler struct {
	modelProviderService *service.ModelProviderService
}

// NewModelHandler NewProviderHandler create provider handler
func NewModelHandler(modelProviderService *service.ModelProviderService) *ModelHandler {
	return &ModelHandler{
		modelProviderService: modelProviderService,
	}
}

func (h *ModelHandler) ListAllModels(c *gin.Context) {

	page, pageSize, err := common.ParsePagination(c.Request.URL.Query())
	if err != nil {
		common.RespondError(c, common.CodeArgumentError, err.Error())
		return
	}

	// list tenant models
	models, err := h.modelProviderService.ListAllModels(page, pageSize)
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}

	common.RespondSuccessList(c, models, "success")
	return
}

func (h *ModelHandler) ShowModel(c *gin.Context) {
	encodedModelName := c.Param("model_name")
	if encodedModelName == "" {
		common.RespondError(c, common.CodeBadRequest, "Encoded model name is empty")
		return
	}

	decodedModelName, err := common.DecodeFromBase64(encodedModelName)
	if err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}
	if decodedModelName == "" {
		common.RespondError(c, common.CodeBadRequest, "Decoded model name is empty")
		return
	}

	// Get model
	model, err := h.modelProviderService.ShowModel(decodedModelName)
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}

	common.RespondSuccess(c, model, "success")
}
