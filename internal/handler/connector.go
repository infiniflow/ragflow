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
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"ragflow/internal/cache"
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

const (
	gmailWebFlowTTLSeconds          = 15 * 60
	gmailWebOAuthTokenURL           = "https://oauth2.googleapis.com/token"
	defaultGmailWebOAuthRedirectURI = "http://localhost:9380/api/v1/connectors/gmail/oauth/web/callback"
	gmailWebOAuthHTTPTimeout        = 15 * time.Second
)

var gmailWebOAuthHTTPClient = &http.Client{Timeout: gmailWebOAuthHTTPTimeout}

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

func gmailWebResultCacheKey(flowID string) string {
	return "gmail_web_flow_result:" + flowID
}

// capitalizeFirst uppercases the first rune and keeps the remaining runes unchanged.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && size == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

func renderGoogleWebOAuthPopup(c *gin.Context, flowID string, success bool, message string, source string) {
	status := "error"
	autoClose := ""
	if success {
		status = "success"
		autoClose = "window.close();"
	}
	payloadType := fmt.Sprintf("ragflow-%s-oauth", source)
	payloadMap := map[string]string{
		"type":    payloadType,
		"status":  status,
		"flowId":  flowID,
		"message": message,
	}
	payloadBytes, err := json.Marshal(payloadMap)
	if err != nil {
		payloadBytes = []byte(`{"type":"ragflow-gmail-oauth","status":"error","flowId":"","message":"Failed to build OAuth payload."}`)
	}
	payloadJSON := string(payloadBytes)
	page := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <title>Google %s Authorization</title>
  <style>
    body {
      font-family: Arial, sans-serif;
      background: #f8fafc;
      color: #0f172a;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
      margin: 0;
    }
    .card {
      background: white;
      padding: 32px;
      border-radius: 12px;
      box-shadow: 0 8px 30px rgba(15, 23, 42, 0.1);
      max-width: 420px;
      text-align: center;
    }
    h1 {
      font-size: 1.5rem;
      margin-bottom: 12px;
    }
    p {
      font-size: 0.95rem;
      line-height: 1.5;
    }
  </style>
</head>
<body>
  <div class="card">
    <h1>%s</h1>
    <p>%s</p>
    <p>You can close this window.</p>
  </div>
  <script>
    (function(){
      if (window.opener) {
        window.opener.postMessage(%s, "*");
      }
      %s
    })();
  </script>
</body>
</html>`,
		capitalizeFirst(source),
		map[bool]string{true: "Authorization complete", false: "Authorization failed"}[success],
		html.EscapeString(message),
		payloadJSON,
		autoClose,
	)
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, page)
}

// GoogleGmailWebOAuthCallback handles GET /api/v1/connectors/gmail/oauth/web/callback.
func (h *ConnectorHandler) GoogleGmailWebOAuthCallback(c *gin.Context) {
	stateID := c.Query("state")
	errVal := c.Query("error")
	errorDescription := c.Query("error_description")
	if errorDescription == "" {
		errorDescription = errVal
	}

	if stateID == "" {
		renderGoogleWebOAuthPopup(c, "", false, "Missing OAuth state parameter.", "gmail")
		return
	}

	redisClient := cache.Get()
	if redisClient == nil {
		renderGoogleWebOAuthPopup(c, stateID, false, "Authorization session expired. Please restart from the main window.", "gmail")
		return
	}

	cacheKey := gmailWebStateCacheKey(stateID)
	var flowState map[string]interface{}
	if ok := redisClient.GetObj(cacheKey, &flowState); !ok {
		renderGoogleWebOAuthPopup(c, stateID, false, "Authorization session expired. Please restart from the main window.", "gmail")
		return
	}

	redirectURI, _ := flowState["redirect_uri"].(string)
	codeVerifier, _ := flowState["code_verifier"].(string)
	clientConfig, _ := flowState["client_config"].(map[string]interface{})
	webSection, _ := clientConfig["web"].(map[string]interface{})
	clientID, _ := webSection["client_id"].(string)
	clientSecret, _ := webSection["client_secret"].(string)

	if clientID == "" || clientSecret == "" || redirectURI == "" || codeVerifier == "" {
		redisClient.Delete(cacheKey)
		renderGoogleWebOAuthPopup(c, stateID, false, "Authorization session was invalid. Please retry.", "gmail")
		return
	}

	if errVal != "" {
		redisClient.Delete(cacheKey)
		msg := "Authorization was canceled or denied."
		if errorDescription != "" {
			msg = errorDescription
		}
		renderGoogleWebOAuthPopup(c, stateID, false, msg, "gmail")
		return
	}

	code := c.Query("code")
	if code == "" {
		renderGoogleWebOAuthPopup(c, stateID, false, "Missing authorization code from Google.", "gmail")
		return
	}

	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")
	form.Set("code_verifier", codeVerifier)

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, gmailWebOAuthTokenURL, bytes.NewBufferString(form.Encode()))
	if err != nil {
		redisClient.Delete(cacheKey)
		renderGoogleWebOAuthPopup(c, stateID, false, "Failed to exchange tokens with Google. Please retry.", "gmail")
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := gmailWebOAuthHTTPClient.Do(req)
	if err != nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		redisClient.Delete(cacheKey)
		renderGoogleWebOAuthPopup(c, stateID, false, "Failed to exchange tokens with Google. Please retry.", "gmail")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		redisClient.Delete(cacheKey)
		renderGoogleWebOAuthPopup(c, stateID, false, "Failed to exchange tokens with Google. Please retry.", "gmail")
		return
	}

	var tokenPayload map[string]interface{}
	if err := json.Unmarshal(body, &tokenPayload); err != nil {
		redisClient.Delete(cacheKey)
		renderGoogleWebOAuthPopup(c, stateID, false, "Failed to exchange tokens with Google. Please retry.", "gmail")
		return
	}

	if ok := redisClient.SetObj(gmailWebResultCacheKey(stateID), tokenPayload, gmailWebFlowTTLSeconds*time.Second); !ok {
		redisClient.Delete(cacheKey)
		renderGoogleWebOAuthPopup(c, stateID, false, "Failed to store authorization result. Please retry.", "gmail")
		return
	}

	redisClient.Delete(cacheKey)
	renderGoogleWebOAuthPopup(c, stateID, true, "Authorization completed successfully.", "gmail")
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
