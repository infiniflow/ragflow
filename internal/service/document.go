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
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/deepdoc/parser/pdf/pdfoxide"
	"ragflow/internal/engine"
	"ragflow/internal/engine/redis"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/server"
	"ragflow/internal/storage"
	"ragflow/internal/tokenizer"
	"ragflow/internal/utility"

	"github.com/cespare/xxhash/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DocumentService document service
type DocumentService struct {
	documentDAO         *dao.DocumentDAO
	kbDAO               *dao.KnowledgebaseDAO
	ingestionTaskDAO    *dao.IngestionTaskDAO
	ingestionTaskLogDAO *dao.IngestionTaskLogDAO
	docEngine           engine.DocEngine
	engineType          server.EngineType
	metadataSvc         *MetadataService
	taskDAO             *dao.TaskDAO
	file2DocumentDAO    *dao.File2DocumentDAO
	fileDAO             *dao.FileDAO
	canvasDAO           *dao.UserCanvasDAO
	api4ConvDAO         *dao.API4ConversationDAO
}

// NewDocumentService create document service
func NewDocumentService() *DocumentService {
	cfg := server.GetConfig()
	return &DocumentService{
		documentDAO:         dao.NewDocumentDAO(),
		ingestionTaskDAO:    dao.NewIngestionTaskDAO(),
		ingestionTaskLogDAO: dao.NewIngestionTaskLogDAO(),
		kbDAO:               dao.NewKnowledgebaseDAO(),
		docEngine:           engine.Get(),
		engineType:          cfg.DocEngine.Type,
		metadataSvc:         NewMetadataService(),
		taskDAO:             dao.NewTaskDAO(),
		file2DocumentDAO:    dao.NewFile2DocumentDAO(),
		fileDAO:             dao.NewFileDAO(),
		canvasDAO:           dao.NewUserCanvasDAO(),
		api4ConvDAO:         dao.NewAPI4ConversationDAO(),
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

const imgBase64Prefix = "data:image/png;base64,"

type ArtifactResponse struct {
	Data            []byte
	ContentType     string
	SafeFilename    string
	ForceAttachment bool
}

type UpdateDatasetDocumentRequest struct {
	Name         *string        `json:"name"`
	ChunkMethod  *string        `json:"chunk_method"`
	ParserID     *string        `json:"parser_id"`
	ChunkCount   *int64         `json:"chunk_count"`
	TokenCount   *int64         `json:"token_count"`
	PipelineID   *string        `json:"pipeline_id"`
	Enabled      *int           `json:"enabled"`
	Progress     *float64       `json:"progress"`
	ParserConfig map[string]any `json:"parser_config"`
	MetaFields   map[string]any `json:"meta_fields"`
}

// PATCH /api/v1/datasets/:dataset_id/documents/:document_id.
type UpdateDatasetDocumentResponse struct {
	ID              string                 `json:"id"`
	Thumbnail       *string                `json:"thumbnail,omitempty"`
	DatasetID       string                 `json:"dataset_id"`
	ChunkMethod     string                 `json:"chunk_method"`
	PipelineID      *string                `json:"pipeline_id,omitempty"`
	ParserConfig    map[string]interface{} `json:"parser_config"`
	SourceType      string                 `json:"source_type"`
	Type            string                 `json:"type"`
	CreatedBy       string                 `json:"created_by"`
	Name            *string                `json:"name,omitempty"`
	Location        *string                `json:"location,omitempty"`
	Size            int64                  `json:"size"`
	TokenCount      int64                  `json:"token_count"`
	ChunkCount      int64                  `json:"chunk_count"`
	Progress        float64                `json:"progress"`
	ProgressMsg     *string                `json:"progress_msg,omitempty"`
	ProcessBeginAt  *time.Time             `json:"process_begin_at,omitempty"`
	ProcessDuration float64                `json:"process_duration"`
	ContentHash     *string                `json:"content_hash,omitempty"`
	MetaFields      map[string]interface{} `json:"meta_fields,omitempty"`
	Suffix          string                 `json:"suffix"`
	Run             string                 `json:"run"`
	Status          *string                `json:"status,omitempty"`
	CreateTime      *int64                 `json:"create_time,omitempty"`
	CreateDate      *time.Time             `json:"create_date,omitempty"`
	UpdateTime      *int64                 `json:"update_time,omitempty"`
	UpdateDate      *time.Time             `json:"update_date,omitempty"`
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
	parts := strings.SplitN(imageID, "-", 2)
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
//
// userID scopes the lookup: a CodeExec sandbox artifact is only
// returned when the caller owns (or has team access to) at least
// one agent session whose `message` references this filename (or
// its `documents/artifact/<name>` URL form). The authorization
// gate runs BEFORE the storage read so a probe of an unknown
// filename cannot distinguish "you cannot see it" from "it
// exists" — both return ErrArtifactNotFound. Mirrors PR #16169.
func (s *DocumentService) GetDocumentArtifact(filename, userID string) (*ArtifactResponse, error) {
	basename := filepath.Base(filename)
	if basename != filename || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return nil, ErrArtifactInvalidFilename
	}

	ext := strings.ToLower(filepath.Ext(basename))
	contentType, ok := artifactContentTypes[ext]
	if !ok {
		return nil, ErrArtifactInvalidFileType
	}

	if !s.sandboxArtifactAccessible(basename, userID) {
		// Same error as "object does not exist" to avoid leaking
		// whether the artifact exists for a different user/agent.
		return nil, ErrArtifactNotFound
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

// sandboxArtifactDialogIDsForUser returns the distinct agent
// (canvas) dialog_ids for sessions owned by userID whose
// `message` blob references filename. A CodeExec artifact URL
// appears in `message` as either a bare filename or the
// `documents/artifact/<name>` form, so the helper matches both.
//
// Implemented as a direct GORM query on the
// API4Conversation table — GORM's `Contains` maps to MySQL
// `LIKE '%...%'` which is fine here because the storage path is
// short and indexed lookup on (user_id, exp_user_id) keeps the
// scan narrow.
func (s *DocumentService) sandboxArtifactDialogIDsForUser(filename, userID string) []string {
	if filename == "" || userID == "" {
		return nil
	}
	// Escape SQL LIKE wildcards (%, _) before building the pattern.
	// Without escaping, a caller could submit a filename like
	// "%.png" or "_" and the LIKE query would match arbitrary
	// referenced artifacts in any user's conversation — letting the
	// caller pass the authorization check against one filename and
	// then GET another artifact by name (PR review round 5, Major #8).
	//
	// Escape character: '!'. We avoid '\\' because SQL string
	// literal parsing of '\\' is driver-specific (SQLite treats
	// it as a single backslash, MySQL treats it as one, Postgres
	// rejects the unterminated string) — '!' is a benign character
	// in real filenames (artifact names rarely contain '!') and
	// parses identically in every driver.
	filenameSafe := escapeSQLLikePattern(filename)
	artifactRefSafe := escapeSQLLikePattern("documents/artifact/" + filename)
	filenamePattern := "%" + filenameSafe + "%"
	artifactRefPattern := "%" + artifactRefSafe + "%"
	dialogIDs := make(map[string]struct{})
	rows, err := dao.DB.Model(&entity.API4Conversation{}).
		Select("dialog_id").
		Where("user_id = ? OR exp_user_id = ?", userID, userID).
		Where(`message LIKE ? ESCAPE '!' OR message LIKE ? ESCAPE '!'`,
			filenamePattern, artifactRefPattern).
		Distinct("dialog_id").
		Rows()
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err == nil && d != "" {
			dialogIDs[d] = struct{}{}
		}
	}
	out := make([]string, 0, len(dialogIDs))
	for d := range dialogIDs {
		out = append(out, d)
	}
	return out
}

// escapeSQLLikePattern escapes the SQL LIKE wildcards ('%', '_') and
// the escape character itself ('!') so a literal user-supplied
// filename can be safely interpolated into a `LIKE ? ESCAPE '!'`
// pattern. Without this, "%.png" would match any string ending in
// ".png" and "_" would match a single character — bypassing the
// filename-specific authorization check. PR review round 5, Major #8.
func escapeSQLLikePattern(s string) string {
	r := strings.NewReplacer(`!`, `!!`, `%`, `!%`, `_`, `!_`)
	return r.Replace(s)
}

// sandboxArtifactAccessible reports whether userID may reach at
// least one agent canvas whose session references filename.
// Mirrors `UserCanvasService.accessible(dialog_id, user_id)` from
// the Python fix; on the Go side this is the same predicate as
// UserCanvasDAO.Accessible (owner or team permission, with the
// latter scoped to the caller's tenant membership — PR review
// round 5).
func (s *DocumentService) sandboxArtifactAccessible(filename, userID string) bool {
	if userID == "" {
		return false
	}
	// Fetch the caller's tenant list once; passing it into
	// canvasDAO.Accessible ensures the team-permission branch only
	// matches canvases the caller can actually see. An empty list
	// (callers without tenant data) is safe — it effectively disables
	// the team branch, so the only matches are canvases the caller
	// directly owns.
	tenantIDs, terr := dao.NewUserTenantDAO().GetTenantIDsByUserID(userID)
	if terr != nil {
		tenantIDs = nil
	}
	for _, dialogID := range s.sandboxArtifactDialogIDsForUser(filename, userID) {
		if s.canvasDAO.Accessible(dialogID, userID, tenantIDs) {
			return true
		}
	}
	return false
}

func sandboxArtifactBucket() string {
	if bucket := os.Getenv("SANDBOX_ARTIFACT_BUCKET"); bucket != "" {
		return bucket
	}
	return "sandbox-artifacts"
}

// Accessible reports whether docID belongs to a knowledge base
// reachable by userID. Used by agent endpoints (e.g. RerunAgent,
// PR #15145) to gate destructive / run-again actions on a document
// the caller has access to. Returns false on any lookup failure or
// empty inputs so callers can treat a denial as a 404-equivalent
// and avoid leaking whether the document exists at all.
func (s *DocumentService) Accessible(docID, userID string) bool {
	if docID == "" || userID == "" {
		return false
	}
	doc, err := s.documentDAO.GetByID(docID)
	if err != nil || doc == nil {
		return false
	}
	return s.kbDAO.Accessible(doc.KbID, userID)
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
		ID:           utility.GenerateUUID(),
		Name:         &req.Name,
		KbID:         req.KbID,
		ParserID:     req.ParserID,
		ParserConfig: entity.JSONMap{},
		CreatedBy:    req.CreatedBy,
		Type:         req.Type,
		SourceType:   req.Source,
		Suffix:       ".doc",
		Status:       func() *string { s := "1"; return &s }(),
	}

	if err := s.InsertDocument(document); err != nil {
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

		result = tx.Model(&entity.Knowledgebase{}).
			Where("id = ?", kbID).
			Updates(map[string]interface{}{
				"doc_num":   gorm.Expr("doc_num - 1"),
				"chunk_num": gorm.Expr("chunk_num - ?", doc.ChunkNum),
				"token_num": gorm.Expr("token_num - ?", doc.TokenNum),
			})
		if result.Error != nil {
			return fmt.Errorf("failed to decrement counters for KB %s: %w", kbID, result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("knowledgebase %s not found", kbID)
		}
		return nil
	})
}

func (s *DocumentService) rollbackAddFileFromKBError(doc *entity.Document, kbID string, err error) error {
	if cleanupErr := s.deleteDocRecordWithCounters(doc, kbID); cleanupErr != nil {
		return fmt.Errorf("%w; rollback cleanup failed: %w", err, cleanupErr)
	}
	return err
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

func (s *DocumentService) GetThumbnails(userID string, docIDs []string) (map[string]string, error) {
	if len(docIDs) == 0 {
		return map[string]string{}, nil
	}

	tenantIDs := []string{userID}
	if userID != "" {
		ids, err := dao.NewUserTenantDAO().GetTenantIDsByUserID(userID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch user tenants: %w", err)
		}
		tenantIDs = append(tenantIDs, ids...)
	}

	documents, err := s.documentDAO.GetByIDsAndTenantIDs(docIDs, tenantIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch document thumbnails: %w", err)
	}

	result := make(map[string]string, len(documents))
	for _, document := range documents {
		if document == nil {
			continue
		}

		thumbnail := ""
		if document.Thumbnail != nil && *document.Thumbnail != "" {
			if strings.HasPrefix(*document.Thumbnail, imgBase64Prefix) {
				thumbnail = *document.Thumbnail
			} else {
				thumbnail = fmt.Sprintf(
					"/api/v1/documents/images/%s-%s",
					document.KbID,
					*document.Thumbnail,
				)
			}
		}

		result[document.ID] = thumbnail
	}

	return result, nil
}

func (s *DocumentService) BatchUpdateDocumentStatus(userID, datasetID, status string, documentIDs []string) (map[string]interface{}, common.ErrorCode, error) {
	kb, err := s.kbDAO.GetByIDAndTenantID(datasetID, userID)
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("You don't own the dataset.")
	}
	statusInt, convErr := strconv.Atoi(status)
	if convErr != nil {
		return nil, common.CodeArgumentError, fmt.Errorf("invalid status: %s", status)
	}

	result := make(map[string]interface{}, len(documentIDs))
	hasError := false

	documents, err := s.documentDAO.GetByIDs(documentIDs)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to fetch documents: %w", err)
	}
	documentByID := make(map[string]*entity.Document, len(documents))
	for _, doc := range documents {
		documentByID[doc.ID] = doc
	}

	for _, docID := range documentIDs {
		doc, ok := documentByID[docID]
		if !ok {
			result[docID] = map[string]string{"error": "Document not found"}
			hasError = true
			continue
		}

		if doc.KbID != datasetID {
			result[docID] = map[string]string{"error": "Document not found in this dataset."}
			hasError = true
			continue
		}

		currentStatus := ""
		if doc.Status != nil {
			currentStatus = *doc.Status
		}
		if currentStatus == status {
			result[docID] = map[string]string{"status": status}
			continue
		}
		previousStatus := interface{}(nil)
		if doc.Status != nil {
			previousStatus = *doc.Status
		}
		if err := s.documentDAO.UpdateByID(docID, map[string]interface{}{"status": status}); err != nil {
			result[docID] = map[string]string{"error": "Database error (Document update)!"}
			hasError = true
			continue
		}

		if doc.ChunkNum > 0 {
			if s.docEngine == nil {
				_ = s.documentDAO.UpdateByID(docID, map[string]interface{}{"status": previousStatus})
				result[docID] = map[string]string{"error": "Document store update failed: document engine not initialized"}
				hasError = true
				continue
			}
			err := s.docEngine.UpdateChunks(
				context.Background(),
				map[string]interface{}{"doc_id": docID},
				map[string]interface{}{"available_int": statusInt},
				fmt.Sprintf("ragflow_%s", kb.TenantID),
				doc.KbID,
			)
			if err != nil {
				_ = s.documentDAO.UpdateByID(docID, map[string]interface{}{"status": previousStatus})
				msg := err.Error()
				if strings.Contains(msg, "3022") {
					result[docID] = map[string]string{"error": "Document store table missing."}
				} else {
					result[docID] = map[string]string{"error": "Document store update failed: " + msg}
				}
				hasError = true
				continue
			}
		}
		result[docID] = map[string]string{"status": status}
	}

	if hasError {
		return result, common.CodeServerError, fmt.Errorf("Partial failure")
	}
	return result, common.CodeSuccess, nil
}

// ListDocumentsByDatasetID list documents by knowledge base ID
func (s *DocumentService) ListDocumentsByDatasetID(kbID, keywords string, page, pageSize int) ([]*entity.DocumentListItem, int64, error) {
	return s.ListDocumentsByDatasetIDWithOptions(dao.DocumentListOptions{
		KbID:     kbID,
		Keywords: keywords,
		OrderBy:  "create_time",
		Desc:     true,
	}, page, pageSize)
}

// ListDocumentsByDatasetIDWithOptions lists documents by knowledge base ID with filters.
func (s *DocumentService) ListDocumentsByDatasetIDWithOptions(opts dao.DocumentListOptions, page, pageSize int) ([]*entity.DocumentListItem, int64, error) {
	opts.Offset = (page - 1) * pageSize
	opts.Limit = pageSize
	if opts.OrderBy == "" {
		opts.OrderBy = "create_time"
	}
	documents, total, err := s.documentDAO.ListByKBIDWithOptions(opts)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*entity.DocumentListItem, len(documents))
	for i, doc := range documents {
		responses[i] = doc
	}

	return responses, total, nil
}

