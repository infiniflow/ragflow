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

package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"ragflow/internal/engine/redis"
	"strings"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
)

const (
	connectorInputTypePoll   = "poll"
	connectorStatusUnstarted = "0"
	defaultConnectorFreq     = 5
	defaultConnectorTimeout  = 60 * 29
	webFlowTTL               = 15 * time.Minute
	googleOAuthAuthorizeURL  = "https://accounts.google.com/o/oauth2/auth"
	googleOAuthTokenURL      = "https://oauth2.googleapis.com/token"
	googleOAuthHTTPTimeout   = 7 * time.Second
)

var (
	googleDriveOAuthScopes = []string{
		"https://www.googleapis.com/auth/drive.readonly",
		"https://www.googleapis.com/auth/drive.metadata.readonly",
		"https://www.googleapis.com/auth/admin.directory.group.readonly",
		"https://www.googleapis.com/auth/admin.directory.user.readonly",
	}
	gmailOAuthScopes = []string{
		"https://www.googleapis.com/auth/gmail.readonly",
		"https://www.googleapis.com/auth/admin.directory.user.readonly",
		"https://www.googleapis.com/auth/admin.directory.group.readonly",
	}
)

// Sentinel errors so handlers can map to the proper response codes,
// mirroring the Python connector_api responses.
var (
	// ErrConnectorNotFound mirrors Python's "Can't find this Connector!".
	ErrConnectorNotFound = errors.New("can't find this Connector")
	// ErrConnectorNoAuth mirrors Python's "No authorization." denial.
	ErrConnectorNoAuth = errors.New("no authorization")
	// ErrConnectorTestUnsupported is returned for connector sources whose
	// validation path is not yet ported to Go.
	ErrConnectorTestUnsupported = errors.New("test endpoint currently supports only REST API connectors")
)

// ConnectorService connector service
type ConnectorService struct {
	connectorDAO  *dao.ConnectorDAO
	userTenantDAO *dao.UserTenantDAO
}

// NewConnectorService create connector service
func NewConnectorService() *ConnectorService {
	return &ConnectorService{
		connectorDAO:  dao.NewConnectorDAO(),
		userTenantDAO: dao.NewUserTenantDAO(),
	}
}

// ListConnectorsResponse list connectors response
type ListConnectorsResponse struct {
	Connectors []*dao.ConnectorListItem `json:"connectors"`
}

// CreateConnectorRequest creates a connector with Python-compatible defaults.
type CreateConnectorRequest struct {
	Name        string         `json:"name"`
	Source      string         `json:"source"`
	Config      entity.JSONMap `json:"config"`
	RefreshFreq *int64         `json:"refresh_freq,omitempty"`
	PruneFreq   *int64         `json:"prune_freq,omitempty"`
	TimeoutSecs *int64         `json:"timeout_secs,omitempty"`
}

// RebuildConnectorRequest rebuild connector request.
type RebuildConnectorRequest struct {
	KbID string `json:"kb_id"`
}

type StartGoogleWebOAuthRequest struct {
	Credentials json.RawMessage `json:"credentials"`
	RedirectURI string          `json:"redirect_uri,omitempty"`
}

type StartGoogleWebOAuthResponse struct {
	FlowID           string `json:"flow_id"`
	AuthorizationURL string `json:"authorization_url"`
	ExpiresIn        int64  `json:"expires_in"`
}

type PollGoogleWebOAuthResultRequest struct {
	FlowID string `json:"flow_id"`
}

type PollGoogleWebOAuthResultResponse struct {
	Credentials string `json:"credentials"`
}

type googleWebOAuthState struct {
	UserID       string                 `json:"user_id"`
	ClientConfig map[string]interface{} `json:"client_config"`
	RedirectURI  string                 `json:"redirect_uri"`
	CodeVerifier string                 `json:"code_verifier"`
	CreatedAt    int64                  `json:"created_at"`
}

type googleWebOAuthResult struct {
	UserID      string `json:"user_id"`
	Credentials string `json:"credentials"`
}

type googleOAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
}

