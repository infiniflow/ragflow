package document

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/parser/parser"
	"ragflow/internal/parser/tableutil"
	"ragflow/internal/permission"
	"ragflow/internal/service"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
)

var ErrDocumentImageNotFound = errors.New("document image not found")

// GetDocumentImage serves legacy KB-scoped image IDs only when an indexed
// chunk proves which accessible document owns the exact image ID.
func (s *DocumentService) GetDocumentImage(ctx context.Context, imageID string) ([]byte, error) {
	bucket, _, ok := strings.Cut(imageID, "-")
	if !ok || len(bucket) != 32 {
		return nil, ErrDocumentImageNotFound
	}
	if decoded, err := hex.DecodeString(bucket); err != nil || len(decoded) != 16 {
		return nil, ErrDocumentImageNotFound
	}
	docID, err := s.imageDocumentID(ctx, bucket, imageID, "")
	if err != nil || docID == "" {
		return nil, ErrDocumentImageNotFound
	}
	return s.GetDocumentImageForDocument(ctx, docID, imageID)
}

// GetDocumentImageForDocument retrieves an image after proving that the exact
// composite image ID belongs to an indexed chunk of an accessible document.
func (s *DocumentService) GetDocumentImageForDocument(ctx context.Context, docID, imageID string) ([]byte, error) {
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, docID)
	if err != nil || doc == nil || service.CheckDocumentAccess(ctx, doc.ID, doc.KbID, permission.OperationRead) != nil {
		return nil, ErrDocumentImageNotFound
	}
	bucket, objectKey, ok := strings.Cut(imageID, "-")
	if !ok || bucket == "" || objectKey == "" {
		return nil, ErrDocumentImageNotFound
	}
	ownerID, err := s.imageDocumentID(ctx, doc.KbID, imageID, doc.ID)
	if err != nil || ownerID != doc.ID {
		return nil, ErrDocumentImageNotFound
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	data, err := storageImpl.Get(ctx, bucket, objectKey)
	if err != nil || len(data) == 0 {
		return nil, ErrDocumentImageNotFound
	}
	return data, nil
}

// GetDocumentThumbnail resolves the storage key exclusively from authorized
// document metadata.
func (s *DocumentService) GetDocumentThumbnail(ctx context.Context, docID string) ([]byte, error) {
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, docID)
	if err != nil || doc == nil || service.CheckDocumentAccess(ctx, doc.ID, doc.KbID, permission.OperationRead) != nil || doc.Thumbnail == nil || *doc.Thumbnail == "" || strings.HasPrefix(*doc.Thumbnail, imgBase64Prefix) {
		return nil, ErrDocumentImageNotFound
	}
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}
	data, err := storageImpl.Get(ctx, doc.KbID, *doc.Thumbnail)
	if err != nil || len(data) == 0 {
		return nil, ErrDocumentImageNotFound
	}
	return data, nil
}

func (s *DocumentService) imageDocumentID(ctx context.Context, kbID, imageID, docID string) (string, error) {
	if s.docEngine == nil {
		return "", ErrDocumentImageNotFound
	}
	tenantID, err := dao.GetTenantIDByKBID(ctx, dao.DB, kbID)
	if err != nil {
		return "", ErrDocumentImageNotFound
	}
	filter := map[string]any{"img_id": imageID}
	if docID != "" {
		filter["doc_id"] = docID
	}
	result, err := s.docEngine.Search(ctx, &enginetypes.SearchRequest{
		IndexNames:   []string{service.IndexName(tenantID)},
		KbIDs:        []string{kbID},
		Limit:        1,
		SelectFields: []string{"doc_id", "img_id"},
		Filter:       filter,
	})
	if err != nil || result == nil {
		return "", ErrDocumentImageNotFound
	}
	for _, chunk := range result.Chunks {
		foundDocID, _ := chunk["doc_id"].(string)
		foundImageID, _ := chunk["img_id"].(string)
		if foundDocID != "" && foundImageID == imageID && (docID == "" || foundDocID == docID) {
			return foundDocID, nil
		}
	}
	return "", ErrDocumentImageNotFound
}

