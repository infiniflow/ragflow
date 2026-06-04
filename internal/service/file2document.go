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
	"fmt"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// File2DocumentService handles linking files to datasets.
type File2DocumentService struct {
	fileDAO          *dao.FileDAO
	file2DocumentDAO *dao.File2DocumentDAO
	documentDAO      *dao.DocumentDAO
	kbDAO            *dao.KnowledgebaseDAO
	userTenantDAO    *dao.UserTenantDAO
}

// NewFile2DocumentService creates a File2DocumentService.
func NewFile2DocumentService() *File2DocumentService {
	return &File2DocumentService{
		fileDAO:          dao.NewFileDAO(),
		file2DocumentDAO: dao.NewFile2DocumentDAO(),
		documentDAO:      dao.NewDocumentDAO(),
		kbDAO:            dao.NewKnowledgebaseDAO(),
		userTenantDAO:    dao.NewUserTenantDAO(),
	}
}

// LinkToDatasetsRequest is the body for POST /files/link-to-datasets.
type LinkToDatasetsRequest struct {
	FileIDs []string `json:"file_ids" binding:"required"`
	KbIDs   []string `json:"kb_ids"   binding:"required"`
}

// LinkToDatasets validates inputs, expands folders, checks permissions, and
// schedules _convertFiles in a goroutine — mirroring Python convert().
// Returns immediately (fire-and-forget for the heavy DB work).
func (s *File2DocumentService) LinkToDatasets(userID string, req *LinkToDatasetsRequest) error {
	if len(req.FileIDs) == 0 {
		return fmt.Errorf("file_ids is required")
	}
	if len(req.KbIDs) == 0 {
		return fmt.Errorf("kb_ids is required")
	}

	// ── 1. Validate files exist ───────────────────────────────────────────────
	files, err := s.fileDAO.GetByIDs(req.FileIDs)
	if err != nil {
		return fmt.Errorf("failed to look up files: %w", err)
	}
	filesSet := make(map[string]*entity.File, len(files))
	for _, f := range files {
		filesSet[f.ID] = f
	}
	for _, id := range req.FileIDs {
		if filesSet[id] == nil {
			return fmt.Errorf("File not found!")
		}
	}

	// ── 2. Validate KBs exist ────────────────────────────────────────────────
	kbMap := make(map[string]*entity.Knowledgebase, len(req.KbIDs))
	for _, kbID := range req.KbIDs {
		kb, err := s.kbDAO.GetByID(kbID)
		if err != nil || kb == nil {
			return fmt.Errorf("Can't find this dataset!")
		}
		kbMap[kbID] = kb
	}

	// ── 3. Expand folders to leaf files ──────────────────────────────────────
	allFileIDs := make([]string, 0, len(req.FileIDs))
	for _, id := range req.FileIDs {
		file := filesSet[id]
		if file.Type == FileTypeFolder {
			inner, err := s.getAllInnermostFileIDs(id)
			if err != nil {
				return fmt.Errorf("failed to expand folder %s: %w", id, err)
			}
			allFileIDs = append(allFileIDs, inner...)
		} else {
			allFileIDs = append(allFileIDs, id)
		}
	}

	// ── 4. Validate expanded file permissions ─────────────────────────────────
	for _, id := range allFileIDs {
		file, err := s.fileDAO.GetByID(id)
		if err != nil || file == nil {
			return fmt.Errorf("File not found!")
		}
		if !s.checkFileTeamPermission(file, userID) {
			return fmt.Errorf("No authorization.")
		}
	}

	// ── 5. Validate KB permissions ────────────────────────────────────────────
	for _, kb := range kbMap {
		if !s.checkKBTeamPermission(kb, userID) {
			return fmt.Errorf("No authorization.")
		}
	}

	// ── 6. Run conversion in background (fire-and-forget) ────────────────────
	kbIDs := req.KbIDs
	go func() {
		if err := s.convertFiles(allFileIDs, kbIDs, userID); err != nil {
			common.Warn("file2document._convertFiles failed",
				zap.Strings("file_ids", allFileIDs),
				zap.Strings("kb_ids", kbIDs),
				zap.Error(err))
		}
	}()

	return nil
}