type googleOAuthCredentials struct {
	Token        string   `json:"token"`
	RefreshToken string   `json:"refresh_token,omitempty"`
	TokenURI     string   `json:"token_uri"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	Scopes       []string `json:"scopes"`
	Expiry       string   `json:"expiry,omitempty"`
}

// canAccessConnector Test Authentication
func (s *ConnectorService) canAccessConnector(connector *entity.Connector, userID string) bool {
	if connector.TenantID == userID {
		return true
	}

	_, err := s.userTenantDAO.FilterByUserIDAndTenantID(userID, connector.TenantID)
	return err == nil
}

// cancelConnectorTasks Stop connector tasks
func (s *ConnectorService) cancelConnectorTasks(connectorID string) error {
	if err := s.connectorDAO.CancelRunningOrScheduledLogs(connectorID); err != nil {
		return err
	}
	return s.connectorDAO.UpdateByID(connectorID, map[string]interface{}{"status": string(entity.TaskStatusCancel)})
}

// CreateConnector creates a connector owned by the current user.
// Equivalent to Python's create_connector endpoint.
func (s *ConnectorService) CreateConnector(userID string, req *CreateConnectorRequest) (*entity.Connector, error) {
	refreshFreq := int64(defaultConnectorFreq)
	if req.RefreshFreq != nil {
		refreshFreq = *req.RefreshFreq
	}

	pruneFreq := int64(defaultConnectorFreq)
	if req.PruneFreq != nil {
		pruneFreq = *req.PruneFreq
	}

	timeoutSecs := int64(defaultConnectorTimeout)
	if req.TimeoutSecs != nil {
		timeoutSecs = *req.TimeoutSecs
	}

	connector := &entity.Connector{
		ID:          common.GenerateUUID(),
		TenantID:    userID,
		Name:        req.Name,
		Source:      req.Source,
		InputType:   connectorInputTypePoll,
		Config:      req.Config,
		RefreshFreq: refreshFreq,
		PruneFreq:   pruneFreq,
		TimeoutSecs: timeoutSecs,
		Status:      connectorStatusUnstarted,
	}

	if err := s.connectorDAO.Create(connector); err != nil {
		return nil, err
	}

	return s.connectorDAO.GetByID(connector.ID)
}

// GetConnector returns one connector when the user can access its tenant.
func (s *ConnectorService) GetConnector(connectorID, userID string) (*entity.Connector, common.ErrorCode, error) {
	if connectorID == "" {
		return nil, common.CodeDataError, fmt.Errorf("connector_id is required")
	}

	connector, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.CodeDataError, fmt.Errorf("Can't find this Connector!")
		}
		return nil, common.CodeServerError, err
	}

	if connector.TenantID == userID {
		return connector, common.CodeSuccess, nil
	}

	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	for _, tenantID := range tenantIDs {
		if tenantID == connector.TenantID {
			return connector, common.CodeSuccess, nil
		}
	}

	return nil, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
}

// ListConnectors list connectors for a user
// Equivalent to Python's ConnectorService.list(current_user.id)
func (s *ConnectorService) ListConnectors(userID string) (*ListConnectorsResponse, error) {
	// Get tenant IDs by user ID
	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return nil, err
	}

	// For now, use the first tenant ID (primary tenant)
	// This matches the Python implementation behavior
	var tenantID string
	if len(tenantIDs) > 0 {
		tenantID = tenantIDs[0]
	} else {
		tenantID = userID
	}

	// Query connectors by tenant ID
	connectors, err := s.connectorDAO.ListByTenantID(tenantID)
	if err != nil {
		return nil, err
	}

	return &ListConnectorsResponse{
		Connectors: connectors,
	}, nil
}

// accessible reports whether the user can access the connector's tenant.
// Mirrors Python's ConnectorService.accessible: owner access plus joined tenants.
func (s *ConnectorService) accessible(connectorID, userID string) (bool, error) {
	conn, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		return false, ErrConnectorNotFound
	}

	if conn.TenantID == userID {
		return true, nil
	}

	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return false, err
	}
	for _, tid := range tenantIDs {
		if tid == conn.TenantID {
			return true, nil
		}
	}
	return false, nil
}

// TestConnector validates a connector's stored configuration.
// Equivalent to Python's test_connector. Per-connector credential validation
// lives in the Python common.data_source package and is not yet available in
// Go; for now this verifies access, that the connector exists, that the source
// is REST_API (the only source Python currently tests), and that credentials
// are present in the stored config. It returns ErrConnectorTestUnsupported for
// other sources.
func (s *ConnectorService) TestConnector(connectorID, userID string) error {
	ok, err := s.accessible(connectorID, userID)
	if err != nil && errors.Is(err, ErrConnectorNotFound) {
		return ErrConnectorNotFound
	}
	if err != nil {
		return err
	}
	if !ok {
		return ErrConnectorNoAuth
	}

	conn, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		return ErrConnectorNotFound
	}

	if conn.Source != "rest_api" {
		return ErrConnectorTestUnsupported
	}

	config := conn.Config
	if config == nil {
		return fmt.Errorf("connector configuration is missing")
	}
	creds, ok := config["credentials"].(map[string]interface{})
	if !ok || len(creds) == 0 {
		return fmt.Errorf("connector credentials are missing")
	}
	return nil
}

func (s *ConnectorService) StartGoogleWebOAuth(userID, source string, req *StartGoogleWebOAuthRequest) (*StartGoogleWebOAuthResponse, common.ErrorCode, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		source = "google-drive"
	}
	if source != "google-drive" && source != "gmail" {
		return nil, common.CodeArgumentError, fmt.Errorf("Invalid Google OAuth type.")
	}

	if req == nil || len(req.Credentials) == 0 {
		return nil, common.CodeArgumentError, fmt.Errorf("required argument is missing: credentials")
	}

	redirectURI := strings.TrimSpace(req.RedirectURI)
	if redirectURI == "" {
		redirectURI = defaultGoogleWebOAuthRedirectURI(source)
	}
	if redirectURI == "" {
		return nil, common.CodeServerError, fmt.Errorf("Google OAuth redirect URI is not configured on the server.")
	}

	credentials, err := loadGoogleCredentials(req.Credentials)
	if err != nil {
		return nil, common.CodeArgumentError, err
	}
	if hasRefreshToken(credentials) {
		return nil, common.CodeArgumentError, fmt.Errorf("Uploaded credentials already include a refresh token.")
	}

	clientConfig, err := getGoogleWebClientConfig(credentials)
	if err != nil {
		return nil, common.CodeArgumentError, err
	}

	webConfig, _ := clientConfig["web"].(map[string]interface{})
	clientID := strings.TrimSpace(stringValue(webConfig["client_id"]))
	authURI := strings.TrimSpace(stringValue(webConfig["auth_uri"]))
	if authURI == "" {
		authURI = googleOAuthAuthorizeURL
	}
	if clientID == "" || authURI == "" {
		return nil, common.CodeServerError, fmt.Errorf("Failed to initialize Google OAuth flow. Please verify the uploaded client configuration.")
	}

	codeVerifier, codeChallenge, err := newPKCEChallenge()
	if err != nil {
		return nil, common.CodeServerError, err
	}

	flowID := common.GenerateUUID()
	authorizationURL, err := buildGoogleAuthorizationURL(authURI, clientID, redirectURI, flowID, googleOAuthScopesForSource(source), codeChallenge)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("Failed to initialize Google OAuth flow. Please verify the uploaded client configuration.")
	}

	redisClient := redis.Get()
	if redisClient == nil {
		return nil, common.CodeServerError, fmt.Errorf("Redis is not configured on the server.")
	}

	state := googleWebOAuthState{
		UserID:       userID,
		ClientConfig: clientConfig,
		RedirectURI:  redirectURI,
		CodeVerifier: codeVerifier,
		CreatedAt:    time.Now().Unix(),
	}
	if ok := redisClient.SetObj(webStateCacheKey(flowID, source), state, webFlowTTL); !ok {
		return nil, common.CodeServerError, fmt.Errorf("Failed to initialize Google OAuth flow. Please verify the uploaded client configuration.")
	}

	return &StartGoogleWebOAuthResponse{
		FlowID:           flowID,
		AuthorizationURL: authorizationURL,
		ExpiresIn:        int64(webFlowTTL.Seconds()),
	}, common.CodeSuccess, nil
}

func (s *ConnectorService) GoogleWebOAuthCallback(source, stateID, oauthError, errorDescription, code string) string {
	source = strings.TrimSpace(source)
	if source != "google-drive" && source != "gmail" {
		return renderGoogleWebOAuthPopup("", false, "Invalid Google OAuth type.", source)
	}

	stateID = strings.TrimSpace(stateID)
	if stateID == "" {
		return renderGoogleWebOAuthPopup("", false, "Missing OAuth state parameter.", source)
	}

	redisClient := redis.Get()
	if redisClient == nil {
		return renderGoogleWebOAuthPopup(stateID, false, "Authorization session expired. Please restart from the main window.", source)
	}

	stateKey := webStateCacheKey(stateID, source)
	var state googleWebOAuthState
	if ok := redisClient.GetObj(stateKey, &state); !ok {
		return renderGoogleWebOAuthPopup(stateID, false, "Authorization session expired. Please restart from the main window.", source)
	}

	if state.ClientConfig == nil {
		redisClient.Delete(stateKey)
		return renderGoogleWebOAuthPopup(stateID, false, "Authorization session was invalid. Please retry.", source)
	}

	if strings.TrimSpace(oauthError) != "" {
		redisClient.Delete(stateKey)
		message := strings.TrimSpace(errorDescription)
		if message == "" {
			message = strings.TrimSpace(oauthError)
		}
		if message == "" {
			message = "Authorization was cancelled."
		}
		return renderGoogleWebOAuthPopup(stateID, false, message, source)
	}

	code = strings.TrimSpace(code)
	if code == "" {
		return renderGoogleWebOAuthPopup(stateID, false, "Missing authorization code from Google.", source)
	}

	credentials, err := exchangeGoogleWebOAuthCode(state.ClientConfig, googleOAuthScopesForSource(source), state.RedirectURI, code, state.CodeVerifier)
	if err != nil {
		redisClient.Delete(stateKey)
		return renderGoogleWebOAuthPopup(stateID, false, "Failed to exchange tokens with Google. Please retry.", source)
	}

	result := googleWebOAuthResult{
		UserID:      state.UserID,
		Credentials: credentials,
	}
	if ok := redisClient.SetObj(webResultCacheKey(stateID, source), result, webFlowTTL); !ok {
		redisClient.Delete(stateKey)
		return renderGoogleWebOAuthPopup(stateID, false, "Failed to exchange tokens with Google. Please retry.", source)
	}
	redisClient.Delete(stateKey)

	return renderGoogleWebOAuthPopup(stateID, true, "Authorization completed successfully.", source)
}

func (s *ConnectorService) PollGoogleWebOAuthResult(userID, source string, req *PollGoogleWebOAuthResultRequest) (*PollGoogleWebOAuthResultResponse, common.ErrorCode, error) {
	source = strings.TrimSpace(source)
	if source != "google-drive" && source != "gmail" {
		return nil, common.CodeArgumentError, fmt.Errorf("Invalid Google OAuth type.")
	}
	if req == nil || strings.TrimSpace(req.FlowID) == "" {
		return nil, common.CodeArgumentError, fmt.Errorf("required argument is missing: flow_id")
	}

	redisClient := redis.Get()
	if redisClient == nil {
		return nil, common.CodeRunning, fmt.Errorf("Authorization is still pending.")
	}

	resultKey := webResultCacheKey(strings.TrimSpace(req.FlowID), source)
	var result googleWebOAuthResult
	if ok := redisClient.GetObj(resultKey, &result); !ok {
		return nil, common.CodeRunning, fmt.Errorf("Authorization is still pending.")
	}

	if result.UserID != userID {
		return nil, common.CodePermissionError, fmt.Errorf("You are not allowed to access this authorization result.")
	}

	redisClient.Delete(resultKey)
	return &PollGoogleWebOAuthResultResponse{Credentials: result.Credentials}, common.CodeSuccess, nil
}

func defaultGoogleWebOAuthRedirectURI(source string) string {
	if source == "gmail" {
		return getenvDefault("GMAIL_WEB_OAUTH_REDIRECT_URI", "http://localhost:9384/api/v1/connectors/gmail/oauth/web/callback")
	}
	return getenvDefault("GOOGLE_DRIVE_WEB_OAUTH_REDIRECT_URI", "http://localhost:9384/api/v1/connectors/google-drive/oauth/web/callback")
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func webStateCacheKey(flowID, source string) string {
	return fmt.Sprintf("%s_web_flow_state:%s", source, flowID)
}

func webResultCacheKey(flowID, source string) string {
	return fmt.Sprintf("%s_web_flow_result:%s", source, flowID)
}

func loadGoogleCredentials(raw json.RawMessage) (map[string]interface{}, error) {
	var credentials map[string]interface{}
	if err := json.Unmarshal(raw, &credentials); err == nil && credentials != nil {
		return credentials, nil
	}

	var rawString string
	if err := json.Unmarshal(raw, &rawString); err != nil {
		return nil, fmt.Errorf("Invalid Google credentials JSON.")
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(rawString)), &credentials); err != nil || credentials == nil {
		return nil, fmt.Errorf("Invalid Google credentials JSON.")
	}
	return credentials, nil
}

func hasRefreshToken(credentials map[string]interface{}) bool {
	value, ok := credentials["refresh_token"]
	if !ok || value == nil {
		return false
	}
	if token, ok := value.(string); ok {
		return strings.TrimSpace(token) != ""
	}
	return true
}

func getGoogleWebClientConfig(credentials map[string]interface{}) (map[string]interface{}, error) {
	webSection, ok := credentials["web"].(map[string]interface{})
	if !ok || webSection == nil {
		return nil, fmt.Errorf("Google OAuth JSON must include a 'web' client configuration to use browser-based authorization.")
	}
	return map[string]interface{}{"web": webSection}, nil
}

func stringValue(value interface{}) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func googleOAuthScopesForSource(source string) []string {
	if source == "gmail" {
		return gmailOAuthScopes
	}
	return googleDriveOAuthScopes
}

func newPKCEChallenge() (string, string, error) {
	randomBytes := make([]byte, 64)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate OAuth code verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(randomBytes)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func buildGoogleAuthorizationURL(authURI, clientID, redirectURI, state string, scopes []string, codeChallenge string) (string, error) {
	parsedURL, err := url.Parse(authURI)
	if err != nil {
		return "", err
	}

	query := parsedURL.Query()
	query.Set("client_id", clientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(scopes, " "))
	query.Set("access_type", "offline")
	query.Set("include_granted_scopes", "true")
	query.Set("prompt", "consent")
	query.Set("state", state)
	query.Set("code_challenge", codeChallenge)
	query.Set("code_challenge_method", "S256")
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

func exchangeGoogleWebOAuthCode(clientConfig map[string]interface{}, scopes []string, redirectURI, code, codeVerifier string) (string, error) {
	webConfig, ok := clientConfig["web"].(map[string]interface{})
	if !ok || webConfig == nil {
		return "", fmt.Errorf("invalid Google OAuth client configuration")
	}

	clientID := strings.TrimSpace(stringValue(webConfig["client_id"]))
	clientSecret := strings.TrimSpace(stringValue(webConfig["client_secret"]))
	tokenURI := strings.TrimSpace(stringValue(webConfig["token_uri"]))
	if tokenURI == "" {
		tokenURI = googleOAuthTokenURL
	}
	if clientID == "" || tokenURI == "" {
		return "", fmt.Errorf("invalid Google OAuth client configuration")
	}

	form := url.Values{}
	form.Set("client_id", clientID)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")
	if strings.TrimSpace(codeVerifier) != "" {
		form.Set("code_verifier", codeVerifier)
	}

	ctx, cancel := context.WithTimeout(context.Background(), googleOAuthHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var token googleOAuthTokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return "", err
	}
	if resp.StatusCode >= http.StatusBadRequest || token.Error != "" {
		if token.ErrorDesc != "" {
			return "", errors.New(token.ErrorDesc)
		}
		if token.Error != "" {
			return "", errors.New(token.Error)
		}
		return "", fmt.Errorf("google token exchange failed: HTTP %d", resp.StatusCode)
	}
	if token.AccessToken == "" {
		return "", fmt.Errorf("google token exchange failed: empty access_token")
	}

	expiry := ""
	if token.ExpiresIn > 0 {
		expiry = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UTC().Format(time.RFC3339Nano)
	}
	credentials := googleOAuthCredentials{
		Token:        token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenURI:     tokenURI,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       scopes,
		Expiry:       expiry,
	}
	data, err := json.Marshal(credentials)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func renderGoogleWebOAuthPopup(flowID string, success bool, message, source string) string {
	status := "error"
	autoClose := ""
	if success {
		status = "success"
		autoClose = "window.close();"
	}
	payloadType := fmt.Sprintf("ragflow-%s-oauth", source)
	payload, _ := json.Marshal(map[string]string{
		"type":    payloadType,
		"status":  status,
		"flowId":  flowID,
		"message": message,
	})

	title := fmt.Sprintf("%s Authorization", googleOAuthSourceDisplayName(source))
	heading := "Authorization failed"
	if success {
		heading = "Authorization complete"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <title>%s</title>
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
</html>`, html.EscapeString(title), html.EscapeString(heading), html.EscapeString(message), string(payload), autoClose)
}

