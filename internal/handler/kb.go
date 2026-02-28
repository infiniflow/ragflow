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
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

// KnowledgebaseHandler knowledge base handler
type KnowledgebaseHandler struct {
	kbService   *service.KnowledgebaseService
	userService *service.UserService
}

// NewKnowledgebaseHandler create knowledge base handler
func NewKnowledgebaseHandler(kbService *service.KnowledgebaseService, userService *service.UserService) *KnowledgebaseHandler {
	return &KnowledgebaseHandler{
		kbService:   kbService,
		userService: userService,
	}
}

// ListKbs list knowledge bases
// @Summary List Knowledge Bases
// @Description Get list of knowledge bases with filtering and pagination
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Param keywords query string false "search keywords"
// @Param page query int false "page number"
// @Param page_size query int false "items per page"
// @Param parser_id query string false "parser ID filter"
// @Param orderby query string false "order by field"
// @Param desc query bool false "descending order"
// @Param request body service.ListKbsRequest true "filter options"
// @Success 200 {object} service.ListKbsResponse
// @Router /v1/kb/list [post]
func (h *KnowledgebaseHandler) ListKbs(c *gin.Context) {
	// Parse request body - allow empty body
	var req service.ListKbsRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": err.Error(),
			})
			return
		}
	}

	// Extract parameters from query or request body with defaults
	keywords := ""
	if req.Keywords != nil {
		keywords = *req.Keywords
	} else if queryKeywords := c.Query("keywords"); queryKeywords != "" {
		keywords = queryKeywords
	}

	page := 0
	if req.Page != nil {
		page = *req.Page
	} else if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 0
	if req.PageSize != nil {
		pageSize = *req.PageSize
	} else if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
			pageSize = ps
		}
	}

	parserID := ""
	if req.ParserID != nil {
		parserID = *req.ParserID
	} else if queryParserID := c.Query("parser_id"); queryParserID != "" {
		parserID = queryParserID
	}

	orderby := "update_time"
	if req.Orderby != nil {
		orderby = *req.Orderby
	} else if queryOrderby := c.Query("orderby"); queryOrderby != "" {
		orderby = queryOrderby
	}

	desc := true
	if req.Desc != nil {
		desc = *req.Desc
	} else if descStr := c.Query("desc"); descStr != "" {
		desc = descStr == "true"
	}

	var ownerIDs []string
	if req.OwnerIDs != nil {
		ownerIDs = *req.OwnerIDs
	}

	// Get access token from Authorization header
	token := c.GetHeader("Authorization")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Missing Authorization header",
		})
		return
	}

	// Get user by access token
	user, err := h.userService.GetUserByToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Invalid access token",
		})
		return
	}
	userID := user.ID

	// List knowledge bases
	result, err := h.kbService.ListKbs(keywords, page, pageSize, parserID, orderby, desc, ownerIDs, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    result,
		"message": "success",
	})
}
