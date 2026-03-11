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
	"ragflow/internal/common"
	"strconv"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

// SearchHandler search handler
type SearchHandler struct {
	searchService *service.SearchService
	userService   *service.UserService
}

// NewSearchHandler create search handler
func NewSearchHandler(searchService *service.SearchService, userService *service.UserService) *SearchHandler {
	return &SearchHandler{
		searchService: searchService,
		userService:   userService,
	}
}

// ListSearchApps list search apps
// @Summary List Search Apps
// @Description Get list of search apps for the current user with filtering, pagination and sorting
// @Tags search
// @Accept json
// @Produce json
// @Param keywords query string false "search keywords"
// @Param page query int false "page number"
// @Param page_size query int false "items per page"
// @Param orderby query string false "order by field (default: create_time)"
// @Param desc query bool false "descending order (default: true)"
// @Param request body service.ListSearchAppsRequest true "filter options including owner_ids"
// @Success 200 {object} service.ListSearchAppsResponse
// @Router /v1/search/list [post]
func (h *SearchHandler) ListSearchApps(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// Parse query parameters
	keywords := c.Query("keywords")

	page := 0
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 0
	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
			pageSize = ps
		}
	}

	orderby := c.DefaultQuery("orderby", "create_time")

	desc := true
	if descStr := c.Query("desc"); descStr != "" {
		desc = descStr != "false"
	}

	// Parse request body for owner_ids
	var req service.ListSearchAppsRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": err.Error(),
			})
			return
		}
	}

	// List search apps with filtering
	result, err := h.searchService.ListSearchApps(userID, keywords, page, pageSize, orderby, desc, req.OwnerIDs)
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
