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

// ListSearches list search apps
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
// @Router /api/v1/searches [post]
func (h *SearchHandler) ListSearches(c *gin.Context) {
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
	result, err := h.searchService.ListSearches(userID, keywords, page, pageSize, orderby, desc, req.OwnerIDs)
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

// CreateSearch create a new search app
// @Summary Create Search App
// @Description Create a new search app for the current user
// @Tags search
// @Accept json
// @Produce json
// @Param request body service.CreateSearchRequest true "search creation parameters"
// @Success 200 {object} service.CreateSearchResponse
// @Router /api/v1/searches [post]

type CreateSearchRequest struct {
	Name        string  `json:"name" binding:"required"` // required field, max 255 bytes
	Description *string `json:"description,omitempty"`   // optional description
}

func (h *SearchHandler) CreateSearch(c *gin.Context) {
	// Get current user from context (same as Python current_user)
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// Parse request body (same as Python get_request_json())
	var req CreateSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"data":    nil,
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}

	if err := common.ValidateName(req.Name); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	// Create search (same as Python SearchService.save within DB.atomic())
	result, err := h.searchService.CreateSearch(userID, req.Name, req.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    common.CodeServerError,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	// Return success response (same as Python get_json_result(data={"search_id": req["id"]}))
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    result,
		"message": "success",
	})
}

// GetSearch get search app detail
// @Summary Get Search App Detail
// @Description Get detail of a search app by ID
// @Tags search
// @Accept json
// @Produce json
// @Param search_id path string true "search app ID"
// @Success 200 {object} entity.Search
// @Router /api/v1/searches/{search_id} [get]
func (h *SearchHandler) GetSearch(c *gin.Context) {
	// Get current user from context (same as Python current_user)
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// Get search_id from path parameter (same as Python <search_id>)
	searchID := c.Param("search_id")
	if searchID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"data":    nil,
			"message": "search_id is required",
		})
		return
	}

	// Get search detail with permission check
	search, err := h.searchService.GetSearchDetail(userID, searchID)
	if err != nil {
		// Check if it's a permission error
		if err.Error() == "has no permission for this operation" {
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeOperatingError,
				"data":    false,
				"message": "Has no permission for this operation.",
			})
			return
		}
		// Not found error
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	// Convert to response format (same as Python get_json_result(data=search))
	result := map[string]interface{}{
		"id":            search.ID,
		"tenant_id":     search.TenantID,
		"name":          search.Name,
		"description":   search.Description,
		"created_by":    search.CreatedBy,
		"create_time":   search.CreateTime,
		"update_time":   search.UpdateTime,
		"search_config": search.SearchConfig,
	}

	if search.Avatar != nil {
		result["avatar"] = *search.Avatar
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    result,
		"message": "success",
	})
}

// DeleteSearch delete a search app
// @Summary Delete Search App
// @Description Delete a search app by ID
// @Tags search
// @Accept json
// @Produce json
// @Param search_id path string true "search app ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/searches/{search_id} [delete]
func (h *SearchHandler) DeleteSearch(c *gin.Context) {
	// Get current user from context (same as Python current_user)
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// Get search_id from path parameter (same as Python <search_id>)
	searchID := c.Param("search_id")
	if searchID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"data":    nil,
			"message": "search_id is required",
		})
		return
	}

	// Delete search with permission check
	err := h.searchService.DeleteSearch(userID, searchID)
	if err != nil {
		// Check if it's an authorization error
		if err.Error() == "no authorization" {
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeAuthenticationError,
				"data":    false,
				"message": "No authorization.",
			})
			return
		}
		// Delete failed error
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	// Return success response (same as Python get_json_result(data=True))
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    true,
		"message": "success",
	})
}

// UpdateSearch update a search app
// @Summary Update Search App
// @Description Update a search app by ID
// @Tags search
// @Accept json
// @Produce json
// @Param search_id path string true "search app ID"
// @Param request body service.UpdateSearchRequest true "search update parameters"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/searches/{search_id} [put]
func (h *SearchHandler) UpdateSearch(c *gin.Context) {
	// Get current user from context (same as Python current_user)
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// Get search_id from path parameter (same as Python <search_id>)
	searchID := c.Param("search_id")
	if searchID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"data":    nil,
			"message": "search_id is required",
		})
		return
	}

	// Parse request body
	var req service.UpdateSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"data":    nil,
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}

	// Validate name (same as Python validation)
	if err := common.ValidateName(req.Name); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	// Update search
	updatedSearch, err := h.searchService.UpdateSearch(userID, searchID, &req)
	if err != nil {
		errMsg := err.Error()
		switch errMsg {
		case "no authorization":
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeAuthenticationError,
				"data":    false,
				"message": "No authorization.",
			})
		case "duplicated search name":
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeDataError,
				"data":    nil,
				"message": "Duplicated search name.",
			})
		default:
			// Check if it's a "cannot find search" error
			if len(errMsg) > 18 && errMsg[:18] == "cannot find search" {
				c.JSON(http.StatusOK, gin.H{
					"code":    common.CodeDataError,
					"data":    false,
					"message": errMsg,
				})
			} else {
				c.JSON(http.StatusOK, gin.H{
					"code":    common.CodeDataError,
					"data":    nil,
					"message": errMsg,
				})
			}
		}
		return
	}

	// Convert to response format (same as Python updated_search.to_dict())
	result := map[string]interface{}{
		"id":            updatedSearch.ID,
		"tenant_id":     updatedSearch.TenantID,
		"name":          updatedSearch.Name,
		"description":   updatedSearch.Description,
		"created_by":    updatedSearch.CreatedBy,
		"status":        updatedSearch.Status,
		"create_time":   updatedSearch.CreateTime,
		"update_time":   updatedSearch.UpdateTime,
		"search_config": updatedSearch.SearchConfig,
	}

	if updatedSearch.Avatar != nil {
		result["avatar"] = *updatedSearch.Avatar
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    result,
		"message": "success",
	})
}
