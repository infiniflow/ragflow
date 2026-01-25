package dao

import (
	"ragflow/internal/model"
	"strings"
)

// KnowledgebaseDAO knowledge base data access object
type KnowledgebaseDAO struct{}

// NewKnowledgebaseDAO create knowledge base DAO
func NewKnowledgebaseDAO() *KnowledgebaseDAO {
	return &KnowledgebaseDAO{}
}

// ListByTenantIDs list knowledge bases by tenant IDs
func (dao *KnowledgebaseDAO) ListByTenantIDs(tenantIDs []string, userID string, page, pageSize int, orderby string, desc bool, keywords, parserID string) ([]*model.Knowledgebase, int64, error) {
	var kbs []*model.Knowledgebase
	var total int64

	query := DB.Model(&model.Knowledgebase{}).
		Joins("LEFT JOIN user ON knowledgebase.tenant_id = user.id").
		Where("(knowledgebase.tenant_id IN ? AND knowledgebase.permission = ?) OR knowledgebase.tenant_id = ?", tenantIDs, "team", userID).
		Where("knowledgebase.status = ?", "1")

	if keywords != "" {
		query = query.Where("LOWER(knowledgebase.name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}

	if parserID != "" {
		query = query.Where("knowledgebase.parser_id = ?", parserID)
	}

	// Order
	if desc {
		query = query.Order(orderby + " DESC")
	} else {
		query = query.Order(orderby + " ASC")
	}

	// Count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Pagination
	if page > 0 && pageSize > 0 {
		offset := (page - 1) * pageSize
		if err := query.Offset(offset).Limit(pageSize).Find(&kbs).Error; err != nil {
			return nil, 0, err
		}
	} else {
		if err := query.Find(&kbs).Error; err != nil {
			return nil, 0, err
		}
	}

	return kbs, total, nil
}

// ListByOwnerIDs list knowledge bases by owner IDs
func (dao *KnowledgebaseDAO) ListByOwnerIDs(ownerIDs []string, page, pageSize int, orderby string, desc bool, keywords, parserID string) ([]*model.Knowledgebase, int64, error) {
	var kbs []*model.Knowledgebase

	query := DB.Model(&model.Knowledgebase{}).
		Joins("LEFT JOIN user ON knowledgebase.tenant_id = user.id").
		Where("knowledgebase.tenant_id IN ?", ownerIDs).
		Where("knowledgebase.status = ?", "1")

	if keywords != "" {
		query = query.Where("LOWER(knowledgebase.name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}

	if parserID != "" {
		query = query.Where("knowledgebase.parser_id = ?", parserID)
	}

	// Order
	if desc {
		query = query.Order(orderby + " DESC")
	} else {
		query = query.Order(orderby + " ASC")
	}

	if err := query.Find(&kbs).Error; err != nil {
		return nil, 0, err
	}

	total := int64(len(kbs))

	// Manual pagination
	if page > 0 && pageSize > 0 {
		start := (page - 1) * pageSize
		end := start + pageSize
		if end > int(total) {
			end = int(total)
		}
		if start < end {
			kbs = kbs[start:end]
		} else {
			kbs = []*model.Knowledgebase{}
		}
	}

	return kbs, total, nil
}
