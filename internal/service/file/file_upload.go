package file

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"strings"
	"time"
)

// UploadFile uploads files to a folder
func (s *FileService) UploadFile(tenantID, parentID string, files []*multipart.FileHeader) ([]map[string]interface{}, error) {
	if parentID == "" {
		rootFolder, err := s.fileDAO.GetRootFolder(tenantID)
		if err != nil {
			return nil, fmt.Errorf("failed to get root folder: %w", err)
		}
		parentID = rootFolder.ID
	}

	_, err := s.fileDAO.GetByID(parentID)
	if err != nil {
		return nil, fmt.Errorf("Can't find this folder!")
	}

	maxFileNumPerUser := common.GetEnv(common.EnvMaxFileNumPerUser)
	if maxFileNumPerUser != "" {
		var maxNum int64
		if _, err = fmt.Sscanf(maxFileNumPerUser, "%d", &maxNum); err == nil && maxNum > 0 {
			var docCount int64
			docCount, err = s.GetDocCount(tenantID)
			if err != nil {
				return nil, fmt.Errorf("failed to get document count: %w", err)
			}
			if docCount >= maxNum {
				return nil, fmt.Errorf("Exceed the maximum file number of a free user!")
			}
		}
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	var result []map[string]interface{}

	for _, fileHeader := range files {
		filename := fileHeader.Filename
		if filename == "" {
			return nil, fmt.Errorf("No file selected!")
		}

		fileType := utility.FilenameType(filename)

		fileObjNames := s.parseFilePath(filename)

		var idList []string
		idList, err = s.fileDAO.GetIDListByID(parentID, fileObjNames, 1, []string{parentID})
		if err != nil {
			return nil, fmt.Errorf("failed to get file ID list: %w", err)
		}

		var lastFolder *entity.File
		if len(fileObjNames) != len(idList)-1 {
			lastID := idList[len(idList)-1]
			lastFolder, err = s.fileDAO.GetByID(lastID)
			if err != nil {
				return nil, fmt.Errorf("Folder not found!")
			}
			var createdFolder *entity.File
			createdFolder, err = s.createFolderRecursive(lastFolder, fileObjNames, len(idList), tenantID)
			if err != nil {
				return nil, fmt.Errorf("failed to create folder: %w", err)
			}
			lastFolder = createdFolder
		} else {
			lastID := idList[len(idList)-2]
			lastFolder, err = s.fileDAO.GetByID(lastID)
			if err != nil {
				return nil, fmt.Errorf("Folder not found!")
			}
		}

		location := fileObjNames[len(fileObjNames)-1]
		for storageImpl.ObjExist(lastFolder.ID, location) {
			location += "_"
		}

		src, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open uploaded file: %w", err)
		}
		defer src.Close()

		data, err := io.ReadAll(src)
		if err != nil {
			return nil, fmt.Errorf("failed to read file data: %w", err)
		}

		if err = storageImpl.Put(lastFolder.ID, location, data); err != nil {
			return nil, fmt.Errorf("failed to store file: %w", err)
		}

		uniqueName := s.getUniqueFilename(fileObjNames[len(fileObjNames)-1], lastFolder.ID, tenantID)

		fileRecord := &entity.File{
			ID:         utility.GenerateToken(),
			ParentID:   lastFolder.ID,
			TenantID:   tenantID,
			CreatedBy:  tenantID,
			Name:       uniqueName,
			Location:   &location,
			Size:       int64(len(data)),
			Type:       string(fileType),
			SourceType: "",
		}

		if err = s.fileDAO.Insert(fileRecord); err != nil {
			return nil, fmt.Errorf("failed to insert file record: %w", err)
		}

		result = append(result, s.toFileResponse(fileRecord))
	}

	return result, nil
}