// GetDocumentFiltersByDatasetID returns aggregate filter values for documents in a dataset.
func (s *DocumentService) GetDocumentFiltersByDatasetID(opts dao.DocumentListOptions) (map[string]interface{}, int64, error) {
	filters, total, err := s.documentDAO.GetFilterByKBID(opts)
	if err != nil {
		return nil, 0, err
	}
	docIDs, err := s.documentDAO.ListIDsByKBIDWithOptions(opts)
	if err != nil {
		return nil, 0, err
	}
	metadataFilter, err := s.getDocumentMetadataFilter(opts.KbID, docIDs)
	if err != nil {
		return nil, 0, err
	}
	filters["metadata"] = metadataFilter
	return filters, total, nil
}

func (s *DocumentService) getDocumentMetadataFilter(kbID string, docIDs []string) (map[string]interface{}, error) {
	metadataByKey, err := s.GetMetadataByKBs([]string{kbID})
	if err != nil {
		return nil, err
	}
	candidateSet := make(map[string]bool, len(docIDs))
	for _, docID := range docIDs {
		candidateSet[docID] = true
	}

	metadataCounter := map[string]interface{}{}
	docIDsWithMetadata := map[string]bool{}
	for key, rawValues := range metadataByKey {
		values, ok := rawValues.(map[string][]string)
		if !ok {
			continue
		}
		valueCounter := map[string]int64{}
		for value, valueDocIDs := range values {
			for _, docID := range valueDocIDs {
				if !candidateSet[docID] {
					continue
				}
				valueCounter[value]++
				docIDsWithMetadata[docID] = true
			}
		}
		if len(valueCounter) > 0 {
			metadataCounter[key] = valueCounter
		}
	}
	metadataCounter["empty_metadata"] = map[string]int64{"true": int64(len(docIDs) - len(docIDsWithMetadata))}
	return metadataCounter, nil
}

// ListDocumentIDsByDatasetIDWithOptions lists matching document IDs without pagination.
func (s *DocumentService) ListDocumentIDsByDatasetIDWithOptions(opts dao.DocumentListOptions) ([]string, error) {
	return s.documentDAO.ListIDsByKBIDWithOptions(opts)
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

func (s *DocumentService) ListIngestionTasks(userID string, datasetID *string, page, pageSize int) ([]*entity.IngestionTask, error) {
	offset := (page - 1) * pageSize

	var tasks []*entity.IngestionTask
	var err error
	if datasetID == nil {
		tasks, err = s.ingestionTaskDAO.ListByUserID(userID, offset, pageSize)
	} else {
		tasks, err = s.ingestionTaskDAO.ListByUserIDAndDatasetID(userID, *datasetID, offset, pageSize)
	}

	if err != nil {
		return nil, err
	}

	return tasks, nil
}

type ParseDocumentResponse struct {
	DocumentID string `json:"document_id"`
	Result     string `json:"result"`
}

func (s *DocumentService) IngestDocuments(datasetID, userID string, docIDs []string) ([]*ParseDocumentResponse, error) {
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
				Result:     errorMessage,
			})
			continue
		}

		if doc == nil {
			errorMessage := "no such document"
			responses = append(responses, &ParseDocumentResponse{
				DocumentID: docID,
				Result:     errorMessage,
			})
			continue
		}

		task := &entity.IngestionTask{
			DocumentID: docID,
			UserID:     userID,
			DatasetID:  datasetID,
			Schema:     nil,
			Status:     common.CREATED,
		}

		// save the task to database
		task, err = s.ingestionTaskDAO.CheckAndCreate(task)
		if err != nil {
			errorMessage := err.Error()
			responses = append(responses, &ParseDocumentResponse{
				DocumentID: docID,
				Result:     errorMessage,
			})
			continue
		}

		msgQueueEngine := engine.GetMessageQueueEngine()

		taskMessage := common.TaskMessage{
			TaskID:   task.ID,
			TaskType: common.TaskTypeIngestionTask,
		}

		// convert task
		taskMessageStr, err := json.Marshal(taskMessage)
		if err != nil {
			return nil, err
		}

		err = msgQueueEngine.PublishTask("tasks.RAGFLOW", taskMessageStr)
		if err != nil {
			return nil, err
		}

		responses = append(responses, &ParseDocumentResponse{
			DocumentID: docID,
			Result:     fmt.Sprintf("task_id: %s", task.ID),
		})
	}

	common.Info(fmt.Sprintf("parse documents, dataset: %s, documents: %v", datasetID, docIDs))
	return responses, nil
}

func (s *DocumentService) StopIngestionTasks(tasks []string, userID string) ([]*entity.IngestionTask, error) {

	var taskResponses []*entity.IngestionTask
	for _, taskID := range tasks {
		task, err := s.ingestionTaskDAO.SetStoppingByAPIServer(taskID)
		if err != nil {
			return nil, err
		}
		taskResponses = append(taskResponses, task)
	}
	return taskResponses, nil
}

func (s *DocumentService) RemoveIngestionTasks(tasks []string, userID string) ([]map[string]string, error) {

	var deletedTasks []map[string]string
	for _, taskID := range tasks {
		taskRecord := map[string]string{
			"task_id": taskID,
		}
		_, err := s.ingestionTaskDAO.RemoveByAPIServerOrAdminServer(taskID, &userID)
		if err != nil {
			taskRecord["remove"] = fmt.Sprintf("fail: %s", err.Error())
		} else {
			taskRecord["remove"] = "success"
		}
		deletedTasks = append(deletedTasks, taskRecord)
	}
	return deletedTasks, nil
}

type IngestDocumentRequest struct {
	DocIDs  []string    `json:"doc_ids" binding:"required"`
	Run     interface{} `json:"run" binding:"required"`
	Delete  bool        `json:"delete"`
	ApplyKB bool        `json:"apply_kb"`
}

