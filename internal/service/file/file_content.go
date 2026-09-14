package file

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/parser/parser"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"strings"
)

// GetFileContent gets file metadata and checks permission for download
// Matches Python's file_api_service.get_file_content function
func (s *FileService) GetFileContent(ctx context.Context, uid, fileID string) (*entity.File, error) {
	file, err := s.fileDAO.GetByID(ctx, dao.DB, fileID)
	if err != nil || file == nil {
		return nil, fmt.Errorf("document not found")
	}
	if !s.checkFilePerm(ctx, s.fileDAO, file, uid) {
		return nil, fmt.Errorf("no authorization")
	}
	return file, nil
}

// GetStorageAddress gets storage address for a file (fallback for when direct blob is empty)
// Matches Python's File2DocumentService.get_storage_address function
func (s *FileService) GetStorageAddress(ctx context.Context, fileID string) (*StorageAddress, error) {
	// Get file2document mapping
	f2d, err := s.file2DocumentDAO.GetByFileID(ctx, dao.DB, fileID)
	if err != nil || len(f2d) == 0 {
		return nil, fmt.Errorf("file2document mapping not found")
	}

	// Get the file
	if f2d[0].FileID == nil {
		return nil, fmt.Errorf("file_id is nil in file2document mapping")
	}
	file, err := s.fileDAO.GetByID(ctx, dao.DB, *f2d[0].FileID)
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
	doc, err := documentDAO.GetByID(ctx, dao.DB, *f2d[0].DocumentID)
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
func (s *FileService) DownloadAgentFile(ctx context.Context, tenantID, location string) ([]byte, error) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	bucketName := fmt.Sprintf("%s-downloads", tenantID)

	blob, err := storageImpl.Get(ctx, bucketName, location)
	if err != nil {
		return nil, fmt.Errorf("failed to read file from storage: %w", err)
	}

	return blob, nil
}

// GetFileContents fetches file contents (text + image) from storage
// for the given file dicts. Images are always returned as MIME-preserving
// base64 data URIs so the multimodal conversion layer (parseDataURIOrB64)
// accepts them.
//
// File dicts are the descriptors returned by the upload_info endpoint
// (UploadInfos / storeUploadInfoBlob). They contain:
//
//   - "id":         storage location UUID (key in the downloads bucket)
//   - "created_by": the user ID who owns the downloads bucket
//   - "name":       the original filename
//   - "mime_type":  the content type
//
// Blobs are stored directly in "{created_by}-downloads/{id}" in object
// storage WITHOUT a corresponding File entity row in the database.
// Mirrors Python's FileService.get_files → get_blob(user_id, file_id).
func (s *FileService) GetFileContents(ctx context.Context, uid string, fileDicts []map[string]interface{}) (texts []string, images []string, err error) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, nil, fmt.Errorf("storage not initialized")
	}

	for _, fd := range fileDicts {
		id, _ := fd["id"].(string)
		if id == "" {
			continue
		}
		name, _ := fd["name"].(string)
		mimeType, _ := fd["mime_type"].(string)
		createdBy, _ := fd["created_by"].(string)
		if createdBy == "" {
			createdBy = uid
		}
		// Permission: only the owner can access their uploads bucket.
		if createdBy != uid {
			return nil, nil, fmt.Errorf("no authorization")
		}

		data, derr := storageImpl.Get(ctx, createdBy+"-downloads", id)
		if derr != nil || len(data) == 0 {
			continue
		}

		ft := utility.FilenameType(name)
		if ft == utility.FileTypeVISUAL {
			mediaType := strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
			if mediaType == "" {
				ext := utility.GetFileExtension(name)
				mediaType = utility.GetContentType(ext, string(ft))
			}
			images = append(images, "data:"+mediaType+";base64,"+base64.StdEncoding.EncodeToString(data))
		} else {
			texts = append(texts, parseFileContent(ctx, name, data))
		}
	}
	return texts, images, nil
}

// ParseAgentUploads resolves descriptors returned by upload_info from the
// caller's downloads bucket and converts them to sys.files values.
func (s *FileService) ParseAgentUploads(ctx context.Context, userID string, fileDicts []map[string]interface{}, layoutRecognize string) ([]string, error) {
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

		data, err := storageImpl.Get(ctx, createdBy+"-downloads", id)
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

		content, err := parseAgentUploadContent(ctx, name, data, layoutRecognize)
		if err != nil {
			return nil, fmt.Errorf("file %q: parse upload: %w", name, err)
		}
		contents = append(contents, content)
	}
	return contents, nil
}

func parseAgentUploadContent(ctx context.Context, filename string, data []byte, layoutRecognize string) (string, error) {
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
		res := fp.ParseWithResult(ctx, filename, data)
		if res.Err != nil {
			return "", res.Err
		}
		parsed, err := parseResultText(res)
		if err != nil {
			return "", err
		}
		content = parsed
	}
	return fmt.Sprintf("\n -----------------\nFile: %s\nContent as following: \n%s", filename, content), nil
}

