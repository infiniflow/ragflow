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
	"fmt"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"strconv"

	"github.com/gin-gonic/gin"
)

// fileCommitService is the consumer-side interface for FileCommitHandler's service dependency.
type fileCommitService interface {
	CreateCommit(folderID, authorID, message string, changes []entity.FileChange) (*entity.FileCommit, error)
	ListCommits(folderID string, page, pageSize int, orderBy string, desc bool) ([]*entity.FileCommit, int64, error)
	GetCommit(commitID string) (*entity.FileCommit, error)
	ListCommitFiles(commitID string) ([]*entity.FileCommitItem, error)
	DiffCommits(fromID, toID string) ([]entity.DiffEntry, error)
	GetUncommittedChanges(folderID string) ([]entity.DiffEntry, error)
	GetCommitTree(commitID string) (map[string]interface{}, error)
	GetCommitFileContent(folderID, commitID, fileID string) ([]byte, error)
	GetFileVersionHistory(fileID string) ([]entity.VersionEntry, error)
}

// FileCommitHandler file commit handler
type FileCommitHandler struct {
	commitService fileCommitService
	kbDAO         *dao.KnowledgebaseDAO
	fileDAO       *dao.FileDAO
}

// NewFileCommitHandler create file commit handler
func NewFileCommitHandler(commitService fileCommitService) *FileCommitHandler {
	return &FileCommitHandler{
		commitService: commitService,
		kbDAO:         dao.NewKnowledgebaseDAO(),
		fileDAO:       dao.NewFileDAO(),
	}
}

// ResolveFolderID resolves a resource ID (dataset/memory/skill) to its folder_id.
// entityType is the plural resource name (e.g. "datasets", "memories", "skills").
func (h *FileCommitHandler) ResolveFolderID(entityType, entityID string) (string, error) {
	switch entityType {
	case "datasets":
		return h.resolveDatasetFolderID(entityID)
	default:
		return "", fmt.Errorf("unsupported entity type: %s", entityType)
	}
}

// CommitFolderResolver returns a Gin middleware that resolves an entity ID
// (e.g. dataset_id, memory_id, skill_id) to a folder_id and injects it into
// c.Params so that the standard commit handlers can read it via c.Param("folder_id").
//
// entityType must match the key used in ResolveFolderID (e.g. "datasets").
// urlParam is the gin URL param name (e.g. "dataset_id").
func CommitFolderResolver(h *FileCommitHandler, entityType, urlParam string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param(urlParam)
		if id == "" {
			jsonError(c, common.CodeParamError, fmt.Sprintf("%s is required", urlParam))
			c.Abort()
			return
		}
		folderID, err := h.ResolveFolderID(entityType, id)
		if err != nil {
			jsonError(c, common.CodeNotFound, fmt.Sprintf("%s folder not found", entityType))
			c.Abort()
			return
		}
		c.Params = append(c.Params, gin.Param{Key: "folder_id", Value: folderID})
		c.Next()
	}
}

func (h *FileCommitHandler) resolveDatasetFolderID(datasetID string) (string, error) {
	kb, err := h.kbDAO.GetByID(datasetID)
	if err != nil {
		return "", err
	}
	files := h.fileDAO.Query(kb.Name, "")
	for _, f := range files {
		if f.SourceType == string(entity.FileSourceKnowledgebase) && f.Type == "folder" && f.TenantID == kb.TenantID {
			return f.ID, nil
		}
	}
	return "", common.ErrNotFound
}

// CreateCommitRequest represents the request body for creating a commit
type CreateCommitRequest struct {
	Message string              `json:"message" binding:"required"`
	Files   []entity.FileChange `json:"files" binding:"required"`
}

// CreateCommit creates a new commit for a workspace folder
// @Summary Create Commit
// @Description Create a new commit with file changes for a workspace folder
// @Tags file_commit
// @Accept json
// @Produce json
// @Param folder_id path string true "workspace folder ID"
// @Param body body CreateCommitRequest true "commit request"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{folder_id}/commits [post]
func (h *FileCommitHandler) CreateCommit(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	folderID := c.Param("folder_id")
	if folderID == "" {
		jsonError(c, common.CodeParamError, "folder_id is required")
		return
	}

	var req CreateCommitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeBadRequest, err.Error())
		return
	}

	commit, err := h.commitService.CreateCommit(folderID, user.ID, req.Message, req.Files)
	if err != nil {
		jsonInternalError(c, err)
		return
	}

	ct := int64(0)
	if commit.CreateTime != nil {
		ct = *commit.CreateTime
	}

	c.JSON(http.StatusOK, gin.H{
		"code": common.CodeSuccess,
		"data": entity.CommitResponse{
			ID:         commit.ID,
			FolderID:   commit.FolderID,
			ParentID:   commit.ParentID,
			Message:    commit.Message,
			AuthorID:   commit.AuthorID,
			FileCount:  commit.FileCount,
			TreeState:  commit.TreeState,
			CreateTime: &ct,
		},
		"message": common.CodeSuccess.Message(),
	})
}