// UploadInfos mirrors Python's upload_info file branch: store raw bytes in the
// per-user downloads bucket and return lightweight upload descriptors instead
// of creating full File rows in the file-management tree.
func (s *FileService) UploadInfos(userID string, files []*multipart.FileHeader) ([]map[string]interface{}, error) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	results := make([]map[string]interface{}, 0, len(files))
	for _, fileHeader := range files {
		filename := fileHeader.Filename
		if err := s.checkUploadInfoHealth(userID, filename); err != nil {
			return nil, err
		}
		src, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open uploaded file: %w", err)
		}
		data, readErr := readUploadInfoData(src)
		src.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read file data: %w", readErr)
		}

		contentType := fileHeader.Header.Get("Content-Type")
		if contentType == "" {
			contentType = http.DetectContentType(data)
		}
		filename, contentType, data = utility.NormalizeUploadInfoContent(filename, contentType, data)
		resp, err := s.storeUploadInfoBlob(storageImpl, userID, filename, contentType, data)
		if err != nil {
			return nil, err
		}
		results = append(results, resp)
	}
	return results, nil
}

func readUploadInfoData(r io.Reader) ([]byte, error) {
	limited := &io.LimitedReader{R: r, N: maxRemoteFileSize + 1}
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxRemoteFileSize {
		return nil, fmt.Errorf("file size exceeds %d bytes", maxRemoteFileSize)
	}
	return data, nil
}

func (s *FileService) parseFilePath(filename string) []string {
	filename = strings.TrimPrefix(filename, "/")
	parts := strings.Split(filename, "/")
	var result []string
	for _, part := range parts {
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// toUploadInfoResponse converts a newly-uploaded file record to the shape
// Python's upload_info endpoint returns.
func (s *FileService) toUploadInfoResponse(file *entity.File, mimeType string) map[string]interface{} {
	ext := ""
	if idx := strings.LastIndex(file.Name, "."); idx >= 0 {
		ext = strings.ToLower(file.Name[idx+1:])
	}
	return map[string]interface{}{
		"id":          file.ID,
		"name":        file.Name,
		"size":        file.Size,
		"extension":   ext,
		"mime_type":   mimeType,
		"created_by":  file.CreatedBy,
		"created_at":  float64(time.Now().UnixMilli()) / 1000.0,
		"preview_url": nil,
	}
}

func (s *FileService) checkUploadInfoHealth(userID, filename string) error {
	if filename == "" {
		return fmt.Errorf("No file selected!")
	}
	maxFileNumPerUser := common.GetEnv(common.EnvMaxFileNumPerUser)
	if maxFileNumPerUser != "" {
		var maxNum int64
		if _, err := fmt.Sscanf(maxFileNumPerUser, "%d", &maxNum); err == nil && maxNum > 0 {
			var docCount int64
			docCount, err = s.GetDocCount(userID)
			if err != nil {
				return fmt.Errorf("failed to get document count: %w", err)
			}
			if docCount >= maxNum {
				return fmt.Errorf("Exceed the maximum file number of a free user!")
			}
		}
	}
	if len([]byte(filename)) > 255 {
		return fmt.Errorf("Exceed the maximum length of file name!")
	}
	return nil
}

func (s *FileService) storeUploadInfoBlob(storageImpl storage.Storage, userID, filename, contentType string, data []byte) (map[string]interface{}, error) {
	location := utility.GenerateUUID()
	bucket := fmt.Sprintf("%s-downloads", userID)
	if err := storageImpl.Put(bucket, location, data); err != nil {
		return nil, fmt.Errorf("failed to store file: %w", err)
	}
	ext := ""
	if idx := strings.LastIndex(filename, "."); idx >= 0 {
		ext = strings.ToLower(filename[idx+1:])
	}
	return map[string]interface{}{
		"id":          location,
		"name":        filename,
		"size":        int64(len(data)),
		"extension":   ext,
		"mime_type":   contentType,
		"created_by":  userID,
		"created_at":  float64(time.Now().UnixMilli()) / 1000.0,
		"preview_url": nil,
	}, nil
}

// UploadDocumentInfos is the document-level wrapper that stores uploaded blobs
// without creating Document rows, then returns the file metadata including
// size/mime-type/extension.
func (s *FileService) UploadDocumentInfos(userID string, files []*multipart.FileHeader) ([]map[string]interface{}, common.ErrorCode, error) {
	data, err := s.UploadInfos(userID, files)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	return data, common.CodeSuccess, nil
}

// UploadDocumentInfoByURL fetches a remote URL, stores the content without
// creating a Document row, then returns file metadata.
func (s *FileService) UploadDocumentInfoByURL(userID, rawURL string) (map[string]interface{}, common.ErrorCode, error) {
	data, err := s.UploadFromURL(userID, rawURL)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	return data, common.CodeSuccess, nil
}
