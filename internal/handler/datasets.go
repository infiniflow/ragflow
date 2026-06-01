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
	datasetsService  *service.DatasetService
	metadataService  *service.MetadataService
}

type listDatasetsExt struct {
	Keywords string   `json:"keywords,omitempty"`
	OwnerIDs []string `json:"owner_ids,omitempty"`
	ParserID string   `json:"parser_id,omitempty"`
}

// NewDatasetsHandler creates a new datasets handler.
func NewDatasetsHandler(datasetsService *service.DatasetService, metadataService *service.MetadataService) *DatasetsHandler {
	return &DatasetsHandler{
		datasetsService:  datasetsService,
		metadataService:  metadataService,
	}
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

// GetDataset handles GET /api/v1/datasets/:dataset_id.
func (h *DatasetsHandler) GetDataset(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")
	result, code, err := h.datasetsService.GetDataset(datasetID, user.ID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": common.CodeSuccess,
		"data": result,
	})
}

// GetIngestionSummary handles GET /api/v1/datasets/:dataset_id/ingestions/summary.
func (h *DatasetsHandler) GetIngestionSummary(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")
	result, code, err := h.datasetsService.GetIngestionSummary(datasetID, user.ID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// ListIngestionLogs handles GET /api/v1/datasets/:dataset_id/ingestions.
func (h *DatasetsHandler) ListIngestionLogs(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")

	page := 0
	if pageStr := c.Query("page"); pageStr != "" {
		p, err := strconv.Atoi(pageStr)
		if err != nil {
			jsonError(c, common.CodeArgumentError, "page must be an integer")
			return
		}
		page = p
	}

	pageSize := 0
	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		ps, err := strconv.Atoi(pageSizeStr)
		if err != nil {
			jsonError(c, common.CodeArgumentError, "page_size must be an integer")
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
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// GetIngestionLog handles GET /api/v1/datasets/:dataset_id/ingestions/:log_id.
func (h *DatasetsHandler) GetIngestionLog(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")
	logID := c.Param("log_id")
	result, code, err := h.datasetsService.GetIngestionLog(datasetID, user.ID, logID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
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

// GetKnowledgeGraph handles GET /api/v1/datasets/:dataset_id/graph.
func (h *DatasetsHandler) GetKnowledgeGraph(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		jsonError(c, common.CodeDataError, "dataset_id is required")
		return
	}

	dataset, code, err := h.datasetsService.GetDataset(datasetID, user.ID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	tenantID, _ := dataset["tenant_id"].(string)
	if tenantID == "" {
		jsonError(c, common.CodeDataError, "tenant_id is required")
		return
	}

	docEngine := engine.Get()
	if docEngine == nil {
		jsonError(c, common.CodeServerError, "Document engine is not initialized")
		return
	}

	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	exists, err := docEngine.ChunkStoreExists(c.Request.Context(), indexName, datasetID)
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}

	result := gin.H{
		"graph":    map[string]interface{}{},
		"mind_map": map[string]interface{}{},
	}
	if !exists {
		jsonResponse(c, common.CodeSuccess, result, "success")
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
		jsonError(c, common.CodeServerError, err.Error())
		return
	}
	if searchResult == nil || len(searchResult.Chunks) == 0 {
		jsonResponse(c, common.CodeSuccess, result, "success")
		return
	}

	chunk := searchResult.Chunks[0]
	graphType := firstStringValue(chunk["knowledge_graph_kwd"])
	contentWithWeight, _ := chunk["content_with_weight"].(string)
	if strings.TrimSpace(contentWithWeight) == "" {
		jsonResponse(c, common.CodeSuccess, result, "success")
		return
	}

	var graphData map[string]interface{}
	if err := json.Unmarshal([]byte(contentWithWeight), &graphData); err != nil {
		jsonResponse(c, common.CodeSuccess, result, "success")
		return
	}
	if len(graphData) == 0 {
		jsonResponse(c, common.CodeSuccess, result, "success")
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

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// DeleteKnowledgeGraph handles DELETE /api/v1/datasets/:dataset_id/graph.
func (h *DatasetsHandler) DeleteKnowledgeGraph(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		jsonError(c, common.CodeDataError, "dataset_id is required")
		return
	}

	dataset, code, err := h.datasetsService.GetDataset(datasetID, user.ID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	tenantID, _ := dataset["tenant_id"].(string)
	if tenantID == "" {
		jsonError(c, common.CodeDataError, "tenant_id is required")
		return
	}

	docEngine := engine.Get()
	if docEngine == nil {
		jsonError(c, common.CodeServerError, "Document engine is not initialized")
		return
	}

	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	if _, err := docEngine.DeleteChunks(c.Request.Context(), map[string]interface{}{
		"knowledge_graph_kwd": []string{"graph", "subgraph", "entity", "relation", "community_report"},
	}, indexName, datasetID); err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, true, "success")
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
		jsonError(c, errorCode, errorMessage)
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		jsonError(c, common.CodeDataError, "dataset_id is required")
		return
	}

	dataset, code, err := h.datasetsService.GetDataset(datasetID, user.ID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	tenantID, _ := dataset["tenant_id"].(string)
	if tenantID == "" {
		jsonError(c, common.CodeDataError, "tenant_id is required")
		return
	}

	var req struct {
		Tags []string `json:"tags" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	docEngine := engine.Get()
	if docEngine == nil {
		jsonError(c, common.CodeServerError, "Document engine is not initialized")
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
			jsonError(c, common.CodeServerError, "Failed to remove tag: "+err.Error())
			return
		}
	}

	jsonResponse(c, common.CodeSuccess, true, "success")
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
		jsonError(c, errorCode, errorMessage)
		return
	}

	datasetIDsStr := c.Query("dataset_ids")
	if datasetIDsStr == "" {
		jsonError(c, common.CodeDataError, "dataset_ids is required")
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
		jsonError(c, common.CodeDataError, "dataset_ids is required")
		return
	}

	// Check access for each dataset
	for _, datasetID := range datasetIDs {
		if !h.datasetsService.Accessible(datasetID, user.ID) {
			jsonError(c, common.CodeAuthenticationError, "No authorization for dataset: "+datasetID)
			return
		}
	}

	flattenedMeta, err := h.metadataService.GetFlattedMetaByKBs(datasetIDs)
	if err != nil {
		jsonError(c, common.CodeServerError, "Failed to get metadata: "+err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, flattenedMeta, "success")
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
