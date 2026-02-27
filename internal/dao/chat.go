package dao

import (
	"fmt"
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

// GetExistingNames gets existing dialog names for a tenant
func (dao *ChatDAO) GetExistingNames(tenantID string, status string) ([]string, error) {
	var names []string
	err := DB.Model(&model.Chat{}).
		Where("tenant_id = ? AND status = ?", tenantID, status).
		Pluck("name", &names).Error
	return names, err
}

// Create creates a new chat/dialog
func (dao *ChatDAO) Create(chat *model.Chat) error {
	return DB.Create(chat).Error
}

// UpdateByID updates a chat by ID
func (dao *ChatDAO) UpdateByID(id string, updates map[string]interface{}) error {
	return DB.Model(&model.Chat{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateManyByID updates multiple chats by ID (batch update)
func (dao *ChatDAO) UpdateManyByID(updates []map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	// Use transaction for batch update
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	for _, update := range updates {
		id, ok := update["id"].(string)
		if !ok {
			tx.Rollback()
			return fmt.Errorf("invalid id in update")
		}

		// Remove id from updates map
		updatesWithoutID := make(map[string]interface{})
		for k, v := range update {
			if k != "id" {
				updatesWithoutID[k] = v
			}
		}

		if err := tx.Model(&model.Chat{}).Where("id = ?", id).Updates(updatesWithoutID).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}