// GetDocumentArtifact retrieves a sandbox artifact from object storage.
//
// The request identity scopes the lookup: a CodeExec sandbox artifact is only
// returned when the caller owns (or has team access to) at least
// one agent session whose `message` references this filename (or
// its `documents/artifact/<name>` URL form). The authorization
// gate runs BEFORE the storage read so a probe of an unknown
// filename cannot distinguish "you cannot see it" from "it
// exists" — both return ErrArtifactNotFound. Mirrors PR #16169.
func (s *DocumentService) GetDocumentArtifact(ctx context.Context, filename string) (*ArtifactResponse, error) {
	basename := filepath.Base(filename)
	if basename != filename || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return nil, ErrArtifactInvalidFilename
	}

	ext := strings.ToLower(filepath.Ext(basename))
	contentType, ok := common.SandboxArtifactContentTypes[ext]
	if !ok {
		return nil, ErrArtifactInvalidFileType
	}

	if !s.sandboxArtifactAccessible(ctx, basename) {
		// Same error as "object does not exist" to avoid leaking
		// whether the artifact exists for a different user/agent.
		return nil, ErrArtifactNotFound
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	bucket := common.SandboxArtifactBucket()
	if !storageImpl.ObjExist(ctx, bucket, basename) {
		return nil, ErrArtifactNotFound
	}

	data, err := storageImpl.Get(ctx, bucket, basename)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, ErrArtifactNotFound
	}

	return &ArtifactResponse{
		Data:            data,
		ContentType:     contentType,
		SafeFilename:    sanitizeArtifactFilename(basename),
		ForceAttachment: shouldForceArtifactAttachment(ext, contentType),
	}, nil
}

// sandboxArtifactDialogIDs returns the distinct agent (canvas) dialog IDs for
// sessions owned by the request identity whose message references filename.
// Artifact URLs appear as either a bare filename or the
// documents/artifact/<name> form.
func (s *DocumentService) sandboxArtifactDialogIDs(ctx context.Context, filename string) []string {
	userID := permission.UserIDFromContext(ctx)
	if filename == "" || userID == "" {
		return nil
	}

	// Escape SQL LIKE wildcards before building the pattern. Without this, a
	// filename such as %.png could match another user's referenced artifact.
	// Use ! as the escape character because backslash handling differs between
	// database drivers, while ! is parsed consistently by SQLite, MySQL, and
	// Postgres.
	filenameSafe := escapeSQLLikePattern(filename)
	artifactRefSafe := escapeSQLLikePattern("documents/artifact/" + filename)
	filenamePattern := "%" + filenameSafe + "%"
	artifactRefPattern := "%" + artifactRefSafe + "%"
	dialogIDs := make(map[string]struct{})
	rows, err := dao.DB.WithContext(ctx).Table("api_4_conversation AS c").
		Select("c.dialog_id").
		Joins("JOIN api_4_conversation_message AS cm ON cm.conversation_id = c.id").
		Where("c.user_id = ? OR c.exp_user_id = ?", userID, userID).
		Where("cm.content LIKE ? ESCAPE '!' OR cm.content LIKE ? ESCAPE '!'",
			filenamePattern, artifactRefPattern).
		Distinct("c.dialog_id").Rows()
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var dialogID string
		if err = rows.Scan(&dialogID); err == nil && dialogID != "" {
			dialogIDs[dialogID] = struct{}{}
		}
	}
	out := make([]string, 0, len(dialogIDs))
	for dialogID := range dialogIDs {
		out = append(out, dialogID)
	}
	return out
}

func (s *DocumentService) sandboxArtifactAccessible(ctx context.Context, filename string) bool {
	for _, dialogID := range s.sandboxArtifactDialogIDs(ctx, filename) {
		if permission.NewDatabaseChecker(dao.DB).CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindCanvas, ID: dialogID}, permission.OperationRead) == nil {
			return true
		}
	}
	return false
}

