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
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/kvrocks"
	"ragflow/internal/entity"
	"ragflow/internal/server"
	"ragflow/internal/server/config"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

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

// OAuthCallbackResult is returned to the handler after a successful callback
// so it can mint the user-facing auth response (cookie + redirect).
type OAuthCallbackResult struct {
	User      *entity.User
	IsNewUser bool
}

// oauthStateTTL bounds how long an in-flight login may take between the
// redirect to the IdP and the callback.
const oauthStateTTL = 10 * time.Minute

// ---------------------------------------------------------------------------
// OIDC discovery + JWKS caches
// ---------------------------------------------------------------------------

// oidcMetadata is the subset of the OIDC discovery document we consume.
type oidcMetadata struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	UserinfoEndpoint      string   `json:"userinfo_endpoint"`
	JWKSURI               string   `json:"jwks_uri"`
	SigningAlgsSupported  []string `json:"id_token_signing_alg_values_supported"`
}

type cachedMetadata struct {
	meta      *oidcMetadata
	fetchedAt time.Time
}

type cachedJWKS struct {
	set       *jose.JSONWebKeySet
	fetchedAt time.Time
}

var (
	oidcMetaCache = map[string]*cachedMetadata{}
	oidcMetaMu    sync.Mutex

	jwksCache = map[string]*cachedJWKS{}
	jwksMu    sync.Mutex

	// discoveryTTL keeps discovery/JWKS lookups cheap; a login burst for 100+
	// users should not re-fetch the discovery document every time.
	discoveryTTL = time.Hour
)

// Asymmetric signing algorithms safe to accept for OIDC ID tokens. Symmetric
// HMAC (HS*) and "none" are intentionally excluded: when the verification key
// is the asymmetric public key from the provider's JWKS, accepting HS256 lets
// an attacker forge tokens by HMAC-signing with the public key bytes
// (RSA/HMAC algorithm-confusion, CWE-347); "none" disables verification
// entirely. This mirrors api/apps/auth/oidc.py.
var allowedOIDCSigningAlgs = map[string]struct{}{
	"RS256": {}, "RS384": {}, "RS512": {},
	"ES256": {}, "ES384": {}, "ES512": {},
	"PS256": {}, "PS384": {}, "PS512": {},
	"EdDSA": {},
}

// resolveSigningAlgs intersects the provider-advertised algorithms with the
// safe allowlist, falling back to RS256 (the OIDC Core spec default) — never
// to whatever a JWT header claims at verification time.
func resolveSigningAlgs(advertised []string) []string {
	safe := make([]string, 0, len(advertised))
	for _, a := range advertised {
		if _, ok := allowedOIDCSigningAlgs[a]; ok {
			safe = append(safe, a)
		}
	}
	if len(safe) == 0 {
		return []string{"RS256"}
	}
	return safe
}

func fetchOIDCMetadata(ctx context.Context, issuer string) (*oidcMetadata, error) {
	oidcMetaMu.Lock()
	if c, ok := oidcMetaCache[issuer]; ok && time.Since(c.fetchedAt) < discoveryTTL {
		m := c.meta
		oidcMetaMu.Unlock()
		return m, nil
	}
	oidcMetaMu.Unlock()

	metaURL := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	body, err := httpGet(ctx, metaURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch OIDC metadata: %w", err)
	}
	var m oidcMetadata
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("decode OIDC metadata: %w", err)
	}
	if m.Issuer == "" || m.AuthorizationEndpoint == "" || m.TokenEndpoint == "" {
		return nil, fmt.Errorf("incomplete OIDC metadata from %s", metaURL)
	}

	oidcMetaMu.Lock()
	oidcMetaCache[issuer] = &cachedMetadata{meta: &m, fetchedAt: time.Now()}
	oidcMetaMu.Unlock()
	return &m, nil
}

// fetchJWKS returns the provider's JWKS, cached for discoveryTTL. When force is
// true the cache is bypassed UNLESS it was refreshed within the last minute —
// this lets verifyIDToken re-fetch once after a signing-key rotation (a new kid)
// while rate-limiting refreshes so tokens carrying random/unknown kids cannot
// trigger a fetch on every request.
func fetchJWKS(ctx context.Context, uri string, force bool) (*jose.JSONWebKeySet, error) {
	jwksMu.Lock()
	if e, ok := jwksCache[uri]; ok && time.Since(e.fetchedAt) < discoveryTTL &&
		(!force || time.Since(e.fetchedAt) < time.Minute) {
		s := e.set
		jwksMu.Unlock()
		return s, nil
	}
	jwksMu.Unlock()

	body, err := httpGet(ctx, uri, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch JWKS: %w", err)
	}
	var set jose.JSONWebKeySet
	if err := json.Unmarshal(body, &set); err != nil {
		return nil, fmt.Errorf("decode JWKS: %w", err)
	}

	jwksMu.Lock()
	jwksCache[uri] = &cachedJWKS{set: &set, fetchedAt: time.Now()}
	jwksMu.Unlock()
	return &set, nil
}

