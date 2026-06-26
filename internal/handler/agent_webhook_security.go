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

// Webhook security helpers.
//
// These mirror api/apps/restful_apis/agent_api.py:1602-1810
// (validate_webhook_security + the six sub-validators) from the Python
// webhook handler. The Go port preserves the Python semantics exactly:
//
//   - max body size (with the 10 MB cap at agent_api.py:1652)
//   - IP whitelist (CIDR + exact match)
//   - rate limit (token bucket via redis.EvalTokenBucketStrict — strict
//     fail-closed; see redis.go)
//   - token auth (header check)
//   - basic auth (HTTP Basic)
//   - JWT (HS/RS256, audience/issuer/required-claims)
//
// All helpers return a Go error so the handler can surface the
// Python-shaped 102 envelope directly. Empty security config means
// "no checks" (matches agent_api.py:1607).

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	rediscli "ragflow/internal/engine/redis"
)

const (
	// webhookBodyMaxBytes is the hard ceiling above which a configured
	// max_body_size is rejected at config-validation time. Mirrors the
	// python MAX_LIMIT = 10 * 1024 * 1024 at agent_api.py:1652.
	webhookBodyMaxBytes int64 = 10 * 1024 * 1024

	// webhookRateLimitTimeout is the redis call timeout used for the
	// strict token-bucket lookup. Short on purpose — a security gate
	// must not stall the request thread.
	webhookRateLimitTimeout = 500 * time.Millisecond

	// jwtReservedClaims is the python-parity set at agent_api.py:1800
	// — required_claims must NOT name any of these.
	jwtReservedClaims = "exp,sub,aud,iss,nbf,iat"
)

// validateWebhookSecurity is the orchestrator. Empty/nil cfg → no-op
// (matches agent_api.py:1607 "No security config → allowed by default").
//
// Sub-validators run in the python-defined order:
//  1. validateMaxBodySize
//  2. validateIPWhitelist
//  3. validateRateLimit
//  4. validateAuth (dispatches on auth_type)
func validateWebhookSecurity(
	securityCfg map[string]any,
	c *gin.Context,
	canvasID string,
) error {
	if len(securityCfg) == 0 {
		return nil
	}
	if err := validateMaxBodySize(c, securityCfg); err != nil {
		return err
	}
	if err := validateIPWhitelist(c, securityCfg); err != nil {
		return err
	}
	if err := validateRateLimit(canvasID, securityCfg); err != nil {
		return err
	}
	return validateAuth(c, securityCfg)
}

// validateMaxBodySize mirrors python agent_api.py:1636-1658.
//
// Format: "<n>kb" | "<n>mb" (case-insensitive). Anything else is a
// config bug. The configured limit is capped at webhookBodyMaxBytes
// (10 MB) — exceeding that is also a config error, not silently raised.
// The actual request size is then compared against the parsed limit.
func validateMaxBodySize(c *gin.Context, cfg map[string]any) error {
	limit, err := parseMaxBodySize(cfg)
	if err != nil {
		return err
	}
	if limit <= 0 {
		return nil
	}
	contentLength := c.Request.ContentLength
	if contentLength < 0 {
		contentLength = 0
	}
	if contentLength > limit {
		return fmt.Errorf("request body too large: %d > %d", contentLength, limit)
	}
	return nil
}

// parseMaxBodySize returns the byte limit configured by
// `max_body_size` (with the 10 MB cap enforced) or 0 when no limit
// is configured. The handler uses this to wrap c.Request.Body in
// http.MaxBytesReader so the actual stream read is bounded — the
// Content-Length header check alone is insufficient because a client
// can advertise a small Content-Length and stream more.
func parseMaxBodySize(cfg map[string]any) (int64, error) {
	raw, ok := cfg["max_body_size"].(string)
	if !ok || raw == "" {
		return 0, nil
	}
	sizeStr := strings.ToLower(strings.TrimSpace(raw))
	var limit int64
	switch {
	case strings.HasSuffix(sizeStr, "kb"):
		n, err := strconv.ParseInt(strings.TrimSuffix(sizeStr, "kb"), 10, 64)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid max_body_size format")
		}
		limit = n * 1024
	case strings.HasSuffix(sizeStr, "mb"):
		n, err := strconv.ParseInt(strings.TrimSuffix(sizeStr, "mb"), 10, 64)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid max_body_size format")
		}
		limit = n * 1024 * 1024
	default:
		return 0, fmt.Errorf("invalid max_body_size format")
	}
	if limit > webhookBodyMaxBytes {
		return 0, fmt.Errorf("max_body_size exceeds maximum allowed size (10MB)")
	}
	return limit, nil
}

