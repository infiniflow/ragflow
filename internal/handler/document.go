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
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

var IMG_BASE64_PREFIX = "data:image/png;base64,"

// documentServiceIface defines the DocumentService methods used by DocumentHandler.
type documentServiceIface interface {
	CreateDocument(req *service.CreateDocumentRequest) (*entity.Document, error)
	GetDocumentByID(id string) (*service.DocumentResponse, error)
	UpdateDocument(id string, req *service.UpdateDocumentRequest) error
	DeleteDocument(id string) error
	DeleteDocuments(ids []string, deleteAll bool, datasetID, userID string) (int, error)
	ParseDocuments(datasetID, userID string, docIDs []string) ([]*service.ParseDocumentResponse, error)
	StopParseDocuments(datasetID string, docIDs []string) (map[string]interface{}, error)
	ListDocuments(page, pageSize int) ([]*service.DocumentResponse, int64, error)
	ListDocumentsByDatasetID(kbID string, page, pageSize int) ([]*entity.DocumentListItem, int64, error)
	GetDocumentsByAuthorID(authorID, page, pageSize int) ([]*service.DocumentResponse, int64, error)
	GetThumbnail(docID string) (*service.ThumbnailResponse, error)
	GetDocumentImage(imageID string) ([]byte, error)
	GetMetadataSummary(kbID string, docIDs []string) (map[string]interface{}, error)
	SetDocumentMetadata(docID string, meta map[string]interface{}) error
	DeleteDocumentMetadata(docID string, keys []string) error
	DeleteDocumentAllMetadata(docID string) error
	GetDocumentMetadataByID(docID string) (map[string]interface{}, error)
	GetDocumentArtifact(filename string) (*service.ArtifactResponse, error)
	GetDocumentPreview(docID string) (*service.DocumentPreview, error)
	DownloadDocument(datasetID, docID string) (*service.DownloadDocumentResp, error)
	UpdateDatasetDocument(userID, datasetID, documentID string, req *service.UpdateDatasetDocumentRequest, present map[string]bool) (*service.UpdateDatasetDocumentResponse, common.ErrorCode, error)
	ListIngestionTasks(userID string, datasetID *string, page, pageSize int) ([]*entity.IngestionTask, error)
	IngestDocuments(datasetID, userID string, docIDs []string) ([]*service.ParseDocumentResponse, error)
	StopIngestionTasks(tasks []string, userID string) ([]*entity.IngestionTask, error)
	RemoveIngestionTasks(tasks []string, userID string) ([]map[string]string, error)
}

// DocumentHandler document handler
type DocumentHandler struct {
	documentService documentServiceIface
	datasetService  *service.DatasetService
}

// NewDocumentHandler create document handler
func NewDocumentHandler(documentService documentServiceIface, datasetService *service.DatasetService) *DocumentHandler {
	return &DocumentHandler{
		documentService: documentService,
		datasetService:  datasetService,
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

// GetThumbnail Get thumbnails for documents.
func (h *DocumentHandler) GetThumbnail(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	id := c.Query("doc_ids")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": errors.New("invalid document id"),
		})
		return
	}

	result, err := h.documentService.GetThumbnail(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Errorf("thumbnail not found"),
		})
		return
	}

	if result.Thumbnail != nil && *result.Thumbnail != "" {
		newThumbURL := fmt.Sprintf("/api/v1/documents/images/%s-%s", result.KbID, *result.Thumbnail)
		result.Thumbnail = &newThumbURL
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    map[string]interface{}{result.ID: result.Thumbnail},
		"message": "success",
	})
}

// GetDocumentImage returns a document image from object storage.
func (h *DocumentHandler) GetDocumentImage(c *gin.Context) {
	imageID := c.Param("image_id")
	data, err := h.documentService.GetDocumentImage(imageID)
	if err != nil {
		jsonError(c, common.CodeDataError, "Image not found.")
		return
	}

	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(imageID)))
	if contentType == "" {
		contentType = "image/JPEG"
	}
	c.Data(http.StatusOK, contentType, data)
}

