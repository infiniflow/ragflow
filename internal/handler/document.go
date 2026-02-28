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
	"strconv"

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
// @Router /api/v1/documents [get]
func (h *DocumentHandler) ListDocuments(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	documents, total, err := h.documentService.ListDocuments(page, pageSize)
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
