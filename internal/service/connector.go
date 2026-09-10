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
	"ragflow/internal/engine/redis"
	syncerconnector "ragflow/internal/syncer/connector"
	"ragflow/internal/utility"
	"strings"
	"time"

	"go.uber.org/zap"
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
	boxOAuthAuthorizeURL     = "https://account.box.com/api/oauth2/authorize"
	boxOAuthTokenURL         = "https://api.box.com/oauth2/token"
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
	connectorRedisGet = redis.Get
)

// Sentinel errors so handlers can map to the proper response codes.
var (
	// ErrConnectorNotFound is returned when a connector is not found.
	ErrConnectorNotFound = errors.New("can't find this Connector")
	// ErrConnectorNoAuth is returned when the caller cannot access the connector or knowledge base.
	ErrConnectorNoAuth = errors.New("no authorization")
	// ErrConnectorNotBoundToKB is returned when the connector is not bound to the kb being rebuilt.
	ErrConnectorNotBoundToKB = errors.New("connector is not bound to this knowledge base")
	// ErrConnectorIDRequired is returned when a connector ID is missing.
	ErrConnectorIDRequired = errors.New("connector_id is required")
	// ErrConnectorTestUnsupported is returned for connector sources without a settings validator.
	ErrConnectorTestUnsupported = errors.New("connector test is not supported for this source")
	// ErrConnectorSourceNotImplemented is returned for connector sources not registered in the Go syncer.
	ErrConnectorSourceNotImplemented = errors.New("connector source is not implemented")
	// ErrConnectorInternal is a generic, safe-to-expose internal failure.
	ErrConnectorInternal = errors.New("Internal server error")
)

// ConnectorService connector service
type ConnectorService struct {
	connectorDAO      *dao.ConnectorDAO
	knowledgebaseDAO  *dao.KnowledgebaseDAO
	userTenantDAO     *dao.UserTenantDAO
	connectorRegistry *syncerconnector.Registry
}

type syncTaskPublisher interface {
	PublishSyncerTask(taskID string) error
}

type syncTaskWakeupPublisher interface {
	PublishSyncerTaskWakeup(taskID string) error
}

type syncCheckpointLoader interface {
	LoadSyncCheckpoint(ctx context.Context, taskID string) (*syncerconnector.SyncCheckpointState, error)
}

type syncCheckpointDeleter interface {
	DeleteSyncCheckpoint(ctx context.Context, taskID string) error
}

var getSyncerTaskPublisher = func() (syncTaskPublisher, bool) {
	publisher, ok := engine.GetMessageQueueEngine().(syncTaskPublisher)
	return publisher, ok
}

var getSyncCheckpointLoader = func() (syncCheckpointLoader, bool) {
	loader, ok := engine.GetMessageQueueEngine().(syncCheckpointLoader)
	return loader, ok
}

var getSyncCheckpointDeleter = func() (syncCheckpointDeleter, bool) {
	deleter, ok := engine.GetMessageQueueEngine().(syncCheckpointDeleter)
	return deleter, ok
}

// NewConnectorService create connector service
func NewConnectorService() *ConnectorService {
	return &ConnectorService{
		connectorDAO:      dao.NewConnectorDAO(),
		knowledgebaseDAO:  dao.NewKnowledgebaseDAO(),
		userTenantDAO:     dao.NewUserTenantDAO(),
		connectorRegistry: newConnectorRegistry(),
	}
}

func newConnectorRegistry() *syncerconnector.Registry {
	registry := syncerconnector.NewRegistry()
	syncerconnector.RegisterBuiltIns(registry)
	return registry
}

// ListConnectorsResponse list connectors response
type ListConnectorsResponse struct {
	Connectors []*dao.ConnectorListItem `json:"connectors"`
}

// CreateConnectorRequest holds the fields used to create a connector.
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

