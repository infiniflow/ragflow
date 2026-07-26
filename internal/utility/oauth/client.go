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

// Package oauth ports the auth-client surface from api/apps/auth (Python)
// to Go. It wires three flavors of OAuth/OIDC providers behind a common
// Client interface so the login + callback handlers can stay flavor-blind:
//
//   - "oauth2": vanilla OAuth 2.0 authorization-code flow with a
//     provider-supplied /userinfo endpoint
//   - "oidc": OAuth 2.0 + OIDC discovery via .well-known/openid-configuration
//   - "GitHub": OAuth 2.0 plus GitHub's split user / emails endpoints
//
// Note on OIDC ID-token validation: the Python OIDCClient verifies the
// id_token signature against the discovered JWKS and pulls extra claims out
// of it. We deliberately do not yet pull in a JWT library here; the
// /userinfo endpoint returns the same claims authenticated via the
// access_token, which is the path we use exclusively. This is documented on
// OIDCClient and tracked as a follow-up.
package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config is the channel configuration consumed by NewClient. It mirrors the
// shape of server.OAuthConfig but is copied here to keep this package free
// of imports from the rest of the server.
type Config struct {
	Type             string
	ClientID         string
	ClientSecret     string
	AuthorizationURL string
	TokenURL         string
	UserinfoURL      string
	RedirectURI      string
	Scope            string
	Issuer           string
}

// UserInfo is the normalized user profile returned by FetchUserInfo. Email
// is the only field treated as required by the callback handler; the rest
// are best-effort.
type UserInfo struct {
	Email     string `json:"email"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}

// Client is the auth-client surface used by the login + callback handlers.
type Client interface {
	AuthorizationURL(state string) (string, error)
	ExchangeCodeForToken(ctx context.Context, code string) (*TokenResponse, error)
	FetchUserInfo(ctx context.Context, accessToken, idToken string) (*UserInfo, error)
}

// TokenResponse is the subset of fields we use from the token endpoint
// response.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type,omitempty"`
	IDToken     string `json:"id_token,omitempty"`
	ExpiresIn   int    `json:"expires_in,omitempty"`
	Scope       string `json:"scope,omitempty"`
}

// HTTPRequestTimeout is the per-request timeout applied to token and
// userinfo calls. Matches the Python http_request_timeout (7s).
const HTTPRequestTimeout = 7 * time.Second

// NewClient returns the Client implementation matching cfg.Type. When type
// is empty, Issuer presence selects OIDC; otherwise OAuth2.
func NewClient(cfg Config) (Client, error) {
	t := strings.ToLower(strings.TrimSpace(cfg.Type))
	if t == "" {
		if cfg.Issuer != "" {
			t = "oidc"
		} else {
			t = "oauth2"
		}
	}
	switch t {
	case "oauth2":
		return newOAuthClient(cfg)
	case "oidc":
		return newOIDCClient(cfg)
	case "github":
		return newGitHubClient(cfg)
	default:
		return nil, fmt.Errorf("unsupported type: %s", t)
	}
}

// oauthClient is the base OAuth 2.0 implementation. The OIDC and GitHub
// flavors embed it and override fetchUserInfo.
type oauthClient struct {
	cfg        Config
	httpClient *http.Client
}

func newOAuthClient(cfg Config) (*oauthClient, error) {
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("oauth: client_id is required")
	}
	if cfg.AuthorizationURL == "" {
		return nil, fmt.Errorf("oauth: authorization_url is required")
	}
	if cfg.TokenURL == "" {
		return nil, fmt.Errorf("oauth: token_url is required")
	}
	if cfg.RedirectURI == "" {
		return nil, fmt.Errorf("oauth: redirect_uri is required")
	}
	return &oauthClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: HTTPRequestTimeout},
	}, nil
}

// AuthorizationURL builds the URL the browser should be redirected to.
// Mirrors OAuthClient.get_authorization_url.
func (c *oauthClient) AuthorizationURL(state string) (string, error) {
	params := url.Values{}
	params.Set("client_id", c.cfg.ClientID)
	params.Set("redirect_uri", c.cfg.RedirectURI)
	params.Set("response_type", "code")
	if c.cfg.Scope != "" {
		params.Set("scope", c.cfg.Scope)
	}
	if state != "" {
		params.Set("state", state)
	}
	sep := "?"
	if strings.Contains(c.cfg.AuthorizationURL, "?") {
		sep = "&"
	}
	return c.cfg.AuthorizationURL + sep + params.Encode(), nil
}

// ExchangeCodeForToken exchanges an authorization code for an access token.
// Mirrors OAuthClient.exchange_code_for_token.
func (c *oauthClient) ExchangeCodeForToken(ctx context.Context, code string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", c.cfg.ClientID)
	form.Set("client_secret", c.cfg.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", c.cfg.RedirectURI)
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to exchange authorization code for token: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange authorization code for token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to exchange authorization code for token: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("failed to exchange authorization code for token: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	token := &TokenResponse{}
	if jerr := json.Unmarshal(body, token); jerr != nil {
		// Some providers (notably GitHub when Accept is not set) return
		// application/x-www-form-urlencoded here instead of JSON.
		if values, perr := url.ParseQuery(string(body)); perr == nil {
			token.AccessToken = values.Get("access_token")
			token.TokenType = values.Get("token_type")
			token.IDToken = values.Get("id_token")
			token.Scope = values.Get("scope")
		} else {
			return nil, fmt.Errorf("failed to exchange authorization code for token: parse response: %w", jerr)
		}
	}
	if token.AccessToken == "" {
		return nil, fmt.Errorf("failed to exchange authorization code for token: empty access_token")
	}
	return token, nil
}

// FetchUserInfo fetches user information using the access token.
// Mirrors OAuthClient.fetch_user_info / normalize_user_info.
func (c *oauthClient) FetchUserInfo(ctx context.Context, accessToken, idToken string) (*UserInfo, error) {
	if c.cfg.UserinfoURL == "" {
		return nil, fmt.Errorf("failed to fetch user info: userinfo_url is required")
	}
	raw, err := c.fetchUserinfoRaw(ctx, c.cfg.UserinfoURL, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user info: %w", err)
	}
	return normalizeUserInfo(raw), nil
}

func (c *oauthClient) fetchUserinfoRaw(ctx context.Context, endpoint, accessToken string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out map[string]interface{}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse userinfo response: %w", err)
	}
	return out, nil
}

// normalizeUserInfo mirrors the Python normalize_user_info defaults: username
// falls back to the email local part, nickname falls back to username, and
// avatar_url falls back to OIDC's "picture" claim.
func normalizeUserInfo(raw map[string]interface{}) *UserInfo {
	ui := &UserInfo{}
	if v, ok := raw["email"].(string); ok {
		ui.Email = v
	}
	if v, ok := raw["username"].(string); ok && v != "" {
		ui.Username = v
	} else if ui.Email != "" {
		if at := strings.IndexByte(ui.Email, '@'); at >= 0 {
			ui.Username = ui.Email[:at]
		} else {
			ui.Username = ui.Email
		}
	}
	if v, ok := raw["nickname"].(string); ok && v != "" {
		ui.Nickname = v
	} else {
		ui.Nickname = ui.Username
	}
	if v, ok := raw["avatar_url"].(string); ok && v != "" {
		ui.AvatarURL = v
	} else if v, ok := raw["picture"].(string); ok {
		ui.AvatarURL = v
	}
	return ui
}
