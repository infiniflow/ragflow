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
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

type connectorServiceIface interface {
	ListConnectors(userID string) (*service.ListConnectorsResponse, error)
	CreateConnector(userID string, req *service.CreateConnectorRequest) (*entity.Connector, error)
	GetConnector(connectorID, userID string) (*entity.Connector, common.ErrorCode, error)
	ListLog(connectorID, userID string, page, pageSize int) ([]*entity.ConnectorSyncLog, int64, common.ErrorCode, error)
	DeleteConnector(connectorID, userID string) (bool, common.ErrorCode, error)
	RebuildConnector(connectorID, userID, kbID string) (bool, common.ErrorCode, error)
	TestConnector(connectorID, userID string) error
}

// ConnectorHandler connector handler
type ConnectorHandler struct {
	connectorService connectorServiceIface
	userService      *service.UserService
}

// NewConnectorHandler create connector handler
func NewConnectorHandler(connectorService *service.ConnectorService, userService *service.UserService) *ConnectorHandler {
	return &ConnectorHandler{
		connectorService: connectorService,
		userService:      userService,
	}
}

// ListConnectors list connectors
// @Summary List Connectors
// @Description Get list of connectors for the current user (equivalent to Python's list_connector)
// @Tags connector
// @Accept json
// @Produce json
// @Success 200 {object} service.ListConnectorsResponse
// @Router /connector/list [get]
func (h *ConnectorHandler) ListConnectors(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// List connectors
	result, err := h.connectorService.ListConnectors(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    result.Connectors,
		"message": "success",
	})
}

// connectorErrorResponse maps service sentinel errors to the response codes used
// by the Python connector_api, and writes the JSON response. It returns true when
// the error was handled.
func connectorErrorResponse(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, service.ErrConnectorNoAuth):
		c.JSON(http.StatusOK, gin.H{"code": common.CodeAuthenticationError, "data": false, "message": "No authorization."})
	case errors.Is(err, service.ErrConnectorNotFound):
		c.JSON(http.StatusOK, gin.H{"code": common.CodeDataError, "data": nil, "message": "Can't find this Connector!"})
	case errors.Is(err, service.ErrConnectorTestUnsupported):
		c.JSON(http.StatusOK, gin.H{"code": common.CodeArgumentError, "data": false, "message": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"code": common.CodeServerError, "data": nil, "message": err.Error()})
	}
	return true
}

// GetConnector get connector
// @Summary Get Connector
// @Description Get connector details for the current user
// @Tags connector
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/connectors/{connector_id} [get]
func (h *ConnectorHandler) GetConnector(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	connector, code, err := h.connectorService.GetConnector(c.Param("connector_id"), user.ID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    connector,
		"message": "success",
	})
}

// ListLogs list connector sync logs.
// @Summary List Connector Logs
// @Description List sync logs for a connector the current user can access
// @Tags connector
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/connectors/{connector_id}/logs [get]
func (h *ConnectorHandler) ListLogs(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	page := 1
	if rawPage := strings.TrimSpace(c.DefaultQuery("page", "1")); rawPage != "" {
		parsedPage, err := strconv.Atoi(rawPage)
		if err != nil {
			jsonError(c, common.CodeArgumentError, "page must be an integer")
			return
		}
		page = parsedPage
	}

	pageSize := 15
	if rawPageSize := strings.TrimSpace(c.DefaultQuery("page_size", "15")); rawPageSize != "" {
		parsedPageSize, err := strconv.Atoi(rawPageSize)
		if err != nil {
			jsonError(c, common.CodeArgumentError, "page_size must be an integer")
			return
		}
		pageSize = parsedPageSize
	}

	logs, total, code, err := h.connectorService.ListLog(c.Param("connector_id"), user.ID, page, pageSize)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}
	if logs == nil {
		logs = []*entity.ConnectorSyncLog{}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    gin.H{"total": total, "logs": logs},
		"message": "success",
	})
}

// CreateConnector create connector
// @Summary create Connectors
// @Description create a connectors for the current user
// @Tags connector
// @Accept json
// @Produce json
// @Success 200 {object} service.ListConnectorsResponse
// @Router /connector/ [post]
func (h *ConnectorHandler) CreateConnector(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req service.CreateConnectorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"data":    nil,
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"data":    nil,
			"message": "name is required",
		})
		return
	}
	if strings.TrimSpace(req.Source) == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"data":    nil,
			"message": "source is required",
		})
		return
	}
	if req.Config == nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"data":    nil,
			"message": "config is required",
		})
		return
	}

	connector, err := h.connectorService.CreateConnector(user.ID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    common.CodeServerError,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    connector,
		"message": "success",
	})
}

// TestConnector validates an accessible connector's stored credentials.
// @Summary Test Connector
// @Description Validate connector credentials / connection (equivalent to Python's test_connector)
// @Tags connector
// @Produce json
// @Param connector_id path string true "connector ID"
// @Router /api/v1/connectors/{connector_id}/test [post]
func (h *ConnectorHandler) TestConnector(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	connectorID := c.Param("connector_id")
	if connectorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeBadRequest, "data": nil, "message": "connector_id is required"})
		return
	}

	err := h.connectorService.TestConnector(connectorID, user.ID)
	if errors.Is(err, service.ErrConnectorTestUnsupported) {
		connectorErrorResponse(c, err)
		return
	}
	if err != nil && !errors.Is(err, service.ErrConnectorNoAuth) && !errors.Is(err, service.ErrConnectorNotFound) {
		// Validation failure (e.g. missing credentials): mirror Python's DATA_ERROR with data=false.
		c.JSON(http.StatusOK, gin.H{"code": common.CodeDataError, "data": false, "message": err.Error()})
		return
	}
	if connectorErrorResponse(c, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": true, "message": "success"})
}

// DeleteConnector delete connector
// @Description Detele Connector
// @Tags connector
// @Accept json
// @Produce json
func (h *ConnectorHandler) DeleteConnector(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	ok, code, err := h.connectorService.DeleteConnector(c.Param("connector_id"), user.ID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    ok,
		"message": "success",
	})
}

// RebuildConnector rebuild connector
// @Summary Rebuild Connector
// @Description Trigger a rebuild for an accessible connector and knowledge base
// @Tags connector
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /connector/:connector_id/rebuild [post]
func (h *ConnectorHandler) RebuildConnector(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	// Parse request body to get kb_id
	var req struct {
		KbID string `json:"kb_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"data":    nil,
			"message": "required argument is missing: kb_id",
		})
		return
	}

	if strings.TrimSpace(req.KbID) == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"data":    nil,
			"message": "kb_id cannot be empty",
		})
		return
	}

	ok, code, err := h.connectorService.RebuildConnector(c.Param("connector_id"), user.ID, req.KbID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    ok,
		"message": "success",
	})
}
