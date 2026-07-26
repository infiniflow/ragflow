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
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func securityCtx(t *testing.T, remoteAddr string, headers map[string]string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/agents/c1/webhook", strings.NewReader("{}"))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.RemoteAddr = remoteAddr
	for k, v := range headers {
		c.Request.Header.Set(k, v)
	}
	return c
}

// TestValidateMaxBodySize_Allowed covers the no-op branch.
func TestValidateMaxBodySize_Allowed(t *testing.T) {
	c := securityCtx(t, "1.2.3.4:0", nil)
	c.Request.ContentLength = 100
	if err := validateMaxBodySize(c, map[string]any{"max_body_size": "1kb"}); err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}

// TestValidateMaxBodySize_TooLarge covers the size-mismatch branch.
func TestValidateMaxBodySize_TooLarge(t *testing.T) {
	c := securityCtx(t, "1.2.3.4:0", nil)
	c.Request.ContentLength = 2048
	err := validateMaxBodySize(c, map[string]any{"max_body_size": "1kb"})
	if err == nil || !strings.Contains(err.Error(), "request body too large") {
		t.Errorf("err = %v, want 'request body too large'", err)
	}
}

// TestValidateMaxBodySize_BadFormat covers non-numeric format.
func TestValidateMaxBodySize_BadFormat(t *testing.T) {
	c := securityCtx(t, "1.2.3.4:0", nil)
	err := validateMaxBodySize(c, map[string]any{"max_body_size": "1gb"})
	if err == nil || !strings.Contains(err.Error(), "invalid max_body_size format") {
		t.Errorf("err = %v, want 'invalid max_body_size format'", err)
	}
}

// TestValidateIPWhitelist_EmptyIsAllow covers the empty-list branch.
func TestValidateIPWhitelist_EmptyIsAllow(t *testing.T) {
	c := securityCtx(t, "1.2.3.4:0", nil)
	if err := validateIPWhitelist(c, map[string]any{"ip_whitelist": []any{}}); err != nil {
		t.Errorf("empty whitelist: err = %v, want nil", err)
	}
}

// TestValidateIPWhitelist_ExactMatch passes when client IP matches.
func TestValidateIPWhitelist_ExactMatch(t *testing.T) {
	c := securityCtx(t, "10.0.0.5:0", nil)
	cfg := map[string]any{"ip_whitelist": []any{"10.0.0.5"}}
	if err := validateIPWhitelist(c, cfg); err != nil {
		t.Errorf("exact match: err = %v, want nil", err)
	}
}

// TestValidateIPWhitelist_CIDR covers the CIDR branch.
func TestValidateIPWhitelist_CIDR(t *testing.T) {
	c := securityCtx(t, "10.0.0.5:0", nil)
	cfg := map[string]any{"ip_whitelist": []any{"10.0.0.0/8"}}
	if err := validateIPWhitelist(c, cfg); err != nil {
		t.Errorf("cidr match: err = %v, want nil", err)
	}
}

// TestValidateIPWhitelist_RejectForeign confirms a foreign IP is denied.
func TestValidateIPWhitelist_RejectForeign(t *testing.T) {
	c := securityCtx(t, "192.168.1.5:0", nil)
	cfg := map[string]any{"ip_whitelist": []any{"10.0.0.0/8"}}
	err := validateIPWhitelist(c, cfg)
	if err == nil || !strings.Contains(err.Error(), "not allowed by whitelist") {
		t.Errorf("err = %v, want 'not allowed by whitelist'", err)
	}
}

// TestValidateAuth_NoneRequiresOptIn covers the auth_type=="none"
// opt-in: the old "fail open" default was closed by PR #14890.
// An anonymous webhook must explicitly set allow_anonymous=true
// to pass; the bare {"auth_type":"none"} block now rejects.
func TestValidateAuth_NoneRequiresOptIn(t *testing.T) {
	c := securityCtx(t, "1.2.3.4:0", nil)
	// Bare auth_type=none → must reject (no opt-in).
	if err := validateAuth(c, map[string]any{"auth_type": "none"}); err == nil {
		t.Errorf("bare auth_type=none: want error (no opt-in), got nil")
	}
	// With explicit allow_anonymous=true → must pass.
	if err := validateAuth(c, map[string]any{"auth_type": "none", "allow_anonymous": true}); err != nil {
		t.Errorf("opt-in auth_type=none + allow_anonymous=true: err = %v, want nil", err)
	}
}

