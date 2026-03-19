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
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/service"
	"ragflow/internal/utility"
)

// DatasetsHandler handles the RESTful dataset endpoints.
type DatasetsHandler struct {
	datasetsService *service.DatasetsService
}

// NewDatasetsHandler creates a new datasets handler.
func NewDatasetsHandler(datasetsService *service.DatasetsService) *DatasetsHandler {
	return &DatasetsHandler{datasetsService: datasetsService}
}

// ListDatasets handles GET /api/v1/datasets.
func (h *DatasetsHandler) ListDatasets(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		utility.WriteAPIError(c, errorCode, errorMessage)
		return
	}

	req, err := buildListDatasetsRequest(c)
	if err != nil {
		utility.WriteAPIError(c, common.CodeArgumentError, err.Error())
		return
	}
	if err := h.datasetsService.ValidateListDatasetsRequest(req); err != nil {
		utility.WriteAPIError(c, common.CodeArgumentError, err.Error())
		return
	}

	result, err := h.datasetsService.ListDatasets(user.ID, req)
	if err != nil {
		utility.WriteAPIError(c, common.CodeDataError, err.Error())
		return
	}

	utility.WriteAPISuccessWithExtras(c, result.Data, map[string]interface{}{
		"total_datasets": result.Total,
	})
}

// CreateDataset handles POST /api/v1/datasets.
func (h *DatasetsHandler) CreateDataset(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		utility.WriteAPIError(c, errorCode, errorMessage)
		return
	}

	var req service.CreateDatasetRequest
	rawFields, err := bindJSONBody(c, &req)
	if err != nil {
		utility.WriteAPIError(c, common.CodeArgumentError, err.Error())
		return
	}
	_, req.ChunkMethodProvided = rawFields["chunk_method"]

	if err := h.datasetsService.ValidateCreateDatasetRequest(&req); err != nil {
		utility.WriteAPIError(c, common.CodeArgumentError, err.Error())
		return
	}

	result, err := h.datasetsService.CreateDataset(user.ID, &req)
	if err != nil {
		utility.WriteAPIError(c, common.CodeDataError, err.Error())
		return
	}

	utility.WriteAPISuccess(c, result)
}

// DeleteDatasets handles DELETE /api/v1/datasets.
func (h *DatasetsHandler) DeleteDatasets(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		utility.WriteAPIError(c, errorCode, errorMessage)
		return
	}

	var req service.DeleteDatasetsRequest
	if _, err := bindJSONBody(c, &req); err != nil {
		utility.WriteAPIError(c, common.CodeArgumentError, err.Error())
		return
	}
	if err := h.datasetsService.ValidateDeleteDatasetsRequest(&req); err != nil {
		utility.WriteAPIError(c, common.CodeArgumentError, err.Error())
		return
	}

	result, err := h.datasetsService.DeleteDatasets(user.ID, &req)
	if err != nil {
		utility.WriteAPIError(c, common.CodeDataError, err.Error())
		return
	}

	utility.WriteAPISuccess(c, result)
}

func buildListDatasetsRequest(c *gin.Context) (*service.ListDatasetsRequest, error) {
	req := &service.ListDatasetsRequest{
		Page:     1,
		PageSize: 30,
		OrderBy:  "create_time",
		Desc:     true,
	}

	req.ID = strings.TrimSpace(c.Query("id"))
	req.Name = strings.TrimSpace(c.Query("name"))

	if pageValue := strings.TrimSpace(c.Query("page")); pageValue != "" {
		page, err := strconv.Atoi(pageValue)
		if err != nil {
			return nil, err
		}
		req.Page = page
	}
	if pageSizeValue := strings.TrimSpace(c.Query("page_size")); pageSizeValue != "" {
		pageSize, err := strconv.Atoi(pageSizeValue)
		if err != nil {
			return nil, err
		}
		req.PageSize = pageSize
	}
	if orderBy := strings.TrimSpace(c.Query("orderby")); orderBy != "" {
		req.OrderBy = orderBy
	}
	if descValue := strings.TrimSpace(c.Query("desc")); descValue != "" {
		desc, err := strconv.ParseBool(descValue)
		if err != nil {
			return nil, err
		}
		req.Desc = desc
	}
	if includeParsingStatusValue := strings.TrimSpace(c.Query("include_parsing_status")); includeParsingStatusValue != "" {
		includeParsingStatus, err := strconv.ParseBool(includeParsingStatusValue)
		if err != nil {
			return nil, err
		}
		req.IncludeParsingStatus = includeParsingStatus
	}
	if extValue := strings.TrimSpace(c.Query("ext")); extValue != "" {
		if err := json.Unmarshal([]byte(extValue), &req.Ext); err != nil {
			return nil, err
		}
	}

	return req, nil
}

func bindJSONBody(c *gin.Context, dst interface{}) (map[string]json.RawMessage, error) {
	body, err := c.GetRawData()
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		body = []byte("{}")
	}

	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawFields); err != nil {
		return nil, err
	}
	if rawFields == nil {
		rawFields = make(map[string]json.RawMessage)
	}

	strictDecoder := json.NewDecoder(bytes.NewReader(body))
	strictDecoder.DisallowUnknownFields()
	if err := strictDecoder.Decode(dst); err != nil {
		return nil, err
	}

	return rawFields, nil
}