// validateIPWhitelist mirrors python agent_api.py:1660-1679. Empty
// list → allow. Supports CIDR ("10.0.0.0/8") and exact ("1.2.3.4").
// The client IP comes from gin's c.ClientIP() which honours
// X-Forwarded-For when trusted proxies are configured.
func validateIPWhitelist(c *gin.Context, cfg map[string]any) error {
	whitelist, _ := cfg["ip_whitelist"].([]any)
	if len(whitelist) == 0 {
		return nil
	}
	clientIP := c.ClientIP()
	for _, raw := range whitelist {
		rule, _ := raw.(string)
		if rule == "" {
			continue
		}
		if strings.Contains(rule, "/") {
			// CIDR
			_, ipNet, err := net.ParseCIDR(rule)
			if err != nil {
				continue
			}
			addr := net.ParseIP(clientIP)
			if addr != nil && ipNet.Contains(addr) {
				return nil
			}
			continue
		}
		// Exact match
		if clientIP == rule {
			return nil
		}
	}
	return fmt.Errorf("IP %s is not allowed by whitelist", clientIP)
}

// validateRateLimit mirrors python agent_api.py:1681-1723.
//
// Window mapping (matches agent_api.py:1692-1697):
//
//	second → 1s, minute → 60s, hour → 3600s, day → 86400s.
//
// Unknown per → error (NOT silently fall through).
//
// Strict fail-closed: any Redis error → error. The webhook handler
// surfaces this as 102 so an operator notices a misconfiguration.
func validateRateLimit(canvasID string, cfg map[string]any) error {
	rawRL, ok := cfg["rate_limit"].(map[string]any)
	if !ok || len(rawRL) == 0 {
		return nil
	}
	limitF, ok := rawRL["limit"].(float64)
	if !ok {
		// JSON numbers often come back as float64; try int as well.
		if limitI, ok2 := rawRL["limit"].(int); ok2 {
			limitF = float64(limitI)
			ok = true
		}
	}
	if !ok || limitF <= 0 {
		return fmt.Errorf("rate_limit.limit must be > 0")
	}
	per, _ := rawRL["per"].(string)
	if per == "" {
		per = "minute"
	}
	var window float64
	switch per {
	case "second":
		window = 1
	case "minute":
		window = 60
	case "hour":
		window = 3600
	case "day":
		window = 86400
	default:
		return fmt.Errorf("invalid rate_limit.per: %s", per)
	}

	key := fmt.Sprintf("rl:tb:%s", canvasID)
	ctx, cancel := context.WithTimeout(context.Background(), webhookRateLimitTimeout)
	defer cancel()

	rdb := rediscli.Get()
	if rdb == nil {
		return fmt.Errorf("rate limit error: redis not initialised")
	}
	allowed, err := rdb.EvalTokenBucketStrict(ctx, key, limitF, limitF/window)
	if err != nil {
		return fmt.Errorf("rate limit error: %s", err.Error())
	}
	if !allowed {
		return fmt.Errorf("too many requests (rate limit exceeded)")
	}
	return nil
}

// validateAuth dispatches on auth_type. Empty cfg or auth_type=="none"
// → allow (matches agent_api.py:1621).
func validateAuth(c *gin.Context, cfg map[string]any) error {
	authType, _ := cfg["auth_type"].(string)
	if authType == "" || authType == "none" {
		return nil
	}
	switch authType {
	case "token":
		return validateTokenAuth(c, cfg)
	case "basic":
		return validateBasicAuth(c, cfg)
	case "jwt":
		return validateJWTAuth(c, cfg)
	}
	return fmt.Errorf("unsupported auth_type: %s", authType)
}

// validateTokenAuth mirrors python agent_api.py:1725-1733.
func validateTokenAuth(c *gin.Context, cfg map[string]any) error {
	rawToken, _ := cfg["token"].(map[string]any)
	if rawToken == nil {
		return fmt.Errorf("Invalid token authentication")
	}
	header, _ := rawToken["token_header"].(string)
	want, _ := rawToken["token_value"].(string)
	if header == "" {
		return fmt.Errorf("Invalid token authentication")
	}
	if c.GetHeader(header) != want {
		return fmt.Errorf("Invalid token authentication")
	}
	return nil
}

