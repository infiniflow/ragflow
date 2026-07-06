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
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ragflow/internal/dao"
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
	ListDocumentsByDatasetID(kbID, keywords string, page, pageSize int) ([]*entity.DocumentListItem, int64, error)
	ListDocumentsByDatasetIDWithOptions(opts dao.DocumentListOptions, page, pageSize int) ([]*entity.DocumentListItem, int64, error)
	ListDocumentIDsByDatasetIDWithOptions(opts dao.DocumentListOptions) ([]string, error)
	GetDocumentFiltersByDatasetID(opts dao.DocumentListOptions) (map[string]interface{}, int64, error)
	GetMetadataByKBs(kbIDs []string) (map[string]interface{}, error)
	GetDocumentsByAuthorID(authorID, page, pageSize int) ([]*service.DocumentResponse, int64, error)
	GetThumbnails(userID string, docIDs []string) (map[string]string, error)
	GetDocumentImage(imageID string) ([]byte, error)
	GetMetadataSummary(kbID string, docIDs []string) (map[string]interface{}, error)
	SetDocumentMetadata(docID string, meta map[string]interface{}) error
	DeleteDocumentMetadata(docID string, keys []string) error
	DeleteDocumentAllMetadata(docID string) error
	GetDocumentMetadataByID(docID string) (map[string]interface{}, error)
	GetDocumentArtifact(filename, userID string) (*service.ArtifactResponse, error)
	GetDocumentPreview(docID string) (*service.DocumentPreview, error)
	UploadLocalDocuments(kb *entity.Knowledgebase, tenantID string, files []*multipart.FileHeader, parentPath string, parserConfigOverride map[string]interface{}) ([]map[string]interface{}, []string)
	UploadWebDocument(kb *entity.Knowledgebase, tenantID, name, url string) (map[string]interface{}, common.ErrorCode, error)
	UploadEmptyDocument(kb *entity.Knowledgebase, tenantID, name string) (map[string]interface{}, common.ErrorCode, error)
	DownloadDocument(datasetID, docID string) (*service.DownloadDocumentResp, error)
	UpdateDatasetDocument(userID, datasetID, documentID string, req *service.UpdateDatasetDocumentRequest, present map[string]bool) (*service.UpdateDatasetDocumentResponse, common.ErrorCode, error)
	BatchUpdateDocumentMetadatas(datasetID string, selector *service.DocumentMetadataSelector, updates []service.DocumentMetadataUpdate, deletes []service.DocumentMetadataDelete) (*service.BatchUpdateDocumentMetadatasResponse, common.ErrorCode, error)
	UploadDocumentInfos(userID string, files []*multipart.FileHeader) ([]map[string]interface{}, common.ErrorCode, error)
	UploadDocumentInfoByURL(userID, rawURL string) (map[string]interface{}, common.ErrorCode, error)
	ListIngestionTasks(userID string, datasetID *string, page, pageSize int) ([]*entity.IngestionTask, error)
	IngestDocuments(datasetID, userID string, docIDs []string) ([]*service.ParseDocumentResponse, error)
	StopIngestionTasks(tasks []string, userID string) ([]*entity.IngestionTask, error)
	Ingest(userID string, req *service.IngestDocumentRequest) (common.ErrorCode, error)
	RemoveIngestionTasks(tasks []string, userID string) ([]map[string]string, error)
	BatchUpdateDocumentStatus(userID, datasetID, status string, DocumentIDs []string) (map[string]interface{}, common.ErrorCode, error)
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
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

	common.SuccessWithData(c, document, "created successfully")
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
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
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	docIDs := parseThumbnailDocIDs(c)
	if len(docIDs) == 0 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, `Lack of "Document ID"`)
		return
	}

	result, err := h.documentService.GetThumbnails(user.ID, docIDs)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, nil, err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

func parseThumbnailDocIDs(c *gin.Context) []string {
	rawValues := c.QueryArray("doc_ids")
	seen := make(map[string]struct{}, len(rawValues))
	docIDs := make([]string, 0, len(rawValues))

	for _, raw := range rawValues {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		docIDs = append(docIDs, id)
	}

	return docIDs
}

