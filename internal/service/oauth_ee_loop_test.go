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
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	jwt "github.com/golang-jwt/jwt/v5"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/server"
)

// setupOIDCServerConfig renders the deployment config template to concrete
// values (the ${VAR:-default} placeholders viper does not expand) and appends a
// single OIDC channel, then loads it into the process-global server config via
// server.Init. The provider's discovery endpoints are overridden at runtime by
// the caller to point at the mock IdP.
func setupOIDCServerConfig(t *testing.T) {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	tmplPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "docker", "service_conf.yaml.template")
	raw, err := os.ReadFile(tmplPath)
	if err != nil {
		t.Fatalf("read config template: %v", err)
	}
	reDef := regexp.MustCompile(`\$\{[A-Za-z0-9_]+:-([^}]*)\}`)
	reBare := regexp.MustCompile(`\$\{[A-Za-z0-9_]+\}`)
	rendered := reBare.ReplaceAllString(reDef.ReplaceAllString(string(raw), "$1"), "")
	rendered += `
authentication:
  enable_register: false
  disable_password_login: true
oauth:
  oidc:
    type: "oidc"
    display_name: "Example SSO"
    icon: "oidc"
    client_id: "placeholder-overridden-at-runtime"
    client_secret: "x"
    issuer: "https://placeholder.example"
    scope: "openid email profile"
    redirect_uri: "https://placeholder.example/api/v1/auth/oauth/oidc/callback"
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "service_conf.yaml")
	if err := os.WriteFile(cfgPath, []byte(rendered), 0o644); err != nil {
		t.Fatalf("write rendered config: %v", err)
	}
	if err := server.Init(cfgPath); err != nil {
		t.Fatalf("server.Init: %v", err)
	}
}

// TestOIDCFullLoopWithConfig exercises the complete service-layer login loop
// against a mock IdP and an in-memory DB, with redis=nil (the consume-once
// replay layer is defense-in-depth over the double-submit state cookie, which
// IS exercised here). It is the offline analogue of the production flow:
// GetLoginChannels → OAuthLoginInitiate → OAuthCallback (provision) →
// OAuthCallback (rotate) → CSRF rejection.
func TestOIDCFullLoopWithConfig(t *testing.T) {
	setupOIDCServerConfig(t)
	oidcAllowLoopback(t)

	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	m := newMockIdP(t)
	prov := server.GetConfig().GetOAuthConfig().Providers["oidc"]
	if prov == nil {
		t.Fatal("oidc provider was not registered from config")
	}
	prov.Issuer = m.baseURL
	prov.ClientID = testClientID
	prov.ClientSecret = testClientSecret
	prov.RedirectURI = "https://ragflow.example.com/api/v1/auth/oauth/oidc/callback"
	prov.Scope = "openid email profile"

	svc := NewUserService()
	ctx := context.Background()

	// Regression guard: OIDC provisioning must NOT be gated by EnableRegister
	// (which governs password self-signup only). The rendered config disables
	// registration, yet a first-time SSO login below must still provision — this
	// matches v0.27.2, where the OAuth callback bypasses the REGISTER_ENABLED check.
	if server.GetConfig().EnableRegister() {
		t.Fatal("test precondition: expected EnableRegister=false")
	}

	// --- GetLoginChannels: the login page must see the channel ---------------
	channels, _, err := svc.GetLoginChannels()
	if err != nil {
		t.Fatalf("GetLoginChannels: %v", err)
	}
	var found *LoginChannel
	for _, c := range channels {
		if c.Channel == "oidc" {
			found = c
		}
	}
	if found == nil {
		t.Fatal("oidc channel not returned by GetLoginChannels")
	}
	if found.DisplayName != "Example SSO" {
		t.Errorf("display name = %q, want Example SSO", found.DisplayName)
	}

	// --- OAuthLoginInitiate: redirect to the IdP authorize endpoint ----------
	init, _, err := svc.OAuthLoginInitiate("oidc", nil)
	if err != nil {
		t.Fatalf("OAuthLoginInitiate: %v", err)
	}
	if init.State == "" {
		t.Error("expected a non-empty CSRF state")
	}
	for _, want := range []string{
		m.baseURL + "/authorize",
		"client_id=" + testClientID,
		"response_type=code",
		"state=" + init.State,
	} {
		if !strings.Contains(init.AuthURL, want) {
			t.Errorf("AuthURL %q missing %q", init.AuthURL, want)
		}
	}

	// --- OAuthCallback: first login provisions a new user --------------------
	m.idTokClaim = jwt.MapClaims{
		"sub":                "user-0001",
		"preferred_username": "jd456",
		"name":               "Jane Doe",
		"email":              "jd456@example.com",
		"email_verified":     true,
	}
	m.userinfo = map[string]any{
		"email":          "jd456@example.com",
		"email_verified": true,
		"picture":        "https://idp.example.com/avatars/jd456.png",
	}

	res, _, err := svc.OAuthCallback(ctx, "oidc", "code-1", "state-A", "state-A", nil)
	if err != nil {
		t.Fatalf("OAuthCallback (new user): %v", err)
	}
	if !res.IsNewUser {
		t.Error("expected IsNewUser=true on first login")
	}
	if res.User.AccessToken == nil || *res.User.AccessToken == "" {
		t.Error("expected an access token to be minted")
	}

	// Verify the persisted identity + provisioning side effects.
	var u entity.User
	if err := db.Where("email = ?", "jd456@example.com").First(&u).Error; err != nil {
		t.Fatalf("provisioned user not found: %v", err)
	}
	if u.Nickname != "Jane Doe" {
		t.Errorf("nickname = %q, want Jane Doe", u.Nickname)
	}
	if u.LoginChannel == nil || *u.LoginChannel != "oidc" {
		t.Errorf("login channel = %v, want oidc", u.LoginChannel)
	}
	if u.Password != nil {
		t.Error("SSO user must not have a password")
	}
	var tenantCount, userTenantCount, fileCount int64
	db.Model(&entity.Tenant{}).Where("id = ?", u.ID).Count(&tenantCount)
	db.Model(&entity.UserTenant{}).Where("user_id = ?", u.ID).Count(&userTenantCount)
	db.Model(&entity.File{}).Where("tenant_id = ?", u.ID).Count(&fileCount)
	if tenantCount != 1 || userTenantCount != 1 || fileCount != 1 {
		t.Errorf("provisioning rows: tenant=%d userTenant=%d file=%d, want 1/1/1",
			tenantCount, userTenantCount, fileCount)
	}
	firstToken := *res.User.AccessToken

	// --- OAuthCallback: returning user rotates token, no new provisioning ----
	res2, _, err := svc.OAuthCallback(ctx, "oidc", "code-2", "state-B", "state-B", nil)
	if err != nil {
		t.Fatalf("OAuthCallback (returning user): %v", err)
	}
	if res2.IsNewUser {
		t.Error("expected IsNewUser=false on second login")
	}
	if *res2.User.AccessToken == firstToken {
		t.Error("expected the access token to be rotated on re-login")
	}
	var userCount int64
	db.Model(&entity.User{}).Where("email = ?", "jd456@example.com").Count(&userCount)
	if userCount != 1 {
		t.Errorf("user count = %d, want 1 (no duplicate provisioning)", userCount)
	}

	// --- CSRF: mismatched double-submit state is rejected --------------------
	_, code, err := svc.OAuthCallback(ctx, "oidc", "code-3", "state-X", "state-Y", nil)
	if err == nil {
		t.Fatal("expected mismatched state to be rejected")
	}
	if code != common.CodeAuthenticationError {
		t.Errorf("mismatched-state code = %v, want CodeAuthenticationError", code)
	}
}

// newOIDCLoopFixture wires server config + in-memory DB + a mock IdP and returns
// a service and the mock with its OIDC provider pointed at the mock.
func newOIDCLoopFixture(t *testing.T) (*UserService, *mockIdP) {
	t.Helper()
	setupOIDCServerConfig(t)
	oidcAllowLoopback(t)
	pushServiceDB(t, setupServiceTestDB(t))
	m := newMockIdP(t)
	prov := server.GetConfig().GetOAuthConfig().Providers["oidc"]
	if prov == nil {
		t.Fatal("oidc provider not registered")
	}
	prov.Issuer = m.baseURL
	prov.ClientID = testClientID
	prov.ClientSecret = testClientSecret
	prov.RedirectURI = "https://ragflow.example.com/api/v1/auth/oauth/oidc/callback"
	prov.Scope = "openid email profile"
	return NewUserService(), m
}

// TestOIDCRejectsUnverifiedEmail: an untrusted provider that does not assert
// email_verified=true must not let a login bind to an email (account-takeover guard).
func TestOIDCRejectsUnverifiedEmail(t *testing.T) {
	svc, m := newOIDCLoopFixture(t)
	m.idTokClaim = jwt.MapClaims{"sub": "s-unverif", "preferred_username": "nover", "email": "nover@example.com"}
	m.userinfo = map[string]any{"email": "nover@example.com"} // no email_verified
	_, code, err := svc.OAuthCallback(context.Background(), "oidc", "c", "s", "s", nil)
	if err == nil {
		t.Fatal("expected unverified email to be rejected")
	}
	if code != common.CodeForbidden {
		t.Errorf("code = %v, want CodeForbidden", code)
	}
}

// TestOIDCTrustEmailVerifiedAllowsUnverified: when the provider is marked
// trust_email_verified, a login is accepted even without the email_verified claim.
func TestOIDCTrustEmailVerifiedAllowsUnverified(t *testing.T) {
	svc, m := newOIDCLoopFixture(t)
	server.GetConfig().GetOAuthConfig().Providers["oidc"].TrustEmailVerified = true
	m.idTokClaim = jwt.MapClaims{"sub": "s-trust", "preferred_username": "tuser", "email": "tuser@example.com"}
	m.userinfo = map[string]any{"email": "tuser@example.com"} // no email_verified
	res, _, err := svc.OAuthCallback(context.Background(), "oidc", "c", "s", "s", nil)
	if err != nil {
		t.Fatalf("trusted provider should accept unverified email: %v", err)
	}
	if !res.IsNewUser {
		t.Error("expected a new user to be provisioned")
	}
}

// TestOIDCRejectsSubMismatch: userinfo sub differing from the ID-token sub is a
// protocol violation (OIDC Core §5.3.2) and must fail.
func TestOIDCRejectsSubMismatch(t *testing.T) {
	svc, m := newOIDCLoopFixture(t)
	m.idTokClaim = jwt.MapClaims{"sub": "id-sub", "email": "x@example.com", "email_verified": true}
	m.userinfo = map[string]any{"sub": "different-sub", "email": "x@example.com", "email_verified": true}
	_, _, err := svc.OAuthCallback(context.Background(), "oidc", "c", "s", "s", nil)
	if err == nil {
		t.Fatal("expected sub mismatch between id_token and userinfo to be rejected")
	}
}
