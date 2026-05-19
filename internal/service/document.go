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
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"os"
	"os/exec"
	"path/filepath"
	"ragflow/internal/entity"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/storage"
	"ragflow/internal/utility"

	"ragflow/internal/server"

	"github.com/zeebo/xxh3"
)

// DocumentService document service
type DocumentService struct {
	documentDAO *dao.DocumentDAO
	kbDAO       *dao.KnowledgebaseDAO
	fileService *FileService
	docEngine   engine.DocEngine
	engineType  server.EngineType
	metadataSvc *MetadataService
}

// NewDocumentService create document service
func NewDocumentService() *DocumentService {
	cfg := server.GetConfig()
	return &DocumentService{
		documentDAO: dao.NewDocumentDAO(),
		kbDAO:       dao.NewKnowledgebaseDAO(),
		fileService: NewFileService(),
		docEngine:   engine.Get(),
		engineType:  cfg.DocEngine.Type,
		metadataSvc: NewMetadataService(),
	}
}

const datasetDocumentNameLimit = 255
const (
	maxBlobSizeThumbnail  = 50 * 1024 * 1024
	maxBlobSizePDF        = 100 * 1024 * 1024
	thumbnailBase64Prefix = "data:image/png;base64,"
)

var documentRunStatusMap = map[string]string{
	"0":       "UNSTART",
	"1":       "RUNNING",
	"2":       "CANCEL",
	"3":       "DONE",
	"4":       "FAIL",
	"UNSTART": "UNSTART",
	"RUNNING": "RUNNING",
	"CANCEL":  "CANCEL",
	"DONE":    "DONE",
	"FAIL":    "FAIL",
}

// UploadDatasetLocalDocuments uploads one or more local files into a dataset.
func (s *DocumentService) UploadDatasetLocalDocuments(datasetID, tenantID, parentPath string, fileHeaders []*multipart.FileHeader) ([]map[string]interface{}, common.ErrorCode, error) {
	kb, err := s.kbDAO.GetByID(datasetID)
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("can't find this dataset")
	}
	if !s.kbDAO.Accessible(datasetID, tenantID) {
		return nil, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
	}
	if len(fileHeaders) == 0 {
		return nil, common.CodeArgumentError, fmt.Errorf("No file selected!")
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, common.CodeServerError, fmt.Errorf("storage not initialized")
	}

	kbFolder, err := s.fileService.getOrCreateKnowledgebaseFolder(tenantID, kb.Name)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to get dataset folder: %w", err)
	}

	safeParentPath := strings.TrimSpace(utility.SanitizeFilename(parentPath))
	uploadedDocs := make([]map[string]interface{}, 0, len(fileHeaders))

	for _, fileHeader := range fileHeaders {
		if err := s.checkTenantUploadLimit(tenantID); err != nil {
			return nil, common.CodeServerError, err
		}
		if fileHeader == nil || fileHeader.Filename == "" {
			return nil, common.CodeArgumentError, fmt.Errorf("No file selected!")
		}
		if len([]byte(fileHeader.Filename)) > datasetDocumentNameLimit {
			return nil, common.CodeArgumentError, fmt.Errorf("File name must be %d bytes or less.", datasetDocumentNameLimit)
		}

		src, err := fileHeader.Open()
		if err != nil {
			return nil, common.CodeServerError, fmt.Errorf("failed to open uploaded file: %w", err)
		}
		data, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			return nil, common.CodeServerError, fmt.Errorf("failed to read file data: %w", err)
		}
		blob := repairPotentialBrokenPDF(fileHeader.Filename, data)
		thumbnail := generateThumbnail(fileHeader.Filename, blob)
		contentHash := computeContentHash(blob)

		filename := fileHeader.Filename
		uniqueName, err := s.duplicateDocumentName(datasetID, filename)
		if err != nil {
			return nil, common.CodeServerError, err
		}

		fileType := utility.FilenameType(uniqueName)
		if fileType == utility.FileTypeOTHER {
			return nil, common.CodeArgumentError, fmt.Errorf("This type of file has not been supported yet!")
		}

		location := uniqueName
		if safeParentPath != "" {
			location = safeParentPath + "/" + uniqueName
		}
		for storageImpl.ObjExist(datasetID, location) {
			location += "_"
		}
		if err := storageImpl.Put(datasetID, location, blob); err != nil {
			return nil, common.CodeServerError, fmt.Errorf("failed to store file: %w", err)
		}

		doc, err := s.insertDatasetDocument(kb, tenantID, uniqueName, location, blob, fileType, sourceTypeLocal, contentHash, thumbnail)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if err := s.fileService.fileDAO.AddFileFromKB(doc, kbFolder.ID, tenantID, s.fileService.file2DocumentDAO); err != nil {
			return nil, common.CodeServerError, err
		}

		uploadedDocs = append(uploadedDocs, mapDocumentForUpload(doc, "0"))
	}

	return uploadedDocs, common.CodeSuccess, nil
}

