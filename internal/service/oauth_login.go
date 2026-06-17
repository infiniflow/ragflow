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
	"encoding/hex"
	"errors"
	"fmt"
	"ragflow/internal/engine/redis"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/server"
	"ragflow/internal/utility"
	"ragflow/internal/utility/oauth"
)

// Sentinel errors surfaced by the OAuth login + callback endpoints. The
// handler maps each to one of Python's `?error=` redirect codes so the
// frontend can show the same messages.
var (
	// ErrOAuthInvalidChannel mirrors Python's ValueError("Invalid channel name: ...").
	ErrOAuthInvalidChannel = errors.New("invalid channel name")
	// ErrOAuthInvalidState is returned when the callback's state mismatches the
	// stored one (CSRF guard) — maps to "?error=invalid_state".
	ErrOAuthInvalidState = errors.New("invalid_state")
	// ErrOAuthMissingCode is returned when the callback URL has no code
	// query param — maps to "?error=missing_code".
	ErrOAuthMissingCode = errors.New("missing_code")
	// ErrOAuthTokenFailed is returned when token exchange yields no
	// access_token — maps to "?error=token_failed".
	ErrOAuthTokenFailed = errors.New("token_failed")
	// ErrOAuthEmailMissing is returned when the /userinfo response has no
	// email claim — maps to "?error=email_missing".
	ErrOAuthEmailMissing = errors.New("email_missing")
	// ErrOAuthUserInactive is returned when the matched user has
	// is_active=0 — maps to "?error=user_inactive".
	ErrOAuthUserInactive = errors.New("user_inactive")
)

// oauthStateTTL bounds how long an in-flight OAuth state token is honored.
// Five minutes matches typical session-cookie flows used by the Python
// counterpart.
const oauthStateTTL = 5 * time.Minute

// oauthStateKey is the Redis key prefix for an in-flight OAuth state.
const oauthStateKey = "oauth:state:"

// OAuthLoginInit prepares a redirect to the channel's authorization URL.
// The returned state must also be set as a short-lived cookie on the caller
// so the callback can perform a CSRF check that ties the state token to the
// browser that initiated the flow.
type OAuthLoginInit struct {
	State        string
	AuthURL      string
	Channel      string
	CookieMaxAge time.Duration
}

// OAuthLoginInitiate generates a state, persists it in Redis with a TTL,
// and returns the authorization URL the browser should be redirected to.
// Mirrors the body of Python's oauth_login.
func (s *UserService) OAuthLoginInitiate(channel string, redis *redis.RedisClient) (*OAuthLoginInit, common.ErrorCode, error) {
	cfg, ok := lookupOAuthConfig(channel)
	if !ok {
		return nil, common.CodeDataError, fmt.Errorf("%w: %s", ErrOAuthInvalidChannel, channel)
	}
	client, err := oauth.NewClient(toOAuthClientConfig(cfg))
	if err != nil {
		return nil, common.CodeServerError, err
	}

	state, err := generateOAuthState()
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("generate oauth state: %w", err)
	}
	if redis != nil {
		if ok := redis.Set(oauthStateKey+state, channel, oauthStateTTL); !ok {
			return nil, common.CodeServerError, errors.New("failed to persist oauth state")
		}
	}

	authURL, err := client.AuthorizationURL(state)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	return &OAuthLoginInit{
		State:        state,
		AuthURL:      authURL,
		Channel:      channel,
		CookieMaxAge: oauthStateTTL,
	}, common.CodeSuccess, nil
}

// OAuthCallbackResult is returned to the handler after a successful callback
// so it can mint the user-facing auth response (cookie + redirect).
type OAuthCallbackResult struct {
	User      *entity.User
	IsNewUser bool
}

