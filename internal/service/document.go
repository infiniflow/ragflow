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

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"regexp"
	"sort"
	"strings"
	"time"

	"ragflow/internal/cache"
	"ragflow/internal/dao"
	"ragflow/internal/engine"

	"ragflow/internal/server"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DocumentService document service
type DocumentService struct {
	documentDAO      *dao.DocumentDAO
	kbDAO            *dao.KnowledgebaseDAO
	ingestionTaskDAO *dao.IngestionDAO
	ingestionLogDAO  *dao.IngestionLogDAO
	docEngine        engine.DocEngine
	engineType       server.EngineType
	metadataSvc      *MetadataService
	taskDAO          *dao.TaskDAO
	file2DocumentDAO *dao.File2DocumentDAO
}

// NewDocumentService create document service
func NewDocumentService() *DocumentService {
	cfg := server.GetConfig()
	return &DocumentService{
		documentDAO:      dao.NewDocumentDAO(),
		ingestionTaskDAO: dao.NewIngestionDAO(),
		ingestionLogDAO:  dao.NewIngestionLogDAO(),
		kbDAO:            dao.NewKnowledgebaseDAO(),
		docEngine:        engine.Get(),
		engineType:       cfg.DocEngine.Type,
		metadataSvc:      NewMetadataService(),
		taskDAO:          dao.NewTaskDAO(),
		file2DocumentDAO: dao.NewFile2DocumentDAO(),
	}
}

// CreateDocumentRequest create document request
type CreateDocumentRequest struct {
	Name      string `json:"name" binding:"required"`
	KbID      string `json:"kb_id" binding:"required"`
	ParserID  string `json:"parser_id" binding:"required"`
	CreatedBy string `json:"created_by" binding:"required"`
	Type      string `json:"type"`
	Source    string `json:"source"`
}

// UpdateDocumentRequest update document request
type UpdateDocumentRequest struct {
	Name        *string  `json:"name"`
	Run         *string  `json:"run"`
	TokenNum    *int64   `json:"token_num"`
	ChunkNum    *int64   `json:"chunk_num"`
	Progress    *float64 `json:"progress"`
	ProgressMsg *string  `json:"progress_msg"`
}

