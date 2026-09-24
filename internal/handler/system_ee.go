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

	"github.com/gin-gonic/gin"
)

// GetEnableAdmin admin access endpoint
// @Description Enable admin access endpoint
// @Tags system
// @Produce plain
// @Router /api/v1/system/enable-admin [GET]
func (h *SystemHandler) GetEnableAdmin(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "EnableAdmin not implemented")
}

// GetCores /api/v1/system/cores [GET]
func (h *SystemHandler) GetCores(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "Get cores not implemented")
}

// SetCores /api/v1/system/cores [PUT]
func (h *SystemHandler) SetCores(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "Set cores not implemented")
}

// GetMemory /api/v1/system/memory [GET]
func (h *SystemHandler) GetMemory(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "Get memory not implemented")
}

// SetMemory /api/v1/system/memory [PUT]
func (h *SystemHandler) SetMemory(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "Set memory not implemented")
}

// GetConcurrency /api/v1/system/concurrency [GET]
func (h *SystemHandler) GetConcurrency(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "Get concurrency not implemented")
}

// SetConcurrency /api/v1/system/concurrency [PUT]
func (h *SystemHandler) SetConcurrency(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "Set concurrency not implemented")
}
