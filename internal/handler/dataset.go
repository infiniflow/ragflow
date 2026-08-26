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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/service"
	dataset "ragflow/internal/service/dataset"
)

// DatasetsHandler handles the RESTful dataset endpoints.
type DatasetsHandler struct {
	datasetsService       *dataset.DatasetService
	metadataService       *service.MetadataService
	searchDatasetsService searchDatasetsService
	searchDatasetService  searchDatasetService
}

type searchDatasetsService interface {
	SearchDatasets(ctx context.Context, req *service.SearchDatasetsRequest, userID string) (*service.SearchDatasetsResponse, error)
}

type searchDatasetService interface {
	SearchDataset(ctx context.Context, datasetID, userID string, req *service.SearchDatasetRequest) (*service.SearchDatasetsResponse, error)
}

type listDatasetsExt struct {
	Keywords string   `json:"keywords,omitempty"`
	OwnerIDs []string `json:"owner_ids,omitempty"`
	ParserID string   `json:"parser_id,omitempty"`
}

// NewDatasetsHandler creates a new datasets' handler.
func NewDatasetsHandler(datasetsService *dataset.DatasetService, metadataService *service.MetadataService) *DatasetsHandler {
	h := &DatasetsHandler{
		datasetsService: datasetsService,
		metadataService: metadataService,
	}
	if datasetsService != nil {
		h.searchDatasetsService = datasetsService
		h.searchDatasetService = datasetsService
	}
	return h
}

// ListDatasets handles GET /api/v1/datasets.
func (h *DatasetsHandler) ListDatasets(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	if c.Query("type") == "filter" {
		ctx := c.Request.Context()
		data, code, err := h.datasetsService.ListDatasetFilters(ctx, user.ID)
		if err != nil {
			common.ErrorWithCode(c, code, err.Error())
			return
		}
		common.SuccessNoMessage(c, data)
		return
	}

	// Mirror pydantic's extra="forbid" on ListDatasetReq.
	for param := range c.Request.URL.Query() {
		if !listDatasetsAllowedParams[param] {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil, fmt.Sprintf("Extra inputs are not permitted: %s", param))
			return
		}
	}

	// Mirror the pydantic validation of Python's ListDatasetReq.
	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		p, err := strconv.Atoi(pageStr)
		if err != nil {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Input should be a valid integer, unable to parse string as an integer")
			return
		}
		if p < 1 {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Input should be greater than or equal to 1")
			return
		}
		page = p
	}

	pageSize := 30
	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		ps, err := strconv.Atoi(pageSizeStr)
		if err != nil {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Input should be a valid integer, unable to parse string as an integer")
			return
		}
		if ps < 1 {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Input should be greater than or equal to 1")
			return
		}
		pageSize = ps
	}

	orderby := "create_time"
	if queryOrderby, exists := c.GetQuery("orderby"); exists {
		if queryOrderby != "create_time" && queryOrderby != "update_time" {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Input should be 'create_time' or 'update_time'")
			return
		}
		orderby = queryOrderby
	}

	desc := true
	if descStr := c.Query("desc"); descStr != "" {
		parsed, ok := parsePythonBool(descStr)
		if !ok {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Input should be a valid boolean, unable to interpret input")
			return
		}
		desc = parsed
	}

	keywords := ""
	parserID := ""
	var ownerIDs []string

	// ext keeps the same compatibility payload as the Python REST API.
	if extStr := c.Query("ext"); extStr != "" {
		var ext listDatasetsExt
		if err := json.Unmarshal([]byte(extStr), &ext); err != nil {
			common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
			return
		}
		keywords = ext.Keywords
		parserID = ext.ParserID
		ownerIDs = ext.OwnerIDs
	}

	// Mirror pydantic: a present-but-empty id fails UUID validation.
	if rawID, exists := c.GetQuery("id"); exists {
		if _, err := dataset.NormalizeDatasetID(strings.TrimSpace(rawID)); err != nil {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
			return
		}
	}

	// Mirror pydantic's ListDatasetReq.ids: each occurrence is comma-split,
	// every value must be a valid UUID, and duplicates are rejected.
	var ids []string
	if rawIDs, exists := c.Request.URL.Query()["ids"]; exists {
		seen := make(map[string]int)
		for _, item := range rawIDs {
			for _, value := range strings.Split(item, ",") {
				if value == "" {
					continue
				}
				normalizedID, err := dataset.NormalizeDatasetID(strings.TrimSpace(value))
				if err != nil {
					common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
					return
				}
				seen[normalizedID]++
				ids = append(ids, normalizedID)
			}
		}
		duplicates := make([]string, 0, len(ids))
		reported := make(map[string]bool)
		for _, normalizedID := range ids {
			if seen[normalizedID] > 1 && !reported[normalizedID] {
				reported[normalizedID] = true
				duplicates = append(duplicates, normalizedID)
			}
		}
		if len(duplicates) > 0 {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil, fmt.Sprintf("Duplicate ids: '%s'", strings.Join(duplicates, ", ")))
			return
		}
	}

	ctx := c.Request.Context()
	data, total, code, err := h.datasetsService.ListDatasets(
		ctx,
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
		ids,
	)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
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
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	bodyBytes, _, ok := parseJSONRequestObject(c)
	if !ok {
		return
	}

	var req service.CreateDatasetRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}
	// Mirror Python's pydantic required validation.
	if req.Name == "" || (len(bodyBytes) > 0 && jsonNullValue(bodyBytes, "name")) {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "Field validation for 'name' failed on the 'required' tag")
		return
	}

	ctx := c.Request.Context()

	result, code, err := h.datasetsService.CreateDataset(ctx, &req, user.ID)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}

	common.SuccessNoMessage(c, result)
}

