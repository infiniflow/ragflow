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
	"strconv"

	"ragflow/internal/common"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

// CompilationTemplateGroupHandler serves the knowledge-compilation template
// group REST APIs. It mirrors the Python compilation_template_group_api blueprint.
type CompilationTemplateGroupHandler struct {
	compilationTemplateGroupService *service.CompilationTemplateGroupService
}

// NewCompilationTemplateGroupHandler creates a CompilationTemplateGroupHandler.
func NewCompilationTemplateGroupHandler(svc *service.CompilationTemplateGroupService) *CompilationTemplateGroupHandler {
	return &CompilationTemplateGroupHandler{compilationTemplateGroupService: svc}
}

// List returns the tenant's compilation template groups with their nested
// templates and pagination.
//
//	@Summary List compilation template groups
//	@Tags compilation template group
//	@Security ApiKeyAuth
//	@Param keywords query string false "name keyword filter"
//	@Param scope query string false "scope filter (file|dataset)"
//	@Param page query int false "page number" default(1)
//	@Param page_size query int false "items per page" default(30)
//	@Param orderby query string false "order field" default(create_time)
//	@Param desc query bool false "descending" default(true)
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-template-groups [get]
func (h *CompilationTemplateGroupHandler) List(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ErrorWithCode(c, code, msg)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "30"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 30
	}
	keywords := c.Query("keywords")
	scope := c.Query("scope")
	orderby := c.DefaultQuery("orderby", "create_time")
	desc := c.DefaultQuery("desc", "true") != "false"

	groups, err := h.compilationTemplateGroupService.ListSaved(c.Request.Context(), user.ID, keywords, scope, orderby, desc)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	total := len(groups)
	if page > 0 && pageSize > 0 {
		start := (page - 1) * pageSize
		if start >= total {
			groups = []*service.GroupListItem{}
		} else {
			end := start + pageSize
			if end > total {
				end = total
			}
			groups = groups[start:end]
		}
	}
	common.SuccessWithData(c, gin.H{"groups": groups, "total": total}, "success")
}

// Get returns a single compilation template group.
//
//	@Summary Get compilation template group
//	@Tags compilation template group
//	@Security ApiKeyAuth
//	@Param group_id path string true "group id"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-template-groups/{group_id} [get]
func (h *CompilationTemplateGroupHandler) Get(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ErrorWithCode(c, code, msg)
		return
	}
	id := c.Param("group_id")
	group, err := h.compilationTemplateGroupService.GetSaved(c.Request.Context(), user.ID, id)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	if group == nil {
		common.ResponseWithCodeData(c, common.CodeNotFound, nil, "Cannot find compilation template group "+id+".")
		return
	}
	common.SuccessWithData(c, group, "success")
}

// Save creates a new compilation template group with its child templates.
//
//	@Summary Save compilation template group
//	@Tags compilation template group
//	@Security ApiKeyAuth
//	@Param request body service.GroupRequest true "group payload"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-template-groups [post]
func (h *CompilationTemplateGroupHandler) Save(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ErrorWithCode(c, code, msg)
		return
	}
	var req service.GroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Invalid request: "+err.Error())
		return
	}
	if err := service.ValidateGroupRequest(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}
	nameExists, err := h.compilationTemplateGroupService.NameExists(c.Request.Context(), user.ID, req.Name)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	if nameExists {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "Duplicated compilation template group name.")
		return
	}
	group, err := h.compilationTemplateGroupService.CreateGroup(c.Request.Context(), user.ID, &req)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}
	common.SuccessWithData(c, group, "success")
}

// Update applies a partial update to a compilation template group.
//
//	@Summary Update compilation template group
//	@Tags compilation template group
//	@Security ApiKeyAuth
//	@Param group_id path string true "group id"
//	@Param request body service.GroupRequest true "group payload"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-template-groups/{group_id} [put]
func (h *CompilationTemplateGroupHandler) Update(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ErrorWithCode(c, code, msg)
		return
	}
	id := c.Param("group_id")
	var req service.GroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Invalid request: "+err.Error())
		return
	}
	if err := service.ValidateGroupRequest(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}
	if req.Name != "" {
		nameExists, nerr := h.compilationTemplateGroupService.NameExists(c.Request.Context(), user.ID, req.Name, id)
		if nerr != nil {
			common.ResponseWithCodeData(c, common.CodeExceptionError, nil, nerr.Error())
			return
		}
		if nameExists {
			common.ResponseWithCodeData(c, common.CodeDataError, nil, "Duplicated compilation template group name.")
			return
		}
	}
	group, err := h.compilationTemplateGroupService.UpdateGroup(c.Request.Context(), user.ID, id, &req)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}
	if group == nil {
		common.ResponseWithCodeData(c, common.CodeNotFound, nil, "Cannot find compilation template group "+id+".")
		return
	}
	common.SuccessWithData(c, group, "success")
}

// Delete soft-deletes a compilation template group and its children.
//
//	@Summary Delete compilation template group
//	@Tags compilation template group
//	@Security ApiKeyAuth
//	@Param group_id path string true "group id"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-template-groups/{group_id} [delete]
func (h *CompilationTemplateGroupHandler) Delete(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ErrorWithCode(c, code, msg)
		return
	}
	id := c.Param("group_id")
	ok, err := h.compilationTemplateGroupService.DeleteGroup(c.Request.Context(), user.ID, id)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	if !ok {
		common.ResponseWithCodeData(c, common.CodeNotFound, nil, "Cannot find compilation template group "+id+".")
		return
	}
	common.SuccessWithData(c, true, "success")
}
