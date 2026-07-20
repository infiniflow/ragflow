package document

import (
	"fmt"
	"strings"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// escapeSQLLikePattern escapes the SQL LIKE wildcards ('%', '_') and
// the escape character itself ('!') so a literal user-supplied
// filename can be safely interpolated into a `LIKE ? ESCAPE '!'`
// pattern. Without this, "%.png" would match any string ending in
// ".png" and "_" would match a single character — bypassing the
// filename-specific authorization check. PR review round 5, Major #8.
func escapeSQLLikePattern(s string) string {
	r := strings.NewReplacer(`!`, `!!`, `%`, `!%`, `_`, `!_`)
	return r.Replace(s)
}

// ListDocuments list documents
func (s *DocumentService) ListDocuments(page, pageSize int) ([]*DocumentResponse, int64, error) {
	offset := (page - 1) * pageSize
	documents, total, err := s.documentDAO.List(offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*DocumentResponse, len(documents))
	for i, doc := range documents {
		responses[i] = s.toResponse(doc)
	}

	return responses, total, nil
}

func (s *DocumentService) GetThumbnails(userID string, docIDs []string) (map[string]string, error) {
	if len(docIDs) == 0 {
		return map[string]string{}, nil
	}

	tenantIDs := []string{userID}
	if userID != "" {
		ids, err := dao.NewUserTenantDAO().GetTenantIDsByUserID(userID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch user tenants: %w", err)
		}
		tenantIDs = append(tenantIDs, ids...)
	}

	documents, err := s.documentDAO.GetByIDsAndTenantIDs(docIDs, tenantIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch document thumbnails: %w", err)
	}

	result := make(map[string]string, len(documents))
	for _, document := range documents {
		if document == nil {
			continue
		}

		thumbnail := ""
		if document.Thumbnail != nil && *document.Thumbnail != "" {
			if strings.HasPrefix(*document.Thumbnail, imgBase64Prefix) {
				thumbnail = *document.Thumbnail
			} else {
				thumbnail = fmt.Sprintf(
					"/api/v1/documents/images/%s-%s",
					document.KbID,
					*document.Thumbnail,
				)
			}
		}

		result[document.ID] = thumbnail
	}

	return result, nil
}

// ListDocumentsByDatasetID list documents by knowledge base ID
func (s *DocumentService) ListDocumentsByDatasetID(kbID, keywords string, page, pageSize int) ([]*entity.DocumentListItem, int64, error) {
	return s.ListDocumentsByDatasetIDWithOptions(dao.DocumentListOptions{
		KbID:     kbID,
		Keywords: keywords,
		OrderBy:  "create_time",
		Desc:     true,
	}, page, pageSize)
}

// ListDocumentsByDatasetIDWithOptions lists documents by knowledge base ID with filters.
func (s *DocumentService) ListDocumentsByDatasetIDWithOptions(opts dao.DocumentListOptions, page, pageSize int) ([]*entity.DocumentListItem, int64, error) {
	opts.Offset = (page - 1) * pageSize
	opts.Limit = pageSize
	if opts.OrderBy == "" {
		opts.OrderBy = "create_time"
	}
	documents, total, err := s.documentDAO.ListByKBIDWithOptions(opts)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*entity.DocumentListItem, len(documents))
	for i, doc := range documents {
		responses[i] = doc
	}

	return responses, total, nil
}

// GetDocumentFiltersByDatasetID returns aggregate filter values for documents in a dataset.
func (s *DocumentService) GetDocumentFiltersByDatasetID(opts dao.DocumentListOptions) (map[string]interface{}, int64, error) {
	filters, total, err := s.documentDAO.GetFilterByKBID(opts)
	if err != nil {
		return nil, 0, err
	}
	docIDs, err := s.documentDAO.ListIDsByKBIDWithOptions(opts)
	if err != nil {
		return nil, 0, err
	}
	metadataFilter, err := s.getDocumentMetadataFilter(opts.KbID, docIDs)
	if err != nil {
		return nil, 0, err
	}
	filters["metadata"] = metadataFilter
	return filters, total, nil
}

func (s *DocumentService) getDocumentMetadataFilter(kbID string, docIDs []string) (map[string]interface{}, error) {
	metadataByKey, err := s.GetMetadataByKBs([]string{kbID})
	if err != nil {
		return nil, err
	}
	candidateSet := make(map[string]bool, len(docIDs))
	for _, docID := range docIDs {
		candidateSet[docID] = true
	}

	metadataCounter := map[string]interface{}{}
	docIDsWithMetadata := map[string]bool{}
	for key, rawValues := range metadataByKey {
		values, ok := rawValues.(map[string][]string)
		if !ok {
			continue
		}
		valueCounter := map[string]int64{}
		for value, valueDocIDs := range values {
			for _, docID := range valueDocIDs {
				if !candidateSet[docID] {
					continue
				}
				valueCounter[value]++
				docIDsWithMetadata[docID] = true
			}
		}
		if len(valueCounter) > 0 {
			metadataCounter[key] = valueCounter
		}
	}
	metadataCounter["empty_metadata"] = map[string]int64{"true": int64(len(docIDs) - len(docIDsWithMetadata))}
	return metadataCounter, nil
}

// ListDocumentIDsByDatasetIDWithOptions lists matching document IDs without pagination.
func (s *DocumentService) ListDocumentIDsByDatasetIDWithOptions(opts dao.DocumentListOptions) ([]string, error) {
	return s.documentDAO.ListIDsByKBIDWithOptions(opts)
}

// GetDocumentsByAuthorID get documents by author ID
func (s *DocumentService) GetDocumentsByAuthorID(authorID, page, pageSize int) ([]*DocumentResponse, int64, error) {
	offset := (page - 1) * pageSize
	documents, total, err := s.documentDAO.GetByAuthorID(fmt.Sprintf("%d", authorID), offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*DocumentResponse, len(documents))
	for i, doc := range documents {
		responses[i] = s.toResponse(doc)
	}

	return responses, total, nil
}

// toResponse convert model.Document to DocumentResponse
func (s *DocumentService) toResponse(doc *entity.Document) *DocumentResponse {
	createdAt := ""
	if doc.CreateTime != nil {
		// Check if timestamp is in milliseconds (13 digits) or seconds (10 digits)
		var ts int64
		if *doc.CreateTime > 1000000000000 {
			// Milliseconds - convert to seconds
			ts = *doc.CreateTime / 1000
		} else {
			ts = *doc.CreateTime
		}
		createdAt = time.Unix(ts, 0).Format("2006-01-02 15:04:05")
	}
	updatedAt := ""
	if doc.UpdateTime != nil {
		// Accept both historical second-based values and current millisecond-based values.
		ts := *doc.UpdateTime
		if ts > 1000000000000 {
			ts /= 1000
		}
		updatedAt = time.Unix(ts, 0).Format("2006-01-02 15:04:05")
	}
	return &DocumentResponse{
		ID:              doc.ID,
		Name:            doc.Name,
		KbID:            doc.KbID,
		ParserID:        doc.ParserID,
		PipelineID:      doc.PipelineID,
		Type:            doc.Type,
		SourceType:      doc.SourceType,
		CreatedBy:       doc.CreatedBy,
		Location:        doc.Location,
		Size:            doc.Size,
		TokenNum:        doc.TokenNum,
		ChunkNum:        doc.ChunkNum,
		Progress:        doc.Progress,
		ProgressMsg:     doc.ProgressMsg,
		ProcessDuration: doc.ProcessDuration,
		Suffix:          doc.Suffix,
		Run:             doc.Run,
		Status:          doc.Status,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}
}