// GetDataset handles GET /api/v1/datasets/:dataset_id.
func (h *DatasetsHandler) GetDataset(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	ctx := c.Request.Context()

	datasetID := c.Param("dataset_id")
	result, code, err := h.datasetsService.GetDataset(ctx, datasetID, user.ID)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}

	common.SuccessNoMessage(c, result)
}

// UpdateDataset Update a dataset.
// parsePythonBool mirrors pydantic's boolean coercion for query params:
// accepts true/false/1/0/yes/no/y/n (case-insensitive).
func parsePythonBool(s string) (bool, bool) {
	switch strings.ToLower(s) {
	case "true", "1", "yes", "y", "on":
		return true, true
	case "false", "0", "no", "n", "off":
		return false, true
	}
	return false, false
}

// parseJSONRequestObject mirrors Python's validate_and_parse_json_request:
// content-type check first, then JSON syntax, then object shape. On failure it
// writes the error response and returns ok=false.
func parseJSONRequestObject(c *gin.Context) (body []byte, raw map[string]interface{}, ok bool) {
	if c.ContentType() != "application/json" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, fmt.Sprintf("Unsupported content type: Expected application/json, got %s", c.GetHeader("Content-Type")))
		return nil, nil, false
	}

	body, err := c.GetRawData()
	if err != nil || len(body) == 0 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Malformed JSON syntax: Missing commas/brackets or invalid encoding")
		return nil, nil, false
	}

	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Malformed JSON syntax: Missing commas/brackets or invalid encoding")
		return nil, nil, false
	}
	raw, isObject := payload.(map[string]interface{})
	if !isObject {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, fmt.Sprintf("Invalid request payload: expected object, got %s", pythonJSONTypeName(payload)))
		return nil, nil, false
	}
	return body, raw, true
}

// jsonNullValue checks if a field is explicitly null in the JSON body.
func jsonNullValue(body []byte, field string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return false
	}
	raw, ok := m[field]
	if !ok {
		return false
	}
	return strings.TrimSpace(string(raw)) == "null"
}

// pythonJSONTypeName maps a decoded JSON value to the Python type name used in
// the "Invalid request payload" contract message.
func pythonJSONTypeName(v interface{}) string {
	switch v.(type) {
	case string:
		return "str"
	case []interface{}:
		return "list"
	case float64:
		return "float"
	case bool:
		return "bool"
	case nil:
		return "NoneType"
	default:
		return "object"
	}
}