type documentParsePageRange struct {
	from int64
	to   int64
}

func (s *DocumentService) Ingest(userID string, req *IngestDocumentRequest) (common.ErrorCode, error) {
	run := fmt.Sprint(req.Run)

	docs, err := s.documentDAO.GetByIDs(req.DocIDs)
	if err != nil {
		return common.CodeExceptionError, fmt.Errorf("fail to get documents: %s", err.Error())
	}

	docsByID := make(map[string]*entity.Document, len(docs))
	for _, doc := range docs {
		if doc != nil {
			docsByID[doc.ID] = doc
		}
	}

	tableDoneCountByKB := make(map[string]int64)

	for _, docID := range req.DocIDs {
		doc := docsByID[docID]
		if doc == nil {
			return common.CodeDataError, fmt.Errorf("Document not found!")
		}

		kb, err := s.kbDAO.GetByID(doc.KbID)
		if err != nil {
			return common.CodeDataError, fmt.Errorf("Tenant not found!")
		}

		if !s.kbDAO.Accessible(kb.ID, userID) {
			return common.CodeAuthenticationError, fmt.Errorf("No authorization.")
		}

		updates := map[string]interface{}{
			"run":      run,
			"progress": 0,
		}

		rerunWithDelete := run == string(entity.TaskStatusRunning) && req.Delete
		if rerunWithDelete {
			updates["progress_msg"] = ""
			updates["chunk_num"] = 0
			updates["token_num"] = 0
		}

		if run == string(entity.TaskStatusCancel) {
			if err := s.cancelDocParse(doc); err != nil {
				return common.CodeDataError, err
			}
		}

		if rerunWithDelete {
			if err := s.prepareDocumentRerunWithDelete(doc, kb.TenantID); err != nil {
				return common.CodeExceptionError, err
			}
		}

		if err := s.documentDAO.UpdateByID(doc.ID, updates); err != nil {
			return common.CodeExceptionError, err
		}

		if req.Delete && !rerunWithDelete {
			_, _ = s.taskDAO.DeleteByDocIDs([]string{doc.ID})
			indexName := fmt.Sprintf("ragflow_%s", kb.TenantID)
			if s.docEngine != nil {
				exists, err := s.docEngine.ChunkStoreExists(context.Background(), indexName, doc.KbID)
				if err != nil {
					return common.CodeExceptionError, err
				}
				if exists {
					if _, err := s.docEngine.DeleteChunks(context.Background(), map[string]interface{}{"doc_id": doc.ID}, indexName, doc.KbID); err != nil {
						return common.CodeExceptionError, err
					}
				}
			}
		}

		if run == string(entity.TaskStatusRunning) {
			if req.ApplyKB {
				if doc.ParserConfig == nil {
					doc.ParserConfig = entity.JSONMap{}
				}
				config := map[string]interface{}{
					"llm_id":          kb.ParserConfig["llm_id"],
					"enable_metadata": false,
					"metadata":        map[string]interface{}{},
				}
				if value, ok := kb.ParserConfig["enable_metadata"]; ok {
					config["enable_metadata"] = value
				}
				if value, ok := kb.ParserConfig["metadata"]; ok {
					config["metadata"] = value
				}
				if err := s.updateDocumentParserConfig(doc.ID, config); err != nil {
					return common.CodeExceptionError, err
				}
				for key, value := range config {
					doc.ParserConfig[key] = value
				}
			}
			if doc.PipelineID != nil && strings.TrimSpace(*doc.PipelineID) != "" {
				if err := s.queueDocumentDataflowTask(kb, doc, strings.TrimSpace(*doc.PipelineID), 0); err != nil {
					return common.CodeExceptionError, err
				}
				continue
			}
			if doc.ParserID == string(entity.ParserTypeTable) {
				doneCount, ok := tableDoneCountByKB[doc.KbID]
				if !ok {
					count, err := s.countDoneDocuments(doc.KbID)
					if err != nil {
						return common.CodeExceptionError, err
					}
					doneCount = count
					tableDoneCountByKB[doc.KbID] = doneCount
					if doneCount <= 0 {
						if err := s.kbDAO.DeleteFieldMap(doc.KbID); err != nil && !dao.IsNotFoundErr(err) {
							return common.CodeExceptionError, err
						}
					}
				}
			}
			if _, err := s.taskDAO.DeleteByDocIDs([]string{doc.ID}); err != nil {
				return common.CodeExceptionError, err
			}
			bucket, objectName, err := s.GetDocumentStorageAddress(doc)
			if err != nil {
				return common.CodeExceptionError, err
			}
			if err := s.queueDocumentParseTasks(doc, bucket, objectName, 0); err != nil {
				return common.CodeExceptionError, err
			}
			if err := s.beginDocumentParse(doc.ID); err != nil {
				return common.CodeExceptionError, err
			}
		}
	}

	return common.CodeSuccess, nil
}

func (s *DocumentService) prepareDocumentRerunWithDelete(doc *entity.Document, tenantID string) error {
	if doc == nil {
		return fmt.Errorf("document is nil")
	}

	s.cancelExistingParseTasksBestEffort(doc.ID)

	if _, err := s.taskDAO.DeleteByDocIDs([]string{doc.ID}); err != nil {
		return err
	}

	if err := s.clearDocumentAndKBCountersForRerun(doc.ID, doc.KbID); err != nil {
		return err
	}

	if s.docEngine == nil {
		return nil
	}

	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	exists, err := s.docEngine.ChunkStoreExists(context.Background(), indexName, doc.KbID)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if _, err := s.docEngine.DeleteChunks(context.Background(), map[string]interface{}{"doc_id": doc.ID}, indexName, doc.KbID); err != nil {
		return err
	}
	return nil
}

func (s *DocumentService) cancelExistingParseTasksBestEffort(docID string) {
	tasks, err := s.taskDAO.GetByDocID(docID)
	if err != nil {
		common.Logger.Warn(fmt.Sprintf("cancelExistingParseTasksBestEffort: failed to get tasks for %s: %v", docID, err))
		return
	}
	redisClient := redis.Get()
	if redisClient == nil {
		return
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		redisClient.Set(fmt.Sprintf("%s-cancel", task.ID), "x", 24*time.Hour)
	}
}

func (s *DocumentService) clearDocumentAndKBCountersForRerun(docID, kbID string) error {
	return dao.DB.Transaction(func(tx *gorm.DB) error {
		var current entity.Document
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND kb_id = ?", docID, kbID).
			First(&current).Error; err != nil {
			return err
		}

		if current.TokenNum == 0 && current.ChunkNum == 0 && current.ProcessDuration == 0 {
			return nil
		}

		result := tx.Model(&entity.Document{}).
			Where("id = ? AND kb_id = ?", docID, kbID).
			Updates(map[string]interface{}{
				"token_num":        0,
				"chunk_num":        0,
				"process_duration": 0,
			})
		if result.Error != nil {
			return result.Error
		}
		if current.TokenNum == 0 && current.ChunkNum == 0 {
			return nil
		}

		result = tx.Model(&entity.Knowledgebase{}).
			Where("id = ?", kbID).
			Updates(map[string]interface{}{
				"token_num": gorm.Expr("token_num - ?", current.TokenNum),
				"chunk_num": gorm.Expr("chunk_num - ?", current.ChunkNum),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("knowledgebase not found")
		}
		return nil
	})
}

func (s *DocumentService) countDoneDocuments(datasetID string) (int64, error) {
	var count int64
	err := dao.GetDB().Model(&entity.Document{}).
		Where("kb_id = ? AND run = ?", datasetID, string(entity.TaskStatusDone)).
		Count(&count).Error
	return count, err
}

func (s *DocumentService) queueDocumentParseTasks(doc *entity.Document, bucket, objectName string, priority int64) error {
	if _, err := s.taskDAO.DeleteByDocIDs([]string{doc.ID}); err != nil {
		return err
	}
	tasks, err := s.newDocumentParseTasks(doc, bucket, objectName, priority)
	if err != nil {
		return err
	}
	if err := s.taskDAO.CreateMany(tasks); err != nil {
		return err
	}
	queueName := documentParseQueueName(doc, priority)
	for _, task := range tasks {
		if task.Progress >= 1 {
			continue
		}
		if redisClient := redis.Get(); redisClient == nil || !redisClient.QueueProduct(queueName, documentTaskMessage(task)) {
			return fmt.Errorf("Can't access Redis. Please check the Redis' status.")
		}
	}
	return nil
}

func (s *DocumentService) queueDocumentDataflowTask(kb *entity.Knowledgebase, doc *entity.Document, flowID string, priority int64) error {
	if _, err := s.taskDAO.DeleteByDocIDs([]string{doc.ID}); err != nil {
		return err
	}
	if err := s.beginDocumentParse(doc.ID); err != nil {
		return err
	}
	task := s.newDocumentParseTask(doc, 0, maximumTaskPageNumber, priority)
	task.TaskType = "dataflow"
	if err := s.taskDAO.CreateMany([]*entity.Task{task}); err != nil {
		return err
	}
	message := documentTaskMessage(task)
	message["task_type"] = task.TaskType
	message["kb_id"] = doc.KbID
	message["tenant_id"] = kb.TenantID
	message["dataflow_id"] = flowID
	message["file"] = nil
	if redisClient := redis.Get(); redisClient == nil || !redisClient.QueueProduct(documentParseQueueName(doc, priority), message) {
		return fmt.Errorf("Can't access Redis. Please check the Redis' status.")
	}
	return nil
}

func (s *DocumentService) newDocumentParseTasks(doc *entity.Document, bucket, objectName string, priority int64) ([]*entity.Task, error) {
	ranges, err := documentParseTaskRanges(doc, bucket, objectName)
	if err != nil {
		return nil, err
	}
	tasks := make([]*entity.Task, 0, len(ranges))
	for _, pageRange := range ranges {
		tasks = append(tasks, s.newDocumentParseTask(doc, pageRange.from, pageRange.to, priority))
	}
	return tasks, nil
}

func (s *DocumentService) newDocumentParseTask(doc *entity.Document, fromPage, toPage, priority int64) *entity.Task {
	now := time.Now()
	progressMsg := ""
	digest := documentParseTaskDigest(doc, fromPage, toPage)
	chunkIDs := ""
	return &entity.Task{
		ID:          utility.GenerateUUID(),
		DocID:       doc.ID,
		FromPage:    fromPage,
		ToPage:      toPage,
		TaskType:    "",
		Priority:    priority,
		BeginAt:     &now,
		Progress:    0,
		ProgressMsg: &progressMsg,
		Digest:      &digest,
		ChunkIDs:    &chunkIDs,
	}
}

func documentParseTaskRanges(doc *entity.Document, bucket, objectName string) ([]documentParsePageRange, error) {
	if doc.Type == "pdf" {
		binary, err := documentStorageBinary(bucket, objectName)
		if err != nil {
			return nil, err
		}
		pages := documentEstimatePDFPageCount(binary)
		pageSize := int64(documentParserConfigInt(doc.ParserConfig, "task_page_size", 12))
		if doc.ParserID == string(entity.ParserTypePaper) {
			pageSize = int64(documentParserConfigInt(doc.ParserConfig, "task_page_size", 22))
		}
		if doc.ParserID == string(entity.ParserTypeOne) ||
			doc.ParserID == string(entity.ParserTypeKG) ||
			documentParserConfigBool(doc.ParserConfig, "toc_extraction", false) {
			pageSize = maximumTaskPageNumber
		}
		if pageSize <= 0 {
			pageSize = 12
		}
		ranges := make([]documentParsePageRange, 0)
		for _, configuredRange := range documentParserConfigPageRanges(doc.ParserConfig) {
			start := configuredRange.from - 1
			if start < 0 {
				start = 0
			}
			end := configuredRange.to - 1
			if pages >= 0 && end > pages {
				end = pages
			}
			for page := start; page < end; page += pageSize {
				to := page + pageSize
				if to > end {
					to = end
				}
				ranges = append(ranges, documentParsePageRange{from: page, to: to})
			}
		}
		if len(ranges) == 0 {
			// pages == 0 means page count detection failed (e.g. compressed
			// PDF where both regex and pdfoxide fallbacks failed). Fall back
			// to maximumTaskPageNumber so the Python parser processes all
			// pages via slicing (Python gracefully caps at actual page count).
			ranges = append(ranges, documentParsePageRange{from: 0, to: maximumTaskPageNumber})
		}
		return ranges, nil
	}
	if doc.ParserID == string(entity.ParserTypeTable) {
		binary, err := documentStorageBinary(bucket, objectName)
		if err != nil {
			return nil, err
		}
		rows := documentEstimateTableRowCount(documentName(doc), binary)
		if rows <= 0 {
			return []documentParsePageRange{{from: 0, to: maximumTaskPageNumber}}, nil
		}
		ranges := make([]documentParsePageRange, 0, (rows+2999)/3000)
		for row := int64(0); row < int64(rows); row += 3000 {
			to := row + 3000
			if to > int64(rows) {
				to = int64(rows)
			}
			ranges = append(ranges, documentParsePageRange{from: row, to: to})
		}
		return ranges, nil
	}
	return []documentParsePageRange{{from: 0, to: maximumTaskPageNumber}}, nil
}

func documentStorageBinary(bucket, objectName string) ([]byte, error) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}
	return storageImpl.Get(bucket, objectName)
}

