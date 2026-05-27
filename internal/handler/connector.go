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
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"ragflow/internal/common"
	"ragflow/internal/cache"
	"ragflow/internal/entity"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"ragflow/internal/service"
)

type connectorService interface {
	ListConnectors(userID string) (*service.ListConnectorsResponse, error)
	CreateConnector(userID string, req *service.CreateConnectorRequest) (*entity.Connector, error)
	GetConnector(connectorID string, userID string) (*entity.Connector, common.ErrorCode, error)
}

// ConnectorHandler connector handler
type ConnectorHandler struct {
	connectorService connectorService
	userService      *service.UserService
}

const (
	gmailWebFlowTTLSeconds         = 15 * 60
	gmailWebOAuthTokenURL          = "https://oauth2.googleapis.com/token"
	defaultGmailWebOAuthRedirectURI = "http://localhost:9380/api/v1/connectors/gmail/oauth/web/callback"
	gmailWebOAuthHTTPTimeout       = 15 * time.Second
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

func gmailWebStateCacheKey(flowID string) string {
	return "gmail_web_flow_state:" + flowID
}

func gmailWebResultCacheKey(flowID string) string {
	return "gmail_web_flow_result:" + flowID
}

func renderGoogleWebOAuthPopup(c *gin.Context, flowID string, success bool, message string, source string) {
	status := "error"
	autoClose := ""
	if success {
		status = "success"
		autoClose = "window.close();"
	}
	payloadType := fmt.Sprintf("ragflow-%s-oauth", source)
	payloadJSON := fmt.Sprintf(
		`{"type":"%s","status":"%s","flowId":"%s","message":%q}`,
		payloadType,
		status,
		flowID,
		message,
	)
	page := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8" /><title>Google %s Authorization</title></head>
<body style="font-family: Arial, sans-serif; background: #f8fafc; color: #0f172a; display:flex; align-items:center; justify-content:center; min-height:100vh; margin:0;">
  <div style="background:white; padding:32px; border-radius:12px; box-shadow:0 8px 30px rgba(15,23,42,0.1); max-width:420px; text-align:center;">
    <h1>%s</h1>
    <p>%s</p>
    <p>You can close this window.</p>
  </div>
  <script>
    (function(){
      if (window.opener) { window.opener.postMessage(%s, "*"); }
      %s
    })();
  </script>
</body>
</html>`,
		cases.Title(language.Und, cases.NoLower).String(source),
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

	var stateObj map[string]interface{}
	if ok := redisClient.GetObj(gmailWebStateCacheKey(stateID), &stateObj); !ok {
		renderGoogleWebOAuthPopup(c, stateID, false, "Authorization session expired. Please restart from the main window.", "gmail")
		return
	}

	clientConfigAny, hasClientConfig := stateObj["client_config"]
	clientConfig, okClientCfg := clientConfigAny.(map[string]interface{})
	if !hasClientConfig || !okClientCfg {
		redisClient.Delete(gmailWebStateCacheKey(stateID))
		renderGoogleWebOAuthPopup(c, stateID, false, "Authorization session was invalid. Please retry.", "gmail")
		return
	}

	webConfigAny, hasWeb := clientConfig["web"]
	webConfig, okWebCfg := webConfigAny.(map[string]interface{})
	if !hasWeb || !okWebCfg {
		redisClient.Delete(gmailWebStateCacheKey(stateID))
		renderGoogleWebOAuthPopup(c, stateID, false, "Authorization session was invalid. Please retry.", "gmail")
		return
	}

	if errVal != "" {
		redisClient.Delete(gmailWebStateCacheKey(stateID))
		msg := errorDescription
		if msg == "" {
			msg = "Authorization was cancelled."
		}
		renderGoogleWebOAuthPopup(c, stateID, false, msg, "gmail")
		return
	}

	code := c.Query("code")
	if code == "" {
		renderGoogleWebOAuthPopup(c, stateID, false, "Missing authorization code from Google.", "gmail")
		return
	}

	redirectURI := defaultGmailWebOAuthRedirectURI
	if rawRedirectURI, ok := stateObj["redirect_uri"].(string); ok && strings.TrimSpace(rawRedirectURI) != "" {
		redirectURI = strings.TrimSpace(rawRedirectURI)
	}
	clientID, _ := webConfig["client_id"].(string)
	clientSecret, _ := webConfig["client_secret"].(string)
	codeVerifier, _ := stateObj["code_verifier"].(string)

	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")
	if codeVerifier != "" {
		form.Set("code_verifier", codeVerifier)
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, gmailWebOAuthTokenURL, bytes.NewBufferString(form.Encode()))
	if err != nil {
		redisClient.Delete(gmailWebStateCacheKey(stateID))
		renderGoogleWebOAuthPopup(c, stateID, false, "Failed to exchange tokens with Google. Please retry.", "gmail")
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := gmailWebOAuthHTTPClient.Do(req)
	if err != nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		redisClient.Delete(gmailWebStateCacheKey(stateID))
		renderGoogleWebOAuthPopup(c, stateID, false, "Failed to exchange tokens with Google. Please retry.", "gmail")
		return
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		redisClient.Delete(gmailWebStateCacheKey(stateID))
		renderGoogleWebOAuthPopup(c, stateID, false, "Failed to exchange tokens with Google. Please retry.", "gmail")
		return
	}

	var tokenMap map[string]interface{}
	if err := json.Unmarshal(body, &tokenMap); err != nil {
		redisClient.Delete(gmailWebStateCacheKey(stateID))
		renderGoogleWebOAuthPopup(c, stateID, false, "Failed to exchange tokens with Google. Please retry.", "gmail")
		return
	}
	tokenMap["client_id"] = clientID
	tokenMap["client_secret"] = clientSecret
	tokenBytes, _ := json.Marshal(tokenMap)

	resultPayload := map[string]interface{}{
		"user_id":      stateObj["user_id"],
		"credentials": string(tokenBytes),
	}
	redisClient.SetObj(gmailWebResultCacheKey(stateID), resultPayload, time.Duration(gmailWebFlowTTLSeconds)*time.Second)
	redisClient.Delete(gmailWebStateCacheKey(stateID))

	renderGoogleWebOAuthPopup(c, stateID, true, "Authorization completed successfully.", "gmail")
}