func (h *DocumentHandler) GetDocumentArtifact(c *gin.Context) {
	filename := c.Param("filename")
	artifact, err := h.documentService.GetDocumentArtifact(filename)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrArtifactInvalidFilename),
			errors.Is(err, service.ErrArtifactInvalidFileType),
			errors.Is(err, service.ErrArtifactNotFound):
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeDataError,
				"message": err.Error(),
			})
		default:
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeExceptionError,
				"data":    nil,
				"message": err.Error(),
			})
		}
		return
	}

	c.Header("Content-Type", artifact.ContentType)
	if artifact.ForceAttachment {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Content-Disposition", "attachment")
	} else {
		c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, artifact.SafeFilename))
	}
	c.Data(http.StatusOK, artifact.ContentType, artifact.Data)
}

func (h *DocumentHandler) GetDocumentPreview(c *gin.Context) {
	docID := c.Param("id")

	if docID == "" {
		jsonError(c, common.CodeParamError, "id is required")
		return
	}

	preview, err := h.documentService.GetDocumentPreview(docID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"message": "Document not found!",
		})
		return
	}

	ext := utility.GetFileExtension(preview.FileName)
	if preview.ContentType != "" {
		c.Header("Content-Type", preview.ContentType)
	}

	if utility.ShouldForceAttachment(ext, preview.ContentType) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Content-Disposition", "attachment")
	}

	c.Data(http.StatusOK, preview.ContentType, preview.Data)
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

// DeleteDocuments handles DELETE /api/v1/datasets/:dataset_id/documents
func (h *DocumentHandler) DeleteDocuments(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")
	if datasetID == "" {
		jsonError(c, common.CodeArgumentError, "dataset_id is required")
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
	if len(ids) > 0 && req.DeleteAll {
		jsonError(c, common.CodeArgumentError, "should not provide both ids and delete_all")
		return
	}
	if len(ids) == 0 && !req.DeleteAll {
		jsonError(c, common.CodeArgumentError, "should either provide doc ids or set delete_all(true)")
		return
	}

	userID := c.GetString("user_id")
	deleted, err := h.documentService.DeleteDocuments(ids, req.DeleteAll, datasetID, userID)
	if err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, map[string]interface{}{"deleted": deleted}, "success")
}

// ListDocuments document list

func (h *DocumentHandler) ListDocuments(c *gin.Context) {

	datasetID := c.Param("dataset_id")
	pageStr := c.Query("page")
	pageSizeStr := c.Query("page_size")
	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)

	userID := c.GetString("user_id")

	if !h.datasetService.Accessible(datasetID, userID) {
		jsonError(c, common.CodeAuthenticationError, "No authorization.")
		return
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// Use kbID to filter documents
	documents, total, err := h.documentService.ListDocumentsByDatasetID(datasetID, page, pageSize)
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

		docs = append(docs, mapDocumentListItem(doc, metaFields))
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

func (h *DocumentHandler) DownloadDocument(c *gin.Context) {
	datasetID := c.Param("dataset_id")
	docID := c.Param("document_id")

	if docID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"message": "Specify document_id please.",
		})
		return
	}
	if datasetID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"message": fmt.Sprintf("The dataset not own the document %s.", docID),
		})
		return
	}

	res, err := h.documentService.DownloadDocument(datasetID, docID)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"message": err.Error(),
		})
		return
	}

	c.Header("Content-Type", res.ContentType)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, res.FileName))
	c.Data(http.StatusOK, res.ContentType, res.Data)
}

func mapDocumentListItem(doc *entity.DocumentListItem, metaFields map[string]interface{}) map[string]interface{} {
	item := map[string]interface{}{
		"id":               doc.ID,
		"dataset_id":       doc.KbID,
		"name":             stringValue(doc.Name),
		"thumbnail":        stringValue(doc.Thumbnail),
		"size":             doc.Size,
		"type":             doc.Type,
		"created_by":       doc.CreatedBy,
		"location":         stringValue(doc.Location),
		"token_count":      doc.TokenNum,
		"chunk_count":      doc.ChunkNum,
		"progress":         doc.Progress,
		"progress_msg":     stringValue(doc.ProgressMsg),
		"process_begin_at": formatTimePtr(doc.ProcessBeginAt),
		"process_duration": doc.ProcessDuration,
		"suffix":           doc.Suffix,
		"run":              mapRunStatus(doc.Run),
		"status":           stringValue(doc.Status),
		"chunk_method":     doc.ParserID,
		"parser_id":        doc.ParserID,
		"pipeline_id":      stringValue(doc.PipelineID),
		"pipeline_name":    stringValue(doc.PipelineName),
		"nickname":         stringValue(doc.Nickname),
		"parser_config":    decodeJSONMap(string(doc.ParserConfig)),
		"meta_fields":      metaFields,
		"create_time":      int64(0),
		"create_date":      "",
		"update_time":      int64(0),
		"update_date":      "",
	}

	if doc.CreateTime != nil {
		item["create_time"] = *doc.CreateTime
	}
	if doc.CreateDate != nil {
		item["create_date"] = doc.CreateDate.Format("2006-01-02 15:04:05")
	}
	if doc.UpdateTime != nil {
		item["update_time"] = *doc.UpdateTime
	}
	if doc.UpdateDate != nil {
		item["update_date"] = doc.UpdateDate.Format("2006-01-02 15:04:05")
	}

	return item
}

