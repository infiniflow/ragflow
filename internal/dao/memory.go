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

// Package dao implements the data access layer
// This file implements Memory-related database operations
// Consistent with Python memory_service.py
package dao

import (
	"fmt"
	"strings"

	"ragflow/internal/model"
)

// Memory type bit flag constants, consistent with Python MemoryType enum
const (
	MemoryTypeRaw        = 0b0001 // Raw memory (binary: 0001)
	MemoryTypeSemantic   = 0b0010 // Semantic memory (binary: 0010)
	MemoryTypeEpisodic   = 0b0100 // Episodic memory (binary: 0100)
	MemoryTypeProcedural = 0b1000 // Procedural memory (binary: 1000)
)

// MemoryTypeMap maps memory type names to bit flags
// Exported for use by service package
var MemoryTypeMap = map[string]int{
	"raw":        MemoryTypeRaw,
	"semantic":   MemoryTypeSemantic,
	"episodic":   MemoryTypeEpisodic,
	"procedural": MemoryTypeProcedural,
}

// CalculateMemoryType converts memory type names array to bit flags integer
//
// Parameters:
//   - memoryTypeNames: Memory type names array
//
// Returns:
//   - int64: Bit flags integer
//
// Example:
//
//	CalculateMemoryType([]string{"raw", "semantic"}) returns 3 (0b0011)
func CalculateMemoryType(memoryTypeNames []string) int64 {
	memoryType := 0
	for _, name := range memoryTypeNames {
		lowerName := strings.ToLower(name)
		if mt, ok := MemoryTypeMap[lowerName]; ok {
			memoryType |= mt
		}
	}
	return int64(memoryType)
}

// GetMemoryTypeHuman converts memory type bit flags to human-readable names
//
// Parameters:
//   - memoryType: Bit flags integer representing memory types
//
// Returns:
//   - []string: Array of human-readable memory type names
//
// Example:
//
//	GetMemoryTypeHuman(3) returns ["raw", "semantic"]
func GetMemoryTypeHuman(memoryType int64) []string {
	var result []string
	if memoryType&int64(MemoryTypeRaw) != 0 {
		result = append(result, "raw")
	}
	if memoryType&int64(MemoryTypeSemantic) != 0 {
		result = append(result, "semantic")
	}
	if memoryType&int64(MemoryTypeEpisodic) != 0 {
		result = append(result, "episodic")
	}
	if memoryType&int64(MemoryTypeProcedural) != 0 {
		result = append(result, "procedural")
	}
	return result
}

// MemoryDAO handles all Memory-related database operations
type MemoryDAO struct{}

// NewMemoryDAO creates a new MemoryDAO instance
//
// Returns:
//   - *MemoryDAO: Initialized DAO instance
func NewMemoryDAO() *MemoryDAO {
	return &MemoryDAO{}
}

// Create inserts a new memory record into the database
//
// Parameters:
//   - memory: Memory model pointer
//
// Returns:
//   - error: Database operation error
func (dao *MemoryDAO) Create(memory *model.Memory) error {
	return DB.Create(memory).Error
}

// GetByID retrieves a memory record by ID from database
//
// Parameters:
//   - id: Memory ID
//
// Returns:
//   - *model.Memory: Memory model pointer
//   - error: Database operation error
func (dao *MemoryDAO) GetByID(id string) (*model.Memory, error) {
	var memory model.Memory
	err := DB.Where("id = ?", id).First(&memory).Error
	if err != nil {
		return nil, err
	}
	return &memory, nil
}

// GetByTenantID retrieves all memories for a tenant
//
// Parameters:
//   - tenantID: Tenant ID
//
// Returns:
//   - []*model.Memory: Memory model pointer array
//   - error: Database operation error
func (dao *MemoryDAO) GetByTenantID(tenantID string) ([]*model.Memory, error) {
	var memories []*model.Memory
	err := DB.Where("tenant_id = ?", tenantID).Find(&memories).Error
	return memories, err
}

// GetByNameAndTenant checks if memory exists by name and tenant ID
// Used for duplicate name deduplication
//
// Parameters:
//   - name: Memory name
//   - tenantID: Tenant ID
//
// Returns:
//   - []*model.Memory: Matching memory list (for existence check)
//   - error: Database operation error
func (dao *MemoryDAO) GetByNameAndTenant(name string, tenantID string) ([]*model.Memory, error) {
	var memories []*model.Memory
	err := DB.Where("name = ? AND tenant_id = ?", name, tenantID).Find(&memories).Error
	return memories, err
}

// GetByIDs retrieves memories by multiple IDs
//
// Parameters:
//   - ids: Memory ID list
//
// Returns:
//   - []*model.Memory: Memory model pointer array
//   - error: Database operation error
func (dao *MemoryDAO) GetByIDs(ids []string) ([]*model.Memory, error) {
	var memories []*model.Memory
	err := DB.Where("id IN ?", ids).Find(&memories).Error
	return memories, err
}

