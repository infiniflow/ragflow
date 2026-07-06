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
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

// SearchHandler search handler
type SearchHandler struct {
	searchService *service.SearchService
	userService   *service.UserService
	streamLLM     *service.ModelProviderService
	askService    *service.AskService
	sseWriter     SSEWriter
}

// NewSearchHandler create search handler
func NewSearchHandler(searchService *service.SearchService, userService *service.UserService) *SearchHandler {
	return &SearchHandler{
		searchService: searchService,
		userService:   userService,
		sseWriter:     &ginSSEWriter{},
	}
}

// SetCompletionDependencies wires the streaming search completion runtime.
func (h *SearchHandler) SetCompletionDependencies(streamLLM *service.ModelProviderService, askService *service.AskService) {
	h.streamLLM = streamLLM
	h.askService = askService
}

func getSearchOwnerIDs(c *gin.Context) []string {
	values := c.QueryArray("owner_ids")
	if len(values) == 0 {
		values = c.QueryArray("owner_id")
	}
	ownerIDs := make([]string, 0, len(values))
	for _, value := range values {
		for _, ownerID := range strings.Split(value, ",") {
			ownerID = strings.TrimSpace(ownerID)
			if ownerID != "" {
				ownerIDs = append(ownerIDs, ownerID)
			}
		}
	}
	return ownerIDs
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
// @Param owner_ids query []string false "owner IDs"
// @Success 200 {object} service.ListSearchAppsResponse
// @Router /api/v1/searches [get]
func (h *SearchHandler) ListSearches(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
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

	ownerIDs := getSearchOwnerIDs(c)

	// Keep body parsing as a compatibility fallback for existing callers that
	// send owner_ids in a GET body. Python reads owner_ids from the query.
	var req service.ListSearchAppsRequest
	if len(ownerIDs) == 0 && c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 400, nil, err.Error())
			return
		}
		ownerIDs = req.OwnerIDs
	}

	// List search apps with filtering
	result, err := h.searchService.ListSearches(userID, keywords, page, pageSize, orderby, desc, ownerIDs)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, 500, nil, err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}
	userID := user.ID

	// Parse request body (same as Python get_request_json())
	var req CreateSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, common.CodeBadRequest, nil, "Invalid request body: "+err.Error())
		return
	}

	if err := common.ValidateName(req.Name); err != nil {
		common.ErrorWithCode(c, int(common.CodeDataError), err.Error())
		return
	}

	// Create search (same as Python SearchService.save within DB.atomic())
	result, err := h.searchService.CreateSearch(userID, req.Name, req.Description)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, common.CodeBadRequest, nil, err.Error())
		return
	}

	// Return success response (same as Python get_json_result(data={"search_id": req["id"]}))
	common.SuccessWithData(c, result, "success")
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}
	userID := user.ID

	// Get search_id from path parameter (same as Python <search_id>)
	searchID := c.Param("search_id")
	if searchID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "search_id is required")
		return
	}

	// Get search detail with permission check
	search, err := h.searchService.GetSearchDetail(userID, searchID)
	if err != nil {
		// Check if it's a permission error
		if err.Error() == "has no permission for this operation" {
			common.ResponseWithCodeData(c, common.CodeOperatingError, false, "Has no permission for this operation.")
			return
		}
		// Not found error
		common.ErrorWithCode(c, int(common.CodeDataError), err.Error())
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
	common.SuccessWithData(c, result, "success")
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}
	userID := user.ID

	// Get search_id from path parameter (same as Python <search_id>)
	searchID := c.Param("search_id")
	if searchID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "search_id is required")
		return
	}

	// Delete search with permission check
	err := h.searchService.DeleteSearch(userID, searchID)
	if err != nil {
		// Check if it's an authorization error
		if err.Error() == "no authorization" {
			common.ResponseWithCodeData(c, common.CodeDataError, false, "No authorization")
			return
		}
		// Delete failed error
		common.ErrorWithCode(c, int(common.CodeDataError), err.Error())
		return
	}

	// Return success response (same as Python get_json_result(data=True))
	common.SuccessWithData(c, true, "success")
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}
	userID := user.ID

	// Get search_id from path parameter (same as Python <search_id>)
	searchID := c.Param("search_id")
	if searchID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "search_id is required")
		return
	}

	// Parse request body
	var req service.UpdateSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Invalid request body: "+err.Error())
		return
	}

	// Validate name (same as Python validation)
	if err := common.ValidateName(req.Name); err != nil {
		common.ErrorWithCode(c, int(common.CodeDataError), err.Error())
		return
	}

	// Update search
	updatedSearch, err := h.searchService.UpdateSearch(userID, searchID, &req)
	if err != nil {
		errMsg := err.Error()
		switch errMsg {
		case "no authorization":
			common.ResponseWithCodeData(c, common.CodeDataError, false, "No authorization")
		case "duplicated search name":
			common.ResponseWithCodeData(c, common.CodeDataError, nil, "Duplicated search name.")
		default:
			// Check if it's a "cannot find search" error
			if len(errMsg) > 18 && errMsg[:18] == "cannot find search" {
				common.ResponseWithCodeData(c, common.CodeDataError, false, errMsg)
			} else {
				common.ResponseWithCodeData(c, common.CodeDataError, nil, errMsg)
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
	common.SuccessWithData(c, result, "success")
}

func (h *SearchHandler) Completion(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req service.SearchCompletionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
			"question is required")
		return
	}

	searchSvc := h.searchService
	if searchSvc == nil {
		searchSvc = service.NewSearchService()
	}

	plan, code, err := searchSvc.PrepareCompletion(user.ID, c.Param("search_id"), &req)
	if err != nil {
		if code == common.CodeAuthenticationError {
			common.ResponseWithCodeData(c, code, false, err.Error())
			return
		}
		if code == common.CodeServerError {
			jsonInternalError(c, err)
			return
		}
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}
	if plan == nil {
		common.ResponseWithCodeData(c, common.CodeServerError, nil, "completion plan is nil")
		return
	}

	disableWriteDeadlineForSSE(c)
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	writer := h.sseWriter
	if writer == nil {
		writer = &ginSSEWriter{}
	}
	if plan.ModelID == "" {
		writer.Write(c, sseError("chat model not configured"))
		return
	}
	if h.askService == nil {
		writer.Write(c, sseError("ask service not configured"))
		return
	}
	if h.streamLLM == nil {
		writer.Write(c, sseError("streaming LLM not configured"))
		return
	}

	adapter := &service.TenantStreamAdapter{LLM: h.streamLLM, TenantID: plan.UserID, ModelID: plan.ModelID}

	hadError := false
	for delta := range h.askService.StreamWithOptions(c.Request.Context(), adapter, plan.UserID, plan.Question, plan.KBIDs, plan.Options) {
		switch delta.Kind {
		case service.AskDeltaAnswer:
			writer.Write(c, sseAnswer(delta.Value, nil, false))
		case service.AskDeltaMarker:
			writer.Write(c, sseMarker(delta.Value))
		case service.AskDeltaError:
			hadError = true
			writer.Write(c, sseError(delta.Value))
		case service.AskDeltaFinal:
			writer.Write(c, sseAnswer(delta.Value, delta.Refs, true))
		}
	}
	if !hadError {
		writer.Write(c, "data: {\"code\": 0, \"message\": \"\", \"data\": true}\n\n")
	}
}