// GetDocumentImage returns a document image from object storage.
func (h *DocumentHandler) GetDocumentImage(c *gin.Context) {
	imageID := c.Param("image_id")
	data, err := h.documentService.GetDocumentImage(imageID)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "Image not found.")
		return
	}

	contentType := documentImageContentType(imageID, data)
	c.Data(http.StatusOK, contentType, data)
}

func documentImageContentType(imageID string, data []byte) string {
	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(imageID))); strings.HasPrefix(contentType, "image/") {
		return contentType
	}
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case len(data) >= 3 && bytes.Equal(data[:3], []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg"
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return "image/gif"
	case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp"
	case bytes.HasPrefix(data, []byte("BM")):
		return "image/bmp"
	default:
		return "application/octet-stream"
	}
}

func (h *DocumentHandler) GetDocumentArtifact(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	filename := c.Param("filename")
	artifact, err := h.documentService.GetDocumentArtifact(filename, user.ID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrArtifactInvalidFilename),
			errors.Is(err, service.ErrArtifactInvalidFileType),
			errors.Is(err, service.ErrArtifactNotFound):
			common.ErrorWithCode(c, int(common.CodeDataError), err.Error())

		default:
			common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
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
		common.ResponseWithCodeData(c, common.CodeParamError, nil, "id is required")
		return
	}

	preview, err := h.documentService.GetDocumentPreview(docID)
	if err != nil {
		common.ErrorWithCode(c, int(common.CodeDataError), "Document not found!")
		return
	}

	ext := utility.GetFileExtension(preview.FileName)
	// Use the shared preview-headers helper so that safe types get
	// Content-Disposition: inline with filename, while dangerous
	// types (HTML, SVG, XML) fall back to forced attachment with
	// nosniff. Mirrors Python document_api.py:2063 which calls
	// apply_preview_file_response_headers() with the document name.
	utility.SetPreviewFileResponseHeaders(c.Writer.Header(), preview.ContentType, ext, preview.FileName)

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
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid document id",
		})
		return
	}

	doc, err := h.documentService.GetDocumentByID(id)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, "document not found!")
		return
	}
	if !h.datasetService.Accessible(doc.KbID, user.ID) {
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "No authorization.")
		return
	}

	var req service.UpdateDocumentRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	if err = h.documentService.UpdateDocument(id, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	common.SuccessWithMessage(c, "updated successfully")
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
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid document id",
		})
		return
	}

	doc, err := h.documentService.GetDocumentByID(id)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, "document not found!")
		return
	}
	if !h.datasetService.Accessible(doc.KbID, user.ID) {
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "No authorization.")
		return
	}

	if err = h.documentService.DeleteDocument(id); err != nil {
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")
	if datasetID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "dataset_id is required")
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
	if len(ids) > 0 && req.DeleteAll {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "should not provide both ids and delete_all")
		return
	}
	if len(ids) == 0 && !req.DeleteAll {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "should either provide doc ids or set delete_all(true)")
		return
	}

	userID := c.GetString("user_id")
	deleted, err := h.documentService.DeleteDocuments(ids, req.DeleteAll, datasetID, userID)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	common.SuccessWithData(c, map[string]interface{}{"deleted": deleted}, "success")
}

