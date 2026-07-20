package document

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
)

// UploadLocalDocuments stores each uploaded file in object storage and inserts a
// matching Document row into the dataset. It mirrors Python
// FileService.upload_document: it derives parser_id by filetype, merges the
// optional parser_config override into the dataset config, dedup-renames the
// filename, records size + xxhash content hash, and links each document into the
// file manager (a File row under the dataset folder + a file2document mapping)
// so it surfaces in the dataset's document list. Chunking/embedding happen later
// in the parse step, so nothing here touches the doc store index.
//
// Gaps vs Python (documented, not yet ported): thumbnail generation and
// read_potential_broken_pdf repair.
func (s *DocumentService) UploadLocalDocuments(kb *entity.Knowledgebase, tenantID string, files []*multipart.FileHeader, parentPath string, parserConfigOverride map[string]interface{}) ([]map[string]interface{}, []string) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, []string{"storage not initialized"}
	}

	// Resolve (and create if needed) the dataset's file-manager folder up front.
	// Without the File / file2document linkage the document list (which inner-joins
	// file2document + file) would never surface the uploaded files.
	kbFolder, err := s.ensureKBFolder(kb, tenantID)
	if err != nil {
		return nil, []string{err.Error()}
	}

	// Merge parser_config override (allow-listed keys only) over the dataset config.
	merged := entity.JSONMap{}
	for k, v := range kb.ParserConfig {
		merged[k] = v
	}
	for k, v := range parserConfigOverride {
		merged[k] = v
	}

	safeParent := utility.SanitizeFilename(parentPath)

	// Don't silently disable dedupe protection: a transient lookup failure means
	// the existing-name set is unknown, so fail rather than risk duplicates.
	names, err := s.documentDAO.ListNamesByKbID(kb.ID)
	if err != nil {
		return nil, []string{err.Error()}
	}
	taken := map[string]bool{}
	for _, n := range names {
		taken[n] = true
	}

	var results []map[string]interface{}
	var errMsgs []string

	for _, fh := range files {
		blob, err := readFileHeaderBytes(fh)
		if err != nil {
			errMsgs = append(errMsgs, fh.Filename+": "+err.Error())
			continue
		}

		filename := uniqueUploadName(fh.Filename, taken)

		filetype := utility.FilenameType(filename)
		if filetype == utility.FileTypeOTHER {
			errMsgs = append(errMsgs, fh.Filename+": This type of file has not been supported yet!")
			continue
		}

		location := filename
		if safeParent != "" {
			location = safeParent + "/" + filename
		}
		for storageImpl.ObjExist(kb.ID, location) {
			location += "_"
		}
		if err := storageImpl.Put(kb.ID, location, blob); err != nil {
			errMsgs = append(errMsgs, fh.Filename+": "+err.Error())
			continue
		}

		doc := s.newDatasetDocument(kb, tenantID, filename, location, string(filetype), merged, "local", int64(len(blob)), blob)
		if err := s.InsertDocument(doc); err != nil {
			// Roll back the orphaned blob so a failed insert doesn't leak storage.
			_ = storageImpl.Remove(kb.ID, location)
			errMsgs = append(errMsgs, fh.Filename+": "+err.Error())
			continue
		}
		if err := s.addFileFromKB(doc, kbFolder.ID, kb.TenantID); err != nil {
			// Linkage failed: roll back the document row and blob so the partial
			// state doesn't leave an invisible (unlisted) document behind.
			err = s.rollbackAddFileFromKBError(doc, kb.ID, err)
			_ = storageImpl.Remove(kb.ID, location)
			errMsgs = append(errMsgs, fh.Filename+": "+err.Error())
			continue
		}
		// Only reserve the name once the write fully succeeds.
		taken[filename] = true
		results = append(results, docToRawMap(doc))
	}

	return results, errMsgs
}

