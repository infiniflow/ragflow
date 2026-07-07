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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"sort"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// FileCommitService file commit service
type FileCommitService struct {
	commitDAO     *dao.FileCommitDAO
	commitItemDAO *dao.FileCommitItemDAO
	fileDAO       *dao.FileDAO
}

// NewFileCommitService create file commit service
func NewFileCommitService() *FileCommitService {
	return &FileCommitService{
		commitDAO:     dao.NewFileCommitDAO(),
		commitItemDAO: dao.NewFileCommitItemDAO(),
		fileDAO:       dao.NewFileDAO(),
	}
}

// CreateCommit creates a new commit for a workspace folder
func (s *FileCommitService) CreateCommit(folderID, authorID, message string, changes []entity.FileChange) (*entity.FileCommit, error) {
	// 1. Get the latest commit for this folder
	latestCommit, _ := s.commitDAO.GetLatestByFolderID(folderID)

	// 2. Build tree state from latest commit
	treeState := make(map[string]interface{})
	if latestCommit != nil && latestCommit.TreeState != nil {
		if err := json.Unmarshal([]byte(*latestCommit.TreeState), &treeState); err != nil {
			common.Warn("failed to unmarshal previous tree state", zap.Error(err))
			treeState = make(map[string]interface{})
		}
	}

	// 3. Create commit record
	commitID := utility.GenerateUUID()
	nowMs := time.Now().UnixMilli()

	commit := &entity.FileCommit{
		ID:        commitID,
		FolderID:  folderID,
		Message:   message,
		AuthorID:  authorID,
		FileCount: len(changes),
	}

	if latestCommit != nil {
		parentID := latestCommit.ID
		commit.ParentID = &parentID
	}

	// All DB operations run inside a single transaction.
	var treeStr string
	if err := dao.DB.Transaction(func(tx *gorm.DB) error {
		// Save commit
		if err := tx.Create(commit).Error; err != nil {
			return fmt.Errorf("failed to create commit: %w", err)
		}

		// Backfill parent_id for existing tree_state entries
		for fid, entry := range treeState {
			if m, ok := entry.(map[string]interface{}); ok {
				if _, has := m["parent_id"]; !has {
					var fileRec entity.File
					if err := tx.Select("parent_id").Where("id = ?", fid).First(&fileRec).Error; err == nil {
						m["parent_id"] = fileRec.ParentID
					}
				}
			}
		}

		storageImpl := storage.GetStorageFactory().GetStorage()

		for _, change := range changes {
			item := &entity.FileCommitItem{
				ID:        utility.GenerateUUID(),
				CommitID:  commitID,
				FileID:    change.FileID,
				Operation: change.Operation,
			}

			switch change.Operation {
			case "add", "modify":
				contentBytes := []byte(change.Content)
				hash := sha256.Sum256(contentBytes)
				hashHex := hex.EncodeToString(hash[:])
				objKey := ".objects/" + hashHex

				if storageImpl != nil {
					if err := storageImpl.Put(folderID, objKey, contentBytes); err != nil {
						return fmt.Errorf("failed to store object: %w", err)
					}
				}

				if change.Operation == "modify" {
					if oldEntry, ok := treeState[change.FileID]; ok {
						if oldMap, ok := oldEntry.(map[string]interface{}); ok {
							if oldHash, ok := oldMap["hash"].(string); ok {
								item.OldHash = &oldHash
							}
							if oldLoc, ok := oldMap["location"].(string); ok {
								item.OldLocation = &oldLoc
							}
						}
					}
				}

				item.NewHash = &hashHex
				item.NewLocation = &objKey

				fSize := int64(len(contentBytes))
				if err := tx.Model(&entity.File{}).Where("id = ?", change.FileID).Updates(map[string]interface{}{
					"location": objKey,
					"size":     fSize,
				}).Error; err != nil {
					return fmt.Errorf("failed to update file record: %w", err)
				}

				// Look up parent_id from the File table
				fileParentID := ""
				var fileRec entity.File
				if err := tx.Select("parent_id").Where("id = ?", change.FileID).First(&fileRec).Error; err == nil {
					fileParentID = fileRec.ParentID
				}

				treeState[change.FileID] = map[string]interface{}{
					"hash":      hashHex,
					"location":  objKey,
					"name":      change.FileName,
					"size":      fSize,
					"status":    "1",
					"parent_id": fileParentID,
				}

			case "delete":
				if oldEntry, ok := treeState[change.FileID]; ok {
					if oldMap, ok := oldEntry.(map[string]interface{}); ok {
						if oldHash, ok := oldMap["hash"].(string); ok {
							item.OldHash = &oldHash
						}
						if oldLoc, ok := oldMap["location"].(string); ok {
							item.OldLocation = &oldLoc
						}
					}
				}

				if err := tx.Model(&entity.File{}).Where("id = ?", change.FileID).Update("status", "0").Error; err != nil {
					return fmt.Errorf("failed to soft-delete file: %w", err)
				}

				if entry, ok := treeState[change.FileID]; ok {
					if entryMap, ok := entry.(map[string]interface{}); ok {
						entryMap["status"] = "0"
					}
				}

			case "rename":
				item.OldName = &change.OldName
				item.NewName = &change.NewName

				if err := tx.Model(&entity.File{}).Where("id = ?", change.FileID).Update("name", change.NewName).Error; err != nil {
					return fmt.Errorf("failed to rename file: %w", err)
				}

				if entry, ok := treeState[change.FileID]; ok {
					if entryMap, ok := entry.(map[string]interface{}); ok {
						entryMap["name"] = change.NewName
					}
				} else {
					treeState[change.FileID] = map[string]interface{}{
						"name":   change.NewName,
						"status": "1",
					}
				}
			}

			// Save commit item
			if err := tx.Create(item).Error; err != nil {
				return fmt.Errorf("failed to create commit item: %w", err)
			}
		}

		// Serialize and save tree state
		treeJSON, err := json.Marshal(treeState)
		if err != nil {
			return fmt.Errorf("failed to marshal tree state: %w", err)
		}
		if err := tx.Model(&entity.FileCommit{}).Where("id = ?", commitID).Update("tree_state", string(treeJSON)).Error; err != nil {
			return fmt.Errorf("failed to update tree state: %w", err)
		}
		treeStr = string(treeJSON)

		return nil
	}); err != nil {
		return nil, err
	}

	commit.TreeState = &treeStr
	commit.CreateTime = &nowMs

	return commit, nil
}