// sanitizeArtifactFilename scrubs characters that are unsafe inside a storage
// object key for sandbox artifacts. It intentionally only replaces the
// artifact-specific unsafe set (artifactUnsafeFilenameChars) and does NOT strip
// directory components, reject reserved device names, or bound length — those
// concerns belong to the general-purpose sanitizeFilename used for uploaded /
// URL-derived filenames. The two are deliberately separate because their
// safety rules differ; do not merge them.
func sanitizeArtifactFilename(filename string) string {
	return artifactUnsafeFilenameChars.ReplaceAllString(filename, "_")
}

func shouldForceArtifactAttachment(ext, contentType string) bool {
	if _, ok := artifactForceAttachmentExtensions[strings.ToLower(ext)]; ok {
		return true
	}
	_, ok := artifactForceAttachmentContentTypes[strings.ToLower(contentType)]
	return ok
}

func (s *DocumentService) GetDocumentPreview(ctx context.Context, docID string) (*DocumentPreview, error) {
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, docID)
	if err != nil || doc == nil {
		return nil, ErrPreviewDocumentNotFound
	}

	// Keep access denial indistinguishable from a missing document so callers
	// cannot probe document IDs.
	if service.CheckDocumentAccess(ctx, doc.ID, doc.KbID, permission.OperationRead) != nil {
		return nil, ErrPreviewDocumentNotFound
	}

	bucket, name, err := s.GetDocumentStorageAddress(ctx, doc)
	if err != nil {
		return nil, fmt.Errorf("resolve storage address for document %s: %w", docID, err)
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	data, err := storageImpl.Get(ctx, bucket, name)
	if err != nil {
		return nil, fmt.Errorf("read document object %s/%s: %w", bucket, name, err)
	}
	if len(data) == 0 {
		return nil, ErrPreviewFileEmpty
	}

	fileName := ""
	if doc.Name != nil {
		fileName = *doc.Name
	}

	ext := utility.GetFileExtension(fileName)
	contentType := utility.GetContentType(ext, doc.Type)

	// Legacy .doc (OLE2) documents cannot be rendered by the
	// .docx-only web previewer; serve extracted plain text instead.
	if ext == "doc" {
		if text, cerr := extractDOCPreviewText(ctx, fileName, data); cerr == nil {
			data = []byte(text)
			contentType = "text/plain; charset=utf-8"
		}
	}

	return &DocumentPreview{
		Data:        data,
		ContentType: contentType,
		FileName:    fileName,
	}, nil
}

// extractDOCPreviewText extracts plain text from a legacy .doc
// document via the office_oxide-backed DOCParser. The parser only emits JSON
// payloads, so the preview text is flattened from the JSON items.
func extractDOCPreviewText(ctx context.Context, filename string, data []byte) (string, error) {
	p, actual, err := parser.GetTextOnlyParser(utility.FileTypeDOC, data)
	if err != nil {
		return "", err
	}
	if actual != utility.FileTypeDOC {
		common.Warn(fmt.Sprintf("file %q: requested format doc; detected %s; continuing with the %s parser", filename, actual, actual))
	}
	res := p.ParseWithResult(ctx, filename, data)
	if res.Err != nil {
		return "", res.Err
	}
	return docPreviewText(res)
}

// docPreviewText flattens parser items for the legacy .doc preview. Table
// items carry only TableData, so render them instead of dropping the table
// content from the preview.
func docPreviewText(res parser.ParseResult) (string, error) {
	if len(res.JSON) == 0 {
		return "", fmt.Errorf("doc preview: no content extracted")
	}
	parts := make([]string, 0, len(res.JSON))
	for _, item := range res.JSON {
		m := item.ToMap()
		if text, ok := m["text"].(string); ok && text != "" {
			parts = append(parts, text)
			continue
		}
		if table, ok := m["table"].(*entity.TableData); ok && table != nil {
			parts = append(parts, tableutil.RenderTableHTML(table))
		}
	}
	return strings.Join(parts, "\n"), nil
}
