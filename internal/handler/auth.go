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
	"ragflow/internal/logger"
	"ragflow/internal/server/local"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

// AuthHandler auth handler
type AuthHandler struct {
	userService *service.UserService
}

// NewAuthHandler create auth handler
func NewAuthHandler() *AuthHandler {
	return &AuthHandler{
		userService: service.NewUserService(),
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
		}

		if *user.IsSuperuser {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    common.CodeForbidden,
				"message": "Super user shouldn't access the URL",
			})
			return
		}

		if !local.IsAdminAvailable() {
			license := local.GetAdminStatus()
			errMsg := fmt.Sprintf("server license %s", license.Reason)
			logger.Warn(errMsg)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"code":    common.CodeUnauthorized,
				"message": errMsg,
				"data":    "No",
			})
			return
		}

		c.Set("user", user)
		c.Set("user_id", user.ID)
		c.Set("email", user.Email)
		c.Next()
	}
}
