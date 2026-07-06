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
)

// DatasetsHandler handles the RESTful dataset endpoints.
type DatasetsHandler struct {
	datasetsService       *service.DatasetService
	metadataService       *service.MetadataService
	searchDatasetsService searchDatasetsService
	searchDatasetService  searchDatasetService
}

type searchDatasetsService interface {
	SearchDatasets(req *service.SearchDatasetsRequest, userID string) (*service.SearchDatasetsResponse, error)
}

type searchDatasetService interface {
	SearchDataset(datasetID, userID string, req *service.SearchDatasetRequest) (*service.SearchDatasetsResponse, error)
}

type listDatasetsExt struct {
	Keywords string   `json:"keywords,omitempty"`
	OwnerIDs []string `json:"owner_ids,omitempty"`
	ParserID string   `json:"parser_id,omitempty"`
}

// NewDatasetsHandler creates a new datasets handler.
func NewDatasetsHandler(datasetsService *service.DatasetService, metadataService *service.MetadataService) *DatasetsHandler {
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
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
			common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
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
		common.ErrorWithCode(c, int(code), err.Error())
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req service.CreateDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	result, code, err := h.datasetsService.CreateDataset(&req, user.ID)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessNoMessage(c, result)
}

// GetDataset handles GET /api/v1/datasets/:dataset_id.
func (h *DatasetsHandler) GetDataset(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")
	result, code, err := h.datasetsService.GetDataset(datasetID, user.ID)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessNoMessage(c, result)
}

// UpdateDataset Update a dataset.
func (h *DatasetsHandler) UpdateDataset(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
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

	var req service.UpdateDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	result, code, err := h.datasetsService.UpdateDataset(datasetID, userID, req)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")
	result, code, err := h.datasetsService.GetMetadataConfig(datasetID, user.ID)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessNoMessage(c, result)
}

func (h *DatasetsHandler) UpdateMetadataConfig(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")
	var req service.MetadataConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	result, code, err := h.datasetsService.UpdateMetadataConfig(datasetID, user.ID, &req)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessNoMessage(c, result)
}

