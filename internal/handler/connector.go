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
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/entity"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

type connectorService interface {
	GetConnector(connectorID, userID string) (*entity.Connector, common.ErrorCode, error)
	ListConnectors(userID string) (*service.ListConnectorsResponse, error)
	UpdateConnector(connectorID, userID string, req *service.UpdateConnectorRequest) (*entity.Connector, common.ErrorCode, error)
}

// ConnectorResponse describes the standard JSON envelope for connector details.
type ConnectorResponse struct {
	Code    common.ErrorCode  `json:"code"`
	Data    *entity.Connector `json:"data"`
	Message string            `json:"message"`
}

// ConnectorHandler connector handler
type ConnectorHandler struct {
	connectorService connectorService
	userService      *service.UserService
}

// NewConnectorHandler create connector handler
func NewConnectorHandler(connectorService connectorService, userService *service.UserService) *ConnectorHandler {
	return &ConnectorHandler{
		connectorService: connectorService,
		userService:      userService,
	}
}

// GetConnector gets connector details.
// @Summary Get Connector
// @Description Get connector details when the current user can access it
// @Tags connector
// @Accept json
// @Produce json
// @Success 200 {object} ConnectorResponse
// @Router /api/v1/connectors/{connector_id} [get]
func (h *ConnectorHandler) GetConnector(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	result, code, err := h.connectorService.GetConnector(c.Param("connector_id"), user.ID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
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

// UpdateConnector updates connector settings.
// @Summary Update Connector
// @Description Update connector details when the current user can access it
// @Tags connector
// @Accept json
// @Produce json
// @Success 200 {object} ConnectorResponse
// @Router /api/v1/connectors/{connector_id} [patch]
func (h *ConnectorHandler) UpdateConnector(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}
	body, err = unwrapConnectorPayload(body)
	if err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	var req service.UpdateConnectorRequest
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
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

func unwrapConnectorPayload(body []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return body, nil
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, err
	}
	if data, ok := payload["data"]; ok {
		var dataObj map[string]json.RawMessage
		if err := json.Unmarshal(data, &dataObj); err == nil && dataObj != nil {
			return data, nil
		}
		return nil, errors.New("field 'data' must be a JSON object")
	}
	return trimmed, nil
}
