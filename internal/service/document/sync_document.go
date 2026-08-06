//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package document

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	"ragflow/internal/storage"
	"ragflow/internal/utility"

	"gorm.io/gorm"
)

var syncFilenameUnsafeRE = regexp.MustCompile(`[\\/:*?"<>|]+`)

// Upsert stores one normalized source document.
func (s *DocumentService) Upsert(ctx context.Context, input service.DocumentUpsertInput) (service.DocumentUpsertResult, error) {
	if input.DocumentID == "" {
		return service.DocumentUpsertResult{}, fmt.Errorf("document id is empty")
	}

	// get base info
	kb := &input.TaskContext.Knowledgebase
	tenantID := input.TaskContext.Connector.TenantID
	// return normalized filename
	filename := syncDocumentFilename(input.SourceDocument.SemanticIdentifier, input.SourceDocument.Extension, input.SourceDocument.SourceID)
	filetype := utility.FilenameType(filename)
	if filetype == utility.FileTypeOTHER {
		return service.DocumentUpsertResult{}, fmt.Errorf("document type %q is not supported", filename)
	}

	// get storage
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return service.DocumentUpsertResult{}, fmt.Errorf("storage not initialized")
	}

	// get content hash
	contentHash := input.SourceDocument.Fingerprint
	if contentHash == "" {
		contentHash = contentHashHex(input.SourceDocument.Blob)
	}

	// if the 'file' is existing
	existing, err := s.documentDAO.GetByID(ctx, dao.DB, input.DocumentID)
	if err != nil && !errorsIsRecordNotFound(err) {
		return service.DocumentUpsertResult{}, err
	}
	if existing != nil && existing.ContentHash != nil && *existing.ContentHash == contentHash {
		if err := s.ensureSyncDocumentPostWrite(ctx, input, existing); err != nil {
			return service.DocumentUpsertResult{}, err
		}
		return service.DocumentUpsertResult{DocID: input.DocumentID, Action: service.DocumentActionSkipped}, nil
	}

	// store location
	location := syncDocumentStagedLocation(input.SourceType, input.DocumentID, filename)
	cleanupLocation := true
	defer func() {
		if cleanupLocation {
			_ = storageImpl.Remove(context.WithoutCancel(ctx), kb.ID, location)
		}
	}()
	if err = storageImpl.Put(ctx, kb.ID, location, input.SourceDocument.Blob); err != nil {
		cleanupLocation = false
		return service.DocumentUpsertResult{}, err
	}

	// not existing, insert a new 'file'
	if existing == nil {
		result, insertErr := s.insertSyncDocument(ctx, input, filename, location, string(filetype), contentHash, tenantID)
		if insertErr == nil {
			cleanupLocation = false
		}
		return result, insertErr
	}
	// just update the file
	result, updateErr := s.updateSyncDocument(ctx, input, existing, filename, location, string(filetype), contentHash)
	if updateErr == nil {
		cleanupLocation = false
	}
	return result, updateErr
}

// insertSyncDocument inserts a new synced document and file-manager link.
func (s *DocumentService) insertSyncDocument(ctx context.Context, input service.DocumentUpsertInput, filename, location, filetype, contentHash, tenantID string) (service.DocumentUpsertResult, error) {
	// base info
	kb := &input.TaskContext.Knowledgebase
	kbFolder, err := s.ensureKBFolder(ctx, kb, tenantID)
	if err != nil {
		return service.DocumentUpsertResult{}, err
	}

	// create the 'doc'
	doc := s.newDatasetDocument(kb, tenantID, filename, "", filetype, copyJSONMap(kb.ParserConfig), input.SourceType, int64(len(input.SourceDocument.Blob)), input.SourceDocument.Blob)
	doc.ID = input.DocumentID

	// put 'file' in mysql `document`
	if err = s.InsertDocument(doc); err != nil {
		return service.DocumentUpsertResult{}, err
	}
	if err = s.publishSyncDocument(ctx, doc, location, contentHash); err != nil {
		return service.DocumentUpsertResult{}, s.rollbackAddFileFromKBError(ctx, doc, kb.ID, err)
	}
	// link 'file' 2 'file_system'
	if err = s.addFileFromKB(ctx, doc, kbFolder.ID, kb.TenantID); err != nil {
		return service.DocumentUpsertResult{}, s.rollbackAddFileFromKBError(ctx, doc, kb.ID, err)
	}
	// write metadata and do `auto_parse` (or not)
	if err = s.afterSyncDocumentUpsert(ctx, input, doc, false); err != nil {
		return service.DocumentUpsertResult{}, err
	}

	return service.DocumentUpsertResult{DocID: doc.ID, Action: service.DocumentActionAdded}, nil
}

// updateSyncDocument updates an existing synced document and file-manager row.
func (s *DocumentService) updateSyncDocument(ctx context.Context, input service.DocumentUpsertInput, doc *entity.Document, filename, location, filetype, contentHash string) (service.DocumentUpsertResult, error) {
	suffix := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	updates := map[string]any{
		"name":        filename,
		"size":        int64(len(input.SourceDocument.Blob)),
		"type":        filetype,
		"suffix":      suffix,
		"source_type": input.SourceType,
	}
	if err := s.documentDAO.UpdateByID(ctx, dao.DB, doc.ID, updates); err != nil {
		return service.DocumentUpsertResult{}, err
	}

	doc.Name = &filename
	doc.Size = int64(len(input.SourceDocument.Blob))
	doc.Type = filetype
	doc.Suffix = suffix
	doc.SourceType = input.SourceType

	if err := s.publishSyncDocument(ctx, doc, location, contentHash); err != nil {
		return service.DocumentUpsertResult{}, err
	}
	// update new updated file
	if err := s.updateSyncDocumentFile(ctx, input, doc); err != nil {
		return service.DocumentUpsertResult{}, err
	}
	// write metadata and do `auto_parse`
	if err := s.afterSyncDocumentUpsert(ctx, input, doc, true); err != nil {
		return service.DocumentUpsertResult{}, err
	}
	return service.DocumentUpsertResult{DocID: doc.ID, Action: service.DocumentActionUpdated}, nil
}

