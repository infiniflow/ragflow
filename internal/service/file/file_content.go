package file

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/parser/parser"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"strings"
)

// GetFileContent gets file metadata and checks permission for download
// Matches Python's file_api_service.get_file_content function
func (s *FileService) GetFileContent(uid, fileID string) (*entity.File, error) {
	file, err := s.fileDAO.GetByID(fileID)
	if err != nil || file == nil {
		return nil, fmt.Errorf("Document not found!")
	}
	if !s.checkFilePerm(s.fileDAO, file, uid) {
		return nil, fmt.Errorf("No authorization.")
	}
	return file, nil
}

// GetStorageAddress gets storage address for a file (fallback for when direct blob is empty)
// Matches Python's File2DocumentService.get_storage_address function
func (s *FileService) GetStorageAddress(fileID string) (*StorageAddress, error) {
	// Get file2document mapping
	f2d, err := s.file2DocumentDAO.GetByFileID(fileID)
	if err != nil || len(f2d) == 0 {
		return nil, fmt.Errorf("file2document mapping not found")
	}

	// Get the file
	if f2d[0].FileID == nil {
		return nil, fmt.Errorf("file_id is nil in file2document mapping")
	}
	file, err := s.fileDAO.GetByID(*f2d[0].FileID)
	if err != nil || file == nil {
		return nil, fmt.Errorf("file not found")
	}

	// If source_type is empty or local, return file's parent_id and location
	if file.SourceType == "" || entity.FileSource(file.SourceType) == entity.FileSourceLocal {
		if file.Location == nil || *file.Location == "" {
			return nil, fmt.Errorf("file location is empty")
		}
		return &StorageAddress{
			Bucket: file.ParentID,
			Name:   *file.Location,
		}, nil
	}

	// Otherwise, use document's kb_id and location
	if f2d[0].DocumentID == nil {
		return nil, fmt.Errorf("document_id is required")
	}

	documentDAO := dao.NewDocumentDAO()
	doc, err := documentDAO.GetByID(*f2d[0].DocumentID)
	if err != nil || doc == nil {
		return nil, fmt.Errorf("document not found")
	}

	if doc.Location == nil || *doc.Location == "" {
		return nil, fmt.Errorf("document location is empty")
	}

	return &StorageAddress{
		Bucket: doc.KbID,
		Name:   *doc.Location,
	}, nil
}

// DownloadAgentFile downloads an agent-generated file directly from MinIO without querying the database.
func (s *FileService) DownloadAgentFile(tenantID, location string) ([]byte, error) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	bucketName := fmt.Sprintf("%s-downloads", tenantID)

	blob, err := storageImpl.Get(bucketName, location)
	if err != nil {
		return nil, fmt.Errorf("failed to read file from storage: %w", err)
	}

	return blob, nil
}

// GetFileContents fetches file contents (text + image) from storage
// for the given file dicts.
//   - raw=false: images returned as base64 data URIs in images; non-images parsed and returned as text.
//   - raw=true:  images returned as raw bytes in images; non-images parsed and returned as text.
func (s *FileService) GetFileContents(uid string, fileDicts []map[string]interface{}, raw bool) (texts []string, images []string, err error) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, nil, fmt.Errorf("storage not initialized")
	}

	for _, fd := range fileDicts {
		id, _ := fd["id"].(string)
		if id == "" {
			continue
		}
		file, ferr := s.fileDAO.GetByID(id)
		if ferr != nil || file == nil || file.Location == nil || *file.Location == "" {
			continue
		}
		if !s.checkFilePerm(s.fileDAO, file, uid) {
			return nil, nil, fmt.Errorf("No authorization.")
		}
		data, derr := storageImpl.Get(file.ParentID, *file.Location)
		if derr != nil || len(data) == 0 {
			continue
		}
		ft := utility.FilenameType(file.Name)
		if ft == utility.FileTypeVISUAL {
			if raw {
				images = append(images, string(data))
			} else {
				ext := utility.GetFileExtension(file.Name)
				mime := utility.GetContentType(ext, string(ft))
				images = append(images, "data:"+mime+";base64,"+base64.StdEncoding.EncodeToString(data))
			}
		} else {
			texts = append(texts, parseFileContent(file.Name, data))
		}
	}
	return texts, images, nil
}

// parseAgentUploads resolves descriptors returned by upload_info from the
// caller's downloads bucket and converts them to sys.files values.
func (s *FileService) ParseAgentUploads(userID string, fileDicts []map[string]interface{}, layoutRecognize string) ([]string, error) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	contents := make([]string, 0, len(fileDicts))
	for i, fd := range fileDicts {
		id, _ := fd["id"].(string)
		name, _ := fd["name"].(string)
		mimeType, _ := fd["mime_type"].(string)
		createdBy, _ := fd["created_by"].(string)
		if id == "" || name == "" || mimeType == "" || createdBy == "" {
			return nil, fmt.Errorf("file %d: id, name, mime_type, and created_by are required", i)
		}
		if createdBy != userID {
			return nil, fmt.Errorf("file %q: created_by does not match the current user", name)
		}

		data, err := storageImpl.Get(createdBy+"-downloads", id)
		if err != nil {
			return nil, fmt.Errorf("file %q: read upload: %w", name, err)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("file %q: upload is empty", name)
		}

		mediaType := strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
		if strings.HasPrefix(mediaType, "image/") {
			contents = append(contents, "data:"+mediaType+";base64,"+base64.StdEncoding.EncodeToString(data))
			continue
		}

		content, err := parseAgentUploadContent(name, data, layoutRecognize)
		if err != nil {
			return nil, fmt.Errorf("file %q: parse upload: %w", name, err)
		}
		contents = append(contents, content)
	}
	return contents, nil
}

func parseAgentUploadContent(filename string, data []byte, layoutRecognize string) (string, error) {
	content := string(data)
	fileType := utility.GetFileType(filename)
	if fileType != utility.FileTypeOTHER {
		fp, err := parser.GetParser(fileType)
		if err != nil {
			return "", err
		}
		if configurable, ok := fp.(interface{ ConfigureFromSetup(map[string]any) }); ok {
			configurable.ConfigureFromSetup(map[string]any{"layout_recognize": layoutRecognize})
		}
		res := fp.ParseWithResult(filename, data)
		if res.Err != nil {
			return "", res.Err
		}
		switch res.OutputFormat {
		case "text":
			content = res.Text
		case "markdown":
			content = res.Markdown
		case "html":
			content = res.HTML
		case "json":
			parts := make([]string, 0, len(res.JSON))
			for _, item := range res.JSON {
				if text, ok := item["text"].(string); ok {
					parts = append(parts, text)
					continue
				}
				raw, err := json.Marshal(item)
				if err != nil {
					return "", err
				}
				parts = append(parts, string(raw))
			}
			content = strings.Join(parts, "\n")
		}
	}
	return fmt.Sprintf("\n -----------------\nFile: %s\nContent as following: \n%s", filename, content), nil
}

// parseFileContent tries to parse a file's contents using the appropriate parser.
// Falls back to returning raw text if no parser is available.
func parseFileContent(filename string, data []byte) string {
	fileType := utility.GetFileType(filename)
	if fileType == utility.FileTypeOTHER {
		return string(data)
	}
	fp, err := parser.GetParser(fileType)
	if err != nil {
		return string(data)
	}
	res := fp.ParseWithResult(filename, data)
	if res.Err != nil {
		return string(data)
	}
	switch res.OutputFormat {
	case "text":
		return res.Text
	case "markdown":
		return res.Markdown
	case "html":
		return res.HTML
	case "json":
		return string(data)
	default:
		return string(data)
	}
}
