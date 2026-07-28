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
	"context"
	"errors"
	"fmt"
	"path"
	"ragflow/internal/entity"

	"strconv"
	"strings"

	"gorm.io/gorm"
)

// GetTenantIDByKBID is a convenience function that retrieves the tenant ID
// for a given knowledge base ID. It is a package-level helper so both the
// service and engine layers can use it without circular imports.
func GetTenantIDByKBID(ctx context.Context, db *gorm.DB, kbID string) (string, error) {
	kbDAO := NewKnowledgebaseDAO()
	kb, err := kbDAO.GetByID(ctx, db, kbID)
	if err != nil {
		return "", fmt.Errorf("dataset not found: %w", err)
	}
	return kb.TenantID, nil
}

// KnowledgebaseDAO knowledge base data access object
type KnowledgebaseDAO struct{}

// IsNotFoundErr returns true if the error indicates a record not found
func IsNotFoundErr(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// IsDuplicateKeyErr returns true if the error is a unique-constraint violation.
func IsDuplicateKeyErr(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey)
}

// NewKnowledgebaseDAO create knowledge base DAO
func NewKnowledgebaseDAO() *KnowledgebaseDAO {
	return &KnowledgebaseDAO{}
}

// Create creates a new knowledge base record
func (dao *KnowledgebaseDAO) Create(ctx context.Context, db *gorm.DB, kb *entity.Knowledgebase) error {
	return db.WithContext(ctx).Create(kb).Error
}

// Update updates a knowledge base record
func (dao *KnowledgebaseDAO) Update(ctx context.Context, db *gorm.DB, kb *entity.Knowledgebase) error {
	return db.WithContext(ctx).Save(kb).Error
}

// UpdateByID updates a knowledge base by ID with the given fields
func (dao *KnowledgebaseDAO) UpdateByID(ctx context.Context, db *gorm.DB, id string, updates map[string]interface{}) error {
	return db.WithContext(ctx).Model(&entity.Knowledgebase{}).Where("id = ?", id).Updates(updates).Error
}

// Delete soft deletes a knowledge base by setting status to invalid
func (dao *KnowledgebaseDAO) Delete(ctx context.Context, db *gorm.DB, id string) error {
	return db.WithContext(ctx).Model(&entity.Knowledgebase{}).Where("id = ?", id).Update("status", string(entity.StatusInvalid)).Error
}

// GetByID retrieves a knowledge base by ID
func (dao *KnowledgebaseDAO) GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.Knowledgebase, error) {
	var kb entity.Knowledgebase
	err := db.WithContext(ctx).Where("id = ? AND status = ?", id, string(entity.StatusValid)).First(&kb).Error
	if err != nil {
		return nil, err
	}
	return &kb, nil
}

// GetByIDAndTenantID retrieves a knowledge base by ID and tenant ID
func (dao *KnowledgebaseDAO) GetByIDAndTenantID(ctx context.Context, db *gorm.DB, id, tenantID string) (*entity.Knowledgebase, error) {
	var kb entity.Knowledgebase
	err := db.WithContext(ctx).Where("id = ? AND tenant_id = ? AND status = ?", id, tenantID, string(entity.StatusValid)).First(&kb).Error
	if err != nil {
		return nil, err
	}
	return &kb, nil
}

// GetByIDs retrieves multiple knowledge bases by IDs
func (dao *KnowledgebaseDAO) GetByIDs(ctx context.Context, db *gorm.DB, ids []string) ([]*entity.Knowledgebase, error) {
	var kbs []*entity.Knowledgebase
	err := db.WithContext(ctx).Where("id IN ? AND status = ?", ids, string(entity.StatusValid)).Find(&kbs).Error
	return kbs, err
}

// GetByName retrieves a knowledge base by name and tenant ID
func (dao *KnowledgebaseDAO) GetByName(ctx context.Context, db *gorm.DB, name, tenantID string) (*entity.Knowledgebase, error) {
	var kb entity.Knowledgebase
	err := db.WithContext(ctx).Where("LOWER(name) = LOWER(?) AND tenant_id = ? AND status = ?", name, tenantID, string(entity.StatusValid)).First(&kb).Error
	if err != nil {
		return nil, err
	}
	return &kb, nil
}

