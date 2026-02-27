package dao

import (
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
