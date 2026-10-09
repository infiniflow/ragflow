package document

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/pipeline"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
)

// applyColumnOverride writes an upload's column settings onto the component
// node this document will actually run with.
//
// It runs after the parser is resolved rather than merged into the dataset
// config before it, because the two can disagree: a spreadsheet uploaded to a
// dataset configured with another template is switched to the table pipeline for
// its own sake, and that pipeline's TableChunker node carries a different id. An
// override keyed by the dataset's id would then be dropped as unknown and the
// upload would report success with the roles ignored — so when the resolved
// config holds exactly one TableChunker node the override is mapped onto it, and
// when several nodes exist the request has to name the one it means.
func applyColumnOverride(config entity.JSONMap, override map[string]interface{}) (entity.JSONMap, error) {
	if len(override) == 0 {
		return config, nil
	}

	targets := make([]string, 0, 2)
	for key := range config {
		if pipeline.IsTableChunkerNodeKey(key) {
			targets = append(targets, key)
		}
	}
	sort.Strings(targets)

	out := make(entity.JSONMap, len(config))
	for key, value := range config {
		out[key] = value
	}

	for cpnID, raw := range override {
		if !pipeline.IsTableChunkerNodeKey(cpnID) {
			return nil, fmt.Errorf("parser_config[%q] must be keyed by a TableChunker node", cpnID)
		}
		params, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("parser_config[%q] must be an object of component parameters", cpnID)
		}
		if _, _, err := pipeline.ValidateTableColumnOverride(params); err != nil {
			return nil, fmt.Errorf("parser_config[%q]: %v", cpnID, err)
		}

		target := cpnID
		if _, exists := out[cpnID]; !exists {
			switch {
			case len(targets) == 1:
				target = targets[0]
			case len(targets) == 0:
				return nil, fmt.Errorf("the parser this document runs has no TableChunker node to apply %q to", cpnID)
			default:
				return nil, fmt.Errorf("parser_config[%q] names a node this document does not run; it has several TableChunker nodes (%s), so the setting must name one of them",
					cpnID, strings.Join(targets, ", "))
			}
		}

		base, _ := out[target].(map[string]interface{})
		merged, err := pipeline.MergeTableChunkerParams(base, params)
		if err != nil {
			return nil, fmt.Errorf("parser_config[%q]: %v", target, err)
		}
		out[target] = merged
	}
	return out, nil
}

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
func (s *DocumentService) UploadLocalDocuments(ctx context.Context, kb *entity.Knowledgebase, tenantID string, files []*multipart.FileHeader, parentPath string, parserConfigOverride map[string]interface{}) ([]map[string]interface{}, []string) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, []string{"storage not initialized"}
	}

	// Resolve (and create if needed) the dataset's file-manager folder up front.
	// Without the File / file2document linkage the document list (which inner-joins
	// file2document + file) would never surface the uploaded files.
	kbFolder, err := s.ensureKBFolder(ctx, kb, tenantID)
	if err != nil {
		return nil, []string{err.Error()}
	}

	// The dataset's configuration, without the upload override: the override is
	// applied after the parser is resolved, because the node it names has to be
	// the node this document will actually run.
	merged := entity.JSONMap{}
	for k, v := range kb.ParserConfig {
		merged[k] = v
	}

	safeParent := utility.SanitizeFilename(parentPath)

	var results []map[string]interface{}
	var errMsgs []string

	for _, fh := range files {
		var blob []byte
		blob, err = readFileHeaderBytes(fh)
		if err != nil {
			errMsgs = append(errMsgs, fh.Filename+": "+err.Error())
			continue
		}

		filename, err := common.UniqueFileName(fh.Filename, 255, func(candidate string) (bool, error) {
			return s.documentDAO.NameExistsInKB(ctx, dao.DB, kb.ID, candidate)
		})
		if err != nil {
			errMsgs = append(errMsgs, fh.Filename+": "+err.Error())
			continue
		}

		filetype := utility.FilenameType(filename)
		if filetype == utility.FileTypeOTHER {
			errMsgs = append(errMsgs, fh.Filename+": This type of file has not been supported yet!")
			continue
		}

		location := filename
		if safeParent != "" {
			location = safeParent + "/" + filename
		}
		for storageImpl.ObjExist(ctx, kb.ID, location) {
			location += "_"
		}
		if err = storageImpl.Put(ctx, kb.ID, location, blob); err != nil {
			errMsgs = append(errMsgs, fh.Filename+": "+err.Error())
			continue
		}

		parserID, parserConfig := resolveDocumentParser(ctx, kb, filename, filetype, merged)
		parserConfig, err = applyColumnOverride(parserConfig, parserConfigOverride)
		if err != nil {
			// The blob is already stored; a refused override must not leave it
			// without a document row, as every later upload of the same name
			// would then collide with it.
			if rmErr := removeObjectBestEffort(ctx, storageImpl, kb.ID, location); rmErr != nil {
				common.Warn(fmt.Sprintf("upload rollback: failed to remove orphaned blob %s/%s: %v", kb.ID, location, rmErr))
			}
			errMsgs = append(errMsgs, fh.Filename+": "+err.Error())
			continue
		}
		doc := s.newDatasetDocument(kb, tenantID, filename, location, string(filetype), parserID, parserConfig, "local", int64(len(blob)), blob)
		if err = s.InsertDocument(doc); err != nil {
			// Roll back the orphaned blob so a failed insert doesn't leak storage.
			rmErr := removeObjectBestEffort(ctx, storageImpl, kb.ID, location)
			if rmErr != nil {
				common.Warn(fmt.Sprintf("upload rollback: failed to remove orphaned blob %s/%s: %v", kb.ID, location, rmErr))
			}
			errMsgs = append(errMsgs, fh.Filename+": "+err.Error())
			continue
		}
		if err = s.addFileFromKB(ctx, doc, kbFolder.ID, kb.TenantID); err != nil {
			// Linkage failed: roll back the document row and blob so the partial
			// state doesn't leave an invisible (unlisted) document behind.
			err = s.rollbackAddFileFromKBError(ctx, doc, kb.ID, err)
			rmErr := removeObjectBestEffort(ctx, storageImpl, kb.ID, location)
			if rmErr != nil {
				common.Warn(fmt.Sprintf("UploadLocalDocuments: failed to remove blob %s/%s: %v", kb.ID, location, rmErr))
			}
			errMsgs = append(errMsgs, fh.Filename+": "+err.Error())
			continue
		}
		results = append(results, docToRawMap(doc))
	}

	return results, errMsgs
}

