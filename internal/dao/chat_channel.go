package dao

import "ragflow/internal/entity"

type ChatChannelDAO struct{}

func NewChatChannel() *ChatChannelDAO {
	return &ChatChannelDAO{}
}

func (dao *ChatChannelDAO) Create(channel *entity.ChatChannel) error {
	return DB.Create(channel).Error
}

func (dao *ChatChannelDAO) GetByIDOnly(id string) (*entity.ChatChannel, error) {
	var channel entity.ChatChannel
	err := DB.Where("id = ?", id).First(&channel).Error
	if err != nil {
		return nil, err
	}
	return &channel, err
}

func (dao *ChatChannelDAO) GetByID(id string, tenantID string) (*entity.ChatChannel, error) {
	var channel entity.ChatChannel
	err := DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&channel).Error
	if err != nil {
		return nil, err
	}
	return &channel, err
}

// UpdateByID Update a single record by ID
func (dao *ChatChannelDAO) UpdateByID(id string, tenantID string, updates map[string]any) error {
	return DB.Model(&entity.ChatChannel{}).Where("id = ? AND tenant_id = ?", id, tenantID).Updates(updates).Error
}

// DeleteByID Delete a single record by ID
func (dao *ChatChannelDAO) DeleteByID(id string, tenantID string) error {
	return DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&entity.ChatChannel{}).Error
}

// ListByTenantID List a single record by TenantID
func (dao *ChatChannelDAO) ListByTenantID(tenantID string) ([]*entity.ChatChannelListResponse, error) {
	results := make([]*entity.ChatChannelListResponse, 0)

	err := DB.Table("chat_channel").
		Select("chat_channel.id, chat_channel.name, chat_channel.channel, chat_channel.chat_id, chat_channel.status, dialog.name as dialog_name").
		Joins("LEFT JOIN dialog ON dialog.id = chat_channel.chat_id").
		Where("chat_channel.tenant_id = ?", tenantID).
		Order("chat_channel.create_time DESC").
		Scan(&results).Error

	return results, err
}