func documentName(doc *entity.Document) string {
	if doc == nil || doc.Name == nil {
		return ""
	}
	return *doc.Name
}

func documentParserConfigInt(config map[string]interface{}, key string, fallback int) int {
	value, ok := config[key]
	if !ok || value == nil {
		return fallback
	}
	switch typedValue := value.(type) {
	case int:
		return typedValue
	case int64:
		return int(typedValue)
	case float64:
		return int(typedValue)
	case json.Number:
		if intValue, err := typedValue.Int64(); err == nil {
			return int(intValue)
		}
	case string:
		if intValue, err := strconv.Atoi(strings.TrimSpace(typedValue)); err == nil {
			return intValue
		}
	}
	return fallback
}

func documentParserConfigBool(config map[string]interface{}, key string, fallback bool) bool {
	value, ok := config[key]
	if !ok || value == nil {
		return fallback
	}
	switch typedValue := value.(type) {
	case bool:
		return typedValue
	case string:
		switch strings.ToLower(strings.TrimSpace(typedValue)) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}
	return fallback
}

func documentParserConfigPageRanges(config map[string]interface{}) []documentParsePageRange {
	defaultRanges := []documentParsePageRange{{from: 1, to: 100000}}
	raw, ok := config["pages"]
	if !ok || raw == nil {
		return defaultRanges
	}
	rawRanges, ok := raw.([]interface{})
	if !ok || len(rawRanges) == 0 {
		return defaultRanges
	}
	ranges := make([]documentParsePageRange, 0, len(rawRanges))
	for _, rawRange := range rawRanges {
		rangeValues, ok := rawRange.([]interface{})
		if !ok || len(rangeValues) < 2 {
			continue
		}
		from, okFrom := documentToInt64(rangeValues[0])
		to, okTo := documentToInt64(rangeValues[1])
		if okFrom && okTo && to > from {
			ranges = append(ranges, documentParsePageRange{from: from, to: to})
		}
	}
	if len(ranges) == 0 {
		return defaultRanges
	}
	return ranges
}

func documentToInt64(value interface{}) (int64, bool) {
	switch typedValue := value.(type) {
	case int:
		return int64(typedValue), true
	case int64:
		return typedValue, true
	case float64:
		return int64(typedValue), true
	case json.Number:
		intValue, err := typedValue.Int64()
		return intValue, err == nil
	case string:
		intValue, err := strconv.ParseInt(strings.TrimSpace(typedValue), 10, 64)
		return intValue, err == nil
	default:
		return 0, false
	}
}

var documentPDFPagePattern = regexp.MustCompile(`/Type\s*/Page\b`)

func documentEstimatePDFPageCount(binary []byte) int64 {
	if len(binary) == 0 {
		return 0
	}
	// Fast path: regex works for uncompressed PDFs.
	count := int64(len(documentPDFPagePattern.FindAll(binary, -1)))
	if count > 0 {
		return count
	}
	// Fallback for compressed PDFs where /Type /Page is inside a
	// compressed object stream: use pdf_oxide to get the real page count.
	if doc, err := pdfoxide.OpenBytes(binary); err == nil {
		defer doc.Close()
		if pages, err := doc.PageCount(); err == nil {
			return int64(pages)
		}
	}
	return 0
}

func documentEstimateTableRowCount(name string, binary []byte) int {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".xlsx":
		if rows, err := documentCountXLSXRows(binary); err == nil {
			return rows
		}
	case ".csv", ".tsv", ".txt":
		return documentCountDelimitedRows(name, binary)
	}
	return 0
}

func documentCountDelimitedRows(name string, binary []byte) int {
	reader := csv.NewReader(bytes.NewReader(binary))
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = true
	if strings.EqualFold(filepath.Ext(name), ".tsv") {
		reader.Comma = '\t'
	}
	rows := 0
	for {
		_, err := reader.Read()
		if err == nil {
			rows++
			continue
		}
		if err == io.EOF {
			break
		}
		rows += bytes.Count(binary, []byte{'\n'})
		if len(binary) > 0 && binary[len(binary)-1] != '\n' {
			rows++
		}
		break
	}
	return rows
}

func documentCountXLSXRows(binary []byte) (int, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(binary), int64(len(binary)))
	if err != nil {
		return 0, err
	}
	maxRows := 0
	for _, file := range zipReader.File {
		if !strings.HasPrefix(file.Name, "xl/worksheets/") || !strings.HasSuffix(file.Name, ".xml") {
			continue
		}
		rows, err := documentCountWorksheetRows(file)
		if err != nil {
			return 0, err
		}
		if rows > maxRows {
			maxRows = rows
		}
	}
	return maxRows, nil
}

func documentCountWorksheetRows(file *zip.File) (int, error) {
	reader, err := file.Open()
	if err != nil {
		return 0, err
	}
	defer reader.Close()
	decoder := xml.NewDecoder(reader)
	rows := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
		start, ok := token.(xml.StartElement)
		if ok && start.Name.Local == "row" {
			rows++
		}
	}
	return rows, nil
}

func (s *DocumentService) beginDocumentParse(docID string) error {
	now := time.Now()
	return dao.GetDB().Model(&entity.Document{}).Where("id = ?", docID).Updates(map[string]interface{}{
		"progress_msg":     "Task is queued...",
		"process_begin_at": now,
		"progress":         rand.Float64() * 0.01,
		"run":              string(entity.TaskStatusRunning),
		"chunk_num":        0,
		"token_num":        0,
	}).Error
}

func documentParseQueueName(doc *entity.Document, priority int64) string {
	suffix := "common"
	if doc.ParserID == string(entity.ParserTypeResume) {
		suffix = "resume"
	}
	return fmt.Sprintf("te.%d.%s", priority, suffix)
}

func documentTaskMessage(task *entity.Task) map[string]interface{} {
	beginAt := ""
	if task.BeginAt != nil {
		beginAt = task.BeginAt.Format("2006-01-02 15:04:05")
	}
	digest := ""
	if task.Digest != nil {
		digest = *task.Digest
	}
	return map[string]interface{}{
		"id":        task.ID,
		"doc_id":    task.DocID,
		"from_page": task.FromPage,
		"to_page":   task.ToPage,
		"progress":  task.Progress,
		"priority":  task.Priority,
		"begin_at":  beginAt,
		"digest":    digest,
	}
}

func documentParseTaskDigest(doc *entity.Document, fromPage, toPage int64) string {
	hasher := xxhash.New()
	config := map[string]interface{}{
		"doc_id":        doc.ID,
		"kb_id":         doc.KbID,
		"parser_id":     doc.ParserID,
		"parser_config": doc.ParserConfig,
	}
	keys := make([]string, 0, len(config))
	for key := range config {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		b, err := json.Marshal(config[key])
		if err != nil {
			hasher.WriteString(fmt.Sprint(config[key]))
		} else {
			hasher.Write(b)
		}
	}
	hasher.WriteString(doc.ID)
	hasher.WriteString(strconv.FormatInt(fromPage, 10))
	hasher.WriteString(strconv.FormatInt(toPage, 10))
	return fmt.Sprintf("%x", hasher.Sum64())
}

func (s *DocumentService) clearKBChunkNumWhenRerun(doc *entity.Document) error {
	if doc == nil {
		return fmt.Errorf("document is nil")
	}
	return dao.GetDB().Model(&entity.Knowledgebase{}).Where("id = ?", doc.KbID).Updates(map[string]interface{}{
		"token_num": gorm.Expr("token_num - ?", doc.TokenNum),
		"chunk_num": gorm.Expr("chunk_num - ?", doc.ChunkNum),
	}).Error
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
				Result:     errorMessage,
			})
			continue
		}
		if doc == nil {
			errorMessage := "no such document"
			responses = append(responses, &ParseDocumentResponse{
				DocumentID: docID,
				Result:     errorMessage,
			})
			continue
		}

		if doc.Status != nil && *doc.Status != "0" {
			errorMessage := fmt.Sprintf("document %s is already parsed", docID)
			responses = append(responses, &ParseDocumentResponse{
				DocumentID: docID,
				Result:     errorMessage,
			})
			continue
		}

		// create task for each document
		//task := &entity.IngestionTask{
		//	ID:         utility.GenerateToken(),
		//	DocumentID: docID,
		//	UserID:     userID,
		//}

		// save the task to database
		//err = s.ingestionTaskDAO.Create(task)
		//if err != nil {
		//	errorMessage := err.Error()
		//	responses = append(responses, &ParseDocumentResponse{
		//		DocumentID: docID,
		//		Result:     &errorMessage,
		//	})
		//	continue
		//}

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
	redisClient := redis.Get()
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

	if err := s.docEngine.UpdateMetadata(context.Background(), docID, doc.KbID, meta, tenantID); err != nil {
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

			// Aggregate value counts. Flatten nested arrays so malformed values do
			// not surface in the UI as the literal string "[]".
			values := flattenMetadataSummaryValues(v)
			for _, vv := range values {
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

func flattenMetadataSummaryValues(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		result := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			result = append(result, flattenMetadataSummaryValues(item)...)
		}
		return result
	case []string:
		result := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result
	case nil:
		return nil
	default:
		return []interface{}{typed}
	}
}