// UploadEmptyDocument inserts a zero-byte "virtual" document into the dataset.
func (s *DocumentService) UploadEmptyDocument(ctx context.Context, kb *entity.Knowledgebase, tenantID, name string) (map[string]interface{}, common.ErrorCode, error) {
	// Fall back to a numbered name when the requested one is already taken.
	name, err := common.UniqueFileName(name, 255, func(candidate string) (bool, error) {
		return s.documentDAO.NameExistsInKB(ctx, dao.DB, kb.ID, candidate)
	})
	if err != nil {
		return nil, common.CodeServerError, err
	}

	kbFolder, err := s.ensureKBFolder(ctx, kb, tenantID)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	// Virtual documents have no file type to route on, so they keep the
	// dataset parser (mirrors Python's _upload_empty_document).
	doc := s.newDatasetDocument(kb, tenantID, name, "", "virtual", kb.ParserID, kb.ParserConfig, "local", 0, nil)
	if err = s.InsertDocument(doc); err != nil {
		return nil, common.CodeServerError, err
	}
	if err = s.addFileFromKB(ctx, doc, kbFolder.ID, kb.TenantID); err != nil {
		return nil, common.CodeServerError, s.rollbackAddFileFromKBError(ctx, doc, kb.ID, err)
	}
	return docToRawMap(doc), common.CodeSuccess, nil
}

// ensureKBFolder resolves (creating as needed) the per-dataset file-manager
// folder: root -> .knowledgebase -> <dataset name>. Mirrors Python
// get_root_folder + get_kb_folder + new_a_file_from_kb.
func (s *DocumentService) ensureKBFolder(ctx context.Context, kb *entity.Knowledgebase, tenantID string) (*entity.File, error) {
	root, err := s.fileDAO.GetRootFolder(ctx, dao.DB, tenantID)
	if err != nil {
		return nil, err
	}
	kbRoot, err := s.newAFileFromKB(ctx, tenantID, knowledgebaseFolderName, root.ID)
	if err != nil {
		return nil, err
	}
	return s.newAFileFromKB(ctx, kb.TenantID, kb.Name, kbRoot.ID)
}

// newAFileFromKB returns the existing folder named name under parentID, or
// creates it. Mirrors Python FileService.new_a_file_from_kb.
func (s *DocumentService) newAFileFromKB(ctx context.Context, tenantID, name, parentID string) (*entity.File, error) {
	existingFolders, err := s.fileDAO.Query(ctx, dao.DB, name, parentID, tenantID)
	if err != nil {
		return nil, err
	}
	for _, f := range existingFolders {
		if f.TenantID == tenantID {
			return f, nil
		}
	}
	loc := ""
	folder := &entity.File{
		ID:         common.GenerateToken(),
		ParentID:   parentID,
		TenantID:   tenantID,
		CreatedBy:  tenantID,
		Name:       name,
		Type:       "folder",
		Size:       0,
		Location:   &loc,
		SourceType: string(entity.FileSourceKnowledgebase),
	}
	if err := s.fileDAO.Create(ctx, dao.DB, folder); err != nil {
		return nil, err
	}
	return folder, nil
}

