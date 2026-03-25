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

package contextfs

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler handles HTTP requests for ContextFS
type Handler struct {
	service *Service
}

// NewHandler creates a new ContextFS handler
func NewHandler() *Handler {
	return &Handler{
	service: NewService(),
	}
}

// RegisterRoutes registers ContextFS routes
func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	// ContextFS routes are under /contextfs
	// Router should be /api/v1 to produce /api/v1/contextfs/*
	contextfs := router.Group("/contextfs")
	{
		contextfs.GET("/ls", h.List)
		contextfs.POST("/search", h.Search)
	}
}

// List handles the ls command
func (h *Handler) List(c *gin.Context) {
	var req ListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
	c.JSON(http.StatusBadRequest, ListResponse{
		Code:    -1,
		Message: "Invalid request: " + err.Error(),
	})
	return
	}

	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("user_id")
	if !exists {
	c.JSON(http.StatusUnauthorized, ListResponse{
		Code:    -1,
		Message: "Unauthorized",
	})
	return
	}

	// Default to root path
	if req.Path == "" {
	req.Path = "/"
	}

	nodes, err := h.service.List(c.Request.Context(), userID.(string), req.Path)
	if err != nil {
	c.JSON(http.StatusInternalServerError, ListResponse{
		Code:    -1,
		Message: err.Error(),
	})
	return
	}

	c.JSON(http.StatusOK, ListResponse{
	Code:    0,
	Data:    nodes,
	Message: "Success",
	})
}

// Search handles the search command
func (h *Handler) Search(c *gin.Context) {
	var opts SearchOptions
	if err := c.ShouldBindJSON(&opts); err != nil {
	c.JSON(http.StatusBadRequest, SearchResponse{
		Code:    -1,
		Message: "Invalid request: " + err.Error(),
	})
	return
	}

	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
	c.JSON(http.StatusUnauthorized, SearchResponse{
		Code:    -1,
		Message: "Unauthorized",
	})
	return
	}
	opts.UserID = userID.(string)

	results, total, err := h.service.Search(c.Request.Context(), opts)
	if err != nil {
	c.JSON(http.StatusInternalServerError, SearchResponse{
		Code:    -1,
		Message: err.Error(),
	})
	return
	}

	c.JSON(http.StatusOK, SearchResponse{
	Code:    0,
	Data:    results,
	Total:   total,
	Message: "Success",
	})
}