// DocumentResponse document response
type DocumentResponse struct {
	ID              string  `json:"id"`
	Name            *string `json:"name,omitempty"`
	KbID            string  `json:"kb_id"`
	ParserID        string  `json:"parser_id"`
	PipelineID      *string `json:"pipeline_id,omitempty"`
	Type            string  `json:"type"`
	SourceType      string  `json:"source_type"`
	CreatedBy       string  `json:"created_by"`
	Location        *string `json:"location,omitempty"`
	Size            int64   `json:"size"`
	TokenNum        int64   `json:"token_num"`
	ChunkNum        int64   `json:"chunk_num"`
	Progress        float64 `json:"progress"`
	ProgressMsg     *string `json:"progress_msg,omitempty"`
	ProcessDuration float64 `json:"process_duration"`
	Suffix          string  `json:"suffix"`
	Run             *string `json:"run,omitempty"`
	Status          *string `json:"status,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type ThumbnailResponse struct {
	ID        string  `json:"id"`
	Thumbnail *string `json:"thumbnail,omitempty"`
	KbID      string  `json:"kb_id"`
}

type ArtifactResponse struct {
	Data            []byte
	ContentType     string
	SafeFilename    string
	ForceAttachment bool
}

var (
	ErrArtifactInvalidFilename = errors.New("Invalid filename.")
	ErrArtifactInvalidFileType = errors.New("Invalid file type.")
	ErrArtifactNotFound        = errors.New("Artifact not found.")
)

var artifactContentTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".svg":  "image/svg+xml",
	".pdf":  "application/pdf",
	".csv":  "text/csv",
	".json": "application/json",
	".html": "text/html",
}

var artifactForceAttachmentExtensions = map[string]struct{}{
	".htm":   {},
	".html":  {},
	".shtml": {},
	".xht":   {},
	".xhtml": {},
	".xml":   {},
	".mhtml": {},
	".svg":   {},
}
var artifactForceAttachmentContentTypes = map[string]struct{}{
	"text/html":             {},
	"image/svg+xml":         {},
	"application/xhtml+xml": {},
	"text/xml":              {},
	"application/xml":       {},
	"multipart/related":     {},
}

var artifactUnsafeFilenameChars = regexp.MustCompile(`[^\pL\pN_.-]`)

// GetDocumentImage retrieves an image object from storage.
func (s *DocumentService) GetDocumentImage(imageID string) ([]byte, error) {
	parts := strings.Split(imageID, "-")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("Image not found.")
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	return storageImpl.Get(parts[0], parts[1])
}

// GetDocumentArtifact retrieves a sandbox artifact from object storage.
func (s *DocumentService) GetDocumentArtifact(filename string) (*ArtifactResponse, error) {
	basename := filepath.Base(filename)
	if basename != filename || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return nil, ErrArtifactInvalidFilename
	}

	ext := strings.ToLower(filepath.Ext(basename))
	contentType, ok := artifactContentTypes[ext]
	if !ok {
		return nil, ErrArtifactInvalidFileType
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	bucket := sandboxArtifactBucket()
	if !storageImpl.ObjExist(bucket, basename) {
		return nil, ErrArtifactNotFound
	}

	data, err := storageImpl.Get(bucket, basename)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, ErrArtifactNotFound
	}

	return &ArtifactResponse{
		Data:            data,
		ContentType:     contentType,
		SafeFilename:    sanitizeArtifactFilename(basename),
		ForceAttachment: shouldForceArtifactAttachment(ext, contentType),
	}, nil
}

func sandboxArtifactBucket() string {
	if bucket := os.Getenv("SANDBOX_ARTIFACT_BUCKET"); bucket != "" {
		return bucket
	}
	return "sandbox-artifacts"
}

func sanitizeArtifactFilename(filename string) string {
	return artifactUnsafeFilenameChars.ReplaceAllString(filename, "_")
}

func shouldForceArtifactAttachment(ext, contentType string) bool {
	if _, ok := artifactForceAttachmentExtensions[strings.ToLower(ext)]; ok {
		return true
	}
	_, ok := artifactForceAttachmentContentTypes[strings.ToLower(contentType)]
	return ok
}

type DocumentPreview struct {
	Data        []byte
	ContentType string
	FileName    string
}

func (s *DocumentService) GetDocumentPreview(docID string) (*DocumentPreview, error) {
	doc, err := s.documentDAO.GetByID(docID)
	if err != nil {
		return nil, err
	}

	bucket, name, err := s.GetDocumentStorageAddress(doc)
	if err != nil {
		return nil, err
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	data, err := storageImpl.Get(bucket, name)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, ErrArtifactNotFound
	}

	fileName := ""
	if doc.Name != nil {
		fileName = *doc.Name
	}

	ext := utility.GetFileExtension(fileName)
	contentType := utility.GetContentType(ext, doc.Type)

	return &DocumentPreview{
		Data:        data,
		ContentType: contentType,
		FileName:    fileName,
	}, nil
}

func (s *DocumentService) GetDocumentStorageAddress(doc *entity.Document) (string, string, error) {
	if doc == nil {
		return "", "", fmt.Errorf("document is nil")
	}

	file2DocumentDAO := dao.NewFile2DocumentDAO()
	fileDAO := dao.NewFileDAO()

	mappings, err := file2DocumentDAO.GetByDocumentID(doc.ID)
	if err != nil {
		return "", "", err
	}

	if len(mappings) > 0 && mappings[0].FileID != nil {
		file, err := fileDAO.GetByID(*mappings[0].FileID)
		if err != nil {
			return "", "", err
		}

		if file.SourceType == "" || entity.FileSource(file.SourceType) == entity.FileSourceLocal {
			if file.Location == nil || *file.Location == "" {
				return "", "", fmt.Errorf("file location is empty")
			}
			return file.ParentID, *file.Location, nil
		}
	}

	if doc.Location == nil || *doc.Location == "" {
		return "", "", fmt.Errorf("document location is empty")
	}
	return doc.KbID, *doc.Location, nil
}

type DownloadDocumentResp struct {
	Data        []byte
	FileName    string
	ContentType string
}

func (s *DocumentService) DownloadDocument(datasetID, docID string) (*DownloadDocumentResp, error) {
	if docID == "" {
		return nil, fmt.Errorf("Specify document_id please.")
	}
	doc, err := s.documentDAO.GetByID(docID)
	if err != nil || doc.KbID != datasetID {
		return nil, fmt.Errorf("The dataset not own the document %s.", docID)
	}
	bucket, name, err := s.GetDocumentStorageAddress(doc)
	if err != nil {
		return nil, err
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	data, err := storageImpl.Get(bucket, name)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("This file is empty.")
	}

	fileName := ""
	if doc.Name != nil {
		fileName = *doc.Name
	}

	return &DownloadDocumentResp{
		Data:        data,
		FileName:    fileName,
		ContentType: "application/octet-stream",
	}, nil
}

// CreateDocument create document
func (s *DocumentService) CreateDocument(req *CreateDocumentRequest) (*entity.Document, error) {
	document := &entity.Document{
		Name:       &req.Name,
		KbID:       req.KbID,
		ParserID:   req.ParserID,
		CreatedBy:  req.CreatedBy,
		Type:       req.Type,
		SourceType: req.Source,
		Suffix:     ".doc",
		Status:     func() *string { s := "0"; return &s }(),
	}

	if err := s.documentDAO.Create(document); err != nil {
		return nil, fmt.Errorf("failed to create document: %w", err)
	}

	return document, nil
}

// GetDocumentByID get document by ID
func (s *DocumentService) GetDocumentByID(id string) (*DocumentResponse, error) {
	document, err := s.documentDAO.GetByID(id)
	if err != nil {
		return nil, err
	}

	return s.toResponse(document), nil
}

// UpdateDocument update document
func (s *DocumentService) UpdateDocument(id string, req *UpdateDocumentRequest) error {
	document, err := s.documentDAO.GetByID(id)
	if err != nil {
		return err
	}

	if req.Name != nil {
		document.Name = req.Name
	}
	if req.Run != nil {
		document.Run = req.Run
	}
	if req.TokenNum != nil {
		document.TokenNum = *req.TokenNum
	}
	if req.ChunkNum != nil {
		document.ChunkNum = *req.ChunkNum
	}
	if req.Progress != nil {
		document.Progress = *req.Progress
	}
	if req.ProgressMsg != nil {
		document.ProgressMsg = req.ProgressMsg
	}

	return s.documentDAO.Update(document)
}

// DeleteDocument delete document — delegates to full cleanup logic.
func (s *DocumentService) DeleteDocument(id string) error {
	return s.deleteDocumentFull(id)
}

// DeleteDocuments deletes multiple documents under a dataset.
//
//	ids: specific document IDs; deleteAll: delete all docs in the dataset.
//	Returns the number of successfully deleted documents.
func (s *DocumentService) DeleteDocuments(ids []string, deleteAll bool, datasetID, userID string) (int, error) {
	// 1. Check dataset is accessible by the user
	if !s.kbDAO.Accessible(datasetID, userID) {
		return 0, fmt.Errorf("You don't own the dataset %s.", datasetID)
	}

	// 2. Resolve document IDs
	if deleteAll {
		if err := dao.DB.Model(&entity.Document{}).
			Where("kb_id = ?", datasetID).
			Pluck("id", &ids).Error; err != nil {
			return 0, fmt.Errorf("failed to query documents: %w", err)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}

	// 3. Deduplicate (before validation so dup count doesn't matter)
	ids = common.Deduplicate(ids)

	// 4. Validate IDs belong to this dataset (only for explicit ids; deleteAll is already scoped)
	if !deleteAll {
		if _, err := s.validateDocsInDataset(ids, datasetID); err != nil {
			return 0, err
		}
	}

	// 5. Delete each document (non-critical failures are tolerated per doc)
	deleted := 0
	for _, docID := range ids {
		if err := s.deleteDocumentFull(docID); err != nil {
			common.Logger.Warn(fmt.Sprintf("DeleteDocuments: failed to delete %s: %v", docID, err))
			continue
		}
		deleted++
	}

	return deleted, nil
}

// deleteDocumentFull performs full document cleanup. Non-critical failures
// are tolerated (logged and continue). Critical failures (e.g. document or
// KB not found) return an error immediately.
func (s *DocumentService) deleteDocumentFull(docID string) error {
	doc, kb, err := s.resolveDocAndKB(docID)
	if err != nil {
		return err
	}

	// Delete tasks from DB
	if _, delErr := s.taskDAO.DeleteByDocIDs([]string{docID}); delErr != nil {
		common.Logger.Warn(fmt.Sprintf("failed to delete tasks for %s: %v", docID, delErr))
	}
	s.deleteDocEngineData(docID, kb.TenantID, doc.KbID)
	if err := s.deleteDocRecordWithCounters(doc, kb.ID); err != nil {
		return err
	}
	s.cleanupFileReferences(docID)

	return nil
}

// RemoveDocumentKeepFile removes a document's chunks/metadata and the document
// row, decrementing the KB counters (doc_num/chunk_num/token_num), WITHOUT
// deleting the underlying file record, its storage blob, or its file2document
// mappings. Mirrors Python DocumentService.remove_document — the caller is
// responsible for cleaning up the file2document mappings separately.
func (s *DocumentService) RemoveDocumentKeepFile(docID string) error {
	doc, kb, err := s.resolveDocAndKB(docID)
	if err != nil {
		return err
	}
	if _, delErr := s.taskDAO.DeleteByDocIDs([]string{docID}); delErr != nil {
		common.Logger.Warn(fmt.Sprintf("RemoveDocumentKeepFile: failed to delete tasks for %s: %v", docID, delErr))
	}
	s.deleteDocEngineData(docID, kb.TenantID, doc.KbID)
	return s.deleteDocRecordWithCounters(doc, kb.ID)
}

// InsertDocument creates a document row and increments the owning KB's doc_num
// counter in a single transaction. Mirrors Python DocumentService.insert, which
// updates dataset/document counters on insert. The document's ID and timestamps
// are populated by the caller / model hooks before insertion.
func (s *DocumentService) InsertDocument(doc *entity.Document) error {
	return dao.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(doc).Error; err != nil {
			return fmt.Errorf("failed to create document: %w", err)
		}
		// Guard the counter bump with RowsAffected: documents.kb_id has no DB-level
		// FK, so Create can succeed against a non-existent KB and the Update would
		// then report a nil error with 0 rows touched, silently desyncing doc_num.
		// Roll the whole transaction back in that case (mirrors the counter checks
		// in deleteDocRecordWithCounters).
		result := tx.Model(&entity.Knowledgebase{}).
			Where("id = ?", doc.KbID).
			Update("doc_num", gorm.Expr("doc_num + 1"))
		if result.Error != nil {
			return fmt.Errorf("failed to increment doc_num for KB %s: %w", doc.KbID, result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("knowledgebase %s not found", doc.KbID)
		}
		return nil
	})
}

// resolveDocAndKB loads the document and its knowledgebase, returning both or
// an error.
func (s *DocumentService) resolveDocAndKB(docID string) (*entity.Document, *entity.Knowledgebase, error) {
	doc, err := s.documentDAO.GetByID(docID)
	if err != nil {
		return nil, nil, fmt.Errorf("document not found: %w", err)
	}
	kb, err := s.kbDAO.GetByID(doc.KbID)
	if err != nil {
		return nil, nil, fmt.Errorf("knowledgebase not found: %w", err)
	}
	return doc, kb, nil
}

// deleteDocEngineData removes chunks and metadata from the document engine.
// No-op when the engine is nil.
func (s *DocumentService) deleteDocEngineData(docID, tenantID, kbID string) {
	if s.docEngine == nil {
		return
	}
	ctx := context.Background()
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	if _, delErr := s.docEngine.DeleteChunks(ctx, map[string]interface{}{"doc_id": docID}, indexName, kbID); delErr != nil {
		common.Logger.Warn(fmt.Sprintf("deleteDocEngineData: failed to delete chunks for %s: %v", docID, delErr))
	}
	if s.metadataSvc != nil {
		_ = s.DeleteDocumentAllMetadata(docID) // logs internally
	}
}

// deleteDocRecordWithCounters hard-deletes the document row and decrements the
// KB counters in a single transaction. Counters are only decremented when a
// document row was actually removed (RowsAffected > 0), guarding against
// double-decrement on retries or concurrent deletes.
func (s *DocumentService) deleteDocRecordWithCounters(doc *entity.Document, kbID string) error {
	return dao.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ?", doc.ID).Delete(&entity.Document{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete document %s: %w", doc.ID, result.Error)
		}
		if result.RowsAffected == 0 {
			return nil // already deleted by a concurrent request — skip counters
		}

		decErr := tx.Model(&entity.Knowledgebase{}).
			Where("id = ?", kbID).
			Updates(map[string]interface{}{
				"doc_num":   gorm.Expr("doc_num - 1"),
				"chunk_num": gorm.Expr("chunk_num - ?", doc.ChunkNum),
				"token_num": gorm.Expr("token_num - ?", doc.TokenNum),
			}).Error
		if decErr != nil {
			common.Logger.Warn(fmt.Sprintf("deleteDocRecordWithCounters: failed to decrement KB %s: %v", kbID, decErr))
		}
		return nil
	})
}

// cleanupFileReferences deletes file2document mappings for docID, and for each
// referenced file, only hard-deletes the file record and its storage blob when
// no other document still references the same file_id.
func (s *DocumentService) cleanupFileReferences(docID string) {
	mappings, mapErr := s.file2DocumentDAO.GetByDocumentID(docID)
	if mapErr != nil {
		common.Logger.Warn(fmt.Sprintf("cleanupFileReferences: failed to get f2d mappings for %s: %v", docID, mapErr))
	}
	if len(mappings) == 0 {
		return
	}

	// Collect unique file_ids
	seen := make(map[string]bool)
	var fileIDs []string
	for _, m := range mappings {
		if m.FileID == nil || seen[*m.FileID] {
			continue
		}
		seen[*m.FileID] = true
		fileIDs = append(fileIDs, *m.FileID)
	}

	// Delete all file2document rows for this document
	if delErr := s.file2DocumentDAO.DeleteByDocumentID(docID); delErr != nil {
		common.Logger.Warn(fmt.Sprintf("cleanupFileReferences: failed to delete f2d for %s: %v", docID, delErr))
	}

	// For each file, only delete the record and blob when no other doc references it
	for _, fileID := range fileIDs {
		remaining, remErr := s.file2DocumentDAO.GetByFileID(fileID)
		if remErr != nil {
			common.Logger.Warn(fmt.Sprintf("cleanupFileReferences: failed to check remaining f2d for %s: %v", fileID, remErr))
			continue
		}
		if len(remaining) > 0 {
			continue
		}

		fileDAO := dao.NewFileDAO()
		file, fErr := fileDAO.GetByID(fileID)
		if fErr != nil || file == nil {
			common.Logger.Warn(fmt.Sprintf("cleanupFileReferences: file not found %s: %v", fileID, fErr))
			continue
		}
		if _, delErr := fileDAO.DeleteByIDs([]string{fileID}); delErr != nil {
			common.Logger.Warn(fmt.Sprintf("cleanupFileReferences: failed to delete file %s: %v", fileID, delErr))
			continue // keep the blob so the live file row still has its object
		}
		if file.Location != nil && *file.Location != "" {
			storageImpl := storage.GetStorageFactory().GetStorage()
			if storageImpl != nil {
				if rmErr := storageImpl.Remove(file.ParentID, *file.Location); rmErr != nil {
					common.Logger.Warn(fmt.Sprintf("cleanupFileReferences: failed to remove blob %s/%s: %v", file.ParentID, *file.Location, rmErr))
				}
			}
		}
	}
}

// ListDocuments list documents
func (s *DocumentService) ListDocuments(page, pageSize int) ([]*DocumentResponse, int64, error) {
	offset := (page - 1) * pageSize
	documents, total, err := s.documentDAO.List(offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*DocumentResponse, len(documents))
	for i, doc := range documents {
		responses[i] = s.toResponse(doc)
	}

	return responses, total, nil
}

func (s *DocumentService) GetThumbnail(docID string) (*ThumbnailResponse, error) {
	document, err := s.documentDAO.GetByID(docID)
	if err != nil {
		return nil, err
	}

	var result ThumbnailResponse
	result.ID = document.ID
	result.Thumbnail = document.Thumbnail
	result.KbID = document.KbID
	return &result, nil
}

// ListDocumentsByDatasetID list documents by knowledge base ID
func (s *DocumentService) ListDocumentsByDatasetID(kbID string, page, pageSize int) ([]*entity.DocumentListItem, int64, error) {
	offset := (page - 1) * pageSize
	documents, total, err := s.documentDAO.ListByKBID(kbID, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*entity.DocumentListItem, len(documents))
	for i, doc := range documents {
		responses[i] = doc
	}

	return responses, total, nil
}

// GetDocumentsByAuthorID get documents by author ID
func (s *DocumentService) GetDocumentsByAuthorID(authorID, page, pageSize int) ([]*DocumentResponse, int64, error) {
	offset := (page - 1) * pageSize
	documents, total, err := s.documentDAO.GetByAuthorID(fmt.Sprintf("%d", authorID), offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*DocumentResponse, len(documents))
	for i, doc := range documents {
		responses[i] = s.toResponse(doc)
	}

	return responses, total, nil
}

type ParseDocumentResponse struct {
	DocumentID string  `json:"document_id"`
	Result     *string `json:"result"`
}

func (s *DocumentService) ParseDocuments(datasetID, userID string, docIDs []string) ([]*ParseDocumentResponse, error) {
	// create document parse id
	// save to task table
	// send to message queue

	// deduplicate the document id
	uniqueDocIDs := common.Deduplicate(docIDs)
	if uniqueDocIDs == nil || len(uniqueDocIDs) == 0 {
		return nil, fmt.Errorf("no documents to parse")
	}

	var responses []*ParseDocumentResponse

	// query database, if the document ids are valid
	for _, docID := range uniqueDocIDs {
		doc, err := s.documentDAO.GetByID(docID)
		if err != nil {
			errorMessage := err.Error()
			responses = append(responses, &ParseDocumentResponse{
				DocumentID: docID,
				Result:     &errorMessage,
			})
			continue
		}
		if doc == nil {
			errorMessage := "no such document"
			responses = append(responses, &ParseDocumentResponse{
				DocumentID: docID,
				Result:     &errorMessage,
			})
			continue
		}

		if doc.Status != nil && *doc.Status != "0" {
			errorMessage := fmt.Sprintf("document %s is already parsed", docID)
			responses = append(responses, &ParseDocumentResponse{
				DocumentID: docID,
				Result:     &errorMessage,
			})
			continue
		}

		// create task for each document
		task := &entity.IngestionTask{
			ID:         uuid.New().String(),
			DocumentID: docID,
			UserID:     userID,
			Config:     nil,
			TryCount:   1,
		}

		// save the task to database
		err = s.ingestionTaskDAO.Create(task)
		if err != nil {
			errorMessage := err.Error()
			responses = append(responses, &ParseDocumentResponse{
				DocumentID: docID,
				Result:     &errorMessage,
			})
			continue
		}

		// Send task to message queue

	}

	common.Info(fmt.Sprintf("parse documents, dataset: %s, documents: %v", datasetID, docIDs))
	return responses, nil
}

// StopParseDocuments stops parsing for the given documents in a dataset.
// It sets Redis cancel signals for associated tasks and updates doc.run to CANCEL.
// Returns a map with success_count and optionally errors.
func (s *DocumentService) StopParseDocuments(datasetID string, docIDs []string) (map[string]interface{}, error) {
	deduped := common.Deduplicate(docIDs)
	if len(deduped) == 0 {
		return nil, fmt.Errorf("no document IDs provided")
	}

	docs, err := s.validateDocsInDataset(deduped, datasetID)
	if err != nil {
		return nil, err
	}

	var errors []string
	successCount := 0
	for _, doc := range docs {
		if cancelErr := s.cancelDocParse(doc); cancelErr != nil {
			errors = append(errors, cancelErr.Error())
			continue
		}
		successCount++
	}

	result := map[string]interface{}{"success_count": successCount}
	if len(errors) > 0 {
		result["errors"] = errors
	}
	return result, nil
}

// validateDocsInDataset deduplicates IDs, fetches the documents, and ensures
// every document exists and belongs to the given dataset. Returns the resolved
// documents.
func (s *DocumentService) validateDocsInDataset(docIDs []string, datasetID string) ([]*entity.Document, error) {
	docs, err := s.documentDAO.GetByIDs(docIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch documents: %w", err)
	}
	if len(docs) != len(docIDs) {
		return nil, fmt.Errorf("some document IDs not found in dataset %s", datasetID)
	}
	var invalid []string
	for _, d := range docs {
		if d.KbID != datasetID {
			invalid = append(invalid, d.ID)
		}
	}
	if len(invalid) > 0 {
		return nil, fmt.Errorf("these documents do not belong to dataset %s: %v", datasetID, invalid)
	}
	return docs, nil
}

// cancelDocParse sets Redis cancel signals for the document's active tasks and
// marks the document run status as CANCEL. Returns an error if the document is
// not in a cancellable state or the status update fails.
func (s *DocumentService) cancelDocParse(doc *entity.Document) error {
	tasks, taskErr := s.taskDAO.GetByDocID(doc.ID)
	if taskErr != nil {
		return fmt.Errorf("failed to get tasks for %s: %v", doc.ID, taskErr)
	}

	hasUnfinishedTask := false
	for _, t := range tasks {
		if t.Progress < 1 {
			hasUnfinishedTask = true
			break
		}
	}

	canCancel := false
	if doc.Run != nil {
		if *doc.Run == string(entity.TaskStatusRunning) || *doc.Run == string(entity.TaskStatusCancel) {
			canCancel = true
		}
	}
	if hasUnfinishedTask {
		canCancel = true
	}
	if !canCancel {
		return fmt.Errorf("can't stop parsing document that has not started or already completed")
	}

	// Set Redis cancel signal for each task (best-effort)
	redisClient := cache.Get()
	for _, t := range tasks {
		if redisClient != nil {
			redisClient.Set(fmt.Sprintf("%s-cancel", t.ID), "x", 0)
		}
	}

	if upErr := s.documentDAO.UpdateByID(doc.ID, map[string]interface{}{"run": string(entity.TaskStatusCancel)}); upErr != nil {
		return fmt.Errorf("failed to update document %s: %v", doc.ID, upErr)
	}
	return nil
}

// toResponse convert model.Document to DocumentResponse
func (s *DocumentService) toResponse(doc *entity.Document) *DocumentResponse {
	createdAt := ""
	if doc.CreateTime != nil {
		// Check if timestamp is in milliseconds (13 digits) or seconds (10 digits)
		var ts int64
		if *doc.CreateTime > 1000000000000 {
			// Milliseconds - convert to seconds
			ts = *doc.CreateTime / 1000
		} else {
			ts = *doc.CreateTime
		}
		createdAt = time.Unix(ts, 0).Format("2006-01-02 15:04:05")
	}
	updatedAt := ""
	if doc.UpdateTime != nil {
		// Accept both historical second-based values and current millisecond-based values.
		ts := *doc.UpdateTime
		if ts > 1000000000000 {
			ts /= 1000
		}
		updatedAt = time.Unix(ts, 0).Format("2006-01-02 15:04:05")
	}
	return &DocumentResponse{
		ID:              doc.ID,
		Name:            doc.Name,
		KbID:            doc.KbID,
		ParserID:        doc.ParserID,
		PipelineID:      doc.PipelineID,
		Type:            doc.Type,
		SourceType:      doc.SourceType,
		CreatedBy:       doc.CreatedBy,
		Location:        doc.Location,
		Size:            doc.Size,
		TokenNum:        doc.TokenNum,
		ChunkNum:        doc.ChunkNum,
		Progress:        doc.Progress,
		ProgressMsg:     doc.ProgressMsg,
		ProcessDuration: doc.ProcessDuration,
		Suffix:          doc.Suffix,
		Run:             doc.Run,
		Status:          doc.Status,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}
}

// GetMetadataSummaryRequest request for metadata summary
type GetMetadataSummaryRequest struct {
	KBID   string   `json:"kb_id" binding:"required"`
	DocIDs []string `json:"doc_ids"`
}

// GetMetadataSummaryResponse response for metadata summary
type GetMetadataSummaryResponse struct {
	Summary map[string]interface{} `json:"summary"`
}

// GetMetadataSummary get metadata summary for documents
func (s *DocumentService) GetMetadataSummary(kbID string, docIDs []string) (map[string]interface{}, error) {
	tenantID, err := s.metadataSvc.GetTenantIDByKBID(kbID)
	if err != nil {
		return nil, err
	}

	searchResult, err := s.metadataSvc.SearchMetadata(kbID, tenantID, docIDs, 1000)
	if err != nil {
		return nil, err
	}

	// Aggregate metadata from results
	return aggregateMetadata(searchResult.MetadataRecords), nil
}

// SetDocumentMetadata sets metadata for a document in the document engine
func (s *DocumentService) SetDocumentMetadata(docID string, meta map[string]interface{}) error {
	// Get document to find kb_id
	doc, err := s.documentDAO.GetByID(docID)
	if err != nil {
		return fmt.Errorf("document not found: %w", err)
	}

	// Get tenant ID
	tenantID, err := s.metadataSvc.GetTenantIDByKBID(doc.KbID)
	if err != nil {
		return fmt.Errorf("failed to get tenant ID: %w", err)
	}

	// Update metadata using the document engine (merges with existing)
	err = s.docEngine.UpdateMetadata(nil, docID, doc.KbID, meta, tenantID)
	if err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	return nil
}

// DeleteDocumentMetadata deletes metadata keys for a document in the document engine
func (s *DocumentService) DeleteDocumentMetadata(docID string, keys []string) error {
	// Get document to find kb_id
	doc, err := s.documentDAO.GetByID(docID)
	if err != nil {
		return fmt.Errorf("document not found: %w", err)
	}

	// Get tenant ID
	tenantID, err := s.metadataSvc.GetTenantIDByKBID(doc.KbID)
	if err != nil {
		return fmt.Errorf("failed to get tenant ID: %w", err)
	}

	// Delete metadata using the document engine
	err = s.docEngine.DeleteMetadataKeys(nil, docID, doc.KbID, keys, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete metadata: %w", err)
	}

	return nil
}

// DeleteDocumentAllMetadata deletes all metadata for a document in the document engine
func (s *DocumentService) DeleteDocumentAllMetadata(docID string) error {
	// Get document to find kb_id
	doc, err := s.documentDAO.GetByID(docID)
	if err != nil {
		return fmt.Errorf("document not found: %w", err)
	}

	// Get tenant ID
	tenantID, err := s.metadataSvc.GetTenantIDByKBID(doc.KbID)
	if err != nil {
		return fmt.Errorf("failed to get tenant ID: %w", err)
	}

	// Build condition to match the document
	condition := map[string]interface{}{
		"id":    docID,
		"kb_id": doc.KbID,
	}

	// Delete entire document metadata
	_, err = s.docEngine.DeleteMetadata(nil, condition, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete document metadata: %w", err)
	}

	return nil
}

// GetDocumentMetadataByID get metadata for a specific document
func (s *DocumentService) GetDocumentMetadataByID(docID string) (map[string]interface{}, error) {
	// Get document to find kb_id
	doc, err := s.documentDAO.GetByID(docID)
	if err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}

	tenantID, err := s.metadataSvc.GetTenantIDByKBID(doc.KbID)
	if err != nil {
		return nil, err
	}

	searchResult, err := s.metadataSvc.SearchMetadata(doc.KbID, tenantID, []string{docID}, 1)
	if err != nil {
		return nil, err
	}

	// Return metadata if found
	if len(searchResult.MetadataRecords) > 0 {
		metadata := searchResult.MetadataRecords[0]
		return ExtractMetaFields(metadata)
	}

	return make(map[string]interface{}), nil
}

// GetMetadataByKBs get metadata for knowledge bases
func (s *DocumentService) GetMetadataByKBs(kbIDs []string) (map[string]interface{}, error) {
	if len(kbIDs) == 0 {
		return make(map[string]interface{}), nil
	}

	searchResult, err := s.metadataSvc.SearchMetadataByKBs(kbIDs, 10000)
	if err != nil {
		return nil, err
	}

	flattenedMeta := make(map[string]map[string][]string)
	numMetadata := len(searchResult.MetadataRecords)

	var allMetaFields []map[string]interface{}
	if numMetadata > 1 && len(searchResult.MetadataRecords) > 0 {
		firstMetadata := searchResult.MetadataRecords[0]
		if metaFieldsVal := firstMetadata["meta_fields"]; metaFieldsVal != nil {
			if v, ok := metaFieldsVal.([]byte); ok {
				allMetaFields = ParseAllLengthPrefixedJSON(v)
			}
		}
	}

	for idx, metadata := range searchResult.MetadataRecords {
		docID, ok := ExtractDocumentID(metadata)
		if !ok {
			continue
		}

		var metaFields map[string]interface{}
		var metaFieldsVal interface{}

		if len(allMetaFields) > 0 && idx < len(allMetaFields) {
			// Use pre-parsed meta_fields from concatenated data
			metaFields = allMetaFields[idx]
		} else {
			// Normal case - get from chunk
			metaFieldsVal = metadata["meta_fields"]
			if metaFieldsVal != nil {
				switch v := metaFieldsVal.(type) {
				case string:
					if err := json.Unmarshal([]byte(v), &metaFields); err != nil {
						continue
					}
				case []byte:
					// Try direct JSON parse first
					if err := json.Unmarshal(v, &metaFields); err != nil {
						// Try to parse as concatenated JSON objects
						metaFields = ParseLengthPrefixedJSON(v)
					}
				case map[string]interface{}:
					metaFields = v
				default:
					continue
				}
			}
		}

		if metaFields == nil {
			continue
		}

		// Process each metadata field
		for fieldName, fieldValue := range metaFields {
			if fieldName == "kb_id" || fieldName == "id" {
				continue
			}

			if _, ok := flattenedMeta[fieldName]; !ok {
				flattenedMeta[fieldName] = make(map[string][]string)
			}

			// Handle list and single values
			var values []interface{}
			switch v := fieldValue.(type) {
			case []interface{}:
				values = v
			default:
				values = []interface{}{v}
			}

			for _, val := range values {
				if val == nil {
					continue
				}
				strVal := fmt.Sprintf("%v", val)
				flattenedMeta[fieldName][strVal] = append(flattenedMeta[fieldName][strVal], docID)
			}
		}
	}

	// Convert to map[string]interface{} for return
	var metaResult map[string]interface{} = make(map[string]interface{})
	for k, v := range flattenedMeta {
		metaResult[k] = v
	}

	return metaResult, nil
}

// valueInfo holds count and order of first appearance
type valueInfo struct {
	count      int
	firstOrder int
}

// aggregateMetadata aggregates metadata from search results
func aggregateMetadata(chunks []map[string]interface{}) map[string]interface{} {
	// summary: map[fieldName]map[value]valueInfo
	summary := make(map[string]map[string]valueInfo)
	typeCounter := make(map[string]map[string]int)
	orderCounter := 0

	for _, chunk := range chunks {
		// For metadata table, the actual metadata is in the "meta_fields" JSON field
		// Extract it first
		metaFieldsVal := chunk["meta_fields"]
		if metaFieldsVal == nil {
			continue
		}

		// Parse meta_fields - could be a string (JSON) or a map
		var metaFields map[string]interface{}
		switch v := metaFieldsVal.(type) {
		case string:
			// Parse JSON string
			if err := json.Unmarshal([]byte(v), &metaFields); err != nil {
				continue
			}
		case []byte:
			// Handle byte slice - Infinity returns concatenated JSON objects with length prefixes
			rawBytes := v

			// Try to detect and handle length-prefixed format
			// Format: [4-byte length][JSON][4-byte length][JSON]...
			parsedMetaFields := make(map[string]interface{})
			offset := 0
			for offset < len(rawBytes) {
				// Need at least 4 bytes for length prefix
				if offset+4 > len(rawBytes) {
					break
				}

				// Read 4-byte length (little-endian, not big-endian!)
				length := uint32(rawBytes[offset]) | uint32(rawBytes[offset+1])<<8 |
					uint32(rawBytes[offset+2])<<16 | uint32(rawBytes[offset+3])<<24

				// Check if length looks valid (not too large)
				if length > 10000 || length == 0 {
					// Try to find next '{' from current position
					nextBrace := -1
					for i := offset; i < len(rawBytes) && i < offset+100; i++ {
						if rawBytes[i] == '{' {
							nextBrace = i
							break
						}
					}
					if nextBrace > offset {
						// Skip to the next '{'
						offset = nextBrace
						continue
					}
					break
				}

				// Extract JSON data
				jsonStart := offset + 4
				jsonEnd := jsonStart + int(length)
				if jsonEnd > len(rawBytes) {
					jsonEnd = len(rawBytes)
				}

				jsonBytes := rawBytes[jsonStart:jsonEnd]

				// Try to parse this JSON
				var singleMeta map[string]interface{}
				if err := json.Unmarshal(jsonBytes, &singleMeta); err == nil {
					// Merge metadata from this document
					for k, vv := range singleMeta {
						if existing, ok := parsedMetaFields[k]; ok {
							// Combine values
							if existList, ok := existing.([]interface{}); ok {
								if newList, ok := vv.([]interface{}); ok {
									parsedMetaFields[k] = append(existList, newList...)
								} else {
									parsedMetaFields[k] = append(existList, vv)
								}
							} else {
								parsedMetaFields[k] = []interface{}{existing, vv}
							}
						} else {
							parsedMetaFields[k] = vv
						}
					}
				}

				offset = jsonEnd
			}

			// If we successfully parsed multiple JSON objects, use the merged result
			if len(parsedMetaFields) > 0 {
				metaFields = parsedMetaFields
			} else {
				// Fallback: try the original parsing method
				startIdx := -1
				for i, b := range rawBytes {
					if b == '{' {
						startIdx = i
						break
					}
				}
				if startIdx > 0 {
					strVal := string(rawBytes[startIdx:])
					if err := json.Unmarshal([]byte(strVal), &metaFields); err != nil {
						metaFields = map[string]interface{}{"raw": strVal}
					}
				} else if err := json.Unmarshal(rawBytes, &metaFields); err != nil {
					metaFields = map[string]interface{}{"raw": string(rawBytes)}
				}
			}
		case map[string]interface{}:
			metaFields = v
		default:
			continue
		}

		// Now iterate over the extracted metadata fields
		for k, v := range metaFields {
			// Skip nil values
			if v == nil {
				continue
			}

			// Determine value type
			valueType := getMetaValueType(v)

			// Track type counts
			if valueType != "" {
				if _, ok := typeCounter[k]; !ok {
					typeCounter[k] = make(map[string]int)
				}
				typeCounter[k][valueType] = typeCounter[k][valueType] + 1
			}

			// Aggregate value counts
			values := v
			if v, ok := v.([]interface{}); ok {
				values = v
			} else {
				values = []interface{}{v}
			}

			for _, vv := range values.([]interface{}) {
				if vv == nil {
					continue
				}
				sv := fmt.Sprintf("%v", vv)

				if _, ok := summary[k]; !ok {
					summary[k] = make(map[string]valueInfo)
				}

				if existing, ok := summary[k][sv]; ok {
					// Already exists, just increment count
					existing.count++
					summary[k][sv] = existing
				} else {
					// First time seeing this value - record order
					summary[k][sv] = valueInfo{count: 1, firstOrder: orderCounter}
					orderCounter++
				}
			}
		}
	}

	// Build result with type information and sorted values
	result := make(map[string]interface{})
	for k, v := range summary {
		// Sort by count descending, then by firstOrder ascending (to match Python stable sort)
		// values: [value, count, firstOrder]
		values := make([][3]interface{}, 0, len(v))
		for val, info := range v {
			values = append(values, [3]interface{}{val, info.count, info.firstOrder})
		}
		// Use stable sort - sort by count descending, then by firstOrder
		sort.SliceStable(values, func(i, j int) bool {
			cntI := values[i][1].(int)
			cntJ := values[j][1].(int)
			if cntI != cntJ {
				return cntI > cntJ // count descending
			}
			// If counts equal, use firstOrder ascending (earlier appearance first)
			return values[i][2].(int) < values[j][2].(int)
		})

		// Determine dominant type
		valueType := "string"
		if typeCounts, ok := typeCounter[k]; ok {
			maxCount := 0
			for t, c := range typeCounts {
				if c > maxCount {
					maxCount = c
					valueType = t
				}
			}
		}

		// Convert from [value, count, firstOrder] to [value, count] for output
		outputValues := make([][2]interface{}, len(values))
		for i, val := range values {
			outputValues[i] = [2]interface{}{val[0], val[1]}
		}

		result[k] = map[string]interface{}{
			"type":   valueType,
			"values": outputValues,
		}
	}

	return result
}

// getMetaValueType determines the type of a metadata value
func getMetaValueType(value interface{}) string {
	if value == nil {
		return ""
	}

	switch v := value.(type) {
	case []interface{}:
		if len(v) > 0 {
			return "list"
		}
		return ""
	case bool:
		return "string"
	case int, int8, int16, int32, int64:
		return "number"
	case float32, float64:
		return "number"
	case string:
		if isTimeString(v) {
			return "time"
		}
		return "string"
	}
	return "string"
}

// isTimeString checks if a string is an ISO 8601 datetime
func isTimeString(s string) bool {
	matched, _ := regexp.MatchString(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$`, s)
	return matched
}