func decodeJSONMap(raw string) map[string]interface{} {
	if strings.TrimSpace(raw) == "" {
		return map[string]interface{}{}
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return map[string]interface{}{}
	}

	return data
}

func mapRunStatus(run *string) string {
	if run == nil {
		return "UNSTART"
	}

	switch strings.TrimSpace(*run) {
	case "0":
		return "UNSTART"
	case "1":
		return "RUNNING"
	case "2":
		return "CANCEL"
	case "3":
		return "DONE"
	case "4":
		return "FAIL"
	default:
		return strings.TrimSpace(*run)
	}
}

func formatTimePtr(value *time.Time) string {
	if value == nil {
		return ""
	}

	return value.Format("2006-01-02 15:04:05")
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}

	return *value
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

// DeleteMetaRequest represents the request for deleting document metadata
type DeleteMetaRequest struct {
	DocID string `json:"doc_id" binding:"required"`
	Keys  string `json:"keys"` // optional - if provided, deletes specific keys; otherwise deletes entire document metadata
}

// DeleteMeta handles the delete metadata request for a document
// If Keys is provided, deletes specific metadata keys; otherwise deletes entire document metadata
// @Summary Delete Document Metadata
// @Description Delete metadata keys or entire document metadata for a specific document
// @Tags documents
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body DeleteMetaRequest true "metadata keys to delete or empty to delete all"
// @Success 200 {object} map[string]interface{}
// @Router /v1/document/delete_meta [post]
func (h *DocumentHandler) DeleteMeta(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req DeleteMetaRequest
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

	// Authorization: user must be able to access the document's dataset.
	doc, err := h.documentService.GetDocumentByID(req.DocID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    1,
			"message": "document not found",
		})
		return
	}
	if !h.datasetService.Accessible(doc.KbID, user.ID) {
		jsonError(c, common.CodeAuthenticationError, "No authorization.")
		return
	}

	// If Keys is provided, parse and delete specific keys; otherwise delete entire document metadata
	if req.Keys != "" {
		// Parse keys JSON string - expected to be a list of key names to delete
		var keys []string
		if err := json.Unmarshal([]byte(req.Keys), &keys); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    1,
				"message": "Json syntax error: " + err.Error(),
			})
			return
		}

		if keys == nil || len(keys) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    1,
				"message": "keys list is required",
			})
			return
		}

		err := h.documentService.DeleteDocumentMetadata(req.DocID, keys)
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
					"message": "Failed to delete metadata: " + errMsg,
				})
			}
			return
		}
	} else {
		// Delete entire document metadata
		err := h.documentService.DeleteDocumentAllMetadata(req.DocID)
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
					"message": "Failed to delete metadata: " + errMsg,
				})
			}
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    true,
	})
}

type ListIngestionsRequest struct {
	DatasetID *string `json:"dataset_id"`
}

func (h *DocumentHandler) ListIngestionTasks(c *gin.Context) {
	var req ListIngestionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	userID := c.GetString("user_id")

	var parseResult []*entity.IngestionTask
	var err error
	if req.DatasetID != nil {
		if !h.datasetService.Accessible(*req.DatasetID, userID) {
			jsonError(c, common.CodeAuthenticationError, "No authorization to access the dataset.")
			return
		}
	}

	parseResult, err = h.documentService.ListIngestionTasks(userID, req.DatasetID, 0, 0)
	if err != nil {
		jsonError(c, common.CodeExceptionError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    parseResult,
	})
}

type StartParseDocumentsRequest struct {
	DatasetID string   `json:"dataset_id" binding:"required"`
	Documents []string `json:"documents" binding:"required"`
}

