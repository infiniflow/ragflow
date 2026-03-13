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
	"ragflow/internal/model"
	"strings"
	"time"
)

// KnowledgebaseDAO knowledge base data access object
type KnowledgebaseDAO struct{}

// NewKnowledgebaseDAO create knowledge base DAO
func NewKnowledgebaseDAO() *KnowledgebaseDAO {
	return &KnowledgebaseDAO{}
}

// Create creates a new knowledge base record
func (dao *KnowledgebaseDAO) Create(kb *model.Knowledgebase) error {
	return DB.Create(kb).Error
}

// Update updates a knowledge base record
func (dao *KnowledgebaseDAO) Update(kb *model.Knowledgebase) error {
	return DB.Save(kb).Error
}

// UpdateByID updates a knowledge base by ID with the given fields
func (dao *KnowledgebaseDAO) UpdateByID(id string, updates map[string]interface{}) error {
	return DB.Model(&model.Knowledgebase{}).Where("id = ?", id).Updates(updates).Error
}

// Delete soft deletes a knowledge base by setting status to invalid
func (dao *KnowledgebaseDAO) Delete(id string) error {
	return DB.Model(&model.Knowledgebase{}).Where("id = ?", id).Update("status", string(model.StatusInvalid)).Error
}

// GetByID retrieves a knowledge base by ID
func (dao *KnowledgebaseDAO) GetByID(id string) (*model.Knowledgebase, error) {
	var kb model.Knowledgebase
	err := DB.Where("id = ? AND status = ?", id, string(model.StatusValid)).First(&kb).Error
	if err != nil {
		return nil, err
	}
	return &kb, nil
}

// GetByIDAndTenantID retrieves a knowledge base by ID and tenant ID
func (dao *KnowledgebaseDAO) GetByIDAndTenantID(id, tenantID string) (*model.Knowledgebase, error) {
	var kb model.Knowledgebase
	err := DB.Where("id = ? AND tenant_id = ? AND status = ?", id, tenantID, string(model.StatusValid)).First(&kb).Error
	if err != nil {
		return nil, err
	}
	return &kb, nil
}

// GetByIDs retrieves multiple knowledge bases by IDs
func (dao *KnowledgebaseDAO) GetByIDs(ids []string) ([]*model.Knowledgebase, error) {
	var kbs []*model.Knowledgebase
	err := DB.Where("id IN ? AND status = ?", ids, string(model.StatusValid)).Find(&kbs).Error
	return kbs, err
}

// GetByName retrieves a knowledge base by name and tenant ID
func (dao *KnowledgebaseDAO) GetByName(name, tenantID string) (*model.Knowledgebase, error) {
	var kb model.Knowledgebase
	err := DB.Where("name = ? AND tenant_id = ? AND status = ?", name, tenantID, string(model.StatusValid)).First(&kb).Error
	if err != nil {
		return nil, err
	}
	return &kb, nil
}

// GetByCreatedBy retrieves knowledge bases created by a specific user
func (dao *KnowledgebaseDAO) GetByCreatedBy(createdBy string) ([]*model.Knowledgebase, error) {
	var kbs []*model.Knowledgebase
	err := DB.Where("created_by = ? AND status = ?", createdBy, string(model.StatusValid)).Find(&kbs).Error
	return kbs, err
}

// Query retrieves knowledge bases with filters
func (dao *KnowledgebaseDAO) Query(filters map[string]interface{}) ([]*model.Knowledgebase, error) {
	var kbs []*model.Knowledgebase
	query := DB.Where("status = ?", string(model.StatusValid))

	for key, value := range filters {
		if value != nil && value != "" {
			query = query.Where(key+" = ?", value)
		}
	}

	err := query.Find(&kbs).Error
	return kbs, err
}