// TestValidateAuth_Unsupported covers unknown auth_type.
func TestValidateAuth_Unsupported(t *testing.T) {
	c := securityCtx(t, "1.2.3.4:0", nil)
	err := validateAuth(c, map[string]any{"auth_type": "weird"})
	if err == nil || !strings.Contains(err.Error(), "unsupported auth_type") {
		t.Errorf("err = %v, want 'unsupported auth_type'", err)
	}
}

// TestValidateTokenAuth_HeaderValue covers matching and non-matching cases.
func TestValidateTokenAuth_HeaderValue(t *testing.T) {
	cfg := map[string]any{
		"token": map[string]any{
			"token_header": "X-Token",
			"token_value":  "abc",
		},
	}

	// pass
	cPass := securityCtx(t, "1.2.3.4:0", map[string]string{"X-Token": "abc"})
	if err := validateTokenAuth(cPass, cfg); err != nil {
		t.Errorf("matching token: err = %v, want nil", err)
	}

	// fail
	cFail := securityCtx(t, "1.2.3.4:0", map[string]string{"X-Token": "wrong"})
	if err := validateTokenAuth(cFail, cfg); err == nil {
		t.Errorf("non-matching token: err = nil, want error")
	}
}

// TestValidateBasicAuth_PassAndFail covers both branches.
func TestValidateBasicAuth_PassAndFail(t *testing.T) {
	cfg := map[string]any{
		"basic_auth": map[string]any{
			"username": "alice",
			"password": "wonderland",
		},
	}

	cPass := securityCtx(t, "1.2.3.4:0", nil)
	cPass.Request.SetBasicAuth("alice", "wonderland")
	if err := validateBasicAuth(cPass, cfg); err != nil {
		t.Errorf("matching basic: err = %v, want nil", err)
	}

	cFail := securityCtx(t, "1.2.3.4:0", nil)
	cFail.Request.SetBasicAuth("alice", "wrong")
	if err := validateBasicAuth(cFail, cfg); err == nil {
		t.Errorf("non-matching basic: err = nil, want error")
	}
}

// TestValidateJWTAuth_NoSecret rejects empty secret config.
func TestValidateJWTAuth_NoSecret(t *testing.T) {
	c := securityCtx(t, "1.2.3.4:0", map[string]string{"Authorization": "Bearer x"})
	err := validateJWTAuth(c, map[string]any{"jwt": map[string]any{}})
	if err == nil || !strings.Contains(err.Error(), "secret not configured") {
		t.Errorf("err = %v, want 'secret not configured'", err)
	}
}

// TestValidateJWTAuth_MissingBearer rejects when Authorization is absent.
func TestValidateJWTAuth_MissingBearer(t *testing.T) {
	c := securityCtx(t, "1.2.3.4:0", nil)
	err := validateJWTAuth(c, map[string]any{
		"jwt": map[string]any{"secret": "s"},
	})
	if err == nil || !strings.Contains(err.Error(), "missing bearer token") {
		t.Errorf("err = %v, want 'missing bearer token'", err)
	}
}

// TestValidateJWTAuth_RS256HappyPath confirms that an RS256 token
// signed with a real RSA key is accepted when the secret field holds
// the matching PEM-encoded public key. The python reference uses the
// same `secret` slot for both HMAC and RSA inputs; this test pins that
// contract for the Go port.
func TestValidateJWTAuth_RS256HappyPath(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	pubPEM := encodeRSAPublicKeyPEM(&priv.PublicKey)

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "u-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString(priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	c := securityCtx(t, "1.2.3.4:0", map[string]string{"Authorization": "Bearer " + signed})
	err = validateJWTAuth(c, map[string]any{
		"jwt": map[string]any{
			"secret":    string(pubPEM),
			"algorithm": "RS256",
		},
	})
	if err != nil {
		t.Errorf("RS256 happy path: err = %v, want nil", err)
	}
}

