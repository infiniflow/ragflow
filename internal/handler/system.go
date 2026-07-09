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
	"ragflow/internal/server"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

// SystemHandler system handler
type SystemHandler struct {
	systemService *service.SystemService
}

// NewSystemHandler create system handler
func NewSystemHandler(systemService *service.SystemService) *SystemHandler {
	return &SystemHandler{
		systemService: systemService,
	}
}

// Ping health check endpoint
// @Summary Ping
// @Description Simple ping endpoint
// @Tags system
// @Produce plain
// @Success 200 {string} string "pong"
// @Router /v1/system/ping [get]
func (h *SystemHandler) Ping(c *gin.Context) {
	c.String(http.StatusOK, "pong")
}

// Health check
func (h *SystemHandler) Health(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": "ok",
	})
}

// Healthz reports dependency health in the Python-compatible format.
func (h *SystemHandler) Healthz(c *gin.Context) {
	result, allOK := h.systemService.Healthz(c.Request.Context())
	statusCode := http.StatusOK
	if !allOK {
		statusCode = http.StatusInternalServerError
	}
	c.JSON(statusCode, result)
}

// GetConfig get system configuration
// @Summary Get System Configuration
// @Description Get system configuration including register enabled status
// @Tags system
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /v1/system/config [get]
func (h *SystemHandler) GetConfig(c *gin.Context) {
	config, err := h.systemService.GetConfig()
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, 500, nil, "Failed to get system configuration")
		return
	}

	common.SuccessWithData(c, config, "success")
}

// GetConfigs get all system configurations
// @Summary Get All System Configurations
// @Description Get all system configurations from globalConfig
// @Tags system
// @Accept json
// @Produce json
// @Success 200 {object} config.Config
// @Router /v1/system/configs [get]
func (h *SystemHandler) GetConfigs(c *gin.Context) {
	cfg := server.GetConfig()
	if cfg == nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, 500, nil, "Configuration not initialized")
		return
	}

	common.SuccessWithData(c, cfg, "success")
}

// GetStatus get RAGFlow status
func (h *SystemHandler) GetStatus(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	status, err := h.systemService.GetStatus()
	if err != nil {
		jsonInternalError(c, err)
		return
	}

	common.SuccessWithData(c, status, "success")
}

// GetVersion get RAGFlow version
// @Summary Get RAGFlow Version
// @Description Get the current version of the application
// @Tags system
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]interface{}
// @Router /v1/system/version [get]
func (h *SystemHandler) GetVersion(c *gin.Context) {
	version, err := h.systemService.GetVersion()
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, 500, nil, "Failed to get version")
		return
	}

	common.SuccessWithData(c, version.Version, "success")
}

// GetLogLevel returns the current log level. The response uses the
// {"level": <value>} shape — the same shape the admin handler's
// /admin/log_level endpoint returns — so the two log endpoints stay
// in lockstep. Per-package level entries that the old pkgLevels
// table carried (e.g. "peewee", "pdfminer") were inert for the Go
// side and are no longer returned.
func (h *SystemHandler) GetLogLevel(c *gin.Context) {
	common.SuccessWithData(c, gin.H{"level": common.GetLevel()}, "success")
}

// SetLogLevelRequest set log level request. PkgName is accepted for
// backward compatibility with clients that previously targeted
// per-package levels; it is silently ignored. Only the global level
// can be set on the Go side.
type SetLogLevelRequest struct {
	PkgName string `json:"pkg_name"`
	Level   string `json:"level" binding:"required"`
}

// SetLogLevel sets the log level at runtime.
//
// The "pkg_name and level are required" error message is preserved
// verbatim from the pre-Go-port handler so existing clients that
// inspect `message` on the missing-field path keep working. On the
// Go side `pkg_name` is no longer required (per-package filtering
// is gone), but the message wording is unchanged for backward
// compatibility — only `level` is enforced by binding; `pkg_name`
// is accepted but ignored.
func (h *SystemHandler) SetLogLevel(c *gin.Context) {
	var req SetLogLevelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, int(common.CodeDataError), "pkg_name and level are required")
		return
	}

	if err := common.SetLevel(req.Level); err != nil {
		common.ErrorWithCode(c, int(common.CodeDataError), "Invalid log level: "+req.Level)
		return
	}

	if config := server.GetConfig(); config != nil {
		config.Log.Level = common.GetLevel()
	}

	common.SuccessWithData(c, gin.H{"level": req.Level}, "SUCCESS")
}

// ListVariables handle list variables
func (h *SystemHandler) ListVariables(c *gin.Context) {
	variables, err := h.systemService.ListAllVariables()
	if err != nil {
		common.ErrorWithCode(c, 500, err.Error())
		return
	}

	common.SuccessWithData(c, variables, "SUCCESS")
}

// SetVariableHTTPRequest set variable request
type SetVariableHTTPRequest struct {
	VarName  string `json:"var_name" binding:"required"`
	VarValue string `json:"var_value" binding:"required"`
}

// SetVariable handle set variable
// Python logic: update or create a system setting with the given name and value
func (h *SystemHandler) SetVariable(c *gin.Context) {
	var req SetVariableHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, 400, "Var name is required")
		return
	}

	if req.VarName == "" {
		common.ErrorWithCode(c, 400, "Var name is required")
		return
	}

	if req.VarValue == "" {
		common.ErrorWithCode(c, 400, "Var value is required")
		return
	}

	if err := h.systemService.SetVariable(req.VarName, req.VarValue); err != nil {
		common.ErrorWithCode(c, 500, err.Error())
		return
	}

	common.SuccessNoData(c, "SUCCESS")
}

func (h *SystemHandler) ShowVariable(c *gin.Context) {
	encodedVarName := c.Param("var_name")

	varName, err := common.DecodeFromBase64(encodedVarName)
	if err != nil {
		common.ErrorWithCode(c, 400, err.Error())
		return
	}
	if varName == "" {
		common.ErrorWithCode(c, 400, "Var name is required")
		return
	}

	variable, err := h.systemService.ShowVariable(varName)
	if err != nil {
		common.ErrorWithCode(c, 500, err.Error())
		return
	}

	common.SuccessWithData(c, variable, "SUCCESS")
}

// ListEnvironments handle list environments
func (h *SystemHandler) ListEnvironments(c *gin.Context) {
	environments, err := h.systemService.ListEnvironments()
	if err != nil {
		common.ErrorWithCode(c, 500, err.Error())
		return
	}

	common.SuccessWithData(c, environments, "SUCCESS")
}