// UploadEmptyDocument inserts a zero-byte "virtual" document into the dataset.
func (s *DocumentService) UploadEmptyDocument(kb *entity.Knowledgebase, tenantID, name string) (map[string]interface{}, common.ErrorCode, error) {
	// A transient lookup failure means the existing-name set is unknown; fail
	// rather than write blind and risk a duplicate.
	names, err := s.documentDAO.ListNamesByKbID(kb.ID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	for _, n := range names {
		if n == name {
			return nil, common.CodeDataError, fmt.Errorf("Duplicated document name in the same dataset.")
		}
	}

	kbFolder, err := s.ensureKBFolder(kb, tenantID)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	doc := s.newDatasetDocument(kb, tenantID, name, "", "virtual", kb.ParserConfig, "local", 0, nil)
	if err := s.InsertDocument(doc); err != nil {
		return nil, common.CodeServerError, err
	}
	if err := s.addFileFromKB(doc, kbFolder.ID, kb.TenantID); err != nil {
		return nil, common.CodeServerError, s.rollbackAddFileFromKBError(doc, kb.ID, err)
	}
	return docToRawMap(doc), common.CodeSuccess, nil
}

// ensureKBFolder resolves (creating as needed) the per-dataset file-manager
// folder: root -> .knowledgebase -> <dataset name>. Mirrors Python
// get_root_folder + get_kb_folder + new_a_file_from_kb.
func (s *DocumentService) ensureKBFolder(kb *entity.Knowledgebase, tenantID string) (*entity.File, error) {
	root, err := s.fileDAO.GetRootFolder(tenantID)
	if err != nil {
		return nil, err
	}
	kbRoot, err := s.newAFileFromKB(tenantID, knowledgebaseFolderName, root.ID)
	if err != nil {
		return nil, err
	}
	return s.newAFileFromKB(kb.TenantID, kb.Name, kbRoot.ID)
}

// newAFileFromKB returns the existing folder named name under parentID, or
// creates it. Mirrors Python FileService.new_a_file_from_kb.
func (s *DocumentService) newAFileFromKB(tenantID, name, parentID string) (*entity.File, error) {
	for _, f := range s.fileDAO.Query(name, parentID, tenantID) {
		if f.TenantID == tenantID {
			return f, nil
		}
	}
	loc := ""
	folder := &entity.File{
		ID:         utility.GenerateToken(),
		ParentID:   parentID,
		TenantID:   tenantID,
		CreatedBy:  tenantID,
		Name:       name,
		Type:       "folder",
		Size:       0,
		Location:   &loc,
		SourceType: string(entity.FileSourceKnowledgebase),
	}
	if err := s.fileDAO.Create(folder); err != nil {
		return nil, err
	}
	return folder, nil
}

// addFileFromKB links a document into the file manager: a File row under the
// dataset folder plus a file2document mapping. Mirrors Python
// FileService.add_file_from_kb (idempotent on the document mapping).
func (s *DocumentService) addFileFromKB(doc *entity.Document, kbFolderID, tenantID string) error {
	if existing, err := s.file2DocumentDAO.GetByDocumentID(doc.ID); err == nil && len(existing) > 0 {
		return nil
	}
	name := ""
	if doc.Name != nil {
		name = *doc.Name
	}
	loc := ""
	if doc.Location != nil {
		loc = *doc.Location
	}
	fileID := utility.GenerateToken()
	file := &entity.File{
		ID:         fileID,
		ParentID:   kbFolderID,
		TenantID:   tenantID,
		CreatedBy:  tenantID,
		Name:       name,
		Type:       doc.Type,
		Size:       doc.Size,
		Location:   &loc,
		SourceType: string(entity.FileSourceKnowledgebase),
	}
	if err := s.fileDAO.Create(file); err != nil {
		return err
	}
	docID := doc.ID
	if err := s.file2DocumentDAO.Create(&entity.File2Document{
		ID:         utility.GenerateToken(),
		FileID:     &fileID,
		DocumentID: &docID,
	}); err != nil {
		_ = s.fileDAO.Delete(fileID)
		return err
	}
	return nil
}

func (s *DocumentService) UploadWebDocument(kb *entity.Knowledgebase, tenantID, name, url string) (map[string]interface{}, common.ErrorCode, error) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, common.CodeServerError, fmt.Errorf("storage not initialized")
	}

	kbFolder, err := s.ensureKBFolder(kb, tenantID)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	names, err := s.documentDAO.ListNamesByKbID(kb.ID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	taken := map[string]bool{}
	for _, n := range names {
		taken[n] = true
	}

	blob, headers, _, err := utility.FetchRemoteFileSafely(url, maxUploadDocSize)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	contentType := ""
	if headers != nil {
		contentType = headers.Get("Content-Type")
	}
	filename := normalizeWebDocumentName(name, contentType, blob)
	filename, _, blob = utility.NormalizeUploadInfoContent(filename, contentType, blob)
	filename = uniqueUploadName(filename, taken)

	filetype := utility.FilenameType(filename)
	if filetype == utility.FileTypeOTHER {
		return nil, common.CodeDataError, fmt.Errorf("This type of file has not been supported yet!")
	}

	location := filename
	for storageImpl.ObjExist(kb.ID, location) {
		location += "_"
	}
	if err := storageImpl.Put(kb.ID, location, blob); err != nil {
		return nil, common.CodeServerError, err
	}

	doc := s.newDatasetDocument(kb, tenantID, filename, location, string(filetype), kb.ParserConfig, "web", int64(len(blob)), blob)
	if err := s.InsertDocument(doc); err != nil {
		_ = storageImpl.Remove(kb.ID, location)
		return nil, common.CodeServerError, err
	}
	if err := s.addFileFromKB(doc, kbFolder.ID, kb.TenantID); err != nil {
		err = s.rollbackAddFileFromKBError(doc, kb.ID, err)
		_ = storageImpl.Remove(kb.ID, location)
		return nil, common.CodeServerError, err
	}
	return docToRawMap(doc), common.CodeSuccess, nil
}

