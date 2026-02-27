package dao

import (
	"strings"

	"ragflow/internal/model"
)

// ChatDAO chat data access object
type ChatDAO struct{}

// NewChatDAO create chat DAO
func NewChatDAO() *ChatDAO {
	return &ChatDAO{}
}

// ListByTenantID list chats by tenant ID
func (dao *ChatDAO) ListByTenantID(tenantID string, status string) ([]*model.Chat, error) {
	var chats []*model.Chat

	query := DB.Model(&model.Chat{}).
		Where("tenant_id = ?", tenantID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	// Order by create_time desc
	if err := query.Order("create_time DESC").Find(&chats).Error; err != nil {
		return nil, err
	}

	return chats, nil
}

// ListByTenantIDs list chats by tenant IDs with pagination and filtering
func (dao *ChatDAO) ListByTenantIDs(tenantIDs []string, userID string, page, pageSize int, orderby string, desc bool, keywords string) ([]*model.Chat, int64, error) {
	var chats []*model.Chat
	var total int64

	// Build query with join to user table for nickname and avatar
	query := DB.Model(&model.Chat{}).
		Select(`
			dialog.*,
			user.nickname,
			user.avatar as tenant_avatar
		`).
		Joins("LEFT JOIN user ON dialog.tenant_id = user.id").
		Where("(dialog.tenant_id IN ? OR dialog.tenant_id = ?) AND dialog.status = ?", tenantIDs, userID, "1")

	// Apply keyword filter
	if keywords != "" {
		query = query.Where("LOWER(dialog.name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}

	// Apply ordering
	orderDirection := "ASC"
	if desc {
		orderDirection = "DESC"
	}
	query = query.Order(orderby + " " + orderDirection)

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	if page > 0 && pageSize > 0 {
		offset := (page - 1) * pageSize
		if err := query.Offset(offset).Limit(pageSize).Find(&chats).Error; err != nil {
			return nil, 0, err
		}
	} else {
		if err := query.Find(&chats).Error; err != nil {
			return nil, 0, err
		}
	}

	return chats, total, nil
}

// ListByOwnerIDs list chats by owner IDs with filtering (manual pagination)
func (dao *ChatDAO) ListByOwnerIDs(ownerIDs []string, userID string, orderby string, desc bool, keywords string) ([]*model.Chat, int64, error) {
	var chats []*model.Chat

	// Build query with join to user table
	query := DB.Model(&model.Chat{}).
		Select(`
			dialog.*,
			user.nickname,
			user.avatar as tenant_avatar
		`).
		Joins("LEFT JOIN user ON dialog.tenant_id = user.id").
		Where("(dialog.tenant_id IN ? OR dialog.tenant_id = ?) AND dialog.status = ?", ownerIDs, userID, "1")

	// Apply keyword filter
	if keywords != "" {
		query = query.Where("LOWER(dialog.name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}

	// Filter by owner IDs (additional filter to ensure tenant_id is in ownerIDs)
	query = query.Where("dialog.tenant_id IN ?", ownerIDs)

	// Apply ordering
	orderDirection := "ASC"
	if desc {
		orderDirection = "DESC"
	}
	query = query.Order(orderby + " " + orderDirection)

	// Get all matching records
	if err := query.Find(&chats).Error; err != nil {
		return nil, 0, err
	}

	total := int64(len(chats))

	return chats, total, nil
}

// GetByID gets chat by ID
func (dao *ChatDAO) GetByID(id string) (*model.Chat, error) {
	var chat model.Chat
	err := DB.Where("id = ?", id).First(&chat).Error
	if err != nil {
		return nil, err
	}
	return &chat, nil
}

// GetByIDAndStatus gets chat by ID and status
func (dao *ChatDAO) GetByIDAndStatus(id string, status string) (*model.Chat, error) {
	var chat model.Chat
	err := DB.Where("id = ? AND status = ?", id, status).First(&chat).Error
	if err != nil {
		return nil, err
	}
	return &chat, nil
}
