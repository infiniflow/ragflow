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

package dao

import (
	"ragflow/internal/entity"
	"strings"

	"gorm.io/gorm"
)

// DocumentDAO document data access object
type DocumentDAO struct{}

// NewDocumentDAO create document DAO
func NewDocumentDAO() *DocumentDAO {
	return &DocumentDAO{}
}

// Create create document
func (dao *DocumentDAO) Create(document *entity.Document) error {
	return DB.Create(document).Error
}

// GetByID get document by ID
func (dao *DocumentDAO) GetByID(id string) (*entity.Document, error) {
	var document entity.Document
	err := DB.First(&document, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &document, nil
}

// GetByAuthorID get documents by author ID
func (dao *DocumentDAO) GetByAuthorID(authorID string, offset, limit int) ([]*entity.Document, int64, error) {
	var documents []*entity.Document
	var total int64

	query := DB.Model(&entity.Document{}).Where("created_by = ?", authorID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Preload("Author").Offset(offset).Limit(limit).Find(&documents).Error
	return documents, total, err
}

// Update update document
func (dao *DocumentDAO) Update(document *entity.Document) error {
	return DB.Save(document).Error
}

// UpdateByID updates document by ID with the given fields
func (dao *DocumentDAO) UpdateByID(id string, updates map[string]interface{}) error {
	return DB.Model(&entity.Document{}).Where("id = ?", id).Updates(updates).Error
}

// Delete hard-deletes document by ID. Returns rows affected.
func (dao *DocumentDAO) Delete(id string) (int64, error) {
	result := DB.Where("id = ?", id).Delete(&entity.Document{})
	return result.RowsAffected, result.Error
}

// List list documents
func (dao *DocumentDAO) List(offset, limit int) ([]*entity.Document, int64, error) {
	var documents []*entity.Document
	var total int64

	if err := DB.Model(&entity.Document{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := DB.Preload("Author").Offset(offset).Limit(limit).Find(&documents).Error
	return documents, total, err
}

// DocumentListOptions contains filters for listing documents in a dataset.
type DocumentListOptions struct {
	KbID               string
	Keywords           string
	RunStatuses        []string
	Types              []string
	Suffixes           []string
	Name               string
	DocIDs             []string
	DocIDFilterApplied bool
	CreateTimeFrom     int64
	CreateTimeTo       int64
	OrderBy            string
	Desc               bool
	Offset             int
	Limit              int
}

// ListByKBID list documents by knowledge base ID
func (dao *DocumentDAO) ListByKBID(kbID, keywords string, offset, limit int) ([]*entity.DocumentListItem, int64, error) {
	return dao.ListByKBIDWithOptions(DocumentListOptions{
		KbID:     kbID,
		Keywords: keywords,
		OrderBy:  "create_time",
		Desc:     true,
		Offset:   offset,
		Limit:    limit,
	})
}

// ListByKBIDWithOptions lists documents by knowledge base ID with filters.
func (dao *DocumentDAO) ListByKBIDWithOptions(opts DocumentListOptions) ([]*entity.DocumentListItem, int64, error) {
	var documents []*entity.DocumentListItem
	var total int64

	listQuery := DB.Table("document").
		Select(`document.*, user_canvas.title as pipeline_name, user.nickname`).
		Joins("JOIN file2document ON file2document.document_id = document.id").
		Joins("JOIN file ON file.id = file2document.file_id").
		Joins("LEFT JOIN user_canvas ON document.pipeline_id = user_canvas.id").
		Joins("LEFT JOIN user ON document.created_by = user.id")

	listQuery = applyDocumentListFilters(listQuery, opts, true)
	countQuery := applyDocumentListFilters(DB.Model(&entity.Document{}), opts, false)

	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	orderBy := documentListOrderColumn(opts.OrderBy)
	if opts.Desc {
		orderBy += " DESC"
	} else {
		orderBy += " ASC"
	}

	err := listQuery.
		Order(orderBy).
		Offset(opts.Offset).
		Limit(opts.Limit).
		Scan(&documents).Error
	return documents, total, err
}

// GetFilterByKBID returns aggregate filter counts for documents in a dataset.
func (dao *DocumentDAO) GetFilterByKBID(opts DocumentListOptions) (map[string]interface{}, int64, error) {
	var rows []struct {
		ID     string  `gorm:"column:id"`
		Run    *string `gorm:"column:run"`
		Suffix string  `gorm:"column:suffix"`
	}

	query := DB.Table("document").
		Select("document.id, document.run, document.suffix").
		Joins("JOIN file2document ON file2document.document_id = document.id").
		Joins("JOIN file ON file.id = file2document.file_id")
	query = applyDocumentListFilters(query, opts, true)

	if err := query.Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	suffixCounter := map[string]int64{}
	runStatusCounter := map[string]int64{}
	for _, row := range rows {
		if row.Suffix != "" {
			suffixCounter[row.Suffix]++
		}
		if row.Run != nil {
			runStatusCounter[*row.Run]++
		}
	}

	return map[string]interface{}{
		"suffix":     suffixCounter,
		"run_status": runStatusCounter,
		"metadata":   map[string]interface{}{},
	}, int64(len(rows)), nil
}

// ListIDsByKBIDWithOptions lists matching document IDs without pagination.
func (dao *DocumentDAO) ListIDsByKBIDWithOptions(opts DocumentListOptions) ([]string, error) {
	var ids []string
	query := DB.Table("document").
		Select("document.id").
		Joins("JOIN file2document ON file2document.document_id = document.id").
		Joins("JOIN file ON file.id = file2document.file_id")
	query = applyDocumentListFilters(query, opts, true)
	if err := query.Scan(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func applyDocumentListFilters(query *gorm.DB, opts DocumentListOptions, qualified bool) *gorm.DB {
	column := func(name string) string {
		if qualified {
			return "document." + name
		}
		return name
	}

	query = query.Where(column("kb_id")+" = ?", opts.KbID)
	if strings.TrimSpace(opts.Keywords) != "" {
		query = query.Where("LOWER("+column("name")+") LIKE ?", "%"+strings.ToLower(strings.TrimSpace(opts.Keywords))+"%")
	}
	if len(opts.RunStatuses) > 0 {
		query = query.Where(column("run")+" IN ?", opts.RunStatuses)
	}
	if len(opts.Types) > 0 {
		query = query.Where(column("type")+" IN ?", opts.Types)
	}
	if len(opts.Suffixes) > 0 {
		query = query.Where(column("suffix")+" IN ?", opts.Suffixes)
	}
	if opts.Name != "" {
		query = query.Where(column("name")+" = ?", opts.Name)
	}
	if opts.DocIDFilterApplied {
		if len(opts.DocIDs) == 0 {
			query = query.Where("1 = 0")
		} else {
			query = query.Where(column("id")+" IN ?", opts.DocIDs)
		}
	}
	if opts.CreateTimeFrom > 0 {
		query = query.Where(column("create_time")+" >= ?", opts.CreateTimeFrom)
	}
	if opts.CreateTimeTo > 0 {
		query = query.Where(column("create_time")+" <= ?", opts.CreateTimeTo)
	}
	return query
}

func documentListOrderColumn(orderBy string) string {
	switch orderBy {
	case "update_time":
		return "document.update_time"
	case "name":
		return "document.name"
	case "size":
		return "document.size"
	case "type":
		return "document.type"
	case "run":
		return "document.run"
	default:
		return "document.create_time"
	}
}

// GetByKBID retrieves all documents in a knowledge base ordered by create time.
func (dao *DocumentDAO) GetByKBID(kbID string) ([]*entity.Document, int64, error) {
	var documents []*entity.Document
	var total int64

	query := DB.Model(&entity.Document{}).Where("kb_id = ?", kbID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("create_time ASC").Find(&documents).Error
	return documents, total, err
}

// GetChunkingConfig returns the document, dataset, and tenant fields used to
// build a parsing task digest, mirroring DocumentService.get_chunking_config.
func (dao *DocumentDAO) GetChunkingConfig(docID string) (map[string]interface{}, error) {
	var row struct {
		ID           string         `gorm:"column:id"`
		KbID         string         `gorm:"column:kb_id"`
		ParserID     string         `gorm:"column:parser_id"`
		ParserConfig entity.JSONMap `gorm:"column:parser_config;type:longtext"`
		Size         int64          `gorm:"column:size"`
		ContentHash  *string        `gorm:"column:content_hash"`
		Language     *string        `gorm:"column:language"`
		EmbdID       string         `gorm:"column:embd_id"`
		TenantID     string         `gorm:"column:tenant_id"`
		Img2TxtID    string         `gorm:"column:img2txt_id"`
		ASRID        string         `gorm:"column:asr_id"`
		LLMID        string         `gorm:"column:llm_id"`
	}

	err := DB.Table("document").
		Select(`
			document.id,
			document.kb_id,
			document.parser_id,
			document.parser_config,
			document.size,
			document.content_hash,
			knowledgebase.language,
			knowledgebase.embd_id,
			tenant.id AS tenant_id,
			tenant.img2txt_id,
			tenant.asr_id,
			tenant.llm_id
		`).
		Joins("JOIN knowledgebase ON document.kb_id = knowledgebase.id").
		Joins("JOIN tenant ON knowledgebase.tenant_id = tenant.id").
		Where("document.id = ?", docID).
		Take(&row).Error
	if err != nil {
		return nil, err
	}

	config := map[string]interface{}{
		"id":            row.ID,
		"kb_id":         row.KbID,
		"parser_id":     row.ParserID,
		"parser_config": row.ParserConfig,
		"size":          row.Size,
		"embd_id":       row.EmbdID,
		"tenant_id":     row.TenantID,
		"img2txt_id":    row.Img2TxtID,
		"asr_id":        row.ASRID,
		"llm_id":        row.LLMID,
	}
	if row.ContentHash != nil {
		config["content_hash"] = *row.ContentHash
	} else {
		config["content_hash"] = nil
	}
	if row.Language != nil {
		config["language"] = *row.Language
	} else {
		config["language"] = nil
	}
	return config, nil
}

// DeleteByTenantID deletes all documents by tenant ID (hard delete)
func (dao *DocumentDAO) DeleteByTenantID(tenantID string) (int64, error) {
	result := DB.Unscoped().Where("tenant_id = ?", tenantID).Delete(&entity.Document{})
	return result.RowsAffected, result.Error
}

// GetAllDocIDsByKBIDs gets all document IDs by knowledge base IDs
func (dao *DocumentDAO) GetAllDocIDsByKBIDs(kbIDs []string) ([]map[string]string, error) {
	var docs []struct {
		ID   string `gorm:"column:id"`
		KbID string `gorm:"column:kb_id"`
	}
	err := DB.Model(&entity.Document{}).Select("id, kb_id").Where("kb_id IN ?", kbIDs).Find(&docs).Error
	if err != nil {
		return nil, err
	}

	result := make([]map[string]string, len(docs))
	for i, doc := range docs {
		result[i] = map[string]string{"id": doc.ID, "kb_id": doc.KbID}
	}
	return result, nil
}

// GetByIDs retrieves documents by multiple IDs
func (dao *DocumentDAO) GetByIDs(ids []string) ([]*entity.Document, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var documents []*entity.Document
	err := DB.Where("id IN ?", ids).Find(&documents).Error
	if err != nil {
		return nil, err
	}
	return documents, nil
}

// GetByIDsAndTenantIDs retrieves documents by IDs scoped to knowledgebase owners.
func (dao *DocumentDAO) GetByIDsAndTenantIDs(ids, tenantIDs []string) ([]*entity.Document, error) {
	if len(ids) == 0 || len(tenantIDs) == 0 {
		return nil, nil
	}
	var documents []*entity.Document
	err := DB.Model(&entity.Document{}).
		Joins("JOIN knowledgebase ON document.kb_id = knowledgebase.id").
		Where("document.id IN ? AND knowledgebase.tenant_id IN ? AND knowledgebase.status = ?", ids, tenantIDs, string(entity.StatusValid)).
		Find(&documents).Error
	if err != nil {
		return nil, err
	}
	return documents, nil
}

// GetByDocumentIDAndDatasetID retrieves a document by document ID and dataset/KB ID.
func (dao *DocumentDAO) GetByDocumentIDAndDatasetID(documentID, datasetID string) (*entity.Document, error) {
	var document entity.Document
	err := DB.Where("id = ? AND kb_id = ?", documentID, datasetID).First(&document).Error
	return &document, err
}

// CountByTenantID counts documents by tenant ID
func (dao *DocumentDAO) CountByTenantID(tenantID string) (int64, error) {
	var count int64
	err := DB.Model(&entity.Document{}).Where("created_by = ?", tenantID).Count(&count).Error
	return count, err
}

// SumSizeByDatasetID returns the total document size for a dataset.
func (dao *DocumentDAO) SumSizeByDatasetID(datasetID string) (int64, error) {
	var total int64
	err := DB.Model(&entity.Document{}).
		Select("COALESCE(SUM(size), 0)").
		Where("kb_id = ?", datasetID).
		Scan(&total).Error
	return total, err
}

// GetParsingStatusByKBID aggregates document parsing status counts for a
// dataset, mirroring DocumentService.get_parsing_status_by_kb_ids in Python.
func (dao *DocumentDAO) GetParsingStatusByKBID(kbID string) (map[string]int64, error) {
	result := map[string]int64{
		"unstart_count": 0,
		"running_count": 0,
		"cancel_count":  0,
		"done_count":    0,
		"fail_count":    0,
	}

	var rows []struct {
		Run *string `gorm:"column:run"`
		Cnt int64   `gorm:"column:cnt"`
	}
	err := DB.Model(&entity.Document{}).
		Select("run, COUNT(id) as cnt").
		Where("kb_id = ?", kbID).
		Group("run").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	statusFieldMap := map[string]string{
		string(entity.TaskStatusUnstart): "unstart_count",
		string(entity.TaskStatusRunning): "running_count",
		string(entity.TaskStatusCancel):  "cancel_count",
		string(entity.TaskStatusDone):    "done_count",
		string(entity.TaskStatusFail):    "fail_count",
	}
	for _, row := range rows {
		if row.Run == nil {
			continue
		}
		if field, ok := statusFieldMap[*row.Run]; ok {
			result[field] = row.Cnt
		}
	}
	return result, nil
}

func (dao *DocumentDAO) GetByNameAndKBID(name, kbID string) ([]*entity.Document, error) {
	var docs []*entity.Document
	err := DB.Where("name = ? AND kb_id = ?", name, kbID).Find(&docs).Error
	return docs, err
}

// ListNamesByKbID returns every document name in a dataset, used to compute a
// non-colliding upload filename (mirrors Python duplicate_name).
func (dao *DocumentDAO) ListNamesByKbID(kbID string) ([]string, error) {
	var names []string
	err := DB.Model(&entity.Document{}).Where("kb_id = ?", kbID).Pluck("name", &names).Error
	if err != nil {
		return nil, err
	}
	return names, nil
}
