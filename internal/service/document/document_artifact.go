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
	"ragflow/internal/parser/parser"
	"ragflow/internal/service"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
)

var ErrDocumentImageNotFound = errors.New("document image not found")

// GetDocumentImage serves legacy KB-scoped image IDs only when an indexed
// chunk proves which accessible document owns the exact image ID.
func (s *DocumentService) GetDocumentImage(ctx context.Context, userID, imageID string) ([]byte, error) {
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
	return s.GetDocumentImageForDocument(ctx, userID, docID, imageID)
}

// GetDocumentImageForDocument retrieves an image after proving that the exact
// composite image ID belongs to an indexed chunk of an accessible document.
func (s *DocumentService) GetDocumentImageForDocument(ctx context.Context, userID, docID, imageID string) ([]byte, error) {
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, docID)
	if err != nil || doc == nil || !s.kbDAO.Accessible(ctx, dao.DB, doc.KbID, userID) {
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
func (s *DocumentService) GetDocumentThumbnail(ctx context.Context, userID, docID string) ([]byte, error) {
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, docID)
	if err != nil || doc == nil || !s.kbDAO.Accessible(ctx, dao.DB, doc.KbID, userID) || doc.Thumbnail == nil || *doc.Thumbnail == "" || strings.HasPrefix(*doc.Thumbnail, imgBase64Prefix) {
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
// userID scopes the lookup: a CodeExec sandbox artifact is only
// returned when the caller owns (or has team access to) at least
// one agent session whose `message` references this filename (or
// its `documents/artifact/<name>` URL form). The authorization
// gate runs BEFORE the storage read so a probe of an unknown
// filename cannot distinguish "you cannot see it" from "it
// exists" — both return ErrArtifactNotFound. Mirrors PR #16169.
func (s *DocumentService) GetDocumentArtifact(ctx context.Context, filename, userID string) (*ArtifactResponse, error) {
	basename := filepath.Base(filename)
	if basename != filename || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return nil, ErrArtifactInvalidFilename
	}

	ext := strings.ToLower(filepath.Ext(basename))
	contentType, ok := artifactContentTypes[ext]
	if !ok {
		return nil, ErrArtifactInvalidFileType
	}

	if !s.sandboxArtifactAccessible(ctx, basename, userID) {
		// Same error as "object does not exist" to avoid leaking
		// whether the artifact exists for a different user/agent.
		return nil, ErrArtifactNotFound
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	bucket := sandboxArtifactBucket()
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

// sandboxArtifactDialogIDsForUser returns the distinct agent
// (canvas) dialog_ids for sessions owned by userID whose
// `message` blob references filename. A CodeExec artifact URL
// appears in `message` as either a bare filename or the
// `documents/artifact/<name>` form, so the helper matches both.
//
// Implemented as a direct GORM query on the
// API4Conversation table — GORM's `Contains` maps to MySQL
// `LIKE '%...%'` which is fine here because the storage path is
// short and indexed lookup on (user_id, exp_user_id) keeps the
// scan narrow.
func (s *DocumentService) sandboxArtifactDialogIDsForUser(ctx context.Context, filename, userID string) []string {
	if filename == "" || userID == "" {
		return nil
	}
	// Escape SQL LIKE wildcards (%, _) before building the pattern.
	// Without escaping, a caller could submit a filename like
	// "%.png" or "_" and the LIKE query would match arbitrary
	// referenced artifacts in any user's conversation — letting the
	// caller pass the authorization check against one filename and
	// then GET another artifact by name (PR review round 5, Major #8).
	//
	// Escape character: '!'. We avoid '\\' because SQL string
	// literal parsing of '\\' is driver-specific (SQLite treats
	// it as a single backslash, MySQL treats it as one, Postgres
	// rejects the unterminated string) — '!' is a benign character
	// in real filenames (artifact names rarely contain '!') and
	// parses identically in every driver.
	filenameSafe := escapeSQLLikePattern(filename)
	artifactRefSafe := escapeSQLLikePattern("documents/artifact/" + filename)
	filenamePattern := "%" + filenameSafe + "%"
	artifactRefPattern := "%" + artifactRefSafe + "%"
	dialogIDs := make(map[string]struct{})
	rows, err := dao.DB.WithContext(ctx).Table("api_4_conversation AS c").
		Select("c.dialog_id").
		Joins("JOIN api_4_conversation_message AS cm ON cm.conversation_id = c.id").
		Where("c.user_id = ? OR c.exp_user_id = ?", userID, userID).
		Where(`cm.content LIKE ? ESCAPE '!' OR cm.content LIKE ? ESCAPE '!'`,
			filenamePattern, artifactRefPattern).
		Distinct("c.dialog_id").
		Rows()
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		if err = rows.Scan(&d); err == nil && d != "" {
			dialogIDs[d] = struct{}{}
		}
	}
	out := make([]string, 0, len(dialogIDs))
	for d := range dialogIDs {
		out = append(out, d)
	}
	return out
}

// sandboxArtifactAccessible reports whether userID may reach at
// least one agent canvas whose session references filename.
// Mirrors `UserCanvasService.accessible(dialog_id, user_id)` from
// the Python fix; on the Go side this is the same predicate as
// UserCanvasDAO.Accessible (owner or team permission, with the
// latter scoped to the caller's tenant membership — PR review
// round 5).
func (s *DocumentService) sandboxArtifactAccessible(ctx context.Context, filename, userID string) bool {
	if userID == "" {
		return false
	}
	// Fetch the caller's tenant list once; passing it into
	// canvasDAO.Accessible ensures the team-permission branch only
	// matches canvases the caller can actually see. An empty list
	// (callers without tenant data) is safe — it effectively disables
	// the team branch, so the only matches are canvases the caller
	// directly owns.
	tenantIDs, terr := dao.NewUserTenantDAO().GetTenantIDsByUserID(ctx, dao.DB, userID)
	if terr != nil {
		tenantIDs = nil
	}
	for _, dialogID := range s.sandboxArtifactDialogIDsForUser(ctx, filename, userID) {
		if s.canvasDAO.Accessible(ctx, dao.DB, dialogID, userID, tenantIDs) {
			return true
		}
	}
	return false
}

func sandboxArtifactBucket() string {
	if bucket := common.GetEnv(common.EnvSandboxArtifactBucket); bucket != "" {
		return bucket
	}
	return "sandbox-artifacts"
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

func (s *DocumentService) GetDocumentPreview(ctx context.Context, userID, docID string) (*DocumentPreview, error) {
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, docID)
	if err != nil || doc == nil {
		return nil, ErrPreviewDocumentNotFound
	}

	// Reuse KnowledgebaseDAO.Accessible — the exact rule the chunk list on
	// the same page uses — so the two panels can never disagree: the owning
	// tenant always, and tenant members only when the dataset's permission
	// is TEAM. A denial stays indistinguishable from a missing document so
	// an unauthorized caller cannot probe document IDs (mirrors Python
	// DocumentService.accessible in the preview path).
	if !s.kbDAO.Accessible(ctx, dao.DB, doc.KbID, userID) {
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
// document via the office_oxide-backed DOCParser.
func extractDOCPreviewText(ctx context.Context, filename string, data []byte) (string, error) {
	res := parser.NewDOCParser().ParseWithResult(ctx, filename, data)
	if res.Err != nil {
		return "", res.Err
	}
	return res.Text, nil
}