// ResumeFailedSyncRequest resumes a failed connector sync task from checkpoint.
type ResumeFailedSyncRequest struct {
	KbID   string `json:"kb_id"`
	TaskID string `json:"task_id"`
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

type boxWebOAuthState struct {
	UserID       string `json:"user_id"`
	AuthURL      string `json:"auth_url"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
	CreatedAt    int64  `json:"created_at"`
}

type boxWebOAuthCredentials struct {
	UserID       string `json:"user_id,omitempty"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type StartBoxWebOAuthRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri,omitempty"`
}

type StartBoxWebOAuthResponse struct {
	FlowID           string `json:"flow_id"`
	AuthorizationURL string `json:"authorization_url"`
	ExpiresIn        int64  `json:"expires_in"`
}

type PollBoxWebOAuthResultRequest struct {
	FlowID string `json:"flow_id"`
}

type PollBoxWebOAuthResultResponse struct {
	Credentials boxWebOAuthCredentials `json:"credentials"`
}

type boxOAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	RestrictedTo any    `json:"restricted_to,omitempty"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
}

// canAccessConnector Test Authentication
func (s *ConnectorService) canAccessConnector(ctx context.Context, connector *entity.Connector, userID string) (bool, error) {
	if connector.TenantID == userID {
		return true, nil
	}
	_, err := s.userTenantDAO.FilterByUserIDAndTenantID(ctx, dao.DB, userID, connector.TenantID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// cancelConnectorTasks Stop connector tasks
func (s *ConnectorService) cancelConnectorTasks(ctx context.Context, connectorID string) error {
	if err := s.connectorDAO.CancelRunningOrScheduledLogs(ctx, dao.DB, connectorID); err != nil {
		return err
	}
	return s.connectorDAO.UpdateByID(ctx, dao.DB, connectorID, map[string]interface{}{"status": string(entity.TaskStatusCancel)})
}

// CreateConnector creates a connector owned by the current user.
func (s *ConnectorService) CreateConnector(ctx context.Context, userID string, req *CreateConnectorRequest) (*entity.Connector, error) {
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
		ID:          utility.GenerateUUID(),
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

	if err := s.connectorDAO.Create(ctx, dao.DB, connector); err != nil {
		return nil, err
	}

	return s.connectorDAO.GetByID(ctx, dao.DB, connector.ID)
}

// GetConnector returns one connector when the user can access its tenant.
func (s *ConnectorService) GetConnector(ctx context.Context, connectorID, userID string) (*entity.Connector, error) {
	if strings.TrimSpace(connectorID) == "" {
		return nil, ErrConnectorIDRequired
	}

	connector, err := s.connectorDAO.GetByID(ctx, dao.DB, connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrConnectorNotFound
		}
		common.Error("get connector failed", err, zap.String("connector_id", connectorID))
		return nil, ErrConnectorInternal
	}

	canAccess, err := s.canAccessConnector(ctx, connector, userID)
	if err != nil {
		return nil, err
	}
	if !canAccess {
		return nil, ErrConnectorNoAuth
	}
	return connector, nil
}

// ListConnectors list connectors for a user
func (s *ConnectorService) ListConnectors(ctx context.Context, userID string) (*ListConnectorsResponse, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	// Query connectors by tenant ID
	connectors, err := s.connectorDAO.ListByTenantID(ctx, dao.DB, userID)
	if err != nil {
		return nil, err
	}

	return &ListConnectorsResponse{
		Connectors: connectors,
	}, nil
}

// accessible reports whether the user can access the connector's tenant.
func (s *ConnectorService) accessible(ctx context.Context, connectorID, userID string) (bool, error) {
	connector, err := s.connectorDAO.GetByID(ctx, dao.DB, connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrConnectorNotFound
		}
		return false, err
	}
	return s.canAccessConnector(ctx, connector, userID)
}

// TestConnector validates connector settings without persisting or syncing.
func (s *ConnectorService) TestConnector(ctx context.Context, connectorID, userID string, config entity.JSONMap) error {
	var storedConnector *entity.Connector
	if connectorID != "" {
		ok, err := s.accessible(ctx, connectorID, userID)
		if err != nil && !errors.Is(err, ErrConnectorNotFound) {
			return err
		}
		if err == nil && !ok {
			return ErrConnectorNoAuth
		}
		if err == nil {
			storedConnector, err = s.connectorDAO.GetByID(ctx, dao.DB, connectorID)
			if err != nil {
				return ErrConnectorNotFound
			}
		}
		if errors.Is(err, ErrConnectorNotFound) && config == nil {
			return ErrConnectorNotFound
		}
	}

	source, connectorConfig, err := testConnectorSettings(storedConnector, config)
	if err != nil {
		return err
	}
	connector, err := s.connectorRegistry.OpenFromConfig(source, connectorConfig)
	if err != nil {
		var unsupported *syncerconnector.UnsupportedSourceError
		if errors.As(err, &unsupported) {
			return fmt.Errorf("%w: %s", ErrConnectorSourceNotImplemented, unsupported.Source)
		}
		return err
	}
	validator, ok := connector.(syncerconnector.SettingValidator)
	if !ok {
		return ErrConnectorTestUnsupported
	}
	return wrapConnectorValidationError(validator.ValidateConnectorSetting(ctx, connectorConfig))
}

func wrapConnectorValidationError(err error) error {
	if err == nil {
		return nil
	}
	var (
		valErr  *syncerconnector.ConnectorValidationError
		credErr *syncerconnector.ConnectorMissingCredentialError
		rateErr *syncerconnector.RateLimitTriedTooManyTimesError
	)
	if errors.As(err, &valErr) || errors.As(err, &credErr) || errors.As(err, &rateErr) {
		return err
	}
	return &syncerconnector.ConnectorValidationError{Message: err.Error()}
}

func testConnectorSettings(stored *entity.Connector, request entity.JSONMap) (string, entity.JSONMap, error) {
	source := ""
	var config entity.JSONMap
	if stored != nil {
		source = strings.TrimSpace(stored.Source)
		config = stored.Config
	}
	if request != nil {
		if value := strings.TrimSpace(stringConfigValue(request["source"])); value != "" {
			source = value
		}
		if nested, ok := request["config"]; ok {
			config = jsonMapValue(nested)
		} else if _, ok := request["source"]; !ok {
			config = request
		}
	}
	if source == "" {
		return "", nil, fmt.Errorf("connector source is required")
	}
	if config == nil {
		return "", nil, fmt.Errorf("connector configuration is missing")
	}
	return source, config, nil
}

func jsonMapValue(value any) entity.JSONMap {
	switch typed := value.(type) {
	case entity.JSONMap:
		return typed
	case map[string]any:
		return entity.JSONMap(typed)
	default:
		return nil
	}
}

func stringConfigValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func (s *ConnectorService) StartGoogleWebOAuth(ctx context.Context, userID, source string, req *StartGoogleWebOAuthRequest) (*StartGoogleWebOAuthResponse, common.ErrorCode, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		source = "google-drive"
	}
	if source != "google-drive" && source != "gmail" {
		return nil, common.CodeArgumentError, fmt.Errorf("invalid Google OAuth type")
	}

	if req == nil || len(req.Credentials) == 0 {
		return nil, common.CodeArgumentError, fmt.Errorf("required argument is missing: credentials")
	}

	redirectURI := strings.TrimSpace(req.RedirectURI)
	if redirectURI == "" {
		redirectURI = defaultGoogleWebOAuthRedirectURI(source)
	}
	if redirectURI == "" {
		return nil, common.CodeServerError, fmt.Errorf("not configure Google OAuth redirect URI on the server")
	}

	credentials, err := loadGoogleCredentials(req.Credentials)
	if err != nil {
		return nil, common.CodeArgumentError, err
	}
	if hasRefreshToken(credentials) {
		return nil, common.CodeArgumentError, fmt.Errorf("uploaded credentials already include a refresh token")
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
		return nil, common.CodeServerError, fmt.Errorf("failed to initialize Google OAuth flow. Please verify the uploaded client configuration")
	}

	codeVerifier, codeChallenge, err := newPKCEChallenge()
	if err != nil {
		return nil, common.CodeServerError, err
	}

	flowID := utility.GenerateUUID()
	authorizationURL, err := buildGoogleAuthorizationURL(authURI, clientID, redirectURI, flowID, googleOAuthScopesForSource(source), codeChallenge)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to initialize Google OAuth flow. Please verify the uploaded client configuration")
	}

	redisClient := redis.Get()
	if redisClient == nil {
		return nil, common.CodeServerError, fmt.Errorf("no configure Redis on the server")
	}

	state := googleWebOAuthState{
		UserID:       userID,
		ClientConfig: clientConfig,
		RedirectURI:  redirectURI,
		CodeVerifier: codeVerifier,
		CreatedAt:    time.Now().Unix(),
	}
	if ok := redisClient.SetObj(ctx, webStateCacheKey(flowID, source), state, webFlowTTL); !ok {
		return nil, common.CodeServerError, fmt.Errorf("failed to initialize Google OAuth flow. Please verify the uploaded client configuration")
	}

	return &StartGoogleWebOAuthResponse{
		FlowID:           flowID,
		AuthorizationURL: authorizationURL,
		ExpiresIn:        int64(webFlowTTL.Seconds()),
	}, common.CodeSuccess, nil
}

func (s *ConnectorService) GoogleWebOAuthCallback(ctx context.Context, source, stateID, oauthError, errorDescription, code string) string {
	source = strings.TrimSpace(source)
	if source != "google-drive" && source != "gmail" {
		return renderWebOAuthPopup("", false, "Invalid Google OAuth type.", source)
	}

	stateID = strings.TrimSpace(stateID)
	if stateID == "" {
		return renderWebOAuthPopup("", false, "Missing OAuth state parameter.", source)
	}

	redisClient := redis.Get()
	if redisClient == nil {
		return renderWebOAuthPopup(stateID, false, "Authorization session expired. Please restart from the main window.", source)
	}

	stateKey := webStateCacheKey(stateID, source)
	var state googleWebOAuthState
	if ok := redisClient.GetObj(ctx, stateKey, &state); !ok {
		return renderWebOAuthPopup(stateID, false, "Authorization session expired. Please restart from the main window.", source)
	}

	if state.ClientConfig == nil {
		redisClient.Delete(ctx, stateKey)
		return renderWebOAuthPopup(stateID, false, "Authorization session was invalid. Please retry.", source)
	}

	if strings.TrimSpace(oauthError) != "" {
		redisClient.Delete(ctx, stateKey)
		message := strings.TrimSpace(errorDescription)
		if message == "" {
			message = strings.TrimSpace(oauthError)
		}
		if message == "" {
			message = "Authorization was cancelled."
		}
		return renderWebOAuthPopup(stateID, false, message, source)
	}

	code = strings.TrimSpace(code)
	if code == "" {
		return renderWebOAuthPopup(stateID, false, "Missing authorization code from Google.", source)
	}

	credentials, err := exchangeGoogleWebOAuthCode(state.ClientConfig, googleOAuthScopesForSource(source), state.RedirectURI, code, state.CodeVerifier)
	if err != nil {
		redisClient.Delete(ctx, stateKey)
		return renderWebOAuthPopup(stateID, false, "Failed to exchange tokens with Google. Please retry.", source)
	}

	result := googleWebOAuthResult{
		UserID:      state.UserID,
		Credentials: credentials,
	}
	if ok := redisClient.SetObj(ctx, webResultCacheKey(stateID, source), result, webFlowTTL); !ok {
		redisClient.Delete(ctx, stateKey)
		return renderWebOAuthPopup(stateID, false, "Failed to exchange tokens with Google. Please retry.", source)
	}
	redisClient.Delete(ctx, stateKey)

	return renderWebOAuthPopup(stateID, true, "Authorization completed successfully.", source)
}

func (s *ConnectorService) PollGoogleWebOAuthResult(ctx context.Context, userID, source string, req *PollGoogleWebOAuthResultRequest) (*PollGoogleWebOAuthResultResponse, common.ErrorCode, error) {
	source = strings.TrimSpace(source)
	if source != "google-drive" && source != "gmail" {
		return nil, common.CodeArgumentError, fmt.Errorf("invalid Google OAuth type")
	}
	if req == nil || strings.TrimSpace(req.FlowID) == "" {
		return nil, common.CodeArgumentError, fmt.Errorf("required argument is missing: flow_id")
	}

	redisClient := redis.Get()
	if redisClient == nil {
		return nil, common.CodeRunning, fmt.Errorf("authorization is still pending")
	}

	resultKey := webResultCacheKey(strings.TrimSpace(req.FlowID), source)
	var result googleWebOAuthResult
	if ok := redisClient.GetObj(ctx, resultKey, &result); !ok {
		return nil, common.CodeRunning, fmt.Errorf("authorization is still pending")
	}

	if result.UserID != userID {
		return nil, common.CodePermissionError, fmt.Errorf("you are not allowed to access this authorization result")
	}

	redisClient.Delete(ctx, resultKey)
	return &PollGoogleWebOAuthResultResponse{Credentials: result.Credentials}, common.CodeSuccess, nil
}

func defaultGoogleWebOAuthRedirectURI(source string) string {
	if source == "gmail" {
		return getEnvDefault(common.EnvGmailWebOAuthRedirectURI, "http://localhost:9384/api/v1/connectors/gmail/oauth/web/callback")
	}
	return getEnvDefault(common.EnvGoogleDriveWebOAuthRedirectURI, "http://localhost:9384/api/v1/connectors/google-drive/oauth/web/callback")
}

func getEnvDefault(key, fallback string) string {
	if value := strings.TrimSpace(common.GetEnv(key)); value != "" {
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
		return nil, fmt.Errorf("invalid Google credentials JSON")
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(rawString)), &credentials); err != nil || credentials == nil {
		return nil, fmt.Errorf("invalid Google credentials JSON")
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
		return nil, fmt.Errorf("google OAuth JSON must include a 'web' client configuration to use browser-based authorization")
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

func renderWebOAuthPopup(flowID string, success bool, message, source string) string {
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

	title := fmt.Sprintf("%s Authorization", webOAuthSourceDisplayName(source))
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

func webOAuthSourceDisplayName(source string) string {
	if source == "gmail" {
		return "Gmail"
	}
	if source == "google-drive" {
		return "Google Drive"
	}
	if source == "box" {
		return "Box"
	}
	return "OAuth"
}

func (s *ConnectorService) DeleteConnector(ctx context.Context, connectorID, userID string) (bool, common.ErrorCode, error) {
	if connectorID == "" {
		return false, common.CodeDataError, fmt.Errorf("connector_id is required")
	}

	connector, err := s.connectorDAO.GetByID(ctx, dao.DB, connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, common.CodeDataError, fmt.Errorf("can't find this Connector")
		}
		return false, common.CodeServerError, err
	}

	canAccess, err := s.canAccessConnector(ctx, connector, userID)
	if err != nil {
		return false, common.CodeServerError, err
	}
	if !canAccess {
		return false, common.CodeAuthenticationError, fmt.Errorf("no authorization")
	}

	if err = s.cancelConnectorTasks(ctx, connector.ID); err != nil {
		return false, common.CodeServerError, err
	}

	if err = s.connectorDAO.DeleteByID(ctx, dao.DB, connector.ID); err != nil {
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

func (s *ConnectorService) UpdateConnector(ctx context.Context, connectorID, userID string, req *UpdateConnectorRequest) (*entity.Connector, common.ErrorCode, error) {
	if connectorID == "" {
		return nil, common.CodeDataError, fmt.Errorf("connector_id is required")
	}

	connector, err := s.connectorDAO.GetByID(ctx, dao.DB, connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.CodeDataError, fmt.Errorf("can't find this Connector")
		}
		return nil, common.CodeServerError, err
	}

	canAccess, err := s.canAccessConnector(ctx, connector, userID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if !canAccess {
		return nil, common.CodeAuthenticationError, fmt.Errorf("no authorization")
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
		if err = s.connectorDAO.UpdateByID(ctx, dao.DB, connectorID, updates); err != nil {
			return nil, common.CodeServerError, err
		}
	}

	if req != nil {
		if req.Reschedule {
			if err = s.cancelConnectorTasks(ctx, connectorID); err != nil {
				return nil, common.CodeServerError, err
			}
			taskIDs, err := s.connectorDAO.ScheduleConnectorTasks(ctx, dao.DB, connectorID)
			if err != nil {
				return nil, common.CodeServerError, err
			}
			publishSyncerTasks(taskIDs)
		} else if isConnectorCancelStatus(req.Status) {
			if err = s.cancelConnectorTasks(ctx, connectorID); err != nil {
				return nil, common.CodeServerError, err
			}
		} else if isConnectorScheduleStatus(req.Status) {
			taskIDs, err := s.connectorDAO.ScheduleConnectorTasks(ctx, dao.DB, connectorID)
			if err != nil {
				return nil, common.CodeServerError, err
			}
			publishSyncerTasks(taskIDs)
		}
	}

	connector, err = s.connectorDAO.GetByID(ctx, dao.DB, connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.CodeDataError, fmt.Errorf("can't find this Connector")
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
func (s *ConnectorService) RebuildConnector(ctx context.Context, connectorID, userID, kbID string) (bool, common.ErrorCode, error) {
	if connectorID == "" {
		return false, common.CodeDataError, fmt.Errorf("connector_id is required")
	}
	if kbID == "" {
		return false, common.CodeArgumentError, fmt.Errorf("required argument is missing: kb_id")
	}

	connector, err := s.connectorDAO.GetByID(ctx, dao.DB, connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, common.CodeDataError, fmt.Errorf("can't find this Connector")
		}
		return false, common.CodeServerError, err
	}

	canAccess, err := s.canAccessConnector(ctx, connector, userID)
	if err != nil {
		return false, common.CodeServerError, err
	}
	if !canAccess {
		return false, common.CodeAuthenticationError, fmt.Errorf("no authorization")
	}

	// The caller-supplied kb is targeted by delete + re-sync below, so it must
	// be accessible to the caller and the connector must be bound to it.
	if !s.knowledgebaseDAO.Accessible(ctx, dao.DB, kbID, userID) {
		common.Warn("rebuild denied: kb not accessible",
			zap.String("connector_id", connectorID), zap.String("kb_id", kbID), zap.String("user_id", userID))
		return false, common.CodeAuthenticationError, ErrConnectorNoAuth
	}
	bound, err := s.connectorDAO.Connector2KBExists(ctx, dao.DB, connectorID, kbID)
	if err != nil {
		return false, common.CodeServerError, err
	}
	if !bound {
		common.Warn("rebuild denied: connector not bound to kb",
			zap.String("connector_id", connectorID), zap.String("kb_id", kbID), zap.String("user_id", userID))
		return false, common.CodeAuthenticationError, ErrConnectorNotBoundToKB
	}

	sourceType := fmt.Sprintf("%s/%s", connector.Source, connector.ID)
	documents, err := s.connectorDAO.ListDocumentsByKBAndSourceType(ctx, dao.DB, kbID, sourceType)
	if err != nil {
		return false, common.CodeServerError, err
	}

	s.deleteConnectorDocumentChunks(ctx, connector.TenantID, kbID, documents)

	taskIDs, oldSyncTaskIDs, err := s.connectorDAO.RebuildConnector(ctx, dao.DB, connector, kbID, documents)
	if err != nil {
		return false, common.CodeServerError, err
	}

	if err = deleteSyncCheckpoints(ctx, oldSyncTaskIDs); err != nil {
		common.Warn("delete sync checkpoints failed during rebuild",
			zap.String("connector_id", connectorID), zap.Error(err))
	}

	publishSyncerTasks(taskIDs)
	return true, common.CodeSuccess, nil
}

// ResumeFailedSync schedules a failed sync task to continue from its saved checkpoint.
func (s *ConnectorService) ResumeFailedSync(ctx context.Context, connectorID, userID string, req *ResumeFailedSyncRequest) (bool, common.ErrorCode, error) {
	if connectorID == "" {
		return false, common.CodeDataError, fmt.Errorf("connector_id is required")
	}
	if req == nil {
		return false, common.CodeArgumentError, fmt.Errorf("request is required")
	}
	// check KBid and TaskID
	req.KbID = strings.TrimSpace(req.KbID)
	req.TaskID = strings.TrimSpace(req.TaskID)
	if req.KbID == "" {
		return false, common.CodeArgumentError, fmt.Errorf("required argument is missing: kb_id")
	}
	if req.TaskID == "" {
		return false, common.CodeArgumentError, fmt.Errorf("required argument is missing: task_id")
	}
	// get connector
	connector, err := s.connectorDAO.GetByID(ctx, dao.DB, connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, common.CodeDataError, fmt.Errorf("can't find this Connector")
		}
		return false, common.CodeServerError, err
	}
	// check access
	canAccess, err := s.canAccessConnector(ctx, connector, userID)
	if err != nil {
		return false, common.CodeServerError, err
	}
	if !canAccess {
		return false, common.CodeAuthenticationError, fmt.Errorf("no authorization")
	}

	// load the checkpoint
	loader, ok := getSyncCheckpointLoader()
	if !ok {
		return false, common.CodeServerError, fmt.Errorf("sync checkpoint store is not configured")
	}

	checkpoint, err := loader.LoadSyncCheckpoint(ctx, req.TaskID)
	if err != nil {
		return false, common.CodeServerError, err
	}
	if checkpoint == nil || checkpoint.Checkpoint == nil {
		return false, common.CodeDataError, fmt.Errorf("checkpoint not found, cannot resume failed sync task")
	}
	if checkpoint.TaskID != req.TaskID || checkpoint.ConnectorID != connectorID || checkpoint.KBID != req.KbID {
		return false, common.CodeDataError, fmt.Errorf("checkpoint does not match connector, knowledge base, and task")
	}
	// resume the task
	if err = s.connectorDAO.ResumeFailedSyncTask(ctx, dao.DB, connectorID, req.KbID, req.TaskID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, common.CodeDataError, fmt.Errorf("failed sync task not found")
		}
		return false, common.CodeServerError, err
	}
	if err = publishSyncerTaskWakeup(req.TaskID); err != nil {
		return false, common.CodeServerError, err
	}
	return true, common.CodeSuccess, nil
}

func publishSyncerTasks(taskIDs []string) {
	if len(taskIDs) == 0 {
		return
	}
	publisher, ok := getSyncerTaskPublisher()
	if !ok {
		common.Warn("syncer task publisher is not configured")
		return
	}
	for _, taskID := range taskIDs {
		if taskID == "" {
			continue
		}
		if err := publisher.PublishSyncerTask(taskID); err != nil {
			common.Warn("syncer task publish failed", zap.String("task_id", taskID), zap.Error(err))
		}
	}
}

// publishSyncerTaskWakeup publish syncer task wakeup message to nats
func publishSyncerTaskWakeup(taskID string) error {
	publisher, ok := getSyncerTaskPublisher()
	if !ok {
		return fmt.Errorf("syncer task publisher is not configured")
	}
	if wakeupPublisher, ok := publisher.(syncTaskWakeupPublisher); ok {
		return wakeupPublisher.PublishSyncerTaskWakeup(taskID)
	}
	return publisher.PublishSyncerTask(taskID)
}

func deleteSyncCheckpoints(ctx context.Context, taskIDs []string) error {
	if len(taskIDs) == 0 {
		return nil
	}
	deleter, ok := getSyncCheckpointDeleter()
	if !ok {
		return fmt.Errorf("sync checkpoint store is not configured")
	}
	for _, taskID := range taskIDs {
		if taskID == "" {
			continue
		}
		if err := deleter.DeleteSyncCheckpoint(ctx, taskID); err != nil {
			return err
		}
	}
	return nil
}

func (s *ConnectorService) deleteConnectorDocumentChunks(ctx context.Context, tenantID, kbID string, documents []*entity.Document) {
	docEngine := engine.Get()
	if docEngine == nil {
		return
	}

	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	for _, document := range documents {
		_, _ = docEngine.DeleteChunks(ctx, map[string]interface{}{"doc_id": document.ID}, indexName, kbID)
	}
}

func (s *ConnectorService) ListLog(ctx context.Context, connectorID, userID string, page, pageSize int) ([]*entity.ConnectorSyncLog, int64, common.ErrorCode, error) {
	if connectorID == "" {
		return nil, 0, common.CodeDataError, fmt.Errorf("connector_id is required")
	}

	connector, err := s.connectorDAO.GetByID(ctx, dao.DB, connectorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, common.CodeDataError, fmt.Errorf("can't find this Connector")
		}
		return nil, 0, common.CodeServerError, err
	}

	canAccess, err := s.canAccessConnector(ctx, connector, userID)
	if err != nil {
		return nil, 0, common.CodeServerError, err
	}
	if !canAccess {
		return nil, 0, common.CodeAuthenticationError, fmt.Errorf("no authorization")
	}

	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 15
	}
	offset := (page - 1) * pageSize

	logs, total, err := s.connectorDAO.ListLogsByConnectorID(ctx, dao.DB, connectorID, offset, pageSize)
	if err != nil {
		return nil, 0, common.CodeServerError, fmt.Errorf("failed to fetch connector logs: %w", err)
	}
	if logs == nil {
		logs = []*entity.ConnectorSyncLog{}
	}
	return logs, total, common.CodeSuccess, nil
}

// ListLogs lists sync logs for the current user with pagination.
// When datasetID is non-empty, only logs of that dataset are returned.
func (s *ConnectorService) ListLogs(ctx context.Context, userID, datasetID string, page, pageSize int) ([]*entity.ConnectorSyncLog, int64, common.ErrorCode, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, 0, common.CodeDataError, fmt.Errorf("user_id is required")
	}

	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(ctx, dao.DB, userID)
	if err != nil {
		return nil, 0, common.CodeServerError, err
	}
	tenantIDs = append(tenantIDs, userID)

	offset, limit := 0, pageSize
	if pageSize <= 0 {
		// pageSize == 0 means no pagination: return every matching row.
		limit = 0
	} else {
		if page < 1 {
			page = 1
		}
		if pageSize > 100 {
			limit = 15
		}
		offset = (page - 1) * limit
	}

	logs, total, err := s.connectorDAO.ListLogs(ctx, dao.DB, tenantIDs, datasetID, offset, limit)
	if err != nil {
		return nil, 0, common.CodeServerError, fmt.Errorf("failed to fetch sync logs: %w", err)
	}
	if logs == nil {
		logs = []*entity.ConnectorSyncLog{}
	}
	return logs, total, common.CodeSuccess, nil
}

func (s *ConnectorService) StartBoxWebOAuth(ctx context.Context, userID string, req *StartBoxWebOAuthRequest) (*StartBoxWebOAuthResponse, common.ErrorCode, error) {
	var clientID, clientSecret, redirectURI string
	if req != nil {
		clientID = strings.TrimSpace(req.ClientID)
		clientSecret = strings.TrimSpace(req.ClientSecret)
		redirectURI = strings.TrimSpace(req.RedirectURI)
	}
	if clientID == "" || clientSecret == "" {
		return nil, common.CodeArgumentError, fmt.Errorf("box client_id and client_secret are required")
	}
	if redirectURI == "" {
		redirectURI = defaultBoxWebOAuthRedirectURI()
	}

	flowID := utility.GenerateUUID()
	authorizationURL, err := buildBoxAuthorizationURL(clientID, redirectURI, flowID)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	redisClient := connectorRedisGet()
	if redisClient == nil {
		return nil, common.CodeServerError, fmt.Errorf("not connected Redis client on the server")
	}

	state := boxWebOAuthState{
		UserID:       userID,
		AuthURL:      authorizationURL,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURI:  redirectURI,
		CreatedAt:    time.Now().Unix(),
	}
	if ok := redisClient.SetObj(ctx, webStateCacheKey(flowID, "box"), state, webFlowTTL); !ok {
		return nil, common.CodeServerError, fmt.Errorf("failed to initialize Box OAuth flow. Please verify the client configuration")
	}

	return &StartBoxWebOAuthResponse{
		FlowID:           flowID,
		AuthorizationURL: authorizationURL,
		ExpiresIn:        int64(webFlowTTL.Seconds()),
	}, common.CodeSuccess, nil
}

func (s *ConnectorService) BoxWebOAuthCallback(ctx context.Context, flowID string, oauthError string, errorDescription string, code string) string {
	flowID = strings.TrimSpace(flowID)
	if flowID == "" {
		return renderWebOAuthPopup("", false, "Missing OAuth parameters.", "box")
	}

	redisClient := connectorRedisGet()
	if redisClient == nil {
		return renderWebOAuthPopup(flowID, false, "Box OAuth session expired or invalid.", "box")
	}

	stateKey := webStateCacheKey(flowID, "box")
	var state boxWebOAuthState
	if ok := redisClient.GetObj(ctx, stateKey, &state); !ok {
		return renderWebOAuthPopup(flowID, false, "Box OAuth session expired or invalid.", "box")
	}

	if strings.TrimSpace(oauthError) != "" {
		redisClient.Delete(ctx, stateKey)
		message := strings.TrimSpace(errorDescription)
		if message == "" {
			message = strings.TrimSpace(oauthError)
		}
		if message == "" {
			message = "Authorization failed."
		}
		return renderWebOAuthPopup(flowID, false, message, "box")
	}

	code = strings.TrimSpace(code)
	if code == "" {
		return renderWebOAuthPopup(flowID, false, "Missing authorization code from Box.", "box")
	}

	token, err := exchangeBoxAuthorizationCode(state.ClientID, state.ClientSecret, state.RedirectURI, code)
	if err != nil {
		redisClient.Delete(ctx, stateKey)
		return renderWebOAuthPopup(flowID, false, "Failed to exchange tokens with Box. Please retry.", "box")
	}

	result := boxWebOAuthCredentials{
		UserID:       state.UserID,
		ClientID:     state.ClientID,
		ClientSecret: state.ClientSecret,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
	}
	if ok := redisClient.SetObj(ctx, webResultCacheKey(flowID, "box"), result, webFlowTTL); !ok {
		redisClient.Delete(ctx, stateKey)
		return renderWebOAuthPopup(flowID, false, "Failed to exchange tokens with Box. Please retry.", "box")
	}
	redisClient.Delete(ctx, stateKey)

	return renderWebOAuthPopup(flowID, true, "Authorization completed successfully.", "box")
}

func (s *ConnectorService) PollBoxWebOAuthResult(ctx context.Context, userID string, req *PollBoxWebOAuthResultRequest) (*PollBoxWebOAuthResultResponse, common.ErrorCode, error) {
	if req == nil || strings.TrimSpace(req.FlowID) == "" {
		return nil, common.CodeArgumentError, fmt.Errorf("required argument is missing: flow_id")
	}

	redisClient := connectorRedisGet()
	if redisClient == nil {
		return nil, common.CodeRunning, fmt.Errorf("authorization is still pending")
	}

	resultKey := webResultCacheKey(strings.TrimSpace(req.FlowID), "box")
	var result boxWebOAuthCredentials
	if ok := redisClient.GetObj(ctx, resultKey, &result); !ok {
		return nil, common.CodeRunning, fmt.Errorf("authorization is still pending")
	}

	if result.UserID != userID {
		return nil, common.CodePermissionError, fmt.Errorf("you are not allowed to access this authorization result")
	}

	redisClient.Delete(ctx, resultKey)
	result.UserID = ""
	return &PollBoxWebOAuthResultResponse{Credentials: result}, common.CodeSuccess, nil
}

func defaultBoxWebOAuthRedirectURI() string {
	return getEnvDefault(
		common.EnvBoxWebOAuthRedirectURI,
		"http://localhost:9384/api/v1/connectors/box/oauth/web/callback",
	)
}

func buildBoxAuthorizationURL(clientID string, redirectURI string, state string) (string, error) {
	parsedURL, err := url.Parse(boxOAuthAuthorizeURL)
	if err != nil {
		return "", err
	}

	query := parsedURL.Query()
	query.Set("client_id", clientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("response_type", "code")
	query.Set("state", state)
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

func exchangeBoxAuthorizationCode(clientID string, clientSecret string, redirectURI string, code string) (*boxOAuthTokenResponse, error) {
	_ = redirectURI

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)

	ctx, cancel := context.WithTimeout(context.Background(), googleOAuthHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, boxOAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var token boxOAuthTokenResponse
	if err = json.Unmarshal(body, &token); err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest || token.Error != "" {
		if token.ErrorDesc != "" {
			return nil, errors.New(token.ErrorDesc)
		}
		if token.Error != "" {
			return nil, errors.New(token.Error)
		}
		return nil, fmt.Errorf("box token exchange failed: HTTP %d", resp.StatusCode)
	}
	if token.AccessToken == "" {
		return nil, fmt.Errorf("box token exchange failed: empty access_token")
	}
	return &token, nil
}