// convertFiles mirrors Python _convert_files: for each file, delete existing
// documents + mappings, then create a new document in each target KB.
func (s *File2DocumentService) convertFiles(fileIDs, kbIDs []string, userID string) error {
	for _, fileID := range fileIDs {
		// Delete existing mappings and their documents.
		mappings, err := s.file2DocumentDAO.GetByFileID(fileID)
		if err != nil {
			common.Warn("convertFiles: GetByFileID failed", zap.String("fileID", fileID), zap.Error(err))
		}
		for _, m := range mappings {
			if m.DocumentID == nil {
				continue
			}
			doc, err := s.documentDAO.GetByID(*m.DocumentID)
			if err != nil || doc == nil {
				continue
			}
			// Get tenant from KB.
			kb, err := s.kbDAO.GetByID(doc.KbID)
			if err != nil || kb == nil {
				continue
			}
			// Hard-delete document (ignoring chunk store cleanup for simplicity).
			if _, err := s.documentDAO.Delete(*m.DocumentID); err != nil {
				common.Warn("convertFiles: Delete document failed",
					zap.String("docID", *m.DocumentID), zap.Error(err))
			}
		}
		if err := s.file2DocumentDAO.DeleteByFileID(fileID); err != nil {
			common.Warn("convertFiles: DeleteByFileID failed", zap.String("fileID", fileID), zap.Error(err))
		}

		// Get source file.
		file, err := s.fileDAO.GetByID(fileID)
		if err != nil || file == nil {
			continue
		}

		// Create document + mapping in each target KB.
		for _, kbID := range kbIDs {
			kb, ok := func() (*entity.Knowledgebase, bool) {
				kb, err := s.kbDAO.GetByID(kbID)
				return kb, err == nil && kb != nil
			}()
			if !ok {
				continue
			}

			parserID := getParser(file.Type, file.Name, kb.ParserID)
			suffix := strings.TrimPrefix(filepath.Ext(file.Name), ".")
			doc := &entity.Document{
				ID:           common.GenerateUUID(),
				KbID:         kb.ID,
				ParserID:     parserID,
				ParserConfig: entity.JSONMap(kb.ParserConfig),
				CreatedBy:    userID,
				Type:         file.Type,
				Name:         &file.Name,
				Suffix:       suffix,
				Size:         file.Size,
			}
			if file.Location != nil {
				doc.Location = file.Location
			}
			if kb.PipelineID != nil {
				doc.PipelineID = kb.PipelineID
			}

			if err := s.documentDAO.Create(doc); err != nil {
				common.Warn("convertFiles: Create document failed",
					zap.String("kbID", kbID), zap.String("fileID", fileID), zap.Error(err))
				continue
			}

			mapping := &entity.File2Document{
				ID:         common.GenerateUUID(),
				FileID:     &fileID,
				DocumentID: &doc.ID,
			}
			if err := s.file2DocumentDAO.Create(mapping); err != nil {
				common.Warn("convertFiles: Create file2document mapping failed",
					zap.String("fileID", fileID), zap.String("docID", doc.ID), zap.Error(err))
			}
		}
	}
	return nil
}

// getAllInnermostFileIDs recursively collects all non-folder file IDs under a folder.
// Mirrors Python FileService.get_all_innermost_file_ids.
func (s *File2DocumentService) getAllInnermostFileIDs(folderID string) ([]string, error) {
	children, err := s.fileDAO.ListByParentID(folderID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, child := range children {
		if child.Type == FileTypeFolder {
			sub, err := s.getAllInnermostFileIDs(child.ID)
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

// checkFileTeamPermission mirrors Python check_file_team_permission:
// true when file.TenantID == userID or user is in the file tenant's team.
func (s *File2DocumentService) checkFileTeamPermission(file *entity.File, userID string) bool {
	if file.TenantID == userID {
		return true
	}
	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return false
	}
	for _, t := range tenants {
		if t.TenantID == file.TenantID {
			return true
		}
	}
	return false
}

// checkKBTeamPermission mirrors Python check_kb_team_permission:
// true when kb.TenantID == userID or user is in the KB tenant's team.
func (s *File2DocumentService) checkKBTeamPermission(kb *entity.Knowledgebase, userID string) bool {
	if kb.TenantID == userID {
		return true
	}
	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return false
	}
	for _, t := range tenants {
		if t.TenantID == kb.TenantID {
			return true
		}
	}
	return false
}

// getParser maps (fileType, fileName, kbParserID) → a parser ID.
// Mirrors Python FileService.get_parser — falls back to the KB's parser.
func getParser(fileType, fileName, kbParserID string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(fileName), "."))
	switch ext {
	case "pdf":
		return "pdf"
	case "doc", "docx":
		return "naive"
	case "ppt", "pptx":
		return "presentation"
	case "xls", "xlsx":
		return "table"
	case "txt", "md":
		return "naive"
	case "png", "jpg", "jpeg", "gif", "bmp", "webp":
		return "picture"
	}
	if kbParserID != "" {
		return kbParserID
	}
	return "naive"
}
