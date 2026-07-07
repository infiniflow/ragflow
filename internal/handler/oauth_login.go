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
	"errors"
	"fmt"
	"net/http"
	"ragflow/internal/engine/redis"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/server"
	"ragflow/internal/service"
	"ragflow/internal/utility"
)

// oauthStateCookie is the HttpOnly cookie name that ties the in-flight
// state token to the browser that initiated the flow. The handler reads
// it back from the callback request to defend against CSRF in addition to
// the Redis-side verification.
const oauthStateCookie = "ragflow_oauth_state"

// oauthAuthCookie is the cookie the callback writes on success, so the SPA
// can pick up the signed access token after the redirect. The frontend
// reads it and either re-issues the value as an Authorization header on
// subsequent API calls or hands it off to its own token store. Not
// HttpOnly so the SPA's JS can read it.
const oauthAuthCookie = "ragflow_auth"

// OAuthLogin starts an OAuth/OIDC login flow for the configured channel.
// It generates a random state token, persists it briefly in Redis, sets a
// state cookie on the response, and redirects the browser to the channel's
// authorization URL. Mirrors Python's GET /auth/login/<channel>.
//
// @Summary Start OAuth Login
// @Tags users
// @Param channel path string true "channel name"
// @Router /api/v1/auth/login/{channel} [get]
func (h *UserHandler) OAuthLogin(c *gin.Context) {
	channel := c.Param("channel")
	if channel == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, "channel is required")
		return
	}

	init, code, err := h.userService.OAuthLoginInitiate(channel, redis.Get())
	if err != nil {
		// Mirror Python's oauth_login: the raised ValueError propagates to
		// server_error_response, which replies HTTP 200 with code 100 and
		// the exception's repr() as the message (no short error code).
		if errors.Is(err, service.ErrOAuthInvalidChannel) {
			common.ResponseWithCodeData(c, common.CodeExceptionError, nil, fmt.Sprintf("ValueError('Invalid channel name: %s')", channel))
			return
		}
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, code, nil, err.Error())
		return
	}

	setOAuthStateCookie(c, init.State, int(init.CookieMaxAge.Seconds()))
	c.Redirect(http.StatusFound, init.AuthURL)
}

// OAuthCallback handles the OAuth/OIDC callback for the configured channel.
// Mirrors Python's GET /auth/oauth/<channel>/callback: it verifies the
// state, exchanges the code for an access token, fetches user info, and
// then either logs in an existing user or registers a new one. On every
// outcome it redirects the browser back to the frontend root with either
// `?auth=<user_id>` or `?error=<code>` so the SPA can show the right page.
//
// @Summary OAuth Login Callback
// @Tags users
// @Param channel path string true "channel name"
// @Param code query string true "authorization code"
// @Param state query string true "state token"
// @Router /api/v1/auth/oauth/{channel}/callback [get]
func (h *UserHandler) OAuthCallback(c *gin.Context) {
	channel := c.Param("channel")
	// An empty channel segment (/auth/oauth//callback) is a malformed path,
	// not a real channel. Python's router never matches it and returns 404;
	// match that here instead of flowing into the callback and emitting a
	// bogus "Invalid channel name:" redirect.
	if channel == "" {
		HandleNoRoute(c)
		return
	}
	queryCode := c.Query("code")
	queryState := c.Query("state")
	cookieState := readOAuthStateCookie(c)
	clearOAuthStateCookie(c)

	frontendBase := frontendRedirectBase()

	result, _, err := h.userService.OAuthCallback(c.Request.Context(), channel, queryCode, queryState, cookieState, redis.Get())
	if err != nil {
		c.Redirect(http.StatusFound, frontendBase+"?error="+callbackError(channel, err))
		return
	}

	secretKey, kerr := server.GetSecretKey(redis.Get())
	if kerr != nil {
		c.Redirect(http.StatusFound, frontendBase+"?error=server_error")
		return
	}
	authToken, terr := utility.DumpAccessToken(*result.User.AccessToken, secretKey)
	if terr != nil {
		c.Redirect(http.StatusFound, frontendBase+"?error=server_error")
		return
	}

	setOAuthAuthCookie(c, authToken)
	c.Header("Authorization", authToken)
	c.Header("Access-Control-Expose-Headers", "Authorization")
	c.Redirect(http.StatusFound, frontendBase+"?auth="+result.User.ID)
}

// callbackError maps the OAuth callback errors to the `?error=` strings
// Python's oauth_callback emits. Python redirects with `?error={str(e)}`,
// so an invalid channel surfaces the full "Invalid channel name: <channel>"
// message (str of the ValueError), while the other failures use the short
// tokens Python hard-codes. The value is intentionally not URL-encoded to
// match Python's raw f-string redirect.
func callbackError(channel string, err error) string {
	switch {
	case errors.Is(err, service.ErrOAuthInvalidChannel):
		return "Invalid channel name: " + channel
	case errors.Is(err, service.ErrOAuthInvalidState):
		return "invalid_state"
	case errors.Is(err, service.ErrOAuthMissingCode):
		return "missing_code"
	case errors.Is(err, service.ErrOAuthTokenFailed):
		return "token_failed"
	case errors.Is(err, service.ErrOAuthEmailMissing):
		return "email_missing"
	case errors.Is(err, service.ErrOAuthUserInactive):
		return "user_inactive"
	default:
		return "server_error"
	}
}

// setOAuthStateCookie writes the state token as an HttpOnly cookie scoped
// to the API host. SameSite=Lax keeps the cookie attached on the top-level
// navigation that brings the user back to the callback.
func setOAuthStateCookie(c *gin.Context, state string, maxAgeSec int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    state,
		Path:     "/",
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.Request.TLS != nil,
	})
}

func readOAuthStateCookie(c *gin.Context) string {
	if cookie, err := c.Request.Cookie(oauthStateCookie); err == nil {
		return cookie.Value
	}
	return ""
}

func clearOAuthStateCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.Request.TLS != nil,
	})
}

// setOAuthAuthCookie writes the signed access token so the SPA can pick it
// up after the redirect. Not HttpOnly so the SPA can copy it into its
// Authorization header on subsequent fetches. Lifetime mirrors the
// access-token TTL used by the rest of the app.
func setOAuthAuthCookie(c *gin.Context, token string) {
	// the SPA's bootstrap credential after the OAuth redirect. The
	// SPA reads it via document.cookie and copies it into the
	// Authorization header. Setting HttpOnly would break the login
	// flow. The token is short-lived (7 days) and signed with itsdangerous.
	// codeql[go/cookie-httponly-not-set] Intentional: this cookie is
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthAuthCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 7,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.Request.TLS != nil,
	})
}

// frontendRedirectBase returns the URL prefix the OAuth callback should
// redirect back to. Mirrors Python's oauth_callback, which always issues
// relative "/?auth=..." / "/?error=..." redirects so the browser stays on
// the same origin that served the SPA.
func frontendRedirectBase() string {
	return "/"
}