// UploadDatasetEmptyDocument creates an empty document in a dataset.
func (s *DocumentService) UploadDatasetEmptyDocument(datasetID, tenantID, name string) (map[string]interface{}, common.ErrorCode, error) {
	kb, err := s.kbDAO.GetByID(datasetID)
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("can't find this dataset")
	}
	if !s.kbDAO.Accessible(datasetID, tenantID) {
		return nil, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, common.CodeArgumentError, fmt.Errorf("File name can't be empty.")
	}
	if len([]byte(name)) > datasetDocumentNameLimit {
		return nil, common.CodeArgumentError, fmt.Errorf("File name must be %d bytes or less.", datasetDocumentNameLimit)
	}

	uniqueName, err := s.duplicateDocumentName(datasetID, name)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	kbFolder, err := s.fileService.getOrCreateKnowledgebaseFolder(tenantID, kb.Name)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to get dataset folder: %w", err)
	}

	doc, err := s.insertDatasetDocument(kb, tenantID, uniqueName, "", nil, "virtual", sourceTypeLocal, "", "")
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if err := s.fileService.fileDAO.AddFileFromKB(doc, kbFolder.ID, tenantID, s.fileService.file2DocumentDAO); err != nil {
		return nil, common.CodeServerError, err
	}

	return mapDocumentForUpload(doc, ""), common.CodeSuccess, nil
}

const sourceTypeLocal = "local"

func (s *DocumentService) duplicateDocumentName(datasetID, name string) (string, error) {
	return common.DuplicateName(func(candidate string, _ string) bool {
		docs, err := s.documentDAO.GetByNameAndKBID(candidate, datasetID)
		return err == nil && len(docs) > 0
	}, name, datasetID)
}

func (s *DocumentService) insertDatasetDocument(kb *entity.Knowledgebase, tenantID, name, location string, blob []byte, fileType, sourceType, contentHash, thumbnail string) (*entity.Document, error) {
	parserID := kb.ParserID
	switch {
	case fileType == utility.FileTypeVISUAL:
		parserID = "picture"
	case fileType == utility.FileTypeAURAL:
		parserID = "audio"
	case strings.HasSuffix(strings.ToLower(name), ".ppt") || strings.HasSuffix(strings.ToLower(name), ".pptx") || strings.HasSuffix(strings.ToLower(name), ".pages"):
		parserID = "presentation"
	case strings.HasSuffix(strings.ToLower(name), ".eml"):
		parserID = "email"
	}

	docName := name
	doc := &entity.Document{
		ID:           common.GenerateUUID(),
		KbID:         kb.ID,
		ParserID:     parserID,
		PipelineID:   kb.PipelineID,
		ParserConfig: entity.JSONMap(common.GetParserConfig(kb.ParserID, map[string]interface{}(kb.ParserConfig))),
		SourceType:   sourceType,
		Type:         fileType,
		CreatedBy:    tenantID,
		Name:         &docName,
		Location:     func() *string { loc := location; return &loc }(),
		Size:         int64(len(blob)),
		ContentHash:  stringPtrIfNotEmpty(contentHash),
		Thumbnail:    stringPtrIfNotEmpty(thumbnail),
		Suffix:       strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), "."),
		Run:          func() *string { s := "0"; return &s }(),
		Status:       func() *string { s := "1"; return &s }(),
	}

	if err := s.documentDAO.Create(doc); err != nil {
		return nil, fmt.Errorf("failed to create document: %w", err)
	}
	if err := s.kbDAO.AtomicIncreaseDocNumByID(kb.ID); err != nil {
		return nil, fmt.Errorf("failed to update dataset document count: %w", err)
	}
	return doc, nil
}

