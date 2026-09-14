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

// CompilationTemplateHandler serves the knowledge-compilation template REST
// APIs. It mirrors the Python compilation_template_api blueprint.
type CompilationTemplateHandler struct {
	compilationTemplateService *service.CompilationTemplateService
}

// NewCompilationTemplateHandler creates a CompilationTemplateHandler.
func NewCompilationTemplateHandler(svc *service.CompilationTemplateService) *CompilationTemplateHandler {
	return &CompilationTemplateHandler{compilationTemplateService: svc}
}

// ListBuiltins returns the built-in template palette used by the frontend
// "Add template group" panel.
//
//	@Summary List built-in compilation templates
//	@Tags compilation template
//	@Security ApiKeyAuth
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-templates/builtins [get]
func (h *CompilationTemplateHandler) ListBuiltins(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ErrorWithCode(c, code, msg)
		return
	}
	templates, err := h.compilationTemplateService.ListBuiltins(c.Request.Context(), user.ID)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	common.SuccessWithData(c, templates, "success")
}

// ListWikiPresets returns the wiki page-structure presets loaded from YAML.
//
//	@Summary List wiki presets
//	@Tags compilation template
//	@Security ApiKeyAuth
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-templates/wiki-presets [get]
func (h *CompilationTemplateHandler) ListWikiPresets(c *gin.Context) {
	_, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ErrorWithCode(c, code, msg)
		return
	}
	presets, err := h.compilationTemplateService.LoadWikiPresets()
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	common.SuccessWithData(c, presets, "success")
}
