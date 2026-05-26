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
	"ragflow/internal/common"
	"ragflow/internal/entity"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

type connectorService interface {
	ListConnectors(userID string) (*service.ListConnectorsResponse, error)
	UpdateConnector(connectorID, userID string, req *service.UpdateConnectorRequest) (*entity.Connector, common.ErrorCode, error)
}

// ConnectorHandler connector handler
type ConnectorHandler struct {
	connectorService connectorService
	userService      *service.UserService
}

// NewConnectorHandler create connector handler
func NewConnectorHandler(connectorService *service.ConnectorService, userService *service.UserService) *ConnectorHandler {
	return &ConnectorHandler{
		connectorService: connectorService,
		userService:      userService,
	}
}

// UpdateConnector updates a connector.
// @Summary Update Connector
// @Description Update an accessible connector's polling configuration
// @Tags connector
// @Accept json
// @Produce json
// @Success 200 {object} entity.Connector
// @Router /api/v1/connectors/{connector_id} [patch]
func (h *ConnectorHandler) UpdateConnector(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var payload map[string]json.RawMessage
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&payload); err != nil {
			jsonError(c, common.CodeDataError, err.Error())
			return
		}
	}

	rawPayload, err := unwrapConnectorPayload(payload)
	if err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	var req service.UpdateConnectorRequest
	if rawPayload != nil {
		if err := json.Unmarshal(rawPayload, &req); err != nil {
			jsonError(c, common.CodeDataError, err.Error())
			return
		}
	}

	result, code, err := h.connectorService.UpdateConnector(c.Param("connector_id"), user.ID, &req)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

func unwrapConnectorPayload(payload map[string]json.RawMessage) ([]byte, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	if data, ok := payload["data"]; ok {
		var dataObj map[string]json.RawMessage
		if err := json.Unmarshal(data, &dataObj); err == nil && dataObj != nil {
			return data, nil
		}
		return nil, errors.New("field 'data' must be a JSON object")
	}
	return json.Marshal(payload)
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
