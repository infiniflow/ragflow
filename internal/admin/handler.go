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

package admin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Common errors
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidToken       = errors.New("invalid token")
)

// Handler admin handler
type Handler struct {
	service *Service
}

// NewHandler create admin handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Health health check
func (h *Handler) Health(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok"})
}

// LoginHTTPRequest login request body
type LoginHTTPRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login handle admin login
func (h *Handler) Login(c *gin.Context) {
	var req LoginHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	svcReq := &LoginRequest{
		Email:    req.Email,
		Password: req.Password,
	}

	resp, err := h.service.Login(svcReq)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			c.JSON(401, gin.H{"error": "invalid credentials"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"token": resp.Token,
		"user": gin.H{
			"id":       resp.UserID,
			"email":    resp.Email,
			"nickname": resp.Nickname,
		},
	})
}

// ListUsers handle list users
func (h *Handler) ListUsers(c *gin.Context) {
	// Parse pagination params
	offset := 0
	limit := 20

	svcReq := &ListUsersRequest{
		Offset: offset,
		Limit:  limit,
	}

	resp, err := h.service.ListUsers(svcReq)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// Convert to response format
	var result []gin.H
	for _, user := range resp.Users {
		result = append(result, gin.H{
			"id":          user.ID,
			"email":       user.Email,
			"nickname":    user.Nickname,
			"is_active":   user.IsActive,
			"create_time": user.CreateTime,
			"update_time": user.UpdateTime,
		})
	}

	c.JSON(200, gin.H{
		"data":  result,
		"total": resp.Total,
	})
}

// GetUser handle get user
func (h *Handler) GetUser(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(400, gin.H{"error": "user id is required"})
		return
	}

	svcReq := &GetUserRequest{ID: id}
	user, err := h.service.GetUser(svcReq)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			c.JSON(404, gin.H{"error": "user not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"id":          user.ID,
		"email":       user.Email,
		"nickname":    user.Nickname,
		"is_active":   user.IsActive,
		"create_time": user.CreateTime,
		"update_time": user.UpdateTime,
	})
}

// UpdateUserHTTPRequest update user request body
type UpdateUserHTTPRequest struct {
	Nickname string  `json:"nickname"`
	IsActive *string `json:"is_active,omitempty"`
}

// UpdateUser handle update user
func (h *Handler) UpdateUser(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(400, gin.H{"error": "user id is required"})
		return
	}

	var req UpdateUserHTTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	svcReq := &UpdateUserRequest{
		ID:       id,
		Nickname: req.Nickname,
		IsActive: req.IsActive,
	}

	if err := h.service.UpdateUser(svcReq); err != nil {
		if errors.Is(err, ErrUserNotFound) {
			c.JSON(404, gin.H{"error": "user not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "user updated"})
}

// DeleteUser handle delete user
func (h *Handler) DeleteUser(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(400, gin.H{"error": "user id is required"})
		return
	}

	svcReq := &DeleteUserRequest{ID: id}
	if err := h.service.DeleteUser(svcReq); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "user deleted"})
}

// GetConfig handle get system config
func (h *Handler) GetConfig(c *gin.Context) {
	config := h.service.GetSystemConfig()
	c.JSON(200, config)
}

// UpdateConfig handle update system config
func (h *Handler) UpdateConfig(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.UpdateSystemConfig(req); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "config updated"})
}

// GetStatus handle get system status
func (h *Handler) GetStatus(c *gin.Context) {
	status := h.service.GetSystemStatus()
	c.JSON(200, status)
}

// AuthMiddleware JWT auth middleware
func (h *Handler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			c.JSON(401, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		// Remove "Bearer " prefix
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}

		// Validate token
		user, err := h.service.ValidateToken(token)
		if err != nil {
			c.JSON(401, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		c.Set("user", user)
		c.Next()
	}
}

// HandleNoRoute handle undefined routes
func (h *Handler) HandleNoRoute(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{
		"error":   "Not Found",
		"message": "The requested resource was not found",
		"path":    c.Request.URL.Path,
	})
}