// parseResultText converts a parser result into the readable text expected by
// sys.files. JSON results are flattened in item order, preferring each item's
// text field and serializing items without one as a final fallback.
func parseResultText(res parser.ParseResult) (string, error) {
	switch strings.ToLower(strings.TrimSpace(res.OutputFormat)) {
	case "text":
		return res.Text, nil
	case "markdown":
		return res.Markdown, nil
	case "html":
		return res.HTML, nil
	case "json":
		if len(res.JSON) > 0 {
			if rendered, ok := renderSpreadsheetJSON(res.JSON); ok {
				return rendered, nil
			}
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
			return strings.Join(parts, "\n"), nil
		}
		// Some legacy parsers mark the result as JSON while only populating a
		// rendered companion field. Preserve that content instead of returning
		// an empty sys.files value.
		for _, fallback := range []string{res.Markdown, res.HTML, res.Text} {
			if fallback != "" {
				return fallback, nil
			}
		}
		return "", nil
	default:
		return "", fmt.Errorf("unsupported parser output format %q", res.OutputFormat)
	}
}

func renderSpreadsheetJSON(items []map[string]any) (string, bool) {
	parts := make([]string, 0, len(items))
	rendered := false
	for i := 0; i < len(items); {
		if !isSpreadsheetRowItem(items[i]) {
			if text, ok := items[i]["text"].(string); ok {
				parts = append(parts, text)
			} else {
				raw, err := json.Marshal(items[i])
				if err != nil {
					return "", false
				}
				parts = append(parts, string(raw))
			}
			i++
			continue
		}

		start := i
		key := spreadsheetTableKey(items[i])
		for i < len(items) && isSpreadsheetRowItem(items[i]) && spreadsheetTableKey(items[i]) == key {
			i++
		}
		parts = append(parts, renderSpreadsheetTable(items[start:i]))
		rendered = true
	}
	if !rendered {
		return "", false
	}
	return strings.Join(parts, "\n"), true
}

func isSpreadsheetRowItem(item map[string]any) bool {
	kind, _ := item["ck_type"].(string)
	return kind == "table_header" || kind == "table_row"
}

func spreadsheetTableKey(item map[string]any) string {
	if tableID, _ := item["table_id"].(string); strings.TrimSpace(tableID) != "" {
		return "table:" + tableID
	}
	if sheet, _ := item["sheet"].(string); strings.TrimSpace(sheet) != "" {
		return "sheet:" + sheet
	}
	return ""
}

func renderSpreadsheetTable(items []map[string]any) string {
	if len(items) == 0 {
		return ""
	}
	header := spreadsheetStringSlice(items[0]["cells"])
	if len(header) == 0 {
		header = spreadsheetStringSlice(items[0]["headers"])
	}
	if len(header) == 0 {
		header = spreadsheetStringSlice(items[0]["text"])
	}

	sheet, _ := items[0]["sheet"].(string)
	var builder strings.Builder
	builder.WriteString("<table>")
	if strings.TrimSpace(sheet) != "" {
		builder.WriteString("<caption>")
		builder.WriteString(html.EscapeString(strings.TrimSpace(sheet)))
		builder.WriteString("</caption>")
	}
	if len(header) > 0 {
		builder.WriteString("<tr>")
		for _, cell := range header {
			builder.WriteString("<th>")
			builder.WriteString(html.EscapeString(cell))
			builder.WriteString("</th>")
		}
		builder.WriteString("</tr>")
	}
	for _, item := range items {
		kind, _ := item["ck_type"].(string)
		if kind == "table_header" {
			continue
		}
		cells := spreadsheetStringSlice(item["cells"])
		if len(cells) == 0 {
			cells = spreadsheetStringSlice(item["text"])
		}
		builder.WriteString("<tr>")
		for _, cell := range cells {
			builder.WriteString("<td>")
			builder.WriteString(html.EscapeString(cell))
			builder.WriteString("</td>")
		}
		builder.WriteString("</tr>")
	}
	builder.WriteString("</table>")
	return builder.String()
}

func spreadsheetStringSlice(value any) []string {
	switch values := value.(type) {
	case []string:
		out := make([]string, len(values))
		for i, value := range values {
			out[i] = strings.TrimSpace(value)
		}
		return out
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			out = append(out, strings.TrimSpace(fmt.Sprint(value)))
		}
		return out
	case string:
		if strings.TrimSpace(values) == "" {
			return nil
		}
		return []string{strings.TrimSpace(values)}
	default:
		return nil
	}
}

// parseFileContent tries to parse a file's contents using the appropriate parser.
// Falls back to returning raw text if no parser is available.
func parseFileContent(ctx context.Context, filename string, data []byte) string {
	fileType := utility.GetFileType(filename)
	if fileType == utility.FileTypeOTHER {
		return string(data)
	}
	fp, err := parser.GetParser(fileType)
	if err != nil {
		return string(data)
	}
	res := fp.ParseWithResult(ctx, filename, data)
	if res.Err != nil {
		return string(data)
	}
	content, err := parseResultText(res)
	if err != nil {
		return string(data)
	}
	return content
}
