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
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
	"ragflow/internal/service"
	"ragflow/internal/utility"
)

// Sentinel errors returned by File2DocumentService. Handlers map these to
// Python-compatible response codes/messages. Returning sentinels (instead of
// wrapped DAO/runtime errors) prevents internal DB details from leaking through
// the API response path.
var (
	// ErrLinkFileNotFound mirrors Python "File not found!".
	ErrLinkFileNotFound = errors.New("File not found!")
	// ErrLinkDatasetNotFound mirrors Python "Can't find this dataset!".
	ErrLinkDatasetNotFound = errors.New("Can't find this dataset!")
	// ErrLinkInternal is a generic, safe-to-expose internal failure.
	ErrLinkInternal = errors.New("Internal server error.")
)

// convertTimeout caps one background conversion. It is a ceiling against stuck
// backends, not an expected duration, and is generous so large folder links are
// not silently truncated after the HTTP response has already returned.
const convertTimeout = 30 * time.Minute

// File2DocumentService handles linking files to datasets.
type File2DocumentService struct {
	fileDAO          *dao.FileDAO
	file2DocumentDAO *dao.File2DocumentDAO
	kbDAO            *dao.KnowledgebaseDAO
	userTenantDAO    *dao.UserTenantDAO
	documentSvc      *DocumentService
	documentDAO      *dao.DocumentDAO
	convertFilesFn   func(ctx context.Context, fileIDs, kbIDs []string, userID, mode string) error
}

// NewFile2DocumentService creates a File2DocumentService.
func NewFile2DocumentService() *File2DocumentService {
	return &File2DocumentService{
		fileDAO:          dao.NewFileDAO(),
		file2DocumentDAO: dao.NewFile2DocumentDAO(),
		kbDAO:            dao.NewKnowledgebaseDAO(),
		userTenantDAO:    dao.NewUserTenantDAO(),
		documentSvc:      NewDocumentService(),
		documentDAO:      dao.NewDocumentDAO(),
	}
}

// LinkToDatasetsRequest is the body for POST /files/link-to-datasets.
type LinkToDatasetsRequest struct {
	FileIDs []string `json:"file_ids"`
	KbIDs   []string `json:"kb_ids"`
}

// LinkToDatasets validates inputs, expands folders, checks permissions, and
// schedules convertFiles in a goroutine — mirroring Python convert().
//
// On validation failure it returns a sentinel error (see ErrLink* above) so the
// handler can map it to a Python-compatible response without leaking internals.
func (s *File2DocumentService) LinkToDatasets(ctx context.Context, userID string, req *LinkToDatasetsRequest, mode string) error {
	// ── 1. Validate files exist ───────────────────────────────────────────────
	files, err := s.fileDAO.GetByIDs(ctx, dao.DB, req.FileIDs)
	if err != nil {
		common.Warn("LinkToDatasets: GetByIDs failed", zap.Error(err))
		return ErrLinkInternal
	}
	filesSet := make(map[string]*entity.File, len(files))
	for _, f := range files {
		filesSet[f.ID] = f
	}
	for _, id := range req.FileIDs {
		if filesSet[id] == nil {
			return ErrLinkFileNotFound
		}
	}

	// ── 2. Validate KBs exist ────────────────────────────────────────────────
	// Clone so the background goroutine never shares a backing array with the
	// caller's request.
	kbIDs := slices.Clone(req.KbIDs)
	kbMap := make(map[string]*entity.Knowledgebase, len(kbIDs))
	for _, kbID := range kbIDs {
		kb, err := s.kbDAO.GetByID(ctx, dao.DB, kbID)
		if err != nil || kb == nil {
			return ErrLinkDatasetNotFound
		}
		kbMap[kbID] = kb
	}

	// ── 3. Expand folders to leaf files, then deduplicate ─────────────────────
	checker := permission.NewDatabaseChecker(dao.DB)
	checkFileRead := func(file *entity.File) error {
		kind := permission.ResourceKindFile
		if file.Type == "folder" {
			kind = permission.ResourceKindFolder
		}
		return checker.CheckResource(ctx, permission.Subject{UserID: userID}, permission.ResourceRef{
			Kind: kind,
			ID:   file.ID,
		}, permission.OperationRead)
	}
	// Mixed folder + direct file inputs (or overlapping folders) can yield the
	// same leaf file more than once; dedupe so each file is converted exactly
	// once.
	expanded := make([]string, 0, len(req.FileIDs))
	for _, id := range req.FileIDs {
		file := filesSet[id]
		if file.Type == "folder" {
			if err := checkFileRead(file); err != nil {
				_, permissionErr := permissionresponse.Normalize(err)
				return permissionErr
			}
			inner, err := s.getAllInnermostFileIDs(ctx, id)
			if err != nil {
				common.Warn("LinkToDatasets: folder expansion failed", zap.String("fileID", id), zap.Error(err))
				return ErrLinkInternal
			}
			expanded = append(expanded, inner...)
		} else {
			expanded = append(expanded, id)
		}
	}
	allFileIDs := dedupeStrings(expanded)

	// ── 4. Validate expanded file permissions ─────────────────────────────────
	for _, id := range allFileIDs {
		file, err := s.fileDAO.GetByID(ctx, dao.DB, id)
		if err != nil || file == nil {
			return ErrLinkFileNotFound
		}
		if err := checkFileRead(file); err != nil {
			_, permissionErr := permissionresponse.Normalize(err)
			return permissionErr
		}
	}

	// ── 5. Validate KB permissions ────────────────────────────────────────────
	for _, kb := range kbMap {
		if err := service.CheckDatasetAccess(ctx, permission.Subject{UserID: userID}, kb.ID, permission.OperationUpdate); err != nil {
			_, permissionErr := permissionresponse.Normalize(err)
			return permissionErr
		}
	}

	// ── 6. Run conversion in background (fire-and-forget) ────────────────────
	go s.runConversion(ctx, allFileIDs, kbIDs, userID, mode)

	return nil
}