// ---------------------------------------------------------------------------
// provider resolution
// ---------------------------------------------------------------------------

// resolvedProvider merges static config with OIDC-discovered endpoints.
type resolvedProvider struct {
	provider    *config.OAuthProvider
	issuer      string
	authURL     string
	tokenURL    string
	userinfoURL string
	jwksURI     string
	signingAlgs []string
}

func (s *UserService) resolveProvider(ctx context.Context, channel string) (*resolvedProvider, common.ErrorCode, error) {
	p, ok := server.GetConfig().GetOAuthConfig().Providers[channel]
	if !ok {
		return nil, common.CodeNotFound, fmt.Errorf("unknown login channel: %s", channel)
	}
	r := &resolvedProvider{
		provider:    p,
		issuer:      p.Issuer,
		authURL:     p.AuthorizationURL,
		tokenURL:    p.TokenURL,
		userinfoURL: p.UserinfoURL,
	}
	if strings.EqualFold(p.Type, "oidc") || p.Issuer != "" {
		if p.Issuer == "" {
			return nil, common.CodeExceptionError, fmt.Errorf("oidc channel %q missing issuer", channel)
		}
		m, err := fetchOIDCMetadata(ctx, p.Issuer)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		// The ID token's `iss` must equal the issuer the provider DECLARES in its
		// discovery document, not the configured discovery base — they differ
		// routinely by a trailing slash (e.g. a provider advertises
		// "https://idp.example.com/oidc/" while the config carries it without
		// the slash). Verify against the discovered value (OIDC Core §3.1.3.7).
		if m.Issuer != "" {
			r.issuer = m.Issuer
		}
		r.authURL = m.AuthorizationEndpoint
		r.tokenURL = m.TokenEndpoint
		r.userinfoURL = m.UserinfoEndpoint
		r.jwksURI = m.JWKSURI
		r.signingAlgs = resolveSigningAlgs(m.SigningAlgsSupported)
	}
	if r.authURL == "" || r.tokenURL == "" {
		return nil, common.CodeExceptionError, fmt.Errorf("channel %q has no usable endpoints", channel)
	}
	return r, common.CodeSuccess, nil
}

// ---------------------------------------------------------------------------
// login initiate
// ---------------------------------------------------------------------------

func (s *UserService) OAuthLoginInitiate(channel string, redis *kvrocks.Client) (*OAuthLoginInit, common.ErrorCode, error) {
	ctx := context.Background()
	r, code, err := s.resolveProvider(ctx, channel)
	if err != nil {
		return nil, code, err
	}

	state := common.GenerateUUID()
	if redis != nil {
		// consume-once CSRF/replay guard, complementary to the state cookie.
		redis.Set(ctx, oauthStateKey(channel, state), channel, oauthStateTTL)
	}

	params := url.Values{}
	params.Set("client_id", r.provider.ClientID)
	params.Set("redirect_uri", r.provider.RedirectURI)
	params.Set("response_type", "code")
	if r.provider.Scope != "" {
		params.Set("scope", r.provider.Scope)
	}
	params.Set("state", state)

	sep := "?"
	if strings.Contains(r.authURL, "?") {
		sep = "&"
	}
	return &OAuthLoginInit{
		State:        state,
		AuthURL:      r.authURL + sep + params.Encode(),
		Channel:      channel,
		CookieMaxAge: oauthStateTTL,
	}, common.CodeSuccess, nil
}

// ---------------------------------------------------------------------------
// callback
// ---------------------------------------------------------------------------