// UpdateByID updates a memory by ID
// Supports partial updates - only updates passed fields
// Automatically handles field type conversions
//
// Parameters:
//   - id: Memory ID
//   - updates: Fields to update map
//
// Returns:
//   - error: Database operation error
//
// Field type handling:
//   - memory_type: []string converts to bit flags integer
//   - temperature: string converts to float64
//   - name: Uses string value directly
//   - permissions, forgetting_policy: Uses string value directly
//
// Example:
//
//	updates := map[string]interface{}{"name": "NewName", "memory_type": []string{"semantic"}}
//	err := dao.UpdateByID("memory123", updates)
func (dao *MemoryDAO) UpdateByID(id string, updates map[string]interface{}) error {
	if updates == nil || len(updates) == 0 {
		return nil
	}

	for key, value := range updates {
		switch key {
		case "memory_type":
			if types, ok := value.([]string); ok {
				updates[key] = CalculateMemoryType(types)
			}
		case "temperature":
			if tempStr, ok := value.(string); ok {
				var temp float64
				fmt.Sscanf(tempStr, "%f", &temp)
				updates[key] = temp
			}
		}
	}

	return DB.Model(&model.Memory{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteByID deletes a memory by ID
//
// Parameters:
//   - id: Memory ID
//
// Returns:
//   - error: Database operation error
//
// Example:
//
//	err := dao.DeleteByID("memory123")
func (dao *MemoryDAO) DeleteByID(id string) error {
	return DB.Where("id = ?", id).Delete(&model.Memory{}).Error
}

// GetWithOwnerNameByID retrieves a memory with owner name by ID
// Joins with User table to get owner's nickname
//
// Parameters:
//   - id: Memory ID
//
// Returns:
//   - *model.MemoryListItem: Memory detail with owner name populated
//   - error: Database operation error
//
// Example:
//
//	memory, err := dao.GetWithOwnerNameByID("memory123")
func (dao *MemoryDAO) GetWithOwnerNameByID(id string) (*model.MemoryListItem, error) {
	querySQL := `
		SELECT m.id, m.name, m.avatar, m.tenant_id, m.memory_type,
			m.storage_type, m.embd_id, m.tenant_embd_id, m.llm_id, m.tenant_llm_id,
			m.permissions, m.description, m.memory_size, m.forgetting_policy,
			m.temperature, m.system_prompt, m.user_prompt, m.create_time, m.create_date,
			m.update_time, m.update_date,
			u.nickname as owner_name
		FROM memory m
		LEFT JOIN user u ON m.tenant_id = u.id
		WHERE m.id = ?
	`

	var rawResult struct {
		model.Memory
		OwnerName *string `gorm:"column:owner_name"`
	}

	if err := DB.Raw(querySQL, id).Scan(&rawResult).Error; err != nil {
		return nil, err
	}

	return &model.MemoryListItem{
		Memory:    rawResult.Memory,
		OwnerName: rawResult.OwnerName,
	}, nil
}

// GetByFilter retrieves memories with optional filters
// Supports filtering by tenant_id, memory_type, storage_type, and keywords
// Returns paginated results with owner_name from user table JOIN
//
// Parameters:
//   - tenantIDs: Array of tenant IDs to filter by (empty means all tenants)
//   - memoryTypes: Array of memory type names to filter by (empty means all types)
//   - storageType: Storage type to filter by (empty means all types)
//   - keywords: Keywords to search in memory names (empty means no keyword filter)
//   - page: Page number (1-based)
//   - pageSize: Number of items per page
//
// Returns:
//   - []*model.MemoryListItem: Memory list items with owner name populated
//   - int64: Total count of matching memories
//   - error: Database operation error
//
// Example:
//
//	memories, total, err := dao.GetByFilter([]string{"tenant1"}, []string{"semantic"}, "table", "test", 1, 10)
func (dao *MemoryDAO) GetByFilter(tenantIDs []string, memoryTypes []string, storageType string, keywords string, page int, pageSize int) ([]*model.MemoryListItem, int64, error) {
	var conditions []string
	var args []interface{}

	if len(tenantIDs) > 0 {
		conditions = append(conditions, "m.tenant_id IN ?")
		args = append(args, tenantIDs)
	}

	if len(memoryTypes) > 0 {
		memoryTypeInt := CalculateMemoryType(memoryTypes)
		conditions = append(conditions, "m.memory_type & ? > 0")
		args = append(args, memoryTypeInt)
	}

	if storageType != "" {
		conditions = append(conditions, "m.storage_type = ?")
		args = append(args, storageType)
	}

	if keywords != "" {
		conditions = append(conditions, "m.name LIKE ?")
		args = append(args, "%"+keywords+"%")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM memory m %s", whereClause)
	var total int64
	if err := DB.Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	querySQL := fmt.Sprintf(`
		SELECT m.id, m.name, m.avatar, m.tenant_id, m.memory_type,
			m.storage_type, m.embd_id, m.tenant_embd_id, m.llm_id, m.tenant_llm_id,
			m.permissions, m.description, m.memory_size, m.forgetting_policy,
			m.temperature, m.system_prompt, m.user_prompt, m.create_time, m.create_date,
			m.update_time, m.update_date,
			u.nickname as owner_name
		FROM memory m
		LEFT JOIN user u ON m.tenant_id = u.id
		%s
		ORDER BY m.update_time DESC
		LIMIT ? OFFSET ?
	`, whereClause)

	queryArgs := append(args, pageSize, offset)

	var rawResults []struct {
		model.Memory
		OwnerName *string `gorm:"column:owner_name"`
	}

	if err := DB.Raw(querySQL, queryArgs...).Scan(&rawResults).Error; err != nil {
		return nil, 0, err
	}

	memories := make([]*model.MemoryListItem, len(rawResults))
	for i, r := range rawResults {
		memories[i] = &model.MemoryListItem{
			Memory:    r.Memory,
			OwnerName: r.OwnerName,
		}
	}

	return memories, total, nil
}