// BatchUpdateDocumentStatus Batch update status of documents within a dataset.
func (h *DocumentHandler) BatchUpdateDocumentStatus(c *gin.Context) {
	user, code, errorMessage := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, errorMessage)
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "invalid user id")
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "dataset_id is required")
		return
	}

	var req struct {
		DocumentIDs []interface{} `json:"doc_ids"`
		Status      interface{}   `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	if req.DocumentIDs == nil || len(req.DocumentIDs) == 0 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, `"doc_ids" must be a non-empty list.`)
		return
	}
	documentIDs := make([]string, 0, len(req.DocumentIDs))
	for _, rawDocID := range req.DocumentIDs {
		docID, ok := rawDocID.(string)
		if !ok || strings.TrimSpace(docID) == "" {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
				`"doc_ids" must contain non-empty document IDs.`)
			return
		}
		documentIDs = append(documentIDs, docID)
	}

	status := "-1"
	if req.Status != nil {
		status = fmt.Sprint(req.Status)
	}
	if status != "0" && status != "1" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, fmt.Sprintf(`"Status" must be either 0 or 1:%s!`, status))
		return
	}

	result, code, err := h.documentService.BatchUpdateDocumentStatus(userID, datasetID, status, documentIDs)
	if err != nil {
		message := err.Error()
		if code == common.CodeServerError {
			message = "Partial failure"
		}
		common.ResponseWithCodeData(c, code, result, message)
		return
	}

	common.ResponseWithCodeData(c, code, result, "success")
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
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "No authorization to access the dataset.")
		return
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	opts, errMsg := parseDocumentListOptions(c, datasetID)
	if errMsg != "" {
		common.ResponseWithCodeData(c, common.CodeDataError, map[string]interface{}{"total": 0, "docs": []interface{}{}}, errMsg)
		return
	}
	opts, errMsg = h.applyDocumentMetadataFilter(c, opts)
	if errMsg != "" {
		common.ResponseWithCodeData(c, common.CodeDataError, map[string]interface{}{"total": 0, "docs": []interface{}{}}, errMsg)
		return
	}

	if c.Query("type") == "filter" {
		filters, total, err := h.documentService.GetDocumentFiltersByDatasetID(opts)
		if err != nil {
			common.ResponseWithCodeData(c, common.CodeExceptionError, map[string]interface{}{"total": 0, "filter": map[string]interface{}{}}, "failed to get document filters")
			return
		}
		common.SuccessWithData(c, gin.H{"total": total, "filter": filters}, "success")
		return
	}

	// Use kbID to filter documents
	documents, total, err := h.documentService.ListDocumentsByDatasetIDWithOptions(opts, page, pageSize)
	if err != nil {
		common.ResponseWithCodeData(c, 1, map[string]interface{}{"total": 0, "docs": []interface{}{}}, "failed to get documents")
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

	common.SuccessWithData(c, gin.H{"total": total, "docs": docs}, "success")
}

func parseDocumentListOptions(c *gin.Context, datasetID string) (dao.DocumentListOptions, string) {
	opts := dao.DocumentListOptions{
		KbID:     datasetID,
		Keywords: c.Query("keywords"),
		OrderBy:  c.DefaultQuery("orderby", "create_time"),
		Desc:     strings.ToLower(strings.TrimSpace(c.DefaultQuery("desc", "true"))) != "false",
		Suffixes: queryValues(c, "suffix"),
		Types:    queryValues(c, "types"),
	}

	opts.RunStatuses = normalizeRunStatusFilter(queryValues(c, "run", "run_status"))
	if len(queryValues(c, "run", "run_status")) > 0 && len(opts.RunStatuses) == 0 {
		return opts, "Invalid filter run status conditions"
	}

	opts.Name = c.Query("name")
	docID := c.Query("id")
	docIDs := queryValues(c, "ids")
	if docID != "" && len(docIDs) > 0 {
		return opts, fmt.Sprintf("Should not provide both 'id':%s and 'ids'%v", docID, docIDs)
	}
	if docID != "" {
		opts.DocIDs = []string{docID}
		opts.DocIDFilterApplied = true
	} else if len(docIDs) > 0 {
		opts.DocIDs = docIDs
		opts.DocIDFilterApplied = true
	}

	if v := c.Query("create_time_from"); v != "" {
		createTimeFrom, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return opts, "create_time_from must be an integer"
		}
		opts.CreateTimeFrom = createTimeFrom
	}
	if v := c.Query("create_time_to"); v != "" {
		createTimeTo, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return opts, "create_time_to must be an integer"
		}
		opts.CreateTimeTo = createTimeTo
	}

	return opts, ""
}

func (h *DocumentHandler) applyDocumentMetadataFilter(c *gin.Context, opts dao.DocumentListOptions) (dao.DocumentListOptions, string) {
	metadata, err := parseMetadataQuery(c.Request.URL.Query())
	if err != nil {
		return opts, err.Error()
	}
	returnEmptyMetadata := strings.ToLower(strings.TrimSpace(c.Query("return_empty_metadata"))) == "true"
	if !returnEmptyMetadata && len(metadata) == 0 {
		return opts, ""
	}

	candidateIDs, err := h.documentService.ListDocumentIDsByDatasetIDWithOptions(opts)
	if err != nil {
		return opts, "failed to get documents"
	}
	candidateSet := stringSet(candidateIDs)

	metadataByKey, err := h.documentService.GetMetadataByKBs([]string{opts.KbID})
	if err != nil {
		return opts, err.Error()
	}

	docIDsWithMetadata := map[string]bool{}
	matchedIDs := map[string]bool{}
	firstMetadataKey := true
	for key, values := range metadata {
		valueMatches := map[string]bool{}
		rawValues, _ := metadataByKey[key].(map[string][]string)
		for _, value := range values {
			for _, docID := range rawValues[value] {
				valueMatches[docID] = true
				docIDsWithMetadata[docID] = true
			}
		}
		if firstMetadataKey {
			matchedIDs = valueMatches
			firstMetadataKey = false
		} else {
			matchedIDs = intersectStringSets(matchedIDs, valueMatches)
		}
	}
	if returnEmptyMetadata {
		for _, rawValue := range metadataByKey {
			values, _ := rawValue.(map[string][]string)
			for _, docIDs := range values {
				for _, docID := range docIDs {
					docIDsWithMetadata[docID] = true
				}
			}
		}
	}

	filteredIDs := make([]string, 0)
	if returnEmptyMetadata {
		for _, docID := range candidateIDs {
			if !docIDsWithMetadata[docID] {
				filteredIDs = append(filteredIDs, docID)
			}
		}
	} else {
		for docID := range matchedIDs {
			if candidateSet[docID] {
				filteredIDs = append(filteredIDs, docID)
			}
		}
	}

	opts.DocIDs = filteredIDs
	opts.DocIDFilterApplied = true
	return opts, ""
}

func parseMetadataQuery(values url.Values) (map[string][]string, error) {
	metadata := map[string][]string{}
	if raw := strings.TrimSpace(values.Get("metadata")); raw != "" {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return nil, fmt.Errorf("metadata must be valid JSON")
		}
		for key, value := range parsed {
			for _, item := range interfaceToStringSlice(value) {
				metadata[key] = append(metadata[key], item)
			}
		}
	}

	for key, vals := range values {
		if !strings.HasPrefix(key, "metadata[") || !strings.HasSuffix(key, "]") {
			continue
		}
		name := strings.TrimPrefix(key, "metadata[")
		if end := strings.Index(name, "]"); end >= 0 {
			name = name[:end]
		}
		if name == "" || name == "empty_metadata" {
			continue
		}
		for _, value := range vals {
			for _, item := range interfaceToStringSlice(value) {
				metadata[name] = append(metadata[name], item)
			}
		}
	}
	return metadata, nil
}

func interfaceToStringSlice(value interface{}) []string {
	switch typed := value.(type) {
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if item == nil {
				continue
			}
			if s := strings.TrimSpace(fmt.Sprintf("%v", item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s := strings.TrimSpace(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{strings.TrimSpace(typed)}
	default:
		if value == nil {
			return nil
		}
		return []string{fmt.Sprintf("%v", value)}
	}
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

func intersectStringSets(left, right map[string]bool) map[string]bool {
	out := make(map[string]bool)
	for value := range left {
		if right[value] {
			out[value] = true
		}
	}
	return out
}

func queryValues(c *gin.Context, names ...string) []string {
	values := make([]string, 0)
	for _, name := range names {
		values = append(values, c.QueryArray(name)...)
		values = append(values, c.QueryArray(name+"[]")...)
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func normalizeRunStatusFilter(statuses []string) []string {
	if len(statuses) == 0 {
		return nil
	}
	statusTextToNumeric := map[string]string{
		"UNSTART": string(entity.TaskStatusUnstart),
		"RUNNING": string(entity.TaskStatusRunning),
		"CANCEL":  string(entity.TaskStatusCancel),
		"DONE":    string(entity.TaskStatusDone),
		"FAIL":    string(entity.TaskStatusFail),
	}
	validStatuses := map[string]bool{
		string(entity.TaskStatusUnstart): true,
		string(entity.TaskStatusRunning): true,
		string(entity.TaskStatusCancel):  true,
		string(entity.TaskStatusDone):    true,
		string(entity.TaskStatusFail):    true,
	}
	out := make([]string, 0, len(statuses))
	for _, status := range statuses {
		normalized := statusTextToNumeric[strings.ToUpper(status)]
		if normalized == "" {
			normalized = status
		}
		if !validStatuses[normalized] {
			return nil
		}
		out = append(out, normalized)
	}
	return out
}

func (h *DocumentHandler) UploadDocuments(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}
	tenantID := user.ID
	datasetID := c.Param("dataset_id")
	uploadType := strings.ToLower(c.DefaultQuery("type", "local"))

	kb, err := h.datasetService.GetKnowledgebaseByID(datasetID)
	if err != nil || kb == nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, fmt.Sprintf("Can't find the dataset with ID %s!", datasetID))
		return
	}
	if !h.datasetService.CheckKBTeamPermission(kb, tenantID) {
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "No authorization.")
		return
	}

	switch uploadType {
	case "web":
		h.uploadWebDocument(c, kb, tenantID)
	case "empty":
		h.uploadEmptyDocument(c, kb, tenantID)
	case "local":
		h.uploadLocalDocuments(c, kb, tenantID)
	default:
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, `"type" must be one of "local", "web", or "empty".`)
	}
}

func (h *DocumentHandler) uploadLocalDocuments(c *gin.Context, kb *entity.Knowledgebase, tenantID string) {
	form, err := c.MultipartForm()
	if err != nil || form == nil || len(form.File["file"]) == 0 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "No file part!")
		return
	}
	files := form.File["file"]
	for _, fh := range files {
		if fh == nil || fh.Filename == "" {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
				"No file selected!")
			return
		}
		if len([]byte(fh.Filename)) > 255 {
			common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
				"File name must be 255 bytes or less.")
			return
		}
	}

	// Optional parser_config override — only the allow-listed table column keys.
	// Python ignores malformed or non-object input here instead of failing the
	// whole upload request.
	var override map[string]interface{}
	if raw := strings.TrimSpace(c.PostForm("parser_config")); raw != "" {
		var parsed map[string]interface{}
		if err = json.Unmarshal([]byte(raw), &parsed); err == nil && parsed != nil {
			override = map[string]interface{}{}
			for _, k := range []string{"table_column_mode", "table_column_roles"} {
				if v, ok := parsed[k]; ok {
					override[k] = v
				}
			}
			if len(override) == 0 {
				override = nil
			}
		}
	}

	data, errMsgs := h.documentService.UploadLocalDocuments(kb, tenantID, files, c.PostForm("parent_path"), override)
	if len(data) == 0 && len(errMsgs) > 0 {
		common.ResponseWithCodeData(c, common.CodeServerError, nil, strings.Join(errMsgs, "\n"))
		return
	}
	if len(data) == 0 {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "There seems to be an issue with your file format. please verify it is correct and not corrupted.")
		return
	}

	if strings.ToLower(c.DefaultQuery("return_raw_files", "false")) == "true" {
		if len(errMsgs) > 0 {
			common.ResponseWithCodeData(c, common.CodeServerError, data, strings.Join(errMsgs, "\n"))
			return
		}
		common.SuccessNoMessage(c, data)
		return
	}
	mapped := make([]map[string]interface{}, len(data))
	for i, d := range data {
		mapped[i] = mapDocKeysWithRunStatus(d)
	}
	if len(errMsgs) > 0 {
		common.ResponseWithCodeData(c, common.CodeServerError, mapped, strings.Join(errMsgs, "\n"))
		return
	}
	common.SuccessNoMessage(c, mapped)
}

func (h *DocumentHandler) uploadEmptyDocument(c *gin.Context, kb *entity.Knowledgebase, tenantID string) {
	var req struct {
		Name string `json:"name"`
	}
	// An empty body is valid (falls through to the name-required check below);
	// a non-empty but malformed body should report the syntax error, not a
	// misleading "File name can't be empty."
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Invalid JSON body: "+err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "File name can't be empty.")
		return
	}
	if len([]byte(name)) > 255 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "File name must be 255 bytes or less.")
		return
	}
	data, code, err := h.documentService.UploadEmptyDocument(kb, tenantID, name)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}
	common.SuccessNoMessage(c, mapDocKeysWithRunStatus(data))
}

func (h *DocumentHandler) uploadWebDocument(c *gin.Context, kb *entity.Knowledgebase, tenantID string) {
	name := strings.TrimSpace(c.PostForm("name"))
	rawURL := c.PostForm("url")
	if name == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, `Lack of "name"`)
		return
	}
	if rawURL == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, `Lack of "url"`)
		return
	}
	if len([]byte(name)) > 255 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "File name must be 255 bytes or less.")
		return
	}
	if !isValidHTTPURL(rawURL) {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "The URL format is invalid")
		return
	}
	data, code, err := h.documentService.UploadWebDocument(kb, tenantID, name, rawURL)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}
	common.SuccessNoMessage(c, mapDocKeysWithRunStatus(data))
}

// mapDocKeysWithRunStatus renames a freshly-created document's raw keys to the
// public response shape (chunk_num→chunk_count, token_num→token_count,
// kb_id→dataset_id, parser_id→chunk_method) and reports run as a label.
// Mirrors Python map_doc_keys_with_run_status / map_doc_keys.
func mapDocKeysWithRunStatus(raw map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{
		"chunk_count":  raw["chunk_num"],
		"token_count":  raw["token_num"],
		"dataset_id":   raw["kb_id"],
		"chunk_method": raw["parser_id"],
		"run":          "UNSTART",
	}
	for _, k := range []string{"id", "name", "type", "size", "suffix", "source_type", "created_by", "parser_config", "location", "pipeline_id", "content_hash"} {
		if v, ok := raw[k]; ok {
			out[k] = v
		}
	}
	return out
}

// isValidHTTPURL mirrors Python is_valid_url: requires an http/https scheme and a host.
func isValidHTTPURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func (h *DocumentHandler) DownloadDocument(c *gin.Context) {
	datasetID := c.Param("dataset_id")
	docID := c.Param("document_id")

	if docID == "" {
		common.ErrorWithCode(c, int(common.CodeDataError), "Specify document_id please.")
		return
	}
	if datasetID == "" {
		common.ErrorWithCode(c, int(common.CodeDataError), fmt.Sprintf("The dataset not own the document %s.", docID))
		return
	}

	res, err := h.documentService.DownloadDocument(datasetID, docID)

	if err != nil {
		common.ErrorWithCode(c, int(common.CodeDataError), err.Error())
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

// MetadataSummary handles the metadata summary request
func (h *DocumentHandler) MetadataSummary(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var requestBody struct {
		KBID   string   `json:"kb_id" binding:"required"`
		DocIDs []string `json:"doc_ids"`
	}

	if err := c.ShouldBindJSON(&requestBody); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, "kb_id is required")
		return
	}

	kbID := requestBody.KBID
	if kbID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, "kb_id is required")
		return
	}

	summary, err := h.documentService.GetMetadataSummary(kbID, requestBody.DocIDs)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, 1, nil, "Failed to get metadata summary: "+err.Error())
		return
	}

	common.SuccessWithData(c, gin.H{"summary": summary}, "success")
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
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req SetMetaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, err.Error())
		return
	}

	if req.DocID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, "doc_id is required")
		return
	}

	// Parse meta JSON string
	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(req.Meta), &meta); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, "Json syntax error: "+err.Error())
		return
	}

	if meta == nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, "meta is required")
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
					if _, ok = item.(float64); !ok {
						common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, fmt.Sprintf("Unsupported type in list for key %s: %T", k, item))
						return
					}
				}
			}
		default:
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, fmt.Sprintf("Unsupported type for key %s: %T", k, v))
			return
		}
	}

	// Authorization: user must be able to access the document's dataset.
	doc, err := h.documentService.GetDocumentByID(req.DocID)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, "document not found")
		return
	}
	if !h.datasetService.Accessible(doc.KbID, user.ID) {
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "No authorization.")
		return
	}

	err = h.documentService.SetDocumentMetadata(req.DocID, meta)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "no such document") || strings.Contains(errMsg, "document not found") {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, errMsg)
		} else {
			common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, 1, nil, "Failed to set metadata: "+errMsg)
		}
		return
	}

	common.SuccessWithData(c, true, "success")
}

func (h *DocumentHandler) Ingest(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "No Authentication")
		return
	}

	var req service.IngestDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}

	if code, err := h.documentService.Ingest(userID, &req); err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithData(c, true, "success")
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req DeleteMetaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, err.Error())
		return
	}

	if req.DocID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, "doc_id is required")
		return
	}

	// Authorization: user must be able to access the document's dataset.
	doc, err := h.documentService.GetDocumentByID(req.DocID)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, "document not found")
		return
	}
	if !h.datasetService.Accessible(doc.KbID, user.ID) {
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "No authorization.")
		return
	}

	// If Keys is provided, parse and delete specific keys; otherwise delete entire document metadata
	if req.Keys != "" {
		// Parse keys JSON string - expected to be a list of key names to delete
		var keys []string
		if err = json.Unmarshal([]byte(req.Keys), &keys); err != nil {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, "Json syntax error: "+err.Error())
			return
		}

		if keys == nil || len(keys) == 0 {
			common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, "keys list is required")
			return
		}

		err = h.documentService.DeleteDocumentMetadata(req.DocID, keys)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "no such document") || strings.Contains(errMsg, "document not found") {
				common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, errMsg)
			} else {
				common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, 1, nil, "Failed to delete metadata: "+errMsg)
			}
			return
		}
	} else {
		// Delete entire document metadata
		err = h.documentService.DeleteDocumentAllMetadata(req.DocID)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "no such document") || strings.Contains(errMsg, "document not found") {
				common.ResponseWithHttpCodeData(c, http.StatusBadRequest, 1, nil, errMsg)
			} else {
				common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, 1, nil, "Failed to delete metadata: "+errMsg)
			}
			return
		}
	}

	common.SuccessWithData(c, true, "success")
}

type ListIngestionsRequest struct {
	DatasetID *string `json:"dataset_id"`
}

func (h *DocumentHandler) ListIngestionTasks(c *gin.Context) {
	var req ListIngestionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}

	userID := c.GetString("user_id")

	var parseResult []*entity.IngestionTask
	var err error
	if req.DatasetID != nil {
		if !h.datasetService.Accessible(*req.DatasetID, userID) {
			common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "No authorization to access the dataset.")
			return
		}
	}

	parseResult, err = h.documentService.ListIngestionTasks(userID, req.DatasetID, 0, 0)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}

	common.SuccessWithData(c, parseResult, "success")
}

type StartParseDocumentsRequest struct {
	DatasetID string   `json:"dataset_id" binding:"required"`
	Documents []string `json:"documents" binding:"required"`
}

func (h *DocumentHandler) StartIngestionTask(c *gin.Context) {
	var req StartParseDocumentsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}

	userID := c.GetString("user_id")

	if !h.datasetService.Accessible(req.DatasetID, userID) {
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "No authorization to access the dataset.")
		return
	}

	parseResult, err := h.documentService.IngestDocuments(req.DatasetID, userID, req.Documents)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}

	common.SuccessWithData(c, parseResult, "success")
}

type StopIngestionsRequest struct {
	Tasks []string `json:"tasks" binding:"required"`
}

func (h *DocumentHandler) StopIngestionTasks(c *gin.Context) {
	var req StopIngestionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}

	userID := c.GetString("user_id")

	parseResult, err := h.documentService.StopIngestionTasks(req.Tasks, userID)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	common.SuccessWithData(c, parseResult, "success")
}

type RemoveIngestionsRequest struct {
	Tasks []string `json:"tasks" binding:"required"`
}

func (h *DocumentHandler) RemoveIngestionTasks(c *gin.Context) {
	var req RemoveIngestionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}

	if req.Tasks == nil || len(req.Tasks) == 0 {
		common.ErrorWithCode(c, 1, "task_ids is required")
		return
	}

	userID := c.GetString("user_id")

	deletedTasks, err := h.documentService.RemoveIngestionTasks(req.Tasks, userID)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	common.SuccessWithData(c, deletedTasks, "success")
}

type ParseDocumentRequest struct {
	Documents []string `json:"documents" binding:"required"`
}

func (h *DocumentHandler) ParseDocuments(c *gin.Context) {
	datasetID := c.Param("dataset_id")

	var req ParseDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}

	userID := c.GetString("user_id")

	if !h.datasetService.Accessible(datasetID, userID) {
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "No authorization to access the dataset.")
		return
	}

	parseResult, err := h.documentService.ParseDocuments(datasetID, userID, req.Documents)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	common.SuccessWithData(c, parseResult, "success")
}

type StopParseDocumentRequest struct {
	DocumentIDs []string `json:"document_ids" binding:"required"`
}

func (h *DocumentHandler) StopParseDocuments(c *gin.Context) {
	datasetID := c.Param("dataset_id")

	var req StopParseDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, int(common.CodeBadRequest), err.Error())
		return
	}

	if len(req.DocumentIDs) == 0 {
		common.ErrorWithCode(c, int(common.CodeBadRequest), "`document_ids` is required")
		return
	}

	userID := c.GetString("user_id")

	if !h.datasetService.Accessible(datasetID, userID) {
		common.ResponseWithCodeData(c, common.CodeAuthenticationError, nil, "You don't own the dataset.")
		return
	}

	result, err := h.documentService.StopParseDocuments(datasetID, req.DocumentIDs)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	common.SuccessWithData(c, result, "success")
}

func (h *DocumentHandler) MetadataSummaryByDataset(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	datasetID := c.Param("dataset_id")
	if datasetID == "" {
		common.ErrorWithCode(c, int(common.CodeServerError), "dataset_id is required")
		return
	}
	if !h.datasetService.Accessible(datasetID, user.ID) {
		common.ErrorWithCode(c, int(common.CodeServerError), "You don't own the dataset "+datasetID)
		return
	}

	var docIDS []string
	if docIDsParam := c.Query("doc_ids"); docIDsParam != "" {
		docIDS = strings.Split(docIDsParam, ",")
	}

	summary, err := h.documentService.GetMetadataSummary(datasetID, docIDS)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, common.CodeServerError, nil, "Failed to get metadata summary"+err.Error())
		return
	}

	common.SuccessWithData(c, gin.H{"summary": summary}, "success")
}

func (h *DocumentHandler) UpdateDatasetDocument(c *gin.Context) {
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

	body, err := c.GetRawData()
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}
	present := make(map[string]bool, len(raw))
	for key := range raw {
		present[key] = true
	}
	var req service.UpdateDatasetDocumentRequest
	if err = json.Unmarshal(body, &req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	data, code, err := h.documentService.UpdateDatasetDocument(user.ID, datasetID, documentID, &req, present)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessNoMessage(c, data)
}

func (h *DocumentHandler) UploadInfo(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	form, err := c.MultipartForm()
	if err != nil && !strings.Contains(err.Error(), "request Content-Type isn't multipart/form-data") {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Failed to parse multipart form: "+err.Error())
		return
	}

	var fileHeaders []*multipart.FileHeader
	if form != nil && form.File != nil {
		fileHeaders = form.File["file"]
	}
	rawURL := strings.TrimSpace(c.Query("url"))

	if len(fileHeaders) > 0 && rawURL != "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Provide either multipart file(s) or ?url=..., not both.")
		return
	}
	if len(fileHeaders) == 0 && rawURL == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Missing input: provide multipart file(s) or url")
		return
	}

	if rawURL != "" {
		data, code, err := h.documentService.UploadDocumentInfoByURL(user.ID, rawURL)
		if err != nil {
			common.ErrorWithCode(c, int(code), err.Error())
			return
		}
		common.SuccessWithData(c, data, "success")
		return
	}

	data, code, err := h.documentService.UploadDocumentInfos(user.ID, fileHeaders)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	var payload interface{}
	if len(data) == 1 {
		payload = data[0]
	} else {
		payload = data
	}
	common.SuccessWithData(c, payload, "success")
}

type documentMetadataBatchRequest struct {
	Selector *service.DocumentMetadataSelector `json:"selector"`
	Updates  []service.DocumentMetadataUpdate  `json:"updates"`
	Deletes  []service.DocumentMetadataDelete  `json:"deletes"`
}

func (h *DocumentHandler) MetadataBatchUpdate(c *gin.Context) {
	h.handleBatchUpdateDocumentMetadatas(c)
}

func (h *DocumentHandler) UpdateDocumentMetadatas(c *gin.Context) {
	h.handleBatchUpdateDocumentMetadatas(c)
}

func (h *DocumentHandler) handleBatchUpdateDocumentMetadatas(c *gin.Context) {
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
	if !h.datasetService.Accessible(datasetID, user.ID) {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "You don't own the dataset "+datasetID+".")
		return
	}

	var req documentMetadataBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}
	if req.Selector == nil {
		req.Selector = &service.DocumentMetadataSelector{}
	}
	if req.Updates == nil {
		req.Updates = []service.DocumentMetadataUpdate{}
	}
	if req.Deletes == nil {
		req.Deletes = []service.DocumentMetadataDelete{}
	}

	resp, code, err := h.documentService.BatchUpdateDocumentMetadatas(datasetID, req.Selector, req.Updates, req.Deletes)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}
	common.SuccessWithData(c, resp, "success")
}