func (s *UserService) OAuthCallback(ctx context.Context, channel, code, callbackState, expectedState string, redis *kvrocks.Client) (*OAuthCallbackResult, common.ErrorCode, error) {
	if code == "" {
		return nil, common.CodeArgumentError, fmt.Errorf("missing authorization code")
	}

	// CSRF: double-submit cookie. The state echoed by the IdP must match the
	// state we set as an HttpOnly cookie on this browser at initiate time.
	if callbackState == "" || expectedState == "" ||
		subtle.ConstantTimeCompare([]byte(callbackState), []byte(expectedState)) != 1 {
		return nil, common.CodeAuthenticationError, fmt.Errorf("invalid oauth state")
	}
	// Consume-once in redis defends against replay even if the cookie leaks.
	if redis != nil {
		key := oauthStateKey(channel, callbackState)
		if stored, _ := redis.Get(ctx, key); stored != channel {
			return nil, common.CodeAuthenticationError, fmt.Errorf("oauth state expired or already used")
		}
		redis.Delete(ctx, key)
	}

	r, rcode, err := s.resolveProvider(ctx, channel)
	if err != nil {
		return nil, rcode, err
	}

	tok, err := exchangeCode(ctx, r, code)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	info, err := fetchUserInfo(ctx, r, tok)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if info.Email == "" {
		return nil, common.CodeDataError, fmt.Errorf("identity provider did not return an email address")
	}

	user, isNew, ucode, err := s.upsertOAuthUser(ctx, channel, info)
	if err != nil {
		return nil, ucode, err
	}
	return &OAuthCallbackResult{User: user, IsNewUser: isNew}, common.CodeSuccess, nil
}

// ---------------------------------------------------------------------------
// token exchange + userinfo
// ---------------------------------------------------------------------------

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
}

func exchangeCode(ctx context.Context, r *resolvedProvider, code string) (*tokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", r.provider.ClientID)
	form.Set("client_secret", r.provider.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", r.provider.RedirectURI)
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := common.GetSSRFHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchange authorization code: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("token endpoint did not return an access_token")
	}
	return &tr, nil
}

// normalizedUser is the OIDC-normalized identity (see normalizeUser).
type normalizedUser struct {
	Email         string
	EmailVerified bool
	Subject       string
	Username      string
	Nickname      string
	Avatar        string
}

// fetchUserInfo gathers claims from the (optionally verified) ID token and the
// userinfo endpoint, with userinfo taking precedence, then normalizes them.
func fetchUserInfo(ctx context.Context, r *resolvedProvider, tok *tokenResponse) (*normalizedUser, error) {
	claims := map[string]interface{}{}

	// ID token (OIDC): signature-verified against the provider's JWKS with a
	// pinned algorithm allowlist. Failure is fatal — we never trust an
	// unverified ID token.
	var idTokenSub string
	if tok.IDToken != "" && r.jwksURI != "" {
		idClaims, err := verifyIDToken(ctx, r, tok.IDToken)
		if err != nil {
			return nil, fmt.Errorf("verify id token: %w", err)
		}
		idTokenSub, _ = idClaims["sub"].(string)
		for k, v := range idClaims {
			claims[k] = v
		}
	}

	// Userinfo endpoint (authoritative back-channel over TLS).
	if r.userinfoURL != "" {
		body, err := httpGet(ctx, r.userinfoURL, map[string]string{
			"Authorization": "Bearer " + tok.AccessToken,
		})
		if err != nil {
			return nil, fmt.Errorf("fetch userinfo: %w", err)
		}
		var ui map[string]interface{}
		if err := json.Unmarshal(body, &ui); err != nil {
			return nil, fmt.Errorf("decode userinfo: %w", err)
		}
		// OIDC Core §5.3.2: the userinfo `sub` MUST match the ID token `sub`.
		// Reject a mismatch rather than let userinfo claims (which overwrite the
		// ID-token claims below) describe a different principal.
		if uiSub, _ := ui["sub"].(string); idTokenSub != "" && uiSub != "" && uiSub != idTokenSub {
			return nil, fmt.Errorf("userinfo sub does not match id token sub")
		}
		for k, v := range ui {
			claims[k] = v
		}
	}

	return normalizeUser(claims, r.provider), nil
}