// isTimeString checks if a string is an ISO 8601 datetime
func isTimeString(s string) bool {
	matched, _ := regexp.MatchString(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$`, s)
	return matched
}

func (s *DocumentService) UpdateDatasetDocument(userID, datasetID, documentID string, req *UpdateDatasetDocumentRequest, present map[string]bool) (*UpdateDatasetDocumentResponse, common.ErrorCode, error) {
	tenantID := userID
	kb, err := s.kbDAO.GetByIDAndTenantID(datasetID, tenantID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("You don't own the dataset.")
		}
		return nil, common.CodeDataError, errors.New("Can't find this dataset!")
	}

	doc, err := s.documentDAO.GetByDocumentIDAndDatasetID(documentID, datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("The dataset doesn't own the document.")
		}
		return nil, common.CodeServerError, err
	}

	if code, err := s.validateDatasetDocumentUpdate(doc, req, present); err != nil {
		return nil, code, err
	}

	if present["meta_fields"] {
		if err := s.replaceDocumentMetadata(documentID, req.MetaFields); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	if present["name"] && req.Name != nil && (doc.Name == nil || *req.Name != *doc.Name) {
		if err := s.updateDocumentNameOnly(doc, kb.TenantID, *req.Name); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	if present["parser_config"] && req.ParserConfig != nil {
		if err := s.updateDocumentParserConfig(doc.ID, req.ParserConfig); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	if req.PipelineID != nil && *req.PipelineID != "" {
		if err := s.resetDocumentForReparse(doc, kb.TenantID, nil, req.PipelineID); err != nil {
			return nil, common.CodeDataError, err
		}
	} else if present["parser_id"] && req.ParserID != nil && strings.TrimSpace(*req.ParserID) != "" {
		parserID := strings.TrimSpace(*req.ParserID)
		if err := s.resetDocumentForReparse(doc, kb.TenantID, &parserID, nil); err != nil {
			return nil, common.CodeDataError, err
		}
	} else if req.ChunkMethod != nil && *req.ChunkMethod != "" {
		if err := s.updateChunkMethod(doc, kb.TenantID, *req.ChunkMethod, req.ParserConfig, present["parser_config"]); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	if present["enabled"] && req.Enabled != nil {
		if err := s.updateDocumentStatusOnly(doc, kb, *req.Enabled); err != nil {
			return nil, common.CodeServerError, err
		}
	}

	updatedDoc, err := s.documentDAO.GetByID(doc.ID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("Can not get document by id:%s", doc.ID)
		}
		return nil, common.CodeDataError, errors.New("Database operation failed")
	}

	metaFields := map[string]interface{}{}
	if s.docEngine != nil && s.metadataSvc != nil {
		metaFields, _ = s.GetDocumentMetadataByID(updatedDoc.ID)
	}

	return s.toUpdateDatasetDocumentResponse(updatedDoc, metaFields), common.CodeSuccess, nil
}

var allowedDocumentChunkMethods = map[string]struct{}{
	"naive":           {},
	"manual":          {},
	"qa":              {},
	"table":           {},
	"paper":           {},
	"book":            {},
	"laws":            {},
	"presentation":    {},
	"picture":         {},
	"one":             {},
	"knowledge_graph": {},
	"email":           {},
	"tag":             {},
}

func (s *DocumentService) validateDatasetDocumentUpdate(doc *entity.Document, req *UpdateDatasetDocumentRequest, present map[string]bool) (common.ErrorCode, error) {
	if req == nil {
		return common.CodeDataError, errors.New("Invalid request payload")
	}
	if present["chunk_count"] && req.ChunkCount != nil && *req.ChunkCount != 0 && *req.ChunkCount != doc.ChunkNum {
		return common.CodeDataError, errors.New("Can't change `chunk_count`.")
	}
	if present["token_count"] && req.TokenCount != nil && *req.TokenCount != 0 && *req.TokenCount != doc.TokenNum {
		return common.CodeDataError, errors.New("Can't change `token_count`.")
	}
	if present["progress"] && req.Progress != nil && *req.Progress != 0 && math.Abs(*req.Progress-doc.Progress) > 1e-9 {
		return common.CodeDataError, errors.New("Can't change `progress`.")
	}

	if present["enabled"] {
		if req.Enabled == nil || (*req.Enabled != 0 && *req.Enabled != 1) {
			return common.CodeDataError, errors.New("`enabled` value invalid, only accept 0 or 1")
		}
	}

	if present["chunk_method"] {
		if req.ChunkMethod == nil || strings.TrimSpace(*req.ChunkMethod) == "" {
			return common.CodeDataError, errors.New("`chunk_method` (empty string) is not valid")
		}
		chunkMethod := strings.TrimSpace(*req.ChunkMethod)
		if _, ok := allowedDocumentChunkMethods[chunkMethod]; !ok {
			return common.CodeDataError, fmt.Errorf("`chunk_method` %s doesn't exist", chunkMethod)
		}
		if doc.Type == "visual" || isPresentationFile(doc.Name) {
			return common.CodeDataError, errors.New("Not supported yet!")
		}
	}
	if present["parser_id"] && req.ParserID != nil {
		parserID := strings.TrimSpace(*req.ParserID)
		if (doc.Type == "visual" && parserID != "picture") || (isPresentationFile(doc.Name) && parserID != "presentation") {
			return common.CodeDataError, errors.New("Not supported yet!")
		}
	}
	if present["name"] && req.Name != nil {
		if err := s.validateDocumentName(doc, *req.Name); err != nil {
			return common.CodeDataError, err
		}
	}

	if present["meta_fields"] {
		if err := validateMetaFields(req.MetaFields); err != nil {
			return common.CodeDataError, err
		}
	}

	return common.CodeSuccess, nil
}

func (s *DocumentService) validateDocumentName(doc *entity.Document, newName string) error {
	if strings.TrimSpace(newName) == "" {
		return errors.New("File name can't be empty.")
	}
	if len([]byte(newName)) > 255 {
		return errors.New("File name must be 255 bytes or less.")
	}

	oldName := ""
	if doc.Name != nil {
		oldName = *doc.Name
	}

	if strings.ToLower(filepath.Ext(newName)) != strings.ToLower(filepath.Ext(oldName)) {
		return errors.New("The extension of file can't be changed")
	}

	docs, err := s.documentDAO.GetByNameAndKBID(newName, doc.KbID)
	if err != nil {
		return err
	}
	for _, d := range docs {
		if d.ID != doc.ID && d.Name != nil && *d.Name == newName {
			return errors.New("Duplicated document name in the same dataset.")
		}
	}

	return nil
}

func isPresentationFile(name *string) bool {
	if name == nil {
		return false
	}
	ext := strings.ToLower(filepath.Ext(*name))
	return ext == ".ppt" || ext == ".pptx" || ext == ".pages"
}

func validateMetaFields(meta map[string]any) error {
	if meta == nil {
		return nil
	}

	for _, v := range meta {
		switch typed := v.(type) {
		case string, float64, int, int64, float32:
			continue
		case []any:
			for _, item := range typed {
				switch item.(type) {
				case string, float64, int, int64, float32:
					continue
				default:
					return fmt.Errorf("The type is not supported in list: %v", typed)
				}
			}
		default:
			return fmt.Errorf("The type is not supported: %v", v)
		}
	}

	return nil
}

func (s *DocumentService) replaceDocumentMetadata(docID string, meta map[string]any) error {
	if s.docEngine == nil || s.metadataSvc == nil {
		return nil
	}
	if err := s.DeleteDocumentAllMetadata(docID); err != nil {
		return err
	}
	return s.SetDocumentMetadata(docID, map[string]interface{}(meta))
}

func (s *DocumentService) patchDocumentMetadata(docID string, before, after map[string]interface{}) error {
	if s.docEngine == nil || s.metadataSvc == nil {
		return nil
	}

	deleteKeys := make([]string, 0)
	for key := range before {
		if _, ok := after[key]; !ok {
			deleteKeys = append(deleteKeys, key)
		}
	}
	if len(deleteKeys) > 0 {
		if err := s.DeleteDocumentMetadata(docID, deleteKeys); err != nil {
			return err
		}
	}

	updateFields := make(map[string]interface{})
	for key, value := range after {
		if !reflect.DeepEqual(before[key], value) {
			updateFields[key] = value
		}
	}
	if len(updateFields) == 0 {
		return nil
	}
	return s.SetDocumentMetadata(docID, updateFields)
}

func (s *DocumentService) updateDocumentNameOnly(doc *entity.Document, tenantID, newName string) error {
	if err := s.documentDAO.UpdateByID(doc.ID, map[string]interface{}{"name": newName}); err != nil {
		return errors.New("Database error (Document rename)!")
	}

	mappings, err := s.file2DocumentDAO.GetByDocumentID(doc.ID)
	if err == nil && len(mappings) > 0 && mappings[0].FileID != nil && s.fileDAO != nil {
		_ = s.fileDAO.UpdateByID(*mappings[0].FileID, map[string]interface{}{"name": newName})
	}

	if s.docEngine == nil {
		return nil
	}

	titleTks, _ := tokenizer.Tokenize(newName)
	titleSmTks, _ := tokenizer.FineGrainedTokenize(titleTks)
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	return s.docEngine.UpdateChunks(
		context.Background(),
		map[string]interface{}{"doc_id": doc.ID},
		map[string]interface{}{
			"docnm_kwd":    newName,
			"title_tks":    titleTks,
			"title_sm_tks": titleSmTks,
		},
		indexName,
		doc.KbID,
	)
}

func (s *DocumentService) updateDocumentParserConfig(documentID string, config map[string]any) error {
	if len(config) == 0 {
		return nil
	}

	doc, err := s.documentDAO.GetByID(documentID)
	if err != nil {
		return fmt.Errorf("Document(%s) not found.", documentID)
	}

	merged := common.DeepMergeMaps(map[string]interface{}(doc.ParserConfig), map[string]interface{}(config))
	if _, ok := config["raptor"]; !ok {
		delete(merged, "raptor")
	}

	return s.documentDAO.UpdateByID(documentID, map[string]interface{}{
		"parser_config": entity.JSONMap(merged),
	})
}

func (s *DocumentService) resetDocumentForReparse(doc *entity.Document, tenantID string, parserID *string, pipelineID *string) error {
	progressMsg := ""
	run := string(entity.TaskStatusUnstart)
	updates := map[string]interface{}{
		"progress":     0,
		"progress_msg": progressMsg,
		"run":          run,
	}
	if parserID != nil {
		updates["parser_id"] = *parserID
	}
	if pipelineID != nil {
		updates["pipeline_id"] = *pipelineID
	}

	if err := s.documentDAO.UpdateByID(doc.ID, updates); err != nil {
		return errors.New("Document not found!")
	}

	if doc.TokenNum > 0 {
		decremented, err := s.decrementDocumentAndKBCountersForReparse(doc)
		if err != nil {
			return errors.New("Document not found!")
		}
		if !decremented {
			return nil
		}
		if s.docEngine != nil {
			indexName := fmt.Sprintf("ragflow_%s", tenantID)
			s.deleteChunkImages(doc, indexName)
			if _, err := s.docEngine.DeleteChunks(context.Background(), map[string]interface{}{"doc_id": doc.ID}, indexName, doc.KbID); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *DocumentService) deleteChunkImages(doc *entity.Document, indexName string) {
	if s.docEngine == nil {
		return
	}
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return
	}

	const pageSize = 1000
	for offset := 0; ; offset += pageSize {
		result, err := s.docEngine.Search(context.Background(), &enginetypes.SearchRequest{
			IndexNames:   []string{indexName},
			KbIDs:        []string{doc.KbID},
			Offset:       offset,
			Limit:        pageSize,
			SelectFields: []string{"id", "img_id"},
			Filter:       map[string]interface{}{"doc_id": doc.ID},
			MatchExprs:   nil,
			OrderBy:      nil,
			RankFeature:  nil,
		})
		if err != nil || result == nil || len(result.Chunks) == 0 {
			return
		}
		for _, chunk := range result.Chunks {
			imageKey, ok := chunkImageStorageKey(doc.KbID, chunk)
			if !ok {
				continue
			}
			if storageImpl.ObjExist(doc.KbID, imageKey) {
				_ = storageImpl.Remove(doc.KbID, imageKey)
			}
		}
	}
}

func chunkImageStorageKey(defaultBucket string, chunk map[string]interface{}) (string, bool) {
	imgID := firstStringField(chunk, "img_id")
	if imgID != "" {
		prefix := defaultBucket + "-"
		if strings.HasPrefix(imgID, prefix) && len(imgID) > len(prefix) {
			return strings.TrimPrefix(imgID, prefix), true
		}
		return imgID, true
	}

	chunkID := firstStringField(chunk, "id", "_id")
	if chunkID == "" {
		return "", false
	}
	return chunkID, true
}

func firstStringField(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			if s, ok := value.(string); ok {
				return s
			}
		}
	}
	return ""
}

func (s *DocumentService) decrementDocumentAndKBCountersForReparse(doc *entity.Document) (bool, error) {
	decremented := false
	err := dao.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&entity.Document{}).
			Where("id = ? AND kb_id = ? AND token_num = ? AND chunk_num = ?", doc.ID, doc.KbID, doc.TokenNum, doc.ChunkNum).
			Updates(map[string]interface{}{
				"token_num":        gorm.Expr("token_num - ?", doc.TokenNum),
				"chunk_num":        gorm.Expr("chunk_num - ?", doc.ChunkNum),
				"process_duration": gorm.Expr("process_duration - ?", doc.ProcessDuration),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		decremented = true

		return tx.Model(&entity.Knowledgebase{}).
			Where("id = ?", doc.KbID).
			Updates(map[string]interface{}{
				"token_num": gorm.Expr("token_num - ?", doc.TokenNum),
				"chunk_num": gorm.Expr("chunk_num - ?", doc.ChunkNum),
			}).Error
	})
	return decremented, err
}

func (s *DocumentService) updateChunkMethod(doc *entity.Document, tenantID string, chunkMethod string, parserConfig map[string]any, hasParserConfig bool) error {
	chunkMethod = strings.TrimSpace(chunkMethod)
	if !strings.EqualFold(doc.ParserID, chunkMethod) {
		if err := s.resetDocumentForReparse(doc, tenantID, &chunkMethod, nil); err != nil {
			return err
		}
	}
	if !hasParserConfig {
		defaultConfig := common.GetParserConfig(chunkMethod, nil)
		if err := s.updateDocumentParserConfig(doc.ID, defaultConfig); err != nil {
			return err
		}
	}
	return nil
}

func (s *DocumentService) updateDocumentStatusOnly(doc *entity.Document, kb *entity.Knowledgebase, status int) error {
	statusStr := strconv.Itoa(status)
	if doc.Status != nil && *doc.Status == statusStr {
		return nil
	}

	if err := s.documentDAO.UpdateByID(doc.ID, map[string]interface{}{"status": statusStr}); err != nil {
		return errors.New("Database error (Document update)!")
	}

	if s.docEngine == nil {
		return nil
	}

	indexName := fmt.Sprintf("ragflow_%s", kb.TenantID)
	return s.docEngine.UpdateChunks(
		context.Background(),
		map[string]interface{}{"doc_id": doc.ID},
		map[string]interface{}{"available_int": status},
		indexName,
		doc.KbID,
	)
}

func (s *DocumentService) toUpdateDatasetDocumentResponse(doc *entity.Document, metaFields map[string]interface{}) *UpdateDatasetDocumentResponse {
	if metaFields == nil {
		metaFields = map[string]interface{}{}
	}
	return &UpdateDatasetDocumentResponse{
		ID:              doc.ID,
		Thumbnail:       doc.Thumbnail,
		DatasetID:       doc.KbID,
		ChunkMethod:     doc.ParserID,
		PipelineID:      doc.PipelineID,
		ParserConfig:    map[string]interface{}(doc.ParserConfig),
		SourceType:      doc.SourceType,
		Type:            doc.Type,
		CreatedBy:       doc.CreatedBy,
		Name:            doc.Name,
		Location:        doc.Location,
		Size:            doc.Size,
		TokenCount:      doc.TokenNum,
		ChunkCount:      doc.ChunkNum,
		Progress:        doc.Progress,
		ProgressMsg:     doc.ProgressMsg,
		ProcessBeginAt:  doc.ProcessBeginAt,
		ProcessDuration: doc.ProcessDuration,
		ContentHash:     doc.ContentHash,
		MetaFields:      metaFields,
		Suffix:          doc.Suffix,
		Run:             mapDocumentRunStatus(doc.Run),
		Status:          doc.Status,
		CreateTime:      doc.CreateTime,
		CreateDate:      doc.CreateDate,
		UpdateTime:      doc.UpdateTime,
		UpdateDate:      doc.UpdateDate,
	}
}

func mapDocumentRunStatus(run *string) string {
	if run == nil {
		return "UNSTART"
	}
	switch *run {
	case string(entity.TaskStatusRunning):
		return "RUNNING"
	case string(entity.TaskStatusCancel):
		return "CANCEL"
	case string(entity.TaskStatusDone):
		return "DONE"
	case string(entity.TaskStatusFail):
		return "FAIL"
	default:
		return "UNSTART"
	}
}

// UploadLocalDocuments stores each uploaded file in object storage and inserts a
// matching Document row into the dataset. It mirrors Python
// FileService.upload_document: it derives parser_id by filetype, merges the
// optional parser_config override into the dataset config, dedup-renames the
// filename, records size + xxhash content hash, and links each document into the
// file manager (a File row under the dataset folder + a file2document mapping)
// so it surfaces in the dataset's document list. Chunking/embedding happen later
// in the parse step, so nothing here touches the doc store index.
//
// Gaps vs Python (documented, not yet ported): thumbnail generation and
// read_potential_broken_pdf repair.
func (s *DocumentService) UploadLocalDocuments(kb *entity.Knowledgebase, tenantID string, files []*multipart.FileHeader, parentPath string, parserConfigOverride map[string]interface{}) ([]map[string]interface{}, []string) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, []string{"storage not initialized"}
	}

	// Resolve (and create if needed) the dataset's file-manager folder up front.
	// Without the File / file2document linkage the document list (which inner-joins
	// file2document + file) would never surface the uploaded files.
	kbFolder, err := s.ensureKBFolder(kb, tenantID)
	if err != nil {
		return nil, []string{err.Error()}
	}

	// Merge parser_config override (allow-listed keys only) over the dataset config.
	merged := entity.JSONMap{}
	for k, v := range kb.ParserConfig {
		merged[k] = v
	}
	for k, v := range parserConfigOverride {
		merged[k] = v
	}

	safeParent := utility.SanitizeFilename(parentPath)

	// Don't silently disable dedupe protection: a transient lookup failure means
	// the existing-name set is unknown, so fail rather than risk duplicates.
	names, err := s.documentDAO.ListNamesByKbID(kb.ID)
	if err != nil {
		return nil, []string{err.Error()}
	}
	taken := map[string]bool{}
	for _, n := range names {
		taken[n] = true
	}

	var results []map[string]interface{}
	var errMsgs []string

	for _, fh := range files {
		blob, err := readFileHeaderBytes(fh)
		if err != nil {
			errMsgs = append(errMsgs, fh.Filename+": "+err.Error())
			continue
		}

		filename := uniqueUploadName(fh.Filename, taken)

		filetype := utility.FilenameType(filename)
		if filetype == utility.FileTypeOTHER {
			errMsgs = append(errMsgs, fh.Filename+": This type of file has not been supported yet!")
			continue
		}

		location := filename
		if safeParent != "" {
			location = safeParent + "/" + filename
		}
		for storageImpl.ObjExist(kb.ID, location) {
			location += "_"
		}
		if err := storageImpl.Put(kb.ID, location, blob); err != nil {
			errMsgs = append(errMsgs, fh.Filename+": "+err.Error())
			continue
		}

		doc := s.newDatasetDocument(kb, tenantID, filename, location, string(filetype), merged, "local", int64(len(blob)), blob)
		if err := s.InsertDocument(doc); err != nil {
			// Roll back the orphaned blob so a failed insert doesn't leak storage.
			_ = storageImpl.Remove(kb.ID, location)
			errMsgs = append(errMsgs, fh.Filename+": "+err.Error())
			continue
		}
		if err := s.addFileFromKB(doc, kbFolder.ID, kb.TenantID); err != nil {
			// Linkage failed: roll back the document row and blob so the partial
			// state doesn't leave an invisible (unlisted) document behind.
			err = s.rollbackAddFileFromKBError(doc, kb.ID, err)
			_ = storageImpl.Remove(kb.ID, location)
			errMsgs = append(errMsgs, fh.Filename+": "+err.Error())
			continue
		}
		// Only reserve the name once the write fully succeeds.
		taken[filename] = true
		results = append(results, docToRawMap(doc))
	}

	return results, errMsgs
}

// UploadEmptyDocument inserts a zero-byte "virtual" document into the dataset.
func (s *DocumentService) UploadEmptyDocument(kb *entity.Knowledgebase, tenantID, name string) (map[string]interface{}, common.ErrorCode, error) {
	// A transient lookup failure means the existing-name set is unknown; fail
	// rather than write blind and risk a duplicate.
	names, err := s.documentDAO.ListNamesByKbID(kb.ID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	for _, n := range names {
		if n == name {
			return nil, common.CodeDataError, fmt.Errorf("Duplicated document name in the same dataset.")
		}
	}

	kbFolder, err := s.ensureKBFolder(kb, tenantID)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	doc := s.newDatasetDocument(kb, tenantID, name, "", "virtual", kb.ParserConfig, "local", 0, nil)
	if err := s.InsertDocument(doc); err != nil {
		return nil, common.CodeServerError, err
	}
	if err := s.addFileFromKB(doc, kbFolder.ID, kb.TenantID); err != nil {
		return nil, common.CodeServerError, s.rollbackAddFileFromKBError(doc, kb.ID, err)
	}
	return docToRawMap(doc), common.CodeSuccess, nil
}

// knowledgebaseFolderName is the file-manager folder under each tenant's root
// that holds per-dataset subfolders, mirroring Python KNOWLEDGEBASE_FOLDER_NAME.
const knowledgebaseFolderName = ".knowledgebase"

// ensureKBFolder resolves (creating as needed) the per-dataset file-manager
// folder: root -> .knowledgebase -> <dataset name>. Mirrors Python
// get_root_folder + get_kb_folder + new_a_file_from_kb.
func (s *DocumentService) ensureKBFolder(kb *entity.Knowledgebase, tenantID string) (*entity.File, error) {
	root, err := s.fileDAO.GetRootFolder(tenantID)
	if err != nil {
		return nil, err
	}
	kbRoot, err := s.newAFileFromKB(tenantID, knowledgebaseFolderName, root.ID)
	if err != nil {
		return nil, err
	}
	return s.newAFileFromKB(kb.TenantID, kb.Name, kbRoot.ID)
}

// newAFileFromKB returns the existing folder named name under parentID, or
// creates it. Mirrors Python FileService.new_a_file_from_kb.
func (s *DocumentService) newAFileFromKB(tenantID, name, parentID string) (*entity.File, error) {
	for _, f := range s.fileDAO.Query(name, parentID, tenantID) {
		if f.TenantID == tenantID {
			return f, nil
		}
	}
	loc := ""
	folder := &entity.File{
		ID:         utility.GenerateToken(),
		ParentID:   parentID,
		TenantID:   tenantID,
		CreatedBy:  tenantID,
		Name:       name,
		Type:       "folder",
		Size:       0,
		Location:   &loc,
		SourceType: string(entity.FileSourceKnowledgebase),
	}
	if err := s.fileDAO.Create(folder); err != nil {
		return nil, err
	}
	return folder, nil
}

// addFileFromKB links a document into the file manager: a File row under the
// dataset folder plus a file2document mapping. Mirrors Python
// FileService.add_file_from_kb (idempotent on the document mapping).
func (s *DocumentService) addFileFromKB(doc *entity.Document, kbFolderID, tenantID string) error {
	if existing, err := s.file2DocumentDAO.GetByDocumentID(doc.ID); err == nil && len(existing) > 0 {
		return nil
	}
	name := ""
	if doc.Name != nil {
		name = *doc.Name
	}
	loc := ""
	if doc.Location != nil {
		loc = *doc.Location
	}
	fileID := utility.GenerateToken()
	file := &entity.File{
		ID:         fileID,
		ParentID:   kbFolderID,
		TenantID:   tenantID,
		CreatedBy:  tenantID,
		Name:       name,
		Type:       doc.Type,
		Size:       doc.Size,
		Location:   &loc,
		SourceType: string(entity.FileSourceKnowledgebase),
	}
	if err := s.fileDAO.Create(file); err != nil {
		return err
	}
	docID := doc.ID
	if err := s.file2DocumentDAO.Create(&entity.File2Document{
		ID:         utility.GenerateToken(),
		FileID:     &fileID,
		DocumentID: &docID,
	}); err != nil {
		_ = s.fileDAO.Delete(fileID)
		return err
	}
	return nil
}

func (s *DocumentService) UploadWebDocument(kb *entity.Knowledgebase, tenantID, name, url string) (map[string]interface{}, common.ErrorCode, error) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, common.CodeServerError, fmt.Errorf("storage not initialized")
	}

	kbFolder, err := s.ensureKBFolder(kb, tenantID)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	names, err := s.documentDAO.ListNamesByKbID(kb.ID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	taken := map[string]bool{}
	for _, n := range names {
		taken[n] = true
	}

	blob, headers, _, err := fetchRemoteFileSafely(url, maxUploadDocSize)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	contentType := ""
	if headers != nil {
		contentType = headers.Get("Content-Type")
	}
	filename := normalizeWebDocumentName(name, contentType, blob)
	filename, _, blob = normalizeUploadInfoContent(filename, contentType, blob)
	filename = uniqueUploadName(filename, taken)

	filetype := utility.FilenameType(filename)
	if filetype == utility.FileTypeOTHER {
		return nil, common.CodeDataError, fmt.Errorf("This type of file has not been supported yet!")
	}

	location := filename
	for storageImpl.ObjExist(kb.ID, location) {
		location += "_"
	}
	if err := storageImpl.Put(kb.ID, location, blob); err != nil {
		return nil, common.CodeServerError, err
	}

	doc := s.newDatasetDocument(kb, tenantID, filename, location, string(filetype), kb.ParserConfig, "web", int64(len(blob)), blob)
	if err := s.InsertDocument(doc); err != nil {
		_ = storageImpl.Remove(kb.ID, location)
		return nil, common.CodeServerError, err
	}
	if err := s.addFileFromKB(doc, kbFolder.ID, kb.TenantID); err != nil {
		err = s.rollbackAddFileFromKBError(doc, kb.ID, err)
		_ = storageImpl.Remove(kb.ID, location)
		return nil, common.CodeServerError, err
	}
	return docToRawMap(doc), common.CodeSuccess, nil
}

func normalizeWebDocumentName(name, contentType string, blob []byte) string {
	filename := utility.SanitizeFilename(name)
	if filepath.Ext(filename) != "" {
		return filename
	}
	lowerCT := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch {
	case lowerCT == "application/pdf" || http.DetectContentType(blob) == "application/pdf" || bytesLooksLikePDF(blob):
		return filename + ".pdf"
	case lowerCT == "text/html" || lowerCT == "application/xhtml+xml" || looksLikeHTML(blob):
		return filename + ".html"
	default:
		return filename
	}
}

// newDatasetDocument builds a Document row for an upload, deriving parser_id,
// suffix and content hash. blob may be nil for the empty/virtual document.
func (s *DocumentService) newDatasetDocument(kb *entity.Knowledgebase, tenantID, filename, location, filetype string, parserConfig entity.JSONMap, src string, size int64, blob []byte) *entity.Document {
	docID := utility.GenerateToken()
	run := "0"
	status := "1"
	suffix := ""
	if i := strings.LastIndex(filename, "."); i >= 0 {
		suffix = filename[i+1:]
	}
	loc := location
	doc := &entity.Document{
		ID:           docID,
		KbID:         kb.ID,
		ParserID:     selectUploadParser(utility.FileType(filetype), filename, kb.ParserID),
		PipelineID:   kb.PipelineID,
		ParserConfig: parserConfig,
		CreatedBy:    tenantID,
		Type:         filetype,
		SourceType:   src,
		Name:         &filename,
		Location:     &loc,
		Size:         size,
		Suffix:       suffix,
		Run:          &run,
		Status:       &status,
	}
	if blob != nil {
		hash := contentHashHex(blob)
		doc.ContentHash = &hash
	}
	return doc
}

// docToRawMap serialises a freshly created Document into the raw key shape the
// handler remaps (chunk_num→chunk_count, kb_id→dataset_id, parser_id→chunk_method).
func docToRawMap(doc *entity.Document) map[string]interface{} {
	m := map[string]interface{}{
		"id":            doc.ID,
		"kb_id":         doc.KbID,
		"parser_id":     doc.ParserID,
		"parser_config": map[string]interface{}(doc.ParserConfig),
		"created_by":    doc.CreatedBy,
		"type":          doc.Type,
		"source_type":   doc.SourceType,
		"size":          doc.Size,
		"chunk_num":     doc.ChunkNum,
		"token_num":     doc.TokenNum,
		"suffix":        doc.Suffix,
		"run":           "0",
	}
	if doc.Name != nil {
		m["name"] = *doc.Name
	}
	if doc.Location != nil {
		m["location"] = *doc.Location
	}
	if doc.PipelineID != nil {
		m["pipeline_id"] = *doc.PipelineID
	}
	if doc.ContentHash != nil {
		m["content_hash"] = *doc.ContentHash
	}
	return m
}

// uniqueUploadName appends a numeric suffix until the name is free, mirroring
// Python duplicate_name.
func uniqueUploadName(name string, taken map[string]bool) string {
	if !taken[name] {
		return name
	}
	base, ext := name, ""
	if i := strings.LastIndex(name, "."); i >= 0 {
		base, ext = name[:i], name[i:]
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s(%d)%s", base, i, ext)
		if !taken[candidate] {
			return candidate
		}
	}
}

// maxUploadDocSize bounds a single uploaded file held in memory, mirroring the
// Python DOC_MAXIMUM_SIZE default (128 MiB; overridable there via MAX_CONTENT_LENGTH).
const maxUploadDocSize = 128 * 1024 * 1024

func readFileHeaderBytes(fh *multipart.FileHeader) ([]byte, error) {
	if fh.Size > maxUploadDocSize {
		return nil, fmt.Errorf("file exceeds the maximum allowed size of %d bytes", maxUploadDocSize)
	}
	src, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	blob, err := io.ReadAll(io.LimitReader(src, maxUploadDocSize+1))
	if err != nil {
		return nil, err
	}
	if len(blob) > maxUploadDocSize {
		return nil, fmt.Errorf("file exceeds the maximum allowed size of %d bytes", maxUploadDocSize)
	}
	return blob, nil
}

// MetadataUpdate is one update item: set key to value.
type DocumentMetadataUpdate struct {
	Key       string      `json:"key"`
	Value     interface{} `json:"value"`
	Match     interface{} `json:"match,omitempty"`
	ValueType string      `json:"valueType,omitempty"`
}

// MetadataDelete removes a whole key, or a specific value from a list field.
type DocumentMetadataDelete struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value,omitempty"`
}

// MetadataSelector selects which documents to target.
type DocumentMetadataSelector struct {
	DocumentIDs       []string               `json:"document_ids"`
	MetadataCondition map[string]interface{} `json:"metadata_condition"`
}

// BatchUpdateDocumentMetadatasResponse summarises the operation.
type BatchUpdateDocumentMetadatasResponse struct {
	Updated     int `json:"updated"`
	MatchedDocs int `json:"matched_docs"`
}

// BatchUpdateDocumentMetadatas implements the shared logic for
// PATCH /datasets/:dataset_id/documents/metadatas  and
// POST  /datasets/:dataset_id/metadata/update.
func (s *DocumentService) BatchUpdateDocumentMetadatas(
	datasetID string,
	selector *DocumentMetadataSelector,
	updates []DocumentMetadataUpdate,
	deletes []DocumentMetadataDelete,
) (*BatchUpdateDocumentMetadatasResponse, common.ErrorCode, error) {
	if selector == nil {
		selector = &DocumentMetadataSelector{}
	}
	if code, err := validateBatchUpdateDocumentMetadatasRequest(selector, updates, deletes); err != nil {
		return nil, code, err
	}

	// Resolve which document IDs to target.
	targetDocIDs := make(map[string]struct{})

	if len(selector.DocumentIDs) > 0 {
		// Validate that supplied IDs actually belong to this dataset.
		allRows, err := s.documentDAO.GetAllDocIDsByKBIDs([]string{datasetID})
		if err != nil {
			return nil, common.CodeServerError, fmt.Errorf("failed to list dataset documents: %w", err)
		}
		kbDocIDSet := make(map[string]struct{}, len(allRows))
		for _, row := range allRows {
			kbDocIDSet[row["id"]] = struct{}{}
		}
		var invalidIDs []string
		for _, id := range selector.DocumentIDs {
			if _, ok := kbDocIDSet[id]; !ok {
				invalidIDs = append(invalidIDs, id)
			}
		}
		if len(invalidIDs) > 0 {
			return nil, common.CodeDataError, fmt.Errorf("these documents do not belong to dataset %s: %s",
				datasetID, strings.Join(invalidIDs, ", "))
		}
		for _, id := range selector.DocumentIDs {
			targetDocIDs[id] = struct{}{}
		}
	}

	// Apply metadata_condition filter.
	if len(selector.MetadataCondition) > 0 {
		flattedMeta, err := s.metadataSvc.GetFlattedMetaByKBs([]string{datasetID})
		if err != nil {
			return nil, common.CodeServerError, fmt.Errorf("failed to get flattened metadata: %w", err)
		}

		// ParseAndConvert mirrors Python convert_conditions: conditions arrive as
		// {name, comparison_operator, value}, the operator is normalised, and the
		// (possibly non-string) value is preserved. MetaFilter then matches against
		// the common.MetaData returned by GetFlattedMetaByKBs.
		filterInput := common.ParseAndConvert(selector.MetadataCondition)
		filteredIDs := common.MetaFilter(flattedMeta, filterInput)

		filteredSet := make(map[string]struct{}, len(filteredIDs))
		for _, id := range filteredIDs {
			filteredSet[id] = struct{}{}
		}

		if len(targetDocIDs) > 0 {
			// Intersect with the document_ids restriction.
			for id := range targetDocIDs {
				if _, ok := filteredSet[id]; !ok {
					delete(targetDocIDs, id)
				}
			}
		} else {
			targetDocIDs = filteredSet
		}

		// Early-exit when conditions given but nothing matched.
		rawConds, _ := selector.MetadataCondition["conditions"]
		if rawConds != nil && len(targetDocIDs) == 0 {
			return &BatchUpdateDocumentMetadatasResponse{Updated: 0, MatchedDocs: 0}, common.CodeSuccess, nil
		}
	}

	ids := make([]string, 0, len(targetDocIDs))
	for id := range targetDocIDs {
		ids = append(ids, id)
	}

	// Apply updates and deletes per document using Python's batch_update_metadata
	// semantics instead of a simple merge-then-delete.
	updated := 0
	for _, docID := range ids {
		currentMeta, err := s.GetDocumentMetadataByID(docID)
		if err != nil {
			common.Warn("BatchUpdateDocumentMetadata: get metadata failed",
				zap.String("docID", docID), zap.Error(err))
			continue
		}

		meta := cloneDocumentMetadata(currentMeta)
		originalMeta := cloneDocumentMetadata(meta)

		changed := applyDocumentMetadataUpdates(meta, updates)
		if applyDocumentMetadataDeletes(meta, deletes) {
			changed = true
		}

		if !changed || reflect.DeepEqual(originalMeta, meta) {
			continue
		}

		if err := s.patchDocumentMetadata(docID, originalMeta, meta); err != nil {
			common.Warn("BatchUpdateDocumentMetadata: patch metadata failed",
				zap.String("docID", docID), zap.Error(err))
			continue
		}
		updated++
	}

	return &BatchUpdateDocumentMetadatasResponse{Updated: updated, MatchedDocs: len(ids)}, common.CodeSuccess, nil
}

func (s *DocumentService) UploadDocumentInfos(userID string, files []*multipart.FileHeader) ([]map[string]interface{}, common.ErrorCode, error) {
	fileSvc := &FileService{
		fileDAO:          s.fileDAO,
		file2DocumentDAO: s.file2DocumentDAO,
		documentService:  s,
	}
	data, err := fileSvc.UploadInfos(userID, files)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	return data, common.CodeSuccess, nil
}

func (s *DocumentService) UploadDocumentInfoByURL(userID, rawURL string) (map[string]interface{}, common.ErrorCode, error) {
	fileSvc := &FileService{
		fileDAO:          s.fileDAO,
		file2DocumentDAO: s.file2DocumentDAO,
		documentService:  s,
	}
	data, err := fileSvc.UploadFromURL(userID, rawURL)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	return data, common.CodeSuccess, nil
}

func validateBatchUpdateDocumentMetadatasRequest(
	selector *DocumentMetadataSelector,
	updates []DocumentMetadataUpdate,
	deletes []DocumentMetadataDelete,
) (common.ErrorCode, error) {
	for _, upd := range updates {
		if strings.TrimSpace(upd.Key) == "" || upd.Value == nil {
			return common.CodeDataError, errors.New("Each update requires key and value.")
		}
	}
	for _, del := range deletes {
		if strings.TrimSpace(del.Key) == "" {
			return common.CodeDataError, errors.New("Each delete requires key.")
		}
	}
	if selector != nil && selector.MetadataCondition != nil {
		if _, ok := selector.MetadataCondition["conditions"]; !ok && len(selector.MetadataCondition) > 0 {
			return common.CodeDataError, errors.New("metadata_condition must be an object.")
		}
	}
	return common.CodeSuccess, nil
}

func cloneDocumentMetadata(meta map[string]interface{}) map[string]interface{} {
	if meta == nil {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(meta))
	for k, v := range meta {
		cloned[k] = cloneDocumentMetadataValue(v)
	}
	return cloned
}

func cloneDocumentMetadataValue(v interface{}) interface{} {
	switch typed := v.(type) {
	case []interface{}:
		cp := make([]interface{}, len(typed))
		copy(cp, typed)
		return cp
	case []string:
		cp := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			cp = append(cp, item)
		}
		return cp
	default:
		return typed
	}
}

