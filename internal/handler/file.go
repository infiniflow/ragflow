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
	"net/http"
	"net/url"
	"ragflow/internal/common"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

// FileHandler file handler
type FileHandler struct {
	fileService *service.FileService
	userService *service.UserService
}

// NewFileHandler create file handler
func NewFileHandler(fileService *service.FileService, userService *service.UserService) *FileHandler {
	return &FileHandler{
		fileService: fileService,
		userService: userService,
	}
}

// ListFiles list files (new endpoint at /api/v1/files matching Python /files)
// @Summary List Files
// @Description Get list of files under a folder with filtering, pagination and sorting (matches Python /files endpoint)
// @Tags file
// @Accept json
// @Produce json
// @Param parent_id query string false "parent folder ID (empty means root folder)"
// @Param keywords query string false "search keywords (case-insensitive)"
// @Param page query int false "page number (default: 1, min: 1)"
// @Param page_size query int false "items per page (default: 15, min: 1, max: 100)"
// @Param orderby query string false "order by field (default: create_time)"
// @Param desc query bool false "descending order (default: true)"
// @Success 200 {object} service.ListFilesResponse
// @Router /api/v1/files [get]
func (h *FileHandler) ListFiles(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	parentID := c.Query("parent_id")
	keywords := c.Query("keywords")

	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p >= 1 {
			page = p
		} else if err != nil {
			jsonError(c, common.CodeParamError, "Invalid page parameter: must be a positive integer")
			return
		}
	}

	pageSize := 15
	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil {
			if ps < 1 {
				jsonError(c, common.CodeParamError, "Invalid page_size parameter: must be at least 1")
				return
			}
			if ps > 100 {
				ps = 100
			}
			pageSize = ps
		} else {
			jsonError(c, common.CodeParamError, "Invalid page_size parameter: must be a positive integer")
			return
		}
	}

	orderby := c.DefaultQuery("orderby", "create_time")
	desc := true
	if descStr := c.Query("desc"); descStr != "" {
		desc = descStr != "false"
	}

	result, err := h.fileService.ListFiles(userID, parentID, page, pageSize, orderby, desc, keywords)
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    result,
		"message": common.CodeSuccess.Message(),
	})
}

// GetRootFolder gets root folder for current user
// @Summary Get Root Folder
// @Description Get or create root folder for the current user
// @Tags file
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /v1/file/root_folder [get]
func (h *FileHandler) GetRootFolder(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// Get root folder
	rootFolder, err := h.fileService.GetRootFolder(userID)
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    gin.H{"root_folder": rootFolder},
		"message": common.CodeSuccess.Message(),
	})
}

// GetParentFolder gets parent folder of a file
// @Summary Get Parent Folder
// @Description Get parent folder of a file by file ID
// @Tags file
// @Accept json
// @Produce json
// @Param file_id query string true "file ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/file/parent_folder [get]
func (h *FileHandler) GetParentFolder(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	// Get file_id from query
	fileID := c.Query("file_id")
	if fileID == "" {
		jsonError(c, common.CodeBadRequest, "file_id is required")
		return
	}

	// Get parent folder
	parentFolder, err := h.fileService.GetParentFolder(fileID)
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    gin.H{"parent_folder": parentFolder},
		"message": common.CodeSuccess.Message(),
	})
}

// GetAllParentFolders gets all parent folders in path
// @Summary Get All Parent Folders
// @Description Get all parent folders in path from file to root
// @Tags file
// @Accept json
// @Produce json
// @Param file_id query string true "file ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/file/all_parent_folder [get]
func (h *FileHandler) GetAllParentFolders(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	// Get file_id from query
	fileID := c.Query("file_id")
	if fileID == "" {
		jsonError(c, common.CodeBadRequest, "file_id is required")
		return
	}

	// Get all parent folders
	parentFolders, err := h.fileService.GetAllParentFolders(fileID)
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    gin.H{"parent_folders": parentFolders},
		"message": common.CodeSuccess.Message(),
	})
}

type CreateFolderRequest struct {
	Name     string `json:"name" binding:"required"`
	ParentID string `json:"parent_id"`
	Type     string `json:"type"`
}

// UploadFile handles file upload and folder creation
// @Summary Upload Files or Create Folder
// @Description Upload files or create a folder based on content type
// @Tags file
// @Accept multipart/form-data, application/json
// @Produce json
// @Param parent_id query string false "parent folder ID (for multipart/form-data)"
// @Param file formData file false "file to upload (for multipart/form-data)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /v1/file/upload [post]
func (h *FileHandler) UploadFile(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	userID := user.ID

	contentType := c.ContentType()

	if strings.Contains(contentType, "multipart/form-data") {
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			jsonError(c, common.CodeBadRequest, "Failed to parse multipart form: "+err.Error())
			return
		}

		form := c.Request.MultipartForm
		if form == nil {
			jsonError(c, common.CodeBadRequest, "No file part!")
			return
		}
		parentID := c.PostForm("parent_id")
		if parentID == "" {
			rootFolder, err := h.fileService.GetRootFolder(userID)
			if err != nil {
				jsonError(c, common.CodeServerError, err.Error())
				return
			}
			parentID = rootFolder["id"].(string)
		}

		files := form.File["file"]
		if len(files) == 0 {
			jsonError(c, common.CodeBadRequest, "No file selected!")
			return
		}

		for _, fileHeader := range files {
			if fileHeader.Filename == "" {
				jsonError(c, common.CodeBadRequest, "No file selected!")
				return
			}
		}

		result, err := h.fileService.UploadFile(userID, parentID, files)
		if err != nil {
			jsonError(c, common.CodeBadRequest, err.Error())
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeSuccess,
			"data":    result,
			"message": common.CodeSuccess.Message(),
		})
		return
	}

	if strings.Contains(contentType, "application/json") {
		var req CreateFolderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": err.Error(),
			})
			return
		}

		parentID := req.ParentID
		if parentID == "" {
			rootFolder, err := h.fileService.GetRootFolder(userID)
			if err != nil {
				jsonError(c, common.CodeServerError, err.Error())
				return
			}
			parentID = rootFolder["id"].(string)
		}

		result, err := h.fileService.CreateFolder(userID, req.Name, parentID, req.Type)
		if err != nil {
			jsonError(c, common.CodeBadRequest, err.Error())
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeSuccess,
			"data":    result,
			"message": common.CodeSuccess.Message(),
		})
		return
	}

	jsonError(c, common.CodeBadRequest, "Unsupported content type")
	return
}

