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
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	permissionresponse "ragflow/internal/permission/response"
	"ragflow/internal/service"
	syncerconnector "ragflow/internal/syncer/connector"
)

type connectorServiceIface interface {
	ListConnectors(ctx context.Context, tenantID string) (*service.ListConnectorsResponse, error)
	CreateConnector(ctx context.Context, req *service.CreateConnectorRequest) (*entity.Connector, error)
	GetConnector(ctx context.Context, connectorID string) (*entity.Connector, error)
	ListLog(ctx context.Context, connectorID string, page, pageSize int) ([]*entity.ConnectorSyncLog, int64, common.ErrorCode, error)
	ListLogs(ctx context.Context, datasetID string, page, pageSize int) ([]*entity.ConnectorSyncLog, int64, common.ErrorCode, error)
	DeleteConnector(ctx context.Context, connectorID string) (bool, common.ErrorCode, error)
	RebuildConnector(ctx context.Context, connectorID, kbID string) (bool, common.ErrorCode, error)
	ResumeFailedSync(ctx context.Context, connectorID string, req *service.ResumeFailedSyncRequest) (bool, common.ErrorCode, error)
	TestConnector(ctx context.Context, connectorID string, config entity.JSONMap) error
	UpdateConnector(ctx context.Context, connectorID string, req *service.UpdateConnectorRequest) (*entity.Connector, common.ErrorCode, error)
	StartGoogleWebOAuth(ctx context.Context, source string, req *service.StartGoogleWebOAuthRequest) (*service.StartGoogleWebOAuthResponse, common.ErrorCode, error)
	GoogleWebOAuthCallback(ctx context.Context, source, stateID, oauthError, errorDescription, code string) string
	PollGoogleWebOAuthResult(ctx context.Context, source string, req *service.PollGoogleWebOAuthResultRequest) (*service.PollGoogleWebOAuthResultResponse, common.ErrorCode, error)
	StartBoxWebOAuth(ctx context.Context, req *service.StartBoxWebOAuthRequest) (*service.StartBoxWebOAuthResponse, common.ErrorCode, error)
	BoxWebOAuthCallback(ctx context.Context, flowID string, oauthError string, errorDescription string, code string) string
	PollBoxWebOAuthResult(ctx context.Context, req *service.PollBoxWebOAuthResultRequest) (*service.PollBoxWebOAuthResultResponse, common.ErrorCode, error)
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

// ListConnectors lists connectors for an owned tenant or a requested tenant the caller belongs to.
// @Summary List Connectors
// @Description Get list of connectors for the current user (equivalent to Python's list_connector)
// @Tags connector
// @Accept json
// @Produce json
// @Param tenant_id query string false "tenant whose connectors are listed"
// @Success 200 {object} service.ListConnectorsResponse
// @Router /connector/list [get]
func (h *ConnectorHandler) ListConnectors(c *gin.Context) {
	ctx := c.Request.Context()

	// List connectors
	result, err := h.connectorService.ListConnectors(ctx, c.Query("tenant_id"))
	if err != nil {
		if connectorErrorResponse(c, err) {
			return
		}
		common.Respond(c, http.StatusInternalServerError, 500, nil, err.Error())
		return
	}

	common.RespondSuccessList(c, result.Connectors, "success")
}

// connectorErrorResponse maps service sentinel errors to the response codes used
// by the Python connector_api, and writes the JSON response. It returns true when
// the error was handled.
func connectorErrorResponse(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case permissionresponse.IsPermissionError(err):
		code, permissionErr := permissionresponse.NormalizeHidden(err)
		common.RespondError(c, code, permissionErr.Error())
	case errors.Is(err, service.ErrConnectorIDRequired):
		common.RespondError(c, common.CodeDataError, err.Error())
	case errors.Is(err, service.ErrConnectorNotFound):
		common.RespondError(c, common.CodeDataError, "Can't find this Connector!")
	case errors.Is(err, service.ErrConnectorTestUnsupported):
		common.RespondError(c, common.CodeNotImplemented, err.Error())
	case errors.Is(err, service.ErrConnectorSourceNotImplemented):
		common.RespondError(c, common.CodeNotImplemented, err.Error())
	default:
		common.Respond(c, http.StatusInternalServerError, common.CodeServerError, nil, err.Error())
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
	ctx := c.Request.Context()

	connector, err := h.connectorService.GetConnector(ctx, c.Param("connector_id"))
	if connectorErrorResponse(c, err) {
		return
	}

	common.RespondSuccess(c, connector, "success")
}

// UpdateConnector Update an accessible connector's polling configuration.
func (h *ConnectorHandler) UpdateConnector(c *gin.Context) {
	common.StartOperationLog(c, "update_connector")
	defer common.FinishOperationLog(c)
	if id := c.Param("connector_id"); id != "" {
		common.SetOperationResource(c, "connector_id", id)
	}

	req, err := decodeUpdateConnectorRequest(c)
	if err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}

	ctx := c.Request.Context()

	connector, code, err := h.connectorService.UpdateConnector(ctx, c.Param("connector_id"), req)
	if err != nil {
		common.RespondError(c, code, err.Error())
		return
	}

	common.RespondSuccess(c, connector, "success")
}