func googleOAuthSourceDisplayName(source string) string {
	if source == "gmail" {
		return "Gmail"
	}
	if source == "google-drive" {
		return "Google Drive"
	}
	return "Google"
}

func (s *ConnectorService) DeleteConnector(connectorID, userID string) (bool, common.ErrorCode, error) {
	if connectorID == "" {
		return false, common.CodeDataError, fmt.Errorf("connector_id is required")
	}

	connector, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, common.CodeDataError, fmt.Errorf("Can't find this Connector!")
		}
		return false, common.CodeServerError, err
	}

	if !s.canAccessConnector(connector, userID) {
		return false, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
	}

	if err = s.cancelConnectorTasks(connector.ID); err != nil {
		return false, common.CodeServerError, err
	}

	if err = s.connectorDAO.DeleteByID(connector.ID); err != nil {
		return false, common.CodeServerError, err
	}
	return true, common.CodeSuccess, nil
}

type UpdateConnectorRequest struct {
	PruneFreq   *int64         `json:"prune_freq,omitempty"`
	RefreshFreq *int64         `json:"refresh_freq,omitempty"`
	Config      entity.JSONMap `json:"config,omitempty"`
	TimeoutSecs *int64         `json:"timeout_secs,omitempty"`
	Reschedule  bool           `json:"reschedule,omitempty"`
	Status      string         `json:"status,omitempty"`
}