// ListCommits lists commits for a workspace folder
// @Summary List Commits
// @Description List all commits for a workspace folder with pagination
// @Tags file_commit
// @Accept json
// @Produce json
// @Param folder_id path string true "workspace folder ID"
// @Param page query int false "page number"
// @Param page_size query int false "items per page"
// @Param order_by query string false "order by field"
// @Param desc query bool false "descending order"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{folder_id}/commits [get]
func (h *FileCommitHandler) ListCommits(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	folderID := c.Param("folder_id")
	if folderID == "" {
		jsonError(c, common.CodeParamError, "folder_id is required")
		return
	}

	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p >= 1 {
			page = p
		}
	}

	pageSize := 15
	if psStr := c.Query("page_size"); psStr != "" {
		if ps, err := strconv.Atoi(psStr); err == nil {
			if ps < 1 {
				ps = 15
			}
			pageSize = ps
		}
	}

	orderBy := c.DefaultQuery("order_by", "create_time")
	desc := true
	if descStr := c.Query("desc"); descStr != "" {
		desc = descStr != "false"
	}

	commits, total, err := h.commitService.ListCommits(folderID, page, pageSize, orderBy, desc)
	if err != nil {
		jsonInternalError(c, err)
		return
	}

	var commitList []entity.CommitResponse
	for _, commit := range commits {
		var ct int64
		if commit.CreateTime != nil {
			ct = *commit.CreateTime
		}
		commitList = append(commitList, entity.CommitResponse{
			ID:         commit.ID,
			FolderID:   commit.FolderID,
			ParentID:   commit.ParentID,
			Message:    commit.Message,
			AuthorID:   commit.AuthorID,
			FileCount:  commit.FileCount,
			CreateTime: &ct,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": common.CodeSuccess,
		"data": gin.H{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
			"commits":   commitList,
		},
		"message": common.CodeSuccess.Message(),
	})
}

// GetCommit gets details of a single commit
// @Summary Get Commit
// @Description Get details of a single commit including file changes
// @Tags file_commit
// @Accept json
// @Produce json
// @Param folder_id path string true "workspace folder ID"
// @Param commit_id path string true "commit ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{folder_id}/commits/{commit_id} [get]
func (h *FileCommitHandler) GetCommit(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	folderID := c.Param("folder_id")
	commitID := c.Param("commit_id")
	if commitID == "" {
		jsonError(c, common.CodeParamError, "commit_id is required")
		return
	}

	commit, err := h.commitService.GetCommit(commitID)
	if err != nil {
		jsonError(c, common.CodeNotFound, "Commit not found")
		return
	}

	if commit.FolderID != folderID {
		jsonError(c, common.CodeNotFound, "Commit not found in workspace")
		return
	}

	items, err := h.commitService.ListCommitFiles(commitID)
	if err != nil {
		items = []*entity.FileCommitItem{}
	}

	var ct int64
	if commit.CreateTime != nil {
		ct = *commit.CreateTime
	}

	c.JSON(http.StatusOK, gin.H{
		"code": common.CodeSuccess,
		"data": gin.H{
			"id":         commit.ID,
			"folder_id":  commit.FolderID,
			"parent_id":  commit.ParentID,
			"message":    commit.Message,
			"author_id":  commit.AuthorID,
			"file_count": commit.FileCount,
			"create_time": ct,
			"files": items,
		},
		"message": common.CodeSuccess.Message(),
	})
}

// ListCommitFiles lists all file changes in a commit
// @Summary List Commit Files
// @Description List all file changes in a given commit
// @Tags file_commit
// @Accept json
// @Produce json
// @Param folder_id path string true "workspace folder ID"
// @Param commit_id path string true "commit ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{folder_id}/commits/{commit_id}/files [get]
func (h *FileCommitHandler) ListCommitFiles(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	folderID := c.Param("folder_id")
	commitID := c.Param("commit_id")
	if commitID == "" {
		jsonError(c, common.CodeParamError, "commit_id is required")
		return
	}

	commit, err := h.commitService.GetCommit(commitID)
	if err != nil {
		jsonError(c, common.CodeNotFound, "Commit not found")
		return
	}
	if commit.FolderID != folderID {
		jsonError(c, common.CodeNotFound, "Commit not found in workspace")
		return
	}

	items, err := h.commitService.ListCommitFiles(commitID)
	if err != nil {
		jsonInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    items,
		"message": common.CodeSuccess.Message(),
	})
}