func decodeUpdateConnectorRequest(c *gin.Context) (*service.UpdateConnectorRequest, error) {
	var raw map[string]json.RawMessage
	if err := c.ShouldBindJSON(&raw); err != nil {
		return nil, err
	}

	payload := raw
	if dataRaw, ok := raw["data"]; ok {
		var nested map[string]json.RawMessage
		if err := json.Unmarshal(dataRaw, &nested); err == nil && nested != nil {
			payload = nested
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	var req service.UpdateConnectorRequest
	if err = json.Unmarshal(data, &req); err != nil {
		return nil, err
	}

	return &req, nil
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

	page, pageSize, err := common.ParsePagination(c.Request.URL.Query())
	if err != nil {
		common.RespondError(c, common.CodeArgumentError, err.Error())
		return
	}

	ctx := c.Request.Context()

	logs, total, code, err := h.connectorService.ListLog(ctx, c.Param("connector_id"), page, pageSize)
	if err != nil {
		common.RespondError(c, code, err.Error())
		return
	}
	if logs == nil {
		logs = []*entity.ConnectorSyncLog{}
	}

	common.RespondSuccess(c, gin.H{"total": total, "logs": logs}, "success")
}

// ListSyncLogs handles GET /api/v1/connectors/sync-logs.
// Lists sync logs for the current user; when dataset_id is provided, only
// the logs of that dataset are returned.
// @Summary list sync logs
// @Description list sync logs for the current user, optionally filtered by dataset_id
// @Tags connector
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/connectors/sync-logs [get]
func (h *ConnectorHandler) ListSyncLogs(c *gin.Context) {

	page, pageSize, err := common.ParsePagination(c.Request.URL.Query())
	if err != nil {
		common.RespondError(c, common.CodeArgumentError, err.Error())
		return
	}

	datasetID := strings.TrimSpace(c.Query("dataset_id"))

	ctx := c.Request.Context()

	logs, total, code, err := h.connectorService.ListLogs(ctx, datasetID, page, pageSize)
	if err != nil {
		common.RespondError(c, code, err.Error())
		return
	}
	if logs == nil {
		logs = []*entity.ConnectorSyncLog{}
	}

	common.RespondSuccess(c, gin.H{"total": total, "logs": logs}, "success")
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
	common.StartOperationLog(c, "create_connector")
	defer common.FinishOperationLog(c)

	var req service.CreateConnectorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Invalid request body: "+err.Error())
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		common.RespondError(c, common.CodeDataError, "name is required")
		return
	}
	if strings.TrimSpace(req.Source) == "" {
		common.RespondError(c, common.CodeDataError, "source is required")
		return
	}
	if req.Config == nil {
		common.RespondError(c, common.CodeDataError, "config is required")
		return
	}
	ctx := c.Request.Context()

	connector, err := h.connectorService.CreateConnector(ctx, &req)
	if err != nil {
		if errors.Is(err, service.ErrConnectorInvalidFreq) || errors.Is(err, service.ErrConnectorInvalidName) || errors.Is(err, service.ErrConnectorEncryptedCredentials) {
			common.RespondError(c, common.CodeDataError, err.Error())
			return
		}
		common.Respond(c, http.StatusInternalServerError, common.CodeServerError, nil, err.Error())
		return
	}

	common.SetOperationResource(c, "connector_id", connector.ID)
	common.RespondSuccess(c, connector, "success")
}

type testConnectorRequest struct {
	Source string         `json:"source"`
	Config entity.JSONMap `json:"config"`
}

// TestConnector validates connector settings.
// @Summary Test Connector
// @Description Validate connector credentials / connection (equivalent to Python's test_connector)
// @Tags connector
// @Produce json
// @Param connector_id path string true "connector ID"
// @Router /api/v1/connectors/{connector_id}/test [post]
func (h *ConnectorHandler) TestConnector(c *gin.Context) {
	common.StartOperationLog(c, "test_connector")
	defer common.FinishOperationLog(c)
	if id := c.Param("connector_id"); id != "" {
		common.SetOperationResource(c, "connector_id", id)
	}

	// check user and connector

	connectorID := c.Param("connector_id")
	if connectorID == "" {
		common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, "connector_id is required")
		return
	}

	ctx := c.Request.Context()

	// build request
	var request entity.JSONMap
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		var body testConnectorRequest
		if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
			common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, err.Error())
			return
		}
		// get source and config from web
		if body.Source != "" || body.Config != nil {
			request = entity.JSONMap{
				"source": body.Source,
				"config": body.Config,
			}
		}
	}

	err := h.connectorService.TestConnector(ctx, connectorID, request)
	if permissionresponse.IsPermissionError(err) {
		connectorErrorResponse(c, err)
		return
	}
	if errors.Is(err, service.ErrConnectorTestUnsupported) || errors.Is(err, service.ErrConnectorSourceNotImplemented) {
		connectorErrorResponse(c, err)
		return
	}
	if err != nil && !errors.Is(err, service.ErrConnectorNotFound) {
		// Schema/credential validation failures map to DATA_ERROR;
		// anything unexpected falls back to SERVER_ERROR.
		var (
			valErr  *syncerconnector.ConnectorValidationError
			credErr *syncerconnector.ConnectorMissingCredentialError
			rateErr *syncerconnector.RateLimitTriedTooManyTimesError
		)
		if errors.As(err, &valErr) || errors.As(err, &credErr) || errors.As(err, &rateErr) {
			common.RespondError(c, common.CodeDataError, err.Error())
			return
		}
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}
	if connectorErrorResponse(c, err) {
		return
	}

	common.RespondSuccess(c, nil, "success")
}