// OAuthCallback verifies the callback state, exchanges the code for a token,
// fetches the user profile, and either creates a new local user or logs an
// existing one in. Mirrors the body of Python's oauth_callback.
//
// expectedState is the value the handler pulled out of the state cookie.
// When redis is non-nil the state is also verified against and consumed
// from Redis, defending against a replay where an attacker fishes a valid
// state out of a victim's URL but does not have the cookie.
func (s *UserService) OAuthCallback(ctx context.Context, channel, code, callbackState, expectedState string, redis *redis.RedisClient) (*OAuthCallbackResult, common.ErrorCode, error) {
	cfg, ok := lookupOAuthConfig(channel)
	if !ok {
		return nil, common.CodeDataError, fmt.Errorf("%w: %s", ErrOAuthInvalidChannel, channel)
	}

	if callbackState == "" || expectedState == "" || callbackState != expectedState {
		return nil, common.CodeDataError, ErrOAuthInvalidState
	}
	if redis != nil {
		stored, _ := redis.Get(oauthStateKey + callbackState)
		// Delete unconditionally so a leaked state cannot be reused even
		// when the Redis lookup fails open above.
		redis.Delete(oauthStateKey + callbackState)
		if stored == "" || stored != channel {
			return nil, common.CodeDataError, ErrOAuthInvalidState
		}
	}
	if code == "" {
		return nil, common.CodeDataError, ErrOAuthMissingCode
	}

	client, err := oauth.NewClient(toOAuthClientConfig(cfg))
	if err != nil {
		return nil, common.CodeServerError, err
	}

	token, err := client.ExchangeCodeForToken(ctx, code)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("%w: %v", ErrOAuthTokenFailed, err)
	}
	if token.AccessToken == "" {
		return nil, common.CodeServerError, ErrOAuthTokenFailed
	}

	info, err := client.FetchUserInfo(ctx, token.AccessToken, token.IDToken)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("Failed to fetch user info: %v", err)
	}
	if info.Email == "" {
		return nil, common.CodeDataError, ErrOAuthEmailMissing
	}

	existing, err := s.userDAO.GetByEmail(info.Email)
	if err == nil && existing != nil {
		if existing.IsActive == "0" {
			return nil, common.CodeForbidden, ErrOAuthUserInactive
		}
		newToken := utility.GenerateToken()
		existing.AccessToken = &newToken
		now := time.Now().Truncate(time.Second)
		existing.LastLoginTime = &now
		if uerr := s.userDAO.Update(existing); uerr != nil {
			return nil, common.CodeServerError, fmt.Errorf("update user: %w", uerr)
		}
		return &OAuthCallbackResult{User: existing, IsNewUser: false}, common.CodeSuccess, nil
	}

	// New user — register a fresh account bound to this channel.
	created, ecode, cerr := s.registerOAuthUser(channel, info)
	if cerr != nil {
		return nil, ecode, cerr
	}
	return &OAuthCallbackResult{User: created, IsNewUser: true}, common.CodeSuccess, nil
}