func applyDocumentMetadataUpdates(meta map[string]interface{}, updates []DocumentMetadataUpdate) bool {
	changed := false
	for _, upd := range updates {
		key := strings.TrimSpace(upd.Key)
		if key == "" {
			continue
		}
		normalizedValue := normalizeDocumentMetadataUpdateValue(upd.Value, upd.ValueType)
		matchProvided := upd.Match != nil && !(fmt.Sprintf("%v", upd.Match) == "")
		current, exists := meta[key]
		if !exists {
			if matchProvided {
				continue
			}
			if listVal, ok := toMetadataInterfaceSlice(normalizedValue); ok {
				meta[key] = dedupeDocumentMetadataList(listVal)
			} else {
				meta[key] = normalizedValue
			}
			changed = true
			continue
		}

		if curList, ok := toMetadataInterfaceSlice(current); ok {
			if !matchProvided {
				newList := append([]interface{}{}, curList...)
				if appendList, ok := toMetadataInterfaceSlice(normalizedValue); ok {
					newList = append(newList, appendList...)
				} else {
					newList = append(newList, normalizedValue)
				}
				newList = dedupeDocumentMetadataList(newList)
				if !reflect.DeepEqual(curList, newList) {
					meta[key] = newList
					changed = true
				}
				continue
			}

			replaced := false
			newList := make([]interface{}, 0, len(curList))
			for _, item := range curList {
				if documentMetadataValuesEqual(item, upd.Match) {
					if replacementList, ok := toMetadataInterfaceSlice(normalizedValue); ok {
						newList = append(newList, replacementList...)
					} else {
						newList = append(newList, normalizedValue)
					}
					replaced = true
				} else {
					newList = append(newList, item)
				}
			}
			newList = dedupeDocumentMetadataList(newList)
			if replaced && !reflect.DeepEqual(curList, newList) {
				meta[key] = newList
				changed = true
			}
			continue
		}

		if !matchProvided {
			if !reflect.DeepEqual(current, normalizedValue) {
				meta[key] = normalizedValue
				changed = true
			}
			continue
		}
		if documentMetadataValuesEqual(current, upd.Match) && !reflect.DeepEqual(current, normalizedValue) {
			meta[key] = normalizedValue
			changed = true
		}
	}
	return changed
}