// addFileFromKB links a document into the file manager: a File row under the
// dataset folder plus a file2document mapping. Mirrors Python
// FileService.add_file_from_kb (idempotent on the document mapping).
func (s *DocumentService) addFileFromKB(ctx context.Context, doc *entity.Document, kbFolderID, tenantID string) error {
	if existing, err := s.file2DocumentDAO.GetByDocumentID(ctx, dao.DB, doc.ID); err == nil && len(existing) > 0 {
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
	fileID := common.GenerateToken()
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
	if err := s.fileDAO.Create(ctx, dao.DB, file); err != nil {
		return err
	}
	docID := doc.ID
	if err := s.file2DocumentDAO.Create(ctx, dao.DB, &entity.File2Document{
		ID:         common.GenerateToken(),
		FileID:     &fileID,
		DocumentID: &docID,
	}); err != nil {
		_ = s.fileDAO.Delete(ctx, dao.DB, fileID)
		return err
	}
	return nil
}

func (s *DocumentService) UploadWebDocument(ctx context.Context, kb *entity.Knowledgebase, tenantID, name, url string) (map[string]interface{}, common.ErrorCode, error) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, common.CodeServerError, fmt.Errorf("storage not initialized")
	}

	kbFolder, err := s.ensureKBFolder(ctx, kb, tenantID)
	if err != nil {
		return nil, common.CodeServerError, err
	}

	blob, headers, _, err := utility.FetchRemoteFileSafely(ctx, url, maxUploadDocSize)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	contentType := ""
	if headers != nil {
		contentType = headers.Get("Content-Type")
	}
	filename := normalizeWebDocumentName(name, contentType, blob)
	filename, _, blob = utility.NormalizeUploadInfoContent(filename, contentType, blob)
	filename, err = common.UniqueFileName(filename, 255, func(candidate string) (bool, error) {
		return s.documentDAO.NameExistsInKB(ctx, dao.DB, kb.ID, candidate)
	})
	if err != nil {
		return nil, common.CodeServerError, err
	}

	filetype := utility.FilenameType(filename)
	if filetype == utility.FileTypeOTHER {
		return nil, common.CodeDataError, fmt.Errorf("this type of file has not been supported yet")
	}

	location := filename
	for storageImpl.ObjExist(ctx, kb.ID, location) {
		location += "_"
	}
	if err = storageImpl.Put(ctx, kb.ID, location, blob); err != nil {
		return nil, common.CodeServerError, err
	}

	parserID, parserConfig := resolveDocumentParser(ctx, kb, filename, filetype, kb.ParserConfig)
	doc := s.newDatasetDocument(kb, tenantID, filename, location, string(filetype), parserID, parserConfig, "web", int64(len(blob)), blob)
	if err = s.InsertDocument(doc); err != nil {
		rmErr := removeObjectBestEffort(ctx, storageImpl, kb.ID, location)
		if rmErr != nil {
			common.Warn(fmt.Sprintf("UploadWebDocument: failed to insert document, remove blob %s/%s: %v", kb.ID, location, rmErr))
		}
		return nil, common.CodeServerError, err
	}
	if err = s.addFileFromKB(ctx, doc, kbFolder.ID, kb.TenantID); err != nil {
		err = s.rollbackAddFileFromKBError(ctx, doc, kb.ID, err)
		rmErr := removeObjectBestEffort(ctx, storageImpl, kb.ID, location)
		if rmErr != nil {
			common.Warn(fmt.Sprintf("UploadWebDocument: failed to add file from knowledge base, remove blob %s/%s: %v", kb.ID, location, rmErr))
		}
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

// newDatasetDocument builds a Document row for an upload, deriving suffix and
// content hash. parserID/parserConfig are resolved by the caller (see
// resolveDocumentParser); blob may be nil for the empty/virtual document.
func (s *DocumentService) newDatasetDocument(kb *entity.Knowledgebase, tenantID, filename, location, filetype, parserID string, parserConfig entity.JSONMap, src string, size int64, blob []byte) *entity.Document {
	docID := common.GenerateToken()
	parserConfig = cloneParserConfigForDocument(parserConfig)
	status := "1"
	suffix := ""
	if i := strings.LastIndex(filename, "."); i >= 0 {
		suffix = filename[i+1:]
	}
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
		Status:       &status,
	}
	if blob != nil {
		hash := contentHashHex(blob)
		doc.ContentHash = &hash
	}
	return doc
}

// cloneParserConfigForDocument copies a dataset's parser_config for a new
// document row. A configuration loaded from the database hands out live nested
// maps, so a component entry written for one document would otherwise be
// written for the dataset and every sibling document too.
func cloneParserConfigForDocument(config entity.JSONMap) entity.JSONMap {
	return entity.JSONMap(common.DeepMergeMaps(config, nil))
}

// docToRawMap serialises a freshly created Document into the raw key shape the
// handler remaps (chunk_num→chunk_count, kb_id→dataset_id).
func docToRawMap(doc *entity.Document) map[string]interface{} {
	m := map[string]interface{}{
		"id":               doc.ID,
		"kb_id":            doc.KbID,
		"parser_id":        doc.ParserID,
		"parser_config":    map[string]interface{}(doc.ParserConfig),
		"created_by":       doc.CreatedBy,
		"type":             doc.Type,
		"source_type":      doc.SourceType,
		"size":             doc.Size,
		"chunk_num":        doc.ChunkNum,
		"token_num":        doc.TokenNum,
		"suffix":           doc.Suffix,
		"ingestion_status": "UNSTART",
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
