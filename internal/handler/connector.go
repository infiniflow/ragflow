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
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"ragflow/internal/common"
	"ragflow/internal/cache"
	"ragflow/internal/entity"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gin-gonic/gin"

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
	webFlowTTLSeconds                = 15 * 60
	googleOAuthAuthorizationURL      = "https://accounts.google.com/o/oauth2/v2/auth"
	defaultGoogleDriveWebRedirectURI = "http://localhost:9380/v1/connector/google-drive/oauth/web/callback"
	defaultGmailWebRedirectURI       = "http://localhost:9380/v1/connector/gmail/oauth/web/callback"
)

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

func getGoogleScopes(source string) []string {
	if source == "gmail" {
		return []string{
			"https://www.googleapis.com/auth/gmail.readonly",
			"https://www.googleapis.com/auth/admin.directory.user.readonly",
			"https://www.googleapis.com/auth/admin.directory.group.readonly",
		}
	}
	return []string{
		"https://www.googleapis.com/auth/drive.readonly",
		"https://www.googleapis.com/auth/drive.metadata.readonly",
		"https://www.googleapis.com/auth/admin.directory.group.readonly",
		"https://www.googleapis.com/auth/admin.directory.user.readonly",
	}
}

func getDefaultGoogleRedirectURI(source string) string {
	if source == "gmail" {
		if v := strings.TrimSpace(os.Getenv("GMAIL_WEB_OAUTH_REDIRECT_URI")); v != "" {
			return v
		}
		return defaultGmailWebRedirectURI
	}
	if v := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_WEB_OAUTH_REDIRECT_URI")); v != "" {
		return v
	}
	return defaultGoogleDriveWebRedirectURI
}

func buildOAuthCodeVerifier() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func buildOAuthCodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func googleWebStateCacheKey(flowID, source string) string {
	return source + "_web_flow_state:" + flowID
}

// StartGoogleWebOAuth handles POST /api/v1/connectors/google/oauth/web/start.
func (h *ConnectorHandler) StartGoogleWebOAuth(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	source := c.DefaultQuery("type", "google-drive")
	if source != "google-drive" && source != "gmail" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"message": "Invalid Google OAuth type.",
			"data":    nil,
		})
		return
	}

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"message": "Invalid request body: " + err.Error(),
			"data":    nil,
		})
		return
	}

	rawCredentials, ok := req["credentials"]
	if !ok {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"message": "required argument is missing: credentials",
			"data":    nil,
		})
		return
	}

	redirectURI := getDefaultGoogleRedirectURI(source)
	if rawRedirectURI, ok := req["redirect_uri"].(string); ok && strings.TrimSpace(rawRedirectURI) != "" {
		redirectURI = strings.TrimSpace(rawRedirectURI)
	}
	if redirectURI == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": "Google OAuth redirect URI is not configured on the server.",
			"data":    nil,
		})
		return
	}

	var credentials map[string]interface{}
	switch v := rawCredentials.(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &credentials); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeArgumentError,
				"message": "Invalid Google credentials JSON.",
				"data":    nil,
			})
			return
		}
	case map[string]interface{}:
		credentials = v
	default:
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"message": "Invalid Google credentials JSON.",
			"data":    nil,
		})
		return
	}

	if _, hasRefreshToken := credentials["refresh_token"]; hasRefreshToken {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"message": "Uploaded credentials already include a refresh token.",
			"data":    nil,
		})
		return
	}

	webSection, ok := credentials["web"].(map[string]interface{})
	if !ok {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"message": "Google OAuth JSON must include a 'web' client configuration to use browser-based authorization.",
			"data":    nil,
		})
		return
	}
	clientID, _ := webSection["client_id"].(string)
	if strings.TrimSpace(clientID) == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"message": "Google OAuth JSON must include web.client_id.",
			"data":    nil,
		})
		return
	}

	flowID := uuid.New().String()
	codeVerifier, err := buildOAuthCodeVerifier()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": "Failed to initialize Google OAuth flow. Please verify the uploaded client configuration.",
			"data":    nil,
		})
		return
	}
	codeChallenge := buildOAuthCodeChallenge(codeVerifier)

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", strings.Join(getGoogleScopes(source), " "))
	params.Set("access_type", "offline")
	params.Set("include_granted_scopes", "true")
	params.Set("prompt", "consent")
	params.Set("state", flowID)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	authorizationURL := googleOAuthAuthorizationURL + "?" + params.Encode()

	redisClient := cache.Get()
	if redisClient == nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": "Failed to initialize Google OAuth flow. Please retry.",
			"data":    nil,
		})
		return
	}

	cachePayload := map[string]interface{}{
		"user_id":       user.ID,
		"client_config": map[string]interface{}{"web": webSection},
		"redirect_uri":  redirectURI,
		"code_verifier": codeVerifier,
		"created_at":    time.Now().Unix(),
	}
	if ok := redisClient.SetObj(googleWebStateCacheKey(flowID, source), cachePayload, webFlowTTLSeconds*time.Second); !ok {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": "Failed to initialize Google OAuth flow. Please retry.",
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
		"data": gin.H{
			"flow_id":           flowID,
			"authorization_url": authorizationURL,
			"expires_in":        webFlowTTLSeconds,
		},
	})
}