// listDatasetsAllowedParams mirrors the query field set of Python's
// ListDatasetReq (BaseListReq + include_parsing_status/ext; `type` is handled
// before validation in the Python endpoint).
var listDatasetsAllowedParams = map[string]bool{
	"id": true, "ids": true, "name": true, "page": true, "page_size": true,
	"orderby": true, "desc": true, "include_parsing_status": true,
	"ext": true, "type": true,
}

// updateDatasetAllowedFields mirrors the field set of Python's UpdateDatasetReq
// (CreateDatasetReq fields + dataset_id/pagerank/language/connectors).
var updateDatasetAllowedFields = map[string]bool{
	"name": true, "avatar": true, "description": true, "embedding_model": true,
	"permission": true, "parse_type": true, "pipeline_id": true, "chunk_method": true,
	"parser_id": true, "parser_config": true, "auto_metadata_config": true, "ext": true,
	"dataset_id": true, "pagerank": true, "language": true, "connectors": true,
}

func (h *DatasetsHandler) UpdateDataset(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "user id is required")
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		common.ResponseWithCodeData(c, common.CodeBadRequest, nil, "dataset id is required")
		return
	}
	// Mirror the pydantic UUID validation of Python's UpdateDatasetReq.
	if _, err := dataset.NormalizeDatasetID(datasetID); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
		return
	}

	bodyBytes, _, ok := parseJSONRequestObject(c)
	if !ok {
		return
	}

	var req service.UpdateDatasetRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	// Detect an explicitly provided parser_config key (even {} or null) so it is not
	// rejected as "no properties were modified", mirroring the Python contract.
	var providedFields map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &providedFields); err == nil {
		_, req.ParserConfigProvided = providedFields["parser_config"]
		// Mirror pydantic's extra="forbid" on UpdateDatasetReq.
		for field := range providedFields {
			if !updateDatasetAllowedFields[field] {
				common.ResponseWithCodeData(c, common.CodeArgumentError, nil, fmt.Sprintf("Extra inputs are not permitted: %s", field))
				return
			}
		}
	}

	ctx := c.Request.Context()

	result, code, err := h.datasetsService.UpdateDataset(ctx, datasetID, userID, req)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, "dataset updated failed")
		return
	}

	common.SuccessNoMessage(c, result)
}

func (h *DatasetsHandler) GetMetadataConfig(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	ctx := c.Request.Context()

	datasetID := c.Param("dataset_id")
	result, code, err := h.datasetsService.GetMetadataConfig(ctx, datasetID, user.ID)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}

	common.SuccessNoMessage(c, result)
}

func (h *DatasetsHandler) UpdateMetadataConfig(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")
	var req service.MetadataConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	ctx := c.Request.Context()

	result, code, err := h.datasetsService.UpdateMetadataConfig(ctx, datasetID, user.ID, &req)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}

	common.SuccessNoMessage(c, result)
}

// GetIngestionSummary handles GET /api/v1/datasets/:dataset_id/ingestions/summary.
func (h *DatasetsHandler) GetIngestionSummary(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}
	ctx := c.Request.Context()
	datasetID := c.Param("dataset_id")
	result, code, err := h.datasetsService.GetIngestionSummary(ctx, datasetID, user.ID)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

// ListIngestionLogs handles GET /api/v1/datasets/:dataset_id/ingestions.
func (h *DatasetsHandler) ListIngestionLogs(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")

	page := 0
	if pageStr := c.Query("page"); pageStr != "" {
		p, err := strconv.Atoi(pageStr)
		if err != nil {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
				"page must be an integer")
			return
		}
		page = p
	}

	pageSize := 0
	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		ps, err := strconv.Atoi(pageSizeStr)
		if err != nil {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
				"page_size must be an integer")
			return
		}
		pageSize = ps
	}

	orderby := c.DefaultQuery("orderby", "create_time")
	// desc defaults to true and is only disabled by the literal value "false".
	desc := strings.ToLower(c.DefaultQuery("desc", "true")) != "false"
	operationStatus := c.QueryArray("operation_status")
	createDateFrom := c.Query("create_date_from")
	createDateTo := c.Query("create_date_to")
	logType := c.DefaultQuery("log_type", "dataset")
	keywords := c.Query("keywords")

	ctx := c.Request.Context()

	result, code, err := h.datasetsService.ListIngestionLogs(ctx, datasetID, user.ID, page, pageSize, orderby, desc, operationStatus, createDateFrom, createDateTo, logType, keywords)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