// ListCommits lists commits for a workspace folder with pagination
func (s *FileCommitService) ListCommits(folderID string, page, pageSize int, orderBy string, desc bool) ([]*entity.FileCommit, int64, error) {
	return s.commitDAO.ListByFolderID(folderID, page, pageSize, orderBy, desc)
}

// GetCommit gets a single commit by ID
func (s *FileCommitService) GetCommit(commitID string) (*entity.FileCommit, error) {
	return s.commitDAO.GetByID(commitID)
}

// ListCommitFiles lists all file change items for a commit
func (s *FileCommitService) ListCommitFiles(commitID string) ([]*entity.FileCommitItem, error) {
	return s.commitItemDAO.ListByCommitID(commitID)
}

// DiffCommits compares two commits and returns the diff
func (s *FileCommitService) DiffCommits(fromID, toID string) ([]entity.DiffEntry, error) {
	fromItems, err := s.commitItemDAO.ListByCommitID(fromID)
	if err != nil {
		return nil, err
	}
	toItems, err := s.commitItemDAO.ListByCommitID(toID)
	if err != nil {
		return nil, err
	}

	fromMap := make(map[string]*entity.FileCommitItem)
	for _, item := range fromItems {
		fromMap[item.FileID] = item
	}
	toMap := make(map[string]*entity.FileCommitItem)
	for _, item := range toItems {
		toMap[item.FileID] = item
	}

	// Get tree state for file names (use to commit)
	toCommit, err := s.commitDAO.GetByID(toID)
	treeState := make(map[string]interface{})
	if err == nil && toCommit != nil && toCommit.TreeState != nil {
		json.Unmarshal([]byte(*toCommit.TreeState), &treeState)
	}

	getFileName := func(fileID string) string {
		if entry, ok := treeState[fileID]; ok {
			if m, ok := entry.(map[string]interface{}); ok {
				if name, ok := m["name"].(string); ok {
					return name
				}
			}
		}
		return fileID
	}

	allFileIDs := make(map[string]bool)
	for fid := range fromMap {
		allFileIDs[fid] = true
	}
	for fid := range toMap {
		allFileIDs[fid] = true
	}

	// Sort for deterministic output
	var sortedIDs []string
	for fid := range allFileIDs {
		sortedIDs = append(sortedIDs, fid)
	}
	sort.Strings(sortedIDs)

	var diff []entity.DiffEntry
	for _, fid := range sortedIDs {
		fromItem := fromMap[fid]
		toItem := toMap[fid]

		var entry entity.DiffEntry
		entry.FileID = fid
		entry.FileName = getFileName(fid)

		if fromItem != nil && toItem == nil {
			// Deleted
			entry.Operation = "delete"
			entry.OldHash = fromItem.NewHash
			entry.OldLocation = fromItem.NewLocation
		} else if fromItem == nil && toItem != nil {
			// Added
			entry.Operation = "add"
			entry.NewHash = toItem.NewHash
			entry.NewLocation = toItem.NewLocation
		} else {
			// Both exist — compare hashes
			fromHash := ""
			if fromItem.NewHash != nil {
				fromHash = *fromItem.NewHash
			}
			toHash := ""
			if toItem.NewHash != nil {
				toHash = *toItem.NewHash
			}
			if fromHash != toHash {
				entry.Operation = "modify"
				entry.OldHash = fromItem.NewHash
				entry.OldLocation = fromItem.NewLocation
				entry.NewHash = toItem.NewHash
				entry.NewLocation = toItem.NewLocation
			}
		}

		if entry.Operation != "" {
			diff = append(diff, entry)
		}
	}

	return diff, nil
}