// publishSyncDocument publishes the staged object key after the document row exists.
func (s *DocumentService) publishSyncDocument(ctx context.Context, doc *entity.Document, location, contentHash string) error {
	if err := s.documentDAO.UpdateByID(ctx, dao.DB, doc.ID, map[string]interface{}{
		"location":     location,
		"content_hash": contentHash,
	}); err != nil {
		return err
	}
	doc.Location = &location
	doc.ContentHash = &contentHash
	return nil
}

// afterSyncDocumentUpsert writes metadata and optionally enqueues parsing.
func (s *DocumentService) afterSyncDocumentUpsert(ctx context.Context, input service.DocumentUpsertInput, doc *entity.Document, rerun bool) error {
	// write metadata
	if len(input.SourceDocument.Metadata) > 0 && s.docEngine != nil {
		if err := s.SetDocumentMetadata(ctx, doc.ID, input.SourceDocument.Metadata); err != nil {
			return err
		}
	}
	// do auto_parse if enabled
	if !input.AutoParse {
		return nil
	}
	return s.StartParseDocuments(ctx, doc, &input.TaskContext.Knowledgebase, input.TaskContext.Connector.TenantID, StartParseOptions{RerunWithDelete: rerun})
}

// ensureSyncDocumentPostWrite retries dependent work before an unchanged document is skipped.
func (s *DocumentService) ensureSyncDocumentPostWrite(ctx context.Context, input service.DocumentUpsertInput, doc *entity.Document) error {
	if err := s.updateSyncDocumentFile(ctx, input, doc); err != nil {
		return err
	}
	if len(input.SourceDocument.Metadata) > 0 && s.docEngine != nil {
		if err := s.SetDocumentMetadata(ctx, doc.ID, input.SourceDocument.Metadata); err != nil {
			return err
		}
	}
	if !input.AutoParse {
		return nil
	}
	task, err := s.ingestionTaskDAO.GetByDocumentID(ctx, dao.DB, doc.ID)
	if err != nil {
		return err
	}
	if task != nil {
		return nil
	}
	return s.StartParseDocuments(ctx, doc, &input.TaskContext.Knowledgebase, input.TaskContext.Connector.TenantID, StartParseOptions{})
}

// updateSyncDocumentFile updates the file-manager row linked to a synced document.
func (s *DocumentService) updateSyncDocumentFile(ctx context.Context, input service.DocumentUpsertInput, doc *entity.Document) error {
	mappings, err := s.file2DocumentDAO.GetByDocumentID(ctx, dao.DB, doc.ID)
	if err != nil {
		return err
	}

	// no updates
	if len(mappings) == 0 || mappings[0].FileID == nil {
		kbFolder, folderErr := s.ensureKBFolder(ctx, &input.TaskContext.Knowledgebase, input.TaskContext.Connector.TenantID)
		if folderErr != nil {
			return folderErr
		}
		return s.addFileFromKB(ctx, doc, kbFolder.ID, input.TaskContext.Knowledgebase.TenantID)
	}

	name := ""
	if doc.Name != nil {
		name = *doc.Name
	}

	location := ""
	if doc.Location != nil {
		location = *doc.Location
	}

	return s.fileDAO.UpdateByID(ctx, dao.DB, *mappings[0].FileID, map[string]interface{}{
		"name":     name,
		"location": location,
		"size":     doc.Size,
		"type":     doc.Type,
	})
}

// syncDocumentFilename returns a bounded filename for a synced source document.
func syncDocumentFilename(name, extension, fallback string) string {
	name = strings.TrimSpace(syncFilenameUnsafeRE.ReplaceAllString(name, "_"))
	if name == "" {
		name = strings.TrimSpace(syncFilenameUnsafeRE.ReplaceAllString(fallback, "_"))
	}
	if name == "" {
		name = "document"
	}

	extension = strings.TrimSpace(extension)
	if extension == "" {
		extension = ".txt"
	}

	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	if !strings.HasSuffix(strings.ToLower(name), strings.ToLower(extension)) {
		name += extension
	}

	if len(name) <= 255 {
		return name
	}

	ext := filepath.Ext(name)
	baseLimit := 255 - len(ext)
	if baseLimit < 1 {
		return name[:255]
	}

	return name[:baseLimit] + ext
}

// syncDocumentStagedLocation returns a unique object-storage key for unpublished synced content.
func syncDocumentStagedLocation(sourceType, docID, filename string) string {
	ext := filepath.Ext(filename)
	sourceType = strings.Trim(strings.ReplaceAll(sourceType, "/", "_"), "_")
	if sourceType == "" {
		sourceType = "sync"
	}
	return fmt.Sprintf("sync/%s/.staged/%s/%s%s", sourceType, utility.GenerateToken(), docID, ext)
}

// copyJSONMap returns a shallow copy of a JSON map.
func copyJSONMap(value entity.JSONMap) entity.JSONMap {
	out := entity.JSONMap{}
	for k, v := range value {
		out[k] = v
	}
	return out
}

// errorsIsRecordNotFound reports whether an error is GORM's not-found error.
func errorsIsRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