func mapDocumentForUpload(doc *entity.Document, runStatus string) map[string]interface{} {
	if doc == nil {
		return nil
	}

	raw, err := json.Marshal(doc)
	if err != nil {
		return map[string]interface{}{}
	}

	var mapped map[string]interface{}
	if err := json.Unmarshal(raw, &mapped); err != nil {
		return map[string]interface{}{}
	}

	if mapped == nil {
		mapped = make(map[string]interface{})
	}

	keyMapping := map[string]string{
		"chunk_num": "chunk_count",
		"kb_id":     "dataset_id",
		"token_num": "token_count",
		"parser_id": "chunk_method",
	}

	renamed := make(map[string]interface{}, len(mapped))
	for key, value := range mapped {
		if newKey, ok := keyMapping[key]; ok {
			renamed[newKey] = value
			continue
		}
		renamed[key] = value
	}

	status := runStatus
	if status == "" {
		if runValue, ok := renamed["run"].(string); ok {
			status = runValue
		}
	}
	if mappedStatus, ok := documentRunStatusMap[status]; ok {
		renamed["run"] = mappedStatus
	}

	return renamed
}

func (s *DocumentService) checkTenantUploadLimit(tenantID string) error {
	limitStr := strings.TrimSpace(os.Getenv("MAX_FILE_NUM_PER_USER"))
	if limitStr == "" {
		return nil
	}

	limit, err := strconv.ParseInt(limitStr, 10, 64)
	if err != nil || limit <= 0 {
		return nil
	}

	count, err := s.documentDAO.CountByTenantID(tenantID)
	if err != nil {
		return fmt.Errorf("failed to get document count: %w", err)
	}
	if count >= limit {
		return fmt.Errorf("Exceed the maximum file number of a free user!")
	}

	return nil
}

func computeContentHash(blob []byte) string {
	if len(blob) == 0 {
		return ""
	}

	sum := xxh3.Hash128(blob)
	return fmt.Sprintf("%x", sum.Bytes())
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func repairPotentialBrokenPDF(filename string, blob []byte) []byte {
	if len(blob) == 0 || len(blob) > maxBlobSizePDF {
		return blob
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		return blob
	}
	if bytes.HasPrefix(blob, []byte("%PDF-")) && bytes.Contains(blob, []byte("%%EOF")) {
		return blob
	}

	gsPath, err := exec.LookPath("gs")
	if err != nil {
		return blob
	}

	tempDir, err := os.MkdirTemp("", "ragflow-pdf-repair-*")
	if err != nil {
		return blob
	}
	defer os.RemoveAll(tempDir)

	inputPath := filepath.Join(tempDir, "input.pdf")
	outputPath := filepath.Join(tempDir, "output.pdf")
	if err := os.WriteFile(inputPath, blob, 0o600); err != nil {
		return blob
	}

	cmd := exec.Command(
		gsPath,
		"-o", outputPath,
		"-sDEVICE=pdfwrite",
		"-dPDFSETTINGS=/prepress",
		inputPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		_ = output
		return blob
	}

	repaired, err := os.ReadFile(outputPath)
	if err != nil || len(repaired) == 0 {
		return blob
	}
	return repaired
}

func generateThumbnail(filename string, blob []byte) string {
	if len(blob) == 0 || len(blob) > maxBlobSizeThumbnail {
		return ""
	}

	if strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		return generatePDFThumbnail(blob)
	}
	if isThumbnailImage(filename) {
		return generateImageThumbnail(blob)
	}

	return ""
}

func isThumbnailImage(filename string) bool {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"), strings.HasSuffix(lower, ".png"),
		strings.HasSuffix(lower, ".gif"), strings.HasSuffix(lower, ".ico"), strings.HasSuffix(lower, ".webp"):
		return true
	default:
		return false
	}
}

