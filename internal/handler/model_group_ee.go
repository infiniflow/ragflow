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

type ModelGroupHandler struct {
}

func NewModelGroupHandler(modelProviderService *service.ModelProviderService) *ModelGroupHandler {
	return &ModelGroupHandler{}
}

func (h *ModelGroupHandler) CreateModelGroup(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "CreateModelGroup not implemented")
	return
}

func (h *ModelGroupHandler) DeleteModelGroup(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "DeleteModelGroup not implemented")
	return
}

func (h *ModelGroupHandler) ShowModelGroup(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "ShowModelGroup not implemented")
	return
}

func (h *ModelGroupHandler) AddModelToGroup(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "AddModelToGroup not implemented")
	return
}

func (h *ModelGroupHandler) RemoveModelFromGroup(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "RemoveModelFromGroup not implemented")
	return
}