func verifyIDToken(ctx context.Context, r *resolvedProvider, idToken string) (jwt.MapClaims, error) {
	set, err := fetchJWKS(ctx, r.jwksURI, false)
	if err != nil {
		return nil, err
	}
	keyfunc := func(t *jwt.Token) (interface{}, error) {
		kid, _ := t.Header["kid"].(string)
		keys := set.Key(kid)
		if len(keys) == 0 {
			// The kid may be new because the IdP rotated its signing key after we
			// cached the JWKS. Force one rate-limited refresh and retry before
			// failing, so logins survive key rotation instead of breaking for up
			// to discoveryTTL.
			if fresh, ferr := fetchJWKS(ctx, r.jwksURI, true); ferr == nil {
				keys = fresh.Key(kid)
			}
		}
		if len(keys) == 0 {
			return nil, fmt.Errorf("no signing key for kid %q", kid)
		}
		return keys[0].Key, nil
	}

	claims := jwt.MapClaims{}
	_, err = jwt.ParseWithClaims(idToken, claims, keyfunc,
		jwt.WithValidMethods(r.signingAlgs),
		jwt.WithAudience(r.provider.ClientID),
		jwt.WithIssuer(r.issuer),
		jwt.WithExpirationRequired(), // OIDC Core §2 makes exp mandatory.
		jwt.WithIssuedAt(),           // reject tokens whose iat is in the future.
	)
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// normalizeUser maps OIDC/OAuth2 claims to a user identity using the provider's
// configured claim precedence (p.UsernameClaims etc.), falling back to standard
// OIDC claims. Keeping the claim names in config (service_conf) lets a
// deployment point at a provider-specific id claim without any code change.
//
//   - email    = first non-empty of EmailClaims    (default: email)
//   - username = first non-empty of UsernameClaims (default: username,
//     preferred_username, sub), else the email local-part
//   - nickname = first non-empty of NicknameClaims (default: nickname, name),
//     else username
//   - avatar   = first non-empty of AvatarClaims   (default: avatar_url, picture)
func normalizeUser(claims map[string]interface{}, p *config.OAuthProvider) *normalizedUser {
	get := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := claims[k]; ok {
				if sv, ok := v.(string); ok && sv != "" {
					return sv
				}
			}
		}
		return ""
	}
	orDefault := func(configured []string, def ...string) []string {
		if len(configured) > 0 {
			return configured
		}
		return def
	}

	var usernameClaims, emailClaims, nicknameClaims, avatarClaims []string
	if p != nil {
		usernameClaims, emailClaims = p.UsernameClaims, p.EmailClaims
		nicknameClaims, avatarClaims = p.NicknameClaims, p.AvatarClaims
	}
	usernameClaims = orDefault(usernameClaims, "username", "preferred_username", "sub")
	emailClaims = orDefault(emailClaims, "email")
	nicknameClaims = orDefault(nicknameClaims, "nickname", "name")
	avatarClaims = orDefault(avatarClaims, "avatar_url", "picture")

	email := get(emailClaims...)
	username := get(usernameClaims...)
	if username == "" && email != "" {
		if i := strings.Index(email, "@"); i > 0 {
			username = email[:i]
		} else {
			username = email
		}
	}
	nickname := get(nicknameClaims...)
	if nickname == "" {
		nickname = username
	}
	avatar := get(avatarClaims...)

	return &normalizedUser{
		Email:         email,
		EmailVerified: getBool(claims, "email_verified"),
		Subject:       get("sub"),
		Username:      username,
		Nickname:      nickname,
		Avatar:        avatar,
	}
}