// TestValidateJWTAuth_RS256BadPEM rejects a malformed public key.
func TestValidateJWTAuth_RS256BadPEM(t *testing.T) {
	c := securityCtx(t, "1.2.3.4:0", map[string]string{"Authorization": "Bearer x"})
	err := validateJWTAuth(c, map[string]any{
		"jwt": map[string]any{
			"secret":    "not a pem block",
			"algorithm": "RS256",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "rsa public key") {
		t.Errorf("err = %v, want 'rsa public key' parse error", err)
	}
}

// TestValidateJWTAuth_UnsupportedAlgorithm rejects truly unknown algos.
func TestValidateJWTAuth_UnsupportedAlgorithm(t *testing.T) {
	c := securityCtx(t, "1.2.3.4:0", map[string]string{"Authorization": "Bearer x"})
	err := validateJWTAuth(c, map[string]any{
		"jwt": map[string]any{
			"secret":    "s",
			"algorithm": "none",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("err = %v, want 'unsupported'", err)
	}
}

// encodeRSAPublicKeyPEM serialises an *rsa.PublicKey into the
// PEM-encoded PKIX form that jwt.ParseRSAPublicKeyFromPEM expects.
func encodeRSAPublicKeyPEM(pub *rsa.PublicKey) []byte {
	asn1, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		panic(err)
	}
	block := &pem.Block{Type: "PUBLIC KEY", Bytes: asn1}
	return pem.EncodeToMemory(block)
}

// TestValidateJWTAuth_ReservedClaimRejected covers the reserved-claim
// guard. We build a valid HS256 JWT first so the parse succeeds, then
// ask for `exp` as a required claim — the validator must reject it.
func TestValidateJWTAuth_ReservedClaimRejected(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "u-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte("s"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	c := securityCtx(t, "1.2.3.4:0", map[string]string{"Authorization": "Bearer " + signed})
	err = validateJWTAuth(c, map[string]any{
		"jwt": map[string]any{
			"secret":          "s",
			"required_claims": "exp",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "reserved jwt claim") {
		t.Errorf("err = %v, want 'reserved jwt claim'", err)
	}
}

// TestValidateRateLimit_NoConfig covers the no-rate-limit branch.
func TestValidateRateLimit_NoConfig(t *testing.T) {
	if err := validateRateLimit("c1", map[string]any{}); err != nil {
		t.Errorf("no rate_limit: err = %v, want nil", err)
	}
}

// TestValidateRateLimit_BadPer rejects unknown per window.
func TestValidateRateLimit_BadPer(t *testing.T) {
	err := validateRateLimit("c1", map[string]any{
		"rate_limit": map[string]any{"limit": 10, "per": "week"},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid rate_limit.per") {
		t.Errorf("err = %v, want 'invalid rate_limit.per'", err)
	}
}

// TestValidateRateLimit_BadLimit rejects non-positive limits.
func TestValidateRateLimit_BadLimit(t *testing.T) {
	err := validateRateLimit("c1", map[string]any{
		"rate_limit": map[string]any{"limit": 0, "per": "minute"},
	})
	if err == nil || !strings.Contains(err.Error(), "must be > 0") {
		t.Errorf("err = %v, want 'must be > 0'", err)
	}
}

// (No helper needed at the bottom of this file; helper functions
// inline above.)
// _securityUnused previously lived here as a placeholder; deleted
// during cleanup (code-review MEDIUM-2).

// TestValidateMaxBodySize_OverflowGuard covers CodeRabbit PR review
// #3: a configured n that would overflow n*bytesPerMB (e.g. a huge
// mb value) must be rejected before the multiplication, not
// silently wrap to a small number and pass the cap check.
//
// The chosen n=9_223_372_036_855 multiplied by 1_000_000 (= 1 MB)
// overflows int64 max (9.22e18), so without the pre-multiplication
// guard this would silently wrap to a small positive number and
// the cap check would (incorrectly) succeed. With the guard in
// place, the rejection happens at the parse step, before any
// multiplication.
func TestValidateMaxBodySize_OverflowGuard(t *testing.T) {
	c := securityCtx(t, "1.2.3.4:0", nil)
	err := validateMaxBodySize(c, map[string]any{"max_body_size": "9223372036855mb"})
	if err == nil {
		t.Errorf("huge mb value: err = nil, want overflow-rejection error")
	} else if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("err = %v, want 'exceeds maximum'", err)
	}
}

// TestParseMaxBodySize_DecimalUnits documents the per-user-request
// SI-decimal unit base: 1 kb = 1 000 B, 1 mb = 1 000 000 B.
// (The python reference uses 1 024 / 1 048 576.)
func TestParseMaxBodySize_DecimalUnits(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1kb", 1000},
		{"5kb", 5000},
		{"1mb", 1_000_000},
		{"10mb", 10_000_000}, // exact cap
	}
	for _, tc := range cases {
		got, err := parseMaxBodySize(map[string]any{"max_body_size": tc.in})
		if err != nil {
			t.Errorf("parseMaxBodySize(%q): err = %v, want nil", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseMaxBodySize(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestValidateTokenAuth_EmptyValueRejected covers CodeRabbit PR
// review #4: an empty configured token_value used to mean "accept
// any request without that header". Now it must be rejected.
func TestValidateTokenAuth_EmptyValueRejected(t *testing.T) {
	cfg := map[string]any{
		"token": map[string]any{
			"token_header": "X-Token",
			"token_value":  "",
		},
	}
	c := securityCtx(t, "1.2.3.4:0", nil) // no header at all
	if err := validateTokenAuth(c, cfg); err == nil {
		t.Errorf("empty token_value: err = nil, want error")
	}
}

// TestValidateWebhookSecurity_RejectsEmptyConfig guards PR
// #14890: empty / nil security config used to be allowed by
// default (fail-open), letting unauthenticated webhooks fire on
// any canvas. The fix requires an explicit opt-in via
// allow_anonymous=true. The handler must return the same generic
// error the python fix uses, so a probe cannot distinguish
// "missing config" from "exists but no allow_anonymous".
func TestValidateWebhookSecurity_RejectsEmptyConfig(t *testing.T) {
	if err := validateWebhookSecurity(map[string]any{}, newSecurityCtx("c1"), "c1"); err == nil {
		t.Fatal("empty config: want error, got nil")
	}
	if err := validateWebhookSecurity(nil, newSecurityCtx("c1"), "c1"); err == nil {
		t.Fatal("nil config: want error, got nil")
	}
}

// TestValidateWebhookSecurity_RejectsAnonymousWithoutOptIn covers
// the auth_type=none case without allow_anonymous — used to be
// allowed silently. Must be rejected.
func TestValidateWebhookSecurity_RejectsAnonymousWithoutOptIn(t *testing.T) {
	cases := []map[string]any{
		{"auth_type": "none"},
		{"auth_type": "none", "allow_anonymous": false},
		{"auth_type": "none", "allow_anonymous": "false"},
		{"auth_type": ""},
		{"auth_type": "", "allow_anonymous": "yes please"},
	}
	for _, cfg := range cases {
		if err := validateWebhookSecurity(cfg, newSecurityCtx("c1"), "c1"); err == nil {
			t.Errorf("cfg %v: want error, got nil", cfg)
		}
	}
}

// TestValidateWebhookSecurity_FailClosedSameError pins PR review
// round 5 (#2): the two fail-closed branches (empty security block
// vs. anonymous-without-opt-in) MUST return the same error so a
// probe cannot distinguish them. Using errors.Is lets the test
// survive cosmetic wording tweaks; the assertion is on identity.
func TestValidateWebhookSecurity_FailClosedSameError(t *testing.T) {
	missing := validateWebhookSecurity(map[string]any{}, newSecurityCtx("c1"), "c1")
	if missing == nil {
		t.Fatal("empty cfg: want errWebhookFailClosed, got nil")
	}
	anon := validateWebhookSecurity(map[string]any{"auth_type": "none"}, newSecurityCtx("c1"), "c1")
	if anon == nil {
		t.Fatal("auth_type=none: want errWebhookFailClosed, got nil")
	}
	if missing.Error() != anon.Error() {
		t.Errorf("fail-closed branches must share one error string\n"+
			"  missing-config: %q\n"+
			"  anonymous:      %q", missing.Error(), anon.Error())
	}
	if !errors.Is(missing, errWebhookFailClosed) || !errors.Is(anon, errWebhookFailClosed) {
		t.Errorf("both branches must be errors.Is(errWebhookFailClosed); missing=%v anon=%v", missing, anon)
	}
}

// TestValidateWebhookSecurity_AllowsAnonymousWithOptIn is the
// positive control: auth_type=none with an explicit
// allow_anonymous=true must pass. The python frontend now
// serialises this when the user picks "None" auth.
func TestValidateWebhookSecurity_AllowsAnonymousWithOptIn(t *testing.T) {
	cases := []map[string]any{
		{"auth_type": "none", "allow_anonymous": true},
		{"auth_type": "none", "allow_anonymous": "true"},
		{"auth_type": "none", "allow_anonymous": "1"},
		{"auth_type": "none", "allow_anonymous": "yes"},
		{"auth_type": "none", "allow_anonymous": "on"},
	}
	for _, cfg := range cases {
		if err := validateWebhookSecurity(cfg, newSecurityCtx("c1"), "c1"); err != nil {
			t.Errorf("cfg %v: want nil, got %v", cfg, err)
		}
	}
}

// newSecurityCtx is a tiny helper that builds a *gin.Context with
// just enough request surface for validateWebhookSecurity to run
// without panicking on a nil receiver.
func newSecurityCtx(canvasID string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+canvasID+"/webhook", nil)
	return c
}
