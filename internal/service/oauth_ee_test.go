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
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"

	"ragflow/internal/common"
	"ragflow/internal/server/config"
)

// oidcAllowLoopback flips the in-memory, test-only SSRF bypass so the OIDC HTTP
// helpers (fetchOIDCMetadata/fetchJWKS/exchangeCode/fetchUserInfo, all of which
// go through the strict SSRF client) can reach the httptest mock IdP bound to
// 127.0.0.1. Mirrors internal/entity/models' withSSRFBypass.
func oidcAllowLoopback(t *testing.T) {
	t.Helper()
	// Reset the package-level discovery/JWKS caches: they are keyed by URL with a
	// 1h TTL, and an httptest server can reuse a just-closed port, so a stale key
	// set (same testKID, different RSA key) would otherwise leak between tests and
	// make verification flaky.
	oidcMetaMu.Lock()
	oidcMetaCache = map[string]*cachedMetadata{}
	oidcMetaMu.Unlock()
	jwksMu.Lock()
	jwksCache = map[string]*cachedJWKS{}
	jwksMu.Unlock()

	prev := common.AllowAnyHostForTest
	common.AllowAnyHostForTest = true
	t.Cleanup(func() { common.AllowAnyHostForTest = prev })
}

const (
	testClientID     = "duku-client-123"
	testClientSecret = "duku-secret-abc"
	testKID          = "duku-test-key-1"
)

// mockIdP is a minimal but spec-faithful OpenID Connect provider for tests:
// discovery + JWKS + token + userinfo, with an RSA signing key whose public
// half is published via JWKS. The signing key and claims are injectable so
// individual tests can forge adversarial ID tokens.
type mockIdP struct {
	ts         *httptest.Server
	baseURL    string
	issuer     string          // advertised issuer; defaults to baseURL (see issuerValue)
	signKey    *rsa.PrivateKey // key the JWKS advertises
	idTokClaim jwt.MapClaims   // claims minted into the id_token by /token
	userinfo   map[string]any  // body returned by /userinfo
}

// issuerValue is the issuer the IdP advertises in discovery AND stamps into the
// id_token's `iss`. Defaults to baseURL; a test may set a different value (e.g.
// baseURL+"/") to reproduce the real-world case where the discovered issuer
// differs from the configured discovery URL.
func (m *mockIdP) issuerValue() string {
	if m.issuer != "" {
		return m.issuer
	}
	return m.baseURL
}

func newMockIdP(t *testing.T) *mockIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	m := &mockIdP{signKey: key}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                                m.issuerValue(),
			"authorization_endpoint":                m.baseURL + "/authorize",
			"token_endpoint":                        m.baseURL + "/token",
			"userinfo_endpoint":                     m.baseURL + "/userinfo",
			"jwks_uri":                              m.baseURL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256", "HS256", "none"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		set := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key:       &m.signKey.PublicKey,
			KeyID:     testKID,
			Algorithm: "RS256",
			Use:       "sig",
		}}}
		writeJSON(w, set)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		idToken := m.mintIDToken(t, m.signKey, "RS256", m.idTokClaim)
		writeJSON(w, map[string]any{
			"access_token": "mock-access-token",
			"id_token":     idToken,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer mock-access-token" {
			http.Error(w, "missing bearer", http.StatusUnauthorized)
			return
		}
		writeJSON(w, m.userinfo)
	})

	m.ts = httptest.NewServer(mux)
	m.baseURL = m.ts.URL
	t.Cleanup(m.ts.Close)
	return m
}

// mintIDToken signs an id_token with the given key/alg. A separate key lets a
// test forge a token the JWKS will not validate; alg="none" produces an
// unsigned token.
func (m *mockIdP) mintIDToken(t *testing.T, key *rsa.PrivateKey, alg string, claims jwt.MapClaims) string {
	t.Helper()
	full := jwt.MapClaims{
		"iss": m.issuerValue(),
		"aud": testClientID,
		"sub": "default-sub",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(10 * time.Minute).Unix(),
	}
	for k, v := range claims {
		full[k] = v
	}
	if alg == "none" {
		tok := jwt.NewWithClaims(jwt.SigningMethodNone, full)
		tok.Header["kid"] = testKID
		s, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
		if err != nil {
			t.Fatalf("sign none: %v", err)
		}
		return s
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, full)
	tok.Header["kid"] = testKID
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign RS256: %v", err)
	}
	return s
}

