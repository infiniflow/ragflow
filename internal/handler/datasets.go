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
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/service"
)

// DatasetsHandler handles the RESTful dataset endpoints.
type DatasetsHandler struct {
	datasetsService *service.DatasetsService
}

type listDatasetsExt struct {
	Keywords string   `json:"keywords,omitempty"`
	OwnerIDs []string `json:"owner_ids,omitempty"`
	ParserID string   `json:"parser_id,omitempty"`
}

// NewDatasetsHandler creates a new datasets handler.
func NewDatasetsHandler(datasetsService *service.DatasetsService) *DatasetsHandler {
	return &DatasetsHandler{datasetsService: datasetsService}
}

// ListDatasets handles GET /api/v1/datasets.
func (h *DatasetsHandler) ListDatasets(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 30
	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
			pageSize = ps
		}
	}

	orderby := "create_time"
	if queryOrderby := c.Query("orderby"); queryOrderby != "" {
		orderby = queryOrderby
	}

	desc := true
	if descStr := c.Query("desc"); descStr != "" {
		desc = strings.ToLower(descStr) == "true"
	}

	keywords := ""
	parserID := ""
	var ownerIDs []string

	// ext keeps the same compatibility payload as the Python REST API.
	if extStr := c.Query("ext"); extStr != "" {
		var ext listDatasetsExt
		if err := json.Unmarshal([]byte(extStr), &ext); err != nil {
			jsonError(c, common.CodeDataError, err.Error())
			return
		}
		keywords = ext.Keywords
		parserID = ext.ParserID
		ownerIDs = ext.OwnerIDs
	}

	data, total, code, err := h.datasetsService.ListDatasets(
		c.Query("id"),
		c.Query("name"),
		page,
		pageSize,
		orderby,
		desc,
		keywords,
		ownerIDs,
		parserID,
		user.ID,
	)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":           common.CodeSuccess,
		"data":           data,
		"total_datasets": total,
	})
}

// CreateDataset handles POST /api/v1/datasets.
func (h *DatasetsHandler) CreateDataset(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req service.CreateDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	result, code, err := h.datasetsService.CreateDataset(&req, user.ID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": common.CodeSuccess,
		"data": result,
	})
}

// DeleteDatasets handles DELETE /api/v1/datasets.
func (h *DatasetsHandler) DeleteDatasets(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req struct {
		IDs       *[]string `json:"ids"`
		DeleteAll bool      `json:"delete_all,omitempty"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			jsonError(c, common.CodeDataError, err.Error())
			return
		}
	}

	var ids []string
	if req.IDs != nil {
		ids = *req.IDs
	}

	result, code, err := h.datasetsService.DeleteDatasets(ids, req.DeleteAll, user.ID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": common.CodeSuccess,
		"data": result,
	})
}
