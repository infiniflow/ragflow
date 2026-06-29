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
	GetUserByToken(authorization string) (*entity.User, common.ErrorCode, error)
	GetUserByAPIToken(token string) (*entity.User, common.ErrorCode, error)
	GetUserByBetaAPIToken(token string) (*entity.User, common.ErrorCode, error)
}

// NewAuthHandler create auth handler
func NewAuthHandler() *AuthHandler {
	return &AuthHandler{
		userService: service.NewUserService(),
	}
}

// BetaAuthMiddleware resolves a `beta` API token from the Authorization
// header and sets the user on the gin.Context, mirroring Python's
// @login_required(auth_types=AUTH_BETA) used by /chatbots and
// /agentbots route groups.
//
// A beta token can also be a regular user JWT — in that case we
// delegate to the existing AuthMiddleware logic. Order of precedence:
//
//  1. JWT (regular session) → existing UserService.GetUserByToken
//  2. Beta API token          → GetUserByBetaAPIToken
//  3. Fall through            → 401
//
// IMPORTANT: the regular-user branch is NOT gated on a "Bearer "
// prefix. UserService.GetUserByToken accepts the raw Authorization
// header value and ExtractAccessToken handles Bearer stripping
// internally. The existing AuthMiddleware() above also passes the
// raw header to GetUserByToken without pre-filtering, so a non-Bearer
// regular user token must keep working here too.
func (h *AuthHandler) BetaAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" {
			jsonError(c, common.CodeUnauthorized, "Authorization required")
			c.Abort()
			return
		}
		// Try regular user session first (handles JWT, Bearer, or
		// raw access_token — same dispatch as AuthMiddleware()).
		if u, code, err := h.userService.GetUserByToken(auth); err == nil && code == common.CodeSuccess {
			c.Set("user", u)
			c.Next()
			return
		}
		// Fall back to beta API token (public bot access).
		if u, code, err := h.userService.GetUserByBetaAPIToken(auth); err == nil && code == common.CodeSuccess {
			c.Set("user", u)
			c.Next()
			return
		}
		jsonError(c, common.CodeUnauthorized, "Invalid auth credentials")
		c.Abort()
	}
}

// AuthMiddleware JWT auth middleware
// Validates that the user is authenticated and is a superuser (admin)
func (h *AuthHandler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "Missing Authorization header",
			})
			c.Abort()
			return
		}

		authViaAPIToken := false

		// Get user by access token
		user, code, err := h.userService.GetUserByToken(token)
		if err != nil {
			user, code, err = h.userService.GetUserByAPIToken(token)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    code,
					"message": "Invalid access token",
				})
				c.Abort()
				return
			}
			authViaAPIToken = true
		}

		if user.IsSuperuser != nil && *user.IsSuperuser {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    common.CodeForbidden,
				"message": "Super user shouldn't access the URL",
			})
			c.Abort()
			return
		}

		if !local.IsAdminAvailable() {
			license := local.GetAdminStatus()
			errMsg := fmt.Sprintf("server license %s", license.Reason)
			common.Warn(errMsg)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"code":    common.CodeUnauthorized,
				"message": errMsg,
				"data":    "No",
			})
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