func (s *ConnectorService) UpdateConnector(connectorID, userID string, req *UpdateConnectorRequest) (*entity.Connector, common.ErrorCode, error) {
	if connectorID == "" {
		return nil, common.CodeDataError, fmt.Errorf("connector_id is required")
	}

	connector, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.CodeDataError, fmt.Errorf("Can't find this Connector!")
		}
		return nil, common.CodeServerError, err
	}

	if !s.canAccessConnector(connector, userID) {
		return nil, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
	}

	updates := map[string]interface{}{}
	if req != nil {
		if req.PruneFreq != nil {
			updates["prune_freq"] = *req.PruneFreq
		}
		if req.RefreshFreq != nil {
			updates["refresh_freq"] = *req.RefreshFreq
		}
		if req.Config != nil {
			updates["config"] = req.Config
		}
		if req.TimeoutSecs != nil {
			updates["timeout_secs"] = *req.TimeoutSecs
		}
	}

	if len(updates) > 0 {
		if err := s.connectorDAO.UpdateByID(connectorID, updates); err != nil {
			return nil, common.CodeServerError, err
		}
	}

	if req != nil {
		if req.Reschedule {
			if err := s.cancelConnectorTasks(connectorID); err != nil {
				return nil, common.CodeServerError, err
			}
			if err := s.connectorDAO.ScheduleConnectorTasks(connectorID); err != nil {
				return nil, common.CodeServerError, err
			}
		} else if isConnectorCancelStatus(req.Status) {
			if err := s.cancelConnectorTasks(connectorID); err != nil {
				return nil, common.CodeServerError, err
			}
		} else if isConnectorScheduleStatus(req.Status) {
			if err := s.connectorDAO.ScheduleConnectorTasks(connectorID); err != nil {
				return nil, common.CodeServerError, err
			}
		}
	}

	connector, err = s.connectorDAO.GetByID(connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.CodeDataError, fmt.Errorf("Can't find this Connector!")
		}
		return nil, common.CodeServerError, err
	}

	return connector, common.CodeSuccess, nil
}

