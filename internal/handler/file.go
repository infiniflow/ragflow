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
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    errorCode,
			"message": errorMessage,
		})
		return
	}

	userID := user.ID

	contentType := c.ContentType()

	if strings.Contains(contentType, "multipart/form-data") {
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "Failed to parse multipart form: " + err.Error(),
			})
			return
		}

		form := c.Request.MultipartForm
		if form == nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "No file part!",
			})
			return
		}
		parentID := c.PostForm("parent_id")
		if parentID == "" {
			rootFolder, err := h.fileService.GetRootFolder(userID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": err.Error(),
				})
				return
			}
			parentID = rootFolder["id"].(string)
		}

		files := form.File["file"]
		if len(files) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "No file selected!",
			})
			return
		}

		for _, fileHeader := range files {
			if fileHeader.Filename == "" {
				c.JSON(http.StatusBadRequest, gin.H{
					"code":    400,
					"message": "No file selected!",
				})
				return
			}
		}

		result, err := h.fileService.UploadFile(userID, parentID, files)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"data":    result,
			"message": "success",
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
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": err.Error(),
				})
				return
			}
			parentID = rootFolder["id"].(string)
		}

		result, err := h.fileService.CreateFolder(userID, req.Name, parentID, req.Type)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"data":    result,
			"message": "success",
		})
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{
		"code":    400,
		"message": "Unsupported content type",
	})
	return
}