// DeleteConnector delete connector
// @Description Detele Connector
// @Tags connector
// @Accept json
// @Produce json
func (h *ConnectorHandler) DeleteConnector(c *gin.Context) {
	common.StartOperationLog(c, "delete_connector")
	defer common.FinishOperationLog(c)
	if id := c.Param("connector_id"); id != "" {
		common.SetOperationResource(c, "connector_id", id)
	}

	ctx := c.Request.Context()

	_, code, err := h.connectorService.DeleteConnector(ctx, c.Param("connector_id"))
	if err != nil {
		common.RespondError(c, code, err.Error())
		return
	}

	common.RespondSuccess(c, nil, "success")
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
	common.StartOperationLog(c, "request_connector_rebuild")
	defer common.FinishOperationLog(c)
	if id := c.Param("connector_id"); id != "" {
		common.SetOperationResource(c, "connector_id", id)
	}

	// Parse request body to get kb_id
	var req struct {
		KbID string `json:"kb_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeDataError, "required argument is missing: kb_id: "+err.Error())
		return
	}

	if strings.TrimSpace(req.KbID) == "" {
		common.RespondError(c, common.CodeDataError, "kb_id cannot be empty")
		return
	}

	common.SetOperationResource(c, "dataset_id", req.KbID)
	ctx := c.Request.Context()
	_, code, err := h.connectorService.RebuildConnector(ctx, c.Param("connector_id"), req.KbID)
	if err != nil {
		common.RespondError(c, code, err.Error())
		return
	}

	common.RespondSuccess(c, nil, "success")
}

// ResumeFailedSync resumes a failed connector sync task from checkpoint. (when network outage)
// @Summary Resume Failed Connector Sync
// @Description Resume a failed connector sync task from its saved checkpoint
// @Tags connector
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /connector/:connector_id/resume-failed-sync [post]
func (h *ConnectorHandler) ResumeFailedSync(c *gin.Context) {
	common.StartOperationLog(c, "request_connector_sync_resume")
	defer common.FinishOperationLog(c)
	if id := c.Param("connector_id"); id != "" {
		common.SetOperationResource(c, "connector_id", id)
	}

	var req service.ResumeFailedSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeDataError, "required argument is missing: kb_id or task_id: "+err.Error())
		return
	}

	if req.KbID != "" {
		common.SetOperationResource(c, "dataset_id", req.KbID)
	}
	if req.TaskID != "" {
		common.SetOperationResource(c, "task_id", req.TaskID)
	}
	ctx := c.Request.Context()
	_, code, err := h.connectorService.ResumeFailedSync(ctx, c.Param("connector_id"), &req)
	if err != nil {
		common.RespondError(c, code, err.Error())
		return
	}
	common.RespondSuccess(c, nil, "success")
}

func (h *ConnectorHandler) StartGoogleWebOAuth(c *gin.Context) {
	common.StartOperationLog(c, "request_connector_oauth")
	defer common.FinishOperationLog(c)

	var req service.StartGoogleWebOAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}

	ctx := c.Request.Context()

	data, code, err := h.connectorService.StartGoogleWebOAuth(ctx, c.DefaultQuery("type", "google-drive"), &req)
	if err != nil {
		common.RespondError(c, code, err.Error())
		return
	}

	common.RespondSuccess(c, data, "success")
}

func (h *ConnectorHandler) PollGoogleWebOAuthResult(c *gin.Context) {

	var req service.PollGoogleWebOAuthResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Respond(c, http.StatusBadRequest, common.CodeBadRequest, nil, err.Error())
		return
	}

	ctx := c.Request.Context()

	data, code, err := h.connectorService.PollGoogleWebOAuthResult(ctx, c.Query("type"), &req)
	if err != nil {
		common.RespondError(c, code, err.Error())
		return
	}

	common.RespondSuccess(c, data, "success")
}

func (h *ConnectorHandler) GoogleWebOAuthCallback(c *gin.Context) {
	h.googleWebOAuthCallback(c, c.Param("source"))
}

func (h *ConnectorHandler) GoogleDriveWebOAuthCallback(c *gin.Context) {
	h.googleWebOAuthCallback(c, "google-drive")
}

func (h *ConnectorHandler) GmailWebOAuthCallback(c *gin.Context) {
	h.googleWebOAuthCallback(c, "gmail")
}

func (h *ConnectorHandler) googleWebOAuthCallback(c *gin.Context, source string) {
	ctx := c.Request.Context()

	html := h.connectorService.GoogleWebOAuthCallback(ctx,
		source,
		c.Query("state"),
		c.Query("error"),
		c.Query("error_description"),
		c.Query("code"),
	)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

func (h *ConnectorHandler) StartBoxWebOAuth(c *gin.Context) {
	common.StartOperationLog(c, "request_connector_oauth")
	defer common.FinishOperationLog(c)

	var req service.StartBoxWebOAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}
	ctx := c.Request.Context()

	resp, code, err := h.connectorService.StartBoxWebOAuth(ctx, &req)
	if err != nil {
		common.RespondError(c, code, err.Error())
		return
	}
	common.RespondSuccess(c, resp, "success")
}

func (h *ConnectorHandler) BoxWebOAuthCallback(c *gin.Context) {
	flowID := c.Query("state")
	oauthError := c.Query("error")
	errorDescription := c.Query("error_description")
	code := c.Query("code")

	ctx := c.Request.Context()

	html := h.connectorService.BoxWebOAuthCallback(ctx, flowID, oauthError, errorDescription, code)

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

func (h *ConnectorHandler) PollBoxWebOAuthResult(c *gin.Context) {
	var req service.PollBoxWebOAuthResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}
	ctx := c.Request.Context()

	resp, code, err := h.connectorService.PollBoxWebOAuthResult(ctx, &req)
	if err != nil {
		common.RespondError(c, code, err.Error())
		return
	}
	common.RespondSuccess(c, resp, "success")
}
