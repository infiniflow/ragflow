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
	"ragflow/internal/common"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

// DocumentHandler document handler
type DocumentHandler struct {
	documentService *service.DocumentService
}

// NewDocumentHandler create document handler
func NewDocumentHandler(documentService *service.DocumentService) *DocumentHandler {
	return &DocumentHandler{
		documentService: documentService,
	}
}

// CreateDocument create document
// @Summary Create Document
// @Description Create new document
// @Tags documents
// @Accept json
// @Produce json
// @Param request body service.CreateDocumentRequest true "document info"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/documents [post]
func (h *DocumentHandler) CreateDocument(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req service.CreateDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	document, err := h.documentService.CreateDocument(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "created successfully",
		"data":    document,
	})
}

// GetDocumentByID get document by ID
// @Summary Get Document Info
// @Description Get document details by ID
// @Tags documents
// @Accept json
// @Produce json
// @Param id path int true "document ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/documents/{id} [get]
func (h *DocumentHandler) GetDocumentByID(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid document id",
		})
		return
	}

	document, err := h.documentService.GetDocumentByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "document not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": document,
	})
}

// UpdateDocument update document
// @Summary Update Document
// @Description Update document info
// @Tags documents
// @Accept json
// @Produce json
// @Param id path int true "document ID"
// @Param request body service.UpdateDocumentRequest true "update info"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/documents/{id} [put]
func (h *DocumentHandler) UpdateDocument(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid document id",
		})
		return
	}

	var req service.UpdateDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	if err := h.documentService.UpdateDocument(id, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "updated successfully",
	})
}

// DeleteDocument delete document
// @Summary Delete Document
// @Description Delete specified document
// @Tags documents
// @Accept json
// @Produce json
// @Param id path int true "document ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/documents/{id} [delete]
func (h *DocumentHandler) DeleteDocument(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid document id",
		})
		return
	}

	if err := h.documentService.DeleteDocument(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "deleted successfully",
	})
}

// ListDocuments document list
// @Summary Document List
// @Description Get paginated document list
// @Tags documents
// @Accept json
// @Produce json
// @Param page query int false "page number" default(1)
// @Param page_size query int false "items per page" default(10)
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/document/list [post]
func (h *DocumentHandler) ListDocuments(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	kbID := c.Query("kb_id")
	if kbID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    1,
			"message": "Lack of KB ID",
			"data":    false,
		})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// Use kbID to filter documents
	documents, total, err := h.documentService.ListDocumentsByKBID(kbID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    1,
			"message": "failed to get documents",
			"data":    map[string]interface{}{"total": 0, "docs": []interface{}{}},
		})
		return
	}

	docs := make([]map[string]interface{}, 0, len(documents))
	for _, doc := range documents {
		metaFields, err := h.documentService.GetDocumentMetadataByID(doc.ID)
		if err != nil {
			metaFields = make(map[string]interface{})
		}

		docs = append(docs, map[string]interface{}{
			"id":          doc.ID,
			"name":        doc.Name,
			"size":        doc.Size,
			"type":        doc.Type,
			"status":      doc.Status,
			"created_at":  doc.CreatedAt,
			"meta_fields": metaFields,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"total": total,
			"docs":  docs,
		},
	})
}

// GetDocumentsByAuthorID get documents by author ID
// @Summary Get Author Documents
// @Description Get paginated document list by author ID
// @Tags documents
// @Accept json
// @Produce json
// @Param author_id path int true "author ID"
// @Param page query int false "page number" default(1)
// @Param page_size query int false "items per page" default(10)
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/authors/{author_id}/documents [get]
func (h *DocumentHandler) GetDocumentsByAuthorID(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	authorIDStr := c.Param("author_id")
	authorID, err := strconv.Atoi(authorIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid author id",
		})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	documents, total, err := h.documentService.GetDocumentsByAuthorID(authorID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get documents",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"items":     documents,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// MetadataSummary handles the metadata summary request
func (h *DocumentHandler) MetadataSummary(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var requestBody struct {
		KBID   string   `json:"kb_id" binding:"required"`
		DocIDs []string `json:"doc_ids"`
	}

	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    1,
			"message": "kb_id is required",
		})
		return
	}

	kbID := requestBody.KBID
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    1,
			"message": "kb_id is required",
		})
		return
	}

	summary, err := h.documentService.GetMetadataSummary(kbID, requestBody.DocIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    1,
			"message": "Failed to get metadata summary: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"summary": summary,
		},
	})
}

// SetMetaRequest represents the request for setting document metadata
type SetMetaRequest struct {
	DocID string `json:"doc_id" binding:"required"`
	Meta  string `json:"meta" binding:"required"`
}

// SetMeta handles the set metadata request for a document
// @Summary Set Document Metadata
// @Description Set metadata for a specific document
// @Tags documents
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body SetMetaRequest true "metadata info"
// @Success 200 {object} map[string]interface{}
// @Router /v1/document/set_meta [post]
func (h *DocumentHandler) SetMeta(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req SetMetaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    1,
			"message": err.Error(),
		})
		return
	}

	if req.DocID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    1,
			"message": "doc_id is required",
		})
		return
	}

	// Parse meta JSON string
	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(req.Meta), &meta); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    1,
			"message": "Json syntax error: " + err.Error(),
		})
		return
	}

	if meta == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    1,
			"message": "meta is required",
		})
		return
	}

	// Validate meta values - must be str, int, float, or list of those
	for k, v := range meta {
		switch val := v.(type) {
		case string, int, float64:
			// Valid
		case []interface{}:
			for _, item := range val {
				if _, ok := item.(string); !ok {
					if _, ok := item.(float64); !ok {
						c.JSON(http.StatusBadRequest, gin.H{
							"code":    1,
							"message": fmt.Sprintf("Unsupported type in list for key %s: %T", k, item),
						})
						return
					}
				}
			}
		default:
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    1,
				"message": fmt.Sprintf("Unsupported type for key %s: %T", k, v),
			})
			return
		}
	}

	err := h.documentService.SetDocumentMetadata(req.DocID, meta)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "no such document") || strings.Contains(errMsg, "document not found") {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    1,
				"message": errMsg,
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    1,
				"message": "Failed to set metadata: " + errMsg,
			})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    true,
	})
}