// getBool reads a claim that may be encoded as a JSON bool or as the strings
// "true"/"false" (some IdPs serialize email_verified as a string).
func getBool(claims map[string]interface{}, key string) bool {
	switch v := claims[key].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// user provisioning
// ---------------------------------------------------------------------------

// upsertOAuthUser rotates the access token for an existing user (keyed by
// email) or provisions a brand-new user+tenant for a first-time SSO login.
func (s *UserService) upsertOAuthUser(ctx context.Context, channel string, info *normalizedUser) (*entity.User, bool, common.ErrorCode, error) {
	// Account-takeover guard: an SSO login is matched to an account by email, so
	// an unverified email must not be trusted. Some IdPs let a user set an
	// arbitrary email without verifying it; honoring that would let an attacker
	// take over any existing account (incl. a superuser) by claiming its email.
	// Require email_verified=true, unless the provider is explicitly marked
	// trusted-for-email (trust_email_verified) — e.g. Example SSO, an
	// institutionally-controlled IdP that may omit the claim.
	if !info.EmailVerified {
		prov := server.GetConfig().GetOAuthConfig().Providers[channel]
		if prov == nil || !prov.TrustEmailVerified {
			return nil, false, common.CodeForbidden, fmt.Errorf("identity provider did not return a verified email")
		}
	}

	existing, err := s.userDAO.GetByEmail(ctx, dao.DB, info.Email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, common.CodeServerError, fmt.Errorf("lookup user by email: %w", err)
	}
	if existing != nil {
		if existing.Status == nil || *existing.Status != "1" {
			return nil, false, common.CodeForbidden, fmt.Errorf("user is disabled")
		}
		token := common.GenerateToken()
		existing.AccessToken = &token
		now := time.Now().Truncate(time.Second)
		existing.LastLoginTime = &now
		if err := s.userDAO.Update(ctx, dao.DB, existing); err != nil {
			return nil, false, common.CodeServerError, fmt.Errorf("update user: %w", err)
		}
		return existing, false, common.CodeSuccess, nil
	}

	// OIDC provisioning is intentionally NOT gated by EnableRegister():
	// EnableRegister() (REGISTER_ENABLED) controls *password* self-signup only.
	// An identity vouched for by the trusted IdP (Example SSO) is always
	// provisioned on first login. This matches the v0.27.2 Python behavior where
	// the OAuth callback (api/apps/restful_apis/user_api.py) calls user_register
	// directly, bypassing the REGISTER_ENABLED check that guards only the
	// password /register endpoint — so new users keep onboarding via SSO
	// even with REGISTER_ENABLED=0.
	cfg := server.GetConfig()
	user, err := s.provisionOAuthUser(channel, info, cfg)
	if err != nil {
		return nil, false, common.CodeServerError, err
	}
	return user, true, common.CodeSuccess, nil
}

// provisionOAuthUser mirrors UserService.Register (user + tenant + user_tenant +
// root folder in one transaction) but sets no password and records the SSO
// login channel.
func (s *UserService) provisionOAuthUser(channel string, info *normalizedUser, cfg *config.Config) (*entity.User, error) {
	userID := common.GenerateToken()
	accessToken := common.GenerateToken()
	status := "1"
	loginChannel := channel
	isSuperuser := false
	language := defaultUserLanguage()
	colorSchema := "Bright"
	timezone := "UTC+8\tAsia/Shanghai"
	now := time.Now().Truncate(time.Second)

	nickname := info.Nickname
	if nickname == "" {
		nickname = info.Username
	}
	var avatar *string
	if info.Avatar != "" {
		avatar = &info.Avatar
	}

	user := &entity.User{
		ID:              userID,
		AccessToken:     &accessToken,
		Email:           info.Email,
		Nickname:        nickname,
		Password:        nil,
		Avatar:          avatar,
		Status:          &status,
		Language:        &language,
		ColorSchema:     &colorSchema,
		Timezone:        &timezone,
		IsActive:        "1",
		IsAuthenticated: "1",
		IsAnonymous:     "0",
		LastLoginTime:   &now,
		LoginChannel:    &loginChannel,
		IsSuperuser:     &isSuperuser,
	}

	tenantName := nickname + "'s Kingdom"
	ttsID := cfg.GetDefaultTTSModel().Name
	ocrID := cfg.GetDefaultOCRModel().Name
	tenant := &entity.Tenant{
		ID:        userID,
		Name:      &tenantName,
		LLMID:     cfg.GetDefaultChatModel().Name,
		EmbdID:    cfg.GetDefaultEmbeddingModel().Name,
		ASRID:     cfg.GetDefaultASRModel().Name,
		Img2TxtID: cfg.GetDefaultVisionModel().Name,
		RerankID:  cfg.GetDefaultRerankModel().Name,
		TTSID:     &ttsID,
		OCRID:     &ocrID,
		ParserIDs: "naive:General,qa:Q&A,manual:Manual,table:Table,paper:Research Paper,book:Book,laws:Laws,presentation:Presentation,picture:Picture,one:One,audio:Audio,email:Email,tag:Tag",
		Status:    &status,
	}
	userTenant := &entity.UserTenant{
		ID:        common.GenerateToken(),
		UserID:    userID,
		TenantID:  userID,
		Role:      "owner",
		InvitedBy: userID,
		Status:    &status,
	}
	fileID := common.GenerateToken()
	emptyLoc := ""
	rootFile := &entity.File{
		ID:        fileID,
		ParentID:  fileID,
		TenantID:  userID,
		CreatedBy: userID,
		Name:      "/",
		Type:      "folder",
		Location:  &emptyLoc,
		Size:      0,
	}

	db := dao.GetDB()
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}
		if err := tx.Create(tenant).Error; err != nil {
			return fmt.Errorf("failed to create tenant: %w", err)
		}
		if err := tx.Create(userTenant).Error; err != nil {
			return fmt.Errorf("failed to create user tenant relation: %w", err)
		}
		if err := tx.Create(rootFile).Error; err != nil {
			return fmt.Errorf("failed to create root folder: %w", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("fail to create transaction: %w", err)
	}
	return user, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func oauthStateKey(channel, state string) string {
	return fmt.Sprintf("oauth_state:%s:%s", channel, state)
}

// httpGet performs a GET with the SSRF-guarded client and returns the body,
// capped at 1 MiB.
func httpGet(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := common.GetSSRFHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %d", rawURL, resp.StatusCode)
	}
	return body, nil
}