// GetIngestionLog handles GET /api/v1/datasets/:dataset_id/ingestions/:log_id.
func (h *DatasetsHandler) GetIngestionLog(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	ctx := c.Request.Context()

	datasetID := c.Param("dataset_id")
	logID := c.Param("log_id")
	result, code, err := h.datasetsService.GetIngestionLog(ctx, datasetID, user.ID, logID)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

// DeleteDatasets handles DELETE /api/v1/datasets.
func (h *DatasetsHandler) DeleteDatasets(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	bodyBytes, rawPayload, ok := parseJSONRequestObject(c)
	if !ok {
		return
	}

	// Mirror pydantic's extra="forbid" on DeleteDatasetReq.
	for field := range rawPayload {
		if field != "ids" && field != "delete_all" {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil, fmt.Sprintf("Extra inputs are not permitted: %s", field))
			return
		}
	}

	var req struct {
		IDs       *[]string `json:"ids"`
		DeleteAll bool      `json:"delete_all,omitempty"`
	}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	// Mirror DeleteReq duplicate detection.
	if req.IDs != nil {
		seen := make(map[string]int, len(*req.IDs))
		duplicates := make([]string, 0)
		for _, id := range *req.IDs {
			seen[id]++
			if seen[id] == 2 {
				duplicates = append(duplicates, id)
			}
		}
		if len(duplicates) > 0 {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil, fmt.Sprintf("Duplicate ids: '%s'", strings.Join(duplicates, ", ")))
			return
		}
	}

	var ids []string
	if req.IDs != nil {
		ids = *req.IDs
	}

	ctx := c.Request.Context()

	result, code, err := h.datasetsService.DeleteDatasets(ctx, ids, req.DeleteAll, user.ID)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}

	common.SuccessNoMessage(c, result)
}

// GetKnowledgeGraph handles GET /api/v1/datasets/:dataset_id/graph.
func (h *DatasetsHandler) GetKnowledgeGraph(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "dataset_id is required")
		return
	}

	ctx := c.Request.Context()
	datasetInstance, code, err := h.datasetsService.GetDataset(ctx, datasetID, user.ID)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}

	tenantID, _ := datasetInstance["tenant_id"].(string)
	if tenantID == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "tenant_id is required")
		return
	}

	docEngine := engine.Get()
	if docEngine == nil {
		common.ResponseWithCodeData(c, common.CodeServerError, nil, "Document engine is not initialized")
		return
	}

	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	exists, err := docEngine.ChunkStoreExists(c.Request.Context(), indexName, datasetID)
	if err != nil {
		jsonInternalError(c, err)
		return
	}

	result := gin.H{
		"graph":    map[string]interface{}{},
		"mind_map": map[string]interface{}{},
	}
	if !exists {
		common.SuccessWithData(c, result, "success")
		return
	}

	searchResult, err := docEngine.Search(c.Request.Context(), &types.SearchRequest{
		IndexNames:   []string{indexName},
		KbIDs:        []string{datasetID},
		Offset:       0,
		Limit:        1,
		SelectFields: []string{"content_with_weight", "knowledge_graph_kwd"},
		Filter: map[string]interface{}{
			"kb_id":               []string{datasetID},
			"knowledge_graph_kwd": []string{"graph"},
		},
	})
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	if searchResult == nil || len(searchResult.Chunks) == 0 {
		common.SuccessWithData(c, result, "success")
		return
	}

	chunk := searchResult.Chunks[0]
	graphType := firstStringValue(chunk["knowledge_graph_kwd"])
	contentWithWeight, _ := chunk["content_with_weight"].(string)
	if strings.TrimSpace(contentWithWeight) == "" {
		common.SuccessWithData(c, result, "success")
		return
	}

	var graphData map[string]interface{}
	if err = json.Unmarshal([]byte(contentWithWeight), &graphData); err != nil {
		common.SuccessWithData(c, result, "success")
		return
	}
	if len(graphData) == 0 {
		common.SuccessWithData(c, result, "success")
		return
	}

	if graphType == "" {
		graphType = "graph"
	}
	if graphType == "graph" {
		sortKnowledgeGraph(graphData)
		result["graph"] = graphData
	} else {
		result[graphType] = graphData
	}

	common.SuccessWithData(c, result, "success")
}