type DeleteFileRequest struct {
	IDs []string `json:"ids" binding:"required,min=1"`
}

// DeleteFiles deletes files
// @Summary Delete Files
// @Description Delete files by IDs
// @Tags file
// @Accept json
// @Produce json
// @Param ids body DeleteFileRequest true "file IDs to delete"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/files [delete]
func (h *FileHandler) DeleteFiles(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req DeleteFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeBadRequest, err.Error())
		return
	}

	success, message := h.fileService.DeleteFiles(c.Request.Context(), user.ID, req.IDs)
	if !success {
		jsonError(c, common.CodeBadRequest, message)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    true,
		"message": common.CodeSuccess.Message(),
	})
}

// MoveFileRequest represents the request body for move files operation
type MoveFileRequest struct {
	SrcFileIDs []string `json:"src_file_ids" binding:"required,min=1"`
	DestFileID string   `json:"dest_file_id"`
	NewName    string   `json:"new_name" binding:"max=255"`
}

// MoveFiles moves and/or renames files
// @Summary Move Files
// @Description Move and/or rename files. Follows Linux mv semantics:
//   - dest_file_id only: move files to a new folder (names unchanged)
//   - new_name only: rename a single file in place (no storage operation)
//   - both: move and rename simultaneously
// @Tags file
// @Accept json
// @Produce json
// @Param body body MoveFileRequest true "Move file request"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/files/move [post]
func (h *FileHandler) MoveFiles(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req MoveFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeBadRequest, err.Error())
		return
	}

	// Validate: at least one of dest_file_id or new_name must be provided
	if req.DestFileID == "" && req.NewName == "" {
		jsonError(c, common.CodeParamError, "At least one of dest_file_id or new_name must be provided")
		return
	}

	// Validate: new_name can only be used with a single file
	if req.NewName != "" && len(req.SrcFileIDs) > 1 {
		jsonError(c, common.CodeParamError, "new_name can only be used with a single file")
		return
	}

	success, message := h.fileService.MoveFiles(user.ID, req.SrcFileIDs, req.DestFileID, req.NewName)
	if !success {
		jsonError(c, common.CodeBadRequest, message)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    true,
		"message": common.CodeSuccess.Message(),
	})
}

// Download handles file download
// @Summary Download File
// @Description Download a file by ID
// @Tags file
// @Accept json
// @Produce octet-stream
// @Param file_id path string true "file ID"
// @Success 200 {file} binary "File stream"
// @Router /api/v1/files/{file_id} [get]
func (h *FileHandler) Download(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	fileID := c.Param("id")
	if fileID == "" {
		jsonError(c, common.CodeParamError, "id is required")
		return
	}

	// Get file metadata and check permission
	file, err := h.fileService.GetFileContent(userID, fileID)
	if err != nil {
		jsonError(c, common.CodeUnauthorized, err.Error())
		return
	}

	// Get storage
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		jsonError(c, common.CodeServerError, "storage not initialized")
		return
	}

	// Try to get file blob from primary location (parent_id, location)
	var blob []byte
	var getErr error
	if file.Location != nil && *file.Location != "" {
		blob, getErr = storageImpl.Get(file.ParentID, *file.Location)
	}

	// If blob is empty, try fallback via file2document
	if len(blob) == 0 {
		storageAddr, err := h.fileService.GetStorageAddress(fileID)
		if err != nil {
			jsonError(c, common.CodeServerError, "Failed to get file storage address: "+err.Error())
			return
		}
		blob, getErr = storageImpl.Get(storageAddr.Bucket, storageAddr.Name)
	}

	// Check if we got valid data
	if len(blob) == 0 {
		errMsg := "Failed to retrieve file blob"
		if getErr != nil {
			errMsg += ": " + getErr.Error()
		}
		jsonError(c, common.CodeServerError, errMsg)
		return
	}

	// Extract file extension
	ext := utility.GetFileExtension(file.Name)

	// Determine content type based on extension and file type
	contentType := utility.GetContentType(ext, file.Type)

	// Set response headers
	if contentType != "" {
		c.Header("Content-Type", contentType)
	}
	if utility.ShouldForceAttachment(ext, contentType) {
		c.Header("X-Content-Type-Options", "nosniff")
		encodedName := url.QueryEscape(file.Name)
		c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+encodedName)
	}

	// Send file data
	c.Data(http.StatusOK, contentType, blob)
}