// GetByCreatedBy retrieves knowledge bases created by a specific user
func (dao *KnowledgebaseDAO) GetByCreatedBy(ctx context.Context, db *gorm.DB, createdBy string) ([]*entity.Knowledgebase, error) {
	var kbs []*entity.Knowledgebase
	err := db.WithContext(ctx).Where("created_by = ? AND status = ?", createdBy, string(entity.StatusValid)).Find(&kbs).Error
	return kbs, err
}

// Query retrieves knowledge bases with filters
func (dao *KnowledgebaseDAO) Query(ctx context.Context, db *gorm.DB, filters map[string]interface{}) ([]*entity.Knowledgebase, error) {
	var kbs []*entity.Knowledgebase
	query := db.WithContext(ctx).Where("status = ?", string(entity.StatusValid))

	for key, value := range filters {
		if value != nil && value != "" {
			query = query.Where(key+" = ?", value)
		}
	}

	err := query.Find(&kbs).Error
	return kbs, err
}

// QueryOne retrieves a single knowledge base with filters
func (dao *KnowledgebaseDAO) QueryOne(ctx context.Context, db *gorm.DB, filters map[string]interface{}) (*entity.Knowledgebase, error) {
	var kb entity.Knowledgebase
	query := db.WithContext(ctx).Where("status = ?", string(entity.StatusValid))

	for key, value := range filters {
		if value != nil && value != "" {
			query = query.Where(key+" = ?", value)
		}
	}

	err := query.First(&kb).Error
	if err != nil {
		return nil, err
	}
	return &kb, nil
}

// Count returns the count of knowledge bases matching the filters
func (dao *KnowledgebaseDAO) Count(ctx context.Context, db *gorm.DB, filters map[string]interface{}) (int64, error) {
	var count int64
	query := db.WithContext(ctx).Model(&entity.Knowledgebase{}).Where("status = ?", string(entity.StatusValid))

	for key, value := range filters {
		if value != nil && value != "" {
			query = query.Where(key+" = ?", value)
		}
	}

	err := query.Count(&count).Error
	return count, err
}