// GetUncommittedChanges gets uncommitted changes for a workspace folder.
// Recursively scans all sub-folders.
func (s *FileCommitService) GetUncommittedChanges(folderID string) ([]entity.DiffEntry, error) {
	// Get latest commit tree state
	latest, err := s.commitDAO.GetLatestByFolderID(folderID)
	committedFiles := make(map[string]map[string]interface{})
	if err == nil && latest != nil && latest.TreeState != nil {
		var treeData map[string]interface{}
		if jsonErr := json.Unmarshal([]byte(*latest.TreeState), &treeData); jsonErr == nil {
			for k, v := range treeData {
				if m, ok := v.(map[string]interface{}); ok {
					committedFiles[k] = m
				}
			}
		}
	}

	// Get all live files recursively under this folder
	liveMap := s.collectAllFilesRecursive(folderID)

	var changes []entity.DiffEntry
	processed := make(map[string]bool)

	// Check committed files for modifications and deletions
	for fid, committedEntry := range committedFiles {
		processed[fid] = true
		if committedEntry["status"] == "0" {
			continue
		}

		if liveFile, ok := liveMap[fid]; ok {
			liveHash := computeLiveFileHash(folderID, fid, liveFile)
			committedHash := ""
			if h, ok := committedEntry["hash"].(string); ok {
				committedHash = h
			}
			if liveHash != "" && liveHash != committedHash {
				changes = append(changes, entity.DiffEntry{
					FileID:    fid,
					FileName:  liveFile.Name,
					Operation: "modify",
				})
			}
		} else {
			name := ""
			if n, ok := committedEntry["name"].(string); ok {
				name = n
			}
			changes = append(changes, entity.DiffEntry{
				FileID:    fid,
				FileName:  name,
				Operation: "delete",
			})
		}
	}

	// Check for newly added files
	for _, liveFile := range liveMap {
		if !processed[liveFile.ID] {
			changes = append(changes, entity.DiffEntry{
				FileID:    liveFile.ID,
				FileName:  liveFile.Name,
				Operation: "add",
			})
		}
	}

	return changes, nil
}

// collectAllFilesRecursive recursively collects all non-folder files under a folder.
func (s *FileCommitService) collectAllFilesRecursive(folderID string) map[string]*entity.File {
	result := make(map[string]*entity.File)
	// Direct files (non-folder)
	files, _ := s.fileDAO.ListNonFolderByParentID(folderID)
	for _, f := range files {
		result[f.ID] = f
	}
	// Sub-folders — recurse
	subFolders, _ := s.fileDAO.ListFolderByParentID(folderID)
	for _, sf := range subFolders {
		sub := s.collectAllFilesRecursive(sf.ID)
		for k, v := range sub {
			result[k] = v
		}
	}
	return result
}

// GetCommitTree gets the tree state snapshot for a commit as a hierarchical tree.
func (s *FileCommitService) GetCommitTree(commitID string) (map[string]interface{}, error) {
	commit, err := s.commitDAO.GetByID(commitID)
	if err != nil {
		return nil, err
	}
	if commit.TreeState == nil {
		return map[string]interface{}{"id": commit.FolderID, "name": "", "type": "folder", "children": []interface{}{}}, nil
	}
	var flat map[string]interface{}
	if err := json.Unmarshal([]byte(*commit.TreeState), &flat); err != nil {
		return nil, err
	}
	return s.buildHierarchicalTree(flat, commit.FolderID), nil
}