func normalizeWebDocumentName(name, contentType string, blob []byte) string {
	filename := utility.SanitizeFilename(name)
	if filepath.Ext(filename) != "" {
		return filename
	}
	lowerCT := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch {
	case lowerCT == "application/pdf" || http.DetectContentType(blob) == "application/pdf" || utility.BytesLooksLikePDF(blob):
		return filename + ".pdf"
	case lowerCT == "text/html" || lowerCT == "application/xhtml+xml" || utility.LooksLikeHTML(blob):
		return filename + ".html"
	default:
		return filename
	}
}

// newDatasetDocument builds a Document row for an upload, deriving parser_id,
// suffix and content hash. blob may be nil for the empty/virtual document.
func (s *DocumentService) newDatasetDocument(kb *entity.Knowledgebase, tenantID, filename, location, filetype string, parserConfig entity.JSONMap, src string, size int64, blob []byte) *entity.Document {
	docID := utility.GenerateToken()
	run := "0"
	status := "1"
	suffix := ""
	if i := strings.LastIndex(filename, "."); i >= 0 {
		suffix = filename[i+1:]
	}
	parserID := kb.ParserID
	if kb.PipelineID != nil {
		parserID = "" // canvas pipeline mode — parser_id not applicable
	}
	loc := location
	doc := &entity.Document{
		ID:           docID,
		KbID:         kb.ID,
		ParserID:     parserID,
		PipelineID:   kb.PipelineID,
		ParserConfig: parserConfig,
		CreatedBy:    tenantID,
		Type:         filetype,
		SourceType:   src,
		Name:         &filename,
		Location:     &loc,
		Size:         size,
		Suffix:       suffix,
		Run:          &run,
		Status:       &status,
	}
	if blob != nil {
		hash := contentHashHex(blob)
		doc.ContentHash = &hash
	}
	return doc
}

// docToRawMap serialises a freshly created Document into the raw key shape the
// handler remaps (chunk_num→chunk_count, kb_id→dataset_id).
func docToRawMap(doc *entity.Document) map[string]interface{} {
	m := map[string]interface{}{
		"id":            doc.ID,
		"kb_id":         doc.KbID,
		"parser_id":     doc.ParserID,
		"parser_config": map[string]interface{}(doc.ParserConfig),
		"created_by":    doc.CreatedBy,
		"type":          doc.Type,
		"source_type":   doc.SourceType,
		"size":          doc.Size,
		"chunk_num":     doc.ChunkNum,
		"token_num":     doc.TokenNum,
		"suffix":        doc.Suffix,
		"run":           "0",
	}
	if doc.Name != nil {
		m["name"] = *doc.Name
	}
	if doc.Location != nil {
		m["location"] = *doc.Location
	}
	if doc.PipelineID != nil {
		m["pipeline_id"] = *doc.PipelineID
	}
	if doc.ContentHash != nil {
		m["content_hash"] = *doc.ContentHash
	}
	return m
}

// uniqueUploadName appends a numeric suffix until the name is free, mirroring
// Python duplicate_name.
func uniqueUploadName(name string, taken map[string]bool) string {
	if !taken[name] {
		return name
	}
	base, ext := name, ""
	if i := strings.LastIndex(name, "."); i >= 0 {
		base, ext = name[:i], name[i:]
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s(%d)%s", base, i, ext)
		if !taken[candidate] {
			return candidate
		}
	}
}

func readFileHeaderBytes(fh *multipart.FileHeader) ([]byte, error) {
	if fh.Size > maxUploadDocSize {
		return nil, fmt.Errorf("file exceeds the maximum allowed size of %d bytes", maxUploadDocSize)
	}
	src, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	blob, err := io.ReadAll(io.LimitReader(src, maxUploadDocSize+1))
	if err != nil {
		return nil, err
	}
	if len(blob) > maxUploadDocSize {
		return nil, fmt.Errorf("file exceeds the maximum allowed size of %d bytes", maxUploadDocSize)
	}
	return blob, nil
}