// QueryOne retrieves a single knowledge base with filters
func (dao *KnowledgebaseDAO) QueryOne(filters map[string]interface{}) (*model.Knowledgebase, error) {
	var kb model.Knowledgebase
	query := DB.Where("status = ?", string(model.StatusValid))

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
func (dao *KnowledgebaseDAO) Count(filters map[string]interface{}) (int64, error) {
	var count int64
	query := DB.Model(&model.Knowledgebase{}).Where("status = ?", string(model.StatusValid))

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
func (dao *KnowledgebaseDAO) GetByTenantIDs(tenantIDs []string, userID string, pageNumber, itemsPerPage int, orderby string, desc bool, keywords, parserID string) ([]*model.KnowledgebaseListItem, int64, error) {
	var kbs []*model.KnowledgebaseListItem
	var total int64

	query := DB.Model(&model.Knowledgebase{}).
		Select(`knowledgebase.id, knowledgebase.avatar, knowledgebase.name,
			knowledgebase.language, knowledgebase.description, knowledgebase.tenant_id,
			knowledgebase.permission, knowledgebase.doc_num, knowledgebase.token_num,
			knowledgebase.chunk_num, knowledgebase.parser_id, knowledgebase.embd_id,
			user.nickname, user.avatar as tenant_avatar, knowledgebase.update_time`).
		Joins("LEFT JOIN user ON knowledgebase.tenant_id = user.id").
		Where("((knowledgebase.tenant_id IN ? AND knowledgebase.permission = ?) OR knowledgebase.tenant_id = ?) AND knowledgebase.status = ?",
			tenantIDs, string(model.TenantPermissionTeam), userID, string(model.StatusValid))

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

// GetAllByTenantIDs retrieves all permitted knowledge bases by tenant IDs
// This matches the Python get_all_kb_by_tenant_ids method
func (dao *KnowledgebaseDAO) GetAllByTenantIDs(tenantIDs []string, userID string) ([]*model.Knowledgebase, error) {
	var kbs []*model.Knowledgebase

	err := DB.Where(
		"(tenant_id IN ? AND permission = ?) OR tenant_id = ?",
		tenantIDs, string(model.TenantPermissionTeam), userID,
	).Order("create_time ASC").Find(&kbs).Error

	return kbs, err
}

// GetDetail retrieves detailed knowledge base information with joined pipeline data
// This matches the Python get_detail method
func (dao *KnowledgebaseDAO) GetDetail(kbID string) (*model.KnowledgebaseDetail, error) {
	var detail model.KnowledgebaseDetail

	err := DB.Table("knowledgebase").
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
		Where("knowledgebase.id = ? AND knowledgebase.status = ?", kbID, string(model.StatusValid)).
		Scan(&detail).Error

	if err != nil {
		return nil, err
	}

	return &detail, nil
}

// Accessible checks if a knowledge base is accessible by a user
// This matches the Python accessible method
func (dao *KnowledgebaseDAO) Accessible(kbID, userID string) bool {
	var count int64
	err := DB.Table("knowledgebase").
		Joins("JOIN user_tenant ON user_tenant.tenant_id = knowledgebase.tenant_id").
		Where("knowledgebase.id = ? AND user_tenant.user_id = ? AND knowledgebase.status = ?",
			kbID, userID, string(model.StatusValid)).
		Count(&count).Error

	if err != nil {
		return false
	}
	return count > 0
}

// Accessible4Deletion checks if a knowledge base can be deleted by a user
// This matches the Python accessible4deletion method
func (dao *KnowledgebaseDAO) Accessible4Deletion(kbID, userID string) bool {
	var count int64
	err := DB.Model(&model.Knowledgebase{}).
		Where("id = ? AND created_by = ? AND status = ?", kbID, userID, string(model.StatusValid)).
		Count(&count).Error

	if err != nil {
		return false
	}
	return count > 0
}

// DuplicateName generates a unique name by appending parentheses if name already exists
// This matches the Python duplicate_name function behavior
func (dao *KnowledgebaseDAO) DuplicateName(name, tenantID string) string {
	var existingNames []string
	DB.Model(&model.Knowledgebase{}).
		Where("name LIKE ? AND tenant_id = ? AND status = ?", name+"%", tenantID, string(model.StatusValid)).
		Pluck("name", &existingNames)

	if len(existingNames) == 0 {
		return name
	}

	nameSet := make(map[string]bool)
	for _, n := range existingNames {
		nameSet[strings.ToLower(n)] = true
	}

	if !nameSet[strings.ToLower(name)] {
		return name
	}

	for i := 1; ; i++ {
		newName := name + " " + strings.Repeat("(", i) + strings.Repeat(")", i)
		if !nameSet[strings.ToLower(newName)] {
			return newName
		}
	}
}

// AtomicIncreaseDocNumByID atomically increments the document count
// This matches the Python atomic_increase_doc_num_by_id method
func (dao *KnowledgebaseDAO) AtomicIncreaseDocNumByID(kbID string) error {
	now := time.Now().Unix()
	nowDate := time.Now()
	return DB.Model(&model.Knowledgebase{}).
		Where("id = ?", kbID).
		Updates(map[string]interface{}{
			"doc_num":     DB.Raw("doc_num + 1"),
			"update_time": now,
			"update_date": nowDate,
		}).Error
}

// DecreaseDocumentNum decreases document, chunk, and token counts
// This matches the Python decrease_document_num_in_delete method
func (dao *KnowledgebaseDAO) DecreaseDocumentNum(kbID string, docNum, chunkNum, tokenNum int64) error {
	now := time.Now().Unix()
	nowDate := time.Now()
	return DB.Model(&model.Knowledgebase{}).
		Where("id = ?", kbID).
		Updates(map[string]interface{}{
			"doc_num":     DB.Raw("doc_num - ?", docNum),
			"chunk_num":   DB.Raw("chunk_num - ?", chunkNum),
			"token_num":   DB.Raw("token_num - ?", tokenNum),
			"update_time": now,
			"update_date": nowDate,
		}).Error
}

// GetKBIDsByTenantID retrieves all knowledge base IDs for a tenant
// This matches the Python get_kb_ids method
func (dao *KnowledgebaseDAO) GetKBIDsByTenantID(tenantID string) ([]string, error) {
	var kbIDs []string
	err := DB.Model(&model.Knowledgebase{}).
		Where("tenant_id = ? AND status = ?", tenantID, string(model.StatusValid)).
		Pluck("id", &kbIDs).Error
	return kbIDs, err
}

// GetAllIDs retrieves all knowledge base IDs
// This matches the Python get_all_ids method
func (dao *KnowledgebaseDAO) GetAllIDs() ([]string, error) {
	var kbIDs []string
	err := DB.Model(&model.Knowledgebase{}).
		Where("status = ?", string(model.StatusValid)).
		Pluck("id", &kbIDs).Error
	return kbIDs, err
}

// UpdateParserConfig updates the parser configuration with deep merge
// This matches the Python update_parser_config method
func (dao *KnowledgebaseDAO) UpdateParserConfig(id string, config map[string]interface{}) error {
	var kb model.Knowledgebase
	if err := DB.Where("id = ? AND status = ?", id, string(model.StatusValid)).First(&kb).Error; err != nil {
		return err
	}

	mergedConfig := mergeConfig(kb.ParserConfig, config)
	return DB.Model(&model.Knowledgebase{}).
		Where("id = ?", id).
		Update("parser_config", mergedConfig).Error
}

// DeleteFieldMap removes the field_map from parser_config
// This matches the Python delete_field_map method
func (dao *KnowledgebaseDAO) DeleteFieldMap(id string) error {
	var kb model.Knowledgebase
	if err := DB.Where("id = ? AND status = ?", id, string(model.StatusValid)).First(&kb).Error; err != nil {
		return err
	}

	if kb.ParserConfig != nil {
		delete(kb.ParserConfig, "field_map")
		return DB.Model(&model.Knowledgebase{}).
			Where("id = ?", id).
			Update("parser_config", kb.ParserConfig).Error
	}
	return nil
}

// GetFieldMap retrieves field mappings from multiple knowledge bases
// This matches the Python get_field_map method
func (dao *KnowledgebaseDAO) GetFieldMap(ids []string) (map[string]interface{}, error) {
	conf := make(map[string]interface{})
	kbs, err := dao.GetByIDs(ids)
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
func (dao *KnowledgebaseDAO) GetKBByIDAndUserID(kbID, userID string) ([]*model.Knowledgebase, error) {
	var kbs []*model.Knowledgebase
	err := DB.Model(&model.Knowledgebase{}).
		Joins("JOIN user_tenant ON user_tenant.tenant_id = knowledgebase.tenant_id").
		Where("knowledgebase.id = ? AND user_tenant.user_id = ?", kbID, userID).
		Limit(1).
		Find(&kbs).Error
	return kbs, err
}

// GetKBByNameAndUserID retrieves a knowledge base by name and user ID with tenant join
// This matches the Python get_kb_by_name method
func (dao *KnowledgebaseDAO) GetKBByNameAndUserID(kbName, userID string) ([]*model.Knowledgebase, error) {
	var kbs []*model.Knowledgebase
	err := DB.Model(&model.Knowledgebase{}).
		Joins("JOIN user_tenant ON user_tenant.tenant_id = knowledgebase.tenant_id").
		Where("knowledgebase.name = ? AND user_tenant.user_id = ?", kbName, userID).
		Limit(1).
		Find(&kbs).Error
	return kbs, err
}

// GetList retrieves knowledge bases with filtering by ID and name
// This matches the Python get_list method
func (dao *KnowledgebaseDAO) GetList(tenantIDs []string, userID string, pageNumber, itemsPerPage int, orderby string, desc bool, id, name string) ([]*model.Knowledgebase, int64, error) {
	var kbs []*model.Knowledgebase
	var total int64

	query := DB.Model(&model.Knowledgebase{}).
		Where("((tenant_id IN ? AND permission = ?) OR tenant_id = ?) AND status = ?",
			tenantIDs, string(model.TenantPermissionTeam), userID, string(model.StatusValid))

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
func (dao *KnowledgebaseDAO) DeleteByTenantID(tenantID string) (int64, error) {
	result := DB.Unscoped().Where("tenant_id = ?", tenantID).Delete(&model.Knowledgebase{})
	return result.RowsAffected, result.Error
}

// GetKBIDsByTenantID gets all knowledge base IDs by tenant ID
func (dao *KnowledgebaseDAO) GetKBIDsByTenantIDSimple(tenantID string) ([]string, error) {
	var kbIDs []string
	err := DB.Model(&model.Knowledgebase{}).
		Where("tenant_id = ?", tenantID).
		Pluck("id", &kbIDs).Error
	return kbIDs, err
}