// validateBasicAuth mirrors python agent_api.py:1735-1743. We use
// gin's c.Request.BasicAuth() which parses the Authorization header
// and returns the (user, pass, ok) triple.
func validateBasicAuth(c *gin.Context, cfg map[string]any) error {
	rawBasic, _ := cfg["basic_auth"].(map[string]any)
	if rawBasic == nil {
		return fmt.Errorf("Invalid Basic Auth credentials")
	}
	username, _ := rawBasic["username"].(string)
	password, _ := rawBasic["password"].(string)
	u, p, ok := c.Request.BasicAuth()
	if !ok || u != username || p != password {
		return fmt.Errorf("Invalid Basic Auth credentials")
	}
	return nil
}

// validateJWTAuth mirrors python agent_api.py:1745-1809.
//
// Algorithm defaults to HS256. audience / issuer are validated only
// when configured. required_claims rejects reserved JWT claims
// (exp sub aud iss nbf iat) and any missing claims.
//
// Algorithms:
//   - HS256 / HS384 / HS512 → secret is a shared HMAC key.
//   - RS256 / RS384 / RS512 → secret is a PEM-encoded RSA public key.
//   - ES256 / ES384 / ES512 → secret is a PEM-encoded EC public key.
//
// The Python reference uses the same `secret` field for all three
// families (the python jwt library is happy to take either a string
// or a PEM block); we mirror that with one `secret` config slot and
// dispatch on the algorithm.
func validateJWTAuth(c *gin.Context, cfg map[string]any) error {
	rawJWT, _ := cfg["jwt"].(map[string]any)
	if rawJWT == nil {
		return fmt.Errorf("jwt secret not configured")
	}
	secret, _ := rawJWT["secret"].(string)
	if secret == "" {
		return fmt.Errorf("jwt secret not configured")
	}

	authHeader := c.GetHeader("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return fmt.Errorf("missing bearer token")
	}
	tokenStr := strings.TrimSpace(authHeader[len(prefix):])
	if tokenStr == "" {
		return fmt.Errorf("empty bearer token")
	}

	alg, _ := rawJWT["algorithm"].(string)
	if alg == "" {
		alg = "HS256"
	}
	alg = strings.ToUpper(alg)

	// Build the parser options.
	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{alg}),
	}
	if aud, ok := rawJWT["audience"].(string); ok && aud != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(aud))
	}
	if iss, ok := rawJWT["issuer"].(string); ok && iss != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(iss))
	}

	keyFunc, keyErr := jwtKeyFunc(alg, secret)
	if keyErr != nil {
		return keyErr
	}

	token, err := jwt.Parse(tokenStr, keyFunc, parserOpts...)
	if err != nil {
		return fmt.Errorf("invalid jwt: %s", err.Error())
	}
	if !token.Valid {
		return fmt.Errorf("invalid jwt")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return fmt.Errorf("invalid jwt claims")
	}

	// Required claims validation (mirrors agent_api.py:1787-1808).
	required := collectStringSlice(rawJWT["required_claims"])
	reserved := splitCSV(jwtReservedClaims)
	for _, claim := range required {
		if contains(reserved, claim) {
			return fmt.Errorf("reserved jwt claim cannot be required: %s", claim)
		}
		if _, present := claims[claim]; !present {
			return fmt.Errorf("missing jwt claim: %s", claim)
		}
	}
	return nil
}

// jwtKeyFunc returns the verification-key closure that jwt.Parse
// invokes. The dispatch mirrors the python jwt library: a string secret
// is treated as an HMAC key for HS* algorithms, and as a PEM block for
// RS*/ES* algorithms.
func jwtKeyFunc(alg, secret string) (jwt.Keyfunc, error) {
	switch alg {
	case "HS256", "HS384", "HS512":
		return func(_ *jwt.Token) (any, error) { return []byte(secret), nil }, nil
	case "RS256", "RS384", "RS512":
		pub, err := jwt.ParseRSAPublicKeyFromPEM([]byte(secret))
		if err != nil {
			return nil, fmt.Errorf("jwt rsa public key: %s", err.Error())
		}
		return func(_ *jwt.Token) (any, error) { return pub, nil }, nil
	case "ES256", "ES384", "ES512":
		pub, err := jwt.ParseECPublicKeyFromPEM([]byte(secret))
		if err != nil {
			return nil, fmt.Errorf("jwt ec public key: %s", err.Error())
		}
		return func(_ *jwt.Token) (any, error) { return pub, nil }, nil
	}
	return nil, fmt.Errorf("unsupported jwt algorithm: %s", alg)
}

// collectStringSlice accepts the python-shaped `required_claims` value:
// a single string OR a list/tuple/set. Mirrors agent_api.py:1788-1798.
func collectStringSlice(v any) []string {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		return []string{s}
	case []string:
		out := make([]string, 0, len(t))
		for _, s := range t {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	}
	return nil
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
