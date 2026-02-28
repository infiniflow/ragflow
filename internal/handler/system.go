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

	"github.com/gin-gonic/gin"

	"ragflow/internal/config"
	"ragflow/internal/service"
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
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to get system configuration",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    config,
	})
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
	cfg := config.Get()
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Configuration not initialized",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    cfg,
	})
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
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to get version",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    version,
	})
}