// registerOAuthUser provisions a new user + tenant for an OAuth identity.
// Models the relevant fields the email-password Register path sets so the
// rest of the app (kbs, files, llm config) sees a fully-shaped tenant.
func (s *UserService) registerOAuthUser(channel string, info *oauth.UserInfo) (*entity.User, common.ErrorCode, error) {
	cfg := server.GetConfig()
	userID := utility.GenerateToken()
	accessToken := utility.GenerateToken()
	status := "1"
	loginChannel := channel
	isSuperuser := false
	nickname := info.Nickname
	if nickname == "" {
		nickname = info.Username
	}
	if nickname == "" {
		nickname = info.Email
	}

	user := &entity.User{
		ID:              userID,
		AccessToken:     &accessToken,
		Email:           info.Email,
		Nickname:        nickname,
		Avatar:          &info.AvatarURL,
		Status:          &status,
		IsActive:        "1",
		IsAuthenticated: "1",
		IsAnonymous:     "0",
		LoginChannel:    &loginChannel,
		IsSuperuser:     &isSuperuser,
	}

	tenantName := nickname + "'s Kingdom"
	tenant := &entity.Tenant{
		ID:        userID,
		Name:      &tenantName,
		LLMID:     cfg.UserDefaultLLM.DefaultModels.ChatModel.Name,
		EmbdID:    cfg.UserDefaultLLM.DefaultModels.EmbeddingModel.Name,
		ASRID:     cfg.UserDefaultLLM.DefaultModels.ASRModel.Name,
		Img2TxtID: cfg.UserDefaultLLM.DefaultModels.Image2TextModel.Name,
		RerankID:  cfg.UserDefaultLLM.DefaultModels.RerankModel.Name,
		ParserIDs: "naive:General,Q&A:Q&A,manual:Manual,table:Table,paper:Research Paper,book:Book,laws:Laws,presentation:Presentation,picture:Picture,one:One,audio:Audio,email:Email,tag:Tag",
		Status:    &status,
	}
	userTenantID := utility.GenerateToken()
	userTenant := &entity.UserTenant{
		ID:        userTenantID,
		UserID:    userID,
		TenantID:  userID,
		Role:      "owner",
		InvitedBy: userID,
		Status:    &status,
	}
	fileID := utility.GenerateToken()
	rootFile := &entity.File{
		ID:        fileID,
		ParentID:  fileID,
		TenantID:  userID,
		CreatedBy: userID,
		Name:      "/",
		Type:      "folder",
		Size:      0,
	}

	tenantDAO := dao.NewTenantDAO()
	userTenantDAO := dao.NewUserTenantDAO()
	fileDAO := dao.NewFileDAO()

	if err := s.userDAO.Create(user); err != nil {
		return nil, common.CodeServerError, fmt.Errorf("Failed to register %s: %w", info.Email, err)
	}
	if err := tenantDAO.Create(tenant); err != nil {
		_ = s.userDAO.DeleteByID(userID)
		return nil, common.CodeServerError, fmt.Errorf("Failed to register %s: %w", info.Email, err)
	}
	if err := userTenantDAO.Create(userTenant); err != nil {
		_ = s.userDAO.DeleteByID(userID)
		_ = tenantDAO.Delete(userID)
		return nil, common.CodeServerError, fmt.Errorf("Failed to register %s: %w", info.Email, err)
	}
	if err := fileDAO.Create(rootFile); err != nil {
		_ = s.userDAO.DeleteByID(userID)
		_ = tenantDAO.Delete(userID)
		_ = userTenantDAO.Delete(userTenantID)
		return nil, common.CodeServerError, fmt.Errorf("Failed to register %s: %w", info.Email, err)
	}
	return user, common.CodeSuccess, nil
}

// lookupOAuthConfig returns the configured OAuth channel by name. The lookup
// is case-insensitive against the keys server.GetConfig() materialises from
// the yaml config file.
func lookupOAuthConfig(channel string) (server.OAuthConfig, bool) {
	cfg := server.GetConfig()
	if cfg == nil {
		return server.OAuthConfig{}, false
	}
	if found, ok := cfg.OAuth[channel]; ok {
		return found, true
	}
	want := strings.ToLower(channel)
	for k, v := range cfg.OAuth {
		if strings.ToLower(k) == want {
			return v, true
		}
	}
	return server.OAuthConfig{}, false
}

// toOAuthClientConfig narrows server.OAuthConfig (which carries display-only
// fields like Icon) into the oauth.Config the client package consumes.
func toOAuthClientConfig(cfg server.OAuthConfig) oauth.Config {
	return oauth.Config{
		Type:             cfg.Type,
		ClientID:         cfg.ClientID,
		ClientSecret:     cfg.ClientSecret,
		AuthorizationURL: cfg.AuthorizationURL,
		TokenURL:         cfg.TokenURL,
		UserinfoURL:      cfg.UserinfoURL,
		RedirectURI:      cfg.RedirectURI,
		Scope:            cfg.Scope,
		Issuer:           cfg.Issuer,
	}
}

// generateOAuthState returns a cryptographically random 32-byte hex string
// used as the OAuth state parameter. 256 bits keeps collisions and
// brute-force guessing comfortably out of reach for the 5-minute TTL.
func generateOAuthState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
