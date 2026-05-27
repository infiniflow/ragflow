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
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"ragflow/internal/common"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

// ConnectorHandler connector handler
type ConnectorHandler struct {
	connectorService *service.ConnectorService
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

// CreateConnector creates a connector for the current tenant.
// @Summary Create Connector
// @Description Create a connector (equivalent to Python's create_connector)
// @Tags connector
// @Accept json
// @Produce json
// @Param request body service.CreateConnectorRequest true "connector creation parameters"
// @Router /api/v1/connectors [post]
func (h *ConnectorHandler) CreateConnector(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req service.CreateConnectorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeBadRequest, "data": nil, "message": "Invalid request body: " + err.Error()})
		return
	}

	conn, err := h.connectorService.CreateConnector(user.ID, &req)
	if connectorErrorResponse(c, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": conn, "message": "success"})
}

// GetConnector returns one connector if the current user can access it.
// @Summary Get Connector
// @Description Get connector details (equivalent to Python's get_connector)
// @Tags connector
// @Produce json
// @Param connector_id path string true "connector ID"
// @Router /api/v1/connectors/{connector_id} [get]
func (h *ConnectorHandler) GetConnector(c *gin.Context) {
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

	conn, err := h.connectorService.GetConnector(connectorID, user.ID)
	if connectorErrorResponse(c, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": conn, "message": "success"})
}

// UpdateConnector updates an accessible connector's polling configuration.
// @Summary Update Connector
// @Description Update a connector (equivalent to Python's update_connector)
// @Tags connector
// @Accept json
// @Produce json
// @Param connector_id path string true "connector ID"
// @Param request body service.UpdateConnectorRequest true "connector update parameters"
// @Router /api/v1/connectors/{connector_id} [patch]
func (h *ConnectorHandler) UpdateConnector(c *gin.Context) {
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

	req, err := bindConnectorUpdate(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeBadRequest, "data": nil, "message": "Invalid request body: " + err.Error()})
		return
	}

	conn, err := h.connectorService.UpdateConnector(connectorID, user.ID, req)
	if connectorErrorResponse(c, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": conn, "message": "success"})
}

// bindConnectorUpdate parses the update payload, unwrapping a top-level {"data": {...}}
// envelope when present (mirroring Python's update_connector handling).
func bindConnectorUpdate(c *gin.Context) (*service.UpdateConnectorRequest, error) {
	body, err := c.GetRawData()
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return &service.UpdateConnectorRequest{}, nil
	}

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	payload := body
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.Data) > 0 {
		payload = envelope.Data
	}

	var req service.UpdateConnectorRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// DeleteConnector deletes an accessible connector after cancelling its sync tasks.
// @Summary Delete Connector
// @Description Delete a connector (equivalent to Python's rm_connector)
// @Tags connector
// @Produce json
// @Param connector_id path string true "connector ID"
// @Router /api/v1/connectors/{connector_id} [delete]
func (h *ConnectorHandler) DeleteConnector(c *gin.Context) {
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

	err := h.connectorService.DeleteConnector(connectorID, user.ID)
	if connectorErrorResponse(c, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": true, "message": "success"})
}

// ListLogs lists sync logs for an accessible connector.
// @Summary List Connector Sync Logs
// @Description List sync logs for a connector (equivalent to Python's list_logs)
// @Tags connector
// @Produce json
// @Param connector_id path string true "connector ID"
// @Param page query int false "page number (default 1)"
// @Param page_size query int false "items per page (default 15)"
// @Router /api/v1/connectors/{connector_id}/logs [get]
func (h *ConnectorHandler) ListLogs(c *gin.Context) {
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

	page := 1
	if v := c.Query("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			page = p
		}
	}
	pageSize := 15
	if v := c.Query("page_size"); v != "" {
		if ps, err := strconv.Atoi(v); err == nil && ps > 0 {
			pageSize = ps
		}
	}

	result, err := h.connectorService.ListLogs(connectorID, user.ID, page, pageSize)
	if connectorErrorResponse(c, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": result, "message": "success"})
}

// RebuildRequest is the body for the rebuild endpoint.
type RebuildRequest struct {
	KbID string `json:"kb_id"`
}

// Rebuild schedules a full re-sync for an accessible connector / knowledge base.
// @Summary Rebuild Connector
// @Description Trigger a full re-sync (equivalent to Python's rebuild)
// @Tags connector
// @Accept json
// @Produce json
// @Param connector_id path string true "connector ID"
// @Param request body handler.RebuildRequest true "rebuild parameters"
// @Router /api/v1/connectors/{connector_id}/rebuild [post]
func (h *ConnectorHandler) Rebuild(c *gin.Context) {
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

	var req RebuildRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeBadRequest, "data": nil, "message": "Invalid request body: " + err.Error()})
		return
	}
	if req.KbID == "" {
		c.JSON(http.StatusOK, gin.H{"code": common.CodeArgumentError, "data": nil, "message": "required argument is missing: kb_id"})
		return
	}

	err := h.connectorService.Rebuild(connectorID, req.KbID, user.ID)
	if connectorErrorResponse(c, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": true, "message": "success"})
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