func (h *DocumentHandler) StartIngestionTask(c *gin.Context) {
	var req StartParseDocumentsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	userID := c.GetString("user_id")

	if !h.datasetService.Accessible(req.DatasetID, userID) {
		jsonError(c, common.CodeAuthenticationError, "No authorization to access the dataset.")
		return
	}

	parseResult, err := h.documentService.IngestDocuments(req.DatasetID, userID, req.Documents)
	if err != nil {
		jsonError(c, common.CodeExceptionError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    parseResult,
	})
}

type StopIngestionsRequest struct {
	Tasks []string `json:"tasks" binding:"required"`
}

func (h *DocumentHandler) StopIngestionTasks(c *gin.Context) {
	var req StopIngestionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	userID := c.GetString("user_id")

	parseResult, err := h.documentService.StopIngestionTasks(req.Tasks, userID)
	if err != nil {
		jsonError(c, common.CodeExceptionError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    parseResult,
	})
}

type RemoveIngestionsRequest struct {
	Tasks []string `json:"tasks" binding:"required"`
}

func (h *DocumentHandler) RemoveIngestionTasks(c *gin.Context) {
	var req RemoveIngestionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	if req.Tasks == nil || len(req.Tasks) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"code":    1,
			"message": "task_ids is required",
		})
		return
	}

	userID := c.GetString("user_id")

	deletedTasks, err := h.documentService.RemoveIngestionTasks(req.Tasks, userID)
	if err != nil {
		jsonError(c, common.CodeExceptionError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    deletedTasks,
	})
}

type ParseDocumentRequest struct {
	Documents []string `json:"documents" binding:"required"`
}

func (h *DocumentHandler) ParseDocuments(c *gin.Context) {
	datasetID := c.Param("dataset_id")

	var req ParseDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	userID := c.GetString("user_id")

	if !h.datasetService.Accessible(datasetID, userID) {
		jsonError(c, common.CodeAuthenticationError, "No authorization to access the dataset.")
		return
	}

	parseResult, err := h.documentService.ParseDocuments(datasetID, userID, req.Documents)
	if err != nil {
		jsonError(c, common.CodeExceptionError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    parseResult,
	})
}

type StopParseDocumentRequest struct {
	DocumentIDs []string `json:"document_ids" binding:"required"`
}

func (h *DocumentHandler) StopParseDocuments(c *gin.Context) {
	datasetID := c.Param("dataset_id")

	var req StopParseDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	if len(req.DocumentIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": "`document_ids` is required",
		})
		return
	}

	userID := c.GetString("user_id")

	if !h.datasetService.Accessible(datasetID, userID) {
		jsonError(c, common.CodeAuthenticationError, "You don't own the dataset.")
		return
	}

	result, err := h.documentService.StopParseDocuments(datasetID, req.DocumentIDs)
	if err != nil {
		jsonError(c, common.CodeExceptionError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    result,
	})
}

func (h *DocumentHandler) MetadataSummaryByDataset(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")
	if datasetID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": "dataset_id is required",
		})
		return
	}
	if !h.datasetService.Accessible(datasetID, user.ID) {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"message": "You don't own the dataset " + datasetID,
		})
		return
	}

	var docIDS []string
	if docIDsParam := c.Query("doc_ids"); docIDsParam != "" {
		docIDS = strings.Split(docIDsParam, ",")
	}

	summary, err := h.documentService.GetMetadataSummary(datasetID, docIDS)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    common.CodeServerError,
			"message": "Failed to  get metadata summary" + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    gin.H{"summary": summary},
	})
}

func (h *DocumentHandler) UpdateDatasetDocument(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		jsonError(c, common.CodeArgumentError, "dataset_id is required")
		return
	}
	documentID := strings.TrimSpace(c.Param("document_id"))
	if documentID == "" {
		jsonError(c, common.CodeArgumentError, "document_id is required")
		return
	}

	body, err := c.GetRawData()
	if err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}
	present := make(map[string]bool, len(raw))
	for key := range raw {
		present[key] = true
	}
	var req service.UpdateDatasetDocumentRequest
	if err := json.Unmarshal(body, &req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	data, code, err := h.documentService.UpdateDatasetDocument(user.ID, datasetID, documentID, &req, present)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": common.CodeSuccess,
		"data": data,
	})
}
