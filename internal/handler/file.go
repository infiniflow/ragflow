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
	"ragflow/internal/common"
	"strconv"

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

// ListFiles list files
// @Summary List Files
// @Description Get list of files for the current user with filtering, pagination and sorting
// @Tags file
// @Accept json
// @Produce json
// @Param parent_id query string false "parent folder ID"
// @Param keywords query string false "search keywords"
// @Param page query int false "page number (default: 1)"
// @Param page_size query int false "items per page (default: 15)"
// @Param orderby query string false "order by field (default: create_time)"
// @Param desc query bool false "descending order (default: true)"
// @Success 200 {object} service.ListFilesResponse
// @Router /v1/file/list [get]
func (h *FileHandler) ListFiles(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// Parse query parameters
	parentID := c.Query("parent_id")
	keywords := c.Query("keywords")

	// Parse page (default: 1)
	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	// Parse page_size (default: 15)
	pageSize := 15
	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
			pageSize = ps
		}
	}

	// Parse orderby (default: create_time)
	orderby := c.DefaultQuery("orderby", "create_time")

	// Parse desc (default: true)
	desc := true
	if descStr := c.Query("desc"); descStr != "" {
		desc = descStr != "false"
	}

	// List files
	result, err := h.fileService.ListFiles(userID, parentID, page, pageSize, orderby, desc, keywords)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    result,
		"message": "success",
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
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    gin.H{"root_folder": rootFolder},
		"message": "success",
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
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "file_id is required",
		})
		return
	}

	// Get parent folder
	parentFolder, err := h.fileService.GetParentFolder(fileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    gin.H{"parent_folder": parentFolder},
		"message": "success",
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
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "file_id is required",
		})
		return
	}

	// Get all parent folders
	parentFolders, err := h.fileService.GetAllParentFolders(fileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    gin.H{"parent_folders": parentFolders},
		"message": "success",
	})
}

// GetFile gets file by ID
// @Summary Get File
// @Description Get file information by file ID
// @Tags file
// @Accept json
// @Produce json
// @Param id query string true "file ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/file/get [get]
func (h *FileHandler) GetFile(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	// Get file_id from query
	fileID := c.Query("id")
	if fileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "id is required",
		})
		return
	}

	// Get file
	file, err := h.fileService.GetFileByID(fileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    file,
		"message": "success",
	})
}

// CreateFolder creates a new folder
// @Summary Create Folder
// @Description Create a new folder under specified parent folder
// @Tags file
// @Accept json
// @Produce json
// @Param name body string true "folder name"
// @Param parent_id body string false "parent folder ID (default: root)"
// @Param type body string false "file type (default: folder)"
// @Success 200 {object} map[string]interface{}
// @Router /v1/file/create [post]
func (h *FileHandler) CreateFolder(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// Parse request body
	var req struct {
		Name     string `json:"name" binding:"required"`
		ParentID string `json:"parent_id"`
		Type     string `json:"type"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	// Create folder
	file, err := h.fileService.CreateFolder(userID, req.Name, req.ParentID, req.Type)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    file,
		"message": "success",
	})
}

// DeleteFiles deletes files by IDs
// @Summary Delete Files
// @Description Delete files or folders by their IDs
// @Tags file
// @Accept json
// @Produce json
// @Param ids body []string true "list of file IDs to delete"
// @Success 200 {object} map[string]interface{}
// @Router /v1/file/delete [post]
func (h *FileHandler) DeleteFiles(c *gin.Context) {
	_, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	// Parse request body
	var req struct {
		IDs []string `json:"ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	// Delete files
	if err := h.fileService.DeleteFiles(req.IDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    true,
		"message": "success",
	})
}