// GetByTenantIDs retrieves knowledge bases by tenant IDs with pagination
// This matches the Python get_by_tenant_ids method
func (dao *KnowledgebaseDAO) GetByTenantIDs(ctx context.Context, db *gorm.DB, tenantIDs []string, userID string, pageNumber, itemsPerPage int, orderby string, desc bool, keywords, parserID, id, name string) ([]*entity.KnowledgebaseListItem, int64, error) {
	var kbs []*entity.KnowledgebaseListItem
	var total int64

	query := db.WithContext(ctx).Model(&entity.Knowledgebase{}).
		Select(`knowledgebase.id, knowledgebase.avatar, knowledgebase.name,
			knowledgebase.language, knowledgebase.description, knowledgebase.tenant_id,
			knowledgebase.permission, knowledgebase.doc_num, knowledgebase.token_num,
			knowledgebase.chunk_num, knowledgebase.parser_id, knowledgebase.embd_id,
			knowledgebase.tenant_embd_id,
			user.nickname, user.avatar as tenant_avatar, knowledgebase.update_time`).
		Joins("LEFT JOIN user ON knowledgebase.tenant_id = user.id").
		Where("((knowledgebase.tenant_id IN ? AND knowledgebase.permission = ?) OR knowledgebase.tenant_id = ?) AND knowledgebase.status = ?",
			tenantIDs, string(entity.TenantPermissionTeam), userID, string(entity.StatusValid))

	if id != "" {
		query = query.Where("knowledgebase.id = ?", id)
	}

	if name != "" {
		query = query.Where("knowledgebase.name = ?", name)
	}

	if keywords != "" {
		query = query.Where("LOWER(knowledgebase.name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}

	if parserID != "" {
		query = query.Where("knowledgebase.parser_id = ?", parserID)
	}

	if desc {
		query = query.Order("knowledgebase." + orderby + " DESC")
	} else {
		query = query.Order("knowledgebase." + orderby + " ASC")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if pageNumber > 0 && itemsPerPage > 0 {
		offset := (pageNumber - 1) * itemsPerPage
		if err := query.Offset(offset).Limit(itemsPerPage).Scan(&kbs).Error; err != nil {
			return nil, 0, err
		}
	} else {
		if err := query.Scan(&kbs).Error; err != nil {
			return nil, 0, err
		}
	}

	return kbs, total, nil
}

// GetOwnerFilter returns owner counts for datasets visible to a user.
func (dao *KnowledgebaseDAO) GetOwnerFilter(tenantIDs []string, userID string) ([]*entity.DatasetOwnerFilter, error) {
	owners := make([]*entity.DatasetOwnerFilter, 0)

	err := DB.Model(&entity.Knowledgebase{}).
		Select("knowledgebase.tenant_id as id, user.nickname as label, COUNT(knowledgebase.id) as count").
		Joins("LEFT JOIN user ON knowledgebase.tenant_id = user.id").
		Where("((knowledgebase.tenant_id IN ? AND knowledgebase.permission = ?) OR knowledgebase.tenant_id = ?) AND knowledgebase.status = ?",
			tenantIDs, string(entity.TenantPermissionTeam), userID, string(entity.StatusValid)).
		Group("knowledgebase.tenant_id, user.nickname").
		Scan(&owners).Error

	return owners, err
}

// GetAllByTenantIDs retrieves all permitted knowledge bases by tenant IDs
// This matches the Python get_all_kb_by_tenant_ids method
func (dao *KnowledgebaseDAO) GetAllByTenantIDs(ctx context.Context, db *gorm.DB, tenantIDs []string, userID string) ([]*entity.Knowledgebase, error) {
	var kbs []*entity.Knowledgebase

	err := db.WithContext(ctx).Where(
		"(tenant_id IN ? AND permission = ?) OR tenant_id = ?",
		tenantIDs, string(entity.TenantPermissionTeam), userID,
	).Order("create_time ASC").Find(&kbs).Error

	return kbs, err
}

// GetDetail retrieves detailed knowledge base information with joined pipeline data
// This matches the Python get_detail method
func (dao *KnowledgebaseDAO) GetDetail(ctx context.Context, db *gorm.DB, kbID string) (*entity.KnowledgebaseDetail, error) {
	var detail entity.KnowledgebaseDetail

	err := db.WithContext(ctx).Table("knowledgebase").
		Select(`knowledgebase.id, knowledgebase.embd_id, knowledgebase.avatar, knowledgebase.name,
			knowledgebase.language, knowledgebase.description, knowledgebase.permission,
			knowledgebase.doc_num, knowledgebase.token_num, knowledgebase.chunk_num,
			knowledgebase.parser_id, knowledgebase.pipeline_id,
			user_canvas.title as pipeline_name, user_canvas.avatar as pipeline_avatar,
			knowledgebase.parser_config, knowledgebase.pagerank,
			knowledgebase.graphrag_task_id, knowledgebase.graphrag_task_finish_at,
			knowledgebase.raptor_task_id, knowledgebase.raptor_task_finish_at,
			knowledgebase.mindmap_task_id, knowledgebase.mindmap_task_finish_at,
			knowledgebase.create_time, knowledgebase.update_time`).
		Joins("LEFT JOIN user_canvas ON knowledgebase.pipeline_id = user_canvas.id").
		Where("knowledgebase.id = ? AND knowledgebase.status = ?", kbID, string(entity.StatusValid)).
		Scan(&detail).Error

	if err != nil {
		return nil, err
	}

	return &detail, nil
}

// Accessible checks if a knowledge base is accessible by a user.
// This matches the Python accessible method:
// 1. KB must exist and be VALID
// 2. If user is the owner tenant, return true
// 3. If permission is "me", only owner tenant can access
// 4. If permission is "team", user must be a member of the tenant
func (dao *KnowledgebaseDAO) Accessible(ctx context.Context, db *gorm.DB, datasetID, userID string) bool {
	var kb entity.Knowledgebase
	err := db.WithContext(ctx).Where("id = ? AND status = ?", datasetID, string(entity.StatusValid)).First(&kb).Error
	if err != nil {
		return false
	}

	// User is the owner tenant itself
	if kb.TenantID == userID {
		return true
	}

	// If permission is "me", only the owner can access
	if kb.Permission == string(entity.TenantPermissionMe) {
		return false
	}

	var count int64
	err = db.WithContext(ctx).Table("user_tenant").
		Where("tenant_id = ? AND user_id = ? AND status = ?", kb.TenantID, userID, "1").
		Count(&count).Error

	if err != nil {
		return false
	}
	return count > 0
}

// Accessible4Deletion checks if a knowledge base can be deleted by a user
// This matches the Python accessible4deletion method
func (dao *KnowledgebaseDAO) Accessible4Deletion(ctx context.Context, db *gorm.DB, kbID, userID string) bool {
	var count int64
	err := db.WithContext(ctx).Model(&entity.Knowledgebase{}).
		Where("id = ? AND created_by = ? AND status = ?", kbID, userID, string(entity.StatusValid)).
		Count(&count).Error

	if err != nil {
		return false
	}
	return count > 0
}

// DuplicateName generates a unique name by appending parentheses if name already exists
// This matches the Python duplicate_name function behavior
func (dao *KnowledgebaseDAO) DuplicateName(ctx context.Context, db *gorm.DB, name, tenantID string) string {
	const maxRetries = 1000

	currentName := name
	for retries := 0; retries < maxRetries; retries++ {
		var count int64
		err := db.WithContext(ctx).Model(&entity.Knowledgebase{}).
			Where("LOWER(name) = ? AND tenant_id = ? AND status = ?", strings.ToLower(currentName), tenantID, string(entity.StatusValid)).
			Count(&count).Error
		if err != nil || count == 0 {
			return currentName
		}

		suffix := path.Ext(currentName)
		stem := strings.TrimSuffix(currentName, suffix)
		mainPart, counter := splitNameCounter(stem)
		nextCounter := 1
		if counter > 0 {
			nextCounter = counter + 1
		}

		currentName = mainPart + "(" + strconv.Itoa(nextCounter) + ")" + suffix
	}

	return currentName
}

func splitNameCounter(name string) (string, int) {
	if !strings.HasSuffix(name, ")") {
		return name, 0
	}

	leftBracketIndex := strings.LastIndex(name, "(")
	if leftBracketIndex < 0 || leftBracketIndex >= len(name)-1 {
		return name, 0
	}

	counterValue := name[leftBracketIndex+1 : len(name)-1]
	counter, err := strconv.Atoi(counterValue)
	if err != nil {
		return name, 0
	}

	return strings.TrimRight(name[:leftBracketIndex], " "), counter
}

// AtomicIncreaseDocNumByID atomically increments the document count
// This matches the Python atomic_increase_doc_num_by_id method
func (dao *KnowledgebaseDAO) AtomicIncreaseDocNumByID(ctx context.Context, db *gorm.DB, kbID string) error {
	return db.WithContext(ctx).Model(&entity.Knowledgebase{}).
		Where("id = ?", kbID).
		Updates(map[string]interface{}{
			"doc_num": db.Raw("doc_num + 1"),
		}).Error
}

// DecreaseDocumentNum decreases document, chunk, and token counts
// This matches the Python decrease_document_num_in_delete method
func (dao *KnowledgebaseDAO) DecreaseDocumentNum(ctx context.Context, db *gorm.DB, kbID string, docNum, chunkNum, tokenNum int64) error {
	return db.WithContext(ctx).Model(&entity.Knowledgebase{}).
		Where("id = ?", kbID).
		Updates(map[string]interface{}{
			"doc_num":   db.Raw("doc_num - ?", docNum),
			"chunk_num": db.Raw("chunk_num - ?", chunkNum),
			"token_num": db.Raw("token_num - ?", tokenNum),
		}).Error
}

// GetKBIDsByTenantID retrieves all knowledge base IDs for a tenant
// This matches the Python get_kb_ids method
func (dao *KnowledgebaseDAO) GetKBIDsByTenantID(ctx context.Context, db *gorm.DB, tenantID string) ([]string, error) {
	var kbIDs []string
	err := db.WithContext(ctx).Model(&entity.Knowledgebase{}).
		Where("tenant_id = ? AND status = ?", tenantID, string(entity.StatusValid)).
		Pluck("id", &kbIDs).Error
	return kbIDs, err
}

// GetAllIDs retrieves all knowledge base IDs
// This matches the Python get_all_ids method
func (dao *KnowledgebaseDAO) GetAllIDs(ctx context.Context, db *gorm.DB) ([]string, error) {
	var kbIDs []string
	err := db.WithContext(ctx).Model(&entity.Knowledgebase{}).
		Where("status = ?", string(entity.StatusValid)).
		Pluck("id", &kbIDs).Error
	return kbIDs, err
}

// UpdateParserConfig updates the parser configuration with deep merge
// This matches the Python update_parser_config method
func (dao *KnowledgebaseDAO) UpdateParserConfig(ctx context.Context, db *gorm.DB, id string, config map[string]interface{}) error {
	var kb entity.Knowledgebase
	if err := db.WithContext(ctx).Where("id = ? AND status = ?", id, string(entity.StatusValid)).First(&kb).Error; err != nil {
		return err
	}

	mergedConfig := mergeConfig(kb.ParserConfig, config)
	return db.WithContext(ctx).Model(&entity.Knowledgebase{}).
		Where("id = ?", id).
		Update("parser_config", mergedConfig).Error
}

// DeleteFieldMap removes the field_map from parser_config
// This matches the Python delete_field_map method
func (dao *KnowledgebaseDAO) DeleteFieldMap(ctx context.Context, db *gorm.DB, id string) error {
	var kb entity.Knowledgebase
	if err := db.WithContext(ctx).Where("id = ? AND status = ?", id, string(entity.StatusValid)).First(&kb).Error; err != nil {
		return err
	}

	if kb.ParserConfig != nil {
		delete(kb.ParserConfig, "field_map")
		return db.WithContext(ctx).Model(&entity.Knowledgebase{}).
			Where("id = ?", id).
			Update("parser_config", kb.ParserConfig).Error
	}
	return nil
}

// GetFieldMap retrieves field mappings from multiple knowledge bases
// This matches the Python get_field_map method
func (dao *KnowledgebaseDAO) GetFieldMap(ctx context.Context, db *gorm.DB, ids []string) (map[string]interface{}, error) {
	conf := make(map[string]interface{})
	kbs, err := dao.GetByIDs(ctx, db, ids)
	if err != nil {
		return nil, err
	}

	for _, kb := range kbs {
		if kb.ParserConfig != nil {
			if fieldMap, ok := kb.ParserConfig["field_map"]; ok {
				if fm, ok := fieldMap.(map[string]interface{}); ok {
					for k, v := range fm {
						conf[k] = v
					}
				}
			}
		}
	}
	return conf, nil
}

// GetKBByIDAndUserID retrieves a knowledge base by ID and user ID with tenant join
// This matches the Python get_kb_by_id method
func (dao *KnowledgebaseDAO) GetKBByIDAndUserID(ctx context.Context, db *gorm.DB, kbID, userID string) ([]*entity.Knowledgebase, error) {
	var kbs []*entity.Knowledgebase
	err := db.WithContext(ctx).Model(&entity.Knowledgebase{}).
		Joins("JOIN user_tenant ON user_tenant.tenant_id = knowledgebase.tenant_id").
		Where("knowledgebase.id = ? AND user_tenant.user_id = ?", kbID, userID).
		Limit(1).
		Find(&kbs).Error
	return kbs, err
}

// GetKBByNameAndUserID retrieves a knowledge base by name and user ID with tenant join
// This matches the Python get_kb_by_name method
func (dao *KnowledgebaseDAO) GetKBByNameAndUserID(ctx context.Context, db *gorm.DB, kbName, userID string) ([]*entity.Knowledgebase, error) {
	var kbs []*entity.Knowledgebase
	err := db.WithContext(ctx).Model(&entity.Knowledgebase{}).
		Joins("JOIN user_tenant ON user_tenant.tenant_id = knowledgebase.tenant_id").
		Where("knowledgebase.name = ? AND user_tenant.user_id = ?", kbName, userID).
		Limit(1).
		Find(&kbs).Error
	return kbs, err
}

// GetList retrieves knowledge bases with filtering by ID and name
// This matches the Python get_list method
func (dao *KnowledgebaseDAO) GetList(ctx context.Context, db *gorm.DB, tenantIDs []string, userID string, pageNumber, itemsPerPage int, orderby string, desc bool, id, name string) ([]*entity.Knowledgebase, int64, error) {
	var kbs []*entity.Knowledgebase
	var total int64

	query := db.WithContext(ctx).Model(&entity.Knowledgebase{}).
		Where("((tenant_id IN ? AND permission = ?) OR tenant_id = ?) AND status = ?",
			tenantIDs, string(entity.TenantPermissionTeam), userID, string(entity.StatusValid))

	if id != "" {
		query = query.Where("id = ?", id)
	}
	if name != "" {
		query = query.Where("name = ?", name)
	}

	if desc {
		query = query.Order(orderby + " DESC")
	} else {
		query = query.Order(orderby + " ASC")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if pageNumber > 0 && itemsPerPage > 0 {
		offset := (pageNumber - 1) * itemsPerPage
		if err := query.Offset(offset).Limit(itemsPerPage).Find(&kbs).Error; err != nil {
			return nil, 0, err
		}
	} else {
		if err := query.Find(&kbs).Error; err != nil {
			return nil, 0, err
		}
	}

	return kbs, total, nil
}

// mergeConfig performs a deep merge of configuration maps
func mergeConfig(old, new map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range old {
		result[k] = v
	}

	for k, v := range new {
		if existing, ok := result[k]; ok {
			if existingMap, ok := existing.(map[string]interface{}); ok {
				if newMap, ok := v.(map[string]interface{}); ok {
					result[k] = mergeConfig(existingMap, newMap)
					continue
				}
			}
			if existingSlice, ok := existing.([]interface{}); ok {
				if newSlice, ok := v.([]interface{}); ok {
					merged := append(existingSlice, newSlice...)
					seen := make(map[interface{}]bool)
					unique := make([]interface{}, 0)
					for _, item := range merged {
						if !seen[item] {
							seen[item] = true
							unique = append(unique, item)
						}
					}
					result[k] = unique
					continue
				}
			}
		}
		result[k] = v
	}

	return result
}

// DeleteByTenantID deletes all knowledge bases by tenant ID (hard delete)
func (dao *KnowledgebaseDAO) DeleteByTenantID(ctx context.Context, db *gorm.DB, tenantID string) (int64, error) {
	result := db.WithContext(ctx).Unscoped().Where("tenant_id = ?", tenantID).Delete(&entity.Knowledgebase{})
	return result.RowsAffected, result.Error
}

// GetKBIDsByTenantIDSimple GetKBIDsByTenantID gets all knowledge base IDs by tenant ID
func (dao *KnowledgebaseDAO) GetKBIDsByTenantIDSimple(ctx context.Context, db *gorm.DB, tenantID string) ([]string, error) {
	var kbIDs []string
	err := db.WithContext(ctx).Model(&entity.Knowledgebase{}).
		Where("tenant_id = ?", tenantID).
		Pluck("id", &kbIDs).Error
	return kbIDs, err
}
