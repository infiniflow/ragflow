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
	"fmt"
	"net/http"
	"net/url"

	"ragflow/internal/common"
	"ragflow/internal/engine/kvrocks"
	"ragflow/internal/entity"
	"ragflow/internal/server"

	"github.com/gin-gonic/gin"
)

// issueUserAccessToken signs the user's access token into the itsdangerous-
// compatible bearer the SPA stores. Shared with the password-login handler.
func issueUserAccessToken(user *entity.User, secretKey string) (string, error) {
	if user == nil || user.AccessToken == nil {
		return "", fmt.Errorf("user access token is missing")
	}
	return common.DumpAccessToken(*user.AccessToken, "", secretKey)
}

// oauthStateCookiePrefix namespaces the per-channel CSRF state cookie set at
// login-initiate time and verified (double-submit) at callback time.
const oauthStateCookiePrefix = "ragflow_oauth_state_"

func oauthStateCookieName(channel string) string {
	return oauthStateCookiePrefix + channel
}

// setOAuthStateCookie stores the opaque CSRF state on the browser that started
// the flow. Unlike the post-login auth cookie this one IS HttpOnly: the SPA
// never needs to read it, and it must survive only the round-trip to the IdP.
func setOAuthStateCookie(c *gin.Context, channel, state string, maxAgeSeconds int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthStateCookieName(channel),
		Value:    state,
		Path:     "/",
		MaxAge:   maxAgeSeconds,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.Request.TLS != nil,
	})
}

func clearOAuthStateCookie(c *gin.Context, channel string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthStateCookieName(channel),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.Request.TLS != nil,
	})
}

// loginErrorURL builds a redirect back to the SPA login page carrying a
// human-readable error. OAuth callbacks are top-level browser navigations, so
// errors must be surfaced as a redirect, not a JSON body.
func loginErrorURL(msg string) string {
	return "/login?error=" + url.QueryEscape(msg)
}

// genericLoginError maps an internal error code to a short, safe message for the
// login redirect. Raw service errors (IdP token-endpoint response bodies, DB
// errors, internal URLs) must never reach the browser URL/history or proxy
// logs — the full error is logged server-side instead.
func genericLoginError(code common.ErrorCode) string {
	switch code {
	case common.CodeAuthenticationError:
		return "login session expired, please try again"
	case common.CodeForbidden:
		return "account is not permitted to sign in"
	default:
		return "login failed"
	}
}

// OAuthLogin starts an SSO login for the channel named in the path by
// redirecting the browser to the provider's authorization endpoint. A CSRF
// state value is both stored server-side (consume-once) and set as an HttpOnly
// cookie for a double-submit check on callback.
func (h *UserHandler) OAuthLogin(c *gin.Context) {
	channel := c.Param("channel")

	init, code, err := h.userService.OAuthLoginInitiate(channel, kvrocks.Get())
	if err != nil {
		common.Error("oauth login initiate failed", err)
		c.Redirect(http.StatusFound, loginErrorURL(genericLoginError(code)))
		return
	}

	setOAuthStateCookie(c, channel, init.State, int(init.CookieMaxAge.Seconds()))
	c.Redirect(http.StatusFound, init.AuthURL)
}

// OAuthChannelCallback completes the SSO flow: it verifies the CSRF state,
// exchanges the authorization code, provisions/updates the user, mints the
// signed access token, and redirects into the SPA with `?auth=<token>` (which
// the frontend root-route loader persists to localStorage) plus the
// ragflow_auth cookie.
func (h *UserHandler) OAuthChannelCallback(c *gin.Context) {
	ctx := c.Request.Context()
	channel := c.Param("channel")

	// The clear must happen regardless of outcome; the state is single-use.
	defer clearOAuthStateCookie(c, channel)

	// The IdP may report an error instead of returning a code.
	if idpErr := c.Query("error"); idpErr != "" {
		if desc := c.Query("error_description"); desc != "" {
			idpErr = idpErr + ": " + desc
		}
		c.Redirect(http.StatusFound, loginErrorURL(idpErr))
		return
	}

	code := c.Query("code")
	state := c.Query("state")
	expectedState, _ := c.Cookie(oauthStateCookieName(channel))

	result, errCode, err := h.userService.OAuthCallback(ctx, channel, code, state, expectedState, kvrocks.Get())
	if err != nil {
		common.Error("oauth callback failed", err)
		c.Redirect(http.StatusFound, loginErrorURL(genericLoginError(errCode)))
		return
	}

	user := result.User
	if user.AccessToken == nil {
		c.Redirect(http.StatusFound, loginErrorURL("login failed: no access token issued"))
		return
	}

	secretKey, err := server.GetSecretKey(ctx, kvrocks.Get())
	if err != nil {
		c.Redirect(http.StatusFound, loginErrorURL("login failed: cannot sign session"))
		return
	}
	authToken, err := issueUserAccessToken(user, secretKey)
	if err != nil {
		c.Redirect(http.StatusFound, loginErrorURL("login failed: cannot sign session"))
		return
	}

	// Same cookie the password login writes, so the SPA bootstraps identically.
	setOAuthAuthCookie(c, authToken)

	// The SPA root-route loader reads `?auth=` and writes it to localStorage,
	// then strips the param and redirects to `/`. No frontend change needed.
	c.Redirect(http.StatusFound, "/?auth="+url.QueryEscape(authToken))
}

// OAuthCallback is the generic (channel-less) callback. Deployments use the
// per-channel route above; this remains a defensive error redirect.
func (h *UserHandler) OAuthCallback(c *gin.Context) {
	c.Redirect(http.StatusFound, loginErrorURL("unsupported oauth callback"))
}

func (h *UserHandler) GitHubAuthCallback(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "GitHubAuthCallback not implemented")
}

func (h *UserHandler) LarkAuthCallback(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "LarkAuthCallback not implemented")
}

func (h *UserHandler) ICBCAuthCallback(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "ICBCAuthCallback not implemented")
}

func (h *UserHandler) AzureAuthCallback(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "AzureAuthCallback not implemented")
}

func (h *UserHandler) AzureAuthLogin(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "AzureAuthLogin not implemented")
}

func (h *UserHandler) Captcha(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "Captcha not implemented")
}

func (h *UserHandler) SendOTP(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "SendOTP not implemented")
}

func (h *UserHandler) VerifyOTP(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "VerifyOTP not implemented")
}

func (h *UserHandler) IsAdmin(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "IsAdmin not implemented")
}

func (h *UserHandler) GetMeta(c *gin.Context) {
	common.ErrorWithCode(c, common.CodeNotImplemented, "GetMeta not implemented")
}

// GetLoginChannels returns the configured SSO channels for the login page.
func (h *UserHandler) GetLoginChannels(c *gin.Context) {
	channels, code, err := h.userService.GetLoginChannels()
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}
	common.SuccessWithData(c, channels, "success")
}