// ListTags handles GET /api/v1/datasets/:dataset_id/tags.
// @Summary List dataset tags
// @Description List tags for a dataset
// @Tags datasets
// @Produce json
// @Security ApiKeyAuth
// @Param dataset_id path string true "Dataset ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/datasets/{dataset_id}/tags [get]
func (h *DatasetsHandler) ListTags(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	ctx := c.Request.Context()

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	result, code, err := h.datasetsService.ListTags(ctx, datasetID, user.ID)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

type renameTagRequest struct {
	FromTag string `json:"from_tag"`
	ToTag   string `json:"to_tag"`
}

func (h *DatasetsHandler) RenameTag(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}
	datasetID := strings.TrimSpace(c.Param("dataset_id"))

	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "Lack of from_tag or to_tag in request body")
		return
	}
	fromTagValue, hasFrom := payload["from_tag"]
	toTagValue, hasTo := payload["to_tag"]
	if !hasFrom || !hasTo {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "Lack of from_tag or to_tag in request body")
		return
	}
	fromTag, okFrom := fromTagValue.(string)
	toTag, okTo := toTagValue.(string)
	if !okFrom || !okTo {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "from_tag and to_tag must be strings")
		return
	}
	req := renameTagRequest{FromTag: fromTag, ToTag: toTag}
	if strings.TrimSpace(req.FromTag) == "" || strings.TrimSpace(req.ToTag) == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "from_tag and to_tag must not be empty")
		return
	}

	ctx := c.Request.Context()

	result, code, err := h.datasetsService.RenameTag(ctx, datasetID, user.ID, req.FromTag, req.ToTag)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

// DeleteKnowledgeGraph handles DELETE /api/v1/datasets/:dataset_id/graph.
func (h *DatasetsHandler) DeleteKnowledgeGraph(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "dataset_id is required")
		return
	}

	ctx := c.Request.Context()
	datasetInstance, code, err := h.datasetsService.GetDataset(ctx, datasetID, user.ID)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}

	tenantID, _ := datasetInstance["tenant_id"].(string)
	if tenantID == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "tenant_id is required")
		return
	}

	docEngine := engine.Get()
	if docEngine == nil {
		common.ResponseWithCodeData(c, common.CodeServerError, nil, "Document engine is not initialized")
		return
	}

	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	if _, err := docEngine.DeleteChunks(c.Request.Context(), map[string]interface{}{
		"knowledge_graph_kwd": []interface{}{"graph", "subgraph", "entity", "relation", "community_report"},
		"kb_id":               datasetID,
	}, indexName, datasetID); err != nil {
		jsonInternalError(c, err)
		return
	}

	common.SuccessWithData(c, true, "success")
}

