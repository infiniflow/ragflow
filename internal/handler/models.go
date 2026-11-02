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
	"strconv"

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

	page := 0
	if v := c.Query("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 0
	if v := c.Query("page_size"); v != "" {
		if ps, err := strconv.Atoi(v); err == nil && ps > 0 {
			pageSize = ps
		}
	}

	// list tenant models
	models, err := h.modelProviderService.ListAllModels(page, pageSize)
	if err != nil {
		common.Respond(c, common.Response{Code: common.CodeDataError, Message: err.Error()})
		return
	}

	common.Respond(c, common.Response{Code: common.CodeSuccess, Message: "success", Data: models})
	return
}

func (h *ModelHandler) ShowModel(c *gin.Context) {
	encodedModelName := c.Param("model_name")
	if encodedModelName == "" {
		common.Respond(c, common.Response{Code: common.CodeBadRequest, Message: "Encoded model name is empty"})
		return
	}

	decodedModelName, err := common.DecodeFromBase64(encodedModelName)
	if err != nil {
		common.Respond(c, common.Response{Code: common.CodeBadRequest, Message: err.Error()})
		return
	}
	if decodedModelName == "" {
		common.Respond(c, common.Response{Code: common.CodeBadRequest, Message: "Decoded model name is empty"})
		return
	}

	// Get model
	model, err := h.modelProviderService.ShowModel(decodedModelName)
	if err != nil {
		common.Respond(c, common.Response{Code: common.CodeDataError, Message: err.Error()})
		return
	}

	common.Respond(c, common.Response{Code: common.CodeSuccess, Message: "success", Data: model})
}