func generateImageThumbnail(blob []byte) string {
	img, _, err := image.Decode(bytes.NewReader(blob))
	if err != nil {
		return ""
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return ""
	}

	const maxDim = 30
	newWidth, newHeight := width, height
	if width > maxDim || height > maxDim {
		if width >= height {
			newWidth = maxDim
			newHeight = maxDim * height / width
		} else {
			newHeight = maxDim
			newWidth = maxDim * width / height
		}
		if newWidth <= 0 {
			newWidth = 1
		}
		if newHeight <= 0 {
			newHeight = 1
		}
	}

	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	for y := 0; y < newHeight; y++ {
		srcY := bounds.Min.Y + y*height/newHeight
		for x := 0; x < newWidth; x++ {
			srcX := bounds.Min.X + x*width/newWidth
			dst.Set(x, y, img.At(srcX, srcY))
		}
	}

	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return ""
	}
	return thumbnailBase64Prefix + base64.StdEncoding.EncodeToString(out.Bytes())
}

func generatePDFThumbnail(blob []byte) string {
	gsPath, err := exec.LookPath("gs")
	if err != nil {
		return ""
	}

	tempDir, err := os.MkdirTemp("", "ragflow-pdf-thumb-*")
	if err != nil {
		return ""
	}
	defer os.RemoveAll(tempDir)

	inputPath := filepath.Join(tempDir, "input.pdf")
	outputPath := filepath.Join(tempDir, "thumbnail.png")
	if err := os.WriteFile(inputPath, blob, 0o600); err != nil {
		return ""
	}

	cmd := exec.Command(
		gsPath,
		"-q",
		"-dBATCH",
		"-dNOPAUSE",
		"-dFirstPage=1",
		"-dLastPage=1",
		"-sDEVICE=png16m",
		"-r32",
		"-sOutputFile="+outputPath,
		inputPath,
	)
	if _, err := cmd.CombinedOutput(); err != nil {
		return ""
	}

	thumb, err := os.ReadFile(outputPath)
	if err != nil || len(thumb) == 0 {
		return ""
	}
	return thumbnailBase64Prefix + base64.StdEncoding.EncodeToString(thumb)
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

// DeleteDocument delete document
func (s *DocumentService) DeleteDocument(id string) error {
	return s.documentDAO.Delete(id)
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

// GetDocumentThumbnails returns thumbnail strings keyed by document ID.
func (s *DocumentService) GetDocumentThumbnails(docIDs []string) (map[string]string, error) {
	docs, err := s.documentDAO.GetByIDs(docIDs)
	if err != nil {
		return nil, err
	}

	thumbnails := make(map[string]string, len(docIDs))
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		if doc.Thumbnail != nil {
			thumbnails[doc.ID] = *doc.Thumbnail
		} else {
			thumbnails[doc.ID] = ""
		}
	}
	for _, docID := range docIDs {
		if _, ok := thumbnails[docID]; !ok {
			thumbnails[docID] = ""
		}
	}
	return thumbnails, nil
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

func (s *DocumentService) ParseDocuments(datasetID, userID string, docIDs []string) error {
	// create document parse id
	// save to task table
	// send to message queue
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
	return aggregateMetadata(searchResult.Chunks), nil
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
	if len(searchResult.Chunks) > 0 {
		chunk := searchResult.Chunks[0]
		return ExtractMetaFields(chunk)
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
	numChunks := len(searchResult.Chunks)

	var allMetaFields []map[string]interface{}
	if numChunks > 1 && len(searchResult.Chunks) > 0 {
		firstChunk := searchResult.Chunks[0]
		if metaFieldsVal := firstChunk["meta_fields"]; metaFieldsVal != nil {
			if v, ok := metaFieldsVal.([]byte); ok {
				allMetaFields = ParseAllLengthPrefixedJSON(v)
			}
		}
	}

	for idx, chunk := range searchResult.Chunks {
		docID, ok := ExtractDocumentID(chunk)
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
			metaFieldsVal = chunk["meta_fields"]
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