// resolved builds the resolvedProvider the service would assemble from discovery
// — without touching the global server config.
func (m *mockIdP) resolved(t *testing.T) *resolvedProvider {
	t.Helper()
	meta, err := fetchOIDCMetadata(context.Background(), m.baseURL)
	if err != nil {
		t.Fatalf("fetchOIDCMetadata: %v", err)
	}
	return &resolvedProvider{
		provider: &config.OAuthProvider{
			Channel:      "oidc",
			Type:         "oidc",
			ClientID:     testClientID,
			ClientSecret: testClientSecret,
			Issuer:       m.baseURL,
			RedirectURI:  "https://ragflow.example.com/api/v1/auth/oauth/oidc/callback",
		},
		issuer:      meta.Issuer,
		authURL:     meta.AuthorizationEndpoint,
		tokenURL:    meta.TokenEndpoint,
		userinfoURL: meta.UserinfoEndpoint,
		jwksURI:     meta.JWKSURI,
		signingAlgs: resolveSigningAlgs(meta.SigningAlgsSupported),
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// --- discovery -------------------------------------------------------------

func TestOIDCDiscoveryAndSigningAlgs(t *testing.T) {
	oidcAllowLoopback(t)
	m := newMockIdP(t)

	meta, err := fetchOIDCMetadata(context.Background(), m.baseURL)
	if err != nil {
		t.Fatalf("fetchOIDCMetadata: %v", err)
	}
	if meta.Issuer != m.baseURL || meta.TokenEndpoint != m.baseURL+"/token" || meta.JWKSURI != m.baseURL+"/jwks" {
		t.Fatalf("unexpected metadata: %+v", meta)
	}

	// The HS256/"none" algorithms the IdP advertises must be filtered out of the
	// accepted set, leaving only the asymmetric RS256 (RSA/HMAC confusion guard).
	algs := resolveSigningAlgs(meta.SigningAlgsSupported)
	if len(algs) != 1 || algs[0] != "RS256" {
		t.Fatalf("expected [RS256], got %v", algs)
	}
}

// --- full happy-path callback pipeline -------------------------------------

func TestOIDCCallbackPipeline(t *testing.T) {
	oidcAllowLoopback(t)
	m := newMockIdP(t)
	// Example SSO shape: no "username" claim, so preferred_username wins; the
	// avatar arrives only via userinfo's "picture", which must override.
	m.idTokClaim = jwt.MapClaims{
		"sub":                "user-0001",
		"preferred_username": "jd456",
		"name":               "Jane Doe",
		"email":              "jd456@example.com",
	}
	m.userinfo = map[string]any{
		"email":          "jd456@example.com",
		"email_verified": true,
		"picture":        "https://idp.example.com/avatars/jd456.png",
	}

	ctx := context.Background()
	r := m.resolved(t)

	tok, err := exchangeCode(ctx, r, "auth-code-xyz")
	if err != nil {
		t.Fatalf("exchangeCode: %v", err)
	}
	if tok.AccessToken != "mock-access-token" || tok.IDToken == "" {
		t.Fatalf("unexpected token response: %+v", tok)
	}

	info, err := fetchUserInfo(ctx, r, tok)
	if err != nil {
		t.Fatalf("fetchUserInfo: %v", err)
	}
	if info.Email != "jd456@example.com" {
		t.Errorf("email = %q", info.Email)
	}
	if info.Username != "jd456" {
		t.Errorf("username = %q, want jd456 (preferred_username)", info.Username)
	}
	if info.Nickname != "Jane Doe" {
		t.Errorf("nickname = %q, want Jane Doe", info.Nickname)
	}
	if info.Avatar != "https://idp.example.com/avatars/jd456.png" {
		t.Errorf("avatar = %q, want userinfo picture", info.Avatar)
	}
	if !info.EmailVerified {
		t.Error("expected EmailVerified=true from userinfo email_verified")
	}
	if info.Subject != "user-0001" {
		t.Errorf("subject = %q, want user-0001", info.Subject)
	}
}

// --- adversarial ID-token verification -------------------------------------

func TestVerifyIDTokenRejections(t *testing.T) {
	oidcAllowLoopback(t)

	t.Run("valid token passes", func(t *testing.T) {
		m := newMockIdP(t)
		r := m.resolved(t)
		idt := m.mintIDToken(t, m.signKey, "RS256", jwt.MapClaims{"email": "ok@example.com"})
		claims, err := verifyIDToken(context.Background(), r, idt)
		if err != nil {
			t.Fatalf("expected valid token, got error: %v", err)
		}
		if claims["email"] != "ok@example.com" {
			t.Fatalf("claims not returned: %v", claims)
		}
	})

	t.Run("wrong audience rejected", func(t *testing.T) {
		m := newMockIdP(t)
		r := m.resolved(t)
		idt := m.mintIDToken(t, m.signKey, "RS256", jwt.MapClaims{"aud": "some-other-client"})
		if _, err := verifyIDToken(context.Background(), r, idt); err == nil {
			t.Fatal("expected audience mismatch to be rejected")
		}
	})

	t.Run("wrong issuer rejected", func(t *testing.T) {
		m := newMockIdP(t)
		r := m.resolved(t)
		idt := m.mintIDToken(t, m.signKey, "RS256", jwt.MapClaims{"iss": "https://evil.example.com"})
		if _, err := verifyIDToken(context.Background(), r, idt); err == nil {
			t.Fatal("expected issuer mismatch to be rejected")
		}
	})

	t.Run("alg none rejected", func(t *testing.T) {
		m := newMockIdP(t)
		r := m.resolved(t)
		idt := m.mintIDToken(t, m.signKey, "none", jwt.MapClaims{})
		if _, err := verifyIDToken(context.Background(), r, idt); err == nil {
			t.Fatal("expected alg=none to be rejected")
		}
	})

	t.Run("signature by unknown key rejected", func(t *testing.T) {
		m := newMockIdP(t)
		r := m.resolved(t)
		attacker, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("gen attacker key: %v", err)
		}
		// Same kid, different key: JWKS lookup finds the published key, signature
		// check fails.
		idt := m.mintIDToken(t, attacker, "RS256", jwt.MapClaims{})
		if _, err := verifyIDToken(context.Background(), r, idt); err == nil {
			t.Fatal("expected forged-signature token to be rejected")
		}
	})

	t.Run("expired token rejected", func(t *testing.T) {
		m := newMockIdP(t)
		r := m.resolved(t)
		idt := m.mintIDToken(t, m.signKey, "RS256", jwt.MapClaims{
			"iat": time.Now().Add(-2 * time.Hour).Unix(),
			"exp": time.Now().Add(-1 * time.Hour).Unix(),
		})
		if _, err := verifyIDToken(context.Background(), r, idt); err == nil {
			t.Fatal("expected expired token to be rejected")
		}
	})
}

// TestVerifyIDTokenDiscoveredIssuerTrailingSlash reproduces a common real-world case: the
// provider advertises an issuer WITH a trailing slash ("…/oidc/") while the
// configured discovery URL has none ("…/oidc"), and the ID token's `iss` carries
// the slash. Verification must use the DISCOVERED issuer, not the config string,
// or real logins fail with "token has invalid issuer".
func TestVerifyIDTokenDiscoveredIssuerTrailingSlash(t *testing.T) {
	oidcAllowLoopback(t)
	m := newMockIdP(t)
	m.issuer = m.baseURL + "/" // discovery + id_token iss carry the trailing slash

	r := m.resolved(t)
	if r.issuer == r.provider.Issuer {
		t.Fatalf("precondition: discovered issuer %q should differ from config %q", r.issuer, r.provider.Issuer)
	}
	if r.issuer != m.baseURL+"/" {
		t.Fatalf("resolved issuer = %q, want %q", r.issuer, m.baseURL+"/")
	}
	idt := m.mintIDToken(t, m.signKey, "RS256", jwt.MapClaims{"email": "slash@example.com"})
	if _, err := verifyIDToken(context.Background(), r, idt); err != nil {
		t.Fatalf("expected trailing-slash discovered issuer to verify, got: %v", err)
	}
}

// --- claim normalization precedence ----------------------------------------
func TestNormalizeUserClaims(t *testing.T) {
	cases := []struct {
		name                                      string
		claims                                    map[string]any
		provider                                  *config.OAuthProvider
		wantUser, wantNick, wantEmail, wantAvatar string
	}{
		{
			name:     "username claim wins over all (default mapping)",
			claims:   map[string]any{"username": "u1", "preferred_username": "p1", "sub": "s1"},
			wantUser: "u1",
			wantNick: "u1",
		},
		{
			name:     "preferred_username over sub (default mapping)",
			claims:   map[string]any{"preferred_username": "p1", "sub": "s1"},
			wantUser: "p1",
			wantNick: "p1",
		},
		{
			name:     "sub as last id fallback (default mapping)",
			claims:   map[string]any{"sub": "s1"},
			wantUser: "s1",
			wantNick: "s1",
		},
		{
			name:      "email prefix when no id claim",
			claims:    map[string]any{"email": "alice@example.com"},
			wantUser:  "alice",
			wantNick:  "alice",
			wantEmail: "alice@example.com",
		},
		{
			name:     "nickname prefers nickname then name",
			claims:   map[string]any{"sub": "s1", "name": "Full Name", "nickname": "Nick"},
			wantUser: "s1",
			wantNick: "Nick",
		},
		{
			name:       "avatar_url over picture",
			claims:     map[string]any{"sub": "s1", "avatar_url": "A", "picture": "P"},
			wantUser:   "s1",
			wantNick:   "s1",
			wantAvatar: "A",
		},
		{
			// Configurable mapping: a deployment can point username at a
			// provider-specific id claim, taking precedence over the defaults.
			name:     "configured username_claims uses a provider-specific claim",
			claims:   map[string]any{"org_uid": "x500", "preferred_username": "p1", "sub": "s1"},
			provider: &config.OAuthProvider{UsernameClaims: []string{"org_uid", "preferred_username", "sub"}},
			wantUser: "x500",
			wantNick: "x500",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeUser(tc.claims, tc.provider)
			if got.Username != tc.wantUser {
				t.Errorf("username = %q, want %q", got.Username, tc.wantUser)
			}
			if got.Nickname != tc.wantNick {
				t.Errorf("nickname = %q, want %q", got.Nickname, tc.wantNick)
			}
			if tc.wantEmail != "" && got.Email != tc.wantEmail {
				t.Errorf("email = %q, want %q", got.Email, tc.wantEmail)
			}
			if tc.wantAvatar != "" && got.Avatar != tc.wantAvatar {
				t.Errorf("avatar = %q, want %q", got.Avatar, tc.wantAvatar)
			}
		})
	}
}
