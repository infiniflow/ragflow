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
	"context"
	"fmt"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/server/local"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

// AuthHandler auth handler
type AuthHandler struct {
	userService userTokenResolver
}

// userTokenResolver is the subset of UserService the auth
// middleware actually depends on. We keep it as a small interface
// so the test suite can swap in a stub without spinning up the
// full UserService (which requires a live Redis + JWT secret).
type userTokenResolver interface {
	GetUserByToken(ctx context.Context, authorization string) (*entity.User, common.ErrorCode, error)
	GetUserByAPIToken(ctx context.Context, token string) (*entity.User, common.ErrorCode, error)
	GetUserByBetaAPIToken(ctx context.Context, token string) (*entity.User, common.ErrorCode, error)
	GetAPITokenByBeta(ctx context.Context, authorization string) (*entity.APIToken, error)
}

// NewAuthHandler create auth handler
func NewAuthHandler() *AuthHandler {
	return &AuthHandler{
		userService: service.NewUserService(),
	}
}

// BetaAuthMiddleware resolves a user token, API token, or `beta` API token
// from the Authorization header and sets the user on the gin.Context.
//
// A beta token can also be a regular user JWT or API token. Order of
// precedence:
//
//  1. Beta API token         → GetUserByBetaAPIToken
//  2. JWT (regular session) → existing UserService.GetUserByToken
//  3. API token              → GetUserByAPIToken
//  4. Fall through           → 401
//
// IMPORTANT: the regular-user branch is NOT gated on a "Bearer "
// prefix. UserService.GetUserByToken accepts the raw Authorization
// header value and ExtractAccessToken handles Bearer stripping
// internally. The existing AuthMiddleware() above also passes the
// raw header to GetUserByToken without pre-filtering, so a non-Bearer
// regular user token must keep working here too.
func (h *AuthHandler) BetaAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		auth := c.GetHeader("Authorization")
		if auth == "" {
			if cookie, err := c.Cookie(oauthAuthCookie); err == nil {
				auth = cookie
			}
		}

		if auth == "" {
			common.ResponseWithCodeData(c, common.CodeUnauthorized, nil, "Authorization required")
			c.Abort()
			return
		}
		// AUTH_JWT
		if u, code, err := h.userService.GetUserByToken(ctx, auth); err == nil && code == common.CodeSuccess {
			c.Set("user", u)
			c.Next()
			return
		}
		// Then try a regular API token (non-beta public bot flow).
		if u, code, err := h.userService.GetUserByAPIToken(ctx, auth); err == nil && code == common.CodeSuccess {
			c.Set("user", u)
			c.Set("auth_via_api_token", true)
			c.Next()
			return
		}
		// Fall back to beta API token (public bot access).
		if u, code, err := h.userService.GetUserByBetaAPIToken(ctx, auth); err == nil && code == common.CodeSuccess {
			c.Set("user", u)
			if tok, terr := h.userService.GetAPITokenByBeta(ctx, auth); terr == nil && tok != nil && tok.DialogID != nil {
				c.Set("agent_id", *tok.DialogID)
			}
			c.Next()
			return
		}
		common.ResponseWithCodeData(c, common.CodeUnauthorized, nil, "Invalid auth credentials")
		c.Abort()
	}
}

// AuthMiddleware JWT auth middleware
// Validates that the user is authenticated and is a superuser (admin)
func (h *AuthHandler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		token := c.GetHeader("Authorization")
		if token == "" {
			common.ResponseWithHttpCodeData(c, http.StatusUnauthorized, 401, nil, "Missing Authorization header")
			c.Abort()
			return
		}

		authViaAPIToken := false

		// Get user by access token
		user, code, err := h.userService.GetUserByToken(ctx, token)
		if err != nil {
			user, code, err = h.userService.GetUserByAPIToken(ctx, token)
			if err != nil {
				common.ResponseWithHttpCodeData(c, http.StatusUnauthorized, code, nil, "Invalid access token")
				c.Abort()
				return
			}
			authViaAPIToken = true
		}

		if user.IsSuperuser != nil && *user.IsSuperuser {
			common.ResponseWithHttpCodeData(c, http.StatusForbidden, common.CodeForbidden, nil, "Super user shouldn't access the URL")
			c.Abort()
			return
		}

		if !local.IsAdminAvailable() {
			license := local.GetAdminStatus()
			errMsg := fmt.Sprintf("server license %s", license.Reason)
			common.Warn(errMsg)
			common.ResponseWithHttpCodeData(c, http.StatusServiceUnavailable, common.CodeUnauthorized, "No", errMsg)
			c.Abort()
			return
		}

		c.Set("user", user)
		c.Set("user_id", user.ID)
		c.Set("email", user.Email)
		c.Set("auth_via_api_token", authViaAPIToken)
		c.Next()
	}
}