// runConversion runs convertFiles detached from the request lifecycle. It is
// synchronous so callers (and tests) decide how to schedule it.
func (s *File2DocumentService) runConversion(ctx context.Context, fileIDs, kbIDs []string, userID, mode string) {
	defer func() {
		if r := recover(); r != nil {
			common.Error("file2document: convertFiles panic", fmt.Errorf("panic: %v", r),
				zap.Strings("file_ids", fileIDs),
				zap.Strings("kb_ids", kbIDs),
				zap.String("stack", string(debug.Stack())),
			)
		}
	}()

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), convertTimeout)
	defer cancel()

	convert := s.convertFiles
	if s.convertFilesFn != nil {
		convert = s.convertFilesFn
	}
	if err := convert(ctx, fileIDs, kbIDs, userID, mode); err != nil {
		common.Warn("file2document.convertFiles failed",
			zap.Strings("file_ids", fileIDs),
			zap.Strings("kb_ids", kbIDs),
			zap.Error(err))
	}
}

// convertFiles mirrors Python _convert_files: keep existing links, remove links
// outside the requested KBs in replace mode, then add any missing links.
// Failures on one file do not stop the others; all errors are returned joined.
func (s *File2DocumentService) convertFiles(ctx context.Context, fileIDs, kbIDs []string, userID, mode string) error {
	replaceExisting := mode != "add"
	kbIDs = dedupeStrings(kbIDs)
	requestedKBIDs := make(map[string]struct{}, len(kbIDs))
	for _, kbID := range kbIDs {
		requestedKBIDs[kbID] = struct{}{}
	}

	var errs []error
	for _, fileID := range fileIDs {
		if ctx.Err() != nil {
			break
		}
		if err := s.convertFile(ctx, fileID, kbIDs, requestedKBIDs, userID, replaceExisting); err != nil {
			errs = append(errs, err)
		}
	}
	if err := ctx.Err(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// convertFile links one file to every requested KB. A failure while inspecting
// or removing its existing links aborts the file, so a later retry cannot
// create duplicates or leave a half-replaced state.
func (s *File2DocumentService) convertFile(ctx context.Context, fileID string, kbIDs []string, requestedKBIDs map[string]struct{}, userID string, replaceExisting bool) error {
	mappings, err := s.file2DocumentDAO.GetByFileID(ctx, dao.DB, fileID)
	if err != nil {
		return fmt.Errorf("get mappings for file %s: %w", fileID, err)
	}

	var errs []error
	existingKBIDs := make(map[string]struct{})
	for _, m := range mappings {
		if m.DocumentID == nil {
			continue
		}
		doc, err := s.documentDAO.GetByID(ctx, dao.DB, *m.DocumentID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// The mapping points to a deleted document; drop it so it does not
			// accumulate.
			if delErr := s.file2DocumentDAO.DeleteByDocumentID(ctx, dao.DB, *m.DocumentID); delErr != nil {
				errs = append(errs, fmt.Errorf("delete dangling mapping for doc %s: %w", *m.DocumentID, delErr))
			}
			continue
		}
		if err != nil {
			return errors.Join(append(errs, fmt.Errorf("get doc %s for file %s: %w", *m.DocumentID, fileID, err))...)
		}

		existingKBIDs[doc.KbID] = struct{}{}
		if !replaceExisting {
			continue
		}
		if _, keep := requestedKBIDs[doc.KbID]; keep {
			continue
		}
		if err := s.documentSvc.RemoveDocumentKeepFile(ctx, doc.ID); err != nil {
			return errors.Join(append(errs, fmt.Errorf("remove doc %s: %w", doc.ID, err))...)
		}
		// The document is gone at this point, so a leftover mapping is dangling
		// and gets cleaned up by the next run; no need to abort the file.
		if err := s.file2DocumentDAO.DeleteByDocumentID(ctx, dao.DB, doc.ID); err != nil {
			errs = append(errs, fmt.Errorf("delete mapping for doc %s: %w", doc.ID, err))
		}
	}

	file, err := s.fileDAO.GetByID(ctx, dao.DB, fileID)
	if err != nil {
		return errors.Join(append(errs, fmt.Errorf("get file %s: %w", fileID, err))...)
	}

	for _, kbID := range kbIDs {
		if ctx.Err() != nil {
			break
		}
		if _, exists := existingKBIDs[kbID]; exists {
			continue
		}
		if err := s.linkFileToKB(ctx, file, kbID, userID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// linkFileToKB creates a document in the KB for the file and records the
// file2document mapping. If the mapping cannot be created, the just-inserted
// document is rolled back so no orphan is left behind.
func (s *File2DocumentService) linkFileToKB(ctx context.Context, file *entity.File, kbID, userID string) error {
	kb, err := s.kbDAO.GetByID(ctx, dao.DB, kbID)
	if err != nil {
		return fmt.Errorf("get kb %s: %w", kbID, err)
	}

	// Mirror Python duplicate_name: generate a non-colliding document
	// name within the dataset so that linking does not silently create
	// duplicate-name documents in the same KB.
	docName, err := s.documentSvc.UniqueDocumentName(ctx, kbID, file.Name)
	if err != nil {
		return fmt.Errorf("unique name for file %s in kb %s: %w", file.ID, kbID, err)
	}

	selection, parserConfig := resolveDocumentParser(ctx, kb, docName, utility.FileType(file.Type), cloneParserConfigForDocument(kb.ParserConfig))
	parserID, pipelineID := parserSelectionColumns(selection)
	suffix := strings.TrimPrefix(filepath.Ext(docName), ".")
	doc := &entity.Document{
		ID:           common.GenerateUUID(),
		KbID:         kb.ID,
		ParserID:     parserID,
		PipelineID:   pipelineID,
		ParserConfig: parserConfig,
		CreatedBy:    userID,
		Type:         file.Type,
		Name:         &docName,
		Suffix:       suffix,
		Size:         file.Size,
	}
	if file.Location != nil {
		doc.Location = file.Location
	}

	// InsertDocument creates the row and increments KB doc_num in one
	// transaction, so a failed insert never leaves a stale counter.
	if err := s.documentSvc.InsertDocument(ctx, doc); err != nil {
		return fmt.Errorf("insert doc %s: %w", doc.ID, err)
	}

	mapping := &entity.File2Document{
		ID:         common.GenerateUUID(),
		FileID:     &file.ID,
		DocumentID: &doc.ID,
	}
	if err := s.file2DocumentDAO.Create(ctx, dao.DB, mapping); err != nil {
		createErr := fmt.Errorf("create mapping for file %s doc %s: %w", file.ID, doc.ID, err)
		// ctx may be the very reason the mapping failed (deadline), so the
		// rollback runs on its own bounded context.
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupBatchTimeout)
		defer cancel()
		if rbErr := s.documentSvc.RemoveDocumentKeepFile(rollbackCtx, doc.ID); rbErr != nil {
			return errors.Join(createErr, fmt.Errorf("rollback doc %s: %w", doc.ID, rbErr))
		}
		return createErr
	}
	return nil
}

// getAllInnermostFileIDs recursively collects all non-folder file IDs under a folder.
// Mirrors Python FileService.get_all_innermost_file_ids.
func (s *File2DocumentService) getAllInnermostFileIDs(ctx context.Context, folderID string) ([]string, error) {
	children, err := s.fileDAO.ListByParentID(ctx, dao.DB, folderID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, child := range children {
		if child.Type == "folder" {
			sub, err := s.getAllInnermostFileIDs(ctx, child.ID)
			if err != nil {
				return nil, err
			}
			ids = append(ids, sub...)
		} else {
			ids = append(ids, child.ID)
		}
	}
	return ids, nil
}

// dedupeStrings returns the input slice with duplicates removed, preserving the
// first-seen order.
func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
