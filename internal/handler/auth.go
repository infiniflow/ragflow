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
	"ragflow/internal/permission"
	"ragflow/internal/server/local"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

// AuthHandler auth handler
type AuthHandler struct {
	userService userTokenResolver
}

// ResolveMCPUser applies the same API-key and user-token resolution used by
// the public MCP route, for the standalone native MCP transport.
func (h *AuthHandler) ResolveMCPUser(ctx context.Context, authorization string) (*entity.User, error) {
	if authorization == "" {
		return nil, fmt.Errorf("missing authorization header")
	}
	if identity, code, err := h.userService.ResolveAccessToken(ctx, authorization); err == nil && code == common.CodeSuccess {
		return identity.User, nil
	}
	if identity, code, err := h.userService.ResolveAPIToken(ctx, authorization); err == nil && code == common.CodeSuccess {
		return identity.User, nil
	}
	if identity, code, err := h.userService.ResolveBetaAPIToken(ctx, authorization); err == nil && code == common.CodeSuccess {
		return identity.User, nil
	}
	return nil, fmt.Errorf("invalid authorization")
}

// userTokenResolver is the subset of UserService the auth
// middleware actually depends on. We keep it as a small interface
// so the test suite can swap in a stub without spinning up the
// full UserService (which requires a live Redis + token signing key).
type userTokenResolver interface {
	ResolveAccessToken(ctx context.Context, authorization string) (*service.AuthIdentity, common.ErrorCode, error)
	ResolveAPIToken(ctx context.Context, token string) (*service.AuthIdentity, common.ErrorCode, error)
	ResolveBetaAPIToken(ctx context.Context, token string) (*service.AuthIdentity, common.ErrorCode, error)
}

// NewAuthHandler create auth handler
func NewAuthHandler() *AuthHandler {
	return &AuthHandler{
		userService: service.NewUserService(),
	}
}

// BetaAuthMiddleware resolves a user token, API token, or beta API token
// from the Authorization header and sets the user on the gin.Context.
//
// A beta endpoint also accepts user and regular API tokens. Order of
// precedence:
//
//  1. User token             → ResolveAccessToken
//  2. API token              → ResolveAPIToken
//  3. Beta API token         → ResolveBetaAPIToken
//  4. Fall through           → code 102 "Authorization is not valid!"
//
// The user-token branch accepts either a raw token or a Bearer token.
func (h *AuthHandler) BetaAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		auth := c.GetHeader("Authorization")
		if auth == "" {
			if cookie, err := c.Cookie(authCookie); err == nil {
				auth = cookie
			}
		}

		if auth == "" {
			// Mirror Python's login_required(auth_types=AUTH_BETA): any auth
			// failure on beta endpoints is a business error, not an HTTP 401.
			common.ResponseWithCodeData(c, common.CodeDataError, nil, "Authorization is not valid!")
			c.Abort()
			return
		}
		if identity, code, err := h.userService.ResolveAccessToken(ctx, auth); err == nil && code == common.CodeSuccess {
			setAuthIdentity(c, identity)
			c.Next()
			return
		}
		// Then try a regular API token (non-beta public bot flow).
		if identity, code, err := h.userService.ResolveAPIToken(ctx, auth); err == nil && code == common.CodeSuccess {
			setAuthIdentity(c, identity)
			c.Set("auth_via_api_token", true)
			c.Next()
			return
		}
		// Fall back to beta API token (public bot access).
		if identity, code, err := h.userService.ResolveBetaAPIToken(ctx, auth); err == nil && code == common.CodeSuccess {
			setAuthIdentity(c, identity)
			c.Set("auth_via_beta_token", true)
			if identity.APIToken != nil && identity.APIToken.DialogID != nil {
				c.Set("agent_id", *identity.APIToken.DialogID)
			}
			c.Next()
			return
		}
		// Mirror Python's login_required(auth_types=AUTH_BETA).
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "Authorization is not valid!")
		c.Abort()
	}
}

// AuthMiddleware authenticates API requests and rejects system admins from
// using the tenant API surface.
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

		identity, code, err := h.userService.ResolveAccessToken(ctx, token)
		if err != nil {
			identity, code, err = h.userService.ResolveAPIToken(ctx, token)
			if err != nil {
				common.ResponseWithHttpCodeData(c, http.StatusUnauthorized, code, nil, "Invalid access token")
				c.Abort()
				return
			}
			authViaAPIToken = true
		}

		if identity.User.IsSuperuser != nil && *identity.User.IsSuperuser {
			common.ResponseWithHttpCodeData(c, http.StatusForbidden, common.CodeForbidden, nil, "Super user shouldn't access the URL")
			c.Abort()
			return
		}

		if !local.IsAdminAvailable() {
			adminStatus := local.GetAdminStatus()
			errMsg := fmt.Sprintf("server error: %s", adminStatus.Reason)
			common.Warn(errMsg)
			common.ResponseWithHttpCodeData(c, http.StatusServiceUnavailable, common.CodeUnauthorized, "No", errMsg)
			c.Abort()
			return
		}

		setAuthIdentity(c, identity)
		c.Set("email", identity.User.Email)
		c.Set("auth_via_api_token", authViaAPIToken)
		c.Next()
	}
}

func setAuthIdentity(c *gin.Context, identity *service.AuthIdentity) {
	c.Set("user", identity.User)
	c.Set("user_id", identity.User.ID)
	c.Set("tenant_id", identity.TenantID)
	c.Request = c.Request.WithContext(permission.WithSubject(c.Request.Context(), permission.Subject{
		UserID:   identity.User.ID,
		TenantID: identity.TenantID,
	}))
}
