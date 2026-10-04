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

// GetByID returns the chat channel with the given id, ignoring tenant isolation.
// Callers must enforce tenant authorization themselves (see service.accessible).
func (dao *ChatChannelDAO) GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.ChatChannel, error) {
	var channel entity.ChatChannel
	if err := db.WithContext(ctx).Take(&channel, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

// UpdateByID updates the channel with the given id, but only when its tenant_id
// matches the expected value. A tenant mismatch is treated as not-found so the
// operation is a no-op rather than a cross-tenant write.
func (dao *ChatChannelDAO) UpdateByID(ctx context.Context, db *gorm.DB, id string, tenantID string, updates map[string]any) error {
	var channel entity.ChatChannel
	if err := db.WithContext(ctx).Where("id = ?", id).First(&channel).Error; err != nil {
		return err
	}
	if channel.TenantID != tenantID {
		return gorm.ErrRecordNotFound
	}
	return db.WithContext(ctx).Model(&entity.ChatChannel{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteByID deletes the channel with the given id, but only when its tenant_id
// matches the expected value. A tenant mismatch is treated as not-found so the
// operation is a no-op rather than a cross-tenant delete.
func (dao *ChatChannelDAO) DeleteByID(ctx context.Context, db *gorm.DB, id string, tenantID string) error {
	var channel entity.ChatChannel
	if err := db.WithContext(ctx).Where("id = ?", id).First(&channel).Error; err != nil {
		return err
	}
	if channel.TenantID != tenantID {
		return gorm.ErrRecordNotFound
	}
	return db.WithContext(ctx).Where("id = ?", id).Delete(&entity.ChatChannel{}).Error
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

// ListActive returns all enabled chat-channel rows for the runtime reconciler.
func (dao *ChatChannelDAO) ListActive(ctx context.Context, db *gorm.DB) ([]*entity.ChatChannel, error) {
	rows := make([]*entity.ChatChannel, 0)
	err := db.WithContext(ctx).Where("status = ?", 1).Find(&rows).Error
	return rows, err
}