func isConnectorCancelStatus(status string) bool {
	status = strings.TrimSpace(status)
	return status == string(entity.TaskStatusCancel) || strings.EqualFold(status, "CANCEL")
}

func isConnectorScheduleStatus(status string) bool {
	status = strings.TrimSpace(status)
	return status == string(entity.TaskStatusSchedule) || strings.EqualFold(status, "SCHEDULE")
}

// RebuildConnector schedules a rebuild for an accessible connector and knowledge base.
func (s *ConnectorService) RebuildConnector(connectorID, userID, kbID string) (bool, common.ErrorCode, error) {
	if connectorID == "" {
		return false, common.CodeDataError, fmt.Errorf("connector_id is required")
	}
	if kbID == "" {
		return false, common.CodeArgumentError, fmt.Errorf("required argument is missing: kb_id")
	}

	connector, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, common.CodeDataError, fmt.Errorf("Can't find this Connector!")
		}
		return false, common.CodeServerError, err
	}

	if !s.canAccessConnector(connector, userID) {
		return false, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
	}

	sourceType := fmt.Sprintf("%s/%s", connector.Source, connector.ID)
	documents, err := s.connectorDAO.ListDocumentsByKBAndSourceType(kbID, sourceType)
	if err != nil {
		return false, common.CodeServerError, err
	}

	s.deleteConnectorDocumentChunks(connector.TenantID, kbID, documents)

	if err := s.connectorDAO.RebuildConnector(connector, kbID, documents); err != nil {
		return false, common.CodeServerError, err
	}
	return true, common.CodeSuccess, nil
}

