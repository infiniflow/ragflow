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

package document

import (
	"errors"
	"ragflow/internal/service"
	"regexp"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
)

// DocumentService document service
type DocumentService struct {
	documentDAO         *dao.DocumentDAO
	kbDAO               *dao.KnowledgebaseDAO
	ingestionTaskDAO    *dao.IngestionTaskDAO
	ingestionTaskLogDAO *dao.IngestionTaskLogDAO
	ingestionTaskSvc    *service.IngestionTaskService
	docEngine           engine.DocEngine
	metadataSvc         *service.MetadataService
	taskDAO             *dao.TaskDAO
	file2DocumentDAO    *dao.File2DocumentDAO
	fileDAO             *dao.FileDAO
	canvasDAO           *dao.UserCanvasDAO
	api4ConvDAO         *dao.API4ConversationDAO
}

// NewDocumentService create document service
func NewDocumentService() *DocumentService {
	publisher := service.NewMessageQueueTaskPublisher()
	ingestionTaskSvc := service.NewIngestionTaskService()
	ingestionTaskSvc.SetTaskPublisher(publisher)
	return &DocumentService{
		documentDAO:         dao.NewDocumentDAO(),
		ingestionTaskDAO:    dao.NewIngestionTaskDAO(),
		ingestionTaskLogDAO: dao.NewIngestionTaskLogDAO(),
		ingestionTaskSvc:    ingestionTaskSvc,
		kbDAO:               dao.NewKnowledgebaseDAO(),
		docEngine:           engine.Get(),
		metadataSvc:         service.NewMetadataService(),
		taskDAO:             dao.NewTaskDAO(),
		file2DocumentDAO:    dao.NewFile2DocumentDAO(),
		fileDAO:             dao.NewFileDAO(),
		canvasDAO:           dao.NewUserCanvasDAO(),
		api4ConvDAO:         dao.NewAPI4ConversationDAO(),
	}
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
	ParserID        string                 `json:"parser_id"`
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

type DocumentPreview struct {
	Data        []byte
	ContentType string
	FileName    string
}

type DownloadDocumentResp struct {
	Data        []byte
	FileName    string
	ContentType string
}

type IngestDocumentRequest struct {
	DocIDs  []string    `json:"doc_ids" binding:"required"`
	Run     interface{} `json:"run" binding:"required"`
	Delete  bool        `json:"delete"`
	ApplyKB bool        `json:"apply_kb"`
}

// StartParseOptions controls StartParseDocuments behavior.
type StartParseOptions struct {
	// ApplyKB merges the knowledgebase's parser_config (llm_id, metadata)
	// into the document before parsing.
	ApplyKB bool
	// RerunWithDelete clears prior chunks/tasks/counters before re-parsing.
	RerunWithDelete bool
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

// valueInfo holds count and order of first appearance
type valueInfo struct {
	count      int
	firstOrder int
}

// knowledgebaseFolderName is the file-manager folder under each tenant's root
// that holds per-dataset subfolders, mirroring Python KNOWLEDGEBASE_FOLDER_NAME.
const knowledgebaseFolderName = ".knowledgebase"

// maxUploadDocSize bounds a single uploaded file held in memory, mirroring the
// Python DOC_MAXIMUM_SIZE default (128 MiB; overridable there via MAX_CONTENT_LENGTH).
const maxUploadDocSize = 128 * 1024 * 1024

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
