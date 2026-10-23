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

package config

import "github.com/spf13/viper"

// OAuthProvider describes a single configured SSO channel under the top-level
// `oauth:` block of service_conf.yaml. Each key under `oauth:` becomes a
// channel (e.g. `oidc`), and its value is parsed into one of these.
//
// For OIDC providers the authorization/token/userinfo/jwks endpoints are NOT
// taken from config; they are discovered at runtime from
// `{issuer}/.well-known/openid-configuration` (see service/oauth_ee.go). Only
// `issuer`, `client_id`, `client_secret`, `scope` and `redirect_uri` are read
// here. Plain OAuth2 providers may instead set the explicit *_url fields.
type OAuthProvider struct {
	Channel      string
	Type         string
	DisplayName  string
	Icon         string
	ClientID     string
	ClientSecret string

	// OIDC: single source of truth, endpoints discovered from this.
	Issuer string

	// Plain OAuth2: explicit endpoints.
	AuthorizationURL string
	TokenURL         string
	UserinfoURL      string

	Scope       string
	RedirectURI string

	// TrustEmailVerified marks this IdP as institutionally trusted for the email
	// address it returns, so a login is accepted even when the provider omits the
	// `email_verified` claim. Leave false for public/multi-tenant IdPs so logins
	// require email_verified=true (account-takeover guard). Set true only for a
	// controlled IdP you operate.
	TrustEmailVerified bool

	// Claim mappings (optional). Each is an ordered precedence list of claim
	// names; the first non-empty match wins. Empty falls back to the OIDC
	// defaults below. This keeps provider-specific claim names (e.g. an
	// institution's own id claim) in config instead of hardcoded in the server.
	//   UsernameClaims default: ["username", "preferred_username", "sub"]
	//   EmailClaims    default: ["email"]
	//   NicknameClaims default: ["nickname", "name"]
	//   AvatarClaims   default: ["avatar_url", "picture"]
	UsernameClaims []string
	EmailClaims    []string
	NicknameClaims []string
	AvatarClaims   []string
}

// OAuthConfig holds every configured SSO channel, keyed by channel name.
type OAuthConfig struct {
	Providers map[string]*OAuthProvider
}

// ParseOAuthConfig reads the optional top-level `oauth:` block. Each sub-key is
// treated as a login channel. A provider is only registered when it carries a
// client_id, so a fully-commented block yields zero channels (password-only
// login). The whole block is optional and a nil/missing block is not an error.
func (c *Config) ParseOAuthConfig(v *viper.Viper) error {
	c.oAuth = OAuthConfig{Providers: map[string]*OAuthProvider{}}

	if !v.IsSet("oauth") {
		return nil
	}
	sub := v.Sub("oauth")
	if sub == nil {
		return nil
	}

	for channel := range sub.AllSettings() {
		ps := sub.Sub(channel)
		if ps == nil {
			continue
		}

		p := &OAuthProvider{
			Channel:     channel,
			Type:        channel,
			DisplayName: channel,
		}
		if ps.IsSet("type") {
			p.Type = ps.GetString("type")
		}
		if ps.IsSet("display_name") {
			p.DisplayName = ps.GetString("display_name")
		}
		if ps.IsSet("icon") {
			p.Icon = ps.GetString("icon")
		}
		if ps.IsSet("client_id") {
			p.ClientID = ps.GetString("client_id")
		}
		if ps.IsSet("client_secret") {
			p.ClientSecret = ps.GetString("client_secret")
		}
		if ps.IsSet("issuer") {
			p.Issuer = ps.GetString("issuer")
		}
		if ps.IsSet("authorization_url") {
			p.AuthorizationURL = ps.GetString("authorization_url")
		}
		if ps.IsSet("token_url") {
			p.TokenURL = ps.GetString("token_url")
		}
		if ps.IsSet("userinfo_url") {
			p.UserinfoURL = ps.GetString("userinfo_url")
		}
		if ps.IsSet("scope") {
			p.Scope = ps.GetString("scope")
		}
		if ps.IsSet("redirect_uri") {
			p.RedirectURI = ps.GetString("redirect_uri")
		}
		if ps.IsSet("trust_email_verified") {
			p.TrustEmailVerified = ps.GetBool("trust_email_verified")
		}
		if ps.IsSet("username_claims") {
			p.UsernameClaims = ps.GetStringSlice("username_claims")
		}
		if ps.IsSet("email_claims") {
			p.EmailClaims = ps.GetStringSlice("email_claims")
		}
		if ps.IsSet("nickname_claims") {
			p.NicknameClaims = ps.GetStringSlice("nickname_claims")
		}
		if ps.IsSet("avatar_claims") {
			p.AvatarClaims = ps.GetStringSlice("avatar_claims")
		}

		// A provider without credentials is a commented-out placeholder.
		if p.ClientID == "" {
			continue
		}
		// Sensible OIDC default so a minimal config still requests the claims
		// the normalization step needs (email + profile).
		if p.Scope == "" {
			p.Scope = "openid email profile"
		}

		c.oAuth.Providers[channel] = p
	}

	return nil
}

// GetOAuthConfig returns the parsed SSO channel configuration.
func (c *Config) GetOAuthConfig() OAuthConfig {
	return c.oAuth
}