// DiffCommits compares two commits
// @Summary Diff Commits
// @Description Compare two commits and return the diff
// @Tags file_commit
// @Accept json
// @Produce json
// @Param folder_id path string true "workspace folder ID"
// @Param from query string true "from commit ID"
// @Param to query string true "to commit ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{folder_id}/commits/diff [get]
func (h *FileCommitHandler) DiffCommits(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	folderID := c.Param("folder_id")
	fromID := c.Query("from")
	toID := c.Query("to")
	if fromID == "" || toID == "" {
		jsonError(c, common.CodeParamError, "'from' and 'to' query parameters are required")
		return
	}

	fromCommit, err := h.commitService.GetCommit(fromID)
	if err != nil {
		jsonError(c, common.CodeNotFound, "Commit not found")
		return
	}
	toCommit, err := h.commitService.GetCommit(toID)
	if err != nil {
		jsonError(c, common.CodeNotFound, "Commit not found")
		return
	}
	if fromCommit.FolderID != folderID || toCommit.FolderID != folderID {
		jsonError(c, common.CodeNotFound, "Commit not found in workspace")
		return
	}

	diff, err := h.commitService.DiffCommits(fromID, toID)
	if err != nil {
		jsonInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    diff,
		"message": common.CodeSuccess.Message(),
	})
}

// GetUncommittedChanges gets uncommitted changes
// @Summary Get Uncommitted Changes
// @Description Get uncommitted changes for a workspace folder (like git status)
// @Tags file_commit
// @Accept json
// @Produce json
// @Param folder_id path string true "workspace folder ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{folder_id}/changes [get]
func (h *FileCommitHandler) GetUncommittedChanges(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	folderID := c.Param("folder_id")
	if folderID == "" {
		jsonError(c, common.CodeParamError, "folder_id is required")
		return
	}

	changes, err := h.commitService.GetUncommittedChanges(folderID)
	if err != nil {
		jsonInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    changes,
		"message": common.CodeSuccess.Message(),
	})
}

// GetCommitTree gets the folder tree snapshot for a commit
// @Summary Get Commit Tree
// @Description Get the folder tree snapshot for a given commit
// @Tags file_commit
// @Accept json
// @Produce json
// @Param folder_id path string true "workspace folder ID"
// @Param commit_id path string true "commit ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{folder_id}/commits/{commit_id}/tree [get]
func (h *FileCommitHandler) GetCommitTree(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	folderID := c.Param("folder_id")
	commitID := c.Param("commit_id")
	if commitID == "" {
		jsonError(c, common.CodeParamError, "commit_id is required")
		return
	}

	commit, err := h.commitService.GetCommit(commitID)
	if err != nil {
		jsonError(c, common.CodeNotFound, "Commit not found")
		return
	}
	if commit.FolderID != folderID {
		jsonError(c, common.CodeNotFound, "Commit not found in workspace")
		return
	}

	tree, err := h.commitService.GetCommitTree(commitID)
	if err != nil {
		jsonInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    tree,
		"message": common.CodeSuccess.Message(),
	})
}

// GetCommitFileContent gets file content as it existed in a given commit
// @Summary Get Commit File Content
// @Description Get file content as it existed in a specific commit
// @Tags file_commit
// @Accept json
// @Produce json
// @Param folder_id path string true "workspace folder ID"
// @Param commit_id path string true "commit ID"
// @Param file_id path string true "file ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{folder_id}/commits/{commit_id}/files/{file_id}/content [get]
func (h *FileCommitHandler) GetCommitFileContent(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	folderID := c.Param("folder_id")
	commitID := c.Param("commit_id")
	fileID := c.Param("file_id")

	if folderID == "" || commitID == "" || fileID == "" {
		jsonError(c, common.CodeParamError, "folder_id, commit_id, and file_id are required")
		return
	}

	commit, err := h.commitService.GetCommit(commitID)
	if err != nil {
		jsonError(c, common.CodeNotFound, "Commit not found")
		return
	}
	if commit.FolderID != folderID {
		jsonError(c, common.CodeNotFound, "Commit not found in workspace")
		return
	}

	content, err := h.commitService.GetCommitFileContent(folderID, commitID, fileID)
	if err != nil {
		jsonError(c, common.CodeNotFound, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": common.CodeSuccess,
		"data": gin.H{
			"content": string(content),
		},
		"message": common.CodeSuccess.Message(),
	})
}

// GetFileVersionHistory gets version history for a specific file
// @Summary Get File Version History
// @Description Get version history for a specific file across all commits
// @Tags file_commit
// @Accept json
// @Produce json
// @Param file_id path string true "file ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/files/{file_id}/versions [get]
func (h *FileCommitHandler) GetFileVersionHistory(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	fileID := c.Param("id")
	if fileID == "" {
		jsonError(c, common.CodeParamError, "file_id is required")
		return
	}

	versions, err := h.commitService.GetFileVersionHistory(fileID)
	if err != nil {
		jsonInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    versions,
		"message": common.CodeSuccess.Message(),
	})
}