// RemoveTags handles DELETE /api/v1/datasets/:dataset_id/tags.
// @Summary Remove Tags
// @Description Remove tags from a dataset
// @Tags datasets
// @Security ApiKeyAuth
// @Param dataset_id path string true "Dataset ID"
// @Param request body object{tags []string} true "tags to remove"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/datasets/{dataset_id}/tags [delete]
func (h *DatasetsHandler) RemoveTags(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "dataset_id is required")
		return
	}

	ctx := c.Request.Context()
	datasetInstance, code, err := h.datasetsService.GetDataset(ctx, datasetID, user.ID)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}

	tenantID, _ := datasetInstance["tenant_id"].(string)
	if tenantID == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "tenant_id is required")
		return
	}

	var req struct {
		Tags []string `json:"tags" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	docEngine := engine.Get()
	if docEngine == nil {
		common.ResponseWithCodeData(c, common.CodeServerError, nil, "Document engine is not initialized")
		return
	}

	for _, tag := range req.Tags {
		condition := map[string]interface{}{
			"tag_kwd": tag,
			"kb_id":   datasetID,
		}
		newValue := map[string]interface{}{
			"remove": map[string]interface{}{
				"tag_kwd": tag,
			},
		}
		if err := docEngine.UpdateChunks(c.Request.Context(), condition, newValue, indexName, datasetID); err != nil {
			common.ResponseWithCodeData(c, common.CodeServerError, nil, "Failed to remove tag: "+err.Error())
			return
		}
	}

	common.SuccessWithData(c, true, "success")
}

// CheckEmbedding Check embedding model compatibility by sampling random chunks,
// re-embedding them with the new model, and computing cosine similarity.
func (h *DatasetsHandler) CheckEmbedding(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "dataset_id is required")
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "user_id is required")
		return
	}

	var req service.CheckEmbeddingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}
	if strings.TrimSpace(req.EmbeddingID) == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "`embd_id` is required.")
		return
	}

	ctx := c.Request.Context()
	data, code, err := h.datasetsService.CheckEmbedding(ctx, userID, datasetID, &req)
	if err != nil {
		if code == common.CodeNotEffective {
			common.ResponseWithCodeData(c, code, data, err.Error())
			return
		}
		common.ErrorWithCode(c, code, err.Error())
		return
	}
	common.SuccessWithData(c, data, "success")
}

// AggregateTags handles GET /api/v1/datasets/tags/aggregation.
// @Summary Aggregate dataset tags
// @Description Aggregate tags across multiple datasets
// @Tags datasets
// @Produce json
// @Security ApiKeyAuth
// @Param dataset_ids query string true "Comma-separated dataset IDs"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/datasets/tags/aggregation [get]
func (h *DatasetsHandler) AggregateTags(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	rawIDs := strings.Split(c.Query("dataset_ids"), ",")
	datasetIDs := make([]string, 0, len(rawIDs))

	for _, rawID := range rawIDs {
		tempID := strings.TrimSpace(rawID)
		if tempID != "" {
			datasetIDs = append(datasetIDs, tempID)
		}
	}
	if len(datasetIDs) == 0 {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "Lack of dataset_ids in query parameters")
		return
	}

	ctx := c.Request.Context()

	result, code, err := h.datasetsService.AggregateTags(ctx, datasetIDs, user.ID)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}
	common.SuccessWithData(c, result, "success")
}

// GetCompilationStatus returns the dataset-level knowledge-compile lifecycle
// state (scheduler contract for API_PROXY_SCHEME=go/hybrid). It replaces the
// Python-era TraceIndex task-progress endpoint for the Go backend.
func (h *DatasetsHandler) GetCompilationStatus(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}
	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "dataset_id is required")
		return
	}
	userID := strings.TrimSpace(user.ID)
	ctx := c.Request.Context()
	st, code, err := h.datasetsService.GetDatasetCompilationStatus(ctx, userID, datasetID)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}
	common.SuccessWithData(c, st, "success")
}

// ListMetadataFlattened handles GET /api/v1/datasets/metadata/flattened.
// @Summary List flattened metadata for datasets
// @Description Get flattened metadata for multiple datasets
// @Tags datasets
// @Produce json
// @Security ApiKeyAuth
// @Param dataset_ids query string true "Comma-separated dataset IDs"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/datasets/metadata/flattened [get]
func (h *DatasetsHandler) ListMetadataFlattened(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	datasetIDsStr := c.Query("dataset_ids")
	if datasetIDsStr == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "dataset_ids is required")
		return
	}

	rawIDs := strings.Split(datasetIDsStr, ",")
	datasetIDs := make([]string, 0, len(rawIDs))
	for _, id := range rawIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			datasetIDs = append(datasetIDs, id)
		}
	}
	if len(datasetIDs) == 0 {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "dataset_ids is required")
		return
	}

	ctx := c.Request.Context()
	// Check access for each dataset
	for _, datasetID := range datasetIDs {
		if !h.datasetsService.Accessible(ctx, datasetID, user.ID) {
			common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "No authorization for dataset: "+datasetID)
			return
		}
	}

	flattenedMeta, err := h.metadataService.GetFlattedMetaByKBs(ctx, datasetIDs)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, nil, "Failed to get metadata: "+err.Error())
		return
	}

	common.SuccessWithData(c, flattenedMeta, "success")
}

func (h *DatasetsHandler) UpdateDocumentMetadataConfig(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "dataset_id is required")
		return
	}
	documentID := strings.TrimSpace(c.Param("document_id"))
	if documentID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "document_id is required")
		return
	}
	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "user_id is required")
		return
	}

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	ctx := c.Request.Context()
	data, code, err := h.datasetsService.UpdateDocumentMetadataConfig(ctx, userID, datasetID, documentID, req)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}

	common.ResponseWithCodeData(c, code, data, "success")
}

// SearchDatasets searches chunks across datasets based on a question
// @Summary Search Datasets
// @Description Search for relevant chunks across one or more datasets based on a question
// @Tags datasets
// @Param request body service.SearchDatasetsRequest true "search parameters"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/datasets/search [post]
func (h *DatasetsHandler) SearchDatasets(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	var req service.SearchDatasetsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
		return
	}

	if req.Page == nil {
		defaultPage := 1
		req.Page = &defaultPage
	}
	if req.Size == nil {
		defaultSize := 30
		req.Size = &defaultSize
	}
	if req.KNNTopK == nil && req.TopK == nil {
		defaultTopK := 1024
		req.KNNTopK = &defaultTopK
	}
	if req.KNNNumCandidates == nil {
		defaultNumCandidates := 2048
		req.KNNNumCandidates = &defaultNumCandidates
	}
	if req.UseKG == nil {
		defaultUseKG := false
		req.UseKG = &defaultUseKG
	}

	req.Question = strings.TrimSpace(req.Question)
	if req.Question == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "question is required")
		return
	}
	if req.DatasetIDs == nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "kb_id is required")
		return
	}

	if len(req.DatasetIDs) == 0 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "kb_id array cannot be empty")
		return
	}
	if err := validateSearchDatasetsRequest(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
		return
	}

	searchService := h.searchDatasetsService
	if searchService == nil {
		searchService = h.datasetsService
	}
	if searchService == nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "dataset service is not initialized")
		return
	}

	ctx := c.Request.Context()

	resp, err := searchService.SearchDatasets(ctx, &req, user.ID)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	common.SuccessNoMessage(c, resp)
}

// SearchDataset searches chunks within a single dataset based on a question.
// @Summary Search Dataset
// @Description Search for relevant chunks within one dataset based on a question
// @Tags datasets
// @Param dataset_id path string true "dataset id"
// @Param request body service.SearchDatasetRequest true "search parameters"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/datasets/{dataset_id}/search [post]
func (h *DatasetsHandler) SearchDataset(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")
	if datasetID == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "dataset_id is required")
		return
	}

	var req service.SearchDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
		return
	}
	req.Question = strings.TrimSpace(req.Question)
	if req.Question == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "question is required")
		return
	}
	if err := validateSearchDatasetRequest(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
		return
	}

	searchService := h.searchDatasetService
	if searchService == nil {
		searchService = h.datasetsService
	}
	if searchService == nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "dataset service is not initialized")
		return
	}

	ctx := c.Request.Context()

	resp, err := searchService.SearchDataset(ctx, datasetID, user.ID, &req)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	common.SuccessNoMessage(c, resp)
}

func validateSearchDatasetsRequest(req *service.SearchDatasetsRequest) error {
	return validateSearchParams(req.Page, req.Size, req.KNNTopK, req.TopK, req.KNNNumCandidates, req.SimilarityThreshold, req.VectorSimilarityWeight)
}

func validateSearchDatasetRequest(req *service.SearchDatasetRequest) error {
	return validateSearchParams(req.Page, req.Size, req.KNNTopK, req.TopK, req.KNNNumCandidates, req.SimilarityThreshold, req.VectorSimilarityWeight)
}

func validateSearchParams(page, size, knnTopK, topK, knnNumCandidates *int, similarityThreshold, vectorSimilarityWeight *float64) error {
	if page != nil && *page < 1 {
		return fmt.Errorf("page must be greater than or equal to 1")
	}
	if size != nil && *size < 1 {
		return fmt.Errorf("size must be greater than or equal to 1")
	}
	effectiveKNNTopK := 1024
	knnTopKName := "knn_top_k"
	if knnTopK != nil {
		effectiveKNNTopK = *knnTopK
	} else if topK != nil {
		effectiveKNNTopK = *topK
		knnTopKName = "top_k (alias for knn_top_k)"
	}
	if effectiveKNNTopK < 1 || effectiveKNNTopK > 2048 {
		return fmt.Errorf("%s must be between 1 and 2048", knnTopKName)
	}
	effectiveKNNNumCandidates := 2048
	if knnNumCandidates != nil {
		effectiveKNNNumCandidates = *knnNumCandidates
	}
	if effectiveKNNNumCandidates < 1 || effectiveKNNNumCandidates > 10000 {
		return fmt.Errorf("knn_num_candidates must be between 1 and 10000")
	}
	if effectiveKNNNumCandidates < effectiveKNNTopK {
		return fmt.Errorf("knn_num_candidates must be greater than or equal to knn_top_k")
	}
	if similarityThreshold != nil && (*similarityThreshold < 0 || *similarityThreshold > 1) {
		return fmt.Errorf("similarity_threshold must be between 0 and 1")
	}
	if vectorSimilarityWeight != nil && (*vectorSimilarityWeight < 0 || *vectorSimilarityWeight > 1) {
		return fmt.Errorf("vector_similarity_weight must be between 0 and 1")
	}
	return nil
}

func firstStringValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []string:
		if len(v) > 0 {
			return strings.TrimSpace(v[0])
		}
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func sortKnowledgeGraph(graphData map[string]interface{}) {
	nodes := mapSlice(graphData["nodes"])
	if len(nodes) > 0 {
		sort.Slice(nodes, func(i, j int) bool {
			return numericValue(nodes[i]["pagerank"]) > numericValue(nodes[j]["pagerank"])
		})
		if len(nodes) > 256 {
			nodes = nodes[:256]
		}
		graphData["nodes"] = nodes
	}

	edges := mapSlice(graphData["edges"])
	if len(edges) > 0 {
		nodeIDSet := make(map[string]struct{}, len(nodes))
		for _, node := range nodes {
			if id, ok := node["id"].(string); ok {
				nodeIDSet[id] = struct{}{}
			}
		}
		filteredEdges := make([]map[string]interface{}, 0, len(edges))
		for _, edge := range edges {
			source, _ := edge["source"].(string)
			target, _ := edge["target"].(string)
			if source == "" || target == "" || source == target {
				continue
			}
			if _, ok := nodeIDSet[source]; !ok {
				continue
			}
			if _, ok := nodeIDSet[target]; !ok {
				continue
			}
			filteredEdges = append(filteredEdges, edge)
		}
		sort.Slice(filteredEdges, func(i, j int) bool {
			return numericValue(filteredEdges[i]["weight"]) > numericValue(filteredEdges[j]["weight"])
		})
		if len(filteredEdges) > 128 {
			filteredEdges = filteredEdges[:128]
		}
		graphData["edges"] = filteredEdges
	}
}

func mapSlice(value interface{}) []map[string]interface{} {
	raw, ok := value.([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]interface{}); ok {
			result = append(result, m)
		}
	}
	return result
}

func numericValue(value interface{}) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return 0
	}
}
