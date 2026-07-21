package document

import (
	"fmt"
	"path/filepath"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
)

// GetDocumentImage retrieves an image object from storage.
func (s *DocumentService) GetDocumentImage(imageID string) ([]byte, error) {
	parts := strings.SplitN(imageID, "-", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("Image not found.")
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	return storageImpl.Get(parts[0], parts[1])
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
func (s *DocumentService) GetDocumentArtifact(filename, userID string) (*ArtifactResponse, error) {
	basename := filepath.Base(filename)
	if basename != filename || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return nil, ErrArtifactInvalidFilename
	}

	ext := strings.ToLower(filepath.Ext(basename))
	contentType, ok := artifactContentTypes[ext]
	if !ok {
		return nil, ErrArtifactInvalidFileType
	}

	if !s.sandboxArtifactAccessible(basename, userID) {
		// Same error as "object does not exist" to avoid leaking
		// whether the artifact exists for a different user/agent.
		return nil, ErrArtifactNotFound
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	bucket := sandboxArtifactBucket()
	if !storageImpl.ObjExist(bucket, basename) {
		return nil, ErrArtifactNotFound
	}

	data, err := storageImpl.Get(bucket, basename)
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
func (s *DocumentService) sandboxArtifactDialogIDsForUser(filename, userID string) []string {
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
	rows, err := dao.DB.Model(&entity.API4Conversation{}).
		Select("dialog_id").
		Where("user_id = ? OR exp_user_id = ?", userID, userID).
		Where(`message LIKE ? ESCAPE '!' OR message LIKE ? ESCAPE '!'`,
			filenamePattern, artifactRefPattern).
		Distinct("dialog_id").
		Rows()
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err == nil && d != "" {
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
func (s *DocumentService) sandboxArtifactAccessible(filename, userID string) bool {
	if userID == "" {
		return false
	}
	// Fetch the caller's tenant list once; passing it into
	// canvasDAO.Accessible ensures the team-permission branch only
	// matches canvases the caller can actually see. An empty list
	// (callers without tenant data) is safe — it effectively disables
	// the team branch, so the only matches are canvases the caller
	// directly owns.
	tenantIDs, terr := dao.NewUserTenantDAO().GetTenantIDsByUserID(userID)
	if terr != nil {
		tenantIDs = nil
	}
	for _, dialogID := range s.sandboxArtifactDialogIDsForUser(filename, userID) {
		if s.canvasDAO.Accessible(dialogID, userID, tenantIDs) {
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

func (s *DocumentService) GetDocumentPreview(docID string) (*DocumentPreview, error) {
	doc, err := s.documentDAO.GetByID(docID)
	if err != nil {
		return nil, err
	}

	bucket, name, err := s.GetDocumentStorageAddress(doc)
	if err != nil {
		return nil, err
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	data, err := storageImpl.Get(bucket, name)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, ErrArtifactNotFound
	}

	fileName := ""
	if doc.Name != nil {
		fileName = *doc.Name
	}

	ext := utility.GetFileExtension(fileName)
	contentType := utility.GetContentType(ext, doc.Type)

	return &DocumentPreview{
		Data:        data,
		ContentType: contentType,
		FileName:    fileName,
	}, nil
}