// buildHierarchicalTree builds a recursive tree from a flat tree_state map.
// Sub-folder hierarchy is resolved from the File table's parent_id.
func (s *FileCommitService) buildHierarchicalTree(flat map[string]interface{}, rootFolderID string) map[string]interface{} {
	// Collect all unique folder IDs
	folderIDs := map[string]bool{rootFolderID: true}
	for _, v := range flat {
		if entry, ok := v.(map[string]interface{}); ok {
			pid, _ := entry["parent_id"].(string)
			if pid == "" {
				pid = rootFolderID
			}
			folderIDs[pid] = true
		}
	}

	// Build folder parent map from File table
	folderParentMap := make(map[string]string)
	for fid := range folderIDs {
		if fid != rootFolderID {
			if f, err := s.fileDAO.GetByID(fid); err == nil {
				folderParentMap[fid] = f.ParentID
			}
		}
	}

	// Group file entries by parent_id
	filesByParent := make(map[string][]string)
	fileEntries := make(map[string]map[string]interface{})
	for fid, v := range flat {
		entry, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		pid, _ := entry["parent_id"].(string)
		if pid == "" {
			pid = rootFolderID
		}
		filesByParent[pid] = append(filesByParent[pid], fid)
		fileEntries[fid] = entry
	}

	// Group sub-folders by their parent
	childrenByFolder := make(map[string][]string)
	for sfid, ppid := range folderParentMap {
		childrenByFolder[ppid] = append(childrenByFolder[ppid], sfid)
	}

	var buildNode func(nodeID string) map[string]interface{}
	buildNode = func(nodeID string) map[string]interface{} {
		nodeName := nodeID
		if f, err := s.fileDAO.GetByID(nodeID); err == nil {
			nodeName = f.Name
		}
		node := map[string]interface{}{
			"id":       nodeID,
			"name":     nodeName,
			"type":     "folder",
			"children": []interface{}{},
		}

		// File children
		for _, fid := range filesByParent[nodeID] {
			entry := fileEntries[fid]
			fn := map[string]interface{}{
				"id":     fid,
				"name":   entry["name"],
				"type":   "file",
				"hash":   entry["hash"],
				"size":   entry["size"],
				"status": entry["status"],
			}
			if loc, ok := entry["location"].(string); ok && loc != "" {
				fn["location"] = loc
			}
			node["children"] = append(node["children"].([]interface{}), fn)
		}
		// Sub-folder children
		for _, sfid := range childrenByFolder[nodeID] {
			child := buildNode(sfid)
			node["children"] = append(node["children"].([]interface{}), child)
		}
		return node
	}

	return buildNode(rootFolderID)
}

// GetCommitFileContent gets file content as it existed in a given commit
func (s *FileCommitService) GetCommitFileContent(folderID, commitID, fileID string) ([]byte, error) {
	_, err := s.commitDAO.GetByID(commitID)
	if err != nil {
		return nil, fmt.Errorf("commit not found: %w", err)
	}

	item, err := s.commitItemDAO.GetByCommitIDAndFileID(commitID, fileID)
	if err != nil {
		return nil, fmt.Errorf("file not found in commit: %w", err)
	}

	if item.NewHash == nil && item.OldHash == nil {
		return nil, fmt.Errorf("file has no content in this commit")
	}

	hash := ""
	if item.NewHash != nil {
		hash = *item.NewHash
	} else if item.OldHash != nil {
		hash = *item.OldHash
	}

	objKey := ".objects/" + hash

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	blob, err := storageImpl.Get(folderID, objKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content from storage: %w", err)
	}

	return blob, nil
}

// GetFileVersionHistory gets version history for a specific file
func (s *FileCommitService) GetFileVersionHistory(fileID string) ([]entity.VersionEntry, error) {
	items, err := s.commitItemDAO.ListByFileID(fileID)
	if err != nil {
		return nil, err
	}

	var versions []entity.VersionEntry
	for _, item := range items {
		commit, err := s.commitDAO.GetByID(item.CommitID)
		if err != nil {
			continue
		}

		h := ""
		if item.NewHash != nil {
			h = *item.NewHash
		} else if item.OldHash != nil {
			h = *item.OldHash
		}

		versions = append(versions, entity.VersionEntry{
			CommitID:   item.CommitID,
			Operation:  item.Operation,
			Hash:       h,
			CreateTime: commit.CreateTime,
			Message:    commit.Message,
		})
	}

	return versions, nil
}

// computeLiveFileHash computes the SHA256 hash of current file content from storage
func computeLiveFileHash(folderID, fileID string, file *entity.File) string {
	if file.Location == nil || *file.Location == "" {
		return ""
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return ""
	}

	data, err := storageImpl.Get(folderID, *file.Location)
	if err != nil {
		return ""
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