func applyDocumentMetadataDeletes(meta map[string]interface{}, deletes []DocumentMetadataDelete) bool {
	changed := false
	for _, del := range deletes {
		key := strings.TrimSpace(del.Key)
		current, exists := meta[key]
		if key == "" || !exists {
			continue
		}

		if curList, ok := toMetadataInterfaceSlice(current); ok {
			if del.Value == nil {
				delete(meta, key)
				changed = true
				continue
			}
			newList := make([]interface{}, 0, len(curList))
			for _, item := range curList {
				if !documentMetadataValuesEqual(item, del.Value) {
					newList = append(newList, item)
				}
			}
			if len(newList) != len(curList) {
				if len(newList) == 0 {
					delete(meta, key)
				} else {
					meta[key] = newList
				}
				changed = true
			}
			continue
		}

		if del.Value == nil || documentMetadataValuesEqual(current, del.Value) {
			delete(meta, key)
			changed = true
		}
	}
	return changed
}

func toMetadataInterfaceSlice(v interface{}) ([]interface{}, bool) {
	switch typed := v.(type) {
	case []interface{}:
		cp := make([]interface{}, len(typed))
		copy(cp, typed)
		return cp, true
	case []string:
		cp := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			cp = append(cp, item)
		}
		return cp, true
	default:
		return nil, false
	}
}

