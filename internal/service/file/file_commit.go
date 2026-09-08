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

package file

import (
	"context"
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
	"strings"
	"sync"
	"sync/atomic"
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
func (s *FileCommitService) CreateCommit(ctx context.Context, folderID, authorID, message string, changes []entity.FileChange) (*entity.FileCommit, error) {
	// 1. Get the latest commit for this folder
	latestCommit, _ := s.commitDAO.GetLatestByFolderID(ctx, dao.DB, folderID)

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
	if err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
					if err := storageImpl.Put(ctx, folderID, objKey, contentBytes); err != nil {
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
		if err = tx.Model(&entity.FileCommit{}).Where("id = ?", commitID).Update("tree_state", string(treeJSON)).Error; err != nil {
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

// PageEditCommitInput carries the data needed to record a single wiki/skill
// page edit as an audit commit.
type PageEditCommitInput struct {
	DatasetID  string // knowledgebase scope, stored on the commit for isolation
	DocID      string // ES doc id of the page content
	Slug       string
	PageType   string
	Title      string
	AuthorID   string
	OldContent string
	NewContent string
}

// wikiFileID derives the stable file key used to scope page-edit commits to a
// specific knowledgebase and page, so identical slugs in different
// knowledgebases never share a commit parent or history.
func wikiFileID(datasetID, pageType, slug string) string {
	return datasetID + "/" + pageType + "/" + slug
}

var pageCommitSeq atomic.Uint64

// RecordPageEdit records a wiki/skill page edit as an audit commit with a
// git-style parent chain (each edit points at the previous commit for the same
// page). The new content_after is referenced in ES by doc_id; a unified diff of
// old vs new content is stored on the commit item. The commit is scoped to the
// dataset via FolderID and a derived page file key so page histories never cross
// knowledgebase boundaries.
//
// This path is independent of the workspace File tree (it does not require a
// File record or a tree_state snapshot).
func (s *FileCommitService) RecordPageEdit(ctx context.Context, in PageEditCommitInput) (*entity.FileCommit, error) {
	// Parent chain: previous commit that touched the same page file key.
	fileID := wikiFileID(in.DatasetID, in.PageType, in.Slug)

	commitID := utility.GenerateUUID()

	diffText := unifiedDiff(in.OldContent, in.NewContent)
	contentAfterStorage := "es"
	contentAfterLocation := in.DocID
	slugKwd := in.Slug
	pageTypeKwd := in.PageType

	item := &entity.FileCommitItem{
		ID:                   utility.GenerateUUID(),
		CommitID:             commitID,
		FileID:               fileID,
		Operation:            "modify",
		Diff:                 &diffText,
		ContentAfterStorage:  &contentAfterStorage,
		ContentAfterLocation: &contentAfterLocation,
		SlugKwd:              &slugKwd,
		PageTypeKwd:          &pageTypeKwd,
	}

	var commit *entity.FileCommit
	// Serialize parent selection with insertion in process so two concurrent
	// edits on the same page cannot both read the same parent and fork the
	// chain. This is backend-agnostic (works identically on MySQL and SQLite)
	// and cheaper than row-level DB locks.
	mu := pageCommitLock(fileID)
	mu.Lock()
	defer mu.Unlock()

	item.Seq = uint(pageCommitSeq.Add(1))

	// Read the parent inside the lock, on the shared connection, so it always
	// reflects the previously committed edit for this page.
	parentID, perr := s.commitItemDAO.GetLatestCommitIDByFileID(ctx, dao.DB, fileID)
	if perr != nil {
		return nil, fmt.Errorf("failed to resolve page commit parent: %w", perr)
	}

	commit = &entity.FileCommit{
		ID:        commitID,
		FolderID:  in.DatasetID,
		Message:   in.Title,
		AuthorID:  in.AuthorID,
		Title:     &in.Title,
		FileCount: 1,
	}
	if parentID != "" {
		commit.ParentID = &parentID
	}

	if err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if cerr := tx.Create(commit).Error; cerr != nil {
			return fmt.Errorf("failed to create page commit: %w", cerr)
		}
		if ierr := tx.Create(item).Error; ierr != nil {
			return fmt.Errorf("failed to create page commit item: %w", ierr)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return commit, nil
}

// pageCommitLocks is a per-page-file-key mutex registry that serializes
// RecordPageEdit calls for the same page. Entries are retained for the process
// lifetime (bounded by the number of distinct pages edited).
var pageCommitLocks sync.Map // fileID -> *sync.Mutex

func pageCommitLock(fileID string) *sync.Mutex {
	v, _ := pageCommitLocks.LoadOrStore(fileID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// ListPageCommits lists audit commits for a specific wiki/skill page.
func (s *FileCommitService) ListPageCommits(ctx context.Context, datasetID, pageType, slug string, page, pageSize int) ([]*entity.FileCommit, int64, error) {
	items, err := s.commitItemDAO.ListByFileID(ctx, dao.DB, wikiFileID(datasetID, pageType, slug))
	if err != nil {
		return nil, 0, err
	}
	commitIDs := make([]string, 0, len(items))
	for _, it := range items {
		commitIDs = append(commitIDs, it.CommitID)
	}
	if len(commitIDs) == 0 {
		return []*entity.FileCommit{}, 0, nil
	}

	commits, total, err := s.commitDAO.ListByIDs(ctx, dao.DB, commitIDs, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	return commits, total, nil
}

// unifiedDiff produces a simple line-based unified diff between two texts.
func unifiedDiff(oldText, newText string) string {
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")

	const maxCtx = 3
	type hunkLine struct {
		prefix string
		text   string
	}
	var hunks []hunkLine

	// Longest common subsequence over lines, then render the diff.
	cur := make([][]int, len(oldLines)+1)
	for i := range cur {
		cur[i] = make([]int, len(newLines)+1)
	}
	for i := len(oldLines) - 1; i >= 0; i-- {
		for j := len(newLines) - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				cur[i][j] = cur[i+1][j+1] + 1
			} else if cur[i+1][j] >= cur[i][j+1] {
				cur[i][j] = cur[i+1][j]
			} else {
				cur[i][j] = cur[i][j+1]
			}
		}
	}

	i, j := 0, 0
	for i < len(oldLines) && j < len(newLines) {
		if oldLines[i] == newLines[j] {
			i++
			j++
		} else if cur[i+1][j] >= cur[i][j+1] {
			hunks = append(hunks, hunkLine{"-", oldLines[i]})
			i++
		} else {
			hunks = append(hunks, hunkLine{"+", newLines[j]})
			j++
		}
	}
	for ; i < len(oldLines); i++ {
		hunks = append(hunks, hunkLine{"-", oldLines[i]})
	}
	for ; j < len(newLines); j++ {
		hunks = append(hunks, hunkLine{"+", newLines[j]})
	}

	if len(hunks) == 0 {
		return ""
	}
	if len(hunks) > maxCtx*2 {
		var b strings.Builder
		for k := 0; k < maxCtx; k++ {
			b.WriteString(hunks[k].prefix)
			b.WriteString(hunks[k].text)
			b.WriteString("\n")
		}
		b.WriteString("... (" + fmt.Sprintf("%d", len(hunks)-maxCtx*2) + " lines omitted) ...\n")
		for k := len(hunks) - maxCtx; k < len(hunks); k++ {
			b.WriteString(hunks[k].prefix)
			b.WriteString(hunks[k].text)
			b.WriteString("\n")
		}
		return b.String()
	}
	var b strings.Builder
	for _, h := range hunks {
		b.WriteString(h.prefix)
		b.WriteString(h.text)
		b.WriteString("\n")
	}
	return b.String()
}

// ListCommits lists commits for a workspace folder with pagination
func (s *FileCommitService) ListCommits(ctx context.Context, folderID string, page, pageSize int, orderBy string, desc bool) ([]*entity.FileCommit, int64, error) {
	return s.commitDAO.ListByFolderID(ctx, dao.DB, folderID, page, pageSize, orderBy, desc)
}

// GetCommit gets a single commit by ID
func (s *FileCommitService) GetCommit(ctx context.Context, commitID string) (*entity.FileCommit, error) {
	return s.commitDAO.GetByID(ctx, dao.DB, commitID)
}

// ListCommitFiles lists all file change items for a commit
func (s *FileCommitService) ListCommitFiles(ctx context.Context, commitID string) ([]*entity.FileCommitItem, error) {
	return s.commitItemDAO.ListByCommitID(ctx, dao.DB, commitID)
}

// DiffCommits compares two commits and returns the diff
func (s *FileCommitService) DiffCommits(ctx context.Context, fromID, toID string) ([]entity.DiffEntry, error) {
	fromItems, err := s.commitItemDAO.ListByCommitID(ctx, dao.DB, fromID)
	if err != nil {
		return nil, err
	}
	toItems, err := s.commitItemDAO.ListByCommitID(ctx, dao.DB, toID)
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
	toCommit, err := s.commitDAO.GetByID(ctx, dao.DB, toID)
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
func (s *FileCommitService) GetUncommittedChanges(ctx context.Context, folderID string) ([]entity.DiffEntry, error) {
	// Get latest commit tree state
	latest, err := s.commitDAO.GetLatestByFolderID(ctx, dao.DB, folderID)
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
	liveMap := s.collectAllFilesRecursive(ctx, folderID)

	var changes []entity.DiffEntry
	processed := make(map[string]bool)

	// Check committed files for modifications and deletions
	for fid, committedEntry := range committedFiles {
		processed[fid] = true
		if committedEntry["status"] == "0" {
			continue
		}

		if liveFile, ok := liveMap[fid]; ok {
			liveHash := computeLiveFileHash(ctx, folderID, fid, liveFile)
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
func (s *FileCommitService) collectAllFilesRecursive(ctx context.Context, folderID string) map[string]*entity.File {
	result := make(map[string]*entity.File)
	// Direct files (non-folder)
	files, _ := s.fileDAO.ListNonFolderByParentID(ctx, dao.DB, folderID)
	for _, f := range files {
		result[f.ID] = f
	}
	// Sub-folders — recurse
	subFolders, _ := s.fileDAO.ListFolderByParentID(ctx, dao.DB, folderID)
	for _, sf := range subFolders {
		sub := s.collectAllFilesRecursive(ctx, sf.ID)
		for k, v := range sub {
			result[k] = v
		}
	}
	return result
}

// GetCommitTree gets the tree state snapshot for a commit as a hierarchical tree.
func (s *FileCommitService) GetCommitTree(ctx context.Context, commitID string) (map[string]interface{}, error) {
	commit, err := s.commitDAO.GetByID(ctx, dao.DB, commitID)
	if err != nil {
		return nil, err
	}
	if commit.TreeState == nil {
		return map[string]interface{}{"id": commit.FolderID, "name": "", "type": "folder", "children": []interface{}{}}, nil
	}
	var flat map[string]interface{}
	if err = json.Unmarshal([]byte(*commit.TreeState), &flat); err != nil {
		return nil, err
	}
	return s.buildHierarchicalTree(ctx, flat, commit.FolderID), nil
}

// buildHierarchicalTree builds a recursive tree from a flat tree_state map.
// Sub-folder hierarchy is resolved from the File table's parent_id.
func (s *FileCommitService) buildHierarchicalTree(ctx context.Context, flat map[string]interface{}, rootFolderID string) map[string]interface{} {
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
			if f, err := s.fileDAO.GetByID(ctx, dao.DB, fid); err == nil {
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
		if f, err := s.fileDAO.GetByID(ctx, dao.DB, nodeID); err == nil {
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
func (s *FileCommitService) GetCommitFileContent(ctx context.Context, folderID, commitID, fileID string) ([]byte, error) {
	_, err := s.commitDAO.GetByID(ctx, dao.DB, commitID)
	if err != nil {
		return nil, fmt.Errorf("commit not found: %w", err)
	}

	item, err := s.commitItemDAO.GetByCommitIDAndFileID(ctx, dao.DB, commitID, fileID)
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

	blob, err := storageImpl.Get(ctx, folderID, objKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content from storage: %w", err)
	}

	return blob, nil
}

// GetFileVersionHistory gets version history for a specific file
func (s *FileCommitService) GetFileVersionHistory(ctx context.Context, fileID string) ([]entity.VersionEntry, error) {
	items, err := s.commitItemDAO.ListByFileID(ctx, dao.DB, fileID)
	if err != nil {
		return nil, err
	}

	var versions []entity.VersionEntry
	for _, item := range items {
		var commit *entity.FileCommit
		commit, err = s.commitDAO.GetByID(ctx, dao.DB, item.CommitID)
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
func computeLiveFileHash(ctx context.Context, folderID, fileID string, file *entity.File) string {
	if file.Location == nil || *file.Location == "" {
		return ""
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return ""
	}

	data, err := storageImpl.Get(ctx, folderID, *file.Location)
	if err != nil {
		return ""
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
