package dao

import (
	"context"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

type ChatChannelDAO struct{}

func NewChatChannel() *ChatChannelDAO {
	return &ChatChannelDAO{}
}

func (dao *ChatChannelDAO) Create(ctx context.Context, db *gorm.DB, channel *entity.ChatChannel) error {
	return db.WithContext(ctx).Create(channel).Error
}

func (dao *ChatChannelDAO) GetByIDOnly(ctx context.Context, db *gorm.DB, id string) (*entity.ChatChannel, error) {
	var channel entity.ChatChannel
	err := db.WithContext(ctx).Where("id = ?", id).First(&channel).Error
	if err != nil {
		return nil, err
	}
	return &channel, err
}

func (dao *ChatChannelDAO) GetByID(ctx context.Context, db *gorm.DB, id string, tenantID string) (*entity.ChatChannel, error) {
	var channel entity.ChatChannel
	err := db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).First(&channel).Error
	if err != nil {
		return nil, err
	}
	return &channel, err
}

// UpdateByID Update a single record by ID
func (dao *ChatChannelDAO) UpdateByID(ctx context.Context, db *gorm.DB, id string, tenantID string, updates map[string]any) error {
	return db.WithContext(ctx).Model(&entity.ChatChannel{}).Where("id = ? AND tenant_id = ?", id, tenantID).Updates(updates).Error
}

// DeleteByID Delete a single record by ID
func (dao *ChatChannelDAO) DeleteByID(ctx context.Context, db *gorm.DB, id string, tenantID string) error {
	return db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&entity.ChatChannel{}).Error
}

// ListByTenantID List a single record by TenantID
func (dao *ChatChannelDAO) ListByTenantID(ctx context.Context, db *gorm.DB, tenantID string) ([]*entity.ChatChannelListResponse, error) {
	results := make([]*entity.ChatChannelListResponse, 0)

	err := db.WithContext(ctx).Table("chat_channel").
		Select("chat_channel.id, chat_channel.name, chat_channel.channel, chat_channel.chat_id, chat_channel.status, dialog.name as dialog_name").
		Joins("LEFT JOIN dialog ON dialog.id = chat_channel.chat_id").
		Where("chat_channel.tenant_id = ?", tenantID).
		Order("chat_channel.create_time DESC").
		Scan(&results).Error

	return results, err
}