// GetIngestionSummary handles GET /api/v1/datasets/:dataset_id/ingestions/summary.
func (h *DatasetsHandler) GetIngestionSummary(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")
	result, code, err := h.datasetsService.GetIngestionSummary(datasetID, user.ID)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

// ListIngestionLogs handles GET /api/v1/datasets/:dataset_id/ingestions.
func (h *DatasetsHandler) ListIngestionLogs(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
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

	result, code, err := h.datasetsService.ListIngestionLogs(datasetID, user.ID, page, pageSize, orderby, desc, operationStatus, createDateFrom, createDateTo, logType, keywords)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

// GetIngestionLog handles GET /api/v1/datasets/:dataset_id/ingestions/:log_id.
func (h *DatasetsHandler) GetIngestionLog(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")
	logID := c.Param("log_id")
	result, code, err := h.datasetsService.GetIngestionLog(datasetID, user.ID, logID)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

// DeleteDatasets handles DELETE /api/v1/datasets.
func (h *DatasetsHandler) DeleteDatasets(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req struct {
		IDs       *[]string `json:"ids"`
		DeleteAll bool      `json:"delete_all,omitempty"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
			return
		}
	}

	var ids []string
	if req.IDs != nil {
		ids = *req.IDs
	}

	result, code, err := h.datasetsService.DeleteDatasets(ids, req.DeleteAll, user.ID)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessNoMessage(c, result)
}

// GetKnowledgeGraph handles GET /api/v1/datasets/:dataset_id/graph.
func (h *DatasetsHandler) GetKnowledgeGraph(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "dataset_id is required")
		return
	}

	dataset, code, err := h.datasetsService.GetDataset(datasetID, user.ID)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	tenantID, _ := dataset["tenant_id"].(string)
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
	if err := json.Unmarshal([]byte(contentWithWeight), &graphData); err != nil {
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	result, code, err := h.datasetsService.ListTags(datasetID, user.ID)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
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

	result, code, err := h.datasetsService.RenameTag(datasetID, user.ID, req.FromTag, req.ToTag)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

// DeleteKnowledgeGraph handles DELETE /api/v1/datasets/:dataset_id/graph.
func (h *DatasetsHandler) DeleteKnowledgeGraph(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "dataset_id is required")
		return
	}

	dataset, code, err := h.datasetsService.GetDataset(datasetID, user.ID)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	tenantID, _ := dataset["tenant_id"].(string)
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
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param dataset_id path string true "Dataset ID"
// @Param request body object{tags []string} true "tags to remove"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/datasets/{dataset_id}/tags [delete]
func (h *DatasetsHandler) RemoveTags(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "dataset_id is required")
		return
	}

	dataset, code, err := h.datasetsService.GetDataset(datasetID, user.ID)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	tenantID, _ := dataset["tenant_id"].(string)
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

// RunEmbedding Run embedding for all documents in a dataset.
func (h *DatasetsHandler) RunEmbedding(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "user_id is required")
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "dataset_id is required")
		return
	}

	result, errorCode, err := h.datasetsService.RunEmbedding(userID, datasetID)
	if err != nil {
		common.ResponseWithCodeData(c, errorCode, nil, err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

// CheckEmbedding Check embedding model compatibility by sampling random chunks,
// re-embedding them with the new model, and computing cosine similarity.
func (h *DatasetsHandler) CheckEmbedding(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
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

	data, code, err := h.datasetsService.CheckEmbedding(userID, datasetID, &req)
	if err != nil {
		if code == common.CodeNotEffective {
			common.ResponseWithCodeData(c, code, data, err.Error())
			return
		}
		common.ErrorWithCode(c, int(code), err.Error())
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
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

	result, code, err := h.datasetsService.AggregateTags(datasetIDs, user.ID)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}
	common.SuccessWithData(c, result, "success")
}

// RunIndex Run an indexing task (graph/raptor/mindmap) for a dataset.
func (h *DatasetsHandler) RunIndex(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
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

	indexType := strings.ToLower(strings.TrimSpace(c.Query("type")))
	data, code, err := h.datasetsService.RunIndex(userID, datasetID, indexType)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithData(c, data, "success")
}

// TraceIndex Trace an indexing task (graph/raptor/mindmap) for a dataset.
func (h *DatasetsHandler) TraceIndex(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
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

	indexType := strings.ToLower(strings.TrimSpace(c.Query("type")))
	result, code, err := h.datasetsService.TraceIndex(datasetID, userID, indexType)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}
	if result == nil {
		common.SuccessWithData(c, map[string]interface{}{}, "success")
		return
	}

	common.SuccessWithData(c, result, "success")
}

// DeleteIndex Delete an indexing task (graph/raptor/mindmap) for a dataset.
func (h *DatasetsHandler) DeleteIndex(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
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

	indexType := strings.ToLower(strings.TrimSpace(c.Param("index_type")))
	if indexType == "" {
		indexType = strings.ToLower(strings.TrimSpace(c.Query("type")))
	}

	wipeArg := strings.ToLower(strings.TrimSpace(c.DefaultQuery("wipe", "true")))
	wipe := true
	switch wipeArg {
	case "false", "0", "no", "off":
		wipe = false
	}

	code, err := h.datasetsService.DeleteIndex(userID, datasetID, indexType, wipe)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithData(c, map[string]interface{}{}, "success")
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
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

	// Check access for each dataset
	for _, datasetID := range datasetIDs {
		if !h.datasetsService.Accessible(datasetID, user.ID) {
			common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "No authorization for dataset: "+datasetID)
			return
		}
	}

	flattenedMeta, err := h.metadataService.GetFlattedMetaByKBs(datasetIDs)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, nil, "Failed to get metadata: "+err.Error())
		return
	}

	common.SuccessWithData(c, flattenedMeta, "success")
}

func (h *DatasetsHandler) UpdateDocumentMetadataConfig(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
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

	data, code, err := h.datasetsService.UpdateDocumentMetadataConfig(userID, datasetID, documentID, req)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.ResponseWithCodeData(c, code, data, "success")
}

// SearchDatasets searches chunks across datasets based on a question
// @Summary Search Datasets
// @Description Search for relevant chunks across one or more datasets based on a question
// @Tags datasets
// @Accept json
// @Produce json
// @Param request body service.SearchDatasetsRequest true "search parameters"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/datasets/search [post]
func (h *DatasetsHandler) SearchDatasets(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
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
	if req.TopK == nil {
		defaultTopK := 1024
		req.TopK = &defaultTopK
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

	resp, err := searchService.SearchDatasets(&req, user.ID)
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
// @Accept json
// @Produce json
// @Param dataset_id path string true "dataset id"
// @Param request body service.SearchDatasetRequest true "search parameters"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/datasets/{dataset_id}/search [post]
func (h *DatasetsHandler) SearchDataset(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
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

	resp, err := searchService.SearchDataset(datasetID, user.ID, &req)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	common.SuccessNoMessage(c, resp)
}

func validateSearchDatasetsRequest(req *service.SearchDatasetsRequest) error {
	return validateSearchParams(req.Page, req.Size, req.TopK, req.SimilarityThreshold, req.VectorSimilarityWeight)
}

func validateSearchDatasetRequest(req *service.SearchDatasetRequest) error {
	return validateSearchParams(req.Page, req.Size, req.TopK, req.SimilarityThreshold, req.VectorSimilarityWeight)
}

func validateSearchParams(page, size, topK *int, similarityThreshold, vectorSimilarityWeight *float64) error {
	if page != nil && *page < 1 {
		return fmt.Errorf("page must be greater than or equal to 1")
	}
	if size != nil && *size < 1 {
		return fmt.Errorf("size must be greater than or equal to 1")
	}
	if topK != nil && *topK < 1 {
		return fmt.Errorf("top_k must be greater than or equal to 1")
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