func dedupeDocumentMetadataList(items []interface{}) []interface{} {
	result := make([]interface{}, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		key := fmt.Sprintf("%T:%v", item, item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func documentMetadataValuesEqual(a, b interface{}) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func normalizeDocumentMetadataUpdateValue(value interface{}, valueType string) interface{} {
	switch strings.ToLower(strings.TrimSpace(valueType)) {
	case "list":
		if list, ok := normalizeMetadataListValue(value); ok {
			return list
		}
		return []interface{}{}
	case "number":
		scalar, ok := firstScalarMetadataValue(value)
		if !ok {
			return value
		}
		switch typed := scalar.(type) {
		case float64, float32, int, int8, int16, int32, int64:
			return typed
		case json.Number:
			if i, err := typed.Int64(); err == nil {
				return i
			}
			if f, err := typed.Float64(); err == nil {
				return f
			}
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed == "" {
				return ""
			}
			if i, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
				return i
			}
			if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
				return f
			}
			return trimmed
		}
		return scalar
	case "string", "time":
		if scalar, ok := firstScalarMetadataValue(value); ok {
			return fmt.Sprintf("%v", scalar)
		}
		return ""
	default:
		return value
	}
}

func normalizeMetadataListValue(value interface{}) ([]interface{}, bool) {
	switch typed := value.(type) {
	case []interface{}:
		result := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			if nested, ok := normalizeMetadataListValue(item); ok {
				result = append(result, nested...)
				continue
			}
			if item != nil {
				result = append(result, item)
			}
		}
		return result, true
	case []string:
		result := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result, true
	default:
		return nil, false
	}
}

func firstScalarMetadataValue(value interface{}) (interface{}, bool) {
	if list, ok := normalizeMetadataListValue(value); ok {
		for _, item := range list {
			if item != nil {
				return item, true
			}
		}
		return nil, false
	}
	if value == nil {
		return nil, false
	}
	return value, true
}