func (s *ConnectorService) deleteConnectorDocumentChunks(tenantID, kbID string, documents []*entity.Document) {
	docEngine := engine.Get()
	if docEngine == nil {
		return
	}

	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	for _, document := range documents {
		_, _ = docEngine.DeleteChunks(context.Background(), map[string]interface{}{"doc_id": document.ID}, indexName, kbID)
	}
}

func (s *ConnectorService) ListLog(connectorID, userID string, page, pageSize int) ([]*entity.ConnectorSyncLog, int64, common.ErrorCode, error) {
	if connectorID == "" {
		return nil, 0, common.CodeDataError, fmt.Errorf("connector_id is required")
	}

	connector, err := s.connectorDAO.GetByID(connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, common.CodeDataError, fmt.Errorf("Can't find this Connector!")
		}
		return nil, 0, common.CodeServerError, err
	}

	if !s.canAccessConnector(connector, userID) {
		return nil, 0, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
	}

	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 15
	}
	offset := (page - 1) * pageSize

	logs, total, err := s.connectorDAO.ListLogsByConnectorID(connectorID, offset, pageSize)
	if err != nil {
		return nil, 0, common.CodeServerError, fmt.Errorf("failed to fetch connector logs: %w", err)
	}
	if logs == nil {
		logs = []*entity.ConnectorSyncLog{}
	}
	return logs, total, common.CodeSuccess, nil
}
