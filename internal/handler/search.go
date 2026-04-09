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
// Reference: api/apps/restful_apis/search_api.py::create
// Python implementation details:
// - Route: @manager.route("/searches", methods=["POST"])
// - Auth: @login_required
// - Validation: @validate_request("name") - ensures 'name' field exists in request
// - Request parsing: req = await get_request_json()
// - Field extraction: search_name = req["name"], description = req.get("description", "")
//
// Validation logic from Python:
// 1. Name must be string: isinstance(search_name, str)
// 2. Name cannot be empty/whitespace: search_name.strip() == ""
// 3. Name max 255 bytes: len(search_name.encode("utf-8")) > 255
// 4. User tenant must exist: TenantService.get_by_id(current_user.id)
//
// Processing logic from Python:
// 1. Trim and deduplicate name: duplicate_name(SearchService.query, name, tenant_id)
// 2. Generate UUID: get_uuid()
// 3. Set fields: id, name, description, tenant_id, created_by
// 4. Atomic save: with DB.atomic(): SearchService.save(**req)
// 5. Return: {"search_id": req["id"]}
//
// Error responses from Python:
// - Name type error: get_data_error_result(message="Search name must be string.")
// - Name empty: get_data_error_result(message="Search name can't be empty.")
// - Name too long: get_data_error_result(message=f"Search name length is {len} which is large than 255.")
// - Tenant auth error: get_data_error_result(message="Authorized identity.")
// - Save error: get_data_error_result() or server_error_response(e)
//
// Go implementation mirrors this flow:
// 1. Extract user from context (via GetUser, same as Python current_user)
// 2. Bind and validate JSON request
// 3. Validate request parameters (name not empty, max 255 bytes)
// 4. Validate user tenant exists
// 5. Call service to create search
// 6. Return success response with search_id
//
// Note: Similar handler patterns in:
// - CreateMemory (memory.go) - same validation flow
// - CreateDataset (datasets.go) - similar deduplication logic

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